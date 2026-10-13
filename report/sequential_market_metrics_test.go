package report

import (
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func v3Source(policy string) string {
	return "dsl v7\nstrategy \"Invented\" { description \"Invented valid OHLC\" }\nmarket { sequential symbol XAUUSD sequential timeframe 1h }\nsetup { type: sequential full sequential profile seq.full.public_approx.v1 sequential policy " + policy + " }\nrisk { sequential riskUsd 90 sequential maxNotionalUsd 1000000 }\n"
}

func v3Fixture(ohlc [][4]float64, fee float64) engine.RunFixture {
	bars := make([]marketdata.Bar, len(ohlc))
	for i, b := range ohlc {
		bars[i] = marketdata.Bar{T: 1709510400000 + float64(i)*3_600_000, O: b[0], H: b[1], L: b[2], C: b[3], V: 1}
	}
	return engine.RunFixture{Schema: "dsl-conformance-run-fixture-v1", Case: "invented", StrategyID: "invented", Symbol: "XAUUSD", Timeframe: "1h", RangeMethod: "zone",
		Costs: engine.Costs{FillOn: "open", Slippage: 0.5, FeePerUnit: fee, StartEquity: 10000}, Bars: bars}
}

var v3Setup = [][4]float64{
	{100, 105, 95, 100}, {100, 105, 95, 100}, {100, 105, 95, 100}, {100, 105, 95, 100}, {100, 105, 95, 101},
	{101, 108, 98, 99}, {99, 107, 97, 98}, {98, 98, 88, 97}, {97, 105, 95, 96}, {96, 104, 94, 95},
	{95, 103, 93, 94}, {94, 102, 92, 93}, {93, 101, 91, 92}, {92, 100, 90, 91},
}

func v3Run(t *testing.T, tail [][4]float64, fee float64) engine.SequentialMarketV3Run {
	t.Helper()
	run, err := engine.RunSequentialMarketV3(v3Fixture(append(append([][4]float64{}, v3Setup...), tail...), fee), v3Source("E1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want && math.Abs(got-want) > 1e-9*math.Max(math.Abs(got), math.Abs(want)) {
		t.Errorf("%s = %.17g, want %.17g", name, got, want)
	}
}

// Entry at bar 14: open 91 + 0.5 = 91.5, stop 87, quantity 20, entry fee 0.01 * 20 = 0.2.
// Bar 14 closes 92: open P&L (92-91.5)*20 = 10. Bar 15 closes 89 without touching
// the stop: (89-91.5)*20 = -50. The series ends there with the position open.
func TestProjectSequentialMarketMetricsOpenPosition(t *testing.T) {
	run := v3Run(t, [][4]float64{{91, 93, 90, 92}, {92, 93, 88.5, 89}}, 0.01)
	if run.Open == nil || len(run.Run.Trades) != 0 {
		t.Fatalf("want one open position and no completed trade: %+v", run.Open)
	}
	m, err := ProjectSequentialMarketMetrics(run.Run, run.Accounting)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "equity[13]", m.Equity[13].Equity, 10000)
	near(t, "equity[14]", m.Equity[14].Equity, 10000-0.2+10)
	near(t, "equity[15]", m.Equity[15].Equity, 10000-0.2-50)
	if len(m.Equity) != 16 {
		t.Fatalf("one mark per observed bar, got %d", len(m.Equity))
	}
	// Marked drawdown: peak 10009.8 on bar 14, trough 9949.8 on bar 15.
	near(t, "maxDD", m.Headline.MaxDD, 60)
	near(t, "maxDDpct", m.Headline.MaxDDpct, 60/10009.8*100)
	// Completed-trade view: nothing completed, nothing settled.
	h := m.Headline
	if h.Trades != 0 || h.WinRate != nil || h.WinRateReason != "no-trades" || h.AvgHoldBarsReason != "no-trades" {
		t.Fatalf("completed statistics must ignore the open trade: %+v", h)
	}
	near(t, "headline.net", h.Net, 0)
	near(t, "headline.endEquity", h.EndEquity, 10000)
	near(t, "headline.returnPct", h.ReturnPct, 0)
	// Marked view: cash 9999.8 plus -50 unrealized.
	near(t, "marked.endEquity", m.Marked.EndEquity, 9949.8)
	near(t, "marked.net", m.Marked.Net, -50.2)
	near(t, "marked.returnPct", m.Marked.ReturnPct, -0.502)
	near(t, "marked.openUnrealizedPnl", m.Marked.OpenUnrealizedPnL, -50)
	near(t, "marked.openEntryFeePaid", m.Marked.OpenEntryFeePaid, 0.2)
	near(t, "open.unrealizedPnl", run.Open.UnrealizedPnL, -50)
	if run.Open.BarsHeld != 2 || run.Open.EntryIndex != 14 || run.Open.Side != "long" {
		t.Fatalf("open trade: %+v", run.Open)
	}
}

// A completed trade followed by a later open one: completed statistics hold the
// first only, the marked curve and drawdown hold both.
func TestProjectSequentialMarketMetricsCompletedThenOpen(t *testing.T) {
	// Target exit on bar 16 (+170 gross, 169.6 net at fee 0.01). Nothing re-enters afterwards
	// on this series, so extend with the same Setup pattern at a new level.
	first := append(append([][4]float64{}, v3Setup...), [4]float64{91, 93, 90, 92}, [4]float64{92, 96, 91, 95}, [4]float64{95, 101, 94, 100})
	rng := rand.New(rand.NewSource(11))
	for seed := int64(1); seed < 400; seed++ {
		rng.Seed(seed)
		bars := append([][4]float64{}, first...)
		price := 100.0
		for i := 0; i < 160; i++ {
			open := price
			c := open + (rng.Float64()-0.5)*4
			bars = append(bars, [4]float64{open, math.Max(open, c) + rng.Float64(), math.Min(open, c) - rng.Float64(), c})
			price = c
		}
		run, err := engine.RunSequentialMarketV3(v3Fixture(bars, 0.01), v3Source("E1"), nil)
		if err != nil || run.Open == nil || len(run.Run.Trades) < 2 {
			continue
		}
		m, err := ProjectSequentialMarketMetrics(run.Run, run.Accounting)
		if err != nil {
			t.Fatal(err)
		}
		var net float64
		for _, tr := range m.TradeAccounting {
			net += tr.NetPnL
		}
		near(t, "headline.net is the sum of completed trades", m.Headline.Net, net)
		near(t, "marked.net", m.Marked.Net, net+run.Open.UnrealizedPnL-run.Open.EntryFeePaid)
		near(t, "marked.endEquity", m.Marked.EndEquity, 10000+m.Marked.Net)
		if m.Headline.Trades != len(run.Run.Trades) || len(m.Equity) != len(bars) {
			t.Fatalf("counts: %d trades, %d points", m.Headline.Trades, len(m.Equity))
		}
		return
	}
	t.Fatal("no series with completed trades and an open position found")
}

func TestProjectSequentialMarketMetricsFlatRunIsTheV1Projection(t *testing.T) {
	run := v3Run(t, [][4]float64{{91, 93, 90, 92}, {92, 96, 91, 95}, {95, 101, 94, 100}}, 0.01)
	if run.Open != nil {
		t.Fatal("expected a completed run")
	}
	got, err := ProjectSequentialMarketMetrics(run.Run, run.Accounting)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ProjectSequentialMetrics(run.Run, run.Accounting)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.SequentialMetrics, want) || got.Marked.EndEquity != want.Headline.EndEquity || got.Marked.Net != want.Headline.Net || got.Marked.OpenUnrealizedPnL != 0 || got.Marked.OpenEntryFeePaid != 0 {
		t.Fatalf("flat run changed: %+v", got)
	}
}

