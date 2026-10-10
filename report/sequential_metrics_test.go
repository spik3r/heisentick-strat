package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
)

type sequentialMetricTestTrade struct {
	fee, credit float64
	hold        int
}

// This hand-built ledger is independent of strategy execution and the shared
// report reducer. Entries and exits are the broker's two separate mutations.
func sequentialMetricTestLedger(start float64, inputs ...sequentialMetricTestTrade) (engine.RunResult, engine.SequentialAccounting) {
	run := engine.RunResult{Costs: engine.Costs{StartEquity: start}, Trades: []engine.Trade{}}
	a := engine.SequentialAccounting{StartEquity: start, Trades: []engine.SequentialTradeAccounting{}, Events: []engine.SequentialAccountingEvent{}}
	realized := 0.0
	for i, input := range inputs {
		entry := len(a.Marks)
		exit := entry + input.hold
		entryT, exitT := float64(entry+1), float64(exit+1)
		before := realized
		realized -= input.fee
		a.Events = append(a.Events, engine.SequentialAccountingEvent{Kind: "entry", TradeIndex: i, Index: entry, T: entryT, RealizedBefore: before, RealizedAfter: realized, Amount: input.fee})
		for bar := entry; bar < exit; bar++ {
			a.Marks = append(a.Marks, engine.SequentialEquityMark{Index: bar, T: float64(bar + 1), Realized: realized, Equity: start + realized})
		}
		before = realized
		realized += input.credit
		a.Events = append(a.Events, engine.SequentialAccountingEvent{Kind: "exit", TradeIndex: i, Index: exit, T: exitT, RealizedBefore: before, RealizedAfter: realized, Amount: input.credit})
		a.Marks = append(a.Marks, engine.SequentialEquityMark{Index: exit, T: exitT, Realized: realized, Equity: start + realized})
		a.Trades = append(a.Trades, engine.SequentialTradeAccounting{
			TradeIndex: i, EntryIndex: entry, ExitIndex: exit, EntryT: entryT, ExitT: exitT,
			Side: "long", Entry: 1, Exit: 1, Size: 1, EntryFee: input.fee, ExitFee: input.fee,
			ExitCredit: input.credit, NetPnL: input.credit - input.fee,
		})
		run.Trades = append(run.Trades, engine.Trade{EntryIndex: entry, ExitIndex: exit, EntryT: entryT, ExitT: exitT, Side: "long", PnL: input.credit})
	}
	if len(inputs) == 0 {
		for i := 0; i < 3; i++ {
			a.Marks = append(a.Marks, engine.SequentialEquityMark{Index: i, T: float64(i + 1), Equity: start})
		}
	}
	run.TradeCount = len(run.Trades)
	a.FinalRealized = realized
	a.EndEquity = start + realized
	return run, a
}

func mustSequentialMetrics(t *testing.T, run engine.RunResult, a engine.SequentialAccounting) SequentialMetrics {
	t.Helper()
	metrics, err := ProjectSequentialMetrics(run, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(metrics); err != nil {
		t.Fatalf("projection is not finite JSON: %v", err)
	}
	return metrics
}

func assertSequentialMetricPointer(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %g", name, got, want)
	}
}

func TestSequentialMetricsIndependentCurrencyAndPercentDrawdowns(t *testing.T) {
	run, a := sequentialMetricTestLedger(100,
		sequentialMetricTestTrade{credit: -20}, sequentialMetricTestTrade{credit: 120}, sequentialMetricTestTrade{credit: -30})
	m := mustSequentialMetrics(t, run, a)
	h := m.Headline
	if h.StartEquity != 100 || h.EndEquity != 170 || h.Net != 70 || h.ReturnPct != 70 || h.MaxDD != 30 || h.MaxDDpct != 20 {
		t.Fatalf("100 -> 80 -> 200 -> 170 headline = %+v", h)
	}
	if m.Basis != SequentialMetricsBasis || len(m.Equity) != 3 || m.Equity[2].Equity != h.EndEquity {
		t.Fatalf("basis/curve = %+v", m)
	}
	assertSequentialMetricPointer(t, "win rate percentage", h.WinRate, (1/float64(len(a.Trades)))*100)
	assertSequentialMetricPointer(t, "profit factor", h.ProfitFactor, 120.0/50)
	assertSequentialMetricPointer(t, "expectancy", h.Expectancy, 70.0/3)
	assertSequentialMetricPointer(t, "average winner", h.AvgWin, 120)
	assertSequentialMetricPointer(t, "average non-winner", h.AvgLoss, -25)
	if h.WorstLoss != -30 || h.MaxWinStreak != 1 || h.MaxLossStreak != 1 {
		t.Fatalf("loss/streak values = %+v", h)
	}
}

