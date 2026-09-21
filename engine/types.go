// Package engine runs compiled DSL strategies against market fixtures.
package engine

import (
	"encoding/json"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	runFixtureSchema = "dsl-conformance-run-fixture-v1"
	tradesSchema     = "dsl-conformance-trades-v1"
)

// Costs are the execution costs supplied by run conformance fixtures.
type Costs struct {
	FeePerUnit  float64 `json:"feePerUnit"`
	FillOn      string  `json:"fillOn"`
	Slippage    float64 `json:"slippage"`
	SlippageBps float64 `json:"slippageBps,omitempty"`
	StartEquity float64 `json:"startEquity"`
}

func (c Costs) normalized() Costs {
	if c.FillOn == "" {
		c.FillOn = "close"
	}
	if c.StartEquity == 0 {
		c.StartEquity = 10000
	}
	return c
}

// RunFixture is the language-agnostic fixture shape under strat/conformance/run.
type RunFixture struct {
	Schema           string           `json:"schema"`
	Case             string           `json:"case"`
	StrategyID       string           `json:"strategyId"`
	Symbol           string           `json:"symbol"`
	Timeframe        string           `json:"timeframe"`
	SourceTimeframe  string           `json:"sourceTimeframe,omitempty"`
	HigherTimeframe  string           `json:"higherTimeframe"`
	RangeMethod      string           `json:"rangeMethod"`
	Costs            Costs            `json:"costs"`
	Bars             []marketdata.Bar `json:"-"`
	RawBars          [][]float64      `json:"bars"`
	SourceBars       []marketdata.Bar `json:"-"`
	RawSourceBars    [][]float64      `json:"sourceBars,omitempty"`
	HTFBars          []marketdata.Bar `json:"-"`
	RawHTFBars       [][]float64      `json:"htfBars,omitempty"`
	SourceHTFBars    []marketdata.Bar `json:"-"`
	RawSourceHTFBars [][]float64      `json:"sourceHtfBars,omitempty"`
}

// RunResult mirrors strat/conformance/run/*.trades.json.
type RunResult struct {
	Case            string  `json:"case"`
	Costs           Costs   `json:"costs"`
	HigherTimeframe string  `json:"higherTimeframe"`
	RangeMethod     string  `json:"rangeMethod"`
	Schema          string  `json:"schema"`
	StrategyID      string  `json:"strategyId"`
	Symbol          string  `json:"symbol"`
	Timeframe       string  `json:"timeframe"`
	TradeCount      int     `json:"tradeCount"`
	Trades          []Trade `json:"trades"`
}

const (
	// ReasonEndOfTest reports the final-data liquidation: a position still
	// open on the last bar, closed at the last bar's close (decision D-20).
	// It is never spelled "eod"; that word stays reserved for a true
	// end-of-day flatten, which no current engine path produces.
	ReasonEndOfTest = "end-of-test"
	// ReasonRule reports a strategy close rule (D-20). The specific rule
	// identity (for example "sma-bearish-cross") is carried separately in
	// Trade.Rule.
	ReasonRule = "rule"
)

// Trade is the stable JSON trade record emitted by the JS broker.
type Trade struct {
	Entry      float64   `json:"entry"`
	EntryIndex int       `json:"entryIndex"`
	EntryT     float64   `json:"entryT"`
	Exit       float64   `json:"exit"`
	ExitIndex  int       `json:"exitIndex"`
	ExitT      float64   `json:"exitT"`
	InitialSL  float64   `json:"initialSl"`
	InitialTP  float64   `json:"initialTp"`
	Meta       TradeMeta `json:"meta"`
	Partial    bool      `json:"partial,omitempty"`
	PnL        float64   `json:"pnl"`
	Points     float64   `json:"points"`
	Reason     string    `json:"reason"`
	// Rule is the strategy-rule identity behind a Reason "rule" exit, for
	// example "sma-bearish-cross", "slow-ema" or "window-close". Empty for
	// every other reason, and omitted from the JSON then.
	Rule     string  `json:"rule,omitempty"`
	Side     string  `json:"side"`
	Size     float64 `json:"size"`
	SL       float64 `json:"sl"`
	Tag      string  `json:"tag"`
	TP       float64 `json:"tp"`
	NoTarget bool    `json:"-"`
	NoStop   bool    `json:"-"`
}

// MarshalJSON adapts absent stops and targets. Ordinary run results retain the
// default encoding and field order used by existing report goldens.
func (result RunResult) MarshalJSON() ([]byte, error) {
	type resultAlias RunResult
	var higherTimeframe any
	if result.HigherTimeframe != "" {
		higherTimeframe = result.HigherTimeframe
	}
	type tradeAlias Trade
	type noTargetTrade struct {
		tradeAlias
		InitialTP any `json:"initialTp"`
		TP        any `json:"tp"`
	}
	type noStopTrade struct {
		tradeAlias
		InitialSL any `json:"initialSl"`
		SL        any `json:"sl"`
	}
	type noBracketTrade struct {
		tradeAlias
		InitialSL any `json:"initialSl"`
		InitialTP any `json:"initialTp"`
		SL        any `json:"sl"`
		TP        any `json:"tp"`
	}
	trades := make([]any, len(result.Trades))
	for i, trade := range result.Trades {
		switch {
		case trade.NoStop && trade.NoTarget:
			trades[i] = noBracketTrade{tradeAlias: tradeAlias(trade), InitialSL: nil, InitialTP: nil, SL: nil, TP: nil}
		case trade.NoStop:
			trades[i] = noStopTrade{tradeAlias: tradeAlias(trade), InitialSL: nil, SL: nil}
		case trade.NoTarget:
			trades[i] = noTargetTrade{tradeAlias: tradeAlias(trade), InitialTP: nil, TP: nil}
		default:
			trades[i] = tradeAlias(trade)
		}
	}
	return json.Marshal(struct {
		resultAlias
		HigherTimeframe any   `json:"higherTimeframe"`
		Trades          []any `json:"trades"`
	}{resultAlias: resultAlias(result), HigherTimeframe: higherTimeframe, Trades: trades})
}

// TradeMeta is the setup-specific JSON metadata emitted with a trade.
type TradeMeta map[string]any

type side int8

const (
	sideLong  side = 1
	sideShort side = -1
)

func (s side) String() string {
	if s == sideShort {
		return "short"
	}
	return "long"
}
