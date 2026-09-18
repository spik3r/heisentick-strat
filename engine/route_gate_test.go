package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestCompiledRouteAllowedNormalizesSliceContainers(t *testing.T) {
	configs := map[string]dsl.Config{
		"parser []any": {
			"slices": []any{
				map[string]any{"symbol": "XAUUSD", "tf": "1h"},
				map[string]any{"symbol": "EURUSD", "tf": "4h"},
			},
		},
		"direct []map": {
			"slices": []map[string]any{
				{"symbol": "XAUUSD", "tf": "1h"},
				{"symbol": "EURUSD", "tf": "4h"},
			},
		},
		"direct []dsl.Config": {
			"slices": []dsl.Config{
				{"symbol": "XAUUSD", "tf": "1h"},
				{"symbol": "EURUSD", "tf": "4h"},
			},
		},
		"mixed []any": {
			"slices": []any{
				map[string]any{"symbol": "XAUUSD", "tf": "1h"},
				"not a route",
				dsl.Config{"symbol": "EURUSD", "tf": "4h"},
			},
		},
	}

	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			if !compiledRouteAllowed(cfg, "XAUUSD", "1h") {
				t.Fatal("first declared route should be allowed")
			}
			if !compiledRouteAllowed(cfg, "EURUSD", "4h") {
				t.Fatal("second declared route should be allowed")
			}
			if compiledRouteAllowed(cfg, "XAUUSD", "4h") {
				t.Fatal("cross-product route should be rejected")
			}
		})
	}
}

func TestCompiledRouteSliceAcceptsPlainAndDSLConfigWithoutCopyingMaps(t *testing.T) {
	type unrelatedMap map[string]any
	tests := []struct {
		name   string
		source any
		want   bool
	}{
		{name: "plain map", source: map[string]any{"symbol": "XAUUSD"}, want: true},
		{name: "dsl.Config", source: dsl.Config{"symbol": "XAUUSD"}, want: true},
		{name: "unrelated named map", source: unrelatedMap{"symbol": "XAUUSD"}, want: false},
		{name: "scalar", source: "XAUUSD", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routeSlice, ok := compiledRouteSlice(tt.source)
			if ok != tt.want {
				t.Fatalf("compiledRouteSlice(%T) accepted = %v, want %v", tt.source, ok, tt.want)
			}
			if !tt.want {
				return
			}

			routeSlice["throughResult"] = true
			switch source := tt.source.(type) {
			case map[string]any:
				if source["throughResult"] != true {
					t.Fatalf("plain source did not observe result mutation: %v", source)
				}
				source["throughSource"] = true
			case dsl.Config:
				if source["throughResult"] != true {
					t.Fatalf("named source did not observe result mutation: %v", source)
				}
				source["throughSource"] = true
			}
			if routeSlice["throughSource"] != true {
				t.Fatalf("result did not observe source mutation: %v", routeSlice)
			}
		})
	}
}

func TestCompiledRouteSlicesPreservesDirectPlainMapSlice(t *testing.T) {
	direct := []map[string]any{{"symbol": "XAUUSD", "tf": "1h"}}
	normalized, declared := compiledRouteSlices(direct)
	if !declared || len(normalized) != 1 {
		t.Fatalf("compiledRouteSlices() = %v, %v, want one declared route", normalized, declared)
	}
	replacement := map[string]any{"symbol": "EURUSD", "tf": "4h"}
	normalized[0] = replacement
	if direct[0]["symbol"] != "EURUSD" || direct[0]["tf"] != "4h" {
		t.Fatalf("direct slice did not observe result element replacement: %v", direct)
	}
}

