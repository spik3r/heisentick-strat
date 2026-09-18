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

func TestVWAPExtensionFadeSessionPhaseDefaultsAndPrecedence(t *testing.T) {
	type namedStrings []string
	var nilStrings []string
	tests := []struct {
		name            string
		cfg             dsl.Config
		wantFamilyPhase []string
		wantSharedPhase []string
	}{
		{
			name:            "family default",
			cfg:             dsl.Config{"setupType": string(dsl.FamilyVWAPExtensionFade)},
			wantFamilyPhase: []string{"lunch", "close"},
		},
		{
			name: "shared authored phases",
			cfg: dsl.Config{
				"setupType":     string(dsl.FamilyVWAPExtensionFade),
				"sessionPhases": []any{"open"},
			},
			wantFamilyPhase: []string{"open"},
			wantSharedPhase: []string{"open"},
		},
		{
			name: "nested phases are ignored",
			cfg: dsl.Config{
				"setupType":     string(dsl.FamilyVWAPExtensionFade),
				"sessionPhases": []any{"open"},
				"vwapExtensionFade": map[string]any{
					"sessionPhases": []any{"middle"},
				},
			},
			wantFamilyPhase: []string{"open"},
			wantSharedPhase: []string{"open"},
		},
		{
			name: "decoded empty top level defaults",
			cfg: dsl.Config{
				"setupType":     string(dsl.FamilyVWAPExtensionFade),
				"sessionPhases": nilStrings,
			},
			wantFamilyPhase: []string{"lunch", "close"},
			wantSharedPhase: []string{},
		},
		{
			name: "unsupported top level decodes empty and defaults",
			cfg: dsl.Config{
				"setupType":     string(dsl.FamilyVWAPExtensionFade),
				"sessionPhases": namedStrings{"middle"},
			},
			wantFamilyPhase: []string{"lunch", "close"},
		},
		{
			name:            "default does not spill into another family",
			cfg:             dsl.Config{"setupType": string(dsl.FamilyBreakRetest)},
			wantFamilyPhase: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := paramsFromConfig(tt.cfg)
			if got := params.VWAPExtensionFade.SessionPhases; !reflect.DeepEqual(got, tt.wantFamilyPhase) {
				t.Fatalf("VWAP-extension-fade phases = %v, want %v", got, tt.wantFamilyPhase)
			}
			if got := params.SessionPhases; !reflect.DeepEqual(got, tt.wantSharedPhase) {
				t.Fatalf("shared session phases = %v, want %v", got, tt.wantSharedPhase)
			}
		})
	}

	nestedShapes := []struct {
		name  string
		value any
	}{
		{name: "valid", value: []any{"middle"}},
		{name: "empty", value: []any{}},
		{name: "mixed", value: []any{"middle", 1, true}},
		{name: "unknown", value: []any{"unknown"}},
		{name: "malformed", value: "middle"},
		{name: "nil", value: nil},
	}
	for _, tt := range nestedShapes {
		t.Run("nested "+tt.name+" defaults without presence", func(t *testing.T) {
			params := paramsFromConfig(dsl.Config{
				"setupType": string(dsl.FamilyVWAPExtensionFade),
				"vwapExtensionFade": map[string]any{
					"sessionPhases": tt.value,
				},
			})
			if got, want := params.VWAPExtensionFade.SessionPhases, []string{"lunch", "close"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("nested shape produced phases %v, want %v", got, want)
			}
			if len(params.SessionPhases) != 0 {
				t.Fatalf("nested shape changed shared phases to %v", params.SessionPhases)
			}
		})
	}

	source := []string{"middle"}
	owned := paramsFromConfig(dsl.Config{
		"setupType":     string(dsl.FamilyVWAPExtensionFade),
		"sessionPhases": source,
	})
	source[0] = "changed"
	if got, want := owned.VWAPExtensionFade.SessionPhases, []string{"middle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top-level source mutation leaked into family phases: got %v want %v", got, want)
	}
	if got, want := owned.SessionPhases, []string{"middle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top-level source mutation leaked into shared phases: got %v want %v", got, want)
	}
	owned.VWAPExtensionFade.SessionPhases[0] = "family changed"
	if got, want := owned.SessionPhases, []string{"middle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("family phase mutation leaked into shared phases: got %v want %v", got, want)
	}
	owned.SessionPhases[0] = "shared changed"
	if got, want := owned.VWAPExtensionFade.SessionPhases, []string{"family changed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared phase mutation leaked into family phases: got %v want %v", got, want)
	}
}

