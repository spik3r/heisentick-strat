package dsl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const goldFlagTestSource = `dsl v7
strategy "Gold flag reference synthetic control" {
 description "Strict audited interpretation"
}
market {
 goldflag timeframe M30 from M15
}
setup {
 type: gold flag reference
 goldflag policy PR388_CAUSAL_STRESS_V1
}
`

func goldFlagParseOK(t *testing.T, source string) Config {
	t.Helper()
	r, err := Parse(source)
	if err != nil || len(r.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, r.Errors)
	}
	if !reflect.DeepEqual(r.Warnings, []string{GoldFlagReferenceDedicatedRunnerRequired}) || len(r.Diagnostics) != 1 || r.Diagnostics[0].Message != GoldFlagReferenceDedicatedRunnerRequired || r.Diagnostics[0].Severity != DiagnosticWarning {
		t.Fatalf("missing native-only warning: %+v", r)
	}
	if _, err := DecodeGoldFlagReference(r.Config); err != nil {
		t.Fatal(err)
	}
	return r.Config
}

func goldFlagRejectSource(t *testing.T, source string) {
	t.Helper()
	r, err := Parse(source)
	if err != nil || len(r.Errors) != 1 || len(r.Config) != 0 || len(r.Diagnostics) != 1 || r.Diagnostics[0].Severity != DiagnosticError || !strings.HasPrefix(r.Errors[0], "gold flag reference: ") {
		t.Fatalf("source escaped strict scanner: %q\nresult=%+v err=%v", source, r, err)
	}
}

func TestGoldFlagReferenceClosedProjection(t *testing.T) {
	cfg := goldFlagParseOK(t, goldFlagTestSource)
	spec, err := DecodeGoldFlagReference(cfg)
	if err != nil || spec.Policy != GoldFlagReferencePolicy || len(cfg) != 5 {
		t.Fatalf("wrong projection: %+v %v", cfg, err)
	}
	raw, _ := json.Marshal(cfg)
	roundTrip, err := DecodeGoldFlagReferenceConfigJSON(raw)
	if err != nil || !reflect.DeepEqual(cfg, roundTrip) {
		t.Fatalf("raw round trip: %v", err)
	}
}

func TestGoldFlagReferenceEquivalentSource(t *testing.T) {
	want := goldFlagParseOK(t, goldFlagTestSource)
	for name, source := range map[string]string{
		"inline":           strings.ReplaceAll(goldFlagTestSource, "\n", " "),
		"comments":         "# type: old family\n" + strings.ReplaceAll(goldFlagTestSource, "\n", " # } { type: flag continuation\n"),
		"keywords":         strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(goldFlagTestSource, "goldflag ", "GOLDFLAG "), "type:", "TYPE:"), "M30 from M15", "m30 FROM m15"),
		"block order":      "dsl v7\n" + goldFlagTestSource[strings.Index(goldFlagTestSource, "setup {"):] + goldFlagTestSource[strings.Index(goldFlagTestSource, "market {"):strings.Index(goldFlagTestSource, "setup {")] + goldFlagTestSource[len("dsl v7\n"):strings.Index(goldFlagTestSource, "market {")],
		"directives order": strings.Replace(goldFlagTestSource, "type: gold flag reference\n goldflag policy PR388_CAUSAL_STRESS_V1", "goldflag policy PR388_CAUSAL_STRESS_V1 type: gold flag reference", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if got := goldFlagParseOK(t, source); !reflect.DeepEqual(got, want) {
				t.Fatal("equivalent syntax changed config")
			}
		})
	}
	metadata := strings.Replace(goldFlagTestSource, "Strict audited interpretation", `type: flag continuation # } { goldflag policy anything \u2028`, 1)
	if got := goldFlagParseOK(t, metadata); !strings.Contains(got["description"].(string), "goldflag policy anything") {
		t.Fatal("quoted metadata was executed")
	}
}

