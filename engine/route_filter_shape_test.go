package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestCompiledRouteRawSymbolsAndTimeframes(t *testing.T) {
	type namedString string
	type namedStrings []string
	var nilStrings []string
	pointer := &[]string{"XAUUSD"}

	tests := []struct {
		name      string
		field     string
		present   bool
		raw       any
		symbol    string
		timeframe string
		want      bool
	}{
		{name: "missing symbols unrestricted", field: "symbols", symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols string slice match", field: "symbols", present: true, raw: []string{"XAUUSD"}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols any slice match", field: "symbols", present: true, raw: []any{"XAUUSD"}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols mixed retains match", field: "symbols", present: true, raw: []any{1, "XAUUSD", true}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols unknown rejects", field: "symbols", present: true, raw: []any{"EURUSD"}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "symbols all malformed rejects", field: "symbols", present: true, raw: []any{1, true, nil}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "symbols scalar exact", field: "symbols", present: true, raw: "XAUUSD", symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols named scalar exact", field: "symbols", present: true, raw: namedString("XAUUSD"), symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols scalar contains", field: "symbols", present: true, raw: "XAUUSD,EURUSD", symbol: "EURUSD", timeframe: "1h", want: true},
		{name: "symbols scalar partial rejects", field: "symbols", present: true, raw: "XAU", symbol: "XAUUSD", timeframe: "1h"},
		{name: "symbols scalar case sensitive", field: "symbols", present: true, raw: "xauusd", symbol: "XAUUSD", timeframe: "1h"},
		{name: "symbols empty string unrestricted", field: "symbols", present: true, raw: "", symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols empty slice unrestricted", field: "symbols", present: true, raw: []any{}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols typed nil slice unrestricted", field: "symbols", present: true, raw: nilStrings, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols empty array unrestricted", field: "symbols", present: true, raw: [0]string{}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols map unrestricted", field: "symbols", present: true, raw: map[string]any{"symbol": "EURUSD"}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols number unrestricted", field: "symbols", present: true, raw: 1, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols pointer unrestricted", field: "symbols", present: true, raw: pointer, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols nil unrestricted", field: "symbols", present: true, raw: nil, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "symbols named slice rejects", field: "symbols", present: true, raw: namedStrings{"XAUUSD"}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "symbols other slice rejects", field: "symbols", present: true, raw: []int{1}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "symbols array rejects", field: "symbols", present: true, raw: [1]string{"XAUUSD"}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "symbols empty request bypasses restriction", field: "symbols", present: true, raw: []any{1, true}, symbol: "", timeframe: "1h", want: true},

		{name: "missing timeframes unrestricted", field: "timeframes", symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes string slice match", field: "timeframes", present: true, raw: []string{"1h"}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes mixed retains match", field: "timeframes", present: true, raw: []any{1, "1h", true}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes unknown rejects", field: "timeframes", present: true, raw: []any{"15m"}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "timeframes all malformed rejects", field: "timeframes", present: true, raw: []any{1, true, nil}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "timeframes scalar exact", field: "timeframes", present: true, raw: "1h", symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes named scalar exact", field: "timeframes", present: true, raw: namedString("1h"), symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes scalar contains", field: "timeframes", present: true, raw: "15m,1h", symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes scalar contains empty candidate", field: "timeframes", present: true, raw: "15m,1h", symbol: "XAUUSD", timeframe: "", want: true},
		{name: "timeframes scalar mismatch rejects", field: "timeframes", present: true, raw: "15m", symbol: "XAUUSD", timeframe: "1h"},
		{name: "timeframes empty string unrestricted", field: "timeframes", present: true, raw: "", symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes empty slice unrestricted", field: "timeframes", present: true, raw: []any{}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes typed nil slice unrestricted", field: "timeframes", present: true, raw: nilStrings, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes empty array unrestricted", field: "timeframes", present: true, raw: [0]string{}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes map unrestricted", field: "timeframes", present: true, raw: map[string]any{"tf": "15m"}, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes number unrestricted", field: "timeframes", present: true, raw: 1, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes pointer unrestricted", field: "timeframes", present: true, raw: pointer, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes nil unrestricted", field: "timeframes", present: true, raw: nil, symbol: "XAUUSD", timeframe: "1h", want: true},
		{name: "timeframes named slice rejects", field: "timeframes", present: true, raw: namedStrings{"1h"}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "timeframes other slice rejects", field: "timeframes", present: true, raw: []int{1}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "timeframes array rejects", field: "timeframes", present: true, raw: [1]string{"1h"}, symbol: "XAUUSD", timeframe: "1h"},
		{name: "timeframes empty request does not bypass list", field: "timeframes", present: true, raw: []any{"1h"}, symbol: "XAUUSD", timeframe: ""},
		{name: "empty symbol bypass does not bypass timeframe", field: "symbols", present: true, raw: []any{1, true}, symbol: "", timeframe: "15m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := dsl.Config{}
			if tt.field == "symbols" {
				cfg["timeframes"] = []any{"1h"}
			} else {
				cfg["symbols"] = []any{"XAUUSD"}
			}
			if tt.present {
				cfg[tt.field] = tt.raw
			}
			if got := compiledRouteAllowed(cfg, tt.symbol, tt.timeframe); got != tt.want {
				t.Fatalf("compiledRouteAllowed(%s=%#v, %q, %q) = %v, want %v", tt.field, tt.raw, tt.symbol, tt.timeframe, got, tt.want)
			}
		})
	}
}

func TestCompiledRouteRawFallbackPreparationSnapshotsDecision(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-break-retest")

	preparedSymbols := []any{"XAUUSD"}
	preparedConfig := cloneTestConfig(t, reviewedConfig)
	preparedConfig["symbols"] = preparedSymbols
	preparedRequest := routeFilterShapeRequest(fixture, preparedConfig)
	prepared, err := PrepareRun(preparedRequest)
	if err != nil {
		t.Fatalf("PrepareRun: %v", err)
	}
	if prepared.offRoute {
		t.Fatal("prepared matching route is off-route")
	}
	preparedSymbols[0] = "EURUSD"
	if prepared.offRoute {
		t.Fatal("caller mutation changed prepared route decision")
	}
	freshPrepared, err := PrepareRun(preparedRequest)
	if err != nil {
		t.Fatalf("fresh PrepareRun: %v", err)
	}
	if !freshPrepared.offRoute {
		t.Fatal("fresh preparation did not observe caller route mutation")
	}

	sharedRequest := routeFilterShapeRequest(fixture, reviewedConfig)
	shared, err := PrepareSharedRunContext(sharedRequest)
	if err != nil {
		t.Fatalf("PrepareSharedRunContext: %v", err)
	}
	variantSymbols := []any{"XAUUSD"}
	variantConfig := cloneTestConfig(t, reviewedConfig)
	variantConfig["symbols"] = variantSymbols
	variant, err := shared.PrepareVariant(variantConfig)
	if err != nil {
		t.Fatalf("PrepareVariant: %v", err)
	}
	if variant.offRoute {
		t.Fatal("prepared matching variant is off-route")
	}
	variantSymbols[0] = "EURUSD"
	if variant.offRoute {
		t.Fatal("caller mutation changed prepared variant route decision")
	}
	freshVariant, err := shared.PrepareVariant(variantConfig)
	if err != nil {
		t.Fatalf("fresh PrepareVariant: %v", err)
	}
	if !freshVariant.offRoute {
		t.Fatal("fresh variant preparation did not observe caller route mutation")
	}
}

func TestCompiledRouteRawFallbackPreservesSlicesPrecedence(t *testing.T) {
	tests := []struct {
		name string
		cfg  dsl.Config
		want bool
	}{
		{
			name: "matching slice ignores malformed fallback filters",
			cfg: dsl.Config{
				"slices":     []any{map[string]any{"symbol": "XAUUSD", "tf": "1h"}},
				"symbols":    []any{1, true},
				"timeframes": "15m",
			},
			want: true,
		},
		{
			name: "malformed declared slice rejects matching fallback filters",
			cfg: dsl.Config{
				"slices":     []any{"not a route"},
				"symbols":    "XAUUSD",
				"timeframes": "1h",
			},
		},
		{
			name: "empty slice uses fallback filters",
			cfg: dsl.Config{
				"slices":     []any{},
				"symbols":    "XAU",
				"timeframes": "1h",
			},
		},
		{
			name: "unsupported slice container uses fallback filters",
			cfg: dsl.Config{
				"slices":     "not a list",
				"symbols":    "XAUUSD",
				"timeframes": "1h",
			},
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

func TestCompiledRouteRawFallbackAcrossCheckedExecutionPaths(t *testing.T) {
	fixture, reviewedConfig := loadEntryAttemptCase(t, "family-break-retest")
	baselineRequest := routeFilterShapeRequest(fixture, reviewedConfig)
	baseline, err := Run(baselineRequest)
	if err != nil {
		t.Fatalf("run reviewed baseline: %v", err)
	}
	baselineEntries := []int{315, 389, 590, 756, 933, 1074, 1093, 1412, 1442, 1580, 1649, 1695, 1987, 2475}
	if got := tradeEntryIndexes(baseline.Trades); !reflect.DeepEqual(got, baselineEntries) {
		t.Fatalf("baseline entries = %v, want %v", got, baselineEntries)
	}
	shared, err := PrepareSharedRunContext(baselineRequest)
	if err != nil {
		t.Fatalf("prepare reviewed shared context: %v", err)
	}

	tests := []struct {
		name      string
		field     string
		raw       any
		wantRoute bool
	}{
		{name: "symbols list on route", field: "symbols", raw: []any{"XAUUSD"}, wantRoute: true},
		{name: "symbols mixed on route", field: "symbols", raw: []any{1, "XAUUSD", true}, wantRoute: true},
		{name: "symbols scalar contains route", field: "symbols", raw: "XAUUSD,EURUSD", wantRoute: true},
		{name: "timeframes scalar contains route", field: "timeframes", raw: "15m,1h", wantRoute: true},
		{name: "symbols malformed off route", field: "symbols", raw: []any{1, true}},
		{name: "timeframes malformed off route", field: "timeframes", raw: []any{1, true}},
		{name: "symbols scalar off route", field: "symbols", raw: "XAU"},
		{name: "timeframes scalar off route", field: "timeframes", raw: "15m"},
	}
	series := marketdata.SeriesFromBars(fixture.Bars)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cloneTestConfig(t, reviewedConfig)
			cfg[tt.field] = tt.raw
			if got := RouteAllowed(cfg, fixture.Symbol, fixture.Timeframe, series); got != tt.wantRoute {
				t.Fatalf("RouteAllowed() = %v, want %v", got, tt.wantRoute)
			}
			result := runRouteFilterShapePaths(t, shared, routeFilterShapeRequest(fixture, cfg), cfg)
			if tt.wantRoute {
				if !reflect.DeepEqual(result, baseline) {
					t.Fatalf("on-route result differs from reviewed baseline\n got: %s\nwant: %s", canonicalJSON(result), canonicalJSON(baseline))
				}
				return
			}
			if result.TradeCount != 0 || len(result.Trades) != 0 {
				t.Fatalf("off-route result = %d/%v, want zero", result.TradeCount, tradeEntryIndexes(result.Trades))
			}
		})
	}
}

func runRouteFilterShapePaths(t *testing.T, shared *SharedRunContext, request RunRequest, cfg dsl.Config) RunResult {
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

func routeFilterShapeRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
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