func TestSequentialMetricsNoTradesAndCustomStart(t *testing.T) {
	for _, start := range []float64{100, 12345.678901234567, math.SmallestNonzeroFloat64, math.MaxFloat64} {
		t.Run(fmtFloatForSequentialTest(start), func(t *testing.T) {
			run, a := sequentialMetricTestLedger(start)
			m := mustSequentialMetrics(t, run, a)
			h := m.Headline
			if h.StartEquity != start || h.EndEquity != start || h.Net != 0 || h.ReturnPct != 0 || h.Trades != 0 || h.MaxDD != 0 || h.MaxDDpct != 0 || h.MaxWinStreak != 0 || h.MaxLossStreak != 0 || h.WorstLoss != 0 {
				t.Fatalf("no-trade scalars = %+v", h)
			}
			if h.WinRate != nil || h.ProfitFactor != nil || h.Expectancy != nil || h.AvgWin != nil || h.AvgLoss != nil || h.AvgHoldBars != nil {
				t.Fatalf("no-trade ratios must be null: %+v", h)
			}
			for _, reason := range []string{h.WinRateReason, h.ProfitFactorReason, h.ExpectancyReason, h.AvgHoldBarsReason} {
				if reason != "no-trades" {
					t.Fatalf("no-trade reason = %q", reason)
				}
			}
			if h.AvgWinReason != "no-winning-trades" || h.AvgLossReason != "no-nonwinning-trades" {
				t.Fatalf("empty class-average reasons = %+v", h)
			}
			for i, mark := range m.Equity {
				if mark.Index != i || mark.Equity != start {
					t.Fatalf("no-trade curve changed: %+v", m.Equity)
				}
			}
		})
	}
}

func fmtFloatForSequentialTest(x float64) string {
	b, _ := json.Marshal(x)
	return string(b)
}

func TestSequentialMetricsClassificationNullsAndZeroDuration(t *testing.T) {
	tests := []struct {
		name                   string
		credits                []float64
		wins, maxWin, maxLoss  int
		pf                     *float64
		pfReason, avgWinReason string
		avgLossReason          string
	}{
		{"all flat", []float64{0, 0, 0}, 0, 0, 3, sequentialMetricPtr(0), "", "no-winning-trades", ""},
		{"winners and flat", []float64{2, 0, 3, 0}, 2, 1, 1, nil, "no-losses", "", ""},
		{"all winners", []float64{2, 3}, 2, 2, 0, nil, "no-losses", "", "no-nonwinning-trades"},
		{"native order", []float64{2, 3, -1, 0, -2, 4}, 3, 2, 3, sequentialMetricPtr(3), "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inputs := make([]sequentialMetricTestTrade, len(tc.credits))
			for i, credit := range tc.credits {
				inputs[i].credit = credit
			}
			run, a := sequentialMetricTestLedger(100, inputs...)
			h := mustSequentialMetrics(t, run, a).Headline
			if h.Trades != len(inputs) || h.MaxWinStreak != tc.maxWin || h.MaxLossStreak != tc.maxLoss {
				t.Fatalf("exact counts/streaks = %+v", h)
			}
			assertSequentialMetricPointer(t, "win rate", h.WinRate, (float64(tc.wins)/float64(len(inputs)))*100)
			assertSequentialMetricPointer(t, "same-bar hold", h.AvgHoldBars, 0)
			if !reflect.DeepEqual(h.ProfitFactor, tc.pf) || h.ProfitFactorReason != tc.pfReason || h.AvgWinReason != tc.avgWinReason || h.AvgLossReason != tc.avgLossReason {
				t.Fatalf("null/ratio contract = %+v", h)
			}
			if tc.name == "native order" {
				assertSequentialMetricPointer(t, "flats in non-winning average", h.AvgLoss, -1)
			}
			if tc.name == "all flat" || tc.name == "winners and flat" {
				assertSequentialMetricPointer(t, "flat average", h.AvgLoss, 0)
				if h.WorstLoss != 0 {
					t.Fatalf("no losing trade worstLoss = %g", h.WorstLoss)
				}
			}
		})
	}
}

