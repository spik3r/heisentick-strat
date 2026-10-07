package dsl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const regimeTestSource = `dsl v7
strategy "Regime engine synthetic control" {
 description "Strict audited interpretation"
}
market {
 regime timeframe M30 from M5
}
setup {
 type: regime engine
 regime profile v9-floor-half-v1
 regime mode source-like-v1
}
`

func regimeParseOK(t *testing.T, source string) Config {
	t.Helper()
	r, err := Parse(source)
	if err != nil || len(r.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, r.Errors)
	}
	if !reflect.DeepEqual(r.Warnings, []string{RegimeEngineDedicatedRunnerRequired}) || len(r.Diagnostics) != 1 || r.Diagnostics[0].Message != RegimeEngineDedicatedRunnerRequired || r.Diagnostics[0].Severity != DiagnosticWarning {
		t.Fatalf("missing native-only warning: %+v", r)
	}
	if _, err := DecodeRegimeEngine(r.Config); err != nil {
		t.Fatal(err)
	}
	return r.Config
}

func regimeRejectSource(t *testing.T, source string) {
	t.Helper()
	r, err := Parse(source)
	if err != nil || len(r.Errors) != 1 || len(r.Config) != 0 || len(r.Diagnostics) != 1 || r.Diagnostics[0].Severity != DiagnosticError || !strings.HasPrefix(r.Errors[0], "regime engine: ") {
		t.Fatalf("source escaped strict scanner: %q\nresult=%+v err=%v", source, r, err)
	}
}

func TestRegimeEngineTwoModesAndClosedProjection(t *testing.T) {
	for _, mode := range []string{RegimeEngineSourceLikeV1, RegimeEngineAuditBaselineV1} {
		cfg := regimeParseOK(t, strings.ReplaceAll(regimeTestSource, RegimeEngineSourceLikeV1, mode))
		spec, err := DecodeRegimeEngine(cfg)
		if err != nil || spec.Mode != mode || len(cfg) != 5 {
			t.Fatalf("wrong projection: %+v %v", cfg, err)
		}
		raw, _ := json.Marshal(cfg)
		roundTrip, err := DecodeRegimeEngineConfigJSON(raw)
		if err != nil || !reflect.DeepEqual(cfg, roundTrip) {
			t.Fatalf("raw round trip: %v", err)
		}
	}
}

func TestRegimeEngineEquivalentSource(t *testing.T) {
	want := regimeParseOK(t, regimeTestSource)
	for name, source := range map[string]string{
		"inline":           strings.ReplaceAll(regimeTestSource, "\n", " "),
		"comments":         "# type: old family\n" + strings.ReplaceAll(regimeTestSource, "\n", " # } { type: flag continuation\n"),
		"keywords":         strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(regimeTestSource, "regime ", "REGIME "), "type:", "TYPE:"), "M30 from M5", "m30 FROM m5"),
		"block order":      "dsl v7\n" + regimeTestSource[strings.Index(regimeTestSource, "setup {"):] + regimeTestSource[strings.Index(regimeTestSource, "market {"):strings.Index(regimeTestSource, "setup {")] + regimeTestSource[len("dsl v7\n"):strings.Index(regimeTestSource, "market {")],
		"directives order": strings.Replace(regimeTestSource, "type: regime engine\n regime profile v9-floor-half-v1\n regime mode source-like-v1", "regime mode source-like-v1 regime profile v9-floor-half-v1 type: regime engine", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if got := regimeParseOK(t, source); !reflect.DeepEqual(got, want) {
				t.Fatal("equivalent syntax changed config")
			}
		})
	}
	metadata := strings.Replace(regimeTestSource, "Strict audited interpretation", `type: flag continuation # } { regime mode anything \u2028`, 1)
	if got := regimeParseOK(t, metadata); !strings.Contains(got["description"].(string), "regime mode anything") {
		t.Fatal("quoted metadata was executed")
	}
}

