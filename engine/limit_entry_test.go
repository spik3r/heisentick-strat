package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestFailedBreakoutParsedLimitEntryQueuesAtSweptEdge(t *testing.T) {
	fixture, source := loadLevelSweepLimitCase(t)
	parsed := parseEngineTestDSL(t, strings.Replace(
		source,
		"entry {",
		"entry {\n  enter with limit at swept edge within 5 candles",
		1,
	))
	params := paramsFromConfig(parsed.Config)
	if params.EntryMode != "limit" || params.EntryExpireCandles != 5 {
		t.Fatalf("entry params = mode %q expiry %d, want limit/5", params.EntryMode, params.EntryExpireCandles)
	}

	b, index := runUntilLevelSweepAdmission(t, fixture, parsed.Config, true)
	if b.hasPosition || len(b.limitOrders) != 1 {
		t.Fatalf("limit admission at %d: position=%v limits=%+v", index, b.hasPosition, b.limitOrders)
	}
	order := b.limitOrders[0]
	wantLimit := order.Meta["rangeLo"].(float64)
	if order.Side == sideShort {
		wantLimit = order.Meta["rangeHi"].(float64)
	}
	if order.Limit != wantLimit || order.PlacedAt != index || order.ExpireAt != index+5 {
		t.Fatalf("queued order = %+v, want swept edge %v placed %d expiring %d", order, wantLimit, index, index+5)
	}
	wantTag := "DSL:" + order.Meta["levelKey"].(string) + ":" + order.Meta["grade"].(string)
	if order.Tag != wantTag || order.Meta["setup"] != "rangeSweepReclaim" {
		t.Fatalf("queued identity = tag %q meta %#v, want tag %q with range-sweep metadata", order.Tag, order.Meta, wantTag)
	}
}

func TestArchivedFailedBreakoutLimitEntryParams(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "strategies", "source", "dslFailedBreakoutLimitEntry.strat"))
	if err != nil {
		t.Fatalf("read archived limit-entry DSL: %v", err)
	}
	parsed := parseEngineTestDSL(t, string(source))
	params := paramsFromConfig(parsed.Config)
	if params.SetupType != "failedBreakout" || params.EntryMode != "limit" || params.EntryExpireCandles != 5 {
		t.Fatalf("archived strategy params = family %q mode %q expiry %d, want failedBreakout/limit/5", params.SetupType, params.EntryMode, params.EntryExpireCandles)
	}
}

func TestFailedBreakoutMarketEntryRemainsImmediate(t *testing.T) {
	fixture, source := loadLevelSweepLimitCase(t)
	parsed := parseEngineTestDSL(t, source)
	params := paramsFromConfig(parsed.Config)
	if params.EntryMode != "market" || params.EntryExpireCandles != 5 {
		t.Fatalf("default entry params = mode %q expiry %d, want market/5", params.EntryMode, params.EntryExpireCandles)
	}

	b, index := runUntilLevelSweepAdmission(t, fixture, parsed.Config, false)
	if !b.hasPosition || len(b.limitOrders) != 0 {
		t.Fatalf("market admission at %d: position=%v limits=%+v", index, b.hasPosition, b.limitOrders)
	}
	if got, want := b.position.Entry, b.series.C[index]+float64(b.position.Side)*b.costs.Slippage; got != want {
		t.Fatalf("market entry = %v, want signal close with slip %v", got, want)
	}
}

func TestEntryModeNormalizationPreservesExplicitExpiry(t *testing.T) {
	for _, mode := range []string{"limit", "limitSweptEdge"} {
		t.Run(mode, func(t *testing.T) {
			params := paramsFromConfig(dsl.Config{
				"entryMode": map[string]any{"type": mode, "expireCandles": 0},
			})
			if params.EntryMode != "limit" || params.EntryExpireCandles != 0 {
				t.Fatalf("entry params = mode %q expiry %d, want limit/0", params.EntryMode, params.EntryExpireCandles)
			}
		})
	}
}