func TestCompiledRouteAllowedMalformedAndEmptySlicesKeepFallbackSemantics(t *testing.T) {
	type namedRouteSlices []map[string]any
	tests := []struct {
		name string
		cfg  dsl.Config
		want bool
	}{
		{
			name: "malformed entry remains authoritative",
			cfg: dsl.Config{
				"slices":     []any{"not a route"},
				"timeframes": []any{"1h"},
			},
			want: false,
		},
		{
			name: "malformed container uses fallback",
			cfg: dsl.Config{
				"slices":     "not a list",
				"timeframes": []any{"1h"},
			},
			want: true,
		},
		{
			name: "empty parser container uses fallback",
			cfg: dsl.Config{
				"slices":     []any{},
				"timeframes": []any{"1h"},
			},
			want: true,
		},
		{
			name: "empty direct container uses fallback",
			cfg: dsl.Config{
				"slices":     []map[string]any{},
				"timeframes": []string{"1h"},
			},
			want: true,
		},
		{
			name: "empty named container uses fallback",
			cfg: dsl.Config{
				"slices":     []dsl.Config{},
				"timeframes": []string{"1h"},
			},
			want: true,
		},
		{
			name: "nonempty named container remains authoritative",
			cfg: dsl.Config{
				"slices":     []dsl.Config{nil},
				"timeframes": []string{"1h"},
			},
			want: false,
		},
		{
			name: "nonempty mixed container remains authoritative",
			cfg: dsl.Config{
				"slices":     []any{dsl.Config(nil), "not a route"},
				"timeframes": []string{"1h"},
			},
			want: false,
		},
		{
			name: "unsupported named container uses fallback",
			cfg: dsl.Config{
				"slices":     namedRouteSlices{{"symbol": "EURUSD", "tf": "4h"}},
				"timeframes": []string{"1h"},
			},
			want: true,
		},
		{
			name: "missing slices uses fallback",
			cfg:  dsl.Config{"timeframes": []string{"1h"}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compiledRouteAllowed(tt.cfg, "XAUUSD", "1h"); got != tt.want {
				t.Fatalf("compiledRouteAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompiledSlicesGateCrossProductRoute(t *testing.T) {
	const caseName = "family-supply-demand"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}

	// GBPUSD and 1h are each routed elsewhere, but the crossed pair is not.
	fixture.Symbol = "GBPUSD"
	fixture.Timeframe = "1h"

	t.Run("conformance fixture", func(t *testing.T) {
		result, err := RunFixtureCase(fixture, string(source))
		if err != nil {
			t.Fatalf("run fixture: %v", err)
		}
		if result.TradeCount != 0 {
			t.Fatalf("off-route trade count = %d, want 0", result.TradeCount)
		}
	})

	t.Run("direct engine", func(t *testing.T) {
		result, err := Run(RunRequest{
			Config:          parsed.Config,
			Series:          marketdata.SeriesFromBars(fixture.Bars),
			HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
			StrategyID:      fixture.StrategyID,
			Symbol:          fixture.Symbol,
			Timeframe:       fixture.Timeframe,
			HigherTimeframe: fixture.HigherTimeframe,
			RangeMethod:     fixture.RangeMethod,
			Costs:           fixture.Costs,
		})
		if err != nil {
			t.Fatalf("run engine: %v", err)
		}
		if result.TradeCount != 0 {
			t.Fatalf("off-route trade count = %d, want 0", result.TradeCount)
		}
	})
}

func TestSliceContainersMatchAcrossPublicAndPreparedRunPaths(t *testing.T) {
	const caseName = "family-break-retest"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) > 0 {
		t.Fatalf("DSL parse errors: %v", parsed.Errors)
	}

	containers := []struct {
		name  string
		value any
	}{
		{
			name: "parser []any",
			value: []any{
				map[string]any{"symbol": fixture.Symbol, "tf": fixture.Timeframe},
			},
		},
		{
			name: "direct []map",
			value: []map[string]any{
				{"symbol": fixture.Symbol, "tf": fixture.Timeframe},
			},
		},
		{
			name: "direct []dsl.Config",
			value: []dsl.Config{
				{"symbol": fixture.Symbol, "tf": fixture.Timeframe},
			},
		},
		{
			name: "mixed []any with dsl.Config",
			value: []any{
				"not a route",
				dsl.Config{"symbol": fixture.Symbol, "tf": fixture.Timeframe},
			},
		},
	}
	series := marketdata.SeriesFromBars(fixture.Bars)
	htfSeries := marketdata.SeriesFromBars(fixture.HTFBars)
	resultByRoute := make(map[string]string)

	for _, container := range containers {
		t.Run(container.name, func(t *testing.T) {
			cfg := make(dsl.Config, len(parsed.Config)+1)
			for key, value := range parsed.Config {
				cfg[key] = value
			}
			cfg["slices"] = container.value

			for _, route := range []struct {
				name           string
				symbol         string
				wantRoute      bool
				wantTradeCount int
			}{
				{name: "on route", symbol: fixture.Symbol, wantRoute: true, wantTradeCount: 13},
				{name: "off route", symbol: "EURUSD", wantRoute: false, wantTradeCount: 0},
			} {
				t.Run(route.name, func(t *testing.T) {
					if got := RouteAllowed(cfg, route.symbol, fixture.Timeframe, series); got != route.wantRoute {
						t.Fatalf("RouteAllowed() = %v, want %v", got, route.wantRoute)
					}

					request := RunRequest{
						Config:          cfg,
						Series:          series,
						HTFSeries:       htfSeries,
						StrategyID:      fixture.StrategyID,
						Symbol:          route.symbol,
						Timeframe:       fixture.Timeframe,
						HigherTimeframe: fixture.HigherTimeframe,
						RangeMethod:     fixture.RangeMethod,
						Costs:           fixture.Costs,
					}
					prepared, err := PrepareRun(request)
					if err != nil {
						t.Fatalf("PrepareRun: %v", err)
					}
					if got, want := prepared.offRoute, !route.wantRoute; got != want {
						t.Fatalf("PrepareRun offRoute = %v, want %v", got, want)
					}
					preparedResult := prepared.Run(request.Costs)
					if preparedResult.TradeCount != route.wantTradeCount {
						t.Fatalf("prepared trade count = %d, want %d", preparedResult.TradeCount, route.wantTradeCount)
					}
					preparedJSON := canonicalJSON(preparedResult)
					if want, ok := resultByRoute[route.name]; ok && preparedJSON != want {
						t.Fatalf("container result differs from equivalent config\n got: %s\nwant: %s", preparedJSON, want)
					}
					resultByRoute[route.name] = preparedJSON

					directResult, err := Run(request)
					if err != nil {
						t.Fatalf("Run: %v", err)
					}
					if got := canonicalJSON(directResult); got != preparedJSON {
						t.Fatalf("direct result differs from prepared result\n got: %s\nwant: %s", got, preparedJSON)
					}

					shared, err := PrepareSharedRunContext(request)
					if err != nil {
						t.Fatalf("PrepareSharedRunContext: %v", err)
					}
					variant, err := shared.PrepareVariant(cfg)
					if err != nil {
						t.Fatalf("PrepareVariant: %v", err)
					}
					if got, want := variant.offRoute, !route.wantRoute; got != want {
						t.Fatalf("PrepareVariant offRoute = %v, want %v", got, want)
					}
					variantResult := variant.Run(request.Costs)
					if got := canonicalJSON(variantResult); got != preparedJSON {
						t.Fatalf("variant result differs from prepared result\n got: %s\nwant: %s", got, preparedJSON)
					}
				})
			}
		})
	}
}
