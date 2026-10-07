package masterstructural

import (
	"fmt"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/master"
)

const portableEnvelopeAllowance uint64 = 4096

// NumericalProvenance identifies arithmetic, not market price quantization.
type NumericalProvenance struct {
	Contract            string `json:"contract"`
	Rounding            string `json:"rounding"`
	ImplicitContraction bool   `json:"implicitContraction"`
	TickQuantization    bool   `json:"tickQuantization"`
}

type PortableDocument struct {
	Schema     string              `json:"schema"`
	Arithmetic NumericalProvenance `json:"arithmetic"`
	DSLHash    string              `json:"dslSha256"`
	ConfigHash string              `json:"configSha256"`
	DataHash   string              `json:"dataSha256"`
	Config     dsl.Config          `json:"config"`
	Run        master.Result       `json:"run"`
}

func portableDocument(source, cfg, data []byte, config dsl.Config, result master.Result) PortableDocument {
	return PortableDocument{
		Schema:     "strat-master-structural-portable-cli-v1",
		Arithmetic: NumericalProvenance{Contract: master.PortableArithmeticContract, Rounding: "IEEE-754 binary64 round-to-nearest ties-to-even after each evaluated arithmetic operation", ImplicitContraction: false, TickQuantization: false},
		DSLHash:    hash(source), ConfigHash: hash(cfg), DataHash: hash(data), Config: config, Run: result,
	}
}

// BuildPortableV1 is the only bounded portable native/WASM report builder.
// Its caller must bound transport conversion/read allocations as well.
func BuildPortableV1(rawMeta, source string, data []byte) ([]byte, error) {
	options, err := DecodeRuntimeRequest(rawMeta)
	if err != nil {
		return nil, err
	}
	if len(source) > MaxSourceBytes || !utf8.ValidString(source) {
		return nil, fmt.Errorf("master runtime source must be valid UTF-8 within %d bytes", MaxSourceBytes)
	}
	if err = ValidateRuntimeHeader(data, len(data)); err != nil {
		return nil, err
	}
	return build([]byte(source), data, options, true)
}
