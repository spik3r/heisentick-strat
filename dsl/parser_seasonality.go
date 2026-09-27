package dsl

import (
	"fmt"
	"strconv"
	"strings"
)

var seasonalityDimensions = map[string]string{
	"intraday": "intraday", "hourly": "intraday", "hour": "intraday",
	"dayofweek": "dayOfWeek", "weekday": "dayOfWeek", "dow": "dayOfWeek",
	"weekofyear": "weekOfYear", "week": "weekOfYear", "weekly": "weekOfYear",
	"monthly": "monthly", "month": "monthly",
}

var seasonalityClasses = map[string]bool{
	"insufficient_data": true, "extreme_bullish": true, "bullish": true,
	"neutral": true, "bearish": true, "extreme_bearish": true,
}

func normalizeSeasonalityToken(value string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(value))
}

func parseSeasonalityLookback(value string) (string, any, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "all" || value == "history" {
		return "all", nil, true
	}
	if strings.HasSuffix(value, "days") {
		value = strings.TrimSuffix(value, "days")
	} else if strings.HasSuffix(value, "day") {
		value = strings.TrimSuffix(value, "day")
	} else if strings.HasSuffix(value, "d") {
		value = strings.TrimSuffix(value, "d")
	} else {
		return "", nil, false
	}
	days, err := strconv.Atoi(value)
	if err != nil || days <= 0 {
		return "", nil, false
	}
	return fmt.Sprintf("%dd", days), days, true
}

func (p *parser) parseSeasonality(line logicalLine, tokens []string) {
	filter, ok := parseSeasonalityFilter(tokens[1:])
	if !ok {
		p.err(line, `seasonality filter must use "seasonality <dimension> [lookback] in (<classes>) [min samples N]" or "seasonality <dimension> [lookback] supports entry"`, "")
		return
	}
	filters, _ := p.config["seasonalityFilters"].([]any)
	p.config["seasonalityFilters"] = append(filters, filter)
}

func parseSeasonalityFilter(tokens []string) (map[string]any, bool) {
	if len(tokens) == 0 {
		return nil, false
	}
	dimensionIndex := -1
	dimension := ""
	for end := 1; end <= len(tokens) && end <= 3; end++ {
		candidate := normalizeSeasonalityToken(strings.Join(tokens[:end], ""))
		if normalized, found := seasonalityDimensions[candidate]; found {
			dimensionIndex = end
			dimension = normalized
		}
	}
	if dimensionIndex < 0 {
		return nil, false
	}

	lookback, lookbackDays, minSamples := "all", any(nil), 5
	seenLookback, seenMin := false, false
	classes := make([]any, 0, len(tokens)-dimensionIndex)
	mode := ""
	for i := dimensionIndex; i < len(tokens); i++ {
		token := strings.ToLower(tokens[i])
		switch token {
		case "lookback":
			if seenLookback || i+1 >= len(tokens) {
				return nil, false
			}
			normalized, days, valid := parseSeasonalityLookback(tokens[i+1])
			if !valid {
				return nil, false
			}
			lookback, lookbackDays, seenLookback = normalized, days, true
			i++
		case "min":
			if seenMin || i+2 >= len(tokens) || !strings.EqualFold(tokens[i+1], "samples") && !strings.EqualFold(tokens[i+1], "sample") {
				return nil, false
			}
			parsed, err := strconv.Atoi(tokens[i+2])
			if err != nil || parsed <= 0 {
				return nil, false
			}
			minSamples, seenMin = parsed, true
			i += 2
		case "supports", "support", "align", "aligns":
			if mode != "" || i+1 >= len(tokens) || !strings.EqualFold(tokens[i+1], "entry") {
				return nil, false
			}
			mode = "supportsEntry"
			i++
		case "in":
			if mode != "" {
				return nil, false
			}
			mode = "classification"
		default:
			if normalized, days, valid := parseSeasonalityLookback(token); valid {
				if seenLookback {
					return nil, false
				}
				lookback, lookbackDays, seenLookback = normalized, days, true
				continue
			}
			if mode != "classification" {
				return nil, false
			}
			class := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(token))
			if !seasonalityClasses[class] {
				return nil, false
			}
			classes = append(classes, class)
		}
	}
	if mode == "" || mode == "classification" && len(classes) == 0 {
		return nil, false
	}

	filter := map[string]any{
		"dimension":    dimension,
		"lookback":     lookback,
		"lookbackDays": lookbackDays,
		"minSamples":   minSamples,
	}
	if mode == "supportsEntry" {
		filter["mode"] = "supportsEntry"
		return filter, true
	}
	filter["mode"] = "classification"
	filter["classifications"] = classes
	return filter, true
}