// The marked-curve drawdown helper must be the same reduction as the one inside
// ProjectSequentialMetrics, bit for bit.
func TestSequentialMarkedDrawdownEqualsTheV1Reduction(t *testing.T) {
	checked := 0
	for seed := int64(1); seed <= 200; seed++ {
		rng := rand.New(rand.NewSource(seed))
		bars := append([][4]float64{}, v3Setup...)
		price := 100.0
		for i := 0; i < 200; i++ {
			open := price
			c := open + (rng.Float64()-0.5)*4
			bars = append(bars, [4]float64{open, math.Max(open, c) + rng.Float64(), math.Min(open, c) - rng.Float64(), c})
			price = c
		}
		run, err := engine.RunSequentialMarketV3(v3Fixture(bars, 0.01), v3Source("E1"), nil)
		if err != nil || run.Open != nil || len(run.Run.Trades) < 2 {
			continue
		}
		m, err := ProjectSequentialMetrics(run.Run, run.Accounting)
		if err != nil {
			t.Fatal(err)
		}
		equities := make([]float64, len(run.Accounting.Marks))
		for i, mark := range run.Accounting.Marks {
			equities[i] = mark.Equity
		}
		dd, pct, err := sequentialMarkedDrawdown(run.Accounting.StartEquity, equities)
		if err != nil || math.Float64bits(dd) != math.Float64bits(m.Headline.MaxDD) || math.Float64bits(pct) != math.Float64bits(m.Headline.MaxDDpct) {
			t.Fatalf("seed %d: helper (%v, %v) differs from reducer (%v, %v)", seed, dd, pct, m.Headline.MaxDD, m.Headline.MaxDDpct)
		}
		if m.Headline.MaxDD > 0 {
			checked++
		}
	}
	if checked < 10 {
		t.Fatalf("only %d runs with a drawdown; the check proves little", checked)
	}
}

