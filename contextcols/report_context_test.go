package contextcols

import (
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestBuildSelectivePriorDayOwnsOHLCAndTypeOnly(t *testing.T) {
	fixture := loadParityFixture(t, "session-expansion-ranges.json", "go-context-ranges-v1")
	series := seriesFromFixture(t, fixture, columnIndex(t, fixture.Columns))
	full := Build(series, Options{})
	selective := Build(series, Options{Selective: true, NeedPriorDay: true})

	for label, pair := range map[string][2][]float64{
		"open":  {selective.PriorDayO, full.PriorDayO},
		"high":  {selective.PriorDayH, full.PriorDayH},
		"low":   {selective.PriorDayL, full.PriorDayL},
		"close": {selective.PriorDayC, full.PriorDayC},
	} {
		assertPriorDayFloatColumnsEqual(t, label, pair[0], pair[1])
	}
	if !reflect.DeepEqual(selective.PriorDayType, full.PriorDayType) {
		t.Fatal("selective prior-day type column differs from full context")
	}
	if len(selective.PriorDayType) != series.Len() {
		t.Fatalf("selective prior-day type rows = %d, want %d", len(selective.PriorDayType), series.Len())
	}
	if len(selective.OpenLocation) != 0 || len(selective.SessionPhase) != 0 || len(selective.Regime) != 0 ||
		len(selective.PriorSession.Asia.H) != 0 || len(selective.VWAP) != 0 || len(selective.VolumeSMA20) != 0 {
		t.Fatalf("selective prior-day context allocated unrelated columns: openLocation=%d sessionPhase=%d regime=%d priorAsia=%d vwap=%d volume=%d",
			len(selective.OpenLocation), len(selective.SessionPhase), len(selective.Regime),
			len(selective.PriorSession.Asia.H), len(selective.VWAP), len(selective.VolumeSMA20))
	}
}

func assertPriorDayFloatColumnsEqual(t *testing.T, label string, got []float64, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("selective prior-day %s rows = %d, want %d", label, len(got), len(want))
	}
	for i := range got {
		if got[i] == want[i] || (math.IsNaN(got[i]) && math.IsNaN(want[i])) {
			continue
		}
		t.Fatalf("selective prior-day %s[%d] = %v, want %v", label, i, got[i], want[i])
	}
}

func TestBuildSelectiveReportTradeContextIncludesEntryLabels(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 1704067200000, O: 100, H: 101, L: 99, C: 100},
		{T: 1704153600000, O: 101, H: 103, L: 100, C: 102},
	})
	cols := Build(series, Options{Selective: true, NeedReportTradeContext: true})
	if len(cols.OpenLocation) != series.Len() || len(cols.PriorDayType) != series.Len() {
		t.Fatalf("report trade context columns missing: open=%d priorDayType=%d want=%d", len(cols.OpenLocation), len(cols.PriorDayType), series.Len())
	}
}

func TestBuildSelectiveOpenLocationOwnsOnlyLocationDependencies(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 1704067200000, O: 100, H: 101, L: 99, C: 100},
		{T: 1704153600000, O: 101, H: 103, L: 100, C: 102},
	})
	cols := Build(series, Options{Selective: true, NeedOpenLocation: true})
	if len(cols.OpenLocation) != series.Len() || len(cols.PriorDayH) != series.Len() || len(cols.PriorSession.Asia.H) != series.Len() {
		t.Fatalf("selective open-location dependencies missing: open=%d priorDay=%d priorAsia=%d want=%d",
			len(cols.OpenLocation), len(cols.PriorDayH), len(cols.PriorSession.Asia.H), series.Len())
	}
	if len(cols.PriorDayType) != 0 {
		t.Fatalf("open-location ownership unexpectedly requested report-only prior-day type: %d", len(cols.PriorDayType))
	}
}

func TestBuildSelectiveRegimeTrendPreservesActiveRangeClassification(t *testing.T) {
	fixture := loadParityFixture(t, "session-expansion-ranges.json", "go-context-ranges-v1")
	series := seriesFromFixture(t, fixture, columnIndex(t, fixture.Columns))
	full := Build(series, Options{})
	selective := Build(series, Options{Selective: true, NeedRegimeTrend: true})
	active := false
	for i := 0; i < series.Len(); i++ {
		if full.Range.Active[i] == 1 {
			active = true
		}
		if selective.Regime[i] != full.Regime[i] || selective.TrendDir[i] != full.TrendDir[i] {
			t.Fatalf("selective regime at %d = (%d, %d), want (%d, %d)", i, selective.Regime[i], selective.TrendDir[i], full.Regime[i], full.TrendDir[i])
		}
	}
	if !active {
		t.Fatal("fixture did not exercise an active range")
	}
}
