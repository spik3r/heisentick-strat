package contextcols

import (
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestComputeCausalIntradaySeasonalityUsesPriorBarsAndNextHour(t *testing.T) {
	start := time.Date(2025, time.January, 1, 10, 0, 0, 0, time.UTC)
	bars := make([]marketdata.Bar, 0, 12)
	for day := 0; day < 6; day++ {
		atTen := start.AddDate(0, 0, day)
		bars = append(bars,
			marketdata.Bar{T: float64(atTen.UnixMilli()), O: 1, H: 2, L: 0, C: 2},
			marketdata.Bar{T: float64(atTen.Add(time.Hour).UnixMilli()), O: 2, H: 3, L: 1, C: 1},
		)
	}
	cols := Build(marketdata.SeriesFromBars(bars), Options{Selective: true, Seasonality: []SeasonalitySpec{{Dimension: "intraday", Lookback: "all", MinSamples: 5}}})
	entries := cols.Seasonality[SeasonalityKey{Dimension: "intraday", Lookback: "all", MinSamples: 5}]
	if got := entries[0].Classification; got != "insufficient_data" || entries[0].DirectionalCount != 0 {
		t.Fatalf("first bar sees seasonality %s/%d; current bar must be excluded", got, entries[0].DirectionalCount)
	}
	if entries[0].Next != nil {
		t.Fatalf("first bar next-hour stats = %+v, want nil without prior observations", entries[0].Next)
	}
	currentDayTen := 10
	entry := entries[currentDayTen]
	if entry.Classification != "extreme_bullish" || entry.DirectionalCount != 5 {
		t.Fatalf("10:00 prior stats = %+v, want five prior bullish bars", entry)
	}
	if entry.Next == nil || entry.Next.Classification != "extreme_bearish" || entry.Next.DirectionalCount != 5 {
		t.Fatalf("10:00 next-hour projection = %+v, want five prior bearish bars", entry.Next)
	}
}

func TestComputeCausalSeasonalityKeepsDifferentMinimumSampleThresholdsSeparate(t *testing.T) {
	start := time.Date(2025, time.January, 1, 10, 0, 0, 0, time.UTC)
	bars := []marketdata.Bar{
		{T: float64(start.UnixMilli()), O: 1, H: 2, L: 0, C: 2},
		{T: float64(start.AddDate(0, 0, 1).UnixMilli()), O: 1, H: 2, L: 0, C: 2},
		{T: float64(start.AddDate(0, 0, 2).UnixMilli()), O: 1, H: 2, L: 0, C: 2},
	}
	cols := ComputeCausalSeasonality(marketdata.SeriesFromBars(bars), []SeasonalitySpec{
		{Dimension: "intraday", Lookback: "all", MinSamples: 2},
		{Dimension: "intraday", Lookback: "all", MinSamples: 5},
	})
	low := cols[SeasonalityKey{Dimension: "intraday", Lookback: "all", MinSamples: 2}][2]
	high := cols[SeasonalityKey{Dimension: "intraday", Lookback: "all", MinSamples: 5}][2]
	if low.Classification != "extreme_bullish" || low.DirectionalCount != 2 {
		t.Fatalf("2-sample classification = %+v", low)
	}
	if high.Classification != "insufficient_data" || high.DirectionalCount != 2 {
		t.Fatalf("5-sample classification = %+v", high)
	}
}

func TestComputeSeasonalityRollingLookbackAndCalendarBuckets(t *testing.T) {
	times := []time.Time{
		time.Date(2025, time.January, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2025, time.January, 2, 10, 30, 0, 0, time.UTC),
		time.Date(2025, time.January, 3, 10, 1, 0, 0, time.UTC),
	}
	bars := make([]marketdata.Bar, len(times))
	for i, at := range times {
		close := 2.0
		if i == 2 {
			close = 0
		}
		bars[i] = marketdata.Bar{T: float64(at.UnixMilli()), O: 1, H: 2, L: 0, C: close}
	}
	cols := ComputeCausalSeasonality(marketdata.SeriesFromBars(bars), []SeasonalitySpec{{Dimension: "intraday", Lookback: "1d", LookbackDays: 1, MinSamples: 1}})
	entry := cols[SeasonalityKey{Dimension: "intraday", Lookback: "1d", MinSamples: 1}][2]
	if entry.DirectionalCount != 1 || entry.Bullish != 1 || entry.Bearish != 0 || entry.Classification != "extreme_bullish" {
		t.Fatalf("rolling 1d entry = %+v, want only Jan 2 bullish observation", entry)
	}

	for _, test := range []struct {
		at        time.Time
		dimension string
		want      string
	}{
		{time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC), "weekOfYear", "Week 53"},
		{time.Date(2021, time.January, 4, 0, 0, 0, 0, time.UTC), "weekOfYear", "Week 1"},
		{time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC), "dayOfWeek", "Friday"},
		{time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC), "monthly", "January"},
	} {
		got, _ := seasonalityBucketNames(test.at.UnixMilli(), test.dimension)
		if got != test.want {
			t.Errorf("%s bucket at %s = %q, want %q", test.dimension, test.at, got, test.want)
		}
	}
}
