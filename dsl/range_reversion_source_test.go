package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRangeReversionExampleStrategiesCompile(t *testing.T) {
	for _, name := range []string{"range-reversion-pine-chart20.strat", "range-reversion-4h-source-1.strat", "range-reversion-4h-source-20.strat", "range-reversion-daily-chop.strat"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "examples", name))
			if err != nil {
				t.Fatal(err)
			}
			result, err := Parse(string(raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			if _, err = DecodeRangeReversion(result.Config); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRangeReversionOptionalDailyCHOPPreservesGateOffConfig(t *testing.T) {
	base, err := Parse(rangeReversionTestSource)
	if err != nil || len(base.Errors) != 0 {
		t.Fatalf("base parse: %v %v", err, base.Errors)
	}
	baseSpec, err := DecodeRangeReversion(base.Config)
	if err != nil || baseSpec.Rules.DailyCHOP != nil {
		t.Fatalf("legacy config changed with gate absent: %+v err=%v", baseSpec.Rules.DailyCHOP, err)
	}
	baseObject := base.Config["rangeReversion"].(map[string]any)
	if _, ok := baseObject["dailyChop"]; ok {
		t.Fatal("disabled daily CHOP appeared in the legacy config projection")
	}

	withGate := strings.Replace(rangeReversionTestSource, "rangereversion atr 14", "rangereversion daily-chop 14 38.2 61.8\n  rangereversion atr 14", 1)
	parsed, err := Parse(withGate)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("daily-gated parse: %v %v", err, parsed.Errors)
	}
	spec, err := DecodeRangeReversion(parsed.Config)
	if err != nil || spec.Rules.DailyCHOP == nil || spec.Rules.DailyCHOP.Period != 14 || spec.Rules.DailyCHOP.Min != 38.2 || spec.Rules.DailyCHOP.Max != 61.8 {
		t.Fatalf("daily gate config=%+v err=%v", spec.Rules.DailyCHOP, err)
	}
}

func TestRangeReversionDailyCHOPIsOptionalButStrict(t *testing.T) {
	cases := []struct {
		name, directive string
		valid           bool
	}{
		{"period too short", "rangereversion daily-chop 1 38.2 61.8", false},
		{"reversed bounds", "rangereversion daily-chop 14 61.8 38.2", false},
		{"equal bounds", "rangereversion daily-chop 14 50 50", false},
		{"nonfinite", "rangereversion daily-chop 14 1e309 61.8", false},
		{"valid", "rangereversion daily-chop 14 38.2 61.8", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.Replace(rangeReversionTestSource, "rangereversion atr 14", tc.directive+"\n  rangereversion atr 14", 1)
			parsed, err := Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(parsed.Errors) == 0; got != tc.valid {
				t.Fatalf("valid=%v errors=%v", got, parsed.Errors)
			}
		})
	}
	duplicate := strings.Replace(rangeReversionTestSource, "rangereversion atr 14", "rangereversion daily-chop 14 38.2 61.8\n  rangereversion daily-chop 14 40 60\n  rangereversion atr 14", 1)
	parsed, err := Parse(duplicate)
	if err != nil || len(parsed.Errors) == 0 {
		t.Fatalf("duplicate daily gate accepted: %v %v", err, parsed.Errors)
	}
}

const rangeReversionTestSource = `dsl v7
strategy "Range reversion frozen control" {
  description "Synthetic parser and runner contract control"
}
market {
  rangereversion timeframe M30
  rangereversion source H4 availability next-native-row
}
setup {
  type: range reversion
  rangereversion policy DELAYED_PINE_OHLC_V1
  rangereversion bounds chart 20
  rangereversion candle-color true
  rangereversion htf-ema true 200
  rangereversion vector-gates true 14 30 48
  rangereversion range-expansion true 1.1
  rangereversion atr 14
  rangereversion stop-atr 0.6
  rangereversion target-r 2
  rangereversion cooldown 2
  rangereversion breakeven true 1 10
  rangereversion tick-size 0.01
}
`

func TestRangeReversionSourceParsesCompleteConfig(t *testing.T) {
	result, err := Parse(rangeReversionTestSource)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != RangeReversionDedicatedRunnerRequired {
		t.Fatalf("unexpected warnings: %v", result.Warnings)
	}
	spec, err := DecodeRangeReversion(result.Config)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Rules.Timeframe != "M30" || spec.Rules.SourceTimeframe != "H4" || spec.Rules.Bounds != "CHART" || spec.Rules.BoundsLookback != 20 || spec.Rules.TickSize != .01 || spec.Rules.BreakEvenOffsetTicks != 10 {
		t.Fatalf("unexpected rules: %+v", spec.Rules)
	}
}

