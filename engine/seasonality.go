package engine

import (
	"github.com/spik3r/heisentick-strat/contextcols"
)

type seasonalityFilter struct {
	Dimension       string
	Lookback        string
	LookbackDays    int
	MinSamples      int
	Mode            string
	Classifications []string
}

func seasonalityFiltersFromConfig(value any) []seasonalityFilter {
	rows, _ := value.([]any)
	filters := make([]seasonalityFilter, 0, len(rows))
	for _, row := range rows {
		filter, ok := row.(map[string]any)
		if !ok {
			continue
		}
		lookback := stringValue(filter, "lookback", "all")
		lookbackDays := intFromAny(filter["lookbackDays"], 0)
		minSamples := intValue(filter, "minSamples", 5)
		if minSamples <= 0 {
			minSamples = 5
		}
		filters = append(filters, seasonalityFilter{
			Dimension:       stringValue(filter, "dimension", ""),
			Lookback:        lookback,
			LookbackDays:    lookbackDays,
			MinSamples:      minSamples,
			Mode:            stringValue(filter, "mode", "classification"),
			Classifications: stringSliceValue(filter["classifications"]),
		})
	}
	return filters
}

func seasonalitySpecsFromConfig(value any) []contextcols.SeasonalitySpec {
	filters := seasonalityFiltersFromConfig(value)
	specs := make([]contextcols.SeasonalitySpec, 0, len(filters))
	for _, filter := range filters {
		specs = append(specs, contextcols.SeasonalitySpec{
			Dimension: filter.Dimension, Lookback: filter.Lookback,
			LookbackDays: filter.LookbackDays, MinSamples: filter.MinSamples,
		})
	}
	return specs
}

func (b *broker) seasonalityGatesOK(i int, side side) bool {
	for _, filter := range b.params.SeasonalityFilters {
		entries, ok := b.cols.Seasonality[contextcols.SeasonalityKey{Dimension: filter.Dimension, Lookback: filter.Lookback, MinSamples: filter.MinSamples}]
		if !ok || i < 0 || i >= len(entries) {
			return false
		}
		entry := entries[i]
		if entry.DirectionalCount < filter.MinSamples {
			return false
		}
		if filter.Mode == "supportsEntry" {
			bullish := entry.Classification == "bullish" || entry.Classification == "extreme_bullish"
			bearish := entry.Classification == "bearish" || entry.Classification == "extreme_bearish"
			if side == sideLong && bullish || side == sideShort && bearish {
				continue
			}
			return false
		}
		if !containsString(filter.Classifications, entry.Classification) {
			return false
		}
	}
	return true
}
