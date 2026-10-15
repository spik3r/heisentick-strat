package dsl

import (
	"fmt"
	"strings"
)

// ParabolicSARSpec is the opt-in ordinary-broker filter contract. An empty
// Predicate requests the series only and does not change entry admission.
type ParabolicSARSpec struct {
	Start     float64
	Step      float64
	Max       float64
	Predicate string
}

// DecodeParabolicSAR validates authored and direct configs identically. The
// bool distinguishes an absent filter from an invalid present value.
func DecodeParabolicSAR(cfg Config) (ParabolicSARSpec, bool, error) {
	spec := ParabolicSARSpec{Start: .02, Step: .02, Max: .2}
	raw, present := cfg["parabolicSAR"]
	if !present {
		return spec, false, nil
	}
	values, ok := raw.(map[string]any)
	if typed, typedOK := raw.(Config); typedOK {
		values, ok = map[string]any(typed), true
	}
	if !ok || values == nil {
		return spec, true, fmt.Errorf("SAR config must be an object")
	}
	for key := range values {
		if key != "start" && key != "step" && key != "max" && key != "predicate" {
			return spec, true, fmt.Errorf("SAR config has unknown field %q", key)
		}
	}
	for _, field := range []struct {
		key string
		out *float64
	}{{"start", &spec.Start}, {"step", &spec.Step}, {"max", &spec.Max}} {
		if _, exists := values[field.key]; exists {
			value, err := clockConfigFloat(values, field.key)
			if err != nil {
				return spec, true, fmt.Errorf("SAR %w", err)
			}
			*field.out = value
		}
	}
	if spec.Start <= 0 || spec.Step <= 0 || spec.Start > spec.Max || spec.Step > spec.Max || spec.Max > 1 {
		return spec, true, fmt.Errorf("SAR requires 0 < start <= max <= 1 and 0 < step <= max")
	}
	if raw, exists := values["predicate"]; exists {
		predicate, ok := raw.(string)
		if !ok || !sarPredicates[predicate] {
			return spec, true, fmt.Errorf("SAR predicate is invalid")
		}
		spec.Predicate = predicate
	}
	// These routes have different source/fill clocks or independent admission
	// contracts. Refuse them rather than silently ignoring a new filter.
	if source, exists := cfg["sourceTimeframe"]; exists && source != "" {
		return spec, true, fmt.Errorf("SAR supports chart-timeframe ordinary broker routes only")
	}
	if entry, exists := cfg["entryTf"]; exists && entry != "" && entry != "current" {
		return spec, true, fmt.Errorf("SAR supports chart-timeframe ordinary broker routes only")
	}
	family, _ := cfg["setupType"].(string)
	if typed, ok := cfg["setupType"].(FamilyID); ok {
		family = string(typed)
	}
	if !sarBrokerFamilies[FamilyID(family)] {
		return spec, true, fmt.Errorf("SAR is not supported by setup family %q", family)
	}
	return spec, true, nil
}

var sarPredicates = map[string]bool{
	"bullish": true, "bearish": true, "closeAbove": true, "closeBelow": true,
	"flipBullish": true, "flipBearish": true, "supportsEntry": true,
}

var sarBrokerFamilies = map[FamilyID]bool{
	FamilyFailedBreakout: true, FamilyFlagContinuation: true, FamilyBreakRetest: true,
	FamilyOpeningRangeBreakout: true, FamilySessionBreakHold: true, FamilyChannelBreakHold: true,
	FamilyInsideDayExpansion: true, FamilyDayOpenReclaim: true, FamilySupplyDemand: true,
	FamilyDoubleTopBottom: true, FamilyRangeBreakFake: true, FamilyTrendPullback: true,
	FamilyDualEMAResumption: true, FamilySMAGoldenCross: true, FamilyFibContinuation: true,
	FamilyTriplePushExhaustion: true, FamilyVWAPExtensionFade: true, FamilyVolumeAnomalyExhaustion: true,
	FamilyElderTripleScreen: true, FamilyPriceMomentum: true, FamilyFairValueGap: true,
	FamilyKeltnerReversion: true, FamilyKeltnerExpansion: true, FamilyNamedLevelSweep: true, FamilyNamedLevelFlag: true,
}

func (p *parser) parseParabolicSAR(line logicalLine, tokens []string) {
	if p.dslVersion < 7 {
		p.err(line, "SAR is available only in dsl v7", "")
		return
	}
	if line.section != "filters" {
		p.err(line, "SAR directives are only allowed in filters { ... }", "")
		return
	}
	if len(tokens) != 3 {
		p.err(line, "SAR requires start N, step N, max N, trend bullish/bearish, close above/below, reverses bullish/bearish, or supports entry", "")
		return
	}
	key := strings.ToLower(tokens[1])
	var value any
	switch key {
	case "start", "step", "max":
		number, ok := parseSMAFiniteNumber(tokens[2])
		if !ok || number <= 0 || number > 1 {
			p.err(line, "SAR acceleration values must be finite numbers greater than zero and at most one", "")
			return
		}
		value = number
	default:
		phrases := map[string]string{"trend bullish": "bullish", "trend bearish": "bearish", "close above": "closeAbove", "close below": "closeBelow", "reverses bullish": "flipBullish", "reverses bearish": "flipBearish", "supports entry": "supportsEntry"}
		predicate, ok := phrases[strings.ToLower(strings.Join(tokens[1:], " "))]
		if !ok {
			p.err(line, "SAR predicate must be trend bullish/bearish, close above/below, reverses bullish/bearish, or supports entry", "")
			return
		}
		key, value = "predicate", predicate
	}
	if p.sarFields == nil {
		p.sarFields = make(map[string]bool)
	}
	if p.sarFields[key] {
		p.err(line, "SAR supports one start, one step, one max, and one predicate", "")
		return
	}
	p.sarFields[key] = true
	values := copyMap(p.config["parabolicSAR"])
	for key, value := range map[string]float64{"start": .02, "step": .02, "max": .2} {
		if values[key] == nil {
			values[key] = value
		}
	}
	values[key] = value
	p.config["parabolicSAR"] = values
}

func (p *parser) validateParabolicSAR() {
	if _, _, err := DecodeParabolicSAR(p.config); err != nil {
		p.errorAt(nil, nil, err.Error(), "")
	}
}
