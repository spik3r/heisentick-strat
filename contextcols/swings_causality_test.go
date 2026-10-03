package contextcols

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func swingTestSeries(highs, lows []float64) marketdata.Series {
	bars := make([]marketdata.Bar, len(highs))
	for i := range highs {
		bars[i] = marketdata.Bar{T: float64(i), O: lows[i], H: highs[i], L: lows[i], C: highs[i], V: 1}
	}
	return marketdata.SeriesFromBars(bars)
}

func TestSwingsAnchorAtPivotButBecomeAvailableAfterRightFlank(t *testing.T) {
	prefix := ComputeSwings(swingTestSeries([]float64{1, 3, 2}, []float64{0, 1, 0}), 1)
	if len(prefix.PivotHighs) != 1 || prefix.PivotHighs[0] != (Pivot{Idx: 1, Price: 3}) {
		t.Fatalf("high pivots = %+v, want price 3 anchored at bar 1", prefix.PivotHighs)
	}
	if !math.IsNaN(prefix.LastHigh[1]) || prefix.LastHigh[2] != 3 {
		t.Fatalf("last high before/at confirmation = %v/%v, want unavailable/3", prefix.LastHigh[1], prefix.LastHigh[2])
	}
	longer := ComputeSwings(swingTestSeries([]float64{1, 3, 2, 5, 4}, []float64{0, 1, 0, 1, 0}), 1)
	for i, want := range prefix.LastHigh {
		got := longer.LastHigh[i]
		if !(math.IsNaN(got) && math.IsNaN(want)) && got != want {
			t.Errorf("append changed prior confirmed swing at bar %d: %v to %v", i, want, got)
		}
	}
}

func TestSwingsRequireUniqueExtremumAcrossEqualTies(t *testing.T) {
	swings := ComputeSwings(swingTestSeries([]float64{4, 6, 6, 4}, []float64{3, 1, 1, 3}), 1)
	if len(swings.PivotHighs) != 0 || len(swings.PivotLows) != 0 {
		t.Fatalf("equal extrema produced pivots: highs %+v lows %+v", swings.PivotHighs, swings.PivotLows)
	}
	for i := range swings.LastHigh {
		if !math.IsNaN(swings.LastHigh[i]) || !math.IsNaN(swings.LastLow[i]) {
			t.Errorf("bar %d equal extrema should remain unavailable: high %v low %v", i, swings.LastHigh[i], swings.LastLow[i])
		}
	}
}
