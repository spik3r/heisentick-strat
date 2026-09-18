// Command dslwasm wraps the native Go DSL parser (backtester/go/dsl) in a
// versioned JSON envelope so it can eventually run in the browser via
// WebAssembly and replace the JavaScript parser. This file is platform-neutral
// (it builds for native and js/wasm alike); the entry points live in main.go
// (native CLI) and main_wasm.go (WASM export). No browser or product wiring is
// done here — that integration is intentionally deferred.
package main

import (
	"encoding/json"

	"github.com/spik3r/heisentick-strat/dsl"
)

// EnvelopeVersion is the schema version of the parse envelope. Bump it on any
// breaking change to the envelope shape so browser callers can guard on it.
const EnvelopeVersion = 1

// EnvelopeAPIVersion is the browser-facing contract version. It is separate
// from EnvelopeVersion so callers can explicitly reject an incompatible parser
// API even when an envelope remains JSON-decodable.
const EnvelopeAPIVersion = 1

// Envelope is a versioned, language-agnostic wrapper around a DSL parse result,
// suitable for crossing the WASM boundary as JSON.
//
// A hard parser failure (rare) is reported via OK=false + Error and no Result.
// Ordinary DSL problems are NOT hard failures: they surface inside
// Result.Errors / Result.Diagnostics with OK=true, mirroring how the JS parser
// returns a result object even for invalid source.
type Envelope struct {
	Version     int              `json:"version"`
	APIVersion  int              `json:"apiVersion"`
	GrammarHash string           `json:"grammarHash"`
	OK          bool             `json:"ok"`
	Result      *dsl.ParseResult `json:"result,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// ParseEnvelope parses DSL source and wraps the outcome in a versioned envelope.
func ParseEnvelope(source string) Envelope {
	envelope := Envelope{
		Version:     EnvelopeVersion,
		APIVersion:  EnvelopeAPIVersion,
		GrammarHash: dsl.GrammarManifestSHA256,
	}
	result, err := dsl.Parse(source)
	if err != nil {
		envelope.Error = err.Error()
		return envelope
	}
	envelope.OK = true
	envelope.Result = &result
	return envelope
}

// ParseEnvelopeJSON returns the marshaled envelope bytes for the given source.
func ParseEnvelopeJSON(source string) ([]byte, error) {
	return json.Marshal(ParseEnvelope(source))
}
