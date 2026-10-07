package engine

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func downShockSourceRequest(t *testing.T, f RunFixture, source string) RunRequest {
	t.Helper()
	parsed, err := dsl.Parse(source)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	return RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(f.Bars),
		SourceSeries: marketdata.SeriesFromBars(f.SourceBars), StrategyID: f.StrategyID,
		Symbol: f.Symbol, Timeframe: f.Timeframe, SourceTimeframe: f.SourceTimeframe,
		RangeMethod: f.RangeMethod, Costs: f.Costs}
}

func inventedDownShockFixture() RunFixture {
	chart, source := testsupport.DownShockBars()
	return RunFixture{Schema: "dsl-conformance-run-fixture-v1", Case: "invented-down-shock",
		StrategyID: "inventedDownShock", Symbol: "XAUUSD", Timeframe: "1m", SourceTimeframe: "15m",
		RangeMethod: "zone", Costs: Costs{FillOn: "close", StartEquity: 10000}, Bars: chart, SourceBars: source}
}

func TestDownShockAdmittedSourceMatchesFixture(t *testing.T) {
	f := inventedDownShockFixture()
	want, err := RunFixtureCase(f, testsupport.DownShockSource)
	if err != nil || want.TradeCount != 1 {
		t.Fatalf("positive fixture: %+v, %v", want, err)
	}
	trade := want.Trades[0]
	if trade.Side != "long" || trade.EntryIndex != 1 || trade.EntryT != f.Bars[1].T || trade.Entry != 98.2 || trade.Reason != "tp" ||
		math.Abs(trade.Exit-98.32766) > 1e-12 || math.Abs(trade.PnL-37.2341666666695) > 1e-10 ||
		trade.Meta["sourceRow"] != float64(460) || trade.Meta["sourceCloseT"] != f.Bars[1].T {
		t.Fatalf("unexpected source-close trade: %+v", trade)
	}
	r := downShockSourceRequest(t, f, testsupport.DownShockSource)
	got, err := Run(r)
	if err != nil || !reflect.DeepEqual(got.Trades, want.Trades) {
		t.Fatalf("direct run: %+v, %v; fixture trades: %+v", got, err, want.Trades)
	}
	// Optional volume is admitted by the columnar API. The family must consume
	// those columns directly rather than reconstructing source rows.
	withoutVolume := r
	withoutVolume.SourceSeries.V = nil
	got, err = Run(withoutVolume)
	if err != nil || !reflect.DeepEqual(got.Trades, want.Trades) {
		t.Fatalf("source without volume: %+v, %v", got, err)
	}
	prepared, err := PrepareRun(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, slip := range []float64{0, 0.06, 0.12, 0} {
		costs := f.Costs
		costs.Slippage = slip
		f.Costs = costs
		fixture, err := RunFixtureCase(f, testsupport.DownShockSource)
		if err != nil {
			t.Fatal(err)
		}
		got, err := prepared.RunChecked(costs)
		if err != nil || got.TradeCount != 1 || !reflect.DeepEqual(got.Trades, fixture.Trades) {
			t.Fatalf("prepared costs %+v: %+v, %v; fixture: %+v", costs, got, err, fixture)
		}
	}
	// The legacy broker-only caller still consumes fixture source rows.
	var b broker
	b.reset(r.Series, contextcols.Columns{}, nil, nil, nil, paramsFromConfig(r.Config), inventedDownShockFixture(), nil)
	legacy, handled := b.runSpecialSetup()
	legacyResult, err := checkedResultEnvelope(inventedDownShockFixture(), legacy)
	if !handled || err != nil || !reflect.DeepEqual(legacyResult.Trades, want.Trades) {
		t.Fatalf("fixture-only special setup: %+v, handled=%v, error=%v", legacyResult, handled, err)
	}
}

func TestDownShockSourceBoundaryAndAdmission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*RunFixture)
		want   int
	}{
		{"before source close", func(f *RunFixture) { f.Bars = f.Bars[:1] }, 0},
		{"at source close", func(f *RunFixture) { f.Bars = f.Bars[1:] }, 1},
		{"no shock in source prefix", func(f *RunFixture) { f.SourceBars = f.SourceBars[:460] }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := inventedDownShockFixture()
			tc.mutate(&f)
			fixture, err := RunFixtureCase(f, testsupport.DownShockSource)
			if err != nil || fixture.TradeCount != tc.want {
				t.Fatalf("fixture: %+v %v", fixture, err)
			}
			got, err := Run(downShockSourceRequest(t, f, testsupport.DownShockSource))
			if err != nil || got.TradeCount != tc.want || !reflect.DeepEqual(got.Trades, fixture.Trades) {
				t.Fatalf("direct: %+v %v", got, err)
			}
		})
	}
	for _, tc := range []struct {
		name, detail string
		mutate       func(*RunRequest)
	}{
		{"missing source", "source market series is required", func(r *RunRequest) { r.SourceSeries = marketdata.Series{} }},
		{"wrong source timeframe", "source timeframe does not match", func(r *RunRequest) { r.SourceTimeframe = "5m" }},
		{"short source column", "source market series close column length", func(r *RunRequest) { r.SourceSeries.C = r.SourceSeries.C[:1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := downShockSourceRequest(t, inventedDownShockFixture(), testsupport.DownShockSource)
			tc.mutate(&r)
			if _, err := Run(r); err == nil || !strings.Contains(err.Error(), tc.detail) {
				t.Fatalf("error=%v, want %q", err, tc.detail)
			}
		})
	}
	for _, includeClose := range []bool{false, true} {
		r := downShockSourceRequest(t, inventedDownShockFixture(), testsupport.DownShockSource)
		closeT := int64(r.Series.T[1])
		want := 0
		if includeClose {
			r.ExecutionWindow = &ExecutionWindow{TradeFromT: &closeT}
			want = 1
		} else {
			r.ExecutionWindow = &ExecutionWindow{TradeToT: &closeT}
		}
		got, err := Run(r)
		if err != nil || got.TradeCount != want {
			t.Fatalf("includeClose=%v: %+v, %v", includeClose, got, err)
		}
	}
}

func TestDownShockAllModesDirectMatchExistingFixtures(t *testing.T) {
	for _, mode := range []string{"immediate-time", "immediate-atr", "immediate-13bp", "reversal-time", "reversal-atr", "reversal-13bp"} {
		t.Run(mode, func(t *testing.T) {
			f, source := shockFixture(t, mode)
			want, err := RunFixtureCase(f, source)
			if err != nil || want.TradeCount == 0 {
				t.Fatalf("positive fixture: %+v %v", want, err)
			}
			got, err := Run(downShockSourceRequest(t, f, source))
			if err != nil || !reflect.DeepEqual(got.Trades, want.Trades) {
				t.Fatalf("direct trades: %+v %v; want %+v", got.Trades, err, want.Trades)
			}
		})
	}
}
