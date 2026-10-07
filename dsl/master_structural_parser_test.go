package dsl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const masterTestSource = `dsl v7
strategy "Master structural synthetic control" {
 description "Strict audited interpretation"
}
market {
 master timeframe M30 from M5
}
setup {
 type: master structural
 master profile v10-phase0-floor-half-reference-v1
 master mode SOURCE_HISTORICAL_REFERENCE
}
`

func masterParseOK(t *testing.T, source string) Config {
	t.Helper()
	r, err := Parse(source)
	if err != nil || len(r.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, r.Errors)
	}
	if !reflect.DeepEqual(r.Warnings, []string{MasterStructuralDedicatedRunnerRequired}) || len(r.Diagnostics) != 1 || r.Diagnostics[0].Message != MasterStructuralDedicatedRunnerRequired || r.Diagnostics[0].Severity != DiagnosticWarning {
		t.Fatalf("missing native-only warning: %+v", r)
	}
	if _, err := DecodeMasterStructural(r.Config); err != nil {
		t.Fatal(err)
	}
	return r.Config
}

func masterRejectSource(t *testing.T, source string) {
	t.Helper()
	r, err := Parse(source)
	if err != nil || len(r.Errors) != 1 || len(r.Config) != 0 || len(r.Diagnostics) != 1 || r.Diagnostics[0].Severity != DiagnosticError || !strings.HasPrefix(r.Errors[0], "master structural: ") {
		t.Fatalf("source escaped strict scanner: %q\nresult=%+v err=%v", source, r, err)
	}
}

func TestMasterStructuralTwoModesAndClosedProjection(t *testing.T) {
	for _, mode := range []string{MasterStructuralSourceHistoricalReference, MasterStructuralProtectedStableReference} {
		cfg := masterParseOK(t, strings.ReplaceAll(masterTestSource, MasterStructuralSourceHistoricalReference, mode))
		spec, err := DecodeMasterStructural(cfg)
		if err != nil || spec.Mode != mode || len(cfg) != 5 {
			t.Fatalf("wrong projection: %+v %v", cfg, err)
		}
		raw, _ := json.Marshal(cfg)
		roundTrip, err := DecodeMasterStructuralConfigJSON(raw)
		if err != nil || !reflect.DeepEqual(cfg, roundTrip) {
			t.Fatalf("raw round trip: %v", err)
		}
	}
}

func TestMasterStructuralEquivalentSource(t *testing.T) {
	want := masterParseOK(t, masterTestSource)
	for name, source := range map[string]string{
		"inline":           strings.ReplaceAll(masterTestSource, "\n", " "),
		"comments":         "# type: old family\n" + strings.ReplaceAll(masterTestSource, "\n", " # } { type: flag continuation\n"),
		"keywords":         strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(masterTestSource, "master ", "MASTER "), "type:", "TYPE:"), "M30 from M5", "m30 FROM m5"),
		"block order":      "dsl v7\n" + masterTestSource[strings.Index(masterTestSource, "setup {"):] + masterTestSource[strings.Index(masterTestSource, "market {"):strings.Index(masterTestSource, "setup {")] + masterTestSource[len("dsl v7\n"):strings.Index(masterTestSource, "market {")],
		"directives order": strings.Replace(masterTestSource, "type: master structural\n master profile v10-phase0-floor-half-reference-v1\n master mode SOURCE_HISTORICAL_REFERENCE", "master mode SOURCE_HISTORICAL_REFERENCE master profile v10-phase0-floor-half-reference-v1 type: master structural", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if got := masterParseOK(t, source); !reflect.DeepEqual(got, want) {
				t.Fatal("equivalent syntax changed config")
			}
		})
	}
	metadata := strings.Replace(masterTestSource, "Strict audited interpretation", `type: flag continuation # } { master mode anything \u2028`, 1)
	if got := masterParseOK(t, metadata); !strings.Contains(got["description"].(string), "master mode anything") {
		t.Fatal("quoted metadata was executed")
	}
}

