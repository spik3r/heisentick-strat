package main

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

func TestRunPreparedCostRowsPropagatesCheckedErrorAndDiscardsPartialOutputs(t *testing.T) {
	parsed, route, strategy := loadCheckedCostCase(t)
	prepared, err := engine.PrepareRun(checkedCostRequest(parsed.Config, route, strategy))
	if err != nil {
		t.Fatalf("prepare run: %v", err)
	}

	rows, trades, err := runPreparedCostRows(prepared, []costMode{
		{Label: "finite", Slip: 0},
		{Label: "max slip", Slip: math.MaxFloat64},
	}, 0)
	assertCheckedCostError(t, err, `cost mode "max slip":`)
	if rows != nil || trades != nil {
		t.Fatalf("checked error returned partial rows/trades: rows=%#v trades=%#v", rows, trades)
	}
}

func TestCostRowWrappersPropagateCheckedError(t *testing.T) {
	parsed, route, strategy := loadCheckedCostCase(t)
	modes := []costMode{{Label: "max slip", Slip: math.MaxFloat64}}

	t.Run("report wrapper", func(t *testing.T) {
		rows, trades, prepared, err := runCostRows(parsed.Config, route, strategy, modes, 0)
		assertCheckedCostError(t, err, `cost mode "max slip":`)
		if rows != nil || trades != nil || prepared != nil {
			t.Fatalf("checked error returned partial report outputs: rows=%#v trades=%#v prepared=%#v", rows, trades, prepared)
		}
	})

	t.Run("shared grid wrapper", func(t *testing.T) {
		shared, err := engine.PrepareSharedRunContext(checkedCostRequest(parsed.Config, route, strategy))
		if err != nil {
			t.Fatalf("prepare shared context: %v", err)
		}
		rows, err := runCostRowsFromShared(shared, parsed.Config, modes)
		assertCheckedCostError(t, err, `cost mode "max slip":`)
		if rows != nil {
			t.Fatalf("checked error returned partial grid rows: %#v", rows)
		}
	})
}

func TestCostRowWrappersPropagateSummaryError(t *testing.T) {
	parsed, route, strategy := loadCheckedCostNamedCase(t, "money-partial-exit")
	parsed.Config["riskUsd"] = math.MaxFloat64 / 5
	modes := []costMode{{Label: "summary overflow", Slip: 0}}
	want := `cost mode "summary overflow": summary grossWin contains non-finite value`

	t.Run("report wrapper", func(t *testing.T) {
		rows, trades, prepared, err := runCostRows(parsed.Config, route, strategy, modes, 0)
		if err == nil || err.Error() != want {
			t.Fatalf("report wrapper error = %v, rows=%+v, want %q", err, rows, want)
		}
		if rows != nil || trades != nil || prepared != nil {
			t.Fatalf("summary error returned partial report outputs: rows=%#v trades=%#v prepared=%#v", rows, trades, prepared)
		}
	})

	t.Run("shared grid wrapper", func(t *testing.T) {
		shared, err := engine.PrepareSharedRunContext(checkedCostRequest(parsed.Config, route, strategy))
		if err != nil {
			t.Fatalf("prepare shared context: %v", err)
		}
		rows, err := runCostRowsFromShared(shared, parsed.Config, modes)
		if err == nil || err.Error() != want {
			t.Fatalf("shared grid wrapper error = %v, rows=%+v, want %q", err, rows, want)
		}
		if rows != nil {
			t.Fatalf("summary error returned partial grid rows: %#v", rows)
		}
	})
}

func TestReportAndGridRejectCheckedOverflowBeforeOutput(t *testing.T) {
	maxFloat := strconv.FormatFloat(math.MaxFloat64, 'g', -1, 64)

	t.Run("report", func(t *testing.T) {
		dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "money-risk-sizing")
		var out bytes.Buffer
		err := run([]string{
			"report",
			"--dsl-file=" + dslFile,
			"--symbol=" + expected.Symbol,
			"--tf=" + expected.Timeframe,
			"--range=" + expected.RangeMethod,
			"--slippage=" + maxFloat,
			"--json-only=1",
			"--data-root=" + dataRoot,
		}, &out)
		assertCheckedCostError(t, err, "cost mode ")
		if out.Len() != 0 {
			t.Fatalf("report wrote %d bytes before checked error: %q", out.Len(), out.String())
		}
	})

	t.Run("one-variant grid", func(t *testing.T) {
		dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "money-risk-sizing")
		var out bytes.Buffer
		err := run([]string{
			"grid",
			"--dsl-file=" + dslFile,
			"--symbol=" + expected.Symbol,
			"--tf=" + expected.Timeframe,
			"--range=" + expected.RangeMethod,
			"--set=riskUsd=200",
			"--slippage=" + maxFloat,
			"--json-only=1",
			"--data-root=" + dataRoot,
		}, &out)
		assertCheckedCostError(t, err, "variant 0: cost mode ")
		if out.Len() != 0 {
			t.Fatalf("grid wrote %d bytes before checked error: %q", out.Len(), out.String())
		}
	})
}

func loadCheckedCostCase(t *testing.T) (dsl.ParseResult, loadedRoute, string) {
	t.Helper()
	return loadCheckedCostNamedCase(t, "money-risk-sizing")
}

func loadCheckedCostNamedCase(t *testing.T, name string) (dsl.ParseResult, loadedRoute, string) {
	t.Helper()
	dslFile, dataRoot, expected := writeConformanceCLIInputs(t, name)
	parsed, err := loadDSLFile(dslFile)
	if err != nil {
		t.Fatalf("load DSL: %v", err)
	}
	route, err := loadRoute(flagSet{
		"symbol":    []string{expected.Symbol},
		"tf":        []string{expected.Timeframe},
		"range":     []string{expected.RangeMethod},
		"data-root": []string{dataRoot},
	}, parsed.Config)
	if err != nil {
		t.Fatalf("load route: %v", err)
	}
	return parsed, route, strategyID(parsed, dslFile)
}

func checkedCostRequest(cfg dsl.Config, route loadedRoute, strategy string) engine.RunRequest {
	return engine.RunRequest{
		Config:          cfg,
		Series:          route.Series,
		HTFSeries:       route.HTFSeries,
		StrategyID:      strategy,
		Symbol:          route.Symbol,
		Timeframe:       route.TF,
		HigherTimeframe: route.HigherTimeframe,
		RangeMethod:     route.Range,
		Costs:           engine.Costs{FillOn: "close", StartEquity: 10000},
	}
}

func assertCheckedCostError(t *testing.T, err error, context string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected checked run error")
	}
	message := err.Error()
	if !strings.Contains(message, context) || !strings.Contains(message, "contains non-finite value") {
		t.Fatalf("checked error = %q, want context %q and non-finite detail", message, context)
	}
}