func TestGoldFlagReferenceStrictSourceMutations(t *testing.T) {
	mutations := map[string]string{
		"version6":              strings.Replace(goldFlagTestSource, "dsl v7", "dsl v6", 1),
		"no version":            strings.Replace(goldFlagTestSource, "dsl v7", "", 1),
		"version repeated":      "dsl v7 " + goldFlagTestSource,
		"trailing":              goldFlagTestSource + "ignored garbage",
		"extra close":           goldFlagTestSource + "}",
		"unclosed":              strings.TrimSuffix(goldFlagTestSource, "}\n"),
		"old relabel":           strings.Replace(goldFlagTestSource, "type: gold flag reference", "type: gold flag reference type: flag continuation", 1),
		"early old relabel":     strings.Replace(goldFlagTestSource, "type: gold flag reference", "type: flag continuation type: gold flag reference", 1),
		"missing type":          strings.Replace(goldFlagTestSource, "type: gold flag reference", "", 1),
		"missing policy":        strings.Replace(goldFlagTestSource, "goldflag policy PR388_CAUSAL_STRESS_V1", "", 1),
		"missing timeframe":     strings.Replace(goldFlagTestSource, "goldflag timeframe M30 from M15", "", 1),
		"wrong source":          strings.Replace(goldFlagTestSource, "from M15", "from M1", 1),
		"wrong native":          strings.Replace(goldFlagTestSource, "M30 from", "H1 from", 1),
		"quoted policy":         strings.Replace(goldFlagTestSource, "policy PR388_CAUSAL_STRESS_V1", `policy "PR388_CAUSAL_STRESS_V1"`, 1),
		"policy case":           strings.Replace(goldFlagTestSource, "PR388_CAUSAL_STRESS_V1", "pr388_causal_stress_v1", 1),
		"unsupported policy":    strings.Replace(goldFlagTestSource, "PR388_CAUSAL_STRESS_V1", "early-bracket-v1", 1),
		"unknown field":         strings.Replace(goldFlagTestSource, "goldflag policy PR388_CAUSAL_STRESS_V1", "goldflag policy PR388_CAUSAL_STRESS_V1 goldflag risk 0.2", 1),
		"wrong section":         strings.Replace(goldFlagTestSource, "goldflag timeframe M30 from M15", "goldflag policy PR388_CAUSAL_STRESS_V1", 1),
		"duplicate policy":      strings.Replace(goldFlagTestSource, "goldflag policy PR388_CAUSAL_STRESS_V1", "goldflag policy PR388_CAUSAL_STRESS_V1 goldflag policy PR388_CAUSAL_STRESS_V1", 1),
		"duplicate description": strings.Replace(goldFlagTestSource, `description "Strict audited interpretation"`, `description "a" description "b"`, 1),
		"duplicate block":       goldFlagTestSource + "setup {}",
		"extra block":           goldFlagTestSource + "risk { risk 200 USD }",
		"embedded directives":   strings.Replace(goldFlagTestSource, "goldflag policy PR388_CAUSAL_STRESS_V1", "goldflag policy PR388_CAUSAL_STRESS_V1 stop 2 ATR", 1),
		"parameter grid":        strings.Replace(goldFlagTestSource, "PR388_CAUSAL_STRESS_V1", "[PR388_CAUSAL_STRESS_V1, OTHER_POLICY]", 1),
		"invalid Unicode":       strings.Replace(goldFlagTestSource, "Strict audited interpretation", `\ud800`, 1),
		"invalid UTF8":          goldFlagTestSource + string([]byte{0xff}),
		"semicolon":             strings.Replace(goldFlagTestSource, "dsl v7", "dsl v7;", 1),
	}
	for name, source := range mutations {
		t.Run(name, func(t *testing.T) { goldFlagRejectSource(t, source) })
	}
}

func TestGoldFlagReferenceReservesSelectorsAndAuthoredFields(t *testing.T) {
	for _, value := range []string{"gold flag reference", "goldFlagReference", "GOLDFLAGREFERENCE", "gold-flag-reference", `"gold flag reference"`, `'gold flag reference'`, "`gold flag reference`", `"\u0067old flag reference"`, "unknown goldFlagReference", "/*invalid*/ gold flag reference", "\ngold flag reference"} {
		for _, suffix := range []string{"", " type: flag continuation"} {
			goldFlagRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { type: "+value+suffix+" }")
		}
	}
	for _, clause := range []string{"goldflag policy PR388_CAUSAL_STRESS_V1", "goldflag timeframe M30 from M15", "goldFlagReference null", "GOLDFLAGREFERENCE: null"} {
		for _, section := range []string{"setup", "risk", "execution", "market"} {
			goldFlagRejectSource(t, "dsl v7\nstrategy \"x\"\n"+section+" { "+clause+" }\nsetup { type: flag continuation }")
		}
	}
	for _, quote := range []string{`"`, "'", "`"} {
		goldFlagRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { description "+quote+"missing type: gold flag reference type: flag continuation }")
		goldFlagRejectSource(t, "dsl v7\nstrategy "+quote+"missing\nsetup { type: gold flag reference type: flag continuation }")
	}
}