func TestRangeReversionReservedIntentFailsClosed(t *testing.T) {
	cases := []struct{ name, source string }{
		{"unknown directive", strings.Replace(rangeReversionTestSource, "rangereversion cooldown 2", "rangereversion invented 2", 1)},
		{"duplicate override", strings.Replace(rangeReversionTestSource, "rangereversion target-r 2", "rangereversion stop-atr 0.7\n  rangereversion target-r 2", 1)},
		{"malformed boolean", strings.Replace(rangeReversionTestSource, "rangereversion candle-color true", "rangereversion candle-color yes", 1)},
		{"missing required", strings.Replace(rangeReversionTestSource, "rangereversion tick-size 0.01\n", "", 1)},
		{"invalid policy", strings.Replace(rangeReversionTestSource, "DELAYED_PINE_OHLC_V1", "unknown", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !rangeReversionSourceCandidate(tc.source) {
				t.Fatal("reserved source escaped strict selection")
			}
			result, err := Parse(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Errors) == 0 {
				t.Fatal("invalid source was accepted")
			}
		})
	}
}

func TestRangeReversionSelectionIgnoresQuotedMetadata(t *testing.T) {
	if rangeReversionSourceCandidate(`strategy "rangereversion controls are okay"`) {
		t.Fatal("quoted metadata selected strict parser")
	}
}

func TestRangeReversionRejectsPinePolicyWithSourceBounds(t *testing.T) {
	source := strings.Replace(rangeReversionTestSource, "rangereversion bounds chart 20", "rangereversion bounds source 20", 1)
	result, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "requires chart bounds") {
		t.Fatalf("Pine policy/source-bound error=%v", result.Errors)
	}
}

func TestRangeReversionClosedConfigAndReservedAliases(t *testing.T) {
	parsed, err := Parse(rangeReversionTestSource)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("valid fixture failed: %v %v", err, parsed.Errors)
	}
	withExtra := Config{}
	for k, v := range parsed.Config {
		withExtra[k] = v
	}
	withExtra["unexpected"] = true
	if _, err = DecodeRangeReversion(withExtra); err == nil {
		t.Fatal("decoder accepted unknown top-level key")
	}
	obj := map[string]any{}
	for k, v := range parsed.Config["rangeReversion"].(map[string]any) {
		obj[k] = v
	}
	obj["futureControl"] = true
	withExtra = Config{}
	for k, v := range parsed.Config {
		withExtra[k] = v
	}
	withExtra["rangeReversion"] = obj
	if _, err = DecodeRangeReversion(withExtra); err == nil {
		t.Fatal("decoder accepted unknown nested key")
	}
	withDailyText := strings.Replace(rangeReversionTestSource, "rangereversion atr 14", "rangereversion daily-chop 14 38.2 61.8\n  rangereversion atr 14", 1)
	withDaily, err := Parse(withDailyText)
	if err != nil || len(withDaily.Errors) != 0 {
		t.Fatalf("daily config parse: %v %v", err, withDaily.Errors)
	}
	dailyCfg := Config{}
	for k, v := range withDaily.Config {
		dailyCfg[k] = v
	}
	dailyObject := map[string]any{}
	for k, v := range withDaily.Config["rangeReversion"].(map[string]any) {
		dailyObject[k] = v
	}
	delete(dailyObject, "requireCandleColor")
	dailyCfg["rangeReversion"] = dailyObject
	if _, err = DecodeRangeReversion(dailyCfg); err == nil {
		t.Fatal("dailyChop config replaced a required legacy key")
	}
	dailyObject = map[string]any{}
	for k, v := range withDaily.Config["rangeReversion"].(map[string]any) {
		dailyObject[k] = v
	}
	nested := map[string]any{}
	for k, v := range dailyObject["dailyChop"].(map[string]any) {
		nested[k] = v
	}
	nested["unknown"] = true
	dailyObject["dailyChop"] = nested
	dailyCfg["rangeReversion"] = dailyObject
	if _, err = DecodeRangeReversion(dailyCfg); err == nil {
		t.Fatal("decoder accepted unknown dailyChop key")
	}
	for _, family := range []string{"RANGEREVERSION", "range-reversion", "range_reversion"} {
		if !IsRangeReversionReserved(Config{"setupType": family}) {
			t.Errorf("family alias %q escaped reservation", family)
		}
	}
}

