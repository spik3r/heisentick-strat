package engine

import "github.com/spik3r/heisentick-strat/marketdata"

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

// ScheduleSourceEvents maps each distinct completed 4h source decision to the
// first actual 15m decision close strictly after it. It never invents bars in
// a session gap and never dispatches on the coincident source-close boundary.
func ScheduleSourceEvents(source, chart marketdata.Series, sourceIndices []int) []ScheduledEntry {
	sourceDuration, chartDuration := inferredDuration(source), inferredDuration(chart)
	if sourceDuration <= 0 || chartDuration <= 0 {
		return nil
	}
	seen := map[int]bool{}
	out := make([]ScheduledEntry, 0, len(sourceIndices))
	chartIndex := 0
	for _, sourceIndex := range sourceIndices {
		if sourceIndex < 0 || sourceIndex >= source.Len() || seen[sourceIndex] {
			continue
		}
		seen[sourceIndex] = true
		sourceClose := source.T[sourceIndex] + sourceDuration
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