func TestSequentialMetricsFeesAndNearZeroSignsAreExact(t *testing.T) {
	up, down := math.Nextafter(1, 2), math.Nextafter(1, 0)
	run, a := sequentialMetricTestLedger(100,
		sequentialMetricTestTrade{fee: 1, credit: up, hold: 2},
		sequentialMetricTestTrade{fee: 1, credit: 1, hold: 0},
		sequentialMetricTestTrade{fee: 1, credit: down, hold: 1},
		sequentialMetricTestTrade{fee: 2, credit: 1, hold: 3})
	// The retained record's positive exit credit cannot establish all-fee wins.
	m := mustSequentialMetrics(t, run, a)
	h := m.Headline
	assertSequentialMetricPointer(t, "exact sign win rate", h.WinRate, 25)
	assertSequentialMetricPointer(t, "duration in original bars", h.AvgHoldBars, 1.5)
	if h.MaxWinStreak != 1 || h.MaxLossStreak != 3 || h.WorstLoss != -1 || h.Trades != 4 {
		t.Fatalf("fee/sign classification = %+v", h)
	}
	for i, want := range []float64{up - 1, 0, down - 1, -1} {
		if m.TradeAccounting[i].Index != i || m.TradeAccounting[i].NetPnL != want || m.TradeAccounting[i].EntryFee != a.Trades[i].EntryFee || m.TradeAccounting[i].ExitFee != a.Trades[i].ExitFee {
			t.Fatalf("raw accounting[%d] = %+v, want net %g", i, m.TradeAccounting[i], want)
		}
	}
}

func TestSequentialMetricsNegativeEquityIsValid(t *testing.T) {
	run, a := sequentialMetricTestLedger(100, sequentialMetricTestTrade{credit: -150})
	h := mustSequentialMetrics(t, run, a).Headline
	if h.EndEquity != -50 || h.MaxDD != 150 || h.MaxDDpct != 150 || h.ReturnPct != -150 {
		t.Fatalf("finite negative-equity metrics = %+v", h)
	}
}

func TestSequentialMetricsCloseMarksIncludeObservedUnrealized(t *testing.T) {
	run, a := sequentialMetricTestLedger(100, sequentialMetricTestTrade{fee: 2, credit: 12, hold: 2})
	a.Marks[0].Unrealized, a.Marks[0].Equity = -8, 90
	a.Marks[1].Unrealized, a.Marks[1].Equity = 22, 120
	m := mustSequentialMetrics(t, run, a)
	if len(m.Equity) != 3 || m.Equity[0].Equity != 90 || m.Equity[1].Equity != 120 || m.Equity[2].Equity != 110 || m.Headline.Net != 10 || m.Headline.MaxDD != 10 || m.Headline.MaxDDpct != 10 {
		t.Fatalf("close MTM marks = %+v", m)
	}
	// A numerically self-consistent mark cannot claim unrealized equity on
	// a bar whose complete event chain has already closed the position.
	a.Marks[2].Unrealized, a.Marks[2].Equity = 1, 111
	assertSequentialMetricsRefusal(t, run, a)
}

func TestSequentialMetricsRetainsTinyAccountNetAfterLargeHistory(t *testing.T) {
	run, a := sequentialMetricTestLedger(1e16,
		sequentialMetricTestTrade{credit: 1e16}, sequentialMetricTestTrade{credit: -1e16}, sequentialMetricTestTrade{credit: 1e-9})
	m := mustSequentialMetrics(t, run, a)
	if m.Headline.Net != 1e-9 || m.Headline.EndEquity != m.Headline.StartEquity || m.Equity[len(m.Equity)-1].Equity != m.Headline.EndEquity {
		t.Fatalf("raw account net was inferred from rounded equity: %+v", m.Headline)
	}
	wantNet := 1e-9
	assertSequentialMetricPointer(t, "raw account expectancy", m.Headline.Expectancy, wantNet/3)
}

