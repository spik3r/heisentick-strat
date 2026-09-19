package report

import (
	"fmt"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

// defaultStartEquity is the equity every report run starts from. It matches
// the engine's normalized default and is the base of every drawdown figure.
const defaultStartEquity = 10000

// runCostRows prepares the route once and runs it per cost mode. The trades
// of the primary row are returned alongside the rows; tradeContext asks the
// engine for the per-trade context columns the annotated trade list needs.
func runCostRows(cfg dsl.Config, route Route, strategy string, modes []CostMode, primaryIndex int, bps *float64, tradeContext bool) ([]CostRow, []engine.Trade, *engine.PreparedRun, error) {
	prepared, err := engine.PrepareRun(engine.RunRequest{
		Config:             cfg,
		Series:             route.Series,
		SourceSeries:       route.SourceSeries,
		HTFSeries:          route.HTFSeries,
		SourceHTFSeries:    route.SourceHTFSeries,
		StrategyID:         strategy,
		Symbol:             route.Symbol,
		Timeframe:          route.TF,
		SourceTimeframe:    route.SourceTimeframe,
		HigherTimeframe:    route.HigherTimeframe,
		RangeMethod:        route.Range,
		ReportTradeContext: tradeContext,
		Costs:              engine.Costs{FillOn: "close", StartEquity: defaultStartEquity},
	})
	if err != nil {
		return nil, nil, nil, err
	}
	rows, trades, err := runPreparedCostRows(prepared, modes, primaryIndex, bps)
	if err != nil {
		return nil, nil, nil, err
	}
	return rows, trades, prepared, nil
}

// RunCostRowsFromShared runs one config variant against a shared route
// context and returns its cost rows. The grid command uses it.
func RunCostRowsFromShared(shared *engine.SharedRunContext, cfg dsl.Config, modes []CostMode) ([]CostRow, error) {
	prepared, err := shared.PrepareVariant(cfg)
	if err != nil {
		return nil, err
	}
	rows, _, err := runPreparedCostRows(prepared, modes, -1, nil)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// runPreparedCostRows runs each cost mode in order and summarises it. The
// trades of the mode at captureTradesAt are copied out (an empty, non-nil
// slice when that mode produced none); -1 captures nothing. A checked run or
// summary error discards every row.
func runPreparedCostRows(prepared *engine.PreparedRun, modes []CostMode, captureTradesAt int, slippageBps *float64) ([]CostRow, []engine.Trade, error) {
	rows := make([]CostRow, 0, len(modes))
	var captured []engine.Trade
	for index, mode := range modes {
		bps := effectiveSlippageBps(mode, slippageBps)
		result, err := prepared.RunChecked(engine.Costs{
			FillOn:      "close",
			StartEquity: defaultStartEquity,
			Slippage:    mode.Slip,
			SlippageBps: bps,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("cost mode %q: %w", mode.Label, err)
		}
		row, err := summarize(mode.Label, mode.Slip, result, bps)
		if err != nil {
			return nil, nil, fmt.Errorf("cost mode %q: %w", mode.Label, err)
		}
		rows = append(rows, row)
		if index == captureTradesAt {
			captured = append([]engine.Trade(nil), result.Trades...)
		}
	}
	if captureTradesAt >= 0 && captured == nil {
		captured = []engine.Trade{}
	}
	return rows, captured, nil
}

func effectiveSlippageBps(mode CostMode, override *float64) float64 {
	if override != nil {
		return *override
	}
	return mode.Bps
}
