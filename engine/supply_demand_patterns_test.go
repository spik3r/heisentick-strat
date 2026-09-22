package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestStringSliceMapValuePreservesPresenceOrderAndOwnership(t *testing.T) {
	type namedString string
	type namedStrings []string

	if got, present := stringSliceMapValue(nil, "patterns"); present || got != nil {
		t.Fatalf("nil map = (%v, %v), want (nil, false)", got, present)
	}
	if got, present := stringSliceMapValue(map[string]any{}, "patterns"); present || got != nil {
		t.Fatalf("missing key = (%v, %v), want (nil, false)", got, present)
	}

	stringsInput := []string{"RBR", "DBR"}
	stringsGot, present := stringSliceMapValue(map[string]any{"patterns": stringsInput}, "patterns")
	if !present || !reflect.DeepEqual(stringsGot, stringsInput) {
		t.Fatalf("string slice = (%v, %v), want (%v, true)", stringsGot, present, stringsInput)
	}
	stringsGot[0] = "changed"
	if stringsInput[0] != "RBR" {
		t.Fatalf("string-slice decode mutated caller input: %v", stringsInput)
	}
	stringsInput[1] = "source changed"
	if stringsGot[1] != "DBR" {
		t.Fatalf("caller mutation changed decoded string slice: %v", stringsGot)
	}

	anyInput := []any{1, "RBR", true, namedString("RBD"), "unknown", nil, "DBR"}
	anyGot, present := stringSliceMapValue(map[string]any{"patterns": anyInput}, "patterns")
	if want := []string{"RBR", "unknown", "DBR"}; !present || !reflect.DeepEqual(anyGot, want) {
		t.Fatalf("mixed slice = (%v, %v), want (%v, true)", anyGot, present, want)
	}
	anyGot[0] = "changed"
	if anyInput[1] != "RBR" {
		t.Fatalf("any-slice decode mutated caller input: %v", anyInput)
	}
	anyInput[6] = "source changed"
	if anyGot[2] != "DBR" {
		t.Fatalf("caller mutation changed decoded any slice: %v", anyGot)
	}
	mixedBuiltIns := []any{1, "DBR", namedString("RBR"), true}
	if got, present := stringSliceMapValue(map[string]any{"patterns": mixedBuiltIns}, "patterns"); !present || !reflect.DeepEqual(got, []string{"DBR"}) {
		t.Fatalf("mixed built-in decode = (%v, %v), want ([DBR], true)", got, present)
	}

	var nilStrings []string
	var nilAny []any
	for _, tt := range []struct {
		name  string
		value any
	}{
		{name: "nil", value: nil},
		{name: "typed nil string slice", value: nilStrings},
		{name: "typed nil any slice", value: nilAny},
		{name: "empty", value: []any{}},
		{name: "all malformed", value: []any{1, true, nil}},
		{name: "defined slice", value: namedStrings{"RBR"}},
		{name: "defined string element", value: []any{namedString("RBR")}},
		{name: "scalar", value: "RBR"},
		{name: "map", value: map[string]any{"pattern": "RBR"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, present := stringSliceMapValue(map[string]any{"patterns": tt.value}, "patterns")
			if !present || len(got) != 0 {
				t.Fatalf("decode = (%v, %v), want empty present result", got, present)
			}
		})
	}
}

func TestSupplyDemandPatternsPresenceAcrossCheckedExecutionPaths(t *testing.T) {
	type namedString string

	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-supply-demand")
	shared, err := PrepareSharedRunContext(supplyDemandPatternsRequest(fixture, reviewedConfig))
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	var nilStrings []string
	tests := []struct {
		name     string
		present  bool
		patterns any
		want     []int
	}{
		{name: "missing defaults all patterns", want: []int{873, 1349, 1804, 2884, 2907}},
		{name: "explicit full list", present: true, patterns: []any{"DBR", "RBR", "RBD", "DBD"}, want: []int{873, 1349, 1804, 2884, 2907}},
		{name: "RBR only", present: true, patterns: []any{"RBR"}, want: []int{873, 2884, 2907}},
		{name: "mixed retains exact built-in strings", present: true, patterns: []any{1, "DBR", namedString("RBR"), true}, want: []int{1349, 1804, 2885}},
		{name: "unknown only", present: true, patterns: []any{"unknown"}, want: []int{}},
		{name: "explicit empty", present: true, patterns: []any{}, want: []int{}},
		{name: "nil", present: true, patterns: nil, want: []int{}},
		{name: "typed nil", present: true, patterns: nilStrings, want: []int{}},
		{name: "all malformed", present: true, patterns: []any{1, true, nil}, want: []int{}},
		{name: "scalar", present: true, patterns: "RBR", want: []int{}},
		{name: "map", present: true, patterns: map[string]any{"pattern": "RBR"}, want: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			supplyDemand := mapValue(cfg, "supplyDemand")
			if tt.present {
				supplyDemand["patterns"] = tt.patterns
			} else {
				delete(supplyDemand, "patterns")
			}
			request := supplyDemandPatternsRequest(fixture, cfg)

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
				t.Fatalf("checked execution paths differ\npublic: %s\nprepared: %s\nshared: %s", canonicalJSON(publicResult), canonicalJSON(preparedResult), canonicalJSON(sharedResult))
			}
			if got := tradeEntryIndexes(publicResult.Trades); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("trade entries = %v, want %v", got, tt.want)
			}
			if publicResult.TradeCount != len(tt.want) {
				t.Fatalf("trade count = %d, want %d", publicResult.TradeCount, len(tt.want))
			}
		})
	}
}

func supplyDemandPatternsRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
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
