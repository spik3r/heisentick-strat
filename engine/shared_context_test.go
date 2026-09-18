package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSharedContextGridVariantsOnlyDifferByRiskSizing(t *testing.T) {
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
	if len(parsed.Errors) > 0 {
		t.Fatalf("DSL parse errors: %v", parsed.Errors)
	}

	lowRisk := cloneTestConfig(t, parsed.Config)
	lowRisk["riskUsd"] = float64(100)
	highRisk := cloneTestConfig(t, parsed.Config)
	highRisk["riskUsd"] = float64(200)

	series := marketdata.SeriesFromBars(fixture.Bars)
	shared, err := PrepareSharedRunContext(RunRequest{
		Config:          lowRisk,
		Series:          series,
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	})
	if err != nil {
		t.Fatalf("prepare shared context: %v", err)
	}

	lowResult := runSharedTestVariant(t, shared, lowRisk, fixture.Costs)
	highResult := runSharedTestVariant(t, shared, highRisk, fixture.Costs)
	if lowResult.TradeCount == 0 {
		t.Fatalf("fixture produced no trades")
	}
	if lowResult.TradeCount != highResult.TradeCount {
		t.Fatalf("trade count changed: low=%d high=%d", lowResult.TradeCount, highResult.TradeCount)
	}
	for i := range lowResult.Trades {
		lowTrade := stableTradeForRiskGrid(lowResult.Trades[i])
		highTrade := stableTradeForRiskGrid(highResult.Trades[i])
		if got, want := canonicalJSON(highTrade), canonicalJSON(lowTrade); got != want {
			t.Fatalf("trade %d changed outside risk sizing\n got: %s\nwant: %s", i, got, want)
		}
		assertClose(t, highResult.Trades[i].Size, lowResult.Trades[i].Size*2, "size")
		assertClose(t, highResult.Trades[i].PnL, lowResult.Trades[i].PnL*2, "pnl")
	}
}

func runSharedTestVariant(t *testing.T, shared *SharedRunContext, cfg dsl.Config, costs Costs) RunResult {
	t.Helper()
	prepared, err := shared.PrepareVariant(cfg)
	if err != nil {
		t.Fatalf("prepare variant: %v", err)
	}
	return prepared.Run(costs)
}

func stableTradeForRiskGrid(trade Trade) Trade {
	trade.PnL = 0
	trade.Size = 0
	return trade
}

func cloneTestConfig(t *testing.T, cfg dsl.Config) dsl.Config {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var out dsl.Config
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return out
}

func assertClose(t *testing.T, got float64, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %.12f, want %.12f", label, got, want)
	}
}
