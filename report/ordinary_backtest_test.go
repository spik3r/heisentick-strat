package report

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
)

// These deliberately synthetic value snapshots test financial projection only.
// They do not execute strategies or qualify an ordinary report endpoint.
func ordinaryProjectionPartialFixture() engine.OrdinaryBacktestFinancials {
	return engine.OrdinaryBacktestFinancials{
		StartEquity: 100, TerminalRealized: -5, LastFlatRealized: -5, EntryFees: 10, ExitFees: 10,
		Events: []engine.OrdinaryBacktestFinancialEvent{
			{Kind: "entry", PositionID: "p1", Side: "long", Index: 0, Ordinal: 0, TradeIndex: -1, T: 0, Price: 10, Size: 10, Fee: 10, RealizedBefore: 0, RealizedAfter: -10},
			{Kind: "partial", PositionID: "p1", Side: "long", Index: 1, Ordinal: 0, TradeIndex: 0, T: 60000, Price: 12, Size: 5, Fee: 5, RealizedBefore: -10, RealizedAfter: -5, ExitCredit: 5},
			{Kind: "exit", PositionID: "p1", Side: "long", Index: 2, Ordinal: 1, TradeIndex: 1, T: 120000, Price: 11, Size: 5, Fee: 5, RealizedBefore: -5, RealizedAfter: -5, ExitCredit: 0},
		},
		Marks: []engine.OrdinaryBacktestFinancialMark{
			{Index: 0, T: 0, Realized: -10, Unrealized: 0, Equity: 90, EntryFees: 10, ExitFees: 0},
			{Index: 1, T: 60000, Realized: -5, Unrealized: 10, Equity: 105, EntryFees: 10, ExitFees: 5},
			{Index: 2, T: 120000, Realized: -5, Unrealized: 0, Equity: 95, EntryFees: 10, ExitFees: 10},
		},
	}
}

func ordinaryProjectionNoTrades(n int) engine.OrdinaryBacktestFinancials {
	a := engine.OrdinaryBacktestFinancials{StartEquity: 100, Events: []engine.OrdinaryBacktestFinancialEvent{}, Marks: make([]engine.OrdinaryBacktestFinancialMark, n)}
	for i := range a.Marks {
		a.Marks[i] = engine.OrdinaryBacktestFinancialMark{Index: i, T: float64(i * 60000), Equity: 100}
	}
	return a
}

// Explicit synthetic completions, using a declared sequence of binary64
// additions for the fixture's observed broker fields. Expectations below are
// independently hand-derived; this is not a native arithmetic replay oracle.
func ordinaryProjectionCompletions(start float64, fees, credits []float64) engine.OrdinaryBacktestFinancials {
	a := engine.OrdinaryBacktestFinancials{StartEquity: start}
	var realized, entryFees, exitFees float64
	for i, credit := range credits {
		fee := fees[i]
		id, tm := fmt.Sprintf("p%d", i), float64(i*60000)
		entryAfter := float64(realized - fee)
		a.Events = append(a.Events, engine.OrdinaryBacktestFinancialEvent{Kind: "entry", PositionID: id, Side: "long", Index: i, Ordinal: 0, TradeIndex: -1, T: tm, Size: 1, Fee: fee, RealizedBefore: realized, RealizedAfter: entryAfter})
		realized = float64(entryAfter + credit)
		a.Events = append(a.Events, engine.OrdinaryBacktestFinancialEvent{Kind: "exit", PositionID: id, Side: "long", Index: i, Ordinal: 1, TradeIndex: i, T: tm, Price: credit, Size: 1, Fee: fee, RealizedBefore: entryAfter, RealizedAfter: realized, ExitCredit: credit})
		entryFees, exitFees = float64(entryFees+fee), float64(exitFees+fee)
		a.Marks = append(a.Marks, engine.OrdinaryBacktestFinancialMark{Index: i, T: tm, Realized: realized, Equity: float64(float64(start+realized) + 0), EntryFees: entryFees, ExitFees: exitFees})
	}
	a.TerminalRealized, a.LastFlatRealized, a.EntryFees, a.ExitFees = realized, realized, entryFees, exitFees
	return a
}

