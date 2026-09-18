package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func sourceProjectionSeries(start, step float64, count int) marketdata.Series {
	bars := make([]marketdata.Bar, count)
	for i := range bars {
		value := float64(100 + i)
		bars[i] = marketdata.Bar{T: start + float64(i)*step, O: value, H: value + 1, L: value - 1, C: value, V: 1}
	}
	return marketdata.SeriesFromBars(bars)
}

func TestRunFixtureLoadsExplicitSourceSeriesSeparately(t *testing.T) {
	fixturePath := filepath.Join(t.TempDir(), "source.fixture.json")
	raw := map[string]any{
		"schema": runFixtureSchema, "case": "source-fixture", "strategyId": "source", "symbol": "XAUUSD",
		"timeframe": "15m", "sourceTimeframe": "1h", "rangeMethod": "zone",
		"bars":          [][]float64{{0, 1, 2, 0, 1, 1}},
		"sourceBars":    [][]float64{{0, 10, 11, 9, 10, 1}},
		"sourceHtfBars": [][]float64{{0, 20, 21, 19, 20, 1}},
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(fixturePath, encoded, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	fixture, err := LoadRunFixture(fixturePath)
	if err != nil {
		t.Fatalf("LoadRunFixture: %v", err)
	}
	if fixture.SourceTimeframe != "1h" || len(fixture.SourceBars) != 1 || fixture.SourceBars[0].C != 10 || len(fixture.SourceHTFBars) != 1 {
		t.Fatalf("loaded source fixture = %+v", fixture)
	}
}

func TestSourceProjectionUsesOnlyCompletedSourceBarsAndKeepsChartLength(t *testing.T) {
	const minute = float64(60 * 1000)
	chart := sourceProjectionSeries(0, 15*minute, 8)
	source := sourceProjectionSeries(0, 60*minute, 2)
	indexes := sourceProjectionIndexes(chart, source)
	want := []int{-1, -1, -1, 0, 0, 0, 0, 1}
	for i := range want {
		if indexes[i] != want[i] {
			t.Fatalf("projection index %d = %d, want %d", i, indexes[i], want[i])
		}
	}
	cols := contextcols.Build(source, contextcols.Options{})
	projected := projectSourceColumns(chart, source, cols)
	if projected.Length != chart.Len() || len(projected.ATR) != chart.Len() {
		t.Fatalf("projected columns are not chart-length: length=%d atr=%d chart=%d", projected.Length, len(projected.ATR), chart.Len())
	}
	if projected.ATR[0] == projected.ATR[0] {
		t.Fatal("forming source bar must fail closed, got a finite ATR")
	}
	if projected.ATR[3] != cols.ATR[0] {
		t.Fatalf("first completed source ATR = %v, want %v", projected.ATR[3], cols.ATR[0])
	}

	trend := []int8{trendDown, trendUp}
	projectedTrend := projectSourceInt8(chart, source, trend)
	if projectedTrend[0] != htfUnavailable || projectedTrend[3] != trendDown || projectedTrend[7] != trendUp {
		t.Fatalf("causal source HTF projection = %v, want unavailable/down/up at boundaries", projectedTrend)
	}
}

func TestSharedContextIdentityAndRequestDoNotLeakExplicitSourceTimeframes(t *testing.T) {
	chart := sourceProjectionSeries(0, 15*60*1000, 8)
	source := sourceProjectionSeries(0, 60*60*1000, 2)
	cfg := dsl.Config{"setupType": string(dsl.FamilyFlagContinuation), "sourceTimeframe": "1h"}
	request := RunRequest{Config: cfg, Series: chart, SourceSeries: source, SourceTimeframe: "1h", Timeframe: "15m"}
	key, err := SharedContextKey(request)
	if err != nil {
		t.Fatalf("SharedContextKey: %v", err)
	}
	other := request
	other.SourceTimeframe = "4h"
	if _, err := SharedContextKey(other); err == nil || err.Error() != "source timeframe does not match strategy configuration" {
		t.Fatalf("mismatched source timeframe error = %v", err)
	}
	if key == "" {
		t.Fatal("source-aware shared context identity is empty")
	}
	missing := request
	missing.SourceSeries = marketdata.Series{}
	if _, err := PrepareSharedRunContext(missing); err == nil || err.Error() != "source market series is required for explicit source timeframe" {
		t.Fatalf("missing explicit source series error = %v", err)
	}
}

func TestSourceEntryRunRequestsExecuteAndSharedGridFailsClosed(t *testing.T) {
	for _, route := range []struct {
		source, entry         string
		sourceStep, entryStep float64
	}{
		{"4h", "30m", 4 * 60 * 60 * 1000, 30 * 60 * 1000},
		{"1h", "5m", 60 * 60 * 1000, 5 * 60 * 1000},
		{"15m", "1m", 15 * 60 * 1000, 60 * 1000},
	} {
		chart := sourceProjectionSeries(0, route.entryStep, 40)
		source := sourceProjectionSeries(0, route.sourceStep, 3)
		request := RunRequest{Config: dsl.Config{"setupType": string(dsl.FamilyFlagContinuation), "sourceTimeframe": route.source, "entryTf": route.entry}, Series: chart, SourceSeries: source, Symbol: "XAUUSD", Timeframe: route.entry, SourceTimeframe: route.source}
		if _, err := Run(request); err != nil {
			t.Fatalf("%s -> %s RunRequest: %v", route.source, route.entry, err)
		}
		if _, err := PrepareSharedRunContext(request); err == nil {
			t.Fatalf("%s -> %s shared grid must fail closed", route.source, route.entry)
		}
		missingSource := request
		missingSource.SourceSeries = marketdata.Series{}
		if _, err := PrepareRun(missingSource); err == nil {
			t.Fatalf("%s -> %s must reject missing source", route.source, route.entry)
		}
	}
}

func TestCanonicalC5FixtureRunsParsedDSLOnSourceBarsButKeepsChartRoute(t *testing.T) {
	const hour = float64(60 * 60 * 1000)
	sourceBars := make([]marketdata.Bar, 21)
	for i := range sourceBars {
		timestamp := 2*hour + float64(i)*4*hour
		sourceBars[i] = marketdata.Bar{T: timestamp, O: 100, H: 100.2, L: 99.8, C: 100, V: 1}
	}
	// A source-timeframe flag ends at index 19. The prior completed day high
	// is 103; the source setup breaks it only on its final 4h bar.
	for index, values := range map[int][4]float64{
		14: {100, 101, 100, 101}, 15: {101, 102, 101, 102}, 16: {102, 103, 102, 103},
		17: {103, 103, 102.9, 102.9}, 18: {102.9, 102.9, 102.8, 102.8},
		19: {102.8, 103.6, 102.8, 103.5}, 20: {103.5, 103.6, 103.4, 103.5},
	} {
		sourceBars[index].O, sourceBars[index].H, sourceBars[index].L, sourceBars[index].C = values[0], values[1], values[2], values[3]
	}
	chartBars := make([]marketdata.Bar, 360)
	for i := range chartBars {
		timestamp := 2*hour + float64(i)*15*60*1000
		chartBars[i] = marketdata.Bar{T: timestamp, O: 103.5, H: 103.6, L: 103.3, C: 103.5, V: 1}
	}
	// The completed source setup is scheduled on the later chart stream, and
	// the following chart bar closes the scheduled long at target.
	chartBars[321].H = 105
	fixture := RunFixture{
		Case: "c5-parsed-route", StrategyID: "c5", Symbol: "XAUUSD", Timeframe: "15m",
		SourceTimeframe: "4h", RangeMethod: "zone", Bars: chartBars, SourceBars: sourceBars,
	}
	source := `dsl v7
strategy "C5 parsed route" { description "source setup uses chart route" }
market conditions { slices(XAUUSD 15m) day type in (trending) or movement below 1 }
setup {
  type: flag continuation
  source timeframe 4h
  pole at least 0.1 ATR over 3 candles
  flag 3 to 3 candles
  movement between 0 and 1
}
entryTf 15m`
	result, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatalf("RunFixtureCase: %v", err)
	}
	if len(result.Trades) != 1 {
		t.Fatalf("parsed C5 source setup trades = %d, want 1", len(result.Trades))
	}
	trade := result.Trades[0]
	if trade.EntryIndex != 320 || trade.ExitIndex != 321 || trade.Reason != "tp" {
		t.Fatalf("scheduled parsed C5 trade = %+v, want entry 320 / exit 321 target", trade)
	}
}
