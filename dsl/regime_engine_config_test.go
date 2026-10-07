package dsl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

func regimeTestConfig(t *testing.T) Config      { t.Helper(); return regimeParseOK(t, regimeTestSource) }
func regimeTestSpec(c Config) map[string]any    { return c["regimeEngine"].(map[string]any) }
func regimeTestProfile(c Config) map[string]any { return regimeTestSpec(c)["profile"].(map[string]any) }

func TestRegimeEngineReservedConfigurations(t *testing.T) {
	for _, key := range []string{"regimeEngine", "REGIMEENGINE", "RegimeEngine", "regime-engine", "regime_engine", "regime engine"} {
		for _, value := range []any{nil, "bad", true, []any{}, map[string]any{}, int64(1)} {
			if !IsRegimeEngineReserved(Config{"setupType": "flagContinuation", key: value}) {
				t.Fatalf("reserved field missed: %q=%v", key, value)
			}
		}
	}
	for _, key := range []string{"setupType", "SETUPTYPE", "SetupType", "setup_type"} {
		for _, value := range []any{"regimeEngine", "REGIMEENGINE", "regime engine", "regime-engine", FamilyRegimeEngine, FamilyID("REGIMEENGINE")} {
			if !IsRegimeEngineReserved(Config{key: value}) {
				t.Fatalf("reserved discriminator missed: %q=%v", key, value)
			}
		}
	}
	for _, c := range []Config{nil, {}, {"setupType": "flagContinuation"}, {"setupType": FamilyFrozenLevelBreakout}, {"description": "regimeEngine"}, {"setupType": nil}, {"setupType": 12}, {"regime": "trend"}} {
		if IsRegimeEngineReserved(c) {
			t.Fatalf("old configuration spuriously reserved: %v", c)
		}
	}
}

func TestRegimeEngineRejectsClosedConfigMutations(t *testing.T) {
	tests := map[string]func(Config){
		"unknown root":     func(c Config) { c["riskUSD"] = 200 },
		"old family":       func(c Config) { c["setupType"] = "flagContinuation" },
		"family casing":    func(c Config) { c["setupType"] = "REGIMEENGINE" },
		"null spec":        func(c Config) { c["regimeEngine"] = nil },
		"scalar spec":      func(c Config) { c["regimeEngine"] = "source-like-v1" },
		"unknown spec":     func(c Config) { regimeTestSpec(c)["grid"] = []any{9, 21} },
		"unsupported mode": func(c Config) { regimeTestSpec(c)["mode"] = "early-bracket-v1" },
		"version6":         func(c Config) { c["dslVersion"] = 6 },
		"empty name":       func(c Config) { c["name"] = "" },
		"invalid name":     func(c Config) { c["name"] = string([]byte{0xff}) },
		"NaN":              func(c Config) { regimeTestProfile(c)["targetATR"] = math.NaN() },
		"infinity":         func(c Config) { regimeTestProfile(c)["targetATR"] = math.Inf(1) },
		"rounded length":   func(c Config) { regimeTestProfile(c)["completeObservedBars"] = json.Number("40.000000000000000001") },
		"rounded fraction": func(c Config) { regimeTestProfile(c)["notionalFraction"] = json.Number("0.100000000000000001") },
		"root casing":      func(c Config) { c["DSLVersion"] = c["dslVersion"]; delete(c, "dslVersion") },
		"profile casing": func(c Config) {
			regimeTestSpec(c)["Profile"] = regimeTestSpec(c)["profile"]
			delete(regimeTestSpec(c), "profile")
		},
	}
	for _, key := range []string{"dslVersion", "name", "description", "setupType", "regimeEngine"} {
		key := key
		tests["missing "+key] = func(c Config) { delete(c, key) }
		tests["null "+key] = func(c Config) { c[key] = nil }
	}
	for _, key := range []string{"contractVersion", "mode", "profile"} {
		key := key
		tests["missing spec "+key] = func(c Config) { delete(regimeTestSpec(c), key) }
		tests["null spec "+key] = func(c Config) { regimeTestSpec(c)[key] = nil }
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c := regimeTestConfig(t)
			mutate(c)
			if _, err := DecodeRegimeEngine(c); err == nil {
				t.Fatalf("invalid config accepted: %v", c)
			}
		})
	}
	// Every fixed field, including every nested field, is closed and mandatory.
	var walk func(map[string]any, []string)
	walk = func(m map[string]any, path []string) {
		for key, value := range m {
			fieldPath := append(append([]string{}, path...), key)
			for _, kind := range []string{"null", "missing", "changed", "unknown"} {
				t.Run(strings.Join(fieldPath, ".")+"/"+kind, func(t *testing.T) {
					c := regimeTestConfig(t)
					at := regimeTestProfile(c)
					for _, p := range path {
						at = at[p].(map[string]any)
					}
					switch kind {
					case "null":
						at[key] = nil
					case "missing":
						delete(at, key)
					case "changed":
						at[key] = "unsupported"
					case "unknown":
						at["unknown"] = true
					}
					if _, err := DecodeRegimeEngine(c); err == nil {
						t.Fatal("fixed-field mutation accepted")
					}
				})
			}
			if nested, ok := value.(map[string]any); ok {
				walk(nested, fieldPath)
			}
		}
	}
	walk(RegimeEngineProfile(), nil)
}