// One synthetic open position supplies intermediate original-close unrealized
// marks. The last point is flattened, never a retained-open paper lifecycle.
func ordinaryProjectionCurve(start float64, values ...float64) engine.OrdinaryBacktestFinancials {
	a := engine.OrdinaryBacktestFinancials{StartEquity: start}
	a.Events = append(a.Events, engine.OrdinaryBacktestFinancialEvent{Kind: "entry", PositionID: "curve", Side: "long", TradeIndex: -1, Size: 1})
	for i, value := range values {
		u := float64(value - start)
		a.Marks = append(a.Marks, engine.OrdinaryBacktestFinancialMark{Index: i, T: float64(i * 60000), Unrealized: u, Equity: float64(start + u)})
	}
	last := len(values) - 1
	r := float64(values[last] - start)
	ordinal := 1 // Final liquidation follows the discarded last mark.
	if last == 0 {
		ordinal = 2
	}
	a.Events = append(a.Events, engine.OrdinaryBacktestFinancialEvent{Kind: "exit", PositionID: "curve", Side: "long", Index: last, Ordinal: ordinal, TradeIndex: 0, T: float64(last * 60000), Price: r, Size: 1, RealizedAfter: r, ExitCredit: r})
	a.Marks[last].Realized, a.Marks[last].Unrealized = r, 0
	a.Marks[last].Equity = float64(float64(start+r) + 0)
	a.TerminalRealized, a.LastFlatRealized = r, r
	return a
}

func ordinaryProjectionClone(a engine.OrdinaryBacktestFinancials) engine.OrdinaryBacktestFinancials {
	if a.Events != nil {
		copyEvents := make([]engine.OrdinaryBacktestFinancialEvent, len(a.Events))
		copy(copyEvents, a.Events)
		a.Events = copyEvents
	}
	if a.Marks != nil {
		copyMarks := make([]engine.OrdinaryBacktestFinancialMark, len(a.Marks))
		copy(copyMarks, a.Marks)
		a.Marks = copyMarks
	}
	return a
}

func ordinaryProjectionEqualBits(t *testing.T, a, b reflect.Value) {
	t.Helper()
	if a.Type() != b.Type() {
		t.Fatal("input type changed")
	}
	switch a.Kind() {
	case reflect.Float64:
		if math.Float64bits(a.Float()) != math.Float64bits(b.Float()) {
			t.Fatal("input float bits changed")
		}
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			ordinaryProjectionEqualBits(t, a.Field(i), b.Field(i))
		}
	case reflect.Slice:
		if a.Len() != b.Len() || a.IsNil() != b.IsNil() {
			t.Fatal("input slice changed")
		}
		for i := 0; i < a.Len(); i++ {
			ordinaryProjectionEqualBits(t, a.Index(i), b.Index(i))
		}
	default:
		if !reflect.DeepEqual(a.Interface(), b.Interface()) {
			t.Fatal("input value changed")
		}
	}
}

func ordinaryProjectionOK(t *testing.T, a engine.OrdinaryBacktestFinancials) OrdinaryBacktestProjection {
	t.Helper()
	before := ordinaryProjectionClone(a)
	p, err := ProjectOrdinaryBacktestFinancials(a)
	ordinaryProjectionEqualBits(t, reflect.ValueOf(a), reflect.ValueOf(before))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	// The task's external schema harness consumes these actual projected
	// sections. No invented success envelope or native output is emitted.
	if len(a.Marks) < 20 {
		t.Logf("ordinary-projection-json: %s", raw)
	}
	return p
}

func ordinaryProjectionRefused(t *testing.T, a engine.OrdinaryBacktestFinancials) error {
	t.Helper()
	before := ordinaryProjectionClone(a)
	p, err := ProjectOrdinaryBacktestFinancials(a)
	ordinaryProjectionEqualBits(t, reflect.ValueOf(a), reflect.ValueOf(before))
	if err == nil || !reflect.DeepEqual(p, OrdinaryBacktestProjection{}) {
		t.Fatalf("wanted zero projection and error; got %+v, %v", p, err)
	}
	return err
}

func ordinaryProjectionValue(t *testing.T, m OrdinaryBacktestMetric, want float64) {
	t.Helper()
	if m.Status != "available" || m.Reason != "" || m.Value == nil || math.Float64bits(*m.Value) != math.Float64bits(want) {
		t.Fatalf("metric=%+v value=%v; want bits %016x", m, m.Value, math.Float64bits(want))
	}
}

func ordinaryProjectionUnavailable(t *testing.T, m OrdinaryBacktestMetric, reason string) {
	t.Helper()
	if m.Status != "unavailable" || m.Reason != reason || m.Value != nil {
		t.Fatalf("unavailable metric=%+v; want %s", m, reason)
	}
}

