package engine

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// compiledRouteAllowed mirrors the JS runtime's specSliceAllowed gate
// (engine/dsl/spec/gates.js) in full: slices() is an exact symbol+timeframe
// allowlist, and without slices() the symbols()/timeframes() market
// conditions each gate independently — an empty request symbol bypasses the
// symbols() check exactly like the JS gate. List comparisons are verbatim and
// scalar strings retain JavaScript's case-sensitive substring membership.
func compiledRouteAllowed(cfg dsl.Config, symbol string, timeframe string) bool {
	if routeSlices, declared := compiledRouteSlices(cfg["slices"]); declared {
		for _, slice := range routeSlices {
			routeSymbol, symbolOK := slice["symbol"].(string)
			routeTimeframe, timeframeOK := slice["tf"].(string)
			if !symbolOK || !timeframeOK {
				continue
			}
			if routeSymbol == symbol && routeTimeframe == timeframe {
				return true
			}
		}
		return false
	}
	symbolsRequired, symbolAllowed := compiledRouteListMatch(cfg["symbols"], symbol)
	if symbolsRequired && symbol != "" && !symbolAllowed {
		return false
	}
	timeframesRequired, timeframeAllowed := compiledRouteListMatch(cfg["timeframes"], timeframe)
	return !timeframesRequired || timeframeAllowed
}

func compiledRouteListMatch(raw any, candidate string) (required bool, matches bool) {
	if !rawLengthNonempty(raw) {
		return false, false
	}
	value := reflect.ValueOf(raw)
	if value.Kind() == reflect.String {
		return true, strings.Contains(value.String(), candidate)
	}
	return true, slices.Contains(stringSliceValue(raw), candidate)
}

func compiledRouteSlice(value any) (map[string]any, bool) {
	switch routeSlice := value.(type) {
	case map[string]any:
		return routeSlice, true
	case dsl.Config:
		return map[string]any(routeSlice), true
	default:
		return nil, false
	}
}

// compiledRouteSlices accepts the parser's JSON-facing []any container and
// the explicit plain-map or dsl.Config slice shapes used by direct Go callers.
// A nonempty supported container remains authoritative even when individual
// entries are malformed, preserving the existing fail-closed slices() behavior.
func compiledRouteSlices(value any) ([]map[string]any, bool) {
	switch routeSlices := value.(type) {
	case []map[string]any:
		return routeSlices, len(routeSlices) > 0
	case []dsl.Config:
		if len(routeSlices) == 0 {
			return nil, false
		}
		normalized := make([]map[string]any, 0, len(routeSlices))
		for _, routeSlice := range routeSlices {
			normalizedRouteSlice, _ := compiledRouteSlice(routeSlice)
			normalized = append(normalized, normalizedRouteSlice)
		}
		return normalized, true
	case []any:
		if len(routeSlices) == 0 {
			return nil, false
		}
		normalized := make([]map[string]any, 0, len(routeSlices))
		for _, raw := range routeSlices {
			if routeSlice, ok := compiledRouteSlice(raw); ok {
				normalized = append(normalized, routeSlice)
			}
		}
		return normalized, true
	default:
		return nil, false
	}
}

// compiledSliceAllowed keeps the original slices()-only entry point name used
// by the run paths; it now routes through the full gate.
func compiledSliceAllowed(cfg dsl.Config, symbol string, timeframe string) bool {
	return compiledRouteAllowed(cfg, symbol, timeframe)
}

// RouteAllowed is the exported route gate for callers outside the engine
// package (heisentick). When the timeframe label is blank it infers one from bar
// spacing, mirroring specTimeframe in the JS runtime.
func RouteAllowed(cfg dsl.Config, symbol string, timeframe string, series marketdata.Series) bool {
	if timeframe == "" {
		timeframe = inferTimeframe(series)
	}
	return compiledRouteAllowed(cfg, symbol, timeframe)
}

// inferTimeframe mirrors specTimeframe in engine/dsl/spec/gates.js: the most
// common bar-to-bar delta over the first 200 bars (ignoring gaps above 4h),
// ties broken by the smaller delta, formatted as "Nm" below an hour and "Nh"
// at or above it.
func inferTimeframe(series marketdata.Series) string {
	counts := map[float64]int{}
	n := series.Len() - 1
	if n > 200 {
		n = 200
	}
	for j := 1; j <= n; j++ {
		delta := series.T[j] - series.T[j-1]
		if delta <= 0 || delta > 4*3600_000 {
			continue
		}
		counts[delta]++
	}
	bestDelta := 0.0
	bestCount := 0
	for delta, count := range counts {
		if count > bestCount || (count == bestCount && count > 0 && delta < bestDelta) {
			bestDelta, bestCount = delta, count
		}
	}
	minutes := 0
	if bestCount > 0 {
		minutes = int(math.Round(bestDelta / 60_000))
	}
	if minutes >= 60 {
		return fmt.Sprintf("%dh", int(math.Round(float64(minutes)/60)))
	}
	return fmt.Sprintf("%dm", minutes)
}
