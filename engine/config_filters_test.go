package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestMapValueAcceptsPlainAndDSLConfigWithoutCopying(t *testing.T) {
	type unrelatedMap map[string]any
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "plain map", value: map[string]any{"value": 1}, want: true},
		{name: "nested dsl.Config", value: dsl.Config{"value": 1}, want: true},
		{name: "missing", value: nil, want: false},
		{name: "scalar", value: 1, want: false},
		{name: "unrelated named map", value: unrelatedMap{"value": 1}, want: false},
	}

	if got := mapValue(nil, "section"); got != nil {
		t.Fatalf("mapValue(nil) = %v, want nil", got)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outer := map[string]any{}
			if tt.value != nil {
				outer["section"] = tt.value
			}
			got := mapValue(outer, "section")
			if (got != nil) != tt.want {
				t.Fatalf("mapValue(%T) = %v, accepted=%v want %v", tt.value, got, got != nil, tt.want)
			}
			if !tt.want {
				return
			}

			got["throughResult"] = 2
			switch source := tt.value.(type) {
			case map[string]any:
				if source["throughResult"] != 2 {
					t.Fatalf("plain source did not observe result mutation: %v", source)
				}
				source["throughSource"] = 3
			case dsl.Config:
				if source["throughResult"] != 2 {
					t.Fatalf("named source did not observe result mutation: %v", source)
				}
				source["throughSource"] = 3
			}
			if got["throughSource"] != 3 {
				t.Fatalf("result did not observe source mutation: %v", got)
			}
		})
	}
}

func TestRunAcceptsNestedDSLConfigTargetSection(t *testing.T) {
	fixture, parsedConfig := loadEntryAttemptCase(t, "family-supply-demand")
	plainConfig := cloneTestConfig(t, parsedConfig)
	namedConfig := cloneTestConfig(t, parsedConfig)
	target := mapValue(namedConfig, "target")
	if target == nil {
		t.Fatal("reviewed fixture target section is missing")
	}
	namedConfig["target"] = dsl.Config(target)

	series := marketdata.SeriesFromBars(fixture.Bars)
	htfSeries := marketdata.SeriesFromBars(fixture.HTFBars)
	run := func(t *testing.T, cfg dsl.Config) RunResult {
		t.Helper()
		result, err := Run(RunRequest{
			Config:          cfg,
			Series:          series,
			HTFSeries:       htfSeries,
			StrategyID:      fixture.StrategyID,
			Symbol:          fixture.Symbol,
			Timeframe:       fixture.Timeframe,
			HigherTimeframe: fixture.HigherTimeframe,
			RangeMethod:     fixture.RangeMethod,
			Costs:           fixture.Costs,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return result
	}

	plainResult := run(t, plainConfig)
	namedResult := run(t, namedConfig)
	if plainResult.TradeCount != 5 || len(plainResult.Trades) != 5 {
		t.Fatalf("reviewed fixture trades = %d/%d, want 5/5", plainResult.TradeCount, len(plainResult.Trades))
	}
	if got, want := plainResult.Trades[0].InitialTP, 1.10157166071429; got != want {
		t.Fatalf("reviewed first target = %.15g, want %.15g", got, want)
	}
	if !reflect.DeepEqual(namedResult, plainResult) {
		t.Fatalf("nested dsl.Config target result differs from plain map\n got: %s\nwant: %s", canonicalJSON(namedResult), canonicalJSON(plainResult))
	}
}

func TestBoolFromAnyInt64AndExistingTypeContract(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		fallback bool
		want     bool
	}{
		{name: "int64 zero overrides true fallback", value: int64(0), fallback: true, want: false},
		{name: "int64 positive", value: int64(1), fallback: false, want: true},
		{name: "int64 negative", value: int64(-1), fallback: false, want: true},
		{name: "bool true preserved", value: true, fallback: false, want: true},
		{name: "bool false preserved", value: false, fallback: true, want: false},
		{name: "int zero preserved", value: int(0), fallback: true, want: false},
		{name: "int negative preserved", value: int(-1), fallback: false, want: true},
		{name: "float64 zero preserved", value: float64(0), fallback: true, want: false},
		{name: "float64 negative preserved", value: float64(-1), fallback: false, want: true},
		{name: "string remains excluded", value: "1", fallback: false, want: false},
		{name: "float32 remains excluded", value: float32(1), fallback: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := boolFromAny(tt.value, tt.fallback); got != tt.want {
				t.Fatalf("boolFromAny(%T(%v), %v) = %v, want %v", tt.value, tt.value, tt.fallback, got, tt.want)
			}
		})
	}
}