func TestOrdinaryProjectionPartialAndFinalFees(t *testing.T) {
	p := ordinaryProjectionOK(t, ordinaryProjectionPartialFixture())
	for _, m := range []OrdinaryBacktestMetric{p.Account.CompletedNetPnl, p.Account.RealizedCashPnl, p.Account.MarkedNetPnl, p.CompletedPositions.NetPnl} {
		ordinaryProjectionValue(t, m, -5)
	}
	ordinaryProjectionValue(t, p.Account.CompletedEndEquity, 95)
	ordinaryProjectionValue(t, p.Account.MarkedEndEquity, 95)
	ordinaryProjectionValue(t, p.Account.CompletedReturnPct, -5)
	ordinaryProjectionValue(t, p.Account.MarkedReturnPct, -5)
	ordinaryProjectionValue(t, p.Account.EntryFeesPaid, 10)
	ordinaryProjectionValue(t, p.Account.ExitFeesPaid, 10)
	ordinaryProjectionValue(t, p.Account.OpenUnrealizedPnl, 0)
	ordinaryProjectionValue(t, p.Account.OpenEntryFeePaid, 0)
	ordinaryProjectionValue(t, p.CompletedPositions.Count, 1)
	ordinaryProjectionValue(t, p.CompletedPositions.Losses, 1)
	ordinaryProjectionValue(t, p.CompletedPositions.Wins, 0)
	ordinaryProjectionValue(t, p.CompletedPositions.Breakeven, 0)
	ordinaryProjectionValue(t, p.CompletedPositions.WinRate, 0)
	ordinaryProjectionValue(t, p.CompletedPositions.ProfitFactor, 0)
	if p.Equity[1].UnrealizedPnl != 10 || p.Equity[1].RealizedCashPnl != -5 || p.Equity[1].Equity != 105 {
		t.Fatal("partial mark changed")
	}
}

func TestOrdinaryProjectionCountsAndAvailability(t *testing.T) {
	cases := []struct {
		name                         string
		credits                      []float64
		wins, losses, breakeven, net float64
		pf                           *float64
		reason                       string
	}{
		{"no trades", nil, 0, 0, 0, 0, nil, "no-trades"},
		{"winner only", []float64{5}, 1, 0, 0, 5, nil, "no-losses"},
		{"loser only", []float64{-5}, 0, 1, 0, -5, new(float64), ""},
		{"breakeven only", []float64{0}, 0, 0, 1, 0, nil, "no-losses"},
		{"mixed", []float64{4, -2, 0, 2}, 2, 1, 1, 4, func() *float64 { v := 3.0; return &v }(), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := ordinaryProjectionNoTrades(1)
			if c.credits != nil {
				a = ordinaryProjectionCompletions(100, make([]float64, len(c.credits)), c.credits)
			}
			p := ordinaryProjectionOK(t, a)
			s := p.CompletedPositions
			ordinaryProjectionValue(t, s.Count, float64(len(c.credits)))
			ordinaryProjectionValue(t, s.Wins, c.wins)
			ordinaryProjectionValue(t, s.Losses, c.losses)
			ordinaryProjectionValue(t, s.Breakeven, c.breakeven)
			ordinaryProjectionValue(t, s.NetPnl, c.net)
			if len(c.credits) == 0 {
				ordinaryProjectionUnavailable(t, s.WinRate, "no-trades")
			} else {
				ordinaryProjectionValue(t, s.WinRate, float64(c.wins/float64(len(c.credits))))
			}
			if c.pf == nil {
				ordinaryProjectionUnavailable(t, s.ProfitFactor, c.reason)
			} else {
				ordinaryProjectionValue(t, s.ProfitFactor, *c.pf)
			}
			for _, m := range []OrdinaryBacktestMetric{s.Expectancy, s.AverageWin, s.AverageLoss, s.WorstLoss} {
				ordinaryProjectionUnavailable(t, m, "not-in-contract")
				if m.Basis != "ordinary-statistic-not-in-contract-v1" || m.Unit != "native-account-unit" {
					t.Fatal("statistic tags")
				}
			}
			v := reflect.ValueOf(p.Analyses)
			if v.NumField() != 7 {
				t.Fatal("analysis count")
			}
			for i := 0; i < v.NumField(); i++ {
				if v.Field(i).Interface() != (OrdinaryBacktestUnavailableAnalysis{"unavailable", "not-in-contract"}) {
					t.Fatal("analysis availability")
				}
			}
		})
	}
}

