package dsl

import (
	"strings"
	"testing"
)

const validIntraHourRunExhaustionSource = `dsl v7
strategy "Intra Hour Run Exhaustion Parser"
market conditions { slices(XAUUSD 15m) }
setup {
  type: intra hour run exhaustion
  run hour candles 4
  run 3 candles
  run size min 0.8 ATR
  run atr length 14
  exhaustion close location 35 percent
  exhaustion stop pad 0.25 ATR
}
`

func TestIntraHourRunExhaustionParserExactGrammar(t *testing.T) {
	result, err := Parse(validIntraHourRunExhaustionSource)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	if result.Config["setupType"] != string(FamilyIntraHourRunExhaustion) {
		t.Fatalf("setupType = %v", result.Config["setupType"])
	}
	family := copyMap(result.Config["intraHourRunExhaustion"])
	if family["hourCandles"] != float64(4) || family["runCandles"] != float64(3) {
		t.Fatalf("candle counts = %#v", family)
	}
	if family["minRunAtr"] != 0.8 || family["atrLen"] != float64(14) {
		t.Fatalf("run gates = %#v", family)
	}
	if family["exhaustLocationPct"] != 0.35 || family["stopPadAtr"] != 0.25 {
		t.Fatalf("exhaustion gates = %#v", family)
	}
	if copyMap(result.Config["stop"])["paddingAtr"] != 0.25 {
		t.Fatalf("stop = %#v", result.Config["stop"])
	}
}

func TestIntraHourRunExhaustionParserAcceptsDecimalPercentAndPoke(t *testing.T) {
	decimal := strings.Replace(validIntraHourRunExhaustionSource, "35 percent", "0.35 percent", 1)
	result, err := Parse(strings.Replace(decimal, "  run 3 candles", "  run 3 candles\n  exhaustion require poke", 1))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	family := copyMap(result.Config["intraHourRunExhaustion"])
	if family["exhaustLocationPct"] != 0.35 || family["requirePoke"] != 1 {
		t.Fatalf("family = %#v", family)
	}
}

func TestIntraHourRunExhaustionParserRejectsInvalidParameters(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{"zero run candles", strings.Replace(validIntraHourRunExhaustionSource, "run 3 candles", "run 0 candles", 1), "positive integer"},
		{"single hour candle", strings.Replace(validIntraHourRunExhaustionSource, "run hour candles 4", "run hour candles 1", 1), "above 1"},
		{"location at half range", strings.Replace(validIntraHourRunExhaustionSource, "35 percent", "60 percent", 1), "below 50"},
		{"negative stop pad", strings.Replace(validIntraHourRunExhaustionSource, "exhaustion stop pad 0.25 ATR", "exhaustion stop pad -1 ATR", 1), "not negative"},
		{"unknown run phrase", strings.Replace(validIntraHourRunExhaustionSource, "run 3 candles", "run 3 bananas", 1), "intra-hour-run-exhaustion phrases are"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Parse(tc.source)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(result.Errors) == 0 {
				t.Fatalf("expected an error for %s", tc.name)
			}
			if !strings.Contains(strings.Join(result.Errors, " "), tc.message) {
				t.Fatalf("errors = %v, want one containing %q", result.Errors, tc.message)
			}
		})
	}
}

func TestIntraHourRunExhaustionPhrasesRejectedOutsideTheFamily(t *testing.T) {
	result, err := Parse("dsl v7\nsetup { type: failed breakout }\nrun 3 candles\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("expected an unknown-directive error")
	}
}