func TestVWAPExtensionFadeSessionPhaseExecutionPrecedence(t *testing.T) {
	const caseName = "family-vwap-extension-fade"
	fixture, reviewedConfig := loadEntryAttemptCase(t, caseName)
	baselineRequest := vwapExtensionFadePhaseRequest(fixture, reviewedConfig)
	baseline, err := Run(baselineRequest)
	if err != nil {
		t.Fatalf("run reviewed baseline: %v", err)
	}
	closeEntries := []int{20, 39, 311, 389, 479, 583, 775, 847, 861, 1044, 1124, 1142, 1238, 1420, 1771}
	middleEntries := []int{107, 195, 839, 933, 1306, 1751}
	assertVWAPExtensionFadePhaseTrades(t, baseline, closeEntries, "close")
	shared, err := PrepareSharedRunContext(baselineRequest)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	nestedShapes := []struct {
		name  string
		value any
	}{
		{name: "valid", value: []any{"middle"}},
		{name: "empty", value: []any{}},
		{name: "mixed", value: []any{"middle", 1, true}},
		{name: "unknown", value: []any{"unknown"}},
		{name: "malformed", value: "middle"},
		{name: "nil", value: nil},
	}
	type testCase struct {
		name         string
		topPresent   bool
		top          any
		nestedSet    bool
		nested       any
		wantEntries  []int
		wantPhase    string
		wantBaseline bool
	}
	tests := []testCase{
		{name: "reviewed close", topPresent: true, top: []any{"close"}, wantEntries: closeEntries, wantPhase: "close", wantBaseline: true},
		{name: "omitted top-level defaults", wantEntries: closeEntries, wantPhase: "close", wantBaseline: true},
		{name: "empty top-level defaults", topPresent: true, top: []any{}, wantEntries: closeEntries, wantPhase: "close", wantBaseline: true},
		{name: "top-level middle ignores nested close", topPresent: true, top: []any{"middle"}, nestedSet: true, nested: []any{"close"}, wantEntries: middleEntries, wantPhase: "middle"},
		{name: "top-level open and lunch", topPresent: true, top: []any{"open", "lunch"}},
	}
	for _, nested := range nestedShapes {
		tests = append(tests, testCase{
			name:         "nested " + nested.name + " ignored",
			nestedSet:    true,
			nested:       nested.value,
			wantEntries:  closeEntries,
			wantPhase:    "close",
			wantBaseline: true,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			if tt.name != "reviewed close" {
				if tt.topPresent {
					cfg["sessionPhases"] = tt.top
				} else {
					delete(cfg, "sessionPhases")
				}
				vef := mapValue(cfg, "vwapExtensionFade")
				if tt.nestedSet {
					vef["sessionPhases"] = tt.nested
				} else {
					delete(vef, "sessionPhases")
				}
			}
			result := runVWAPExtensionFadePhasePaths(t, shared, vwapExtensionFadePhaseRequest(fixture, cfg), cfg)
			assertVWAPExtensionFadePhaseTrades(t, result, tt.wantEntries, tt.wantPhase)
			if tt.wantBaseline && !reflect.DeepEqual(result, baseline) {
				t.Fatalf("result differs from reviewed baseline\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(baseline))
			}
		})
	}
}

func TestVWAPExtensionFadeParserAuthoredEmptyDefaults(t *testing.T) {
	const caseName = "family-vwap-extension-fade"
	fixture, reviewedConfig := loadEntryAttemptCase(t, caseName)
	baseline, err := Run(vwapExtensionFadePhaseRequest(fixture, reviewedConfig))
	if err != nil {
		t.Fatalf("run reviewed baseline: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	mutated := strings.Replace(string(source), "  session phase in (close)\n", "  session phase in ()\n", 1)
	if mutated == string(source) {
		t.Fatal("fixture source did not contain the expected explicit close phase")
	}
	parsed, err := dsl.Parse(mutated)
	if err != nil {
		t.Fatalf("parse authored empty phase list: %v", err)
	}
	if len(parsed.Errors) > 0 {
		t.Fatalf("parse diagnostics: %v", parsed.Errors)
	}
	if got := stringSliceValue(parsed.Config["sessionPhases"]); len(got) != 0 {
		t.Fatalf("parser-authored empty phases = %v, want empty", got)
	}
	params := paramsFromConfig(parsed.Config)
	if got, want := params.VWAPExtensionFade.SessionPhases, []string{"lunch", "close"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("authored empty family phases = %v, want %v", got, want)
	}
	result, err := Run(vwapExtensionFadePhaseRequest(fixture, parsed.Config))
	if err != nil {
		t.Fatalf("run authored empty phase list: %v", err)
	}
	assertVWAPExtensionFadePhaseTrades(t, result, []int{20, 39, 311, 389, 479, 583, 775, 847, 861, 1044, 1124, 1142, 1238, 1420, 1771}, "close")
	if !reflect.DeepEqual(result, baseline) {
		t.Fatalf("parser-authored empty result differs from reviewed baseline\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(baseline))
	}
}

func runVWAPExtensionFadePhasePaths(t *testing.T, shared *SharedRunContext, request RunRequest, cfg dsl.Config) RunResult {
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

func assertVWAPExtensionFadePhaseTrades(t *testing.T, result RunResult, wantEntries []int, wantPhase string) {
	t.Helper()
	gotEntries := tradeEntryIndexes(result.Trades)
	if len(wantEntries) == 0 && len(gotEntries) != 0 {
		t.Fatalf("trade entries = %v, want none", gotEntries)
	}
	if len(wantEntries) > 0 && !reflect.DeepEqual(gotEntries, wantEntries) {
		t.Fatalf("trade entries = %v, want %v", gotEntries, wantEntries)
	}
	if result.TradeCount != len(wantEntries) || len(result.Trades) != len(wantEntries) {
		t.Fatalf("trade count = %d/%d, want %d/%d", result.TradeCount, len(result.Trades), len(wantEntries), len(wantEntries))
	}
	for i, trade := range result.Trades {
		if trade.Tag != "DSL-VEF:VWAP" || trade.Meta["setup"] != "vwapExtensionFade" || trade.Meta["sessionPhase"] != wantPhase {
			t.Fatalf("trade %d identity = tag %q setup %v phase %v, want DSL-VEF:VWAP/vwapExtensionFade/%s", i, trade.Tag, trade.Meta["setup"], trade.Meta["sessionPhase"], wantPhase)
		}
	}
}

func vwapExtensionFadePhaseRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
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
