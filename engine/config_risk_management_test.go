package engine

import (
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestNumberFromAnyFloat32(t *testing.T) {
	fallback := 91.25
	negativeZero := float32(math.Copysign(0, -1))
	tests := []struct {
		name      string
		value     float32
		want      float64
		wantSign  bool
		checkSign bool
	}{
		{name: "finite exact conversion", value: 12.34, want: float64(float32(12.34))},
		{name: "NaN uses fallback", value: float32(math.NaN()), want: fallback},
		{name: "positive infinity", value: float32(math.Inf(1)), want: math.Inf(1)},
		{name: "negative infinity", value: float32(math.Inf(-1)), want: math.Inf(-1)},
		{name: "positive zero", value: 0, want: 0, checkSign: true},
		{name: "negative zero", value: negativeZero, want: math.Copysign(0, -1), wantSign: true, checkSign: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := numberFromAny(tt.value, fallback)
			if got != tt.want {
				t.Fatalf("numberFromAny(%v) = %v, want %v", tt.value, got, tt.want)
			}
			if tt.checkSign && math.Signbit(got) != tt.wantSign {
				t.Fatalf("numberFromAny(%v) signbit = %v, want %v", tt.value, math.Signbit(got), tt.wantSign)
			}
		})
	}
}

func TestFloat32FlowsThroughNestedAndRoundedNumberPaths(t *testing.T) {
	nested := map[string]any{
		"number":    float32(12.34),
		"roundUp":   float32(3.5),
		"roundDown": float32(-2.5),
	}

	if got, want := numberValue(nested, "number", 0), float64(float32(12.34)); got != want {
		t.Fatalf("numberValue() = %.17g, want exact float32 conversion %.17g", got, want)
	}
	if got := intFromAny(float32(3.5), 0); got != 4 {
		t.Fatalf("intFromAny() = %d, want 4", got)
	}
	if got := intValue(nested, "roundDown", 0); got != -3 {
		t.Fatalf("intValue() = %d, want -3", got)
	}
}

func TestRunAcceptsEquivalentFloat32RiskUSD(t *testing.T) {
	fixture, parsedConfig := loadEntryAttemptCase(t, "family-break-retest")
	series := marketdata.SeriesFromBars(fixture.Bars)
	htfSeries := marketdata.SeriesFromBars(fixture.HTFBars)

	run := func(t *testing.T, riskUSD any) RunResult {
		t.Helper()
		cfg := make(map[string]any, len(parsedConfig)+1)
		for key, value := range parsedConfig {
			cfg[key] = value
		}
		cfg["riskUsd"] = riskUSD
		result, err := Run(RunRequest{
			Config:          cfg,
			Series:          series,
			HTFSeries:       htfSeries,
			StrategyID:      fixture.StrategyID,
			Symbol:          fixture.Symbol,
			Timeframe:       fixture.Timeframe,
			HigherTimeframe: fixture.HigherTimeframe,
			RangeMethod:     fixture.RangeMethod,
			Costs:           fixture.Costs,
		})
		if err != nil {
			t.Fatalf("Run(%T): %v", riskUSD, err)
		}
		return result
	}

	float64Result := run(t, float64(128))
	float32Result := run(t, float32(128))
	if float64Result.TradeCount == 0 {
		t.Fatal("reviewed fixture produced no trades")
	}
	if !reflect.DeepEqual(float32Result, float64Result) {
		t.Fatalf("float32 risk result differs from float64 result\n got: %#v\nwant: %#v", float32Result, float64Result)
	}
}
