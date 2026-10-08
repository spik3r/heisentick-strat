package engine

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

type sourceEntryRoute struct {
	source string
	entry  string
}

var supportedSourceEntryRoutes = []sourceEntryRoute{
	{source: "4h", entry: "15m"},
	{source: "4h", entry: "30m"},
	{source: "1h", entry: "5m"},
	{source: "15m", entry: "1m"},
}

func supportedSourceEntryRoute(symbol, source, entry string) bool {
	for _, route := range supportedSourceEntryRoutes {
		if route.source == source && route.entry == entry {
			return true
		}
	}
	return false
}

// sourceEntryRouteAllowed adds the retest-on-entry-timeframe routes (4h source
// with a 15m, 30m or 1h chart) to the dispatch routes above.
func sourceEntryRouteAllowed(cfg dsl.Config, symbol, source, entry string) bool {
	if supportedSourceEntryRoute(symbol, source, entry) {
		return true
	}
	supplyDemand, _ := cfg["supplyDemand"].(map[string]any)
	retest, _ := supplyDemand["retestOnEntryTimeframe"].(int)
	return retest != 0 && source == "4h" && (entry == "15m" || entry == "30m" || entry == "1h")
}

// ScheduledEntry is a causal source setup decision assigned to one actual
// chart decision bar. C5 dispatches its captured order through the chart
// broker, while C6 remains responsible only for choosing a pilot strategy.
type ScheduledEntry struct {
	SourceIndex int
	ChartIndex  int
	SourceClose float64
	EntryTime   float64
}

func inferredDuration(series marketdata.Series) float64 {
	duration := 0.0
	for i := 1; i < series.Len(); i++ {
		delta := series.T[i] - series.T[i-1]
		if delta > 0 && (duration == 0 || delta < duration) {
			duration = delta
		}
	}
	return duration
}

// ScheduleSourceEvents maps each distinct completed source decision to the
// first actual chart decision close strictly after it. It keeps source bars
// before chart coverage for warmup but does not replay their events. It never
// invents bars in a session gap or dispatches on the source-close boundary.
func ScheduleSourceEvents(source, chart marketdata.Series, sourceIndices []int) []ScheduledEntry {
	sourceDuration, chartDuration := inferredDuration(source), inferredDuration(chart)
	if sourceDuration <= 0 || chartDuration <= 0 {
		return nil
	}
	seen := map[int]bool{}
	out := make([]ScheduledEntry, 0, len(sourceIndices))
	chartIndex := 0
	firstChartOpen := chart.T[0]
	for _, sourceIndex := range sourceIndices {
		if sourceIndex < 0 || sourceIndex >= source.Len() || seen[sourceIndex] {
			continue
		}
		seen[sourceIndex] = true
		sourceClose := source.T[sourceIndex] + sourceDuration
		// Source history can start well before the entry route has coverage.
		// Keep those bars available for context warmup, but never replay a
		// historical intent onto the first chart candle.
		if sourceClose < firstChartOpen {
			continue
		}
		for chartIndex < chart.Len() && chart.T[chartIndex]+chartDuration <= sourceClose {
			chartIndex++
		}
		if chartIndex >= chart.Len() {
			continue
		}
		out = append(out, ScheduledEntry{SourceIndex: sourceIndex, ChartIndex: chartIndex, SourceClose: sourceClose, EntryTime: chart.T[chartIndex] + chartDuration})
	}
	return out
}