func TestSequentialMetricsLongCancellationReconciliation(t *testing.T) {
	inputs := make([]sequentialMetricTestTrade, 0, 10000)
	for i := 0; i < 5000; i++ {
		inputs = append(inputs, sequentialMetricTestTrade{credit: 1e16}, sequentialMetricTestTrade{fee: 1e16, credit: 1})
	}
	run, a := sequentialMetricTestLedger(1e16, inputs...)
	m := mustSequentialMetrics(t, run, a)
	tradeSum := 0.0
	for _, trade := range m.TradeAccounting {
		tradeSum += trade.NetPnL
	}
	if m.Headline.Net != 1 || tradeSum != 0 || m.Headline.Trades != 10000 || m.Headline.MaxWinStreak != 1 || m.Headline.MaxLossStreak != 1 {
		t.Fatalf("long cancellation: account=%g trades=%g headline=%+v", m.Headline.Net, tradeSum, m.Headline)
	}
}

func TestSequentialMetricsAcceptsObservedFusedEntryWithoutUnfusedReplay(t *testing.T) {
	run, a := sequentialMetricTestLedger(100, sequentialMetricTestTrade{credit: 1}, sequentialMetricTestTrade{fee: 1})
	// (1+2^-52)*(1-2^-52) rounds to 1, while the original fused
	// 1 - product mutation retains 2^-104. The nominal fee stays exactly 1.
	feePerUnit, size := 1+math.Ldexp(1, -52), 1-math.Ldexp(1, -52)
	residual := math.FMA(-feePerUnit, size, 1)
	if residual != math.Ldexp(1, -104) || float64(feePerUnit*size) != 1 {
		t.Fatal("invalid independent FMA vector")
	}
	a.Events[2].RealizedAfter = residual
	a.Events[3].RealizedBefore, a.Events[3].RealizedAfter = residual, residual
	a.Marks[1].Realized, a.Marks[1].Equity = residual, 100+residual
	a.FinalRealized, a.EndEquity = residual, 100+residual
	m := mustSequentialMetrics(t, run, a)
	if m.Headline.Net != residual {
		t.Fatalf("fused observed residual lost: %g", m.Headline.Net)
	}
}

func TestSequentialMetricsSubnormalAndMaxFloatBounds(t *testing.T) {
	for _, value := range []float64{0, math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64, math.MaxFloat64, -math.MaxFloat64} {
		unit, err := sequentialRoundingUnit(value)
		if err != nil || !sequentialMetricFinite(unit) || unit <= 0 {
			t.Fatalf("finite rounding unit(%g) = %g, %v", value, unit, err)
		}
	}
	unit, _ := sequentialRoundingUnit(math.MaxFloat64)
	if unit != math.Ldexp(1, 971) {
		t.Fatalf("MaxFloat64 rounding unit = %g", unit)
	}
	tiny, _ := sequentialRoundingUnit(0)
	if tiny != math.SmallestNonzeroFloat64 {
		t.Fatalf("zero unit = %g, want minimum subnormal only", tiny)
	}
	budget := math.MaxFloat64
	if err := addSequentialRoundingBound(&budget, math.MaxFloat64); err == nil || !sequentialMetricFinite(budget) {
		t.Fatalf("unrepresentable budget must refuse without writing Inf: %g, %v", budget, err)
	}
	run, a := sequentialMetricTestLedger(1, sequentialMetricTestTrade{credit: math.SmallestNonzeroFloat64})
	m := mustSequentialMetrics(t, run, a)
	if m.Headline.Net != math.SmallestNonzeroFloat64 || m.TradeAccounting[0].NetPnL <= 0 || m.Headline.MaxWinStreak != 1 || m.Headline.MaxLossStreak != 0 {
		t.Fatalf("subnormal sign/count lost: %+v", m.Headline)
	}
	assertSequentialMetricPointer(t, "subnormal win rate", m.Headline.WinRate, 100)
}