func TestGoldFlagReferencePreservesLegacyMetadataAndRegimePhrases(t *testing.T) {
	clauses := []string{"regime trend", "regime range", "description goldflag profile unsupported-profile", "name goldflag policy PR388_CAUSAL_STRESS_V1", "symbols REGIME", "symbols(GOLD, BTCUSDT)", "symbols(goldFlagReference)"}
	for _, q := range []string{`"`, "'", "`"} {
		clauses = append(clauses, "description "+q+"type: gold flag reference # { goldflag policy PR388_CAUSAL_STRESS_V1 }"+q, "name "+q+"goldFlagReference"+q, "symbols("+q+"type: gold flag reference"+q+", GOLD)")
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

func TestGoldFlagReferenceRejectsMixedFamilySelectors(t *testing.T) {
	for _, selector := range []string{"type: regime engine", "type: frozen level breakout", "type: flag continuation", "regime mode source-like-v1", "frozen profile anything", "goldFlagReference: null"} {
		for _, before := range []bool{false, true} {
			anchor := "type: gold flag reference"
			replacement := anchor + " " + selector
			if before {
				replacement = selector + " " + anchor
			}
			goldFlagRejectSource(t, strings.Replace(goldFlagTestSource, anchor, replacement, 1))
		}
	}
}

func TestGoldFlagReferenceSelectorsSurviveLegacyNormalization(t *testing.T) {
	for _, alias := range []string{"gold_flag_reference", "GOLD-FLAG-REFERENCE", "GoldFlagReference", "(gold flag reference)", "[gold flag reference]", "/*junk*/goldFlagReference", "'goldFlagReference'"} {
		for _, location := range []string{"type: " + alias + " type: flag continuation", "{} type: " + alias + " type: flag continuation", "{ type: " + alias + " } type: flag continuation"} {
			goldFlagRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { "+location+" }")
		}
	}
	for _, source := range []string{
		"dsl v7\nstrategy \"x\"\nsetup { type:\ngold flag reference }",
		"dsl v7\nstrategy \"x\"\nsetup { description \"bad quote\n type: gold flag reference type: flag continuation }",
		"dsl v7\nstrategy \"x\"\nsetup { unknown {} type: gold flag reference }",
	} {
		goldFlagRejectSource(t, source)
	}
}

func TestGoldFlagReferenceReservedIntentSurvivesOrdinaryTails(t *testing.T) {
	for _, prefix := range []string{"risk 100 USD", "stop 2 ATR", "unknown", "type: flag continuation", "type: unknown", "type: flag continuation stop 2 ATR"} {
		for _, clause := range []string{"goldflag policy PR388_CAUSAL_STRESS_V1", "goldflag timeframe M30 from M15", "goldFlagReference null", "GOLD_FLAG_REFERENCE: null", "goldflag\npolicy PR388_CAUSAL_STRESS_V1"} {
			for _, section := range []string{"setup", "market", "risk", "execution"} {
				goldFlagRejectSource(t, "dsl v7\nstrategy \"Adversarial\"\n"+section+" { "+prefix+" "+clause+" }")
			}
		}
	}
	for _, body := range []string{"unknown goldFlagReference null risk 100 USD", "description text risk 100 USD goldflag policy PR388_CAUSAL_STRESS_V1", "symbols(XAUUSD) risk 100 USD goldflag policy PR388_CAUSAL_STRESS_V1"} {
		goldFlagRejectSource(t, "dsl v7\nstrategy \"Adversarial\"\nsetup { "+body+" }")
	}
}

func TestGoldFlagReferenceTailRepairPreservesMetadata(t *testing.T) {
	for _, clause := range []string{"description goldflag policy PR388_CAUSAL_STRESS_V1", "description risk 100 USD goldflag policy PR388_CAUSAL_STRESS_V1", "name goldFlagReference", "symbols goldFlagReference", "symbols(goldFlagReference, TYPE, GOLD)", `description "risk 100 USD goldflag policy PR388_CAUSAL_STRESS_V1"`, `description 'stop 2 ATR goldFlagReference null'`, "description `goldflag profile anything`"} {
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
	for _, body := range []string{"description free risk 100 USD goldflag policy PR388_CAUSAL_STRESS_V1", "name x type: goldFlagReference"} {
		source := "dsl v7\nstrategy \"Legacy\" {" + body + "}\nsetup {type: flag continuation}"
		old := newParser(source)
		old.parse()
		got, e := Parse(source)
		if e != nil || !reflect.DeepEqual(got, old.result()) {
			t.Fatal("strategy metadata acquired directives", got.Errors, e)
		}
	}
}

func TestGoldFlagReferenceMalformedTypePunctuationReservesIntent(t *testing.T) {
	for _, head := range []string{"type\u00a0:", "type=", "type->", "type=>", "TYPE="} {
		for _, family := range []string{"gold flag reference", "goldFlagReference", "gold_flag_reference", "Gold-Flag-Reference", `"gold flag reference"`} {
			goldFlagRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { "+head+" "+family+" type: flag continuation }")
		}
	}
	for _, head := range []string{"type=goldFlagReference", "type->gold_flag_reference"} {
		goldFlagRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { "+head+" type: flag continuation }")
	}
}

func TestGoldFlagReferenceUnicodeWhitespaceTailsRemainReserved(t *testing.T) {
	for _, space := range []string{"\u00a0", "\u2003", "\v", "\f", "\u2028", "\u2029"} {
		for _, directive := range []string{"policy PR388_CAUSAL_STRESS_V1", "timeframe M30 from M15"} {
			goldFlagRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { risk 100 USD goldflag"+space+directive+" }")
			source := "dsl v7\nstrategy \"x\"\nsetup { description \"goldflag" + space + directive + "\" type: flag continuation }"
			old := newParser(source)
			old.parse()
			got, e := Parse(source)
			if e != nil || !reflect.DeepEqual(got, old.result()) {
				t.Fatal("Unicode quoted metadata changed", got.Errors, e)
			}
		}
	}
}

func TestGoldFlagReferenceUnicodeObjectKeysRemainReserved(t *testing.T) {
	for _, space := range []string{"\u00a0", "\u2003", "\u202f", "\v", "\f"} {
		goldFlagRejectSource(t, "dsl v7\nstrategy \"x\"\nsetup { risk 100 USD goldflag"+space+"Reference: null }")
	}
}

func TestGoldFlagReferenceOptionalDescriptionAndEmptyName(t *testing.T) {
	cfg := goldFlagParseOK(t, strings.Replace(goldFlagTestSource, `description "Strict audited interpretation"`, "", 1))
	if cfg["description"] != "" {
		t.Fatal("omitted description must project as empty string")
	}
	goldFlagRejectSource(t, strings.Replace(goldFlagTestSource, `"Gold flag reference synthetic control"`, `""`, 1))
	goldFlagRejectSource(t, strings.Replace(goldFlagTestSource, `"Gold flag reference synthetic control"`, `unquoted`, 1))
}

func TestGoldFlagReferenceMalformedNamespaceIsReserved(t *testing.T) {
	for _, field := range []string{
		"goldflag=policy PR388_CAUSAL_STRESS_V1", "goldflag_policy PR388_CAUSAL_STRESS_V1", "[goldflag] policy anything", "/*junk*/goldFlagReference null",
		"goldflag unknown value", "goldflag risk 200", "goldflag costs 0.06", "gold_flag policy anything", "GOLD-FLAG timeframe M30 from M15",
		"gold flag reference: null", "gold\u00a0flag\u2003reference: null", "goldFlagReference null",
	} {
		for _, section := range []string{"setup", "market", "risk", "execution"} {
			goldFlagRejectSource(t, "dsl v7\nstrategy \"Legacy\"\n"+section+" { "+field+" }\nsetup { type: flag continuation }")
		}
	}
	for _, part := range []string{"type: gold flag reference", "goldflag policy PR388_CAUSAL_STRESS_V1", "goldflag timeframe M30 from M15"} {
		goldFlagRejectSource(t, strings.Replace(goldFlagTestSource, part, part+";", 1))
	}
	for _, field := range []string{"stop 2 ATR", "risk 200 USD", "grid (1,2)", "symbols(XAUUSD)", "description unquoted"} {
		goldFlagRejectSource(t, strings.Replace(goldFlagTestSource, "goldflag policy PR388_CAUSAL_STRESS_V1", "goldflag policy PR388_CAUSAL_STRESS_V1 "+field, 1))
	}
	for _, other := range []string{"type: master structural", "master mode SOURCE_HISTORICAL_REFERENCE", "goldflag policy PR388_CAUSAL_STRESS_V1"} {
		goldFlagRejectSource(t, goldFlagTestSource+"setup { "+other+" }")
	}
}

func TestGoldFlagReferenceLeavesGoldenCrossAndQuotedListsAlone(t *testing.T) {
	for _, family := range []string{"sma golden cross", "flag continuation"} {
		for _, metadata := range []string{`description "gold flag reference"`, `symbols("gold flag reference", GOLD)`, "description gold flag reference", `name "goldFlagReference"`} {
			source := "dsl v7\nstrategy \"Legacy\"\n" + metadata + "\nsetup { type: " + family + " }"
			old := newParser(source)
			old.parse()
			got, err := Parse(source)
			if err != nil || !reflect.DeepEqual(got, old.result()) {
				t.Fatalf("legacy source changed: %q: %+v %v", source, got, err)
			}
		}
	}
}
