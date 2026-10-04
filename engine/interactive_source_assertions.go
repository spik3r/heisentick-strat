package engine

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
)

const InteractiveSourceAssertionsSchema = "dsl-source-assertions-v1"

// InteractiveSourceAssertions checks a caller's source-derived registry
// snapshot. These values never override Go's parsed config. The caller remains
// responsible for binding the exact registered source and executable identity.
type InteractiveSourceAssertions struct {
	Schema               string                     `json:"schema"`
	StrategyID           string                     `json:"strategyId"`
	Params               map[string]float64         `json:"params"`
	ContextOptions       map[string]json.RawMessage `json:"contextOptions"`
	ContextRequirements  []string                   `json:"contextRequirements"`
	PreferredRangeMethod *string                    `json:"preferredRangeMethod"`
}

func draftSourceAssertions(fields map[string]json.RawMessage, parsed dsl.ParseResult, symbol, timeframe string) (*InteractiveSourceAssertions, error) {
	raw, present := fields["sourceAssertions"]
	if !present {
		return nil, nil
	}
	claims, err := draftObject(raw, map[string]bool{"schema": true, "strategyId": true, "params": true,
		"contextOptions": true, "contextRequirements": true, "preferredRangeMethod": true})
	if err != nil {
		return nil, err
	}
	if len(claims) != 6 {
		return nil, fmt.Errorf("source assertions require all six explicit fields")
	}
	schema, err := draftString(claims, "schema")
	if err != nil || schema != InteractiveSourceAssertionsSchema {
		return nil, fmt.Errorf("%w: source assertions schema", ErrInteractiveUnsupported)
	}
	id, err := draftString(claims, "strategyId")
	if err != nil {
		return nil, err
	}
	wantTimeframe := map[string]string{
		"dslDualEmaResumptionXauusdDaily":    "1d",
		"dslDualEmaResumptionXauusdFourHour": "4h",
	}[id]
	if wantTimeframe == "" || timeframe != wantTimeframe || symbol != "XAUUSD" ||
		setupTypeFromAny(parsed.Config["setupType"]) != string(dsl.FamilyDualEMAResumption) {
		return nil, fmt.Errorf("%w: source assertions strategy/family/route", ErrInteractiveUnsupported)
	}
	keys := []string{"fastEmaLen", "slowEmaLen", "slowRiseBars", "atrLen", "stopAtr", "trailAtr", "allowLong", "allowShort", "riskUsd"}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	params, err := draftObject(claims["params"], allowed)
	if err != nil {
		return nil, err
	}
	if len(params) != len(keys) {
		return nil, fmt.Errorf("%w: source assertions require exactly nine Dual parameters", ErrInteractiveUnsupported)
	}
	canonical := make(map[string]float64, len(keys))
	dual := mapValue(parsed.Config, "dualEmaResumption")
	for _, key := range keys {
		var value float64
		if string(params[key]) == "null" || json.Unmarshal(params[key], &value) != nil || !isFiniteDerivedOutput(value) {
			return nil, fmt.Errorf("source assertions params.%s must be a finite number", key)
		}
		config := dual
		if key == "allowLong" || key == "allowShort" || key == "riskUsd" {
			config = parsed.Config
		}
		want := numberFromAny(config[key], math.NaN())
		if !isFiniteDerivedOutput(want) || value != want {
			return nil, fmt.Errorf("%w: source assertions params.%s differs from Go source", ErrInteractiveUnsupported, key)
		}
		canonical[key] = want
	}
	options, err := draftObject(claims["contextOptions"], map[string]bool{})
	if err != nil {
		return nil, err
	}
	var requirements []string
	if json.Unmarshal(claims["contextRequirements"], &requirements) != nil || len(requirements) != 1 || requirements[0] != "sessions" {
		return nil, fmt.Errorf("%w: Dual source assertions require exactly sessions context capability", ErrInteractiveUnsupported)
	}
	preference, err := draftString(claims, "preferredRangeMethod")
	if err != nil || preference != "zone" {
		return nil, fmt.Errorf("%w: Dual source assertions require zone registry preference", ErrInteractiveUnsupported)
	}
	// The registry preference is metadata, not the source's actual range method.
	// Sessions acknowledges the existing context capability; it adds no new gate.
	return &InteractiveSourceAssertions{Schema: schema, StrategyID: id, Params: canonical,
		ContextOptions: options, ContextRequirements: requirements, PreferredRangeMethod: &preference}, nil
}
