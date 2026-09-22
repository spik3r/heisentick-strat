package engine

import (
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestBreakRetestLevelPriorityFallbackAcrossCheckedExecutionPaths(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-break-retest")
	baselineRequest := breakRetestPriorityRequest(fixture, reviewedConfig)
	baseline, err := Run(baselineRequest)
	if err != nil {
		t.Fatalf("run reviewed baseline: %v", err)
	}
	baselineEntries := []int{315, 389, 590, 756, 933, 1074, 1093, 1412, 1442, 1580, 1649, 1695, 1987, 2475}
	baselineKeys := []string{"PDL", "PDH", "PDL", "PDL", "PDL", "PDL", "PDL", "PDL", "PDH", "PDH", "PDH", "PDH", "PDH", "PDL"}
	assertBreakRetestPriorityTrades(t, baseline, baselineEntries, baselineKeys)
	shared, err := PrepareSharedRunContext(baselineRequest)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	tests := []struct {
		name         string
		reviewed     bool
		present      bool
		priority     any
		setExplicit  bool
		explicit     any
		wantEntries  []int
		wantKeys     []string
		wantBaseline bool
	}{
		{name: "reviewed PDH PDL", reviewed: true, wantEntries: baselineEntries, wantKeys: baselineKeys, wantBaseline: true},
		{name: "explicit PDL keeps long fallback", present: true, priority: []any{"PDL"}, setExplicit: true, explicit: true, wantEntries: baselineEntries, wantKeys: baselineKeys, wantBaseline: true},
		{name: "explicit PDH keeps short fallback", present: true, priority: []any{"PDH"}, setExplicit: true, explicit: true, wantEntries: baselineEntries, wantKeys: baselineKeys, wantBaseline: true},
		{
			name:        "explicit WH keeps short fallback",
			present:     true,
			priority:    []any{"WH"},
			setExplicit: true,
			explicit:    true,
			wantEntries: []int{315, 430, 590, 756, 933, 1074, 1093, 1412, 1580, 1642, 2475},
			wantKeys:    []string{"PDL", "WH", "PDL", "PDL", "PDL", "PDL", "PDL", "PDL", "WH", "WH", "PDL"},
		},
		{name: "explicit empty falls back", present: true, priority: []any{}, setExplicit: true, explicit: true, wantEntries: baselineEntries, wantKeys: baselineKeys, wantBaseline: true},
		{name: "explicit malformed scalar falls back", present: true, priority: "PDH", setExplicit: true, explicit: true, wantEntries: baselineEntries, wantKeys: baselineKeys, wantBaseline: true},
		{name: "explicit malformed elements fall back", present: true, priority: []any{1, true, nil}, setExplicit: true, explicit: true, wantEntries: baselineEntries, wantKeys: baselineKeys, wantBaseline: true},
		{name: "nonexplicit authored list uses defaults", present: true, priority: []any{"WH", "WL"}, setExplicit: true, explicit: false, wantEntries: baselineEntries, wantKeys: baselineKeys, wantBaseline: true},
		{
			name:        "explicit order prefers week levels first",
			present:     true,
			priority:    []any{"WH", "PDH", "WL", "PDL"},
			setExplicit: true,
			explicit:    true,
			wantEntries: []int{250, 275, 315, 389, 590, 756, 933, 1074, 1093, 1297, 1412, 1442, 1580, 1642, 1695, 1987, 2034, 2475},
			wantKeys:    []string{"WL", "WL", "PDL", "PDH", "PDL", "PDL", "PDL", "WL", "PDL", "WL", "PDL", "PDH", "WH", "WH", "PDH", "PDH", "WL", "WL"},
		},
		{
			name:        "explicit order prefers prior day first",
			present:     true,
			priority:    []any{"PDH", "WH", "PDL", "WL"},
			setExplicit: true,
			explicit:    true,
			wantEntries: []int{250, 275, 315, 389, 590, 756, 933, 1074, 1093, 1297, 1412, 1442, 1580, 1642, 1695, 1987, 2034, 2475},
			wantKeys:    []string{"WL", "WL", "PDL", "PDH", "PDL", "PDL", "PDL", "PDL", "PDL", "WL", "PDL", "PDH", "PDH", "WH", "PDH", "PDH", "WL", "PDL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			if !tt.reviewed {
				if tt.present {
					cfg["levelPriority"] = tt.priority
				} else {
					delete(cfg, "levelPriority")
				}
				if tt.setExplicit {
					cfg["levelPriorityExplicit"] = tt.explicit
				}
			}

			result := runBreakRetestPriorityPaths(t, shared, breakRetestPriorityRequest(fixture, cfg), cfg)
			assertBreakRetestPriorityTrades(t, result, tt.wantEntries, tt.wantKeys)
			if tt.wantBaseline && !reflect.DeepEqual(result, baseline) {
				t.Fatalf("result differs from reviewed baseline\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(baseline))
			}
		})
	}
}

func TestBreakRetestLevelPriorityMissingSideDefaultReturnsNoLevels(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{0},
			H: []float64{1},
			L: []float64{1},
		},
		cols: contextcols.Columns{
			PriorDayH: []float64{math.NaN()},
			PriorDayL: []float64{math.NaN()},
		},
		params: flagParams{
			LevelPriority:         []string{"WH", "WL"},
			LevelPriorityExplicit: true,
		},
	}
	if got := b.breakRetestLevels(0, sideLong); len(got) != 0 {
		t.Fatalf("long levels = %v, want missing PDH fallback to remain empty", got)
	}
	if got := b.breakRetestLevels(0, sideShort); len(got) != 0 {
		t.Fatalf("short levels = %v, want missing PDL fallback to remain empty", got)
	}
}