func TestProjectSequentialMarketMetricsRefusesInconsistentAccounting(t *testing.T) {
	run := v3Run(t, [][4]float64{{91, 93, 90, 92}, {92, 93, 88.5, 89}}, 0.01)
	clone := func() engine.SequentialAccounting {
		a := run.Accounting
		a.Marks = append([]engine.SequentialEquityMark{}, a.Marks...)
		a.Events = append([]engine.SequentialAccountingEvent{}, a.Events...)
		a.Trades = append([]engine.SequentialTradeAccounting{}, a.Trades...)
		return a
	}
	for name, edit := range map[string]func(a *engine.SequentialAccounting){
		"entry event removed":      func(a *engine.SequentialAccounting) { a.Events = a.Events[:0] },
		"extra event":              func(a *engine.SequentialAccounting) { a.Events = append(a.Events, a.Events[0], a.Events[0]) },
		"entry renamed exit":       func(a *engine.SequentialAccounting) { a.Events[0].Kind = "exit" },
		"entry index out of range": func(a *engine.SequentialAccounting) { a.Events[0].Index = 99 },
		"mark equity nudged": func(a *engine.SequentialAccounting) {
			a.Marks[15].Equity = math.Nextafter(a.Marks[15].Equity, math.Inf(1))
		},
		"mark realized differs":   func(a *engine.SequentialAccounting) { a.Marks[15].Realized += 0.5 },
		"mark unrealized dropped": func(a *engine.SequentialAccounting) { a.Marks[15].Unrealized = 0 },
		"no marks":                func(a *engine.SequentialAccounting) { a.Marks = nil },
		"nonfinite mark":          func(a *engine.SequentialAccounting) { a.Marks[3].Equity = math.NaN() },
	} {
		t.Run(name, func(t *testing.T) {
			a := clone()
			edit(&a)
			if _, err := ProjectSequentialMarketMetrics(run.Run, a); err == nil {
				t.Fatal("inconsistent accounting was projected")
			}
		})
	}
	if _, err := ProjectSequentialMarketMetrics(run.Run, run.Accounting); err != nil {
		t.Fatalf("unmodified accounting refused: %v", err)
	}
}