func TestOrdinaryProjectionActualBrokerVersusLogicalReduction(t *testing.T) {
	// First net is 2^53-1. Second net rounds (-2^53)-1 to -2^53,
	// so logical net is -1. Actual realized sequence ends at -2.
	// This deliberate regrouping difference is valid, not a reconciliation error.
	a := ordinaryProjectionCompletions(100, []float64{1, 1}, []float64{9007199254740992, -9007199254740992})
	p := ordinaryProjectionOK(t, a)
	ordinaryProjectionValue(t, p.Account.CompletedNetPnl, -2)
	ordinaryProjectionValue(t, p.Account.RealizedCashPnl, -2)
	ordinaryProjectionValue(t, p.Account.CompletedEndEquity, 98)
	ordinaryProjectionValue(t, p.CompletedPositions.NetPnl, -1)
	ordinaryProjectionValue(t, p.CompletedPositions.Wins, 1)
	ordinaryProjectionValue(t, p.CompletedPositions.Losses, 1)
	ordinaryProjectionValue(t, p.Account.EntryFeesPaid, 2)
	ordinaryProjectionValue(t, p.Account.ExitFeesPaid, 2)
}

func TestOrdinaryProjectionExitAndCompletionReductionOrder(t *testing.T) {
	t.Run("subtract entry fee after ordered credits", func(t *testing.T) {
		a := ordinaryProjectionPartialFixture()
		a.EntryFees, a.ExitFees, a.TerminalRealized, a.LastFlatRealized = 1, 1, 0, 0
		a.Events[0].Fee, a.Events[0].RealizedAfter = 1, -1
		a.Events[1].Fee, a.Events[1].RealizedBefore, a.Events[1].RealizedAfter, a.Events[1].ExitCredit = .5, -1, -9007199254740992, -9007199254740992
		a.Events[2].Fee, a.Events[2].RealizedBefore, a.Events[2].RealizedAfter, a.Events[2].ExitCredit = .5, -9007199254740992, 0, 9007199254740992
		a.Marks[0] = engine.OrdinaryBacktestFinancialMark{Index: 0, Realized: -1, Equity: 99, EntryFees: 1}
		a.Marks[1] = engine.OrdinaryBacktestFinancialMark{Index: 1, T: 60000, Realized: -9007199254740992, Equity: -9007199254740892, EntryFees: 1, ExitFees: .5}
		a.Marks[2] = engine.OrdinaryBacktestFinancialMark{Index: 2, T: 120000, Equity: 100, EntryFees: 1, ExitFees: 1}
		p := ordinaryProjectionOK(t, a)
		ordinaryProjectionValue(t, p.CompletedPositions.NetPnl, -1)
		ordinaryProjectionValue(t, p.Account.CompletedNetPnl, 0)
	})
	t.Run("completion order is not winner loser regrouping", func(t *testing.T) {
		a := ordinaryProjectionCompletions(100, []float64{0, 0, 0}, []float64{1e16, -1e16, 1})
		p := ordinaryProjectionOK(t, a)
		ordinaryProjectionValue(t, p.CompletedPositions.NetPnl, 1)
		ordinaryProjectionValue(t, p.CompletedPositions.ProfitFactor, 1)
		ordinaryProjectionValue(t, p.CompletedPositions.WinRate, float64(2.0/3.0))
	})
}

func TestOrdinaryProjectionObservedNominalFeesAndRounding(t *testing.T) {
	a := ordinaryProjectionCompletions(100, []float64{.1, .1}, []float64{1e16, -1e16})
	if a.Events[2].RealizedBefore != a.Events[2].RealizedAfter {
		t.Fatal("fixture must lose second debit in actual realized")
	}
	p := ordinaryProjectionOK(t, a)
	ordinaryProjectionValue(t, p.Account.EntryFeesPaid, .2)
	ordinaryProjectionValue(t, p.Account.ExitFeesPaid, .2)
	b := ordinaryProjectionCompletions(3, []float64{0}, []float64{1})
	q := ordinaryProjectionOK(t, b)
	ordinaryProjectionValue(t, q.Account.CompletedReturnPct, math.Float64frombits(0x4040aaaaaaaaaaaa))
	ordinaryProjectionValue(t, q.Account.MarkedReturnPct, math.Float64frombits(0x4040aaaaaaaaaaaa))
}

