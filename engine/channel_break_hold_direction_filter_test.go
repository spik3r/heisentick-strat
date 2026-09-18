package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestChannelBreakHoldRawDirectionFilterAcrossCheckedExecutionPaths(t *testing.T) {
	type namedString string
	type namedStrings []string
	directionPointer := &[]string{"ascending"}

	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-channel-break-hold")
	baselineRequest := channelDirectionRequest(fixture, reviewedConfig)
	baseline, err := Run(baselineRequest)
	if err != nil {
		t.Fatalf("run reviewed baseline: %v", err)
	}
	assertChannelDirectionTrades(t, baseline, true)
	shared, err := PrepareSharedRunContext(baselineRequest)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	tests := []struct {
		name         string
		present      bool
		directions   any
		wantTrade    bool
		wantBaseline bool
	}{
		{name: "missing disables filter", wantTrade: true, wantBaseline: true},
		{name: "descending string slice", present: true, directions: []string{"descending"}, wantTrade: true, wantBaseline: true},
		{name: "descending any slice", present: true, directions: []any{"descending"}, wantTrade: true, wantBaseline: true},
		{name: "mixed retains descending", present: true, directions: []any{1, "descending", true}, wantTrade: true, wantBaseline: true},
		{name: "scalar descending", present: true, directions: "descending", wantTrade: true, wantBaseline: true},
		{name: "named scalar descending", present: true, directions: namedString("descending"), wantTrade: true, wantBaseline: true},
		{name: "scalar contains descending", present: true, directions: "ascending,descending", wantTrade: true, wantBaseline: true},
		{name: "ascending list rejects descending channel", present: true, directions: []any{"ascending"}},
		{name: "unknown list rejects descending channel", present: true, directions: []any{"unknown"}},
		{name: "all malformed nonempty list rejects", present: true, directions: []any{1, true, nil}},
		{name: "nonempty named slice requires decoded membership", present: true, directions: namedStrings{"descending"}},
		{name: "nonempty other slice requires decoded membership", present: true, directions: []int{1}},
		{name: "nonempty array requires decoded membership", present: true, directions: [1]string{"descending"}},
		{name: "scalar ascending rejects descending channel", present: true, directions: "ascending"},
		{name: "scalar substring does not match full direction", present: true, directions: "desc"},
		{name: "empty string disables filter", present: true, directions: "", wantTrade: true, wantBaseline: true},
		{name: "empty string slice disables filter", present: true, directions: []string{}, wantTrade: true, wantBaseline: true},
		{name: "empty any slice disables filter", present: true, directions: []any{}, wantTrade: true, wantBaseline: true},
		{name: "empty array disables filter", present: true, directions: [0]string{}, wantTrade: true, wantBaseline: true},
		{name: "map disables filter", present: true, directions: map[string]any{"direction": "ascending"}, wantTrade: true, wantBaseline: true},
		{name: "number disables filter", present: true, directions: 1, wantTrade: true, wantBaseline: true},
		{name: "pointer disables filter", present: true, directions: directionPointer, wantTrade: true, wantBaseline: true},
		{name: "nil disables filter", present: true, directions: nil, wantTrade: true, wantBaseline: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			channel := mapValue(cfg, "channel")
			if tt.present {
				channel["directions"] = tt.directions
			} else {
				delete(channel, "directions")
			}
			result := runChannelDirectionPaths(t, shared, channelDirectionRequest(fixture, cfg), cfg)
			assertChannelDirectionTrades(t, result, tt.wantTrade)
			if tt.wantBaseline && !reflect.DeepEqual(result, baseline) {
				t.Fatalf("result differs from reviewed descending baseline\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(baseline))
			}
		})
	}
}

