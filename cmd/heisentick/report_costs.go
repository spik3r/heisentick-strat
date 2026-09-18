package main

import (
	"fmt"
	"math"
	"strconv"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

func parseSlippageBps(raw string) (*float64, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fmt.Errorf("invalid slippage-bps: %s", raw)
	}
	return &value, nil
}

func runCostRows(cfg dsl.Config, route loadedRoute, strategy string, modes []costMode, captureTradesAt int, bps ...*float64) ([]costRow, []engine.Trade, *engine.PreparedRun, error) {
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
		ReportTradeContext: captureTradesAt >= 0,
		Costs:              engine.Costs{FillOn: "close", StartEquity: 10000},
	})
	if err != nil {
		return nil, nil, nil, err
	}
	rows, trades, err := runPreparedCostRows(prepared, modes, captureTradesAt, bps...)
	if err != nil {
		return nil, nil, nil, err
	}
	return rows, trades, prepared, nil
}

func runCostRowsFromShared(shared *engine.SharedRunContext, cfg dsl.Config, modes []costMode) ([]costRow, error) {
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

func runPreparedCostRows(prepared *engine.PreparedRun, modes []costMode, captureTradesAt int, bps ...*float64) ([]costRow, []engine.Trade, error) {
	var slippageBps *float64
	if len(bps) > 0 {
		slippageBps = bps[0]
	}
	rows := make([]costRow, 0, len(modes))
	var captured []engine.Trade
	for index, mode := range modes {
		result, err := prepared.RunChecked(engine.Costs{
			FillOn:      "close",
			StartEquity: 10000,
			Slippage:    mode.Slip,
			SlippageBps: effectiveSlippageBps(mode, slippageBps),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("cost mode %q: %w", mode.Label, err)
		}
		effectiveBps := mode.Bps
		if slippageBps != nil {
			effectiveBps = *slippageBps
		}
		row, err := summarize(mode.Label, mode.Slip, result, effectiveBps)
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

func effectiveSlippageBps(mode costMode, override *float64) float64 {
	if override != nil {
		return *override
	}
	return mode.Bps
}
