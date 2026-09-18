package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestOpeningRangeSessionsRawOverrideAcrossCheckedExecutionPaths(t *testing.T) {
	type namedString string
	type namedStrings []string
	type namedArray [1]string
	var nilStrings []string
	var nilAny []any
	sessionsPointer := &[]string{"ny"}

	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-opening-range-breakout")
	baselineRequest := openingRangeSessionsRequest(fixture, reviewedConfig)
	baseline, err := Run(baselineRequest)
	if err != nil {
		t.Fatalf("run reviewed baseline: %v", err)
	}
	baselineEntries := []int{35, 223, 294, 844, 1053, 1123, 1146, 1237, 1324, 1421, 1479, 1681}
	baselineSessions := []string{"ny", "ny", "london", "london", "ny", "london", "ny", "ny", "ny", "ny", "london", "ny"}
	assertOpeningRangeSessionTrades(t, baseline, baselineEntries, baselineSessions)
	shared, err := PrepareSharedRunContext(baselineRequest)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	nyEntries := []int{35, 223, 1053, 1146, 1237, 1324, 1421, 1681}
	londonEntries := []int{294, 844, 1123, 1479}
	tests := []struct {
		name         string
		reviewed     bool
		present      bool
		sessions     any
		wantEntries  []int
		wantSessions []string
		wantBaseline bool
	}{
		{name: "reviewed london and ny", reviewed: true, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "missing keeps shared windows", wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "ny string slice", present: true, sessions: []string{"ny"}, wantEntries: nyEntries, wantSessions: repeatedSession("ny", len(nyEntries))},
		{name: "london any slice", present: true, sessions: []any{"london"}, wantEntries: londonEntries, wantSessions: repeatedSession("london", len(londonEntries))},
		{name: "mixed retains ny", present: true, sessions: []any{1, "ny", true}, wantEntries: nyEntries, wantSessions: repeatedSession("ny", len(nyEntries))},
		{name: "empty string slice keeps shared windows", present: true, sessions: []string{}, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "empty any slice keeps shared windows", present: true, sessions: []any{}, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "typed nil string slice keeps shared windows", present: true, sessions: nilStrings, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "typed nil any slice keeps shared windows", present: true, sessions: nilAny, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "empty array keeps shared windows", present: true, sessions: [0]string{}, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "empty string keeps shared windows", present: true, sessions: "", wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "empty named string keeps shared windows", present: true, sessions: namedString(""), wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "nil keeps shared windows", present: true, sessions: nil, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "map keeps shared windows", present: true, sessions: map[string]any{"session": "ny"}, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "number keeps shared windows", present: true, sessions: 1, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "boolean keeps shared windows", present: true, sessions: true, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "pointer keeps shared windows", present: true, sessions: sessionsPointer, wantEntries: baselineEntries, wantSessions: baselineSessions, wantBaseline: true},
		{name: "unknown list closes all windows", present: true, sessions: []any{"unknown"}},
		{name: "all malformed list closes all windows", present: true, sessions: []any{1, true, nil}},
		{name: "other slice closes all windows", present: true, sessions: []int{1}},
		{name: "named slice closes all windows", present: true, sessions: namedStrings{"ny"}},
		{name: "array closes all windows", present: true, sessions: [1]string{"ny"}},
		{name: "named array closes all windows", present: true, sessions: namedArray{"ny"}},
		{name: "scalar ny deliberately closes all windows", present: true, sessions: "ny"},
		{name: "named scalar ny deliberately closes all windows", present: true, sessions: namedString("ny")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			if !tt.reviewed {
				orb := mapValue(cfg, "openingRangeBreakout")
				if tt.present {
					orb["openingSessions"] = tt.sessions
				} else {
					delete(orb, "openingSessions")
				}
			}
			result := runOpeningRangeSessionPaths(t, shared, openingRangeSessionsRequest(fixture, cfg), cfg)
			assertOpeningRangeSessionTrades(t, result, tt.wantEntries, tt.wantSessions)
			if tt.wantBaseline && !reflect.DeepEqual(result, baseline) {
				t.Fatalf("result differs from reviewed baseline\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(baseline))
			}
		})
	}
}

