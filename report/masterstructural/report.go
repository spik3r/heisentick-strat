// Package masterstructural shares the fixed Master reference report between
// the native CLI and a separately bounded WASM transport. It is not a broker.
package masterstructural

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/master"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Options preserves the native CLI's explicit historical endpoints and costs.
type Options struct {
	WarmupFromT, TradeFromT, TradeToT int64
	Spread                            float64
}

// Document deliberately preserves the original CLI field order and schema.
// The schema name predates the dedicated WASM adapter; its meaning is unchanged.
type Document struct {
	Schema     string        `json:"schema"`
	DSLHash    string        `json:"dslSha256"`
	ConfigHash string        `json:"configSha256"`
	DataHash   string        `json:"dataSha256"`
	Config     dsl.Config    `json:"config"`
	Run        master.Result `json:"run"`
}

// Build returns the complete original CLI JSON document, including its final
// newline. Validation and serialization finish before callers receive bytes.
// Native input limits are unchanged; portable transport limits belong to BuildPortableV1.
func Build(source, data []byte, options Options) ([]byte, error) {
	return build(source, data, options, false)
}

func build(source, data []byte, options Options, bounded bool) ([]byte, error) {
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		return nil, err
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("master DSL parse errors: %s", strings.Join(parsed.Errors, "; "))
	}
	if _, err = dsl.DecodeMasterStructural(parsed.Config); err != nil {
		return nil, err
	}
	// Preserve the original six-column/exact-length CLI admission boundary.
	if len(data) < 16 || binary.LittleEndian.Uint32(data[12:16]) != 6 {
		return nil, fmt.Errorf("master requires exactly six BTB1 OHLCV columns")
	}
	count := uint64(binary.LittleEndian.Uint32(data[8:12]))
	if uint64(len(data)) != 16+count*6*8 {
		return nil, fmt.Errorf("master BTB1 byte length mismatch")
	}
	cfg, err := json.Marshal(parsed.Config)
	if err != nil {
		return nil, err
	}
	if bounded {
		// This runs before decoding six float columns, building indicators or
		// master.Run's own JSON validity check. It is an upper bound, not an
		// estimate based on average row size or an assumed six rows per bar.
		if err = preflightRuntime(data, parsed.Config, options); err != nil {
			return nil, err
		}
	}
	series, err := marketdata.DecodeBTB1(data)
	if err != nil {
		return nil, err
	}
	request := master.Request{Config: parsed.Config, M5: series, WarmupFromT: options.WarmupFromT, TradeFromT: options.TradeFromT, TradeToT: options.TradeToT, Costs: master.Costs{Spread: options.Spread, FeePerUnitSide: .5, InitialEquity: 10000}}
	var result master.Result
	if bounded {
		result, err = master.RunPortableV1(request)
	} else {
		result, err = master.Run(request)
	}
	if err != nil {
		return nil, err
	}
	envelope := Document{"strat-master-structural-cli-v1", hash(source), hash(cfg), hash(data), parsed.Config, result}
	var document any = envelope
	if bounded {
		document = portableDocument(source, cfg, data, parsed.Config, result)
	}
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	if bounded && len(raw) > MaxOutputBytes {
		// Defense in depth. Admission was already bounded before Run.
		return nil, fmt.Errorf("master runtime output bound violated")
	}
	return raw, nil
}

func hash(raw []byte) string {
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
