package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestRunFixtureCaseHonorsTypedCandleQuality(t *testing.T) {
	const caseName = "family-break-retest"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	baseline, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	if baseline.TradeCount != 13 || len(baseline.Trades) != 13 {
		t.Fatalf("baseline trades = %d/%d, want reviewed fixture count 13", baseline.TradeCount, len(baseline.Trades))
	}

	for _, directive := range []string{
		"close location at least 1",
		"tail rejection at least 1",
	} {
		t.Run(directive, func(t *testing.T) {
			mutated := strings.Replace(string(source), "setup {", "setup {\n  "+directive, 1)
			if mutated == string(source) {
				t.Fatal("fixture source did not contain the expected setup block")
			}
			result, err := RunFixtureCase(fixture, mutated)
			if err != nil {
				t.Fatalf("run fixture with %q: %v", directive, err)
			}
			if result.TradeCount != 0 || len(result.Trades) != 0 {
				t.Fatalf("%q produced %d trades: %+v; want none", directive, result.TradeCount, result.Trades)
			}
		})
	}

	_, cfg := loadEntryAttemptCase(t, caseName)
	cfg["closeLocationMin"] = 1.0
	runner := newPreparedRunner(fixture, cfg)
	if trades := runner.RunPrepared(); len(trades) != 0 {
		t.Fatalf("quality-rejected prepared run produced %d trades, want 0", len(trades))
	}
	if !runner.broker.hasBREntry || runner.broker.brLastEntry <= 0 {
		t.Fatalf("final quality rejection did not preserve typed candidate consumption: cooldown=%v last=%d",
			runner.broker.hasBREntry, runner.broker.brLastEntry)
	}
}

func TestCandleQualityBoundaryMatchesJSForLongAndShort(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{
		{O: 4, H: 10, L: 0, C: 8},
		{O: 6, H: 10, L: 0, C: 2},
	})
	for _, tt := range []struct {
		name  string
		index int
		side  side
	}{
		{name: "long", index: 0, side: sideLong},
		{name: "short", index: 1, side: sideShort},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !candleQualityOK(series, tt.index, tt.side, 0.4, 0.8) {
				t.Fatal("exact tail and close thresholds should pass")
			}
			if candleQualityOK(series, tt.index, tt.side, 0.4001, 0.8) {
				t.Fatal("tail below the authored threshold should fail")
			}
			if candleQualityOK(series, tt.index, tt.side, 0.4, 0.8001) {
				t.Fatal("close location below the authored threshold should fail")
			}
		})
	}
}

func TestTypedCandleQualityFinalAdmissionPaths(t *testing.T) {
	newBroker := func() broker {
		return broker{
			series: marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 4, H: 10, L: 0, C: 8}}),
			cols: contextcols.Columns{
				Regime: []int8{0},
				ER:     []float64{0},
			},
			params: flagParams{
				UseAsiaWindow:    true,
				AdmitAsiaWindow:  true,
				MaxMovementER:    1,
				CloseLocationMin: 0.8,
				TailRejectionMin: 0.4,
				RiskUSD:          1,
			},
			costs: Costs{StartEquity: 10000},
		}
	}

	t.Run("typed immediate", func(t *testing.T) {
		b := newBroker()
		b.enterSetup(0, setupPlan{Side: sideLong, Stop: -1, Target: 12})
		if !b.hasPosition {
			t.Fatal("quality-matching typed setup did not enter")
		}
	})

	t.Run("flag immediate", func(t *testing.T) {
		b := newBroker()
		b.enter(0, sideLong, flagSetup{Stop: -1, Target: 12})
		if !b.hasPosition {
			t.Fatal("quality-matching flag setup did not enter")
		}
		b = newBroker()
		b.params.TailRejectionMin = 0.41
		b.enter(0, sideLong, flagSetup{Stop: -1, Target: 12})
		if b.hasPosition {
			t.Fatal("quality-rejected flag setup entered")
		}
	})

	t.Run("limit", func(t *testing.T) {
		b := newBroker()
		b.enterLimit(0, 7, setupPlan{Side: sideLong, Stop: -1, Target: 12}, 5)
		if len(b.limitOrders) != 1 {
			t.Fatalf("quality-matching limit orders = %d, want 1", len(b.limitOrders))
		}
		b = newBroker()
		b.params.CloseLocationMin = 0.81
		b.enterLimit(0, 7, setupPlan{Side: sideLong, Stop: -1, Target: 12}, 5)
		if len(b.limitOrders) != 0 {
			t.Fatalf("quality-rejected limit orders = %d, want 0", len(b.limitOrders))
		}
	})
}
