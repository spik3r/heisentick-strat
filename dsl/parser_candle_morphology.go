package dsl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Candle morphology filters mirror engine/dsl/spec/parseFilters.js. The
// feature definitions live in the JavaScript research module; this parser
// only has to agree with it about which lines are legal and what they mean.

type candleMorphologyFeature struct {
	id       string
	windowed bool
}

var candleMorphologyFeatures = map[string]candleMorphologyFeature{
	"body ratio":            {id: "bodyRatio"},
	"signed body ratio":     {id: "signedBodyRatio"},
	"upper wick ratio":      {id: "upperWickRatio"},
	"lower wick ratio":      {id: "lowerWickRatio"},
	"close location":        {id: "closeLocation"},
	"open location":         {id: "openLocation"},
	"wick balance":          {id: "wickBalance"},
	"range percentile":      {id: "rangePercentile", windowed: true},
	"body percentile":       {id: "bodyPercentile", windowed: true},
	"relative range":        {id: "relativeRange", windowed: true},
	"hammer score":          {id: "hammerShapeScore"},
	"shooting star score":   {id: "shootingStarShapeScore"},
	"bullish impulse score": {id: "largeBullishImpulseScore", windowed: true},
	"bearish impulse score": {id: "largeBearishImpulseScore", windowed: true},
}

var candlePathFeatures = []string{
	"path efficiency", "time of high", "time of low", "low before high", "time above open", "reversal count",
}

const (
	candleFeatureExample = "candle body ratio at least 0.70"
	candleDefaultWindow  = 100.0
)

func candleFeatureNames() string {
	names := make([]string, 0, len(candleMorphologyFeatures))
	for name := range candleMorphologyFeatures {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// parseCandleMorphology handles `candle <feature> [window] at least|at most X`.
// It reports whether the line was a morphology filter, so `candle in (...)`
// still falls through to the trigger-candle parser.
func (p *parser) parseCandleMorphology(tokens []string) bool {
	if len(tokens) < 2 || !strings.EqualFold(tokens[0], "candle") || strings.EqualFold(tokens[1], "in") {
		return false
	}
	atIndex := -1
	for index, token := range tokens {
		if strings.EqualFold(token, "at") {
			atIndex = index
			break
		}
	}
	if atIndex < 2 {
		p.errorAt(nil, nil, "candle filter must read: "+candleFeatureExample, candleFeatureExample)
		return true
	}
	if atIndex+2 >= len(tokens) {
		p.errorAt(nil, nil, "candle filter needs a numeric threshold, for example: "+candleFeatureExample, candleFeatureExample)
		return true
	}
	bound := strings.ToLower(tokens[atIndex+1])
	if bound != "least" && bound != "most" {
		p.errorAt(nil, nil, `candle filter must use "at least" or "at most", for example: `+candleFeatureExample, candleFeatureExample)
		return true
	}
	threshold, err := strconv.ParseFloat(tokens[atIndex+2], 64)
	if err != nil {
		p.errorAt(nil, nil, "candle filter needs a numeric threshold, for example: "+candleFeatureExample, candleFeatureExample)
		return true
	}
	words := tokens[1:atIndex]
	window := candleDefaultWindow
	hasWindow := false
	if len(words) > 0 {
		if parsed, numberErr := strconv.ParseFloat(words[len(words)-1], 64); numberErr == nil {
			window = parsed
			hasWindow = true
			words = words[:len(words)-1]
		}
	}
	name := strings.ToLower(strings.Join(words, " "))
	for _, pathFeature := range candlePathFeatures {
		if name == pathFeature {
			p.errorAt(nil, nil, fmt.Sprintf("candle %s needs tick-path data, which no stored dataset provides; remove the filter or supply a tick-path source.", name), "")
			return true
		}
	}
	feature, ok := candleMorphologyFeatures[name]
	if !ok {
		p.errorAt(nil, nil, fmt.Sprintf("Unknown candle feature %q. Supported: %s.", name, candleFeatureNames()), candleFeatureExample)
		return true
	}
	if hasWindow && !feature.windowed {
		p.errorAt(nil, nil, fmt.Sprintf("candle %s does not take a window.", name), "")
		return true
	}
	operator := "min"
	if bound == "most" {
		operator = "max"
	}
	entry := map[string]any{
		"feature": feature.id,
		"label":   name,
		"op":      operator,
		"value":   threshold,
	}
	if feature.windowed {
		entry["window"] = window
	} else {
		entry["window"] = nil
	}
	existing, _ := p.config["candleMorphologyFilters"].([]any)
	p.config["candleMorphologyFilters"] = append(existing, entry)
	return true
}
