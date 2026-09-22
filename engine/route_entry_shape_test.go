package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

type routeEntryNamedString string

func TestCompiledRouteEntryRequiresBuiltInStringFields(t *testing.T) {
	invalidValues := []struct {
		name    string
		value   any
		missing bool
	}{
		{name: "missing", missing: true},
		{name: "nil", value: nil},
		{name: "number", value: 1},
		{name: "bool", value: true},
		{name: "map", value: map[string]any{}},
		{name: "slice", value: []any{}},
		{name: "named string", value: routeEntryNamedString("")},
	}

	for _, field := range []struct {
		name      string
		key       string
		otherKey  string
		other     string
		candidate string
	}{
		{name: "symbol", key: "symbol", otherKey: "tf", other: "1h", candidate: ""},
		{name: "timeframe", key: "tf", otherKey: "symbol", other: "XAUUSD", candidate: ""},
	} {
		t.Run(field.name, func(t *testing.T) {
			for _, invalid := range invalidValues {
				t.Run(invalid.name, func(t *testing.T) {
					entry := map[string]any{field.otherKey: field.other}
					if !invalid.missing {
						entry[field.key] = invalid.value
					}
					cfg := dsl.Config{
						"slices":     []any{entry},
						"symbols":    []string{"XAUUSD"},
						"timeframes": []string{"1h"},
					}
					symbol, timeframe := field.other, field.candidate
					if field.key == "symbol" {
						symbol, timeframe = field.candidate, field.other
					}
					if compiledRouteAllowed(cfg, symbol, timeframe) {
						t.Fatal("malformed declared route matched an empty request field")
					}
				})
			}
		})
	}

	for _, tt := range []struct {
		name      string
		entry     map[string]any
		symbol    string
		timeframe string
	}{
		{name: "explicit empty symbol", entry: map[string]any{"symbol": "", "tf": "1h"}, symbol: "", timeframe: "1h"},
		{name: "explicit empty timeframe", entry: map[string]any{"symbol": "XAUUSD", "tf": ""}, symbol: "XAUUSD", timeframe: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !compiledRouteAllowed(dsl.Config{"slices": []any{tt.entry}}, tt.symbol, tt.timeframe) {
				t.Fatal("exact built-in empty string route should be allowed")
			}
		})
	}

	t.Run("valid sibling still matches", func(t *testing.T) {
		cfg := dsl.Config{"slices": []any{
			map[string]any{"symbol": nil, "tf": "1h"},
			map[string]any{"symbol": "XAUUSD", "tf": "1h"},
		}}
		if !compiledRouteAllowed(cfg, "XAUUSD", "1h") {
			t.Fatal("valid sibling should match after a malformed entry is skipped")
		}
	})

	t.Run("all invalid entries remain authoritative", func(t *testing.T) {
		cfg := dsl.Config{
			"slices":     []any{map[string]any{"symbol": nil, "tf": "1h"}},
			"symbols":    "XAUUSD",
			"timeframes": "1h",
		}
		if compiledRouteAllowed(cfg, "XAUUSD", "1h") {
			t.Fatal("declared slices must not fall back when every entry is invalid")
		}
	})

	for _, tt := range []struct {
		name      string
		symbol    string
		timeframe string
	}{
		{name: "symbol case mismatch", symbol: "xauusd", timeframe: "1h"},
		{name: "symbol substring mismatch", symbol: "XAU", timeframe: "1h"},
		{name: "timeframe case mismatch", symbol: "XAUUSD", timeframe: "1H"},
		{name: "timeframe substring mismatch", symbol: "XAUUSD", timeframe: "1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := dsl.Config{"slices": []any{map[string]any{"symbol": "XAUUSD", "tf": "1h"}}}
			if compiledRouteAllowed(cfg, tt.symbol, tt.timeframe) {
				t.Fatal("route entry fields must compare exactly")
			}
		})
	}
}

func TestCompiledRouteEntryDoesNotMutateSupportedMaps(t *testing.T) {
	plain := map[string]any{"symbol": nil, "tf": "1h"}
	named := dsl.Config{"symbol": "XAUUSD", "tf": routeEntryNamedString("")}
	plainBefore := map[string]any{"symbol": nil, "tf": "1h"}
	namedBefore := dsl.Config{"symbol": "XAUUSD", "tf": routeEntryNamedString("")}

	for name, cfg := range map[string]dsl.Config{
		"plain map":  {"slices": []any{plain}},
		"dsl.Config": {"slices": []any{named}},
	} {
		t.Run(name, func(t *testing.T) {
			if compiledRouteAllowed(cfg, "", "1h") || compiledRouteAllowed(cfg, "XAUUSD", "") {
				t.Fatal("malformed entry should not match")
			}
		})
	}

	if !reflect.DeepEqual(plain, plainBefore) {
		t.Fatalf("plain map mutated: got %v, want %v", plain, plainBefore)
	}
	if !reflect.DeepEqual(named, namedBefore) {
		t.Fatalf("dsl.Config mutated: got %v, want %v", named, namedBefore)
	}
}

