package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// The same strict JSON adapter is used by the native binary and WASM export.
// All bars are complete chart candles; no unclosed tick may be supplied.
type forwardSmokeInput struct {
	Schema          string           `json:"schema"`
	Mode            string           `json:"mode"`
	StrategyID      string           `json:"strategyId"`
	StrategyVersion string           `json:"strategyVersion"`
	Symbol          string           `json:"symbol"`
	Timeframe       string           `json:"timeframe"`
	Costs           native.Costs     `json:"costs"`
	Bars            []marketdata.Bar `json:"bars"`
}

func runForwardSmokeJSON(raw string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var input forwardSmokeInput
	if err := decoder.Decode(&input); err != nil {
		return nil, fmt.Errorf("forward smoke input: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("forward smoke input has trailing JSON data")
	}
	if input.Schema != "forward-smoke-request-v1" {
		return nil, fmt.Errorf("unsupported forward smoke schema %q", input.Schema)
	}
	if input.Mode != "whole" && input.Mode != "prefix" {
		return nil, fmt.Errorf("unsupported forward smoke mode %q", input.Mode)
	}
	request := native.ForwardSmokeRequest{StrategyID: input.StrategyID, StrategyVersion: input.StrategyVersion, Symbol: input.Symbol, Timeframe: input.Timeframe, Costs: input.Costs, Series: marketdata.SeriesFromBars(input.Bars)}
	var result any
	var err error
	if input.Mode == "prefix" {
		result, err = native.RunForwardSmokePrefix(request)
	} else {
		result, err = native.RunForwardSmoke(request)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Schema          string `json:"schema"`
		StrategyVersion string `json:"strategyVersion"`
		Result          any    `json:"result"`
	}{"forward-smoke-result-v1", input.StrategyVersion, result})
}
