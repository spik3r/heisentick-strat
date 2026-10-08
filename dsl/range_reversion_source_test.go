package dsl

import (
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