func TestLimitOrderFillAndExpiryParity(t *testing.T) {
	tests := []struct {
		name        string
		series      marketdata.Series
		expiry      int
		process     []int
		wantOpen    bool
		wantEntry   float64
		wantPending bool
	}{
		{
			name: "exact edge fill has no entry slippage",
			series: limitTestSeries(
				[]float64{100, 100}, []float64{101, 104}, []float64{99, 99}, []float64{100, 101},
			),
			expiry: 5, process: []int{1}, wantOpen: true, wantEntry: 103,
		},
		{
			name: "better gap open has no entry slippage",
			series: limitTestSeries(
				[]float64{100, 105}, []float64{101, 107}, []float64{99, 104}, []float64{100, 106},
			),
			expiry: 5, process: []int{1}, wantOpen: true, wantEntry: 105,
		},
		{
			name: "fillable expiry bar fills before expiry",
			series: limitTestSeries(
				[]float64{100, 100, 100}, []float64{101, 102, 104}, []float64{99, 99, 99}, []float64{100, 101, 101},
			),
			expiry: 2, process: []int{1, 2}, wantOpen: true, wantEntry: 103,
		},
		{
			name: "unfilled order expires",
			series: limitTestSeries(
				[]float64{100, 100, 100}, []float64{101, 102, 102}, []float64{99, 99, 99}, []float64{100, 101, 101},
			),
			expiry: 2, process: []int{1, 2},
		},
		{
			name: "explicit zero expires on first eligible bar",
			series: limitTestSeries(
				[]float64{100, 100}, []float64{101, 102}, []float64{99, 99}, []float64{100, 101},
			),
			expiry: 0, process: []int{1},
		},
		{
			name: "final bar placement remains unfilled",
			series: limitTestSeries(
				[]float64{100}, []float64{104}, []float64{99}, []float64{101},
			),
			expiry: 5, wantPending: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := limitTestBroker(tt.series)
			meta := TradeMeta{"setup": "rangeSweepReclaim", "edge": "high"}
			setup := setupPlan{Side: sideShort, Stop: 110, Target: 90, Tag: "DSL:PDH:A", Meta: meta}
			b.enterLimit(0, 103, setup, tt.expiry)
			for _, index := range tt.process {
				b.fillLimits(index)
			}

			if b.hasPosition != tt.wantOpen {
				t.Fatalf("position open = %v, want %v; limits=%+v", b.hasPosition, tt.wantOpen, b.limitOrders)
			}
			if tt.wantOpen {
				if math.Abs(b.position.Entry-tt.wantEntry) > 1e-12 {
					t.Fatalf("entry = %v, want %v", b.position.Entry, tt.wantEntry)
				}
				if b.position.Tag != setup.Tag || b.position.Meta["setup"] != meta["setup"] || b.position.Meta["edge"] != meta["edge"] {
					t.Fatalf("filled identity = tag %q meta %#v, want %q %#v", b.position.Tag, b.position.Meta, setup.Tag, meta)
				}
			}
			if got := len(b.limitOrders); (got == 1) != tt.wantPending {
				t.Fatalf("pending limits = %d, want pending %v", got, tt.wantPending)
			}
		})
	}
}

func loadLevelSweepLimitCase(t *testing.T) (RunFixture, string) {
	t.Helper()
	const caseName = "family-level-sweep"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	return fixture, string(source)
}

func parseEngineTestDSL(t *testing.T, source string) dsl.ParseResult {
	t.Helper()
	parsed, err := dsl.Parse(source)
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) > 0 {
		t.Fatalf("parse DSL errors: %v", parsed.Errors)
	}
	return parsed
}

func runUntilLevelSweepAdmission(t *testing.T, fixture RunFixture, cfg dsl.Config, wantLimit bool) (broker, int) {
	t.Helper()
	runner := newPreparedRunner(fixture, cfg)
	runner.broker.reset(runner.series, runner.cols, runner.htfTrend, runner.ema, runner.emaSlope, runner.params, fixture, runner.trades)
	for i := 0; i < runner.series.Len(); i++ {
		runner.broker.fillPending(i)
		runner.broker.fillLimits(i)
		runner.broker.resolveIntrabarExit(i)
		runner.broker.onBar(i)
		if (wantLimit && len(runner.broker.limitOrders) > 0) || (!wantLimit && runner.broker.hasPosition) {
			return runner.broker, i
		}
	}
	t.Fatalf("level-sweep %s admission never occurred", map[bool]string{true: "limit", false: "market"}[wantLimit])
	return broker{}, -1
}

func limitTestSeries(opens, highs, lows, closes []float64) marketdata.Series {
	times := make([]float64, len(opens))
	for i := range times {
		times[i] = float64(i) * float64(contextHourMS)
	}
	return marketdata.Series{T: times, O: opens, H: highs, L: lows, C: closes, V: make([]float64, len(opens))}
}

func limitTestBroker(series marketdata.Series) broker {
	return broker{
		series: series,
		costs:  Costs{Slippage: 0.5},
		params: flagParams{AdmitAsiaWindow: true, AdmitMidWindow: true},
	}
}

const contextHourMS = 60 * 60 * 1000
