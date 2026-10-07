package adaptiveflagunit

import (
	"encoding/binary"
	"encoding/json"
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

// PreparedRuntime owns admitted value-only metadata and one prepared Stage A
// source. No caller-supplied ledger or mutable config is an entrypoint.
type PreparedRuntime struct {
	rawPrepared                          *adaptiveflag.PreparedRuntime
	rawRequest                           adaptiveflag.RuntimeRequest
	request                              Request
	requestSHA, rawRequestSHA, sourceSHA string
}

// ErrorJSON delegates to the frozen closed error implementation.
func ErrorJSON(err error) string { return errorJSON(err) }

// Rejection supplies bounded, closed transport failures without exposing core
// constructors or permitting arbitrary error codes/phases/locations.
func Rejection(code, operation, message string) error {
	return newCoreError(code, operation, -1, -1, -1, message)
}

// ProjectionMetadataTransportError maps the unchanged Stage A bounded-string
// helper's request errors to the new metadata role, preserving resource refusals.
func ProjectionMetadataTransportError(err error) error {
	detail, ok := inheritedErrorDetail(adaptiveflag.ErrorJSON(err))
	if !ok {
		return Rejection("internal_error", "transport", "invalid projection metadata transport failure")
	}
	if detail.Code == "resource_limit" {
		return Rejection("resource_limit", "projection_request", detail.Message)
	}
	return Rejection("projection_request_rejected", "projection_request", detail.Message)
}

func PrepareRuntime(rawMetadata, projectionMetadata, source string) (*PreparedRuntime, error) {
	// Preserve Stage A decoding/strict source behavior. The independent unit
	// request is also admitted before any BTB1 header or transport copy.
	rawRequest, err := adaptiveflag.DecodeRuntimeRequest(rawMetadata)
	if err != nil {
		return nil, err
	}
	request, err := DecodeRuntimeRequest(projectionMetadata)
	if err != nil {
		return nil, err
	}
	rawPrepared, err := adaptiveflag.PrepareRuntime(rawMetadata, source)
	if err != nil {
		return nil, err
	}
	if rawRequest.ExecutionWindow != nil {
		w := rawRequest.ExecutionWindow
		if _, err := runtimeCalendarDays(w.TradeFromMS, w.TradeToMS); err != nil {
			return nil, err
		}
	}
	return &PreparedRuntime{rawPrepared, rawRequest, request, rawBytesSHA([]byte(projectionMetadata)), rawBytesSHA([]byte(rawMetadata)), rawBytesSHA([]byte(source))}, nil
}

func BuildRuntime(rawMetadata, projectionMetadata, source string, data []byte) ([]byte, error) {
	prepared, err := PrepareRuntime(rawMetadata, projectionMetadata, source)
	if err != nil {
		return nil, err
	}
	return prepared.Build(data)
}

func runtimeCalendarDays(start, end int64) (int, error) {
	if start < 0 || start >= MaxCalendarEndpointMS || end <= start || end > MaxCalendarEndpointMS {
		return 0, Rejection("projection_domain_rejected", "time_domain", "unit window is outside the UTC 1970..9999 domain")
	}
	days := (end-1)/unitDayMS - start/unitDayMS + 1
	if days < 1 || days > MaxCalendarDays {
		return 0, Rejection("resource_limit", "calendar_count", "unit window exceeds 366 UTC calendar days")
	}
	return int(days), nil
}

// preflight inspects only the bounded timestamp column, before raw engine
// allocation. Stage A still performs its unchanged full retained OHLCV checks.
func (p *PreparedRuntime) preflight(data []byte) (int, int, uint64, error) {
	if err := adaptiveflag.ValidateRuntimeHeader(data, len(data)); err != nil {
		return 0, 0, 0, err
	}
	n := int(binary.LittleEndian.Uint32(data[8:12]))
	step := int64(1800000)
	if p.rawRequest.Timeframe == "H1" {
		step = 3600000
	}
	retained := 0
	var first, last int64
	previous := -1.0
	for row := 0; row < n; row++ {
		at := 16 + row*8
		stamp := math.Float64frombits(binary.LittleEndian.Uint64(data[at : at+8]))
		// Deliberately before finite/grid/calendar validation: an ignored future
		// boundary (including +Inf) and its suffix receive no certification.
		if p.rawRequest.ExecutionWindow != nil && stamp >= float64(p.rawRequest.ExecutionWindow.TradeToMS) {
			break
		}
		if retained == MaxRetainedRows {
			return 0, 0, 0, Rejection("resource_limit", "output_bound", "unit retained prefix exceeds 1024 rows including warmup; no rows were trimmed")
		}
		if math.IsNaN(stamp) || math.IsInf(stamp, 0) || stamp < 0 || stamp > float64(int64(9007199254740991)-step) || math.Trunc(stamp) != stamp || int64(stamp)%step != 0 || stamp <= previous {
			return 0, 0, 0, newCoreError("raw_input_rejected", "time_domain", row, -1, -1, "invalid retained BTB1 timestamp")
		}
		if stamp > float64(MaxCalendarEndpointMS-step) {
			return 0, 0, 0, newCoreError("projection_domain_rejected", "time_domain", row, -1, -1, "retained nominal close is beyond the UTC year 9999 endpoint")
		}
		if retained == 0 {
			first = int64(stamp)
		}
		last = int64(stamp)
		previous = stamp
		retained++
	}
	if retained == 0 {
		return 0, 0, 0, Rejection("raw_input_rejected", "time_domain", "unit runtime requires a nonempty retained prefix")
	}
	start, end := first, last+step
	if w := p.rawRequest.ExecutionWindow; w != nil {
		start, end = w.TradeFromMS, w.TradeToMS
	}
	days, err := runtimeCalendarDays(start, end)
	if err != nil {
		return 0, 0, 0, err
	}
	// The worst permitted E/G reserve is conservative before a native lifecycle
	// exists. The frozen core performs actual count-only E/G admission later.
	limit, err := GeneratedOutputBound(uint64(retained), uint64(days), MaxExposureMidnights, MaxExposureGaps)
	if err != nil || limit > MaxOutputBytes {
		return 0, 0, 0, Rejection("resource_limit", "output_bound", "unit conservative output bound exceeds 64 MiB")
	}
	return retained, days, limit, nil
}

// Build creates exactly one fresh Stage A document, privately decodes that owned
// document, and derives all projected values and identities from that same pair.
// Returning bytes exposes neither raw/core structs nor input/config aliases.
func (p *PreparedRuntime) Build(data []byte) (output []byte, err error) {
	defer func() {
		if recover() != nil {
			output = nil
			err = Rejection("internal_error", "", "adaptive unit runtime failed")
		}
	}()
	if p == nil || p.rawPrepared == nil {
		return nil, Rejection("projection_request_rejected", "projection_request", "unit source was not prepared")
	}
	n, _, limit, err := p.preflight(data)
	if err != nil {
		return nil, err
	}
	raw, err := p.rawPrepared.Build(data) // Sole production raw strategy call.
	if err != nil {
		return nil, err
	}
	rawLimit, err := adaptiveflag.RuntimeOutputBound(uint64(n))
	if err != nil || uint64(len(raw)) > rawLimit || uint64(len(raw)) > limit {
		return nil, Rejection("resource_limit", "raw_build", "owned Stage A bytes exceed the admitted bound")
	}
	var document struct {
		Schema     string                      `json:"schema"`
		Request    adaptiveflag.RuntimeRequest `json:"request"`
		RequestSHA string                      `json:"requestSha256"`
		DSL        string                      `json:"dslSha256"`
		ConfigSHA  string                      `json:"configSha256"`
		BTB1       string                      `json:"btb1Sha256"`
		RawOnly    bool                        `json:"rawOnly"`
		Economics  string                      `json:"economics"`
		Config     dsl.Config                  `json:"config"`
		Run        engine.AdaptiveFlagResult   `json:"run"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, Rejection("internal_error", "raw_decode", "owned Stage A document could not be decoded")
	}
	if document.Schema != adaptiveflag.RuntimeEnvelopeSchema || !document.RawOnly || document.Economics != "unavailable-stage-a" || document.RequestSHA != p.rawRequestSHA || document.DSL != p.sourceSHA || document.BTB1 != rawBytesSHA(data) || len(document.Run.Snapshots) != n {
		return nil, Rejection("internal_error", "identity", "owned Stage A identity mismatch")
	}
	owned := ownedRaw{document.Run, raw, document.DSL, document.ConfigSHA, document.BTB1}
	projection, err := projectCore(owned, p.request)
	if err != nil {
		return nil, err
	}
	// Frozen type/count proofs bound these intermediate serializers before they
	// allocate. Raw bytes are never given to json.Marshal or MarshalIndent.
	projected, err := json.Marshal(projection)
	if err != nil {
		return nil, Rejection("internal_error", "serialize", "unit projection could not be serialized")
	}
	rawSHA := rawBytesSHA(raw)
	header, err := json.Marshal(struct {
		Schema               string  `json:"schema"`
		ProjectionRequest    Request `json:"projectionRequest"`
		ProjectionRequestSHA string  `json:"projectionRequestSha256"`
		RawEnvelopeSHA       string  `json:"rawEnvelopeSha256"`
	}{EnvelopeSchema, p.request, p.requestSHA, rawSHA})
	if err != nil {
		return nil, Rejection("internal_error", "serialize", "unit header could not be serialized")
	}
	const rawKey = `,"raw":`
	const projectionKey = `,"projection":`
	size := uint64(len(header)-1) + uint64(len(rawKey)) + uint64(len(raw)) + uint64(len(projectionKey)) + uint64(len(projected)) + 2
	if size > limit || size > MaxOutputBytes {
		return nil, Rejection("resource_limit", "serialize", "unit composed output exceeds its conservative bound")
	}
	output = make([]byte, 0, int(size))
	output = append(output, header[:len(header)-1]...)
	output = append(output, rawKey...)
	output = append(output, raw...)
	output = append(output, projectionKey...)
	output = append(output, projected...)
	output = append(output, '}', '\n')
	if uint64(len(output)) != size {
		return nil, Rejection("internal_error", "serialize", "unit fixed composition length mismatch")
	}
	return output, nil
}