func TestChannelBreakHoldDirectionFilterShapeOrderOwnershipAndIsolation(t *testing.T) {
	type namedString string

	source := []string{"descending", "ascending"}
	params := paramsFromConfig(dsl.Config{
		"setupType": string(dsl.FamilyChannelBreakHold),
		"channel": map[string]any{
			"directions": source,
		},
	})
	if got, want := params.ChannelBreakHold.ChannelDirections, []string{"descending", "ascending"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded directions = %v, want %v", got, want)
	}
	if !params.ChannelBreakHold.DirectionRequired || params.ChannelBreakHold.DirectionText != "" {
		t.Fatalf("list filter shape = required %v text %q, want true/empty", params.ChannelBreakHold.DirectionRequired, params.ChannelBreakHold.DirectionText)
	}
	source[0] = "changed"
	if params.ChannelBreakHold.ChannelDirections[0] != "descending" {
		t.Fatalf("source mutation leaked into decoded directions: %v", params.ChannelBreakHold.ChannelDirections)
	}

	mixed := paramsFromConfig(dsl.Config{
		"setupType": string(dsl.FamilyChannelBreakHold),
		"channel": map[string]any{
			"directions": []any{1, "ascending", "descending", true},
		},
	})
	if got, want := mixed.ChannelBreakHold.ChannelDirections, []string{"ascending", "descending"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed decoded directions = %v, want %v", got, want)
	}
	if !mixed.ChannelBreakHold.DirectionRequired || mixed.ChannelBreakHold.DirectionText != "" {
		t.Fatalf("mixed filter shape = required %v text %q, want true/empty", mixed.ChannelBreakHold.DirectionRequired, mixed.ChannelBreakHold.DirectionText)
	}

	text := paramsFromConfig(dsl.Config{
		"setupType": string(dsl.FamilyChannelBreakHold),
		"channel": map[string]any{
			"directions": namedString("ascending,descending"),
		},
	})
	if !text.ChannelBreakHold.DirectionRequired || text.ChannelBreakHold.DirectionText != "ascending,descending" || text.ChannelBreakHold.ChannelDirections != nil {
		t.Fatalf("text filter shape = required %v text %q decoded %v", text.ChannelBreakHold.DirectionRequired, text.ChannelBreakHold.DirectionText, text.ChannelBreakHold.ChannelDirections)
	}

	unrelated := paramsFromConfig(dsl.Config{
		"setupType": string(dsl.FamilyFlagContinuation),
		"channel": map[string]any{
			"directions": []any{1, "descending", true},
		},
	})
	if unrelated.ChannelBreakHold.DirectionRequired || unrelated.ChannelBreakHold.DirectionText != "" {
		t.Fatalf("unrelated family activated channel direction filter: %+v", unrelated.ChannelBreakHold)
	}
	if got, want := unrelated.ChannelBreakHold.ChannelDirections, []string{"descending"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unrelated decoded directions = %v, want %v", got, want)
	}
}

func runChannelDirectionPaths(t *testing.T, shared *SharedRunContext, request RunRequest, cfg dsl.Config) RunResult {
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

func assertChannelDirectionTrades(t *testing.T, result RunResult, wantTrade bool) {
	t.Helper()
	if !wantTrade {
		if result.TradeCount != 0 || len(result.Trades) != 0 {
			t.Fatalf("trades = %d/%v, want none", result.TradeCount, tradeEntryIndexes(result.Trades))
		}
		return
	}
	if got := tradeEntryIndexes(result.Trades); !reflect.DeepEqual(got, []int{961}) {
		t.Fatalf("trade entries = %v, want [961]", got)
	}
	if result.TradeCount != 1 || len(result.Trades) != 1 {
		t.Fatalf("trade count = %d/%d, want 1/1", result.TradeCount, len(result.Trades))
	}
	trade := result.Trades[0]
	if trade.Side != "short" || trade.Tag != "DSL-CBH:channel.low" || trade.Meta["channelDirection"] != "descending" {
		t.Fatalf("trade identity = side %q tag %q direction %v, want short/DSL-CBH:channel.low/descending", trade.Side, trade.Tag, trade.Meta["channelDirection"])
	}
}

func channelDirectionRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
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
