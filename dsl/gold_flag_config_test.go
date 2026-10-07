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

func goldFlagTestConfig(t *testing.T) Config {
	t.Helper()
	return goldFlagParseOK(t, goldFlagTestSource)
}
func goldFlagTestSpec(c Config) map[string]any { return c["goldFlagReference"].(map[string]any) }
func goldFlagTestProfile(c Config) map[string]any {
	return goldFlagTestSpec(c)["profile"].(map[string]any)
}

func TestGoldFlagReferenceReservedConfigurations(t *testing.T) {
	for _, key := range []string{"goldFlagReference", "GOLDFLAGREFERENCE", "GoldFlagReference", "gold-flag-reference", "gold_flag_reference", "gold flag reference"} {
		for _, value := range []any{nil, "bad", true, []any{}, map[string]any{}, int64(1)} {
			if !IsGoldFlagReferenceReserved(Config{"setupType": "flagContinuation", key: value}) {
				t.Fatalf("reserved field missed: %q=%v", key, value)
			}
		}
	}
	for _, key := range []string{"setupType", "SETUPTYPE", "SetupType", "setup_type"} {
		for _, value := range []any{"goldFlagReference", "GOLDFLAGREFERENCE", "gold flag reference", "gold-flag-reference", FamilyGoldFlagReference, FamilyID("GOLDFLAGREFERENCE")} {
			if !IsGoldFlagReferenceReserved(Config{key: value}) {
				t.Fatalf("reserved discriminator missed: %q=%v", key, value)
			}
		}
	}
	for _, c := range []Config{nil, {}, {"setupType": "flagContinuation"}, {"setupType": FamilyFrozenLevelBreakout}, {"description": "goldFlagReference"}, {"setupType": nil}, {"setupType": 12}, {"master": "trend"}} {
		if IsGoldFlagReferenceReserved(c) {
			t.Fatalf("old configuration spuriously reserved: %v", c)
		}
	}
}

func TestGoldFlagReferenceRejectsClosedConfigMutations(t *testing.T) {
	tests := map[string]func(Config){
		"unknown root":       func(c Config) { c["riskUSD"] = 200 },
		"old family":         func(c Config) { c["setupType"] = "flagContinuation" },
		"family casing":      func(c Config) { c["setupType"] = "GOLDFLAGREFERENCE" },
		"null spec":          func(c Config) { c["goldFlagReference"] = nil },
		"scalar spec":        func(c Config) { c["goldFlagReference"] = "PR388_CAUSAL_STRESS_V1" },
		"unknown spec":       func(c Config) { goldFlagTestSpec(c)["grid"] = []any{9, 21} },
		"unsupported policy": func(c Config) { goldFlagTestSpec(c)["policy"] = "early-bracket-v1" },
		"version6":           func(c Config) { c["dslVersion"] = 6 },
		"empty name":         func(c Config) { c["name"] = "" },
		"invalid name":       func(c Config) { c["name"] = string([]byte{0xff}) },
		"NaN":                func(c Config) { goldFlagTestProfile(c)["execution"].(map[string]any)["targetR"] = math.NaN() },
		"infinity":           func(c Config) { goldFlagTestProfile(c)["execution"].(map[string]any)["targetR"] = math.Inf(1) },
		"rounded length": func(c Config) {
			goldFlagTestProfile(c)["pattern"].(map[string]any)["poleBars"] = json.Number("6.000000000000000001")
		},
		"rounded fraction": func(c Config) {
			goldFlagTestProfile(c)["pattern"].(map[string]any)["flagMaximumPoleFraction"] = json.Number("0.600000000000000001")
		},
		"root casing": func(c Config) { c["DSLVersion"] = c["dslVersion"]; delete(c, "dslVersion") },
		"profile casing": func(c Config) {
			goldFlagTestSpec(c)["Profile"] = goldFlagTestSpec(c)["profile"]
			delete(goldFlagTestSpec(c), "profile")
		},
	}
	for _, key := range []string{"dslVersion", "name", "description", "setupType", "goldFlagReference"} {
		key := key
		tests["missing "+key] = func(c Config) { delete(c, key) }
		tests["null "+key] = func(c Config) { c[key] = nil }
	}
	for _, key := range []string{"contractVersion", "policy", "profile"} {
		key := key
		tests["missing spec "+key] = func(c Config) { delete(goldFlagTestSpec(c), key) }
		tests["null spec "+key] = func(c Config) { goldFlagTestSpec(c)[key] = nil }
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c := goldFlagTestConfig(t)
			mutate(c)
			if _, err := DecodeGoldFlagReference(c); err == nil {
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
					c := goldFlagTestConfig(t)
					at := goldFlagTestProfile(c)
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
					if _, err := DecodeGoldFlagReference(c); err == nil {
						t.Fatal("fixed-field mutation accepted")
					}
				})
			}
			if nested, ok := value.(map[string]any); ok {
				walk(nested, fieldPath)
			}
		}
	}
	walk(GoldFlagReferenceProfile(), nil)
}