func TestMasterStructuralStrictSourceMutations(t *testing.T) {
	mutations := map[string]string{
		"version6":              strings.Replace(masterTestSource, "dsl v7", "dsl v6", 1),
		"no version":            strings.Replace(masterTestSource, "dsl v7", "", 1),
		"version repeated":      "dsl v7 " + masterTestSource,
		"trailing":              masterTestSource + "ignored garbage",
		"extra close":           masterTestSource + "}",
		"unclosed":              strings.TrimSuffix(masterTestSource, "}\n"),
		"old relabel":           strings.Replace(masterTestSource, "type: master structural", "type: master structural type: flag continuation", 1),
		"early old relabel":     strings.Replace(masterTestSource, "type: master structural", "type: flag continuation type: master structural", 1),
		"missing type":          strings.Replace(masterTestSource, "type: master structural", "", 1),
		"missing profile":       strings.Replace(masterTestSource, "master profile v10-phase0-floor-half-reference-v1", "", 1),
		"missing mode":          strings.Replace(masterTestSource, "master mode SOURCE_HISTORICAL_REFERENCE", "", 1),
		"missing timeframe":     strings.Replace(masterTestSource, "master timeframe M30 from M5", "", 1),
		"wrong profile":         strings.Replace(masterTestSource, "v10-phase0-floor-half-reference-v1", "v10-phase1-ceil-half-reference-v1", 1),
		"wrong source":          strings.Replace(masterTestSource, "from M5", "from M1", 1),
		"wrong native":          strings.Replace(masterTestSource, "M30 from", "H1 from", 1),
		"quoted mode":           strings.Replace(masterTestSource, "mode SOURCE_HISTORICAL_REFERENCE", `mode "SOURCE_HISTORICAL_REFERENCE"`, 1),
		"mode case":             strings.Replace(masterTestSource, "SOURCE_HISTORICAL_REFERENCE", "source_historical_reference", 1),
		"unsupported mode":      strings.Replace(masterTestSource, "SOURCE_HISTORICAL_REFERENCE", "early-bracket-v1", 1),
		"unknown field":         strings.Replace(masterTestSource, "master mode SOURCE_HISTORICAL_REFERENCE", "master mode SOURCE_HISTORICAL_REFERENCE master risk 0.2", 1),
		"wrong section":         strings.Replace(masterTestSource, "master timeframe M30 from M5", "master mode SOURCE_HISTORICAL_REFERENCE", 1),
		"duplicate mode":        strings.Replace(masterTestSource, "master mode SOURCE_HISTORICAL_REFERENCE", "master mode SOURCE_HISTORICAL_REFERENCE master mode SOURCE_HISTORICAL_REFERENCE", 1),
		"duplicate description": strings.Replace(masterTestSource, `description "Strict audited interpretation"`, `description "a" description "b"`, 1),
		"duplicate block":       masterTestSource + "setup {}",
		"extra block":           masterTestSource + "risk { risk 200 USD }",
		"embedded directives":   strings.Replace(masterTestSource, "master mode SOURCE_HISTORICAL_REFERENCE", "master mode SOURCE_HISTORICAL_REFERENCE stop 2 ATR", 1),
		"parameter grid":        strings.Replace(masterTestSource, "SOURCE_HISTORICAL_REFERENCE", "[SOURCE_HISTORICAL_REFERENCE, PROTECTED_STABLE_REFERENCE]", 1),
		"invalid Unicode":       strings.Replace(masterTestSource, "Strict audited interpretation", `\ud800`, 1),
		"invalid UTF8":          masterTestSource + string([]byte{0xff}),
		"semicolon":             strings.Replace(masterTestSource, "dsl v7", "dsl v7;", 1),
	}
	for name, source := range mutations {
		t.Run(name, func(t *testing.T) { masterRejectSource(t, source) })
	}
}