func TestSequentialMetricsReconciliationHasNoAbsoluteEpsilonFloor(t *testing.T) {
	run, a := sequentialMetricTestLedger(1, sequentialMetricTestTrade{credit: 1e-250})
	a.FinalRealized = 2e-250
	a.Events[1].RealizedAfter = a.FinalRealized
	a.Marks[0].Realized = a.FinalRealized
	// Both start+net calculations round to one, so an equity-difference or
	// max(1,scale) check would miss this completely incorrect tiny ledger.
	assertSequentialMetricsRefusal(t, run, a)
}

func TestSequentialMetricsRejectsNonfiniteAndInconsistentAccounting(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*engine.RunResult, *engine.SequentialAccounting)
	}{
		{"missing marks", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Marks = nil }},
		{"nonpositive start", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.StartEquity = 0 }},
		{"nan start", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.StartEquity = math.NaN() }},
		{"count", func(r *engine.RunResult, _ *engine.SequentialAccounting) { r.TradeCount++ }},
		{"trade index", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Trades[0].TradeIndex++ }},
		{"trade order", func(r *engine.RunResult, _ *engine.SequentialAccounting) { r.Trades[0].EntryIndex++ }},
		{"raw nonfinite", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Trades[0].ExitFee = math.Inf(1) }},
		{"event continuity", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Events[1].RealizedBefore = 1 }},
		{"event identity", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Events[0].Kind = "exit" }},
		{"net mismatch", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Trades[0].NetPnL++ }},
		{"mark order", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Marks[0].Index++ }},
		{"mark timestamp", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Marks[0].T = math.Inf(1) }},
		{"mark unrealized", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Marks[0].Unrealized = math.NaN() }},
		{"mark realized", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Marks[0].Realized++ }},
		{"mark equity", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.Marks[0].Equity++ }},
		{"terminal realized", func(_ *engine.RunResult, a *engine.SequentialAccounting) { a.FinalRealized++ }},
		{"terminal exposure", func(_ *engine.RunResult, a *engine.SequentialAccounting) {
			a.Marks[0].Unrealized = 1
			a.Marks[0].Equity++
		}},
		{"beyond rounding budget", func(_ *engine.RunResult, a *engine.SequentialAccounting) {
			a.Events[1].RealizedAfter += 1
			a.FinalRealized += 1
			a.EndEquity += 1
			a.Marks[0].Realized += 1
			a.Marks[0].Equity += 1
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run, a := sequentialMetricTestLedger(100, sequentialMetricTestTrade{credit: 2})
			tc.mutate(&run, &a)
			assertSequentialMetricsRefusal(t, run, a)
		})
	}
}

func assertSequentialMetricsRefusal(t *testing.T, run engine.RunResult, a engine.SequentialAccounting) {
	t.Helper()
	m, err := ProjectSequentialMetrics(run, a)
	var typed *SequentialMetricsError
	if !errors.As(err, &typed) || typed.Kind != "invalid-sequential-metrics" || !reflect.DeepEqual(m, SequentialMetrics{}) {
		t.Fatalf("want typed refusal without partial metrics, got %+v, %v", m, err)
	}
}

func TestSequentialMetricsRejectsDerivedOverflow(t *testing.T) {
	tests := []struct {
		name  string
		start float64
		input []sequentialMetricTestTrade
	}{
		{"end equity", math.MaxFloat64, []sequentialMetricTestTrade{{credit: math.MaxFloat64}}},
		{"return ratio", math.SmallestNonzeroFloat64, []sequentialMetricTestTrade{{credit: 1}}},
		{"profit factor", math.MaxFloat64 / 2, []sequentialMetricTestTrade{{credit: math.MaxFloat64 / 4}, {credit: -math.SmallestNonzeroFloat64}}},
		{"gross reduction", math.MaxFloat64 / 4, []sequentialMetricTestTrade{{credit: math.MaxFloat64 / 2}, {credit: -math.MaxFloat64 / 2}, {credit: math.MaxFloat64 / 2}, {credit: -math.MaxFloat64 / 2}, {credit: math.MaxFloat64 / 2}}},
		{"drawdown", math.MaxFloat64 / 2, []sequentialMetricTestTrade{{credit: math.MaxFloat64 / 2}, {credit: -math.MaxFloat64}, {credit: -math.MaxFloat64 / 2}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run, a := sequentialMetricTestLedger(tc.start, tc.input...)
			assertSequentialMetricsRefusal(t, run, a)
		})
	}
}

