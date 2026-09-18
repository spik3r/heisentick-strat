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

func TestFailedBreakoutRawLevelPriorityAcrossCheckedExecutionPaths(t *testing.T) {
	type namedStrings []string
	type namedString string
	priorityPointer := &[]string{"AL"}

	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-level-sweep")
	baselineRequest := failedBreakoutPriorityRequest(fixture, reviewedConfig)
	baseline, err := Run(baselineRequest)
	if err != nil {
		t.Fatalf("run reviewed baseline: %v", err)
	}
	assertFailedBreakoutPriorityTrade(t, baseline, "AL")
	shared, err := PrepareSharedRunContext(baselineRequest)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	tests := []struct {
		name         string
		priority     any
		wantKey      string
		wantBaseline bool
	}{
		{name: "AL string slice", priority: []string{"AL"}, wantKey: "AL", wantBaseline: true},
		{name: "mixed slice retains AL", priority: []any{1, "AL", true}, wantKey: "AL", wantBaseline: true},
		{name: "nonempty all malformed slice requires a match", priority: []any{1, true, nil}},
		{name: "unknown list requires a match", priority: []any{"unknown"}},
		{name: "nonempty scalar string requires a match", priority: "AL"},
		{name: "nonempty named slice requires a match", priority: namedStrings{"AL"}},
		{name: "nonempty array requires a match", priority: [1]string{"AL"}},
		{name: "nonempty named string requires a match", priority: namedString("AL")},
		{name: "empty string disables the filter", priority: "", wantKey: "range"},
		{name: "empty string slice disables the filter", priority: []string{}, wantKey: "range"},
		{name: "empty any slice disables the filter", priority: []any{}, wantKey: "range"},
		{name: "empty array disables the filter", priority: [0]string{}, wantKey: "range"},
		{name: "map disables the filter", priority: map[string]any{"level": "AL"}, wantKey: "range"},
		{name: "number disables the filter", priority: 1, wantKey: "range"},
		{name: "boolean disables the filter", priority: true, wantKey: "range"},
		{name: "pointer disables the filter", priority: priorityPointer, wantKey: "range"},
		{name: "nil disables the filter", priority: nil, wantKey: "range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			cfg["levelPriority"] = tt.priority
			result := runFailedBreakoutPriorityPaths(t, shared, failedBreakoutPriorityRequest(fixture, cfg), cfg)
			if tt.wantKey == "" {
				assertFailedBreakoutPriorityTrade(t, result, "")
				return
			}
			assertFailedBreakoutPriorityTrade(t, result, tt.wantKey)
			if tt.wantBaseline && !reflect.DeepEqual(result, baseline) {
				t.Fatalf("result differs from reviewed AL baseline\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(baseline))
			}
		})
	}
}

func TestFailedBreakoutAuthoredEmptyPriorityKeepsFilterDisabled(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-level-sweep")
	baselineRequest := failedBreakoutPriorityRequest(fixture, reviewedConfig)
	shared, err := PrepareSharedRunContext(baselineRequest)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "family-level-sweep.strat"))
	if err != nil {
		t.Fatalf("read reviewed source: %v", err)
	}
	mutated := strings.Replace(string(source), "priority(WH, WL, PDH, PDL, AH, AL, LH, LL)", "priority()", 1)
	if mutated == string(source) {
		t.Fatal("reviewed source did not contain expected priority list")
	}
	parsed, err := dsl.Parse(mutated)
	if err != nil {
		t.Fatalf("parse authored empty priority: %v", err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("authored empty priority diagnostics: %v", parsed.Errors)
	}
	if !boolFromAny(parsed.Config["levelPriorityExplicit"], false) {
		t.Fatal("authored empty priority lost explicit provenance")
	}
	if params := paramsFromConfig(parsed.Config); params.FBRequireLevel {
		t.Fatal("authored priority() unexpectedly enabled the Failed Breakout level filter")
	}

	result := runFailedBreakoutPriorityPaths(t, shared, failedBreakoutPriorityRequest(fixture, parsed.Config), parsed.Config)
	assertFailedBreakoutPriorityTrade(t, result, "range")
}

