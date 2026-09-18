package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func loadRangeBreakFakeRouteCase(t *testing.T) (RunFixture, string, dsl.Config) {
	t.Helper()
	const caseName = "family-range-break-fake"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	return fixture, string(source), parsed.Config
}

func TestExecutionPathsInferBlankRouteTimeframe(t *testing.T) {
	fixture, source, cfg := loadRangeBreakFakeRouteCase(t)
	fixture.Timeframe = ""

	fixtureResult, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatalf("run fixture: %v", err)
	}
	if fixtureResult.TradeCount == 0 {
		t.Fatal("blank fixture timeframe should infer the 15m route and remain on-route")
	}

	directResult, err := Run(RunRequest{
		Config:          cfg,
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       "",
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	})
	if err != nil {
		t.Fatalf("run engine: %v", err)
	}
	if directResult.TradeCount != fixtureResult.TradeCount {
		t.Fatalf("blank direct timeframe trade count = %d, fixture path = %d", directResult.TradeCount, fixtureResult.TradeCount)
	}
}

func TestPreparedRunnerEnforcesRouteGate(t *testing.T) {
	fixture, _, cfg := loadRangeBreakFakeRouteCase(t)
	fixture.Timeframe = "1h"

	runner := newPreparedRunner(fixture, cfg)
	if trades := runner.RunPrepared(); len(trades) != 0 {
		t.Fatalf("off-route prepared runner produced %d trades, want 0", len(trades))
	}
}
