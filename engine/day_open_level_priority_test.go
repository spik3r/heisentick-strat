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

func TestDayOpenLevelPriorityPresenceAcrossCheckedExecutionPaths(t *testing.T) {
	type namedString string
	type namedStrings []string

	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-day-open-reclaim")
	baselineRequest := dayOpenPriorityRequest(fixture, reviewedConfig)
	baseline, err := Run(baselineRequest)
	if err != nil {
		t.Fatalf("run reviewed baseline: %v", err)
	}
	assertDayOpenPriorityTrades(t, baseline, []int{300, 483, 2483})
	shared, err := PrepareSharedRunContext(baselineRequest)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	var nilStrings []string
	var nilAny []any
	tests := []struct {
		name         string
		present      bool
		priority     any
		setExplicit  bool
		explicit     any
		want         []int
		wantBaseline bool
	}{
		{name: "missing defaults", want: []int{300, 483, 2483}, wantBaseline: true},
		{name: "full list", present: true, priority: []any{"PDH", "PDL", "WH", "WL"}, want: []int{300, 483, 2483}, wantBaseline: true},
		{name: "PDH only", present: true, priority: []any{"PDH"}, want: []int{300, 483, 2483}, wantBaseline: true},
		{name: "mixed retains exact PDH", present: true, priority: []any{1, "PDH", namedString("PDL"), true}, want: []int{300, 483, 2483}, wantBaseline: true},
		{name: "explicit empty", present: true, priority: []any{}, want: []int{}},
		{name: "nonexplicit empty", present: true, priority: []any{}, setExplicit: true, explicit: false, want: []int{}},
		{name: "nil", present: true, priority: nil, want: []int{}},
		{name: "typed nil string slice", present: true, priority: nilStrings, want: []int{}},
		{name: "typed nil any slice", present: true, priority: nilAny, want: []int{}},
		{name: "all malformed", present: true, priority: []any{1, namedString("PDH"), true, nil}, want: []int{}},
		{name: "scalar", present: true, priority: "PDH", want: []int{}},
		{name: "map", present: true, priority: map[string]any{"level": "PDH"}, want: []int{}},
		{name: "defined slice", present: true, priority: namedStrings{"PDH"}, want: []int{}},
		{name: "unknown only", present: true, priority: []any{"unknown"}, want: []int{}},
		{name: "PDL only", present: true, priority: []any{"PDL"}, want: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			if tt.present {
				cfg["levelPriority"] = tt.priority
			} else {
				delete(cfg, "levelPriority")
			}
			if tt.setExplicit {
				cfg["levelPriorityExplicit"] = tt.explicit
			}

			result := runDayOpenPriorityPaths(t, shared, dayOpenPriorityRequest(fixture, cfg), cfg)
			assertDayOpenPriorityTrades(t, result, tt.want)
			if tt.wantBaseline && !reflect.DeepEqual(result, baseline) {
				t.Fatalf("result differs from reviewed baseline\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(baseline))
			}
		})
	}
}

func TestDayOpenAuthoredEmptyPriorityFailsClosed(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-day-open-reclaim")
	shared, err := PrepareSharedRunContext(dayOpenPriorityRequest(fixture, reviewedConfig))
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "family-day-open-reclaim.strat"))
	if err != nil {
		t.Fatalf("read reviewed source: %v", err)
	}
	mutated := strings.Replace(string(source), "priority(PDH, PDL, WH, WL)", "priority()", 1)
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
	if got := stringSliceValue(parsed.Config["levelPriority"]); len(got) != 0 {
		t.Fatalf("authored priority = %v, want empty", got)
	}

	result := runDayOpenPriorityPaths(t, shared, dayOpenPriorityRequest(fixture, parsed.Config), parsed.Config)
	assertDayOpenPriorityTrades(t, result, []int{})
}

func TestDayOpenLevelPriorityOrderOwnershipAndConsumerIsolation(t *testing.T) {
	type namedString string

	source := []string{"PDH", "WH"}
	cfg := dsl.Config{
		"setupType":             string(dsl.FamilyDayOpenReclaim),
		"levelPriority":         source,
		"levelPriorityExplicit": false,
	}
	params := paramsFromConfig(cfg)
	if got, want := params.DORLevelPriority, []string{"PDH", "WH"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("day-open priority = %v, want %v", got, want)
	}
	if got, want := params.LevelPriority, []string{"PDH", "WH"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared priority = %v, want %v", got, want)
	}
	if got, want := params.VolumeAnomalyExhaustion.LevelPriority, []string{"CAM_R4", "CAM_R3", "CAM_S3", "CAM_S4", "PDH", "PDL", "VWAP"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("nonexplicit VAE priority = %v, want %v", got, want)
	}
	if params.LevelPriorityExplicit {
		t.Fatal("shared explicitness changed")
	}
	if params.SBHAllowedLevels != nil {
		t.Fatalf("session-break-hold levels = %v, want nil", params.SBHAllowedLevels)
	}
	mixed := paramsFromConfig(dsl.Config{
		"setupType":     string(dsl.FamilyDayOpenReclaim),
		"levelPriority": []any{1, "WH", namedString("PDH"), "PDL", true},
	})
	if got, want := mixed.DORLevelPriority, []string{"WH", "PDL"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed day-open priority = %v, want %v", got, want)
	}

	params.DORLevelPriority[0] = "changed"
	if source[0] != "PDH" || params.LevelPriority[0] != "PDH" {
		t.Fatalf("day-open mutation leaked: source=%v shared=%v", source, params.LevelPriority)
	}
	source[1] = "source changed"
	if params.DORLevelPriority[1] != "WH" || params.LevelPriority[1] != "WH" {
		t.Fatalf("source mutation leaked: day-open=%v shared=%v", params.DORLevelPriority, params.LevelPriority)
	}

	missing := paramsFromConfig(dsl.Config{"setupType": string(dsl.FamilyDayOpenReclaim)})
	if got, want := missing.DORLevelPriority, []string{"PDH", "PDL", "WH", "WL"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing day-open priority = %v, want %v", got, want)
	}
	if missing.LevelPriority != nil || missing.SBHAllowedLevels != nil {
		t.Fatalf("missing key broadened other consumers: shared=%v session-break-hold=%v", missing.LevelPriority, missing.SBHAllowedLevels)
	}
}

func runDayOpenPriorityPaths(t *testing.T, shared *SharedRunContext, request RunRequest, cfg dsl.Config) RunResult {
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

func assertDayOpenPriorityTrades(t *testing.T, result RunResult, want []int) {
	t.Helper()
	if got := tradeEntryIndexes(result.Trades); !reflect.DeepEqual(got, want) {
		t.Fatalf("trade entries = %v, want %v", got, want)
	}
	if result.TradeCount != len(want) || len(result.Trades) != len(want) {
		t.Fatalf("trade count = %d/%d, want %d/%d", result.TradeCount, len(result.Trades), len(want), len(want))
	}
	for _, trade := range result.Trades {
		if trade.Tag != "DSL-DOR:PDH" || trade.Meta["levelKey"] != "PDH" {
			t.Fatalf("trade identity = tag %q level %v, want DSL-DOR:PDH/PDH", trade.Tag, trade.Meta["levelKey"])
		}
	}
}

func dayOpenPriorityRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
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