func TestOpeningRangeSessionsRawShapeOwnershipAndIsolation(t *testing.T) {
	type namedString string
	type namedStrings []string
	type namedArray [1]string
	var nilStrings []string
	sessionsPointer := &[]string{"ny"}
	tests := []struct {
		name     string
		sessions any
		wantRaw  bool
		decoded  []string
	}{
		{name: "string slice", sessions: []string{"ny", "london"}, wantRaw: true, decoded: []string{"ny", "london"}},
		{name: "mixed any slice", sessions: []any{1, "london", "ny", true}, wantRaw: true, decoded: []string{"london", "ny"}},
		{name: "unknown list", sessions: []any{"unknown"}, wantRaw: true, decoded: []string{"unknown"}},
		{name: "named slice", sessions: namedStrings{"ny"}, wantRaw: true},
		{name: "named array", sessions: namedArray{"ny"}, wantRaw: true},
		{name: "named string", sessions: namedString("ny"), wantRaw: true},
		{name: "typed nil slice", sessions: nilStrings, decoded: []string{}},
		{name: "empty array", sessions: [0]string{}},
		{name: "empty string", sessions: ""},
		{name: "map", sessions: map[string]any{"session": "ny"}},
		{name: "number", sessions: 1},
		{name: "boolean", sessions: true},
		{name: "pointer", sessions: sessionsPointer},
		{name: "nil", sessions: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := paramsFromConfig(dsl.Config{
				"setupType": string(dsl.FamilyOpeningRangeBreakout),
				"openingRangeBreakout": map[string]any{
					"openingSessions": tt.sessions,
				},
			})
			if params.ORBOpeningSessionsOverride != tt.wantRaw {
				t.Fatalf("override = %v, want %v", params.ORBOpeningSessionsOverride, tt.wantRaw)
			}
			if !reflect.DeepEqual(params.ORBOpeningSessions, tt.decoded) {
				t.Fatalf("decoded sessions = %v, want %v", params.ORBOpeningSessions, tt.decoded)
			}
		})
	}

	source := []string{"ny", "london"}
	owned := paramsFromConfig(dsl.Config{
		"setupType": string(dsl.FamilyOpeningRangeBreakout),
		"openingRangeBreakout": map[string]any{
			"openingSessions": source,
		},
	})
	source[0] = "changed"
	if got, want := owned.ORBOpeningSessions, []string{"ny", "london"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source mutation leaked: got %v want %v", got, want)
	}

	unrelated := paramsFromConfig(dsl.Config{
		"setupType": string(dsl.FamilyFlagContinuation),
		"openingRangeBreakout": map[string]any{
			"openingSessions": []any{"ny"},
		},
	})
	if unrelated.ORBOpeningSessionsOverride {
		t.Fatal("unrelated family activated ORB opening-session override")
	}
	if got, want := unrelated.ORBOpeningSessions, []string{"ny"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unrelated decoded sessions = %v, want %v", got, want)
	}
}

func TestOpeningAllowedWindowsUsesRawOverride(t *testing.T) {
	shared := flagParams{UseMidWindow: true, UseLondonWindow: true}
	if got, want := openingAllowedWindows(shared), map[string]bool{"asia": false, "mid": true, "london": true, "ny": false}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared windows = %v, want %v", got, want)
	}

	closed := shared
	closed.ORBOpeningSessionsOverride = true
	if got, want := openingAllowedWindows(closed), map[string]bool{"asia": false, "mid": false, "london": false, "ny": false}; !reflect.DeepEqual(got, want) {
		t.Fatalf("closed override windows = %v, want %v", got, want)
	}

	ny := closed
	ny.ORBOpeningSessions = []string{"unknown", "ny", "ny"}
	if got, want := openingAllowedWindows(ny), map[string]bool{"asia": false, "mid": false, "london": false, "ny": true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NY override windows = %v, want %v", got, want)
	}
}

func runOpeningRangeSessionPaths(t *testing.T, shared *SharedRunContext, request RunRequest, cfg dsl.Config) RunResult {
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

func assertOpeningRangeSessionTrades(t *testing.T, result RunResult, wantEntries []int, wantSessions []string) {
	t.Helper()
	if len(wantEntries) == 0 {
		if result.TradeCount != 0 || len(result.Trades) != 0 {
			t.Fatalf("trades = %d/%v, want none", result.TradeCount, tradeEntryIndexes(result.Trades))
		}
		return
	}
	if got := tradeEntryIndexes(result.Trades); !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("trade entries = %v, want %v", got, wantEntries)
	}
	if result.TradeCount != len(wantEntries) || len(result.Trades) != len(wantEntries) {
		t.Fatalf("trade count = %d/%d, want %d/%d", result.TradeCount, len(result.Trades), len(wantEntries), len(wantEntries))
	}
	gotSessions := make([]string, len(result.Trades))
	for i, trade := range result.Trades {
		gotSessions[i], _ = trade.Meta["session"].(string)
		wantTag := "DSL-ORB:" + gotSessions[i]
		if trade.Tag != wantTag || trade.Meta["setup"] != "openingRangeBreakout" {
			t.Fatalf("trade %d identity = tag %q setup %v, want %s/openingRangeBreakout", i, trade.Tag, trade.Meta["setup"], wantTag)
		}
	}
	if !reflect.DeepEqual(gotSessions, wantSessions) {
		t.Fatalf("trade sessions = %v, want %v", gotSessions, wantSessions)
	}
}

func repeatedSession(session string, count int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = session
	}
	return out
}

func openingRangeSessionsRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
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