func TestGoldFlagReferenceRawJSONRejectsAmbiguity(t *testing.T) {
	c := goldFlagTestConfig(t)
	raw, _ := json.Marshal(c)
	for name, bad := range map[string][]byte{
		"duplicate root":    bytes.Replace(raw, []byte(`"dslVersion":7`), []byte(`"dslVersion":7,"dslVersion":7`), 1),
		"escaped duplicate": bytes.Replace(raw, []byte(`"policy":`), []byte(`"p\u006flicy":"OTHER_POLICY","policy":`), 1),
		"duplicate nested":  bytes.Replace(raw, []byte(`"poleBars":6`), []byte(`"poleBars":6,"poleBars":6`), 1),
		"null":              []byte("null"), "array": []byte("[]"), "two documents": append(append([]byte{}, raw...), []byte(" {}")...),
		"fractional fixed integer": bytes.Replace(raw, []byte(`"length":14`), []byte(`"length":14.000000000000000001`), 1),
		"fractional fixed decimal": bytes.Replace(raw, []byte(`"flagMaximumPoleFraction":0.6`), []byte(`"flagMaximumPoleFraction":0.600000000000000001`), 1),
		"unknown":                  bytes.Replace(raw, []byte(`"policy":`), []byte(`"unknown":null,"policy":`), 1),
		"surrogate":                bytes.Replace(raw, []byte(`Strict audited interpretation`), []byte(`\ud800`), 1),
		"invalid UTF8":             append(append([]byte{}, raw...), 0xff),
		"oversized":                bytes.Repeat([]byte(" "), (1<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeGoldFlagReferenceConfigJSON(bad); err == nil {
				t.Fatal("raw JSON ambiguity accepted")
			}
		})
	}
	for _, replacement := range []string{"14.0", "1.4e1", "140e-1"} {
		b := bytes.Replace(raw, []byte(`"length":14`), []byte(`"length":`+replacement), 1)
		if got, err := DecodeGoldFlagReferenceConfigJSON(b); err != nil || !reflect.DeepEqual(got, c) {
			t.Fatalf("equivalent exact decimal rejected: %s %v", replacement, err)
		}
	}
}

func TestGoldFlagReferenceProjectionIsolation(t *testing.T) {
	a := goldFlagTestConfig(t)
	b := goldFlagTestConfig(t)
	goldFlagTestProfile(a)["pattern"].(map[string]any)["poleBars"] = int64(10)
	if _, err := DecodeGoldFlagReference(b); err != nil {
		t.Fatal("config mutation escaped into another parse")
	}
	profile := GoldFlagReferenceProfile()
	profile["pattern"].(map[string]any)["poleBars"] = int64(11)
	if _, err := DecodeGoldFlagReference(goldFlagTestConfig(t)); err != nil {
		t.Fatal("profile mutation escaped")
	}
	b["setupType"] = FamilyGoldFlagReference
	if _, err := DecodeGoldFlagReference(b); err != nil {
		t.Fatal("typed canonical family rejected", err)
	}
}

