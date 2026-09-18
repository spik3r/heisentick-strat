package engine

import (
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func weekendExtremeFadeSeries() marketdata.Series {
	start := time.Date(2024, 1, 4, 16, 0, 0, 0, time.UTC)
	bars := make([]marketdata.Bar, 0, 34)
	for i := 0; i < 34; i++ {
		price := 100.0
		bars = append(bars, marketdata.Bar{T: float64(start.Add(time.Duration(i) * 4 * time.Hour).UnixMilli()), O: price, H: price + 10, L: price, C: price + 5, V: 1})
	}
	// Saturday 00 through Sunday 20 closes near the top; Monday 00 confirms bearish.
	for i := 8; i < 20; i++ {
		bars[i].C = 109
	}
	bars[20] = marketdata.Bar{T: bars[20].T, O: 109, H: 110, L: 108, C: 108.5, V: 1}
	return marketdata.SeriesFromBars(bars)
}

func TestWeekendExtremeFadeRuntimeUsesCausalMondayWindow(t *testing.T) {
	b := broker{
		series: weekendExtremeFadeSeries(),
		costs:  Costs{FillOn: "close", StartEquity: 10_000},
		params: flagParams{
			SetupType: string(dsl.FamilyWeekendExtremeFade), AllowShort: true,
			MaxHoldBars: 12, RiskUSD: 200,
			WeekendExtremeFade: weekendExtremeFadeParams{
				ATRLength: 20, MaxWeekendRangeATR: 5, CloseExtremePct: 0.3, StopATR: 0.75, TargetATR: 2,
			},
		},
	}
	trades := b.run()
	if len(trades) != 1 || trades[0].Side != "short" || trades[0].EntryIndex != 20 {
		t.Fatalf("trades = %+v, want one causal Monday short", trades)
	}
}
