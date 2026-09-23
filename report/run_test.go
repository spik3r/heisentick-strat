package report

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestRunPreparedCostRowsPropagatesCheckedErrorAndDiscardsPartialOutputs(t *testing.T) {
	cfg, route, strategy := loadConformanceCase(t, "money-risk-sizing")
	prepared, err := engine.PrepareRun(conformanceRunRequest(cfg, route, strategy))
	if err != nil {
		t.Fatalf("prepare run: %v", err)
	}

	rows, trades, err := runPreparedCostRows(prepared, []CostMode{
		{Label: "finite", Slip: 0},
		{Label: "max slip", Slip: math.MaxFloat64},
	}, 0, nil)
	assertCheckedCostError(t, err, `cost mode "max slip":`)
	if rows != nil || trades != nil {
		t.Fatalf("checked error returned partial rows/trades: rows=%#v trades=%#v", rows, trades)
	}
}

func TestCostRowWrappersPropagateCheckedError(t *testing.T) {
	cfg, route, strategy := loadConformanceCase(t, "money-risk-sizing")
	modes := []CostMode{{Label: "max slip", Slip: math.MaxFloat64}}

	t.Run("report wrapper", func(t *testing.T) {
		rows, trades, prepared, err := runCostRows(cfg, route, strategy, modes, 0, nil, false, nil, false)
		assertCheckedCostError(t, err, `cost mode "max slip":`)
		if rows != nil || trades != nil || prepared != nil {
			t.Fatalf("checked error returned partial report outputs: rows=%#v trades=%#v prepared=%#v", rows, trades, prepared)
		}
	})

	t.Run("shared grid wrapper", func(t *testing.T) {
		shared, err := engine.PrepareSharedRunContext(conformanceRunRequest(cfg, route, strategy))
		if err != nil {
			t.Fatalf("prepare shared context: %v", err)
		}
		rows, err := RunCostRowsFromShared(shared, cfg, modes)
		assertCheckedCostError(t, err, `cost mode "max slip":`)
		if rows != nil {
			t.Fatalf("checked error returned partial grid rows: %#v", rows)
		}
	})
}

func TestCostRowWrappersPropagateSummaryError(t *testing.T) {
	cfg, route, strategy := loadConformanceCase(t, "money-partial-exit")
	cfg["riskUsd"] = math.MaxFloat64 / 5
	modes := []CostMode{{Label: "summary overflow", Slip: 0}}
	want := `cost mode "summary overflow": summary grossWin contains non-finite value`

	t.Run("report wrapper", func(t *testing.T) {
		rows, trades, prepared, err := runCostRows(cfg, route, strategy, modes, 0, nil, false, nil, false)
		if err == nil || err.Error() != want {
			t.Fatalf("report wrapper error = %v, rows=%+v, want %q", err, rows, want)
		}
		if rows != nil || trades != nil || prepared != nil {
			t.Fatalf("summary error returned partial report outputs: rows=%#v trades=%#v prepared=%#v", rows, trades, prepared)
		}
	})

	t.Run("shared grid wrapper", func(t *testing.T) {
		shared, err := engine.PrepareSharedRunContext(conformanceRunRequest(cfg, route, strategy))
		if err != nil {
			t.Fatalf("prepare shared context: %v", err)
		}
		rows, err := RunCostRowsFromShared(shared, cfg, modes)
		if err == nil || err.Error() != want {
			t.Fatalf("shared grid wrapper error = %v, rows=%+v, want %q", err, rows, want)
		}
		if rows != nil {
			t.Fatalf("summary error returned partial grid rows: %#v", rows)
		}
	})
}

// loadConformanceCase builds a Route from a run-corpus fixture and parses its
// strategy, the way the CLI does from files.
func loadConformanceCase(t *testing.T, name string) (dsl.Config, Route, string) {
	t.Helper()
	runDir := filepath.Join(testsupport.StratConformanceRoot(), "run")
	fixture, err := engine.LoadRunFixture(filepath.Join(runDir, name+".fixture.json"))
	if err != nil {
		t.Fatalf("load conformance fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runDir, name+".strat"))
	if err != nil {
		t.Fatalf("read conformance strategy: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		t.Fatalf("parse conformance strategy: %v", err)
	}
	if len(parsed.Errors) > 0 {
		t.Fatalf("conformance strategy parse errors: %v", parsed.Errors)
	}
	series := marketdata.SeriesFromBars(fixture.Bars)
	route := Route{
		Symbol:          fixture.Symbol,
		TF:              fixture.Timeframe,
		SourceTimeframe: ResolveSourceTimeframe(fixture.Timeframe, parsed.Config),
		Range:           fixture.RangeMethod,
		Series:          series,
		SourceSeries:    series,
		HigherTimeframe: fixture.HigherTimeframe,
	}
	if fixture.HigherTimeframe != "" {
		route.HTFSeries = marketdata.SeriesFromBars(fixture.HTFBars)
		route.SourceHTFSeries = route.HTFSeries
	}
	return parsed.Config, route, StrategyID(parsed.Config, name, "")
}

func conformanceRunRequest(cfg dsl.Config, route Route, strategy string) engine.RunRequest {
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
