package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func runFixture(raw, source string) ([]byte, error) {
	fixture, err := native.DecodeRunFixture([]byte(raw))
	if err != nil {
		return nil, err
	}
	result, err := native.RunFixtureCase(fixture, source)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// runCompositionFixture is the JSON transport for the Go-owned authored
// wrappers. Sources is a JSON object keyed by every required child ID. The
// adapter never falls back to a JS strategy or an empty result on dependency
// errors; native callers receive an error and WASM emits {"error":...}.
func runCompositionFixture(rawFixture, rawSources string) ([]byte, error) {
	fixture, err := native.DecodeRunFixture([]byte(rawFixture))
	if err != nil {
		return nil, err
	}
	var sources map[string]string
	decoder := json.NewDecoder(bytes.NewBufferString(rawSources))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sources); err != nil {
		return nil, fmt.Errorf("composition sources: %w", err)
	}
	if sources == nil {
		return nil, errors.New("composition sources must be an object keyed by child ID")
	}
	request := native.RunRequest{
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		SourceSeries:    marketdata.SeriesFromBars(fixture.SourceBars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		SourceHTFSeries: marketdata.SeriesFromBars(fixture.SourceHTFBars),
		StrategyID:      fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
		SourceTimeframe: fixture.SourceTimeframe, HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod: fixture.RangeMethod, Costs: fixture.Costs,
	}
	result, err := native.RunBuiltInComposition(request, sources)
	if err != nil {
		return nil, err
	}
	result.Case = fixture.Case
	return json.Marshal(result)
}

type interactiveFailure struct {
	Schema string `json:"schema"`
	Error  struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// runInteractive returns a structured v1 response for both successes and
// failures. Native and browser callers consume the same bytes.
func runInteractive(raw, source string) []byte {
	return runInteractiveWithComposition(raw, source, false)
}

func runInteractiveVPNYHandoff(raw, source string) []byte {
	return runInteractiveWithComposition(raw, source, true)
}

func runInteractiveWithComposition(raw, source string, vpNYHandoff bool) []byte {
	if !vpNYHandoff && authoredVPWideFixtureID(raw) {
		return runAuthoredVPWideInteractive(raw, source)
	}
	var result native.InteractiveRunResult
	var err error
	schema := native.InteractiveRunSchema
	if vpNYHandoff {
		result, err = native.RunInteractiveVPNYHandoffVetoFixture([]byte(raw), source)
		schema = native.InteractiveVPNYHandoffSchema
	} else {
		result, err = native.RunInteractiveFixture([]byte(raw), source)
	}
	if err == nil {
		out, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			return out
		}
		err = marshalErr
	}
	failure := interactiveFailure{Schema: schema}
	failure.Error.Code = "invalid-request"
	if errors.Is(err, native.ErrInteractiveUnsupported) {
		failure.Error.Code = "unsupported-route"
	}
	failure.Error.Message = err.Error()
	out, _ := json.Marshal(failure)
	return out
}

// columnRunMeta is deliberately narrower than a RunFixture. The T-F0 bridge
// measures one chart-timeframe series only; callers must not silently omit
// source, higher-timeframe, or precomputed-context inputs.
type columnRunMeta struct {
	Schema          string          `json:"schema"`
	Case            string          `json:"case"`
	StrategyID      string          `json:"strategyId"`
	Symbol          string          `json:"symbol"`
	Timeframe       string          `json:"timeframe"`
	SourceTimeframe string          `json:"sourceTimeframe,omitempty"`
	HigherTimeframe string          `json:"higherTimeframe,omitempty"`
	Context         json.RawMessage `json:"context,omitempty"`
	RangeMethod     string          `json:"rangeMethod"`
	Costs           native.Costs    `json:"costs"`
}

type columnTimings struct {
	AdapterMS float64 `json:"adapterMs"`
	EngineMS  float64 `json:"engineMs"`
	PackMS    float64 `json:"packMs"`
}

type columnTradeText struct {
	Side     string           `json:"side"`
	Reason   string           `json:"reason"`
	Rule     string           `json:"rule,omitempty"`
	Tag      string           `json:"tag"`
	Meta     native.TradeMeta `json:"meta"`
	Partial  bool             `json:"partial"`
	NoStop   bool             `json:"noStop"`
	NoTarget bool             `json:"noTarget"`
}

type columnRunOutput struct {
	SummaryJSON string        `json:"summaryJSON"`
	Values      []float64     `json:"values"`
	StringsJSON string        `json:"stringsJSON"`
	Timings     columnTimings `json:"timings"`
}

const columnTradeWidth = 13

func runColumns(rawMeta, source string, columns [6][]float64) (columnRunOutput, error) {
	adapterStart := time.Now()
	decoder := json.NewDecoder(bytes.NewBufferString(rawMeta))
	decoder.DisallowUnknownFields()
	var meta columnRunMeta
	if err := decoder.Decode(&meta); err != nil {
		return columnRunOutput{}, fmt.Errorf("column metadata: %w", err)
	}
	if meta.Schema != "enginewasm-columnar-v1" {
		return columnRunOutput{}, fmt.Errorf("unsupported column schema %q", meta.Schema)
	}
	if meta.SourceTimeframe != "" || meta.HigherTimeframe != "" || len(meta.Context) != 0 {
		return columnRunOutput{}, fmt.Errorf("column bridge supports only one chart-timeframe series; source, higher-timeframe, and precomputed context inputs are unsupported")
	}
	for index, column := range columns {
		if len(column) != len(columns[0]) {
			return columnRunOutput{}, fmt.Errorf("column %d length %d does not match timestamp length %d", index, len(column), len(columns[0]))
		}
	}
	parsed, err := dsl.Parse(source)
	if err != nil {
		return columnRunOutput{}, err
	}
	if len(parsed.Errors) != 0 {
		return columnRunOutput{}, fmt.Errorf("DSL parse errors: %v", parsed.Errors)
	}
	if sourceTimeframe, _ := parsed.Config["sourceTimeframe"].(string); sourceTimeframe != "" {
		return columnRunOutput{}, fmt.Errorf("column bridge does not support strategy source timeframe %q", sourceTimeframe)
	}
	request := native.RunRequest{
		Config: parsed.Config, Series: marketdata.Series{T: columns[0], O: columns[1], H: columns[2], L: columns[3], C: columns[4], V: columns[5]},
		StrategyID: meta.StrategyID, Symbol: meta.Symbol, Timeframe: meta.Timeframe, RangeMethod: meta.RangeMethod, Costs: meta.Costs,
	}
	adapterDone := time.Now()
	engineStart := time.Now()
	result, err := native.Run(request)
	engineDone := time.Now()
	if err != nil {
		return columnRunOutput{}, err
	}
	// RunRequest does not carry a fixture case, but the bridge result must keep
	// the caller's identity so a reconstructed columnar result matches JSON.
	result.Case = meta.Case
	packStart := time.Now()
	values, stringsJSON, summaryJSON, err := packColumnResult(result)
	if err != nil {
		return columnRunOutput{}, err
	}
	return columnRunOutput{SummaryJSON: summaryJSON, Values: values, StringsJSON: stringsJSON, Timings: columnTimings{
		AdapterMS: float64(adapterDone.Sub(adapterStart).Microseconds()) / 1000,
		EngineMS:  float64(engineDone.Sub(engineStart).Microseconds()) / 1000,
		PackMS:    float64(time.Since(packStart).Microseconds()) / 1000,
	}}, nil
}

func packColumnResult(result native.RunResult) ([]float64, string, string, error) {
	values := make([]float64, 0, len(result.Trades)*columnTradeWidth)
	text := make([]columnTradeText, 0, len(result.Trades))
	for _, trade := range result.Trades {
		values = append(values, trade.Entry, float64(trade.EntryIndex), trade.EntryT, trade.Exit, float64(trade.ExitIndex), trade.ExitT, trade.InitialSL, trade.InitialTP, trade.PnL, trade.Points, trade.Size, trade.SL, trade.TP)
		text = append(text, columnTradeText{Side: trade.Side, Reason: trade.Reason, Rule: trade.Rule, Tag: trade.Tag, Meta: trade.Meta, Partial: trade.Partial, NoStop: trade.NoStop, NoTarget: trade.NoTarget})
	}
	stringsJSON, err := json.Marshal(text)
	if err != nil {
		return nil, "", "", err
	}
	summaryJSON, err := json.Marshal(struct {
		Case            string       `json:"case"`
		Costs           native.Costs `json:"costs"`
		HigherTimeframe string       `json:"higherTimeframe"`
		RangeMethod     string       `json:"rangeMethod"`
		Schema          string       `json:"schema"`
		StrategyID      string       `json:"strategyId"`
		Symbol          string       `json:"symbol"`
		Timeframe       string       `json:"timeframe"`
		TradeCount      int          `json:"tradeCount"`
	}{result.Case, result.Costs, result.HigherTimeframe, result.RangeMethod, result.Schema, result.StrategyID, result.Symbol, result.Timeframe, result.TradeCount})
	if err != nil {
		return nil, "", "", err
	}
	return values, string(stringsJSON), string(summaryJSON), nil
}