func TestOrdinaryProjectionShortestRoundTripNotEquitySubtraction(t *testing.T) {
	r := math.Nextafter(1, 2)
	a := ordinaryProjectionCompletions(100, []float64{0}, []float64{r})
	p := ordinaryProjectionOK(t, a)
	ordinaryProjectionValue(t, p.Account.RealizedCashPnl, r)
	ordinaryProjectionValue(t, p.Account.CompletedNetPnl, r)
	ordinaryProjectionValue(t, p.Account.MarkedNetPnl, r)
	ordinaryProjectionValue(t, p.CompletedPositions.NetPnl, r)
	ordinaryProjectionValue(t, p.Account.CompletedEndEquity, 101)
	if *p.Account.CompletedEndEquity.Value-100 == r {
		t.Fatal("fixture must lose precision in equity subtraction")
	}
	raw, err := json.Marshal(p)
	if err != nil || !strings.Contains(string(raw), `"value":1.0000000000000002`) {
		t.Fatal("projection rounded raw financial precision")
	}
}

func TestOrdinaryProjectionDrawdownAndFinalCurve(t *testing.T) {
	cases := []struct {
		name                string
		a                   engine.OrdinaryBacktestFinancials
		amount, percent     float64
		amountAt, percentAt int
	}{
		{"one bar no trades", ordinaryProjectionNoTrades(1), 0, 0, -1, -1},
		{"one bar liquidation", ordinaryProjectionCurve(100, 90), 10, 10, 0, 0},
		{"rising curve", ordinaryProjectionCurve(100, 100, 110, 120), 0, 0, -1, -1},
		{"negative equity over 100 percent", ordinaryProjectionCurve(100, 100, -50, -25), 150, 150, 1, 1},
		{"independent maxima and earliest ties", ordinaryProjectionCurve(100, 50, 50, 200, 120, 120), 80, 50, 3, 0},
		{"discarded provisional peak", ordinaryProjectionCurve(100, 99, 116), 1, 1, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := ordinaryProjectionOK(t, c.a)
			ordinaryProjectionValue(t, p.Drawdown.Amount, c.amount)
			ordinaryProjectionValue(t, p.Drawdown.Percent, c.percent)
			for _, loc := range []struct {
				got  *OrdinaryBacktestLocation
				want int
			}{{p.Drawdown.AmountAt, c.amountAt}, {p.Drawdown.PercentAt, c.percentAt}} {
				if loc.want < 0 {
					if loc.got != nil {
						t.Fatal("zero maximum needs null location")
					}
					continue
				}
				if loc.got == nil || loc.got.Index != loc.want || loc.got.T != c.a.Marks[loc.want].T {
					t.Fatalf("locator=%+v want index %d", loc.got, loc.want)
				}
			}
		})
	}
	// Reproduce the captured financial facts of the actual finalization fixture:
	// S=100, entry 11, original last close 30, provisional peak 119;
	// liquidation at 27 replaces that last mark with 116. No 119 is retained.
	a := ordinaryProjectionCurve(100, 99, 116)
	a.Events[0].Price = 11
	a.Events[1].Price = 27
	p := ordinaryProjectionOK(t, a)
	if p.Equity[0].Equity != 99 || p.Equity[1].Equity != 116 {
		t.Fatal("finalized curve changed")
	}
	ordinaryProjectionValue(t, p.Drawdown.Amount, 1)
}

func TestOrdinaryProjectionLifecycleShapes(t *testing.T) {
	t.Run("same bar exits and reentry", func(t *testing.T) {
		a := ordinaryProjectionCompletions(100, []float64{1, 1}, []float64{5, 2})
		for i := range a.Events {
			a.Events[i].Index = 0
			a.Events[i].T = 0
			a.Events[i].Ordinal = i
		}
		a.Marks = a.Marks[1:]
		a.Marks[0].Index = 0
		a.Marks[0].T = 0
		p := ordinaryProjectionOK(t, a)
		ordinaryProjectionValue(t, p.CompletedPositions.Count, 2)
		ordinaryProjectionValue(t, p.CompletedPositions.NetPnl, 5)
	})
	t.Run("short partial", func(t *testing.T) {
		a := ordinaryProjectionPartialFixture()
		for i := range a.Events {
			a.Events[i].Side = "short"
		}
		a.Events[1].Price = 8
		a.Events[2].Price = 9
		ordinaryProjectionOK(t, a)
	})
	t.Run("final exit before last mark", func(t *testing.T) {
		a := ordinaryProjectionPartialFixture()
		a.Events[2].Ordinal = 0
		ordinaryProjectionOK(t, a)
	})
	t.Run("actual remainder is not subtraction", func(t *testing.T) {
		a := ordinaryProjectionPartialFixture()
		a.Events[0].Size = .3
		a.Events[1].Size = .1
		a.Events[2].Size = .2
		if a.Events[0].Size-a.Events[1].Size == a.Events[2].Size {
			t.Fatal("fixture must witness different bits")
		}
		ordinaryProjectionOK(t, a)
	})
	t.Run("maximum safe timestamp", func(t *testing.T) {
		a := ordinaryProjectionNoTrades(1)
		a.Marks[0].T = 9007199254740991
		ordinaryProjectionOK(t, a)
	})
	t.Run("maximum original marks", func(t *testing.T) {
		p := ordinaryProjectionOK(t, ordinaryProjectionNoTrades(engine.OrdinaryBacktestMaxBars))
		if len(p.Equity) != engine.OrdinaryBacktestMaxBars {
			t.Fatal("marks dropped")
		}
	})
}

