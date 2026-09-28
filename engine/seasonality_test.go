package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSeasonalityGatesApplyClassesSideAndMinimumSamples(t *testing.T) {
	key := contextcols.SeasonalityKey{Dimension: "intraday", Lookback: "all", MinSamples: 2}
	b := broker{
		params: flagParams{SeasonalityFilters: []seasonalityFilter{{
			Dimension: "intraday", Lookback: "all", MinSamples: 2,
			Mode: "classification", Classifications: []string{"extreme_bullish", "bullish"},
		}}},
		cols: contextcols.Columns{Seasonality: map[contextcols.SeasonalityKey][]contextcols.SeasonalityEntry{
			key: {{DirectionalCount: 2, Classification: "extreme_bullish"}},
			{Dimension: "intraday", Lookback: "all", MinSamples: 3}: {{DirectionalCount: 2, Classification: "insufficient_data"}},
		}},
	}
	if !b.seasonalityGatesOK(0, sideLong) {
		t.Fatal("matching classification did not pass")
	}
	b.params.SeasonalityFilters[0].MinSamples = 3
	if b.seasonalityGatesOK(0, sideLong) {
		t.Fatal("below-minimum sample count must fail closed")
	}
	b.params.SeasonalityFilters[0].MinSamples = 2
	b.params.SeasonalityFilters[0].Classifications = []string{"bearish"}
	if b.seasonalityGatesOK(0, sideLong) {
		t.Fatal("classification outside allowlist passed")
	}
	b.params.SeasonalityFilters[0] = seasonalityFilter{Dimension: "intraday", Lookback: "all", MinSamples: 2, Mode: "supportsEntry"}
	if !b.seasonalityGatesOK(0, sideLong) {
		t.Fatal("bullish seasonality should support long entry")
	}
	if b.seasonalityGatesOK(0, sideShort) {
		t.Fatal("bullish seasonality must not support short entry")
	}
	if b.seasonalityGatesOK(1, sideLong) {
		t.Fatal("missing seasonality row must fail closed")
	}
	b.params.SeasonalityFilters = []seasonalityFilter{{
		Dimension: "intraday", Lookback: "all", MinSamples: 5,
		Mode: "classification", Classifications: []string{"insufficient_data"},
	}}
	if b.seasonalityGatesOK(0, sideLong) {
		t.Fatal("a filter with another min-samples threshold must not reuse the 2-sample series")
	}
}

func TestSeasonalityFilterRejectsEveryBrokerEntryPath(t *testing.T) {
	newBroker := func() broker {
		key := contextcols.SeasonalityKey{Dimension: "intraday", Lookback: "all", MinSamples: 2}
		return broker{
			series: marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 4, H: 10, L: 0, C: 8}}),
			cols: contextcols.Columns{
				Regime: []int8{0}, ER: []float64{0},
				Seasonality: map[contextcols.SeasonalityKey][]contextcols.SeasonalityEntry{
					key: {{DirectionalCount: 2, Classification: "bearish"}},
				},
			},
			params: flagParams{
				UseAsiaWindow: true, AdmitAsiaWindow: true, MaxMovementER: 1, RiskUSD: 1,
				SeasonalityFilters: []seasonalityFilter{{
					Dimension: "intraday", Lookback: "all", MinSamples: 2,
					Mode: "classification", Classifications: []string{"extreme_bullish", "bullish"},
				}},
			},
			costs: Costs{StartEquity: 10000},
		}
	}
	t.Run("typed immediate", func(t *testing.T) {
		b := newBroker()
		b.enterSetup(0, setupPlan{Side: sideLong, Stop: -1, Target: 12})
		if b.hasPosition {
			t.Fatal("seasonality-rejected typed entry opened a position")
		}
	})
	t.Run("flag immediate", func(t *testing.T) {
		b := newBroker()
		b.enter(0, sideLong, flagSetup{Stop: -1, Target: 12})
		if b.hasPosition {
			t.Fatal("seasonality-rejected flag entry opened a position")
		}
	})
	t.Run("limit", func(t *testing.T) {
		b := newBroker()
		b.enterLimit(0, 7, setupPlan{Side: sideLong, Stop: -1, Target: 12}, 5)
		if len(b.limitOrders) != 0 {
			t.Fatalf("seasonality-rejected entry created %d limit orders", len(b.limitOrders))
		}
	})
}

func TestSeasonalityFilterConfigMapsToRuntime(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
market conditions {
  seasonality intraday 30d in (neutral, bullish) min samples 24
}
setup { type: failed breakout }
`)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("Parse errors=%v err=%v", parsed.Errors, err)
	}
	wantFilter := seasonalityFilter{Dimension: "intraday", Lookback: "30d", LookbackDays: 30, MinSamples: 24, Mode: "classification", Classifications: []string{"neutral", "bullish"}}
	if got := seasonalityFiltersFromConfig(parsed.Config["seasonalityFilters"]); !reflect.DeepEqual(got, []seasonalityFilter{wantFilter}) {
		t.Fatalf("runtime filters = %#v, want %#v", got, wantFilter)
	}
	options := contextOptions(RunFixture{}, parsed.Config)
	if !reflect.DeepEqual(options.Seasonality, []contextcols.SeasonalitySpec{{Dimension: "intraday", Lookback: "30d", LookbackDays: 30, MinSamples: 24}}) {
		t.Fatalf("context seasonality specs = %#v", options.Seasonality)
	}
}

func TestRunFixtureCaseAppliesSeasonalityMinSampleGate(t *testing.T) {
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "seasonality-filter-min-samples.fixture.json"))
	if err != nil {
		t.Fatalf("LoadRunFixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "seasonality-filter-min-samples.strat"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	result, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("RunFixtureCase: %v", err)
	}
	if result.TradeCount != 0 || len(result.Trades) != 0 {
		t.Fatalf("high minimum sample gate produced %d trades; want none", result.TradeCount)
	}
	withoutSeasonality := strings.Replace(string(source), "  seasonality intraday in (bullish) min samples 999\n", "", 1)
	baseline, err := RunFixtureCase(fixture, withoutSeasonality)
	if err != nil {
		t.Fatalf("RunFixtureCase without seasonality: %v", err)
	}
	if baseline.TradeCount == 0 {
		t.Fatal("fixture must produce a trade without the seasonality gate so the gate is exercised")
	}
}