func TestGoldFlagReferenceContractSchema(t *testing.T) {
	schema, path := loadContractSchema(t, "gold-flag-reference-config-v1.schema.json")
	for _, policy := range []string{GoldFlagReferencePolicy} {
		raw, _ := json.Marshal(goldFlagParseOK(t, strings.ReplaceAll(goldFlagTestSource, GoldFlagReferencePolicy, policy)))
		var value map[string]any
		_ = json.Unmarshal(raw, &value)
		if err := validateContract(value, schema, path, schema, "config"); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"dslVersion", "name", "description", "setupType", "goldFlagReference"} {
			bad := cloneContractObject(value)
			bad[key] = nil
			assertInvalidContract(t, bad, schema, path, "null "+key)
			bad = cloneContractObject(value)
			delete(bad, key)
			assertInvalidContract(t, bad, schema, path, "missing "+key)
		}
		for _, key := range []string{"contractVersion", "policy", "profile"} {
			bad := cloneContractObject(value)
			bad["goldFlagReference"].(map[string]any)[key] = "unknown"
			assertInvalidContract(t, bad, schema, path, "bad "+key)
		}
		var walk func(map[string]any, []string)
		walk = func(m map[string]any, parts []string) {
			for key, v := range m {
				p := append(append([]string{}, parts...), key)
				bad := cloneContractObject(value)
				at := bad["goldFlagReference"].(map[string]any)["profile"].(map[string]any)
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
		walk(value["goldFlagReference"].(map[string]any)["profile"].(map[string]any), nil)
		bad := cloneContractObject(value)
		bad["other"] = 1.
		assertInvalidContract(t, bad, schema, path, "unknown root")
		bad = cloneContractObject(value)
		bad["goldFlagReference"].(map[string]any)["other"] = 1.
		assertInvalidContract(t, bad, schema, path, "unknown spec")
	}
}

func TestGoldFlagReferenceGrammarManifestBinding(t *testing.T) {
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
		if f.ID == "goldFlagReference" {
			found = true
			if f.Classification != "go-only" {
				t.Fatal("family is not Go-only")
			}
		}
	}
	if !found || parserFamilyHandlerBindings["goldFlagReference"] != "parseGoldFlagReferenceSource" {
		t.Fatal("family binding absent")
	}
	aliases := 0
	for _, a := range manifest.Aliases {
		if a.FamilyID == "goldFlagReference" {
			aliases++
			if a.Classification != "go-only" || !strings.HasPrefix(a.Source, "go-") || a.Spelling != "gold flag reference" {
				t.Fatal("unexpected family alias")
			}
		}
	}
	if aliases != 2 || generatedCanonicalSetupFamilies["gold flag reference"] != "goldFlagReference" {
		t.Fatal("family aliases missing")
	}
	for _, head := range []string{"policy", "timeframe"} {
		id := "directive.goldflag-" + head
		found = false
		for _, d := range manifest.DirectiveHeads {
			if d.ID == id {
				found = true
				if d.Classification != "go-only" || d.Spelling != "goldflag "+head {
					t.Fatal("invalid goldflag directive")
				}
			}
		}
		if !found || parserDirectiveHandlerBindings[id] != "goldFlagSourceParser.directive" {
			t.Fatalf("directive binding missing: %s", id)
		}
	}
}

func TestGoldFlagReferenceCostsAreClosedAndIsolated(t *testing.T) {
	for _, value := range []any{nil, []any{}, []any{0, .06, .15, .25}, []any{0, .06, .15, .25, .5, 1}, []any{0, .06, .15, .25, .51}, []float64{0, .06, .15, .25, .5}, "0,.06,.15,.25,.50"} {
		c := goldFlagTestConfig(t)
		goldFlagTestProfile(c)["costs"].(map[string]any)["values"] = value
		if _, err := DecodeGoldFlagReference(c); err == nil {
			t.Fatalf("mutated cost array accepted: %v", value)
		}
	}
	a, b := GoldFlagReferenceProfile(), GoldFlagReferenceProfile()
	a["costs"].(map[string]any)["values"].([]any)[1] = 100.
	if err := goldFlagMatchFixed(b, GoldFlagReferenceProfile(), "profile"); err != nil {
		t.Fatal("cost array mutation escaped")
	}
}

func TestGoldFlagReferenceExactNumbersAndTypes(t *testing.T) {
	raw, _ := json.Marshal(goldFlagTestConfig(t))
	for name, pair := range map[string][2]string{
		"risk rounding":       {`"minimumRiskATR":0.4`, `"minimumRiskATR":0.400000000000000001`},
		"cost rounding":       {`0.06,0.15`, `0.060000000000000001,0.15`},
		"cost string":         {`0.06,0.15`, `"0.06",0.15`},
		"integer string":      {`"length":14`, `"length":"14"`},
		"integer bool":        {`"length":14`, `"length":true`},
		"negative zero":       {`"nativePhaseMinutes":0`, `"nativePhaseMinutes":-0`},
		"fixed zero spelling": {`"nativePhaseMinutes":0`, `"nativePhaseMinutes":0.0`},
		"enormous exponent":   {`"length":14`, `"length":14e10001`},
		"negative risk":       {`"minimumRiskATR":0.4`, `"minimumRiskATR":-0.4`},
		"NaN":                 {`"minimumRiskATR":0.4`, `"minimumRiskATR":NaN`},
		"overflow":            {`"minimumRiskATR":0.4`, `"minimumRiskATR":1e309`},
		"trailing comma":      {`"length":14`, `"length":14,`},
		"array reorder":       {`[0,0.06,0.15,0.25,0.5]`, `[0,0.15,0.06,0.25,0.5]`},
	} {
		t.Run(name, func(t *testing.T) {
			b := bytes.Replace(raw, []byte(pair[0]), []byte(pair[1]), 1)
			if bytes.Equal(raw, b) {
				t.Fatal("mutation did not apply")
			}
			if _, err := DecodeGoldFlagReferenceConfigJSON(b); err == nil {
				t.Fatal("invalid number/type accepted")
			}
		})
	}
	for _, pair := range [][2]string{{`"minimumRiskATR":0.4`, `"minimumRiskATR":4e-1`}, {`0.06,0.15`, `6e-2,0.15`}, {`"poleBars":6`, `"poleBars":6.000`}} {
		b := bytes.Replace(raw, []byte(pair[0]), []byte(pair[1]), 1)
		if bytes.Equal(raw, b) {
			t.Fatal("mutation did not apply")
		}
		if _, err := DecodeGoldFlagReferenceConfigJSON(b); err != nil {
			t.Fatal("exact alternate positive number rejected", err)
		}
	}
}