func TestOrdinaryProjectionSignedZeroAndNoAliasing(t *testing.T) {
	z := math.Copysign(0, -1)
	a := ordinaryProjectionCompletions(100, []float64{z}, []float64{z})
	a.Events[0].T = z
	a.Events[1].T = z
	a.Events[0].RealizedAfter = z
	a.Events[1].RealizedBefore = z
	a.Events[1].RealizedAfter = z
	a.Marks[0].T = z
	a.Marks[0].Realized = z
	a.Marks[0].Unrealized = z
	a.TerminalRealized, a.LastFlatRealized, a.TerminalUnrealized, a.OpenEntryFee = z, z, z, z
	before := ordinaryProjectionClone(a)
	p := ordinaryProjectionOK(t, a)
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), ":-0") {
		t.Fatalf("signed output zero: %s", raw)
	}
	ordinaryProjectionValue(t, p.CompletedPositions.Breakeven, 1)
	ordinaryProjectionValue(t, p.Account.RealizedCashPnl, 0)
	ordinaryProjectionValue(t, p.Account.OpenUnrealizedPnl, 0)
	ordinaryProjectionUnavailable(t, p.CompletedPositions.ProfitFactor, "no-losses")
	*p.Account.StartEquity.Value = 999
	p.Equity[0].Equity = 999
	ordinaryProjectionEqualBits(t, reflect.ValueOf(a), reflect.ValueOf(before))
	q := ordinaryProjectionOK(t, a)
	ordinaryProjectionValue(t, q.Account.StartEquity, 100)
	if q.Equity[0].Equity != 100 {
		t.Fatal("projection output aliases another output")
	}
}