func runBreakRetestPriorityPaths(t *testing.T, shared *SharedRunContext, request RunRequest, cfg dsl.Config) RunResult {
	t.Helper()
	publicResult, err := Run(request)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	prepared, err := PrepareRun(request)
	if err != nil {
		t.Fatalf("PrepareRun: %v", err)
	}
	preparedResult, err := prepared.RunChecked(request.Costs)
	if err != nil {
		t.Fatalf("prepared RunChecked: %v", err)
	}
	variant, err := shared.PrepareVariant(cfg)
	if err != nil {
		t.Fatalf("PrepareVariant: %v", err)
	}
	sharedResult, err := variant.RunChecked(request.Costs)
	if err != nil {
		t.Fatalf("shared RunChecked: %v", err)
	}
	if !reflect.DeepEqual(preparedResult, publicResult) || !reflect.DeepEqual(sharedResult, publicResult) {
		t.Fatalf("checked paths differ\npublic: %s\nprepared: %s\nshared: %s", canonicalJSON(publicResult), canonicalJSON(preparedResult), canonicalJSON(sharedResult))
	}
	return publicResult
}

func assertBreakRetestPriorityTrades(t *testing.T, result RunResult, wantEntries []int, wantKeys []string) {
	t.Helper()
	if got := tradeEntryIndexes(result.Trades); !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("trade entries = %v, want %v", got, wantEntries)
	}
	if result.TradeCount != len(wantEntries) || len(result.Trades) != len(wantEntries) {
		t.Fatalf("trade count = %d/%d, want %d/%d", result.TradeCount, len(result.Trades), len(wantEntries), len(wantEntries))
	}
	gotKeys := make([]string, len(result.Trades))
	for i, trade := range result.Trades {
		if trade.Tag != "DSL-BR" || trade.Meta["setup"] != "breakRetest" {
			t.Fatalf("trade %d identity = tag %q setup %v, want DSL-BR/breakRetest", i, trade.Tag, trade.Meta["setup"])
		}
		gotKeys[i], _ = trade.Meta["levelKey"].(string)
	}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("trade level keys = %v, want %v", gotKeys, wantKeys)
	}
}

func breakRetestPriorityRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
	return RunRequest{
		Config:          cfg,
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	}
}
