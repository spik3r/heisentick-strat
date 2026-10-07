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

func masterTestConfig(t *testing.T) Config      { t.Helper(); return masterParseOK(t, masterTestSource) }
func masterTestSpec(c Config) map[string]any    { return c["masterStructural"].(map[string]any) }
func masterTestProfile(c Config) map[string]any { return masterTestSpec(c)["profile"].(map[string]any) }

func TestMasterStructuralReservedConfigurations(t *testing.T) {
	for _, key := range []string{"masterStructural", "MASTERSTRUCTURAL", "MasterStructural", "master-structural", "master_structural", "master structural"} {
		for _, value := range []any{nil, "bad", true, []any{}, map[string]any{}, int64(1)} {
			if !IsMasterStructuralReserved(Config{"setupType": "flagContinuation", key: value}) {
				t.Fatalf("reserved field missed: %q=%v", key, value)
			}
		}
	}
	for _, key := range []string{"setupType", "SETUPTYPE", "SetupType", "setup_type"} {
		for _, value := range []any{"masterStructural", "MASTERSTRUCTURAL", "master structural", "master-structural", FamilyMasterStructural, FamilyID("MASTERSTRUCTURAL")} {
			if !IsMasterStructuralReserved(Config{key: value}) {
				t.Fatalf("reserved discriminator missed: %q=%v", key, value)
			}
		}
	}
	for _, c := range []Config{nil, {}, {"setupType": "flagContinuation"}, {"setupType": FamilyFrozenLevelBreakout}, {"description": "masterStructural"}, {"setupType": nil}, {"setupType": 12}, {"master": "trend"}} {
		if IsMasterStructuralReserved(c) {
			t.Fatalf("old configuration spuriously reserved: %v", c)
		}
	}
}

func TestMasterStructuralRejectsClosedConfigMutations(t *testing.T) {
	tests := map[string]func(Config){
		"unknown root":     func(c Config) { c["riskUSD"] = 200 },
		"old family":       func(c Config) { c["setupType"] = "flagContinuation" },
		"family casing":    func(c Config) { c["setupType"] = "MASTERSTRUCTURAL" },
		"null spec":        func(c Config) { c["masterStructural"] = nil },
		"scalar spec":      func(c Config) { c["masterStructural"] = "SOURCE_HISTORICAL_REFERENCE" },
		"unknown spec":     func(c Config) { masterTestSpec(c)["grid"] = []any{9, 21} },
		"unsupported mode": func(c Config) { masterTestSpec(c)["mode"] = "early-bracket-v1" },
		"version6":         func(c Config) { c["dslVersion"] = 6 },
		"empty name":       func(c Config) { c["name"] = "" },
		"invalid name":     func(c Config) { c["name"] = string([]byte{0xff}) },
		"NaN":              func(c Config) { masterTestProfile(c)["targetATR"] = math.NaN() },
		"infinity":         func(c Config) { masterTestProfile(c)["targetATR"] = math.Inf(1) },
		"rounded length":   func(c Config) { masterTestProfile(c)["completeObservedBars"] = json.Number("40.000000000000000001") },
		"rounded fraction": func(c Config) { masterTestProfile(c)["notionalFraction"] = json.Number("0.100000000000000001") },
		"root casing":      func(c Config) { c["DSLVersion"] = c["dslVersion"]; delete(c, "dslVersion") },
		"profile casing": func(c Config) {
			masterTestSpec(c)["Profile"] = masterTestSpec(c)["profile"]
			delete(masterTestSpec(c), "profile")
		},
	}
	for _, key := range []string{"dslVersion", "name", "description", "setupType", "masterStructural"} {
		key := key
		tests["missing "+key] = func(c Config) { delete(c, key) }
		tests["null "+key] = func(c Config) { c[key] = nil }
	}
	for _, key := range []string{"contractVersion", "mode", "profile"} {
		key := key
		tests["missing spec "+key] = func(c Config) { delete(masterTestSpec(c), key) }
		tests["null spec "+key] = func(c Config) { masterTestSpec(c)[key] = nil }
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c := masterTestConfig(t)
			mutate(c)
			if _, err := DecodeMasterStructural(c); err == nil {
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
					c := masterTestConfig(t)
					at := masterTestProfile(c)
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
					if _, err := DecodeMasterStructural(c); err == nil {
						t.Fatal("fixed-field mutation accepted")
					}
				})
			}
			if nested, ok := value.(map[string]any); ok {
				walk(nested, fieldPath)
			}
		}
	}
	walk(MasterStructuralProfile(MasterStructuralSourceHistoricalReference), nil)
}