func TestOrdinaryProjectionMalformedFinancials(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*engine.OrdinaryBacktestFinancials)
	}{
		{"missing marks", func(a *engine.OrdinaryBacktestFinancials) { a.Marks = nil }},
		{"too many marks", func(a *engine.OrdinaryBacktestFinancials) {
			a.Marks = make([]engine.OrdinaryBacktestFinancialMark, engine.OrdinaryBacktestMaxBars+1)
		}},
		{"zero start", func(a *engine.OrdinaryBacktestFinancials) { a.StartEquity = 0 }},
		{"negative start", func(a *engine.OrdinaryBacktestFinancials) { a.StartEquity = -1 }},
		{"negative fee total", func(a *engine.OrdinaryBacktestFinancials) { a.EntryFees = -1 }},
		{"wrong entry total", func(a *engine.OrdinaryBacktestFinancials) { a.EntryFees++ }},
		{"wrong exit total", func(a *engine.OrdinaryBacktestFinancials) { a.ExitFees++ }},
		{"wrong terminal realized", func(a *engine.OrdinaryBacktestFinancials) { a.TerminalRealized++ }},
		{"wrong last flat", func(a *engine.OrdinaryBacktestFinancials) { a.LastFlatRealized++ }},
		{"terminal open unrealized", func(a *engine.OrdinaryBacktestFinancials) { a.TerminalUnrealized = 1 }},
		{"terminal open fee", func(a *engine.OrdinaryBacktestFinancials) { a.OpenEntryFee = 1 }},
		{"missing entry", func(a *engine.OrdinaryBacktestFinancials) { a.Events = a.Events[1:] }},
		{"missing partial", func(a *engine.OrdinaryBacktestFinancials) { a.Events = append(a.Events[:1], a.Events[2:]...) }},
		{"missing final exit", func(a *engine.OrdinaryBacktestFinancials) { a.Events = a.Events[:2] }},
		{"missing all events", func(a *engine.OrdinaryBacktestFinancials) { a.Events = nil }},
		{"unknown kind", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Kind = "mark" }},
		{"empty position", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].PositionID = "" }},
		{"wrong position", func(a *engine.OrdinaryBacktestFinancials) { a.Events[1].PositionID = "other" }},
		{"unknown side", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Side = "flat" }},
		{"changed side", func(a *engine.OrdinaryBacktestFinancials) { a.Events[1].Side = "short" }},
		{"zero size", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Size = 0 }},
		{"negative size", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Size = -1 }},
		{"oversized partial", func(a *engine.OrdinaryBacktestFinancials) { a.Events[1].Size = 11 }},
		{"oversized final remainder", func(a *engine.OrdinaryBacktestFinancials) { a.Events[2].Size = 11 }},
		{"negative fee", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Fee = -1 }},
		{"entry exit credit", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].ExitCredit = 1 }},
		{"entry trade index", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].TradeIndex = 0 }},
		{"exit trade index gap", func(a *engine.OrdinaryBacktestFinancials) { a.Events[2].TradeIndex = 2 }},
		{"exit trade index duplicate", func(a *engine.OrdinaryBacktestFinancials) { a.Events[2].TradeIndex = 0 }},
		{"negative event index", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Index = -1 }},
		{"event beyond marks", func(a *engine.OrdinaryBacktestFinancials) { a.Events[2].Index = 3 }},
		{"backwards event order", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Index = 2 }},
		{"event timestamp mismatch", func(a *engine.OrdinaryBacktestFinancials) { a.Events[1].T++ }},
		{"negative ordinal", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Ordinal = -1 }},
		{"missing initial ordinal", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].Ordinal = 1 }},
		{"gap before partial", func(a *engine.OrdinaryBacktestFinancials) { a.Events[1].Ordinal = 1 }},
		{"too large liquidation gap", func(a *engine.OrdinaryBacktestFinancials) { a.Events[2].Ordinal = 2 }},
		{"realized continuity", func(a *engine.OrdinaryBacktestFinancials) { a.Events[1].RealizedBefore++ }},
		{"raw signed zero continuity", func(a *engine.OrdinaryBacktestFinancials) { a.Events[0].RealizedBefore = math.Copysign(0, -1) }},
		{"mark index gap", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[1].Index = 2 }},
		{"mark time duplicate", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[1].T = 0 }},
		{"mark time fractional", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[1].T = .5 }},
		{"mark time negative", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[0].T = -1 }},
		{"mark time unsafe", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[2].T = 9007199254740992 }},
		{"mark realized mismatch", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[1].Realized++ }},
		{"mark unrealized mismatch", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[1].Unrealized++ }},
		{"mark equity mismatch", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[1].Equity++ }},
		{"mark entry fees mismatch", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[1].EntryFees++ }},
		{"mark exit fees mismatch", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[1].ExitFees++ }},
		{"flat mark unrealized", func(a *engine.OrdinaryBacktestFinancials) { a.Marks[2].Unrealized = 1; a.Marks[2].Equity++ }},
		{"reopened before exit", func(a *engine.OrdinaryBacktestFinancials) {
			a.Events[1].Kind = "entry"
			a.Events[1].TradeIndex = -1
			a.Events[1].ExitCredit = 0
		}},
		{"duplicate partial", func(a *engine.OrdinaryBacktestFinancials) { a.Events[2].Kind = "partial"; a.Events[2].Ordinal = 0 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := ordinaryProjectionPartialFixture()
			c.mutate(&a)
			ordinaryProjectionRefused(t, a)
		})
	}
	t.Run("duplicate completed position id", func(t *testing.T) {
		a := ordinaryProjectionCompletions(100, []float64{0, 0}, []float64{1, 1})
		a.Events[2].PositionID = "p0"
		a.Events[3].PositionID = "p0"
		ordinaryProjectionRefused(t, a)
	})
	t.Run("full exit size mismatch", func(t *testing.T) {
		a := ordinaryProjectionCompletions(100, []float64{0}, []float64{1})
		a.Events[1].Size = .5
		ordinaryProjectionRefused(t, a)
	})
	t.Run("open terminal with consistent financial fields", func(t *testing.T) {
		a := ordinaryProjectionNoTrades(1)
		a.Events = []engine.OrdinaryBacktestFinancialEvent{{Kind: "entry", PositionID: "p", Side: "long", TradeIndex: -1, Size: 1}}
		ordinaryProjectionRefused(t, a)
	})
}

