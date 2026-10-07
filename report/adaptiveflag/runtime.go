package adaptiveflag

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

const MaxSourceBytes = dsl.AdaptiveFlagRuntimeMaxSourceBytes

type PreparedRuntime struct {
	request     RuntimeRequest
	source      string
	config      dsl.Config
	requestHash string
	ready       bool
}

// PrepareRuntime completes strict source/profile admission before any full BTB1
// copy. Generic dsl.Parse and its foreign-family allocation paths are not used.
func PrepareRuntime(metadata, source string) (*PreparedRuntime, error) {
	request, err := DecodeRuntimeRequest(metadata)
	if err != nil {
		return nil, err
	}
	if len(source) > MaxSourceBytes {
		return nil, Rejection("resource", "adaptive runtime source exceeds 4096 bytes")
	}
	if !utf8.ValidString(source) {
		return nil, Rejection("source", "adaptive runtime source requires valid UTF-8")
	}
	parsed, err := dsl.ParseAdaptiveVolumeFlagRuntimeSource(source)
	if err != nil {
		return nil, Rejection("source", err.Error())
	}
	if len(parsed.Errors) > 0 {
		return nil, Rejection("source", "adaptive flag DSL parse errors: "+strings.Join(parsed.Errors, "; "))
	}
	for _, diagnostic := range parsed.Diagnostics {
		if diagnostic.Severity == dsl.DiagnosticError {
			return nil, Rejection("source", diagnostic.Message)
		}
	}
	spec, err := dsl.DecodeAdaptiveVolumeFlag(parsed.Config)
	if err != nil {
		return nil, Rejection("source", err.Error())
	}
	if spec.Bundle != "INITIAL" && spec.Bundle != "TWEAKED" && spec.Bundle != "SNAPSHOT_C" {
		return nil, Rejection("source", "adaptive runtime requires exact INITIAL, TWEAKED or SNAPSHOT_C; CUSTOM and research overlays are unsupported")
	}
	if spec.Timeframe != request.Timeframe {
		return nil, Rejection("source", "adaptive runtime source timeframe differs from request")
	}
	return &PreparedRuntime{request: request, source: source, config: parsed.Config, requestHash: hash([]byte(metadata)), ready: true}, nil
}

func BuildRuntime(metadata, source string, data []byte) ([]byte, error) {
	prepared, err := PrepareRuntime(metadata, source)
	if err != nil {
		return nil, err
	}
	return prepared.Build(data)
}

type runtimeDocument struct {
	Schema        string                    `json:"schema"`
	Request       RuntimeRequest            `json:"request"`
	RequestSHA256 string                    `json:"requestSha256"`
	DSLSHA256     string                    `json:"dslSha256"`
	ConfigSHA256  string                    `json:"configSha256"`
	BTB1SHA256    string                    `json:"btb1Sha256"`
	RawOnly       bool                      `json:"rawOnly"`
	Economics     string                    `json:"economics"`
	Config        dsl.Config                `json:"config"`
	Run           engine.AdaptiveFlagResult `json:"run"`
}

func (p *PreparedRuntime) Build(data []byte) ([]byte, error) {
	if p == nil || !p.ready {
		return nil, Rejection("request", "adaptive runtime source was not prepared")
	}
	if err := ValidateRuntimeHeader(data, len(data)); err != nil {
		return nil, err
	}
	retained, err := p.preflightRows(data)
	if err != nil {
		return nil, err
	}
	limit, err := RuntimeOutputBound(uint64(retained))
	if err != nil {
		return nil, Rejection("resource", err.Error())
	}
	if limit > MaxOutputBytes {
		return nil, Rejection("resource", "adaptive runtime conservative output bound exceeds 128 MiB")
	}
	// The exact input, retained-prefix and output bounds are checked before the
	// engine decodes columns, constructs snapshots or marshals intermediate JSON.
	result, err := runRaw(p.config, data, Options{Window: p.request.ExecutionWindow})
	if err != nil {
		return nil, Rejection("execution", err.Error())
	}
	if result.ResearchPolicy != nil || result.ResearchPolicySHA256 != "" || result.ResearchProducer != nil {
		return nil, Rejection("execution", "adaptive baseline runtime rejected research output")
	}
	config, err := json.Marshal(p.config)
	if err != nil {
		return nil, Rejection("execution", err.Error())
	}
	envelope := runtimeDocument{RuntimeEnvelopeSchema, p.request, p.requestHash, hash([]byte(p.source)), hash(config), hash(data), true, "unavailable-stage-a", p.config, result}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, Rejection("execution", err.Error())
	}
	if uint64(len(raw)+1) > limit || len(raw)+1 > MaxOutputBytes {
		return nil, Rejection("resource", "adaptive runtime conservative output bound violated")
	}
	return append(raw, '\n'), nil
}