// These spellings express reserved intent, not additional accepted aliases.
func TestRangeReversionReservedSelectorCannotBeOverwritten(t *testing.T) {
	selectors := []string{
		`type "range reversion"`, "type 'range reversion'", "type `range reversion`",
		`type "range\u0020reversion"`, `type "range" "reversion"`, `type "range" reversion`, `type range "reversion"`, `type range-reversion`, `type range_reversion`,
		`type range.reversion`, `type range/reversion`, `type range:reversion`,
		`type range | reversion`, `type (range reversion)`, `type [range reversion]`,
		`type = range reversion`, `type=range-reversion`, `type rangeReversion`,
		"type range\u00a0reversion", "type range\u2003reversion", "type range\u202freversion",
		"type range\vreversion", "type range\freversion", "type range\u2028reversion",
		"type\n:\nrange reversion", `type range reversion`,
	}
	for _, selector := range selectors {
		for _, sep := range []string{"\n", " "} {
			for _, before := range []bool{false, true} {
				body := selector + sep + "type: price momentum"
				if !before {
					body = "type: price momentum" + sep + selector
				}
				source := "dsl v7\nstrategy \"Invented reservation control\" {}\nsetup { " + body + " }"
				p, err := Parse(source)
				if err != nil || len(p.Errors) == 0 || len(p.Config) != 0 {
					t.Errorf("selector=%q separator=%q before=%v became runnable: err=%v errors=%v family=%v", selector, sep, before, err, p.Errors, p.Config["setupType"])
				}
			}
		}
	}
}

func TestRangeReversionReservedNamespaceAndDiscardedTails(t *testing.T) {
	for _, clause := range []string{
		`unknown rangereversion policy X`, `stop 2 ATR rangereversion target-r 2`,
		`unknown type "range reversion"`, `(type range-reversion)`,
		`unknown { type range_reversion }`, `rangereversion: target-r 2`,
		`range:reversion target-r 2`, `range.reversion target-r 2`,
	} {
		source := "dsl v7\nstrategy \"Invented\" {}\nsetup { " + clause + " type: price momentum }"
		p, err := Parse(source)
		if err != nil || len(p.Errors) == 0 || len(p.Config) != 0 {
			t.Errorf("tail %q escaped: %v %+v", clause, err, p)
		}
	}
	// A damaged metadata quote must not hide a later authored selector.
	source := "dsl v7\nstrategy \"unfinished\nsetup { type \"range reversion\" type: price momentum }"
	if !rangeReversionSourceCandidate(source) {
		t.Fatal("damaged quote hid reserved selector")
	}
}

func TestRangeReversionReservationKeepsDataOpaque(t *testing.T) {
	for _, source := range []string{
		"dsl v7\nstrategy \"range reversion setup { type range-reversion }\"\nsetup { type: price momentum }",
		"dsl v7\nstrategy \"Plain\" { description \"rangereversion type range reversion\" }\nsetup { type: price momentum }",
		"dsl v7\nname range reversion\ndescription rangereversion\nsetup { type: price momentum }",
		"dsl v7\nstrategy range reversion\nsetup { type: price momentum }",
		"dsl v7\nsymbols(rangereversion, range, reversion)\nsetup { type: price momentum }",
		"dsl v7\nsymbols rangereversion range reversion\nsetup { type: price momentum }",
		"dsl v7\nsetup { type: price momentum symbols(rangereversion, range, reversion) }",
		"dsl v7\nsetup { type: price momentum } # type range reversion\n",
		"dsl v7\nsetup { type: range break fake }",
		"dsl v7\nsetup { range high low type: price momentum }",
	} {
		if rangeReversionSourceCandidate(source) {
			t.Errorf("data/legacy source falsely reserved: %q", source)
		}
	}
}

func TestRangeReversionRefusalConfigIsAnEmptyObject(t *testing.T) {
	for _, source := range []string{
		`dsl v7
strategy "Invented" {}
setup { type: "range reversion" type: price momentum }`,
		strings.Replace(rangeReversionTestSource, "rangereversion atr 14", "rangereversion atr NaN", 1),
		strings.Replace(rangeReversionTestSource, "rangereversion atr 14", "rangereversion atr 14\n rangereversion atr 14", 1),
		"dsl v7\nsetup { type: range reversion }\nstrategy \"unfinished",
	} {
		result, err := Parse(source)
		if err != nil || len(result.Errors) == 0 || result.Config == nil || len(result.Config) != 0 {
			t.Fatalf("invalid refusal projection: err=%v result=%+v", err, result)
		}
		raw, err := json.Marshal(result.Config)
		if err != nil || string(raw) != "{}" {
			t.Fatalf("refusal config must be a JSON object: %s %v", raw, err)
		}
	}
}