func TestSequentialMetricsJSONRoundTripsBinary64WithoutChangingRun(t *testing.T) {
	run, a := sequentialMetricTestLedger(100, sequentialMetricTestTrade{credit: math.Nextafter(1, 2)})
	before, _ := json.Marshal(run)
	m := mustSequentialMetrics(t, run, a)
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SequentialMetrics
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Headline.Net != a.FinalRealized || decoded.TradeAccounting[0].NetPnL != a.Trades[0].NetPnL || decoded.Equity[0].Equity != decoded.Headline.EndEquity {
		t.Fatalf("binary64 values did not round-trip: %s", raw)
	}
	after, _ := json.Marshal(run)
	if string(before) != string(after) {
		t.Fatal("projection changed retained RunResult")
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	h := fields["headline"].(map[string]any)
	if _, exists := h["winRateReason"]; exists {
		t.Fatal("non-null metric has a reason")
	}
	if h["profitFactor"] != nil || h["profitFactorReason"] != "no-losses" || h["avgLoss"] != nil || h["avgLossReason"] != "no-nonwinning-trades" {
		t.Fatalf("JSON null reasons = %s", raw)
	}
}

func TestSequentialMetricsJSONNormalizesOnlyCompanionSignedZeros(t *testing.T) {
	negativeZero := math.Copysign(0, -1)
	run, a := sequentialMetricTestLedger(100, sequentialMetricTestTrade{})
	// A fused entry debit can retain negative zero even when its separately
	// rounded nominal fee is zero. Preserve that actual observed state chain.
	a.Events[0].RealizedAfter = negativeZero
	a.Events[1].RealizedBefore, a.Events[1].RealizedAfter = negativeZero, negativeZero
	a.Events[1].Amount = negativeZero
	a.Trades[0].ExitCredit, a.Trades[0].NetPnL = negativeZero, negativeZero
	a.Trades[0].ExitFee = negativeZero
	a.Marks[0].Realized, a.FinalRealized = negativeZero, negativeZero
	run.Trades[0].PnL = negativeZero
	rawBefore, _ := json.Marshal(a)
	runBefore, _ := json.Marshal(run)
	m := mustSequentialMetrics(t, run, a)
	encoded, _ := json.Marshal(m)
	if bytes.Contains(encoded, []byte(":-0")) || math.Signbit(m.Headline.Net) || math.Signbit(*m.Headline.AvgLoss) || math.Signbit(m.TradeAccounting[0].NetPnL) {
		t.Fatalf("companion retained a negative JSON zero: %s", encoded)
	}
	rawAfter, _ := json.Marshal(a)
	runAfter, _ := json.Marshal(run)
	if !bytes.Equal(rawBefore, rawAfter) || !bytes.Equal(runBefore, runAfter) || !math.Signbit(a.FinalRealized) || !math.Signbit(run.Trades[0].PnL) {
		t.Fatal("output normalization mutated captured facts or retained RunResult")
	}
	// Exercise every numeric companion boundary, including marks: valid
	// arithmetic generally makes a cancelled cash-equity zero positive already.
	m = SequentialMetrics{
		Headline: SequentialHeadline{
			StartEquity: negativeZero, EndEquity: negativeZero, Net: negativeZero, ReturnPct: negativeZero,
			WorstLoss: negativeZero, MaxDD: negativeZero, MaxDDpct: negativeZero,
			WinRate: sequentialMetricPtr(negativeZero), ProfitFactor: sequentialMetricPtr(negativeZero),
			Expectancy: sequentialMetricPtr(negativeZero), AvgWin: sequentialMetricPtr(negativeZero),
			AvgLoss: sequentialMetricPtr(negativeZero), AvgHoldBars: sequentialMetricPtr(negativeZero),
		},
		TradeAccounting: []SequentialTradeAccounting{{EntryFee: negativeZero, ExitFee: negativeZero, NetPnL: negativeZero}},
		Equity:          []SequentialEquityPoint{{T: negativeZero, Equity: negativeZero}},
	}
	normalizeSequentialOutputZeros(&m)
	encoded, _ = json.Marshal(m)
	if bytes.Contains(encoded, []byte(":-0")) {
		t.Fatalf("numeric boundary did not normalize signed zero: %s", encoded)
	}
}
