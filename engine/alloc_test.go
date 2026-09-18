package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestPreparedFlagBarLoopHasZeroAllocsPerRun(t *testing.T) {
	fixture, cfg := noTradeFixtureAndConfig(t)
	runner := newPreparedRunner(fixture, cfg)
	runner.RunPrepared()

	allocs := testing.AllocsPerRun(1000, func() {
		runner.RunPrepared()
	})
	if allocs != 0 {
		t.Fatalf("prepared flag bar loop allocations = %.2f allocs/op, want 0", allocs)
	}
	t.Logf("prepared flag bar loop allocations = %.2f allocs/op across %d bars", allocs, len(fixture.Bars))
}

func BenchmarkPreparedFlagBarLoopNoTrade(b *testing.B) {
	fixture, cfg := noTradeFixtureAndConfig(b)
	runner := newPreparedRunner(fixture, cfg)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runner.RunPrepared()
	}
}

func noTradeFixtureAndConfig(tb testing.TB) (RunFixture, dsl.Config) {
	tb.Helper()
	bars := make([]marketdata.Bar, 256)
	t := float64(1_650_000_000_000)
	price := 1800.0
	for i := range bars {
		price += 0.15
		bars[i] = marketdata.Bar{
			T: t + float64(i)*3_600_000,
			O: price,
			H: price + 0.8,
			L: price - 0.8,
			C: price + 0.1,
			V: 1,
		}
	}
	fixture := RunFixture{
		Schema:          runFixtureSchema,
		Case:            "alloc-no-trade",
		StrategyID:      "allocNoTrade",
		Symbol:          "XAUUSD",
		Timeframe:       "1h",
		HigherTimeframe: "4h",
		RangeMethod:     "zone",
		Costs: Costs{
			FillOn:      "close",
			StartEquity: 10000,
		},
		Bars: bars,
	}
	cfg := dsl.Config{
		"allowLong":       1,
		"allowShort":      1,
		"breakeven":       map[string]any{"atR": 0.5, "offsetAtr": 0.05},
		"cooldownCandles": 12,
		"flag": map[string]any{
			"flagCloseFrac":     0.45,
			"freshBreakBars":    24,
			"levelToleranceAtr": 0.5,
			"maxEr":             0.75,
			"maxFlagBars":       10,
			"maxFlagDepth":      0.34,
			"minEr":             0.5,
			"minFlagBars":       3,
			"minPoleNetFrac":    0.25,
			"poleAtr":           1.5,
			"poleBars":          12,
		},
		"htf":            map[string]any{"mode": "off", "timeframe": "auto"},
		"maxHoldCandles": 0,
		"partial":        map[string]any{"enabled": 0, "fraction": 0, "moveBreakeven": 0, "triggerR": 1},
		"riskUsd":        200,
		"sessions":       map[string]any{"asia": 0, "london": 0, "mid": 0, "ny": 0},
		"setupType":      string(dsl.FamilyFlagContinuation),
		"stop":           map[string]any{"extremeCandles": 0, "maxAtr": nil, "minAtr": 0.5, "paddingAtr": 0.1},
		"target":         map[string]any{"edge": "range", "fallbackR": 1, "flagR": 0.8, "minR": 0.6},
		"trail":          map[string]any{"atr": 0, "triggerR": 1},
	}
	return fixture, cfg
}
