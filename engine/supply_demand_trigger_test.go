package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSupplyDemandTriggerProvenanceMapping(t *testing.T) {
	tests := []struct {
		name string
		cfg  dsl.Config
		want bool
	}{
		{
			name: "omitted",
			cfg:  dsl.Config{"setupType": string(dsl.FamilySupplyDemand)},
			want: false,
		},
		{
			name: "explicit any",
			cfg: dsl.Config{
				"setupType":       string(dsl.FamilySupplyDemand),
				"triggerExplicit": true,
				"triggerCandles":  []any{"any"},
			},
			want: false,
		},
		{
			name: "explicit family trigger switch",
			cfg: dsl.Config{
				"setupType":       string(dsl.FamilySupplyDemand),
				"triggerExplicit": true,
				"triggerCandles":  []any{"pin"},
			},
			want: true,
		},
		{
			name: "list containing any disables switch",
			cfg: dsl.Config{
				"setupType":       string(dsl.FamilySupplyDemand),
				"triggerExplicit": true,
				"triggerCandles":  []any{"pin", "any"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paramsFromConfig(tt.cfg).SupplyDemand.UseTrigger; got != tt.want {
				t.Fatalf("Supply/Demand use-trigger = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSupplyDemandAuthoredTriggerMatchesReviewedFixture(t *testing.T) {
	fixture, source := loadSupplyDemandTriggerCase(t)
	baseline, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	wantBaselineIndexes := []int{873, 1804, 2884}
	if got := tradeEntryIndexes(baseline.Trades); baseline.TradeCount != 3 || !reflect.DeepEqual(got, wantBaselineIndexes) {
		t.Fatalf("baseline trades = %d at %v, want 3 at %v", baseline.TradeCount, got, wantBaselineIndexes)
	}

	mutated := strings.Replace(source, "candle in (any)", "candle in (pin)", 1)
	if mutated == source {
		t.Fatal("fixture source did not contain the expected any-candle trigger")
	}
	parsed, err := dsl.Parse(mutated)
	if err != nil {
		t.Fatalf("parse mutated fixture: %v", err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("mutated fixture parse errors: %v", parsed.Errors)
	}
	if got := stringSliceValue(parsed.Config["triggerCandles"]); !reflect.DeepEqual(got, []string{"pin"}) {
		t.Fatalf("mutated trigger candles = %v, want [pin]", got)
	}
	if !boolFromAny(parsed.Config["triggerExplicit"], false) {
		t.Fatal("mutated trigger did not preserve explicit provenance")
	}

	result, err := RunFixtureCase(fixture, mutated)
	if err != nil {
		t.Fatalf("run fixture with authored trigger: %v", err)
	}
	if got, want := tradeEntryIndexes(result.Trades), []int{1804}; result.TradeCount != 1 || !reflect.DeepEqual(got, want) {
		t.Fatalf("authored trigger trades = %d at %v, want 1 at %v", result.TradeCount, got, want)
	}
}

func TestSupplyDemandInt64TriggerProvenanceMatchesParsedBool(t *testing.T) {
	fixture, source := loadSupplyDemandTriggerCase(t)
	mutated := strings.Replace(source, "candle in (any)", "candle in (pin)", 1)
	if mutated == source {
		t.Fatal("fixture source did not contain the expected any-candle trigger")
	}
	parsed, err := dsl.Parse(mutated)
	if err != nil {
		t.Fatalf("parse mutated fixture: %v", err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("mutated fixture parse errors: %v", parsed.Errors)
	}
	if triggerExplicit, ok := parsed.Config["triggerExplicit"].(bool); !ok || !triggerExplicit {
		t.Fatalf("parsed triggerExplicit = %T(%v), want bool(true)", parsed.Config["triggerExplicit"], parsed.Config["triggerExplicit"])
	}

	directConfig := make(dsl.Config, len(parsed.Config))
	for key, value := range parsed.Config {
		directConfig[key] = value
	}
	directConfig["triggerExplicit"] = int64(1)
	run := func(t *testing.T, cfg dsl.Config) RunResult {
		t.Helper()
		result, err := Run(RunRequest{
			Config:          cfg,
			Series:          marketdata.SeriesFromBars(fixture.Bars),
			HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
			StrategyID:      fixture.StrategyID,
			Symbol:          fixture.Symbol,
			Timeframe:       fixture.Timeframe,
			HigherTimeframe: fixture.HigherTimeframe,
			RangeMethod:     fixture.RangeMethod,
			Costs:           fixture.Costs,
		})
		if err != nil {
			t.Fatalf("run engine: %v", err)
		}
		return result
	}

	parsedResult := run(t, parsed.Config)
	directResult := run(t, directConfig)
	if got, want := tradeEntryIndexes(parsedResult.Trades), []int{1804}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed bool entries = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(directResult, parsedResult) {
		t.Fatalf("int64 trigger result differs from parsed bool result\n got: %s\nwant: %s", canonicalJSON(directResult), canonicalJSON(parsedResult))
	}
}

func TestSupplyDemandTriggerRejectionPreservesRetestStateBeforeAdmission(t *testing.T) {
	fixture, source := loadSupplyDemandTriggerCase(t)
	mutated := strings.Replace(source, "candle in (any)", "candle in (pin)", 1)
	parsed, err := dsl.Parse(mutated)
	if err != nil {
		t.Fatalf("parse mutated fixture: %v", err)
	}
	runner := newPreparedRunner(fixture, parsed.Config)
	runner.broker.reset(
		runner.series,
		runner.cols,
		runner.htfTrend,
		runner.ema,
		runner.emaSlope,
		runner.params,
		runner.fixture,
		nil,
	)

	const rejectedEntryIndex = 873
	for i := 0; i <= rejectedEntryIndex; i++ {
		runner.broker.onBar(i)
	}
	if longTrigger(runner.series, rejectedEntryIndex) || shortTrigger(runner.series, rejectedEntryIndex) {
		t.Fatal("reviewed rejected bar unexpectedly satisfies the built-in trigger set")
	}
	if runner.broker.hasPosition || runner.broker.hasFlagEntry {
		t.Fatalf("trigger rejection mutated admission state: position=%v cooldown=%v", runner.broker.hasPosition, runner.broker.hasFlagEntry)
	}
	touched := 0
	for _, zone := range runner.broker.sdZones {
		if zone.HasLastTouch && zone.LastTouchAt == rejectedEntryIndex {
			touched++
			if zone.Used {
				t.Fatal("trigger-rejected zone was marked used")
			}
		}
	}
	if touched == 0 {
		t.Fatal("trigger rejection did not preserve the preceding retest touch state")
	}
}

func loadSupplyDemandTriggerCase(t *testing.T) (RunFixture, string) {
	t.Helper()
	const caseName = "family-supply-demand"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	return fixture, string(source)
}

func tradeEntryIndexes(trades []Trade) []int {
	indexes := make([]int, len(trades))
	for i, trade := range trades {
		indexes[i] = trade.EntryIndex
	}
	return indexes
}