func TestRegimeEngineRawJSONRejectsAmbiguity(t *testing.T) {
	c := regimeTestConfig(t)
	raw, _ := json.Marshal(c)
	for name, bad := range map[string][]byte{
		"duplicate root":    bytes.Replace(raw, []byte(`"dslVersion":7`), []byte(`"dslVersion":7,"dslVersion":7`), 1),
		"escaped duplicate": bytes.Replace(raw, []byte(`"mode":`), []byte(`"m\u006fde":"audit-baseline-v1","mode":`), 1),
		"duplicate nested":  bytes.Replace(raw, []byte(`"fast":9`), []byte(`"fast":9,"fast":9`), 1),
		"null":              []byte("null"), "array": []byte("[]"), "two documents": append(append([]byte{}, raw...), []byte(" {}")...),
		"fractional fixed integer": bytes.Replace(raw, []byte(`"length":14`), []byte(`"length":14.000000000000000001`), 1),
		"fractional fixed decimal": bytes.Replace(raw, []byte(`"notionalFraction":0.1`), []byte(`"notionalFraction":0.100000000000000001`), 1),
		"unknown":                  bytes.Replace(raw, []byte(`"mode":`), []byte(`"unknown":null,"mode":`), 1),
		"surrogate":                bytes.Replace(raw, []byte(`Strict audited interpretation`), []byte(`\ud800`), 1),
		"invalid UTF8":             append(append([]byte{}, raw...), 0xff),
		"oversized":                bytes.Repeat([]byte(" "), (1<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRegimeEngineConfigJSON(bad); err == nil {
				t.Fatal("raw JSON ambiguity accepted")
			}
		})
	}
	for _, replacement := range []string{"14.0", "1.4e1", "140e-1"} {
		b := bytes.Replace(raw, []byte(`"length":14`), []byte(`"length":`+replacement), 1)
		if got, err := DecodeRegimeEngineConfigJSON(b); err != nil || !reflect.DeepEqual(got, c) {
			t.Fatalf("equivalent exact decimal rejected: %s %v", replacement, err)
		}
	}
}

func TestRegimeEngineProjectionIsolation(t *testing.T) {
	a := regimeTestConfig(t)
	b := regimeTestConfig(t)
	regimeTestProfile(a)["hma"].(map[string]any)["fast"] = int64(10)
	if _, err := DecodeRegimeEngine(b); err != nil {
		t.Fatal("config mutation escaped into another parse")
	}
	profile := RegimeEngineProfile()
	profile["hma"].(map[string]any)["fast"] = int64(11)
	if _, err := DecodeRegimeEngine(regimeTestConfig(t)); err != nil {
		t.Fatal("profile mutation escaped")
	}
	b["setupType"] = FamilyRegimeEngine
	if _, err := DecodeRegimeEngine(b); err != nil {
		t.Fatal("typed canonical family rejected", err)
	}
}

