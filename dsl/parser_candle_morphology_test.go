package dsl

import "testing"

const candleMorphologySource = `dsl v7
strategy "candle morphology"
market conditions { slices(EURUSD 1h) }
setup { type: failed breakout }
filters {
 candle body ratio at least 0.70
 candle lower wick ratio at most 0.30
 candle range percentile 100 at least 0.80
}
`

func TestParseCandleMorphologyFilters(t *testing.T) {
	result, err := Parse(candleMorphologySource)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	filters, ok := result.Config["candleMorphologyFilters"].([]any)
	if !ok || len(filters) != 3 {
		t.Fatalf("expected three candle filters, got %#v", result.Config["candleMorphologyFilters"])
	}
	first, _ := filters[0].(map[string]any)
	if first["feature"] != "bodyRatio" || first["op"] != "min" || first["value"] != 0.70 {
		t.Fatalf("unexpected first filter: %#v", first)
	}
	third, _ := filters[2].(map[string]any)
	if third["feature"] != "rangePercentile" || third["window"] != 100.0 {
		t.Fatalf("unexpected windowed filter: %#v", third)
	}
}

func TestCandleTriggerListStillParses(t *testing.T) {
	result, err := Parse("dsl v7\nstrategy \"triggers\"\nsetup { type: failed breakout }\nfilters { candle in (pin, engulf) }\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	triggers, ok := result.Config["triggerCandles"].([]any)
	if !ok || len(triggers) != 2 {
		t.Fatalf("expected the trigger-candle list to survive, got %#v", result.Config["triggerCandles"])
	}
	if result.Config["candleMorphologyFilters"] != nil {
		t.Fatalf("a trigger list must not create a morphology filter")
	}
}

func TestCandlePathFeatureIsRejected(t *testing.T) {
	result, err := Parse("dsl v7\nstrategy \"path\"\nsetup { type: failed breakout }\nfilters { candle path efficiency at least 0.5 }\n")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("expected a tick-path diagnostic")
	}
}