func TestFailedBreakoutRawLevelPriorityShapeAndConsumerIsolation(t *testing.T) {
	type namedStrings []string
	type namedString string
	priorityPointer := &[]string{"AL"}

	tests := []struct {
		name     string
		priority any
		explicit any
		want     bool
		decoded  []string
	}{
		{name: "nonempty string slice", priority: []string{"PDH", "AL"}, explicit: false, want: true, decoded: []string{"PDH", "AL"}},
		{name: "nonempty any slice", priority: []any{1, "AL", "PDH", true}, explicit: false, want: true, decoded: []string{"AL", "PDH"}},
		{name: "nonempty string", priority: "AL", explicit: false, want: true},
		{name: "nonempty named slice", priority: namedStrings{"AL"}, explicit: false, want: true},
		{name: "nonempty array", priority: [1]string{"AL"}, explicit: false, want: true},
		{name: "nonempty named string", priority: namedString("AL"), explicit: false, want: true},
		{name: "empty string slice", priority: []string{}, explicit: true, decoded: []string{}},
		{name: "empty any slice", priority: []any{}, explicit: true, decoded: []string{}},
		{name: "empty array", priority: [0]string{}, explicit: true},
		{name: "empty string", priority: "", explicit: true},
		{name: "map", priority: map[string]any{"level": "AL"}, explicit: true},
		{name: "number", priority: 1, explicit: true},
		{name: "boolean", priority: true, explicit: true},
		{name: "pointer", priority: priorityPointer, explicit: true},
		{name: "nil", priority: nil, explicit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := paramsFromConfig(dsl.Config{
				"setupType":             string(dsl.FamilyFailedBreakout),
				"levelPriority":         tt.priority,
				"levelPriorityExplicit": tt.explicit,
			})
			if params.FBRequireLevel != tt.want {
				t.Fatalf("require level = %v, want %v", params.FBRequireLevel, tt.want)
			}
			if !reflect.DeepEqual(params.LevelPriority, tt.decoded) {
				t.Fatalf("decoded priority = %v, want %v", params.LevelPriority, tt.decoded)
			}
		})
	}

	unrelated := paramsFromConfig(dsl.Config{
		"setupType":             string(dsl.FamilyFlagContinuation),
		"levelPriority":         []any{1, "AL", true},
		"levelPriorityExplicit": true,
	})
	if unrelated.FBRequireLevel {
		t.Fatal("raw level-priority shape enabled a Failed Breakout-only gate for another family")
	}
	if got, want := unrelated.LevelPriority, []string{"AL"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared decoded priority = %v, want %v", got, want)
	}
}

func runFailedBreakoutPriorityPaths(t *testing.T, shared *SharedRunContext, request RunRequest, cfg dsl.Config) RunResult {
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

func assertFailedBreakoutPriorityTrade(t *testing.T, result RunResult, wantKey string) {
	t.Helper()
	if wantKey == "" {
		if result.TradeCount != 0 || len(result.Trades) != 0 {
			t.Fatalf("trades = %d/%v, want none", result.TradeCount, tradeEntryIndexes(result.Trades))
		}
		return
	}
	if got := tradeEntryIndexes(result.Trades); !reflect.DeepEqual(got, []int{677}) {
		t.Fatalf("trade entries = %v, want [677]", got)
	}
	if result.TradeCount != 1 || len(result.Trades) != 1 {
		t.Fatalf("trade count = %d/%d, want 1/1", result.TradeCount, len(result.Trades))
	}
	wantTag := "DSL:" + wantKey + ":A"
	if trade := result.Trades[0]; trade.Tag != wantTag || trade.Meta["levelKey"] != wantKey {
		t.Fatalf("trade identity = tag %q level %v, want %s/%s", trade.Tag, trade.Meta["levelKey"], wantTag, wantKey)
	}
}

func failedBreakoutPriorityRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
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
