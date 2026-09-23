package dsl

import (
	"reflect"
	"testing"
)

func TestParserCompilesSegmentedTradeWindow(t *testing.T) {
	result, err := Parse(`dsl v7
market conditions {
  sessions(mid, london)
  trade window in (mid close, london-first, london_full)
  trade window minutes 30 to 150
}
setup {
  type: price momentum
  lookback 2 candles
  neutral zone 0.5 percent
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v", result.Errors)
	}
	wantSegments := []any{"mid.close", "london.open", "london.all"}
	if got := result.Config["tradeWindowSegments"]; !reflect.DeepEqual(got, wantSegments) {
		t.Fatalf("tradeWindowSegments = %#v, want %#v", got, wantSegments)
	}
	wantRange := map[string]any{"from": float64(30), "to": float64(150)}
	if got := result.Config["tradeWindowMinuteRange"]; !reflect.DeepEqual(got, wantRange) {
		t.Fatalf("tradeWindowMinuteRange = %#v, want %#v", got, wantRange)
	}
}
