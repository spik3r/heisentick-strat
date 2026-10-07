// Package adaptiveflag shares raw adaptive reference reports between native
// transports and a separately bounded WASM route. It is not a strategy engine.
package adaptiveflag

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

type Options struct {
	Window           *engine.AdaptiveFlagExecutionWindow
	ResearchAblation engine.AdaptiveFlagResearchAblation
}

type PreparedLegacy struct {
	source  []byte
	config  dsl.Config
	options Options
	ready   bool
}

func PrepareLegacy(source []byte, options Options) (*PreparedLegacy, error) {
	owned := append([]byte(nil), source...)
	parsed, err := dsl.Parse(string(owned))
	if err != nil {
		return nil, err
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("adaptive flag DSL parse errors: %s", strings.Join(parsed.Errors, "; "))
	}
	if _, err = dsl.DecodeAdaptiveVolumeFlag(parsed.Config); err != nil {
		return nil, err
	}
	if options.Window != nil {
		window := *options.Window
		options.Window = &window
	}
	return &PreparedLegacy{source: owned, config: parsed.Config, options: options, ready: true}, nil
}

func BuildLegacy(source, data []byte, options Options) ([]byte, error) {
	prepared, err := PrepareLegacy(source, options)
	if err != nil {
		return nil, err
	}
	return prepared.Build(data)
}

// legacyDocument preserves the existing CLI schema, field order and bytes.
type legacyDocument struct {
	Schema       string                    `json:"schema"`
	DSLSHA256    string                    `json:"dslSha256"`
	ConfigSHA256 string                    `json:"configSha256"`
	BTB1SHA256   string                    `json:"btb1Sha256"`
	Config       dsl.Config                `json:"config"`
	Run          engine.AdaptiveFlagResult `json:"run"`
}

func (p *PreparedLegacy) Build(data []byte) ([]byte, error) {
	if p == nil || !p.ready {
		return nil, fmt.Errorf("adaptive legacy source was not prepared")
	}
	result, err := runRaw(p.config, data, p.options)
	if err != nil {
		return nil, err
	}
	config, err := json.Marshal(p.config)
	if err != nil {
		return nil, err
	}
	envelope := legacyDocument{"strat-adaptive-volume-flag-cli-v1", hash(p.source), hash(config), hash(data), p.config, result}
	if result.ResearchPolicy != nil {
		envelope.Schema = engine.AdaptiveFlagResearchCLISchema
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func runRaw(config dsl.Config, data []byte, options Options) (engine.AdaptiveFlagResult, error) {
	if len(data) < 16 || binary.LittleEndian.Uint32(data[12:16]) != 6 {
		return engine.AdaptiveFlagResult{}, fmt.Errorf("adaptive flag requires exactly six BTB1 OHLCV columns")
	}
	count := uint64(binary.LittleEndian.Uint32(data[8:12]))
	if uint64(len(data)) != 16+count*6*8 {
		return engine.AdaptiveFlagResult{}, fmt.Errorf("adaptive flag BTB1 exact byte length mismatch")
	}
	series, err := marketdata.DecodeBTB1(data)
	if err != nil {
		return engine.AdaptiveFlagResult{}, err
	}
	return engine.RunAdaptiveVolumeFlag(engine.AdaptiveFlagRequest{Config: config, Series: series, Window: options.Window, ResearchAblation: options.ResearchAblation})
}
func hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