func TestMasterStructuralReservesSelectorsAndAuthoredFields(t *testing.T) {
	for _, value := range []string{"master structural", "masterStructural", "MASTERSTRUCTURAL", "master-structural", `"master structural"`, `'master structural'`, "`master structural`", `"\u006daster structural"`, "unknown masterStructural", "/*invalid*/ master structural", "\nmaster structural"} {
		for _, suffix := range []string{"", " type: flag continuation"} {
			masterRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { type: "+value+suffix+" }")
		}
	}
	for _, clause := range []string{"master mode SOURCE_HISTORICAL_REFERENCE", "master profile v10-phase0-floor-half-reference-v1", "master timeframe M30 from M5", "masterStructural null", "MASTERSTRUCTURAL: null"} {
		for _, section := range []string{"setup", "risk", "execution", "market"} {
			masterRejectSource(t, "dsl v7\nstrategy \"x\"\n"+section+" { "+clause+" }\nsetup { type: flag continuation }")
		}
	}
	for _, quote := range []string{`"`, "'", "`"} {
		masterRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { description "+quote+"missing type: master structural type: flag continuation }")
		masterRejectSource(t, "dsl v7\nstrategy "+quote+"missing\nsetup { type: master structural type: flag continuation }")
	}
}

func TestMasterStructuralPreservesLegacyMetadataAndRegimePhrases(t *testing.T) {
	clauses := []string{"regime trend", "regime range", "description master profile v10-phase0-floor-half-reference-v1", "name master mode SOURCE_HISTORICAL_REFERENCE", "symbols REGIME", "symbols(MASTER, BTCUSDT)", "symbols(masterStructural)"}
	for _, q := range []string{`"`, "'", "`"} {
		clauses = append(clauses, "description "+q+"type: master structural # { master mode SOURCE_HISTORICAL_REFERENCE }"+q, "name "+q+"masterStructural"+q, "symbols("+q+"type: master structural"+q+", MASTER)")
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

func TestMasterStructuralRejectsMixedFamilySelectors(t *testing.T) {
	for _, selector := range []string{"type: regime engine", "type: frozen level breakout", "type: flag continuation", "regime mode source-like-v1", "frozen profile anything", "masterStructural: null"} {
		for _, before := range []bool{false, true} {
			anchor := "type: master structural"
			replacement := anchor + " " + selector
			if before {
				replacement = selector + " " + anchor
			}
			masterRejectSource(t, strings.Replace(masterTestSource, anchor, replacement, 1))
		}
	}
}

func TestMasterStructuralSelectorsSurviveLegacyNormalization(t *testing.T) {
	for _, alias := range []string{"master_structural", "MASTER-STRUCTURAL", "MasterStructural", "(master structural)", "[master structural]", "/*junk*/masterStructural", "'masterStructural'"} {
		for _, location := range []string{"type: " + alias + " type: flag continuation", "{} type: " + alias + " type: flag continuation", "{ type: " + alias + " } type: flag continuation"} {
			masterRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { "+location+" }")
		}
	}
	for _, source := range []string{
		"dsl v7\nstrategy \"x\"\nsetup { type:\nmaster structural }",
		"dsl v7\nstrategy \"x\"\nsetup { description \"bad quote\n type: master structural type: flag continuation }",
		"dsl v7\nstrategy \"x\"\nsetup { unknown {} type: master structural }",
	} {
		masterRejectSource(t, source)
	}
}

func TestMasterStructuralReservedIntentSurvivesOrdinaryTails(t *testing.T) {
	for _, prefix := range []string{"risk 100 USD", "stop 2 ATR", "unknown", "type: flag continuation", "type: unknown", "type: flag continuation stop 2 ATR"} {
		for _, clause := range []string{"master mode SOURCE_HISTORICAL_REFERENCE", "master profile v10-phase0-floor-half-reference-v1", "master timeframe M30 from M5", "masterStructural null", "MASTER_STRUCTURAL: null", "master\nmode SOURCE_HISTORICAL_REFERENCE"} {
			for _, section := range []string{"setup", "market", "risk", "execution"} {
				masterRejectSource(t, "dsl v7\nstrategy \"Adversarial\"\n"+section+" { "+prefix+" "+clause+" }")
			}
		}
	}
	for _, body := range []string{"unknown masterStructural null risk 100 USD", "description text risk 100 USD master mode SOURCE_HISTORICAL_REFERENCE", "symbols(XAUUSD) risk 100 USD master mode SOURCE_HISTORICAL_REFERENCE"} {
		masterRejectSource(t, "dsl v7\nstrategy \"Adversarial\"\nsetup { "+body+" }")
	}
}

func TestMasterStructuralTailRepairPreservesMetadata(t *testing.T) {
	for _, clause := range []string{"description master mode SOURCE_HISTORICAL_REFERENCE", "description risk 100 USD master mode SOURCE_HISTORICAL_REFERENCE", "name masterStructural", "symbols masterStructural", "symbols(masterStructural, TYPE, MASTER)", `description "risk 100 USD master mode SOURCE_HISTORICAL_REFERENCE"`, `description 'stop 2 ATR masterStructural null'`, "description `master profile anything`"} {
		contexts := []string{clause + "\nsetup {type: flag continuation}"}
		// Free metadata has section-specific inline boundaries, while quoted data
		// and balanced lists stay opaque even inside setup.
		if !strings.Contains(clause, "description risk") {
			contexts = append(contexts, "setup { "+clause+" type: flag continuation }")
		}
		for _, body := range contexts {
			source := "dsl v7\nstrategy \"Legacy\"\n" + body
			old := newParser(source)
			old.parse()
			want := old.result()
			got, e := Parse(source)
			if e != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("metadata changed %q: %v %v", source, e, got.Errors)
			}
		}
	}
	for _, body := range []string{"description free risk 100 USD master mode SOURCE_HISTORICAL_REFERENCE", "name x type: masterStructural"} {
		source := "dsl v7\nstrategy \"Legacy\" {" + body + "}\nsetup {type: flag continuation}"
		old := newParser(source)
		old.parse()
		got, e := Parse(source)
		if e != nil || !reflect.DeepEqual(got, old.result()) {
			t.Fatal("strategy metadata acquired directives", got.Errors, e)
		}
	}
}

func TestMasterStructuralMalformedTypePunctuationReservesIntent(t *testing.T) {
	for _, head := range []string{"type\u00a0:", "type=", "type->", "type=>", "TYPE="} {
		for _, family := range []string{"master structural", "masterStructural", "master_structural", "Master-Structural", `"master structural"`} {
			masterRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { "+head+" "+family+" type: flag continuation }")
		}
	}
	for _, head := range []string{"type=masterStructural", "type->master_structural"} {
		masterRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { "+head+" type: flag continuation }")
	}
}

func TestMasterStructuralUnicodeWhitespaceTailsRemainReserved(t *testing.T) {
	for _, space := range []string{"\u00a0", "\u2003", "\v", "\f", "\u2028", "\u2029"} {
		for _, directive := range []string{"mode SOURCE_HISTORICAL_REFERENCE", "profile v10-phase0-floor-half-reference-v1", "timeframe M30 from M5"} {
			masterRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { risk 100 USD master"+space+directive+" }")
			source := "dsl v7\nstrategy \"x\"\nsetup { description \"master" + space + directive + "\" type: flag continuation }"
			old := newParser(source)
			old.parse()
			got, e := Parse(source)
			if e != nil || !reflect.DeepEqual(got, old.result()) {
				t.Fatal("Unicode quoted metadata changed", got.Errors, e)
			}
		}
	}
}

func TestMasterStructuralUnicodeObjectKeysRemainReserved(t *testing.T) {
	for _, space := range []string{"\u00a0", "\u2003", "\u202f", "\v", "\f"} {
		masterRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { risk 100 USD master"+space+"Structural: null }")
	}
}