func TestGoldFlagReferenceRejectsLegacyOverridesAtEveryLevel(t *testing.T) {
	for _, key := range []string{"masterStructural", "regimeEngine", "frozenLevelBreakout", "riskUSD", "costPerFill", "spread", "commission", "slippage", "quantity", "equity", "tickSize", "grid", "params", "sourceTimeframe"} {
		for _, level := range []string{"root", "spec", "profile"} {
			c := goldFlagTestConfig(t)
			var at map[string]any = c
			if level == "spec" {
				at = goldFlagTestSpec(c)
			}
			if level == "profile" {
				at = goldFlagTestProfile(c)
			}
			at[key] = 0.1
			if _, err := DecodeGoldFlagReference(c); err == nil {
				t.Fatalf("override accepted at %s.%s", level, key)
			}
		}
	}
}

func TestGoldFlagReferenceProfileStatesApprovedPolicy(t *testing.T) {
	p := GoldFlagReferenceProfile()
	checks := map[string]map[string]any{
		"atr":       {"length": int64(14), "firstValidIndex": int64(14), "seed": "mean-TR-indices-1-through-14", "smoothing": "wilder-rma"},
		"pattern":   {"poleBars": int64(6), "flagBars": int64(6), "poleMinimumATR": int64(2), "flagMaximumPoleFraction": .6, "flagMaximumATR": 1.5, "retraceMaximumPoleFraction": .5, "extremaTies": "first-observed-occurrence"},
		"context":   {"deATRFilter": false, "volumeFilter": false, "selection": "latest-nominal-close-less-or-equal-decision-close", "long": "flagHigh-greater-or-equal-contextHigh-minus-signalATR", "short": "flagLow-less-or-equal-contextLow-plus-signalATR"},
		"signal":    {"entryBufferATR": .1, "oppositeStopBufferATR": .1, "minimumRiskATR": .4, "cooldownObservedRows": int64(6), "cooldownIncludesInvalidRisk": true, "nextEligibleSignal": "prior-signal-index-plus-7"},
		"pending":   {"activation": "next-observed-row", "expiryObservedRows": int64(4), "missingDataCancellation": false, "terminal": "retain-pending"},
		"execution": {"referenceUnits": int64(1), "equityFeedback": false, "initialBracket": "attached-at-fill", "stopUpdates": "none", "targetR": int64(2), "targetAnchor": "actual-fill", "openingTarget": "cap-at-target", "ambiguousPolicy": "entry-first-stop-first", "timeExitObservedRows": int64(24), "timeExitPriority": "bracket-before-time-exit", "exitRow": "skip-new-signal", "terminalLiquidation": false},
		"costs":     {"effectsOnFillsBracketsRiskSignalsSizing": false, "selection": "explicit-external-report-request", "values": []any{int64(0), .06, .15, .25, .5}},
	}
	for object, fields := range checks {
		for field, want := range fields {
			if !reflect.DeepEqual(p[object].(map[string]any)[field], want) {
				t.Fatalf("approved policy differs at %s.%s", object, field)
			}
		}
	}
	if p["context"].(map[string]any)["h4"].(map[string]any)["lookback"] != int64(12) || p["context"].(map[string]any)["daily"].(map[string]any)["lookback"] != int64(5) {
		t.Fatal("context lookbacks changed")
	}
}
