package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSetupTypeFromAnyContract(t *testing.T) {
	type otherString string
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "built-in string", value: "channelBreakHold", want: "channelBreakHold"},
		{name: "FamilyID", value: dsl.FamilyChannelBreakHold, want: "channelBreakHold"},
		{name: "empty built-in string", value: "", want: ""},
		{name: "missing", value: nil, want: ""},
		{name: "numeric type remains excluded", value: 1, want: ""},
		{name: "unrelated named string remains excluded", value: otherString("channelBreakHold"), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setupTypeFromAny(tt.value); got != tt.want {
				t.Fatalf("setupTypeFromAny(%T(%v)) = %q, want %q", tt.value, tt.value, got, tt.want)
			}
		})
	}
}

func TestFamilyIDSetupTypeMatchesPlainStringAcrossRunPaths(t *testing.T) {
	fixture, parsedConfig := loadEntryAttemptCase(t, "family-channel-break-hold")
	plainConfig := cloneTestConfig(t, parsedConfig)
	plainConfig["setupType"] = string(dsl.FamilyChannelBreakHold)
	namedConfig := cloneTestConfig(t, parsedConfig)
	namedConfig["setupType"] = dsl.FamilyChannelBreakHold

	plainParams := paramsFromConfig(plainConfig)
	namedParams := paramsFromConfig(namedConfig)
	if !reflect.DeepEqual(namedParams, plainParams) {
		t.Fatalf("named FamilyID params differ from plain string\n got: %+v\nwant: %+v", namedParams, plainParams)
	}
	if got, want := namedParams.SetupType, string(dsl.FamilyChannelBreakHold); got != want {
		t.Fatalf("named FamilyID setup type = %q, want %q", got, want)
	}

	plainOptions := contextOptions(fixture, plainConfig)
	namedOptions := contextOptions(fixture, namedConfig)
	if !reflect.DeepEqual(namedOptions, plainOptions) {
		t.Fatalf("named FamilyID context options differ from plain string\n got: %+v\nwant: %+v", namedOptions, plainOptions)
	}
	if !namedOptions.Channel.Enabled {
		t.Fatal("named channel-break-hold FamilyID did not request channel context")
	}

	series := marketdata.SeriesFromBars(fixture.Bars)
	htfSeries := marketdata.SeriesFromBars(fixture.HTFBars)
	request := func(cfg dsl.Config) RunRequest {
		return RunRequest{
			Config:          cfg,
			Series:          series,
			HTFSeries:       htfSeries,
			StrategyID:      fixture.StrategyID,
			Symbol:          fixture.Symbol,
			Timeframe:       fixture.Timeframe,
			HigherTimeframe: fixture.HigherTimeframe,
			RangeMethod:     fixture.RangeMethod,
			Costs:           fixture.Costs,
		}
	}
	plainRequest := request(plainConfig)
	namedRequest := request(namedConfig)

	plainKey, err := SharedContextKey(plainRequest)
	if err != nil {
		t.Fatalf("plain SharedContextKey: %v", err)
	}
	namedKey, err := SharedContextKey(namedRequest)
	if err != nil {
		t.Fatalf("named SharedContextKey: %v", err)
	}
	if namedKey != plainKey {
		t.Fatalf("named FamilyID shared-context key differs\n got: %s\nwant: %s", namedKey, plainKey)
	}

	plainShared, err := PrepareSharedRunContext(plainRequest)
	if err != nil {
		t.Fatalf("plain PrepareSharedRunContext: %v", err)
	}
	namedShared, err := PrepareSharedRunContext(namedRequest)
	if err != nil {
		t.Fatalf("named PrepareSharedRunContext: %v", err)
	}
	if !reflect.DeepEqual(namedShared.options, plainShared.options) {
		t.Fatalf("named shared context options differ\n got: %+v\nwant: %+v", namedShared.options, plainShared.options)
	}

	plainPrepared, err := PrepareRun(plainRequest)
	if err != nil {
		t.Fatalf("plain PrepareRun: %v", err)
	}
	namedPrepared, err := PrepareRun(namedRequest)
	if err != nil {
		t.Fatalf("named PrepareRun: %v", err)
	}
	plainVariant, err := namedShared.PrepareVariant(plainConfig)
	if err != nil {
		t.Fatalf("plain variant on named shared context: %v", err)
	}
	namedVariant, err := plainShared.PrepareVariant(namedConfig)
	if err != nil {
		t.Fatalf("named variant on plain shared context: %v", err)
	}

	plainDirect, err := Run(plainRequest)
	if err != nil {
		t.Fatalf("plain Run: %v", err)
	}
	namedDirect, err := Run(namedRequest)
	if err != nil {
		t.Fatalf("named Run: %v", err)
	}
	results := map[string]RunResult{
		"named direct":                   namedDirect,
		"plain prepared":                 plainPrepared.Run(fixture.Costs),
		"named prepared":                 namedPrepared.Run(fixture.Costs),
		"plain variant on named context": plainVariant.Run(fixture.Costs),
		"named variant on plain context": namedVariant.Run(fixture.Costs),
	}
	if plainDirect.TradeCount != 1 || len(plainDirect.Trades) != 1 {
		t.Fatalf("reviewed fixture trades = %d/%d, want 1/1", plainDirect.TradeCount, len(plainDirect.Trades))
	}
	for name, result := range results {
		if !reflect.DeepEqual(result, plainDirect) {
			t.Fatalf("%s result differs from plain direct result\n got: %s\nwant: %s", name, canonicalJSON(result), canonicalJSON(plainDirect))
		}
	}
}