// ValidateRuntimeHeader can inspect a header-only slice before allocating the
// full transport. actualBytes must be the selected view/complete native length.
func ValidateRuntimeHeader(header []byte, actualBytes int) error {
	if actualBytes < 16 || actualBytes > MaxInputBytes || len(header) < 16 {
		return Rejection("resource", "adaptive runtime BTB1 length outside bounded profile")
	}
	if binary.LittleEndian.Uint32(header[:4]) != 0x31425442 || binary.LittleEndian.Uint32(header[4:8]) != 1 {
		return Rejection("input", "adaptive runtime requires BTB1 version 1")
	}
	count := uint64(binary.LittleEndian.Uint32(header[8:12]))
	if count == 0 || count > MaxProvidedRows || binary.LittleEndian.Uint32(header[12:16]) != 6 {
		return Rejection("input", "adaptive runtime requires 1..16384 supplied rows and exactly six columns")
	}
	if uint64(actualBytes) != 16+count*6*8 {
		return Rejection("input", "adaptive runtime BTB1 exact byte length mismatch")
	}
	return nil
}

func (p *PreparedRuntime) preflightRows(data []byte) (int, error) {
	n := int(binary.LittleEndian.Uint32(data[8:12]))
	value := func(col, row int) float64 {
		at := 16 + (col*n+row)*8
		return math.Float64frombits(binary.LittleEndian.Uint64(data[at : at+8]))
	}
	step := int64(1800000)
	if p.request.Timeframe == "H1" {
		step = 3600000
	}
	previous := -1.
	retained := 0
	finite := func(x float64) bool { return !math.IsInf(x, 0) && !math.IsNaN(x) }
	for row := 0; row < n; row++ {
		timestamp := value(0, row)
		// Keep the original cutoff-before-validation order, including an ignored
		// +Inf boundary timestamp. Ignored suffix values are not certified.
		if p.request.ExecutionWindow != nil && timestamp >= float64(p.request.ExecutionWindow.TradeToMS) {
			break
		}
		if retained == MaxRetainedRows {
			return 0, Rejection("resource", "adaptive runtime retained prefix exceeds 4096 rows; no rows were trimmed")
		}
		if !finite(timestamp) || timestamp < 0 || timestamp > float64(maxExactInteger-step) || math.Trunc(timestamp) != timestamp || int64(timestamp)%step != 0 || timestamp <= previous {
			return 0, Rejection("input", fmt.Sprintf("adaptive runtime invalid retained timestamp at row %d", row))
		}
		previous = timestamp
		values := [5]float64{}
		for col := 1; col < 6; col++ {
			v := value(col, row)
			if !finite(v) || math.Abs(v) > MaxInputMagnitude {
				return 0, Rejection("input", fmt.Sprintf("adaptive runtime retained OHLCV outside finite magnitude domain at row %d", row))
			}
			values[col-1] = v
		}
		o, h, l, c, v := values[0], values[1], values[2], values[3], values[4]
		if l <= 0 || o < l || o > h || c < l || c > h || v < 0 {
			return 0, Rejection("input", fmt.Sprintf("adaptive runtime invalid retained OHLCV at row %d", row))
		}
		retained++
	}
	if retained == 0 {
		return 0, Rejection("input", "adaptive runtime requires a nonempty retained prefix")
	}
	return retained, nil
}