func TestOrdinaryProjectionAllNonfiniteRawFields(t *testing.T) {
	for _, target := range []string{"snapshot", "event", "mark"} {
		a := ordinaryProjectionPartialFixture()
		v := reflect.ValueOf(&a).Elem()
		if target == "event" {
			v = reflect.ValueOf(&a.Events[1]).Elem()
		}
		if target == "mark" {
			v = reflect.ValueOf(&a.Marks[1]).Elem()
		}
		for field := 0; field < v.NumField(); field++ {
			if v.Field(field).Kind() != reflect.Float64 {
				continue
			}
			name := v.Type().Field(field).Name
			for _, bad := range []struct {
				name string
				v    float64
			}{{"nan", math.NaN()}, {"positive infinity", math.Inf(1)}, {"negative infinity", math.Inf(-1)}} {
				t.Run(target+"/"+name+"/"+bad.name, func(t *testing.T) {
					b := ordinaryProjectionPartialFixture()
					w := reflect.ValueOf(&b).Elem()
					if target == "event" {
						w = reflect.ValueOf(&b.Events[1]).Elem()
					}
					if target == "mark" {
						w = reflect.ValueOf(&b.Marks[1]).Elem()
					}
					w.Field(field).SetFloat(bad.v)
					ordinaryProjectionRefused(t, b)
				})
			}
		}
	}
}

func TestOrdinaryProjectionOverflowRefusal(t *testing.T) {
	max := math.MaxFloat64
	cases := []struct {
		name string
		a    engine.OrdinaryBacktestFinancials
	}{
		{"return division", ordinaryProjectionCompletions(math.SmallestNonzeroFloat64, []float64{0}, []float64{1})},
		{"return multiplication", ordinaryProjectionCompletions(1, []float64{0}, []float64{max / 2})},
		{"mark cash", ordinaryProjectionCompletions(max, []float64{0}, []float64{max})},
		{"profit factor", ordinaryProjectionCompletions(100, []float64{0, 0}, []float64{1e300, -1e-300})},
		{"gross wins", ordinaryProjectionCompletions(100, []float64{0, 0, 0}, []float64{max * .75, -max * .75, max * .75})},
		{"gross losses", ordinaryProjectionCompletions(100, []float64{0, 0, 0}, []float64{-max * .75, max * .75, -max * .75})},
		{"drawdown amount", ordinaryProjectionCurve(1, max*.75, -max*.75, 1)},
		{"drawdown percent", ordinaryProjectionCurve(math.SmallestNonzeroFloat64, -1, math.SmallestNonzeroFloat64)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ordinaryProjectionRefused(t, c.a)
			want := map[string]string{"return division": "account reduction", "return multiplication": "account reduction", "mark cash": "mark shape", "profit factor": "profit factor", "gross wins": "completion reduction", "gross losses": "completion reduction", "drawdown amount": "drawdown reduction", "drawdown percent": "drawdown reduction"}[c.name]
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("wrong overflow refusal: %v; want %s", err, want)
			}
		})
	}
	t.Run("exit credit accumulation", func(t *testing.T) {
		a := ordinaryProjectionPartialFixture()
		a.Events[1].ExitCredit = max
		a.Events[2].ExitCredit = max
		ordinaryProjectionRefused(t, a)
	})
	t.Run("completion subtraction", func(t *testing.T) {
		a := ordinaryProjectionPartialFixture()
		a.Events[0].Fee = max
		a.Events[1].ExitCredit = -max
		a.Events[2].ExitCredit = 0
		a.Marks[0].EntryFees = max
		a.Marks[1].EntryFees = max
		a.Marks[2].EntryFees = max
		a.EntryFees = max
		ordinaryProjectionRefused(t, a)
	})
	t.Run("entry fees accumulation", func(t *testing.T) {
		a := ordinaryProjectionCompletions(100, []float64{0, 0}, []float64{0, 0})
		a.Events[0].Fee = max
		a.Events[2].Fee = max
		a.Marks[0].EntryFees = max
		ordinaryProjectionRefused(t, a)
	})
	t.Run("exit fees accumulation", func(t *testing.T) {
		a := ordinaryProjectionPartialFixture()
		a.Events[1].Fee = max
		a.Events[2].Fee = max
		a.Marks[1].ExitFees = max
		ordinaryProjectionRefused(t, a)
	})
}
