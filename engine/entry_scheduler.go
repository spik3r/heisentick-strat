package engine

import (
	"fmt"
	"math"

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

// supplyDemandEntryRetestEnabled agrees with parameter boolean decoding while
// refusing malformed present values instead of silently choosing legacy execution.
func supplyDemandEntryRetestEnabled(cfg dsl.Config) (bool, error) {
	sd := mapValue(cfg, "supplyDemand")
	value, present := sd["retestOnEntryTimeframe"]
	if !present {
		return false, nil
	}
	switch v := value.(type) {
	case bool, int, int64:
		return boolFromAny(value, false), nil
	case float64:
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return v != 0, nil
		}
	}
	return false, fmt.Errorf("supplyDemand.retestOnEntryTimeframe must be a boolean or finite number")
}

func validateSupplyDemandEntryRetest(cfg dsl.Config, timeframe string) error {
	enabled, err := supplyDemandEntryRetestEnabled(cfg)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilySupplyDemand) {
		return fmt.Errorf("retest on entry timeframe requires supply demand")
	}
	entry := stringValue(cfg, "entryTf", "current")
	if sourceTimeframeFromConfig(cfg) != "4h" || (entry != "15m" && entry != "30m" && entry != "1h") {
		return fmt.Errorf("retest on entry timeframe requires source timeframe 4h and entryTf 15m, 30m or 1h")
	}
	if timeframe != entry {
		return fmt.Errorf("retest on entry timeframe requires execution at declared entryTf %s, got %s", entry, timeframe)
	}
	return nil
}

// sourceEntryRouteAllowed preserves legacy routes when the mode is disabled.
func sourceEntryRouteAllowed(cfg dsl.Config, symbol, source, entry string) bool {
	enabled, err := supplyDemandEntryRetestEnabled(cfg)
	if err != nil {
		return false
	}
	if enabled {
		return setupTypeFromAny(cfg["setupType"]) == string(dsl.FamilySupplyDemand) && source == "4h" && (entry == "15m" || entry == "30m" || entry == "1h")
	}
	return supportedSourceEntryRoute(symbol, source, entry)
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
