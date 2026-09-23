package main

import (
	"io"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report"
)

func runForwardPrefix(args []string, out io.Writer) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
	}
	dslFile, err := flags.required("dsl-file")
	if err != nil {
		return err
	}
	parsed, err := loadDSLFile(dslFile)
	if err != nil {
		return err
	}
	route, err := loadRoute(flags, parsed.Config)
	if err != nil {
		return err
	}
	slippage, err := parseFloatFlag("slippage", flags.one("slippage", "0"))
	if err != nil {
		return err
	}
	slippageBps, err := report.ParseSlippageBps(flags.one("slippage-bps", "0"))
	if err != nil {
		return err
	}
	strategyID := report.StrategyID(parsed.Config, fileBaseName(dslFile), flags.one("dsl-id", ""))
	result, err := engine.RunPrefix(engine.RunRequest{
		Config: parsed.Config, Series: route.Series, SourceSeries: route.SourceSeries,
		HTFSeries: route.HTFSeries, SourceHTFSeries: route.SourceHTFSeries,
		StrategyID: strategyID, Symbol: route.Symbol, Timeframe: route.TF,
		SourceTimeframe: route.SourceTimeframe, HigherTimeframe: route.HigherTimeframe,
		RangeMethod: route.Range,
		Costs:       engine.Costs{FillOn: "close", Slippage: slippage, SlippageBps: *slippageBps, StartEquity: 10_000},
	})
	if err != nil {
		return err
	}
	return report.WriteJSON(out, result)
}
