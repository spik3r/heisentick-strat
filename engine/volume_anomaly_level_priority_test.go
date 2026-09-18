package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestVolumeLevelPriorityExplicitnessAndFallback(t *testing.T) {
	type namedStrings []string
	type stringSliceAlias = []string
	var nilPriority []string
	defaultPriority := []string{"CAM_R4", "CAM_R3", "CAM_S3", "CAM_S4", "PDH", "PDL", "VWAP"}
	tests := []struct {
		name        string
		explicit    any
		cfgPriority []string
		nested      any
		want        []string
	}{
		{name: "explicit empty", explicit: true, cfgPriority: []string{}, nested: []any{"PDH"}, want: []string{}},
		{name: "explicit int64 true", explicit: int64(1), cfgPriority: []string{"VWAP", "PDH"}, nested: []any{"PDL"}, want: []string{"VWAP", "PDH"}},
		{name: "explicit alias order", explicit: true, cfgPriority: stringSliceAlias{"PDH", "VWAP"}, want: []string{"PDH", "VWAP"}},
		{name: "explicit typed nil", explicit: true, cfgPriority: nilPriority, nested: []any{"PDH"}, want: nil},
		{name: "explicit malformed decode", explicit: true, cfgPriority: stringSliceValue("VWAP"), nested: []any{"PDH"}, want: nil},
		{name: "explicit defined slice decode", explicit: true, cfgPriority: stringSliceValue(namedStrings{"VWAP"}), nested: []any{"PDH"}, want: nil},
		{name: "explicit order", explicit: true, cfgPriority: []string{"VWAP", "PDH", "CAM_R4"}, nested: []any{"PDL"}, want: []string{"VWAP", "PDH", "CAM_R4"}},
		{name: "nonexplicit nested fallback", explicit: false, cfgPriority: []string{"VWAP"}, nested: []any{"PDL", "PDH"}, want: []string{"PDL", "PDH"}},
		{name: "nonexplicit nested malformed defaults", explicit: false, cfgPriority: []string{"VWAP"}, nested: "PDL", want: defaultPriority},
		{name: "nonexplicit default fallback", explicit: false, cfgPriority: []string{"VWAP"}, want: defaultPriority},
		{name: "missing explicit flag defaults", cfgPriority: []string{"VWAP"}, want: defaultPriority},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := dsl.Config{}
			if tt.explicit != nil {
				cfg["levelPriorityExplicit"] = tt.explicit
			}
			volumeAnomaly := map[string]any{}
			if tt.nested != nil {
				volumeAnomaly["levelPriority"] = tt.nested
			}
			got := volumeLevelPriority(cfg, volumeAnomaly, tt.cfgPriority)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("volumeLevelPriority() = %v, want %v", got, tt.want)
			}
			if boolFromAny(tt.explicit, false) && len(tt.cfgPriority) > 0 && &got[0] != &tt.cfgPriority[0] {
				t.Fatal("explicit priority did not preserve normalized input ownership")
			}
		})
	}

	source := []string{"VWAP", "PDH"}
	decoded := stringSliceValue(source)
	got := volumeLevelPriority(dsl.Config{"levelPriorityExplicit": true}, nil, decoded)
	got[0] = "changed"
	if source[0] != "VWAP" || decoded[0] != "changed" {
		t.Fatalf("explicit ownership = source %v decoded %v, want copied source and returned normalized slice", source, decoded)
	}
}

func TestVolumeLevelPriorityFixtureAcrossCheckedExecutionPaths(t *testing.T) {
	type namedString string
	type namedStrings []string
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-volume-anomaly-exhaustion")
	shared, err := PrepareSharedRunContext(volumeAnomalyPriorityRequest(fixture, reviewedConfig))
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	var nilStrings []string
	tests := []struct {
		name     string
		explicit any
		present  bool
		priority any
		nested   any
		want     int
		entries  []int
	}{
		{name: "reviewed explicit VWAP", explicit: true, present: true, priority: []any{"VWAP"}, want: 3, entries: []int{316, 1323, 1595}},
		{name: "explicit mixed retains VWAP", explicit: true, present: true, priority: []any{1, "VWAP", namedString("PDH")}, want: 3, entries: []int{316, 1323, 1595}},
		{name: "explicit empty", explicit: true, present: true, priority: []any{}, want: 0},
		{name: "explicit typed nil", explicit: true, present: true, priority: nilStrings, want: 0},
		{name: "explicit absent priority", explicit: true, want: 0},
		{name: "explicit malformed scalar", explicit: true, present: true, priority: "VWAP", want: 0},
		{name: "explicit malformed map", explicit: true, present: true, priority: map[string]any{"level": "VWAP"}, want: 0},
		{name: "explicit defined slice", explicit: true, present: true, priority: namedStrings{"VWAP"}, want: 0},
		{name: "explicit all malformed", explicit: true, present: true, priority: []any{1, namedString("VWAP"), nil}, want: 0},
		{name: "nonexplicit nested VWAP", explicit: false, present: true, priority: []any{"PDH"}, nested: []any{"VWAP"}, want: 3, entries: []int{316, 1323, 1595}},
		{name: "nonexplicit defaults", explicit: false, present: true, priority: []any{}, want: 7},
		{name: "missing explicit flag defaults", present: true, priority: []any{"VWAP"}, want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			if tt.explicit == nil {
				delete(cfg, "levelPriorityExplicit")
			} else {
				cfg["levelPriorityExplicit"] = tt.explicit
			}
			if tt.present {
				cfg["levelPriority"] = tt.priority
			} else {
				delete(cfg, "levelPriority")
			}
			volumeAnomaly := mapValue(cfg, "volumeAnomalyExhaustion")
			if tt.nested == nil {
				delete(volumeAnomaly, "levelPriority")
			} else {
				volumeAnomaly["levelPriority"] = tt.nested
			}
			request := volumeAnomalyPriorityRequest(fixture, cfg)

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
			if publicResult.TradeCount != tt.want || len(publicResult.Trades) != tt.want {
				t.Fatalf("trade count = %d/%d, want %d/%d", publicResult.TradeCount, len(publicResult.Trades), tt.want, tt.want)
			}
			if tt.entries != nil {
				if got := tradeEntryIndexes(publicResult.Trades); !reflect.DeepEqual(got, tt.entries) {
					t.Fatalf("entries = %v, want %v", got, tt.entries)
				}
				for _, trade := range publicResult.Trades {
					if trade.Meta["levelKey"] != "VWAP" {
						t.Fatalf("reviewed level = %v, want VWAP", trade.Meta["levelKey"])
					}
				}
			}
		})
	}
}

func TestVolumeLevelPriorityDoesNotChangeOtherConsumers(t *testing.T) {
	cfg := dsl.Config{
		"setupType":             string(dsl.FamilyVolumeAnomalyExhaustion),
		"levelPriorityExplicit": true,
		"levelPriority":         []any{"LH", "PDH"},
	}
	params := paramsFromConfig(cfg)
	if got, want := params.LevelPriority, []string{"LH", "PDH"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared level priority = %v, want %v", got, want)
	}
	if got, want := params.DORLevelPriority, []string{"LH", "PDH"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("day-open priority = %v, want %v", got, want)
	}
	if got, want := params.SBHAllowedLevels, []string{"LH"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("session-break-hold levels = %v, want %v", got, want)
	}
	if !params.LevelPriorityExplicit {
		t.Fatal("break-retest/failed-breakout explicitness was not preserved")
	}
}

func volumeAnomalyPriorityRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
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