func TestCompiledRouteEntryShapeAcrossCheckedExecutionPaths(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-break-retest")
	series := marketdata.SeriesFromBars(fixture.Bars)
	wantEntries := []int{315, 389, 590, 756, 933, 1074, 1093, 1412, 1442, 1580, 1649, 1695, 1987, 2475}

	controlConfig := cloneTestConfig(t, reviewedConfig)
	controlConfig["slices"] = []any{map[string]any{"symbol": "", "tf": fixture.Timeframe}}
	controlRequest := routeEntryShapeRequest(fixture, controlConfig)
	if !RouteAllowed(controlConfig, "", fixture.Timeframe, series) {
		t.Fatal("RouteAllowed rejected the explicit-empty symbol control")
	}
	shared, err := PrepareSharedRunContext(controlRequest)
	if err != nil {
		t.Fatalf("PrepareSharedRunContext: %v", err)
	}
	control := runRouteEntryShapePaths(t, shared, controlRequest, controlConfig, false)
	if got := tradeEntryIndexes(control.Trades); !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("explicit-empty control entries = %v, want %v", got, wantEntries)
	}

	invalidValues := []struct {
		name    string
		value   any
		missing bool
	}{
		{name: "missing", missing: true},
		{name: "nil", value: nil},
		{name: "number", value: 1},
		{name: "bool", value: true},
		{name: "map", value: map[string]any{}},
		{name: "slice", value: []any{}},
		{name: "named string", value: routeEntryNamedString("")},
	}
	for _, invalid := range invalidValues {
		t.Run(invalid.name, func(t *testing.T) {
			entry := map[string]any{"tf": fixture.Timeframe}
			if !invalid.missing {
				entry["symbol"] = invalid.value
			}
			cfg := cloneTestConfig(t, reviewedConfig)
			cfg["slices"] = []any{entry}
			if RouteAllowed(cfg, "", fixture.Timeframe, series) {
				t.Fatal("RouteAllowed admitted malformed symbol entry")
			}
			result := runRouteEntryShapePaths(t, shared, routeEntryShapeRequest(fixture, cfg), cfg, true)
			if result.TradeCount != 0 || len(result.Trades) != 0 {
				t.Fatalf("malformed route result = %d/%v, want zero", result.TradeCount, tradeEntryIndexes(result.Trades))
			}
		})
	}
}

func runRouteEntryShapePaths(t *testing.T, shared *SharedRunContext, request RunRequest, cfg dsl.Config, wantOffRoute bool) RunResult {
	t.Helper()
	publicResult, err := Run(request)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	prepared, err := PrepareRun(request)
	if err != nil {
		t.Fatalf("PrepareRun: %v", err)
	}
	if prepared.offRoute != wantOffRoute {
		t.Fatalf("PrepareRun offRoute = %v, want %v", prepared.offRoute, wantOffRoute)
	}
	preparedResult, err := prepared.RunChecked(request.Costs)
	if err != nil {
		t.Fatalf("PreparedRun.RunChecked: %v", err)
	}
	variant, err := shared.PrepareVariant(cfg)
	if err != nil {
		t.Fatalf("PrepareVariant: %v", err)
	}
	if variant.offRoute != wantOffRoute {
		t.Fatalf("PrepareVariant offRoute = %v, want %v", variant.offRoute, wantOffRoute)
	}
	variantResult, err := variant.RunChecked(request.Costs)
	if err != nil {
		t.Fatalf("PreparedVariant.RunChecked: %v", err)
	}
	if !reflect.DeepEqual(preparedResult, publicResult) || !reflect.DeepEqual(variantResult, publicResult) {
		t.Fatalf("checked paths differ\npublic: %s\nprepared: %s\nshared: %s", canonicalJSON(publicResult), canonicalJSON(preparedResult), canonicalJSON(variantResult))
	}
	return publicResult
}

func routeEntryShapeRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
	return RunRequest{
		Config:          cfg,
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          "",
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	}
}