func TestMasterStructuralRawJSONRejectsAmbiguity(t *testing.T) {
	c := masterTestConfig(t)
	raw, _ := json.Marshal(c)
	for name, bad := range map[string][]byte{
		"duplicate root":    bytes.Replace(raw, []byte(`"dslVersion":7`), []byte(`"dslVersion":7,"dslVersion":7`), 1),
		"escaped duplicate": bytes.Replace(raw, []byte(`"mode":`), []byte(`"m\u006fde":"PROTECTED_STABLE_REFERENCE","mode":`), 1),
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
			if _, err := DecodeMasterStructuralConfigJSON(bad); err == nil {
				t.Fatal("raw JSON ambiguity accepted")
			}
		})
	}
	for _, replacement := range []string{"14.0", "1.4e1", "140e-1"} {
		b := bytes.Replace(raw, []byte(`"length":14`), []byte(`"length":`+replacement), 1)
		if got, err := DecodeMasterStructuralConfigJSON(b); err != nil || !reflect.DeepEqual(got, c) {
			t.Fatalf("equivalent exact decimal rejected: %s %v", replacement, err)
		}
	}
}

func TestMasterStructuralProjectionIsolation(t *testing.T) {
	a := masterTestConfig(t)
	b := masterTestConfig(t)
	masterTestProfile(a)["hma"].(map[string]any)["fast"] = int64(10)
	if _, err := DecodeMasterStructural(b); err != nil {
		t.Fatal("config mutation escaped into another parse")
	}
	profile := MasterStructuralProfile(MasterStructuralSourceHistoricalReference)
	profile["hma"].(map[string]any)["fast"] = int64(11)
	if _, err := DecodeMasterStructural(masterTestConfig(t)); err != nil {
		t.Fatal("profile mutation escaped")
	}
	b["setupType"] = FamilyMasterStructural
	if _, err := DecodeMasterStructural(b); err != nil {
		t.Fatal("typed canonical family rejected", err)
	}
}

func TestMasterStructuralContractSchema(t *testing.T) {
	schema, path := loadContractSchema(t, "master-structural-config-v1.schema.json")
	for _, mode := range []string{MasterStructuralSourceHistoricalReference, MasterStructuralProtectedStableReference} {
		raw, _ := json.Marshal(masterParseOK(t, strings.ReplaceAll(masterTestSource, MasterStructuralSourceHistoricalReference, mode)))
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		if err := validateContract(value, schema, path, schema, "config"); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"dslVersion", "name", "description", "setupType", "masterStructural"} {
			bad := cloneContractObject(value)
			bad[key] = nil
			assertInvalidContract(t, bad, schema, path, "null "+key)
			bad = cloneContractObject(value)
			delete(bad, key)
			assertInvalidContract(t, bad, schema, path, "missing "+key)
		}
		for _, key := range []string{"contractVersion", "mode", "profile"} {
			bad := cloneContractObject(value)
			bad["masterStructural"].(map[string]any)[key] = "unknown"
			assertInvalidContract(t, bad, schema, path, "bad "+key)
		}
		var walk func(map[string]any, []string)
		walk = func(m map[string]any, parts []string) {
			for key, v := range m {
				p := append(append([]string{}, parts...), key)
				bad := cloneContractObject(value)
				at := bad["masterStructural"].(map[string]any)["profile"].(map[string]any)
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
		walk(value["masterStructural"].(map[string]any)["profile"].(map[string]any), nil)
		bad := cloneContractObject(value)
		bad["other"] = 1.
		assertInvalidContract(t, bad, schema, path, "unknown root")
		bad = cloneContractObject(value)
		bad["masterStructural"].(map[string]any)["other"] = 1.
		assertInvalidContract(t, bad, schema, path, "unknown spec")
	}
}

func TestMasterStructuralGrammarManifestBinding(t *testing.T) {
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
		if f.ID == "masterStructural" {
			found = true
			if f.Classification != "go-only" {
				t.Fatal("family is not Go-only")
			}
		}
	}
	if !found || parserFamilyHandlerBindings["masterStructural"] != "parseMasterStructuralSource" {
		t.Fatal("family binding absent")
	}
	aliases := 0
	for _, a := range manifest.Aliases {
		if a.FamilyID == "masterStructural" {
			aliases++
			if a.Classification != "go-only" || !strings.HasPrefix(a.Source, "go-") || a.Spelling != "master structural" {
				t.Fatal("unexpected family alias")
			}
		}
	}
	if aliases != 2 || generatedCanonicalSetupFamilies["master structural"] != "masterStructural" {
		t.Fatal("family aliases missing")
	}
	for _, head := range []string{"mode", "profile", "timeframe"} {
		id := "directive.master-" + head
		found = false
		for _, d := range manifest.DirectiveHeads {
			if d.ID == id {
				found = true
				if d.Classification != "go-only" || d.Spelling != "master "+head {
					t.Fatal("invalid master directive")
				}
			}
		}
		if !found || parserDirectiveHandlerBindings[id] != "masterSourceParser.directive" {
			t.Fatalf("directive binding missing: %s", id)
		}
	}
}