func TestRegimeEngineContractSchema(t *testing.T) {
	schema, path := loadContractSchema(t, "regime-engine-config-v1.schema.json")
	for _, mode := range []string{RegimeEngineSourceLikeV1, RegimeEngineAuditBaselineV1} {
		raw, _ := json.Marshal(regimeParseOK(t, strings.ReplaceAll(regimeTestSource, RegimeEngineSourceLikeV1, mode)))
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		if err := validateContract(value, schema, path, schema, "config"); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"dslVersion", "name", "description", "setupType", "regimeEngine"} {
			bad := cloneContractObject(value)
			bad[key] = nil
			assertInvalidContract(t, bad, schema, path, "null "+key)
			bad = cloneContractObject(value)
			delete(bad, key)
			assertInvalidContract(t, bad, schema, path, "missing "+key)
		}
		for _, key := range []string{"contractVersion", "mode", "profile"} {
			bad := cloneContractObject(value)
			bad["regimeEngine"].(map[string]any)[key] = "unknown"
			assertInvalidContract(t, bad, schema, path, "bad "+key)
		}
		var walk func(map[string]any, []string)
		walk = func(m map[string]any, parts []string) {
			for key, v := range m {
				p := append(append([]string{}, parts...), key)
				bad := cloneContractObject(value)
				at := bad["regimeEngine"].(map[string]any)["profile"].(map[string]any)
				for _, k := range parts {
					at = at[k].(map[string]any)
				}
				at[key] = nil
				assertInvalidContract(t, bad, schema, path, "profile mutation "+strings.Join(p, "."))
				if child, ok := v.(map[string]any); ok {
					walk(child, p)
				}
			}
		}
		walk(value["regimeEngine"].(map[string]any)["profile"].(map[string]any), nil)
		bad := cloneContractObject(value)
		bad["other"] = 1.
		assertInvalidContract(t, bad, schema, path, "unknown root")
		bad = cloneContractObject(value)
		bad["regimeEngine"].(map[string]any)["other"] = 1.
		assertInvalidContract(t, bad, schema, path, "unknown spec")
	}
}

func TestRegimeEngineGrammarManifestBinding(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(testsupport.MustRepoRoot(), "spec", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != GrammarManifestSHA256 {
		t.Fatal("generated grammar provenance differs")
	}
	var manifest struct {
		Families       []struct{ ID, Classification string }
		Aliases        []struct{ FamilyID, Classification, Source, Spelling string }
		DirectiveHeads []struct{ ID, Classification, Spelling string }
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range manifest.Families {
		if f.ID == "regimeEngine" {
			found = true
			if f.Classification != "go-only" {
				t.Fatal("family is not Go-only")
			}
		}
	}
	if !found || parserFamilyHandlerBindings["regimeEngine"] != "parseRegimeEngineSource" {
		t.Fatal("family binding absent")
	}
	aliases := 0
	for _, a := range manifest.Aliases {
		if a.FamilyID == "regimeEngine" {
			aliases++
			if a.Classification != "go-only" || !strings.HasPrefix(a.Source, "go-") || a.Spelling != "regime engine" {
				t.Fatal("unexpected family alias")
			}
		}
	}
	if aliases != 2 || generatedCanonicalSetupFamilies["regime engine"] != "regimeEngine" {
		t.Fatal("family aliases missing")
	}
	for _, head := range []string{"mode", "profile", "timeframe"} {
		id := "directive.regime-" + head
		found = false
		for _, d := range manifest.DirectiveHeads {
			if d.ID == id {
				found = true
				if d.Classification != "go-only" || d.Spelling != "regime "+head {
					t.Fatal("invalid regime directive")
				}
			}
		}
		if !found || parserDirectiveHandlerBindings[id] != "regimeSourceParser.directive" {
			t.Fatalf("directive binding missing: %s", id)
		}
	}
}