func TestRegimeEngineStrictSourceMutations(t *testing.T) {
	mutations := map[string]string{
		"version6":              strings.Replace(regimeTestSource, "dsl v7", "dsl v6", 1),
		"no version":            strings.Replace(regimeTestSource, "dsl v7", "", 1),
		"version repeated":      "dsl v7 " + regimeTestSource,
		"trailing":              regimeTestSource + "ignored garbage",
		"extra close":           regimeTestSource + "}",
		"unclosed":              strings.TrimSuffix(regimeTestSource, "}\n"),
		"old relabel":           strings.Replace(regimeTestSource, "type: regime engine", "type: regime engine type: flag continuation", 1),
		"early old relabel":     strings.Replace(regimeTestSource, "type: regime engine", "type: flag continuation type: regime engine", 1),
		"missing type":          strings.Replace(regimeTestSource, "type: regime engine", "", 1),
		"missing profile":       strings.Replace(regimeTestSource, "regime profile v9-floor-half-v1", "", 1),
		"missing mode":          strings.Replace(regimeTestSource, "regime mode source-like-v1", "", 1),
		"missing timeframe":     strings.Replace(regimeTestSource, "regime timeframe M30 from M5", "", 1),
		"wrong profile":         strings.Replace(regimeTestSource, "v9-floor-half-v1", "v9-ceil-half-v1", 1),
		"wrong source":          strings.Replace(regimeTestSource, "from M5", "from M1", 1),
		"wrong native":          strings.Replace(regimeTestSource, "M30 from", "H1 from", 1),
		"quoted mode":           strings.Replace(regimeTestSource, "mode source-like-v1", `mode "source-like-v1"`, 1),
		"mode case":             strings.Replace(regimeTestSource, "source-like-v1", "SOURCE-LIKE-V1", 1),
		"unsupported mode":      strings.Replace(regimeTestSource, "source-like-v1", "early-bracket-v1", 1),
		"unknown field":         strings.Replace(regimeTestSource, "regime mode source-like-v1", "regime mode source-like-v1 regime risk 0.2", 1),
		"wrong section":         strings.Replace(regimeTestSource, "regime timeframe M30 from M5", "regime mode source-like-v1", 1),
		"duplicate mode":        strings.Replace(regimeTestSource, "regime mode source-like-v1", "regime mode source-like-v1 regime mode source-like-v1", 1),
		"duplicate description": strings.Replace(regimeTestSource, `description "Strict audited interpretation"`, `description "a" description "b"`, 1),
		"duplicate block":       regimeTestSource + "setup {}",
		"extra block":           regimeTestSource + "risk { risk 200 USD }",
		"embedded directives":   strings.Replace(regimeTestSource, "regime mode source-like-v1", "regime mode source-like-v1 stop 2 ATR", 1),
		"parameter grid":        strings.Replace(regimeTestSource, "source-like-v1", "[source-like-v1, audit-baseline-v1]", 1),
		"invalid Unicode":       strings.Replace(regimeTestSource, "Strict audited interpretation", `\ud800`, 1),
		"invalid UTF8":          regimeTestSource + string([]byte{0xff}),
		"semicolon":             strings.Replace(regimeTestSource, "dsl v7", "dsl v7;", 1),
	}
	for name, source := range mutations {
		t.Run(name, func(t *testing.T) { regimeRejectSource(t, source) })
	}
}

func TestRegimeEngineReservesSelectorsAndAuthoredFields(t *testing.T) {
	for _, value := range []string{"regime engine", "regimeEngine", "REGIMEENGINE", "regime-engine", `"regime engine"`, `'regime engine'`, "`regime engine`", `"\u0072egime engine"`, "unknown regimeEngine", "/*invalid*/ regime engine", "\nregime engine"} {
		for _, suffix := range []string{"", " type: flag continuation"} {
			regimeRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { type: "+value+suffix+" }")
		}
	}
	for _, clause := range []string{"regime mode source-like-v1", "regime profile v9-floor-half-v1", "regime timeframe M30 from M5", "regimeEngine null", "REGIMEENGINE: null"} {
		for _, section := range []string{"setup", "risk", "execution", "market"} {
			regimeRejectSource(t, "dsl v7\nstrategy \"x\"\n"+section+" { "+clause+" }\nsetup { type: flag continuation }")
		}
	}
	for _, quote := range []string{`"`, "'", "`"} {
		regimeRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { description "+quote+"missing type: regime engine type: flag continuation }")
		regimeRejectSource(t, "dsl v7\nstrategy "+quote+"missing\nsetup { type: regime engine type: flag continuation }")
	}
}

func TestRegimeEnginePreservesLegacyMetadataAndRegimePhrases(t *testing.T) {
	clauses := []string{"regime trend", "regime range", "description regime profile v9-floor-half-v1", "name regime mode source-like-v1", "symbols REGIME", "symbols(REGIME, BTCUSDT)", "symbols(regimeEngine)"}
	for _, q := range []string{`"`, "'", "`"} {
		clauses = append(clauses, "description "+q+"type: regime engine # { regime mode source-like-v1 }"+q, "name "+q+"regimeEngine"+q, "symbols("+q+"type: regime engine"+q+", REGIME)")
	}
	for _, clause := range clauses {
		t.Run(clause, func(t *testing.T) {
			source := "dsl v7\nstrategy \"Legacy\"\n" + clause + "\nsetup { type: flag continuation }"
			old := newParser(source)
			old.parse()
			want := old.result()
			got, err := Parse(source)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("legacy clause changed: %v %v", err, got.Errors)
			}
		})
	}
}