func TestMasterStructuralModeBundlesCannotBeMixed(t *testing.T) {
	for _, mode := range []string{MasterStructuralSourceHistoricalReference, MasterStructuralProtectedStableReference} {
		other := MasterStructuralSourceHistoricalReference
		if mode == other {
			other = MasterStructuralProtectedStableReference
		}
		base := strings.ReplaceAll(masterTestSource, MasterStructuralSourceHistoricalReference, mode)
		for _, part := range []string{"h4", "lifecycle", "profile", "mode"} {
			t.Run(mode+"/"+part, func(t *testing.T) {
				cfg := masterParseOK(t, base)
				switch part {
				case "mode":
					masterTestSpec(cfg)["mode"] = other
				case "profile":
					masterTestSpec(cfg)["profile"] = MasterStructuralProfile(other)
				default:
					masterTestProfile(cfg)[part] = MasterStructuralProfile(other)[part]
				}
				if _, err := DecodeMasterStructural(cfg); err == nil {
					t.Fatal("cross-mode policy accepted")
				}
				raw, _ := json.Marshal(cfg)
				if _, err := DecodeMasterStructuralConfigJSON(raw); err == nil {
					t.Fatal("cross-mode raw policy accepted")
				}
				schema, path := loadContractSchema(t, "master-structural-config-v1.schema.json")
				var value map[string]any
				_ = json.Unmarshal(raw, &value)
				assertInvalidContract(t, value, schema, path, "cross-mode "+part)
			})
		}
	}
	if got := MasterStructuralProfile("unapproved"); got != nil {
		t.Fatal("unsupported mode acquired a profile")
	}
}

func TestMasterStructuralProfileStatesFrozenContract(t *testing.T) {
	for _, mode := range []string{MasterStructuralSourceHistoricalReference, MasterStructuralProtectedStableReference} {
		profile := MasterStructuralProfile(mode)
		h4 := profile["h4"].(map[string]any)
		lifecycle := profile["lifecycle"].(map[string]any)
		quantity := profile["quantity"].(map[string]any)
		precision := profile["pricePrecision"].(map[string]any)
		if profile["id"] != "v10-phase0-floor-half-reference-v1" || profile["targetATR"] != int64(4) || profile["initialEquityUSD"] != int64(10000) || profile["notionalFraction"] != 0.1 || profile["feePerUnitSide"] != 0.5 {
			t.Fatal("fixed economics/profile changed")
		}
		if quantity["step"] != 0.1 || quantity["epsilonBeforeStepDivision"] != 1e-12 || quantity["formula"] != "floor((equity*0.1/actualEntry+1e-12)/0.1)*0.1" || quantity["nonpositive"] != "terminate-quantity_below_step-before-fee-or-position" {
			t.Fatal("quantity rule changed")
		}
		if precision["arithmetic"] != "continuous-float64-reference" || precision["tickRounding"] != false {
			t.Fatal("price rounding changed")
		}
		if h4["phaseHoursUTC"] != int64(0) || h4["requireComplete"] != false || h4["gate"] != "mapped-direction-alone" || h4["unavailableLine"] != "null" {
			t.Fatal("H4 common policy changed")
		}
		if mode == MasterStructuralSourceHistoricalReference {
			if h4["horizon"] != "M30-decision-close" || lifecycle["initialBracket"] != "after-entry-bar-close" || lifecycle["entryBarProtection"] != false || lifecycle["activation"] != "non-sticky-current-ATR" || lifecycle["stopRatchet"] != false {
				t.Fatal("source mode changed")
			}
		} else {
			if h4["horizon"] != "M30-bar-open" || lifecycle["initialBracket"] != "attached-to-entry-translated-from-fill" || lifecycle["entryBarProtection"] != true || lifecycle["activation"] != "sticky-frozen-signal-ATR" || lifecycle["stopRatchet"] != true {
				t.Fatal("protected mode changed")
			}
		}
	}
}

