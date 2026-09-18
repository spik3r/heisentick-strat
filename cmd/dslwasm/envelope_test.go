package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/testsupport"
)

// conformanceCase points at a paired .strat / .cfg.json fixture in the shared
// DSL conformance corpus, reused here as a golden so the WASM envelope stays
// tied to the same parser-output contract as the JS and Go parsers.
const conformanceCase = "setup-channel-break-hold"

func conformanceDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(testsupport.StratConformanceRoot(), "parse")
}

func TestParseEnvelopeWrapsResult(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(conformanceDir(t), conformanceCase+".strat"))
	if err != nil {
		t.Fatalf("read fixture source: %v", err)
	}
	env := ParseEnvelope(string(source))
	if env.Version != EnvelopeVersion {
		t.Fatalf("version = %d, want %d", env.Version, EnvelopeVersion)
	}
	if env.APIVersion != EnvelopeAPIVersion {
		t.Fatalf("apiVersion = %d, want %d", env.APIVersion, EnvelopeAPIVersion)
	}
	if env.GrammarHash != dsl.GrammarManifestSHA256 {
		t.Fatalf("grammarHash = %q, want canonical grammar manifest hash %q", env.GrammarHash, dsl.GrammarManifestSHA256)
	}
	if !env.OK {
		t.Fatalf("ok = false, error = %q", env.Error)
	}
	if env.Result == nil {
		t.Fatal("result is nil for a valid source")
	}
	if len(env.Result.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", env.Result.Errors)
	}
}

func TestParseEnvelopeMatchesConformanceCfg(t *testing.T) {
	dir := conformanceDir(t)
	source, err := os.ReadFile(filepath.Join(dir, conformanceCase+".strat"))
	if err != nil {
		t.Fatalf("read fixture source: %v", err)
	}
	goldenBytes, err := os.ReadFile(filepath.Join(dir, conformanceCase+".cfg.json"))
	if err != nil {
		t.Fatalf("read fixture golden: %v", err)
	}
	var golden struct {
		Cfg map[string]any `json:"cfg"`
	}
	if err := json.Unmarshal(goldenBytes, &golden); err != nil {
		t.Fatalf("decode golden: %v", err)
	}

	env := ParseEnvelope(string(source))
	if env.Result == nil {
		t.Fatal("result is nil")
	}
	// Normalize the parsed config through JSON so numeric types match the golden
	// (which came from json.Unmarshal into any).
	var got map[string]any
	cfgBytes, err := json.Marshal(env.Result.Config)
	if err != nil {
		t.Fatalf("marshal parsed cfg: %v", err)
	}
	if err := json.Unmarshal(cfgBytes, &got); err != nil {
		t.Fatalf("normalize parsed cfg: %v", err)
	}
	if !reflect.DeepEqual(got, golden.Cfg) {
		t.Fatalf("envelope cfg does not match conformance golden for %s", conformanceCase)
	}
}

func TestParseEnvelopeJSONIsVersionedAndValid(t *testing.T) {
	out, err := ParseEnvelopeJSON("setup failed breakout of prior day high")
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("envelope JSON is not valid: %v", err)
	}
	if v, ok := decoded["version"].(float64); !ok || int(v) != EnvelopeVersion {
		t.Fatalf("envelope JSON version = %v, want %d", decoded["version"], EnvelopeVersion)
	}
	if v, ok := decoded["apiVersion"].(float64); !ok || int(v) != EnvelopeAPIVersion {
		t.Fatalf("envelope JSON apiVersion = %v, want %d", decoded["apiVersion"], EnvelopeAPIVersion)
	}
	if got, ok := decoded["grammarHash"].(string); !ok || got != dsl.GrammarManifestSHA256 {
		t.Fatalf("envelope JSON grammarHash = %v, want canonical grammar manifest hash %q", decoded["grammarHash"], dsl.GrammarManifestSHA256)
	}
	if _, ok := decoded["ok"]; !ok {
		t.Fatal("envelope JSON missing ok field")
	}
}
