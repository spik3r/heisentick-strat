package dsl

import (
	"strings"
	"testing"
)

const validPriceMomentumSource = `dsl v7
strategy "Price Momentum Parser"
setup {
  type: price momentum
  lookback 20 candles
  neutral zone 1.25 percent
}
`

func TestPriceMomentumParserExactGrammar(t *testing.T) {
	result, err := Parse(validPriceMomentumSource)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	if result.Config["setupType"] != string(FamilyPriceMomentum) {
		t.Fatalf("setupType = %v", result.Config["setupType"])
	}
	momentum := copyMap(result.Config["priceMomentum"])
	if momentum["lookbackBars"] != float64(20) || momentum["thresholdPct"] != 1.25 {
		t.Fatalf("priceMomentum = %#v", momentum)
	}
}

func TestPriceMomentumParserRejectsInvalidOrMissingParameters(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{"zero lookback", strings.Replace(validPriceMomentumSource, "lookback 20 candles", "lookback 0 candles", 1), "positive integer"},
		{"fractional lookback", strings.Replace(validPriceMomentumSource, "lookback 20 candles", "lookback 2.5 candles", 1), "positive integer"},
		{"wrong lookback unit", strings.Replace(validPriceMomentumSource, "lookback 20 candles", "lookback 20 bars", 1), "lookback N candles"},
		{"zero threshold", strings.Replace(validPriceMomentumSource, "neutral zone 1.25 percent", "neutral zone 0 percent", 1), "finite positive"},
		{"wrong threshold unit", strings.Replace(validPriceMomentumSource, "neutral zone 1.25 percent", "neutral zone 1.25 pct", 1), "neutral zone X percent"},
		{"missing lookback", strings.Replace(validPriceMomentumSource, "  lookback 20 candles\n", "", 1), "requires \"lookback N candles\""},
		{"missing threshold", strings.Replace(validPriceMomentumSource, "  neutral zone 1.25 percent\n", "", 1), "requires \"neutral zone X percent\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.source)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !containsPriceMomentumError(result.Errors, tt.message) {
				t.Fatalf("errors = %v, want substring %q", result.Errors, tt.message)
			}
		})
	}
}

func containsPriceMomentumError(errors []string, needle string) bool {
	for _, message := range errors {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}
