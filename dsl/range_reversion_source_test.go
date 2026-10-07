package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRangeReversionExampleStrategiesCompile(t *testing.T) {
	for _, name := range []string{"range-reversion-pine-chart20.strat", "range-reversion-4h-source-1.strat", "range-reversion-4h-source-20.strat"} {
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
	for _, family := range []string{"RANGEREVERSION", "range-reversion", "range_reversion"} {
		if !IsRangeReversionReserved(Config{"setupType": family}) {
			t.Errorf("family alias %q escaped reservation", family)
		}
	}
}