func TestMasterStructuralProtectedProfileRejectsEveryMutation(t *testing.T) {
	var walk func(map[string]any, []string)
	walk = func(object map[string]any, path []string) {
		for key, value := range object {
			field := append(append([]string{}, path...), key)
			for _, mutation := range []string{"missing", "null", "changed", "unknown"} {
				t.Run(strings.Join(field, ".")+"/"+mutation, func(t *testing.T) {
					cfg := masterParseOK(t, strings.ReplaceAll(masterTestSource, MasterStructuralSourceHistoricalReference, MasterStructuralProtectedStableReference))
					at := masterTestProfile(cfg)
					for _, step := range path {
						at = at[step].(map[string]any)
					}
					switch mutation {
					case "missing":
						delete(at, key)
					case "null":
						at[key] = nil
					case "changed":
						at[key] = "changed"
					case "unknown":
						at["unexpected"] = true
					}
					if _, err := DecodeMasterStructural(cfg); err == nil {
						t.Fatal("protected fixed field mutation accepted")
					}
				})
			}
			if child, ok := value.(map[string]any); ok {
				walk(child, field)
			}
		}
	}
	walk(MasterStructuralProfile(MasterStructuralProtectedStableReference), nil)
}

func TestMasterStructuralRejectsQuantityPrecisionAndLegacyOverrides(t *testing.T) {
	for _, path := range []string{"tickSize", "lotSize", "quantityStep", "riskUSD", "slippage", "regimeEngine", "frozenLevelBreakout", "params", "grid"} {
		for _, level := range []string{"root", "spec", "profile"} {
			t.Run(level+"/"+path, func(t *testing.T) {
				cfg := masterTestConfig(t)
				var at map[string]any = cfg
				if level == "spec" {
					at = masterTestSpec(cfg)
				}
				if level == "profile" {
					at = masterTestProfile(cfg)
				}
				at[path] = 0.1
				if _, err := DecodeMasterStructural(cfg); err == nil {
					t.Fatal("override accepted")
				}
			})
		}
	}
	raw, _ := json.Marshal(masterTestConfig(t))
	for name, pair := range map[string][2]string{
		"rounded epsilon":   {`"epsilonBeforeStepDivision":1e-12`, `"epsilonBeforeStepDivision":1.000000000000000001e-12`},
		"rounded step":      {`"step":0.1`, `"step":0.100000000000000001`},
		"epsilon disabled":  {`"epsilonBeforeStepDivision":1e-12`, `"epsilonBeforeStepDivision":0`},
		"rounded gate":      {`"atrPercentMinimum":0.08`, `"atrPercentMinimum":0.080000000000000001`},
		"duplicate horizon": {`"horizon":`, `"horizon":"M30-bar-open","horizon":`},
		"bool number":       {`"tickRounding":false`, `"tickRounding":0`},
	} {
		t.Run(name, func(t *testing.T) {
			bad := bytes.Replace(raw, []byte(pair[0]), []byte(pair[1]), 1)
			if bytes.Equal(raw, bad) {
				t.Fatal("test mutation did not apply")
			}
			if _, err := DecodeMasterStructuralConfigJSON(bad); err == nil {
				t.Fatal("numeric/duplicate mutation accepted")
			}
		})
	}
}

func TestMasterStructuralFixedZeroLexicalRestriction(t *testing.T) {
	cfg := masterParseOK(t, masterTestSource)
	raw, e := json.Marshal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"0.0", "0e3", "0.00e-2"} {
		changed := strings.Replace(string(raw), `"phaseHoursUTC":0`, `"phaseHoursUTC":`+value, 1)
		if changed == string(raw) {
			t.Fatal("missing zero fixture field")
		}
		if _, e := DecodeMasterStructuralConfigJSON([]byte(changed)); e == nil {
			t.Fatal("undocumented zero spelling admitted", value)
		}
	}
}
