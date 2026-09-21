package engine

import (
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestOpeningRangeMinuteModeIgnoresInactiveCandleCountAcrossRunPaths(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-opening-range-breakout")
	series := marketdata.SeriesFromBars(fixture.Bars)
	htfSeries := marketdata.SeriesFromBars(fixture.HTFBars)
	requestFor := func(cfg dsl.Config) RunRequest {
		return RunRequest{
			Config:          cfg,
			Series:          series,
			HTFSeries:       htfSeries,
			StrategyID:      fixture.StrategyID,
			Symbol:          fixture.Symbol,
			Timeframe:       fixture.Timeframe,
			HigherTimeframe: fixture.HigherTimeframe,
			RangeMethod:     fixture.RangeMethod,
			Costs:           fixture.Costs,
		}
	}

	reviewed, err := Run(requestFor(reviewedConfig))
	if err != nil {
		t.Fatalf("run reviewed fixture: %v", err)
	}
	if reviewed.TradeCount != 21 || len(reviewed.Trades) != 21 {
		t.Fatalf("reviewed fixture trades = %d/%d, want 21/21", reviewed.TradeCount, len(reviewed.Trades))
	}

	minuteConfig := cloneTestConfig(t, reviewedConfig)
	minuteORB := mapValue(minuteConfig, "openingRangeBreakout")
	minuteORB["firstMinutes"] = float64(60)
	minuteORB["firstCandles"] = float64(0)
	poisonedConfig := cloneTestConfig(t, minuteConfig)
	mapValue(poisonedConfig, "openingRangeBreakout")["firstCandles"] = float64(-1)

	shared, err := PrepareSharedRunContext(requestFor(minuteConfig))
	if err != nil {
		t.Fatalf("prepare shared context: %v", err)
	}
	paths := []struct {
		name string
		run  func(dsl.Config) (RunResult, error)
	}{
		{name: "public Run", run: func(cfg dsl.Config) (RunResult, error) {
			return Run(requestFor(cfg))
		}},
		{name: "prepared RunChecked", run: func(cfg dsl.Config) (RunResult, error) {
			prepared, err := PrepareRun(requestFor(cfg))
			if err != nil {
				return RunResult{}, err
			}
			return prepared.RunChecked(fixture.Costs)
		}},
		{name: "shared variant RunChecked", run: func(cfg dsl.Config) (RunResult, error) {
			prepared, err := shared.PrepareVariant(cfg)
			if err != nil {
				return RunResult{}, err
			}
			return prepared.RunChecked(fixture.Costs)
		}},
	}

	for _, path := range paths {
		for _, config := range []struct {
			name string
			cfg  dsl.Config
		}{
			{name: "zero inactive candles", cfg: minuteConfig},
			{name: "negative inactive candles", cfg: poisonedConfig},
		} {
			t.Run(path.name+"/"+config.name, func(t *testing.T) {
				result, err := path.run(config.cfg)
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				if !reflect.DeepEqual(result, reviewed) {
					t.Fatalf("minute-mode result differs from reviewed four-candle fixture\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(reviewed))
				}
			})
		}
	}
}

func TestOpeningRangeCandleModeRejectsUnsafeCountsBeforeAllocation(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "family-opening-range-breakout")
	runner := newPreparedRunner(fixture, cfg)
	b := broker{series: runner.series, params: runner.params}

	index := -1
	var reviewed openingRange
	for i := 0; i < b.series.Len(); i++ {
		if candidate, ok := b.openingRangeFor(i); ok {
			index, reviewed = i, candidate
			break
		}
	}
	if index < 0 {
		t.Fatal("reviewed fixture produced no opening range")
	}

	for _, count := range []int{-1, 0, math.MaxInt} {
		t.Run(stringCount(count), func(t *testing.T) {
			probe := b
			probe.params.ORBFirstMinutes = 0
			probe.params.ORBFirstCandles = count
			if got, ok := probe.openingRangeFor(index); ok || got != (openingRange{}) {
				t.Fatalf("openingRangeFor count %d = (%+v, %v), want zero/false", count, got, ok)
			}
		})
	}

	valid := b
	valid.params.ORBFirstMinutes = 0
	valid.params.ORBFirstCandles = 4
	if got, ok := valid.openingRangeFor(index); !ok || got != reviewed {
		t.Fatalf("valid four-candle range = (%+v, %v), want (%+v, true)", got, ok, reviewed)
	}
}

func stringCount(count int) string {
	switch count {
	case -1:
		return "negative"
	case 0:
		return "zero"
	default:
		return "maximum integer"
	}
}
