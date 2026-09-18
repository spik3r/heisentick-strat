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

func TestSelectiveFlagRunHonorsPriorDayTypeFiltersWithoutReportContext(t *testing.T) {
	const caseName = "deployed-dsl-flag-continuation-one-four-hour-review"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	sourceBytes, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	source := string(sourceBytes)
	allowSource := addFlagPriorDayTypeFilter(t, source, "prior day type in (trend)")
	blockSource := addFlagPriorDayTypeFilter(t, source, "prior day type not in (trend)")

	baseline, baselineKey, baselineRequest := runSelectiveFlagPriorDayTypeCase(t, fixture, source)
	allowed, allowedKey, _ := runSelectiveFlagPriorDayTypeCase(t, fixture, allowSource)
	blocked, blockedKey, _ := runSelectiveFlagPriorDayTypeCase(t, fixture, blockSource)

	if baseline.TradeCount != 17 || len(baseline.Trades) != 17 {
		t.Fatalf("baseline trades = %d/%d, want reviewed count 17", baseline.TradeCount, len(baseline.Trades))
	}
	if allowed.TradeCount != 3 || len(allowed.Trades) != 3 {
		t.Fatalf("trend allowlist trades = %d/%d, want 3", allowed.TradeCount, len(allowed.Trades))
	}
	if blocked.TradeCount != 14 || len(blocked.Trades) != 14 {
		t.Fatalf("trend blocklist trades = %d/%d, want 14", blocked.TradeCount, len(blocked.Trades))
	}
	if allowedKey != baselineKey || blockedKey != baselineKey {
		t.Fatalf("prior-day filters changed shared context key\n baseline: %s\n allow: %s\n block: %s", baselineKey, allowedKey, blockedKey)
	}

	prepared, err := PrepareRun(baselineRequest)
	if err != nil {
		t.Fatalf("prepare baseline run: %v", err)
	}
	if baselineRequest.ReportTradeContext || len(prepared.cols.PriorDayType) != prepared.series.Len() || len(prepared.cols.OpenLocation) != 0 {
		t.Fatalf("selective Flag context = report:%v priorDayType:%d openLocation:%d bars:%d; want execution type only",
			baselineRequest.ReportTradeContext, len(prepared.cols.PriorDayType), len(prepared.cols.OpenLocation), prepared.series.Len())
	}
	if _, ok := prepared.ReportTradeContext(baseline.Trades[0].EntryIndex); ok {
		t.Fatal("execution-owned prior-day type unexpectedly enabled report trade context")
	}
}

func addFlagPriorDayTypeFilter(t *testing.T, source string, directive string) string {
	t.Helper()
	mutated := strings.Replace(source, "sessions(asia, london, ny)", "sessions(asia, london, ny)\n  "+directive, 1)
	if mutated == source {
		t.Fatal("fixture source did not contain the expected sessions directive")
	}
	return mutated
}

func runSelectiveFlagPriorDayTypeCase(t *testing.T, fixture RunFixture, source string) (RunResult, string, RunRequest) {
	t.Helper()
	parsed, err := dsl.Parse(source)
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) > 0 {
		t.Fatalf("DSL parse errors: %v", parsed.Errors)
	}
	request := RunRequest{
		Config:             parsed.Config,
		Series:             marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:          marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:         fixture.StrategyID,
		Symbol:             fixture.Symbol,
		Timeframe:          fixture.Timeframe,
		HigherTimeframe:    fixture.HigherTimeframe,
		RangeMethod:        fixture.RangeMethod,
		ReportTradeContext: false,
		Costs:              fixture.Costs,
	}
	key, err := SharedContextKey(request)
	if err != nil {
		t.Fatalf("shared context key: %v", err)
	}
	result, err := Run(request)
	if err != nil {
		t.Fatalf("run fixture: %v", err)
	}
	return result, key, request
}

func TestRunFixtureCaseHonorsPriorDayTypeAllowlist(t *testing.T) {
	const caseName = "family-volume-anomaly-exhaustion"
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
	if baseline.TradeCount != 3 {
		t.Fatalf("baseline trade count = %d, want reviewed fixture count 3", baseline.TradeCount)
	}

	for _, tt := range []struct {
		name       string
		allowed    string
		wantTrades int
		wantSame   bool
	}{
		{name: "range rejects trend entries", allowed: "range", wantTrades: 0},
		{name: "trend keeps only completed trend days", allowed: "trend", wantTrades: 2, wantSame: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mutated := strings.Replace(string(source),
				"sessions(london, ny)",
				"sessions(london, ny)\n  prior day type in ("+tt.allowed+")",
				1,
			)
			if mutated == string(source) {
				t.Fatal("fixture source did not contain the expected sessions directive")
			}
			result, err := RunFixtureCase(fixture, mutated)
			if err != nil {
				t.Fatalf("run fixture with prior-day allowlist: %v", err)
			}
			if result.TradeCount != tt.wantTrades || len(result.Trades) != tt.wantTrades {
				t.Fatalf("%s-only allowlist produced %d trades: %+v; want %d", tt.allowed, result.TradeCount, result.Trades, tt.wantTrades)
			}
			if tt.wantSame && !reflect.DeepEqual(result.Trades, baseline.Trades) {
				t.Fatalf("matching allowlist changed baseline trades\n got: %+v\nwant: %+v", result.Trades, baseline.Trades)
			}
		})
	}
}

func TestMarketNonSessionGateHonorsPriorDayTypeAllowlistAndBlocklist(t *testing.T) {
	b := broker{
		params: flagParams{PriorDayTypes: []string{"trend"}},
		cols: contextcols.Columns{
			PriorDayType: []int8{2, 1},
			Regime:       []int8{0, 0},
			ER:           []float64{0, 0},
		},
	}

	if !b.marketNonSessionGatesOK(0) {
		t.Fatal("trend prior day should pass the trend-only allowlist")
	}
	if b.marketNonSessionGatesOK(1) {
		t.Fatal("range prior day should fail the trend-only allowlist")
	}
	b.params.BlockedPriorDayTypes = []string{"trend"}
	if b.marketNonSessionGatesOK(0) {
		t.Fatal("existing blocklist should reject a type that passed the allowlist")
	}
	b.params.BlockedPriorDayTypes = []string{"range"}
	if !b.marketNonSessionGatesOK(0) {
		t.Fatal("nonmatching blocklist should preserve an allowed prior-day type")
	}

	b.cols.PriorDayType = nil
	if b.marketNonSessionGatesOK(0) {
		t.Fatal("missing prior-day context should fail a nonempty allowlist without panicking")
	}
}
