package dsl

import (
	"reflect"
	"testing"
)

func TestParseSeasonalityFilters(t *testing.T) {
	result, err := Parse(`dsl v7
market conditions {
  seasonality intraday all in (neutral, bullish, extreme_bullish) min samples 24
  seasonality dow 365d supports entry
  seasonality week of year lookback 30days in (bearish)
  seasonality month history in (insufficient_data)
}
setup { type: failed breakout }
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	want := []any{
		map[string]any{"dimension": "intraday", "lookback": "all", "lookbackDays": nil, "minSamples": 24, "mode": "classification", "classifications": []any{"neutral", "bullish", "extreme_bullish"}},
		map[string]any{"dimension": "dayOfWeek", "lookback": "365d", "lookbackDays": 365, "minSamples": 5, "mode": "supportsEntry"},
		map[string]any{"dimension": "weekOfYear", "lookback": "30d", "lookbackDays": 30, "minSamples": 5, "mode": "classification", "classifications": []any{"bearish"}},
		map[string]any{"dimension": "monthly", "lookback": "all", "lookbackDays": nil, "minSamples": 5, "mode": "classification", "classifications": []any{"insufficient_data"}},
	}
	if got := result.Config["seasonalityFilters"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("seasonalityFilters = %#v, want %#v", got, want)
	}
}

func TestParseSeasonalityRejectsUnsupportedValues(t *testing.T) {
	tests := []struct{ name, line string }{
		{"unknown dimension", "seasonality yearly in (bullish)"},
		{"bad lookback", "seasonality intraday 0d in (bullish)"},
		{"unknown lookback", "seasonality intraday lookback soon in (bullish)"},
		{"unknown token", "seasonality intraday roughly in (bullish)"},
		{"unknown class", "seasonality intraday in (very_bullish)"},
		{"missing class", "seasonality intraday in ()"},
		{"zero samples", "seasonality intraday in (bullish) min samples 0"},
		{"malformed samples", "seasonality intraday in (bullish) min samples many"},
		{"duplicate lookback", "seasonality intraday 30d lookback 60d in (bullish)"},
		{"conflicting modes", "seasonality intraday supports entry in (bullish)"},
		{"missing supports entry object", "seasonality intraday supports bearish"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Parse("dsl v7\nmarket conditions {\n  " + test.line + "\n}\nsetup { type: failed breakout }\n")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(result.Errors) == 0 {
				t.Fatal("invalid seasonality filter was accepted")
			}
		})
	}
}
