package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func routeMinuteSeries(minutes int, count int) marketdata.Series {
	bars := make([]marketdata.Bar, count)
	for i := range bars {
		bars[i] = marketdata.Bar{T: float64(i * minutes * 60_000), O: 100, H: 101, L: 99, C: 100, V: 1}
	}
	return marketdata.SeriesFromBars(bars)
}

func TestCompiledRouteAllowedSymbolsTimeframesFallback(t *testing.T) {
	cfg := dsl.Config{"symbols": []any{"XAUUSD"}, "timeframes": []any{"1h", "15m"}}

	if !compiledRouteAllowed(cfg, "XAUUSD", "1h") {
		t.Fatal("symbols()+timeframes() match should be allowed")
	}
	if compiledRouteAllowed(cfg, "EURUSD", "1h") {
		t.Fatal("symbols() mismatch should be blocked")
	}
	if !compiledRouteAllowed(cfg, "", "1h") {
		t.Fatal("empty symbol bypasses the symbols() check (JS behavior)")
	}
	if compiledRouteAllowed(cfg, "XAUUSD", "4h") {
		t.Fatal("timeframes() mismatch should be blocked")
	}

	tfOnly := dsl.Config{"timeframes": []any{"15m"}}
	if compiledRouteAllowed(tfOnly, "ANY", "1h") {
		t.Fatal("timeframes()-only mismatch should be blocked")
	}
	if !compiledRouteAllowed(tfOnly, "ANY", "15m") {
		t.Fatal("timeframes()-only match should be allowed")
	}

	open := dsl.Config{}
	if !compiledRouteAllowed(open, "ANYTHING", "1d") {
		t.Fatal("config without route restrictions should allow every route")
	}

	// slices() still wins over the fallback branches and stays verbatim-match.
	sliced := dsl.Config{
		"slices":     []any{map[string]any{"symbol": "XAUUSD", "tf": "4h"}},
		"timeframes": []any{"1h"},
	}
	if !compiledRouteAllowed(sliced, "XAUUSD", "4h") {
		t.Fatal("declared slice should be allowed regardless of timeframes()")
	}
	if compiledRouteAllowed(sliced, "XAUUSD", "1h") {
		t.Fatal("slices() must gate even when timeframes() would allow the route")
	}
}

func TestRouteAllowedInfersTimeframeFromBarSpacing(t *testing.T) {
	cfg := dsl.Config{"timeframes": []any{"15m"}}
	if !RouteAllowed(cfg, "XAUUSD", "", routeMinuteSeries(15, 50)) {
		t.Fatal("blank timeframe should infer 15m from bar spacing")
	}
	if RouteAllowed(cfg, "XAUUSD", "", routeMinuteSeries(60, 50)) {
		t.Fatal("blank timeframe over 1h bars should not match timeframes(15m)")
	}
	hourCfg := dsl.Config{"timeframes": []any{"1h"}}
	if !RouteAllowed(hourCfg, "XAUUSD", "", routeMinuteSeries(60, 50)) {
		t.Fatal("blank timeframe should infer 1h from bar spacing")
	}
}

// timeframes() market conditions must gate fixture runs the same way the JS
// runtime's specSliceAllowed does: family-break-retest declares
// timeframes(15m, 1h), so its 1h fixture trades on-route and produces zero
// trades once the timeframe label moves off the declared list.
func TestRunFixtureCaseBlocksOffTimeframeFallback(t *testing.T) {
	const caseName = "family-break-retest"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}

	onRoute, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("on-route run: %v", err)
	}
	if len(onRoute.Trades) == 0 {
		t.Fatal("on-route fixture should still produce trades")
	}

	offTF := fixture
	offTF.Timeframe = "4h"
	blocked, err := RunFixtureCase(offTF, string(source))
	if err != nil {
		t.Fatalf("off-timeframe run: %v", err)
	}
	if len(blocked.Trades) != 0 {
		t.Fatalf("off-timeframe run produced %d trades, want 0", len(blocked.Trades))
	}
}
