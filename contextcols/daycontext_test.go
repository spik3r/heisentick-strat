package contextcols

import (
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestPriorDayTypeUsesCompletedDaysAndStaysFixedWithinCurrentDay(t *testing.T) {
	const day = 86_400_000.0
	series := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 100, H: 104, L: 99, C: 101},
		{T: 8 * 3_600_000, O: 101, H: 103, L: 100, C: 102},
		{T: 16 * 3_600_000, O: 102, H: 104, L: 100, C: 101},
		{T: day, O: 101, H: 106, L: 97, C: 102},
		{T: day + 8*3_600_000, O: 102, H: 107, L: 96, C: 103},
		{T: day + 16*3_600_000, O: 103, H: 105, L: 98, C: 101},
		{T: 2 * day, O: 101, H: 103, L: 100, C: 102},
		{T: 2*day + 8*3_600_000, O: 102, H: 112, L: 94, C: 111},
		{T: 2*day + 16*3_600_000, O: 111, H: 113, L: 92, C: 93},
	})
	cols := Build(series, Options{Selective: true, NeedPriorDay: true})
	for i := 0; i < 6; i++ {
		if cols.PriorDayType[i] != 0 {
			t.Fatalf("prior-day type[%d] = %d, want unset before two completed days", i, cols.PriorDayType[i])
		}
	}
	for i := 6; i < series.Len(); i++ {
		if cols.PriorDayType[i] != priorDayTypeOutside {
			t.Fatalf("prior-day type[%d] = %d, want outside (%d)", i, cols.PriorDayType[i], priorDayTypeOutside)
		}
	}
}

func TestTrendPersistenceDelaysRegimeEntry(t *testing.T) {
	bars := make([]marketdata.Bar, 12)
	for i := range bars {
		close := 100 + float64(i)
		bars[i] = marketdata.Bar{
			T: float64(i * 3_600_000), O: close, H: close + 1, L: close - 1, C: close, V: 1,
		}
	}
	series := marketdata.SeriesFromBars(bars)
	cols := Build(series, Options{
		Selective: true, NeedRegimeTrend: true, ERLen: 3,
		TrendPersistenceBars: 2, Range: RangeOptions{Disabled: true},
	})
	want := []int8{regimeChoppy, regimeChoppy, regimeChoppy, regimeChoppy, regimeTrending, regimeTrending, regimeTrending, regimeTrending, regimeTrending, regimeTrending, regimeTrending, regimeTrending}
	for i, expected := range want {
		if cols.Regime[i] != expected {
			t.Fatalf("regime[%d] = %d, want %d", i, cols.Regime[i], expected)
		}
	}
}

func TestTrueRangeEfficiencyRejectsWickDrivenCloseTrend(t *testing.T) {
	bars := make([]marketdata.Bar, 8)
	for i := range bars {
		close := 100 + float64(i)
		bars[i] = marketdata.Bar{
			T: float64(i * 3_600_000), O: close, H: close + 10, L: close - 10, C: close, V: 1,
		}
	}
	series := marketdata.SeriesFromBars(bars)
	cols := Build(series, Options{Selective: true, NeedRegimeTrend: true, ERLen: 3, Range: RangeOptions{Disabled: true}, TrendERMode: "trueRange"})
	if cols.ER[3] >= 0.1 || cols.Regime[3] != regimeChoppy {
		t.Fatalf("true-range ER/regime[3] = %.3f/%d, want <0.1/choppy", cols.ER[3], cols.Regime[3])
	}
}
