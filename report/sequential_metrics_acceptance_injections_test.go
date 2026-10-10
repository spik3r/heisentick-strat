package report

// HT-250 injected-only cases for ProjectSequentialMetrics. Each test builds a
// ledger by hand and then injects a state that a valid OHLC run cannot reach.
// Expected values come from docs/sequential-metrics-contract.md and arithmetic
// on invented numbers; none was copied from implementation output. The ledger
// builder below is independent of the existing sequentialMetricTestLedger.

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
)

type ht250Trade struct {
	fee, credit float64
	hold        int
}

// ht250Ledger returns a consistent run and accounting for back-to-back long
// trades: bars are consumed entry..exit per trade, flat marks while held.
func ht250Ledger(start float64, trades ...ht250Trade) (engine.RunResult, engine.SequentialAccounting) {
	run := engine.RunResult{Trades: []engine.Trade{}}
	a := engine.SequentialAccounting{StartEquity: start, Trades: []engine.SequentialTradeAccounting{}, Events: []engine.SequentialAccountingEvent{}}
	realized := 0.0
	mark := func(realized float64) {
		i := len(a.Marks)
		a.Marks = append(a.Marks, engine.SequentialEquityMark{Index: i, T: float64(i + 1), Realized: realized, Equity: start + realized})
	}
	for i, tr := range trades {
		entry := len(a.Marks)
		exit := entry + tr.hold
		before := realized
		realized -= tr.fee
		a.Events = append(a.Events, engine.SequentialAccountingEvent{Kind: "entry", TradeIndex: i, Index: entry, T: float64(entry + 1), RealizedBefore: before, RealizedAfter: realized, Amount: tr.fee})
		for bar := entry; bar < exit; bar++ {
			mark(realized)
		}
		before = realized
		realized += tr.credit
		a.Events = append(a.Events, engine.SequentialAccountingEvent{Kind: "exit", TradeIndex: i, Index: exit, T: float64(exit + 1), RealizedBefore: before, RealizedAfter: realized, Amount: tr.credit})
		mark(realized)
		a.Trades = append(a.Trades, engine.SequentialTradeAccounting{
			TradeIndex: i, EntryIndex: entry, ExitIndex: exit, EntryT: float64(entry + 1), ExitT: float64(exit + 1),
			Side: "long", Entry: 1, Exit: 1, Size: 1, EntryFee: tr.fee, ExitFee: tr.fee, ExitCredit: tr.credit, NetPnL: tr.credit - tr.fee,
		})
		run.Trades = append(run.Trades, engine.Trade{EntryIndex: entry, ExitIndex: exit, EntryT: float64(entry + 1), ExitT: float64(exit + 1), Side: "long", PnL: tr.credit})
	}
	if len(trades) == 0 {
		mark(0)
	}
	run.TradeCount = len(run.Trades)
	a.FinalRealized, a.EndEquity = realized, start+realized
	return run, a
}

func ht250Project(t *testing.T, run engine.RunResult, a engine.SequentialAccounting) SequentialMetrics {
	t.Helper()
	m, err := ProjectSequentialMetrics(run, a)
	if err != nil {
		t.Fatalf("consistent ledger refused: %v", err)
	}
	return m
}

// ht250RequireRefusal asserts a typed refusal naming the guard that fired, the
// index it reports, and no partial metrics. Naming the field keeps one guard
// from masking the removal of another.
func ht250RequireRefusal(t *testing.T, run engine.RunResult, a engine.SequentialAccounting, field string, index int) {
	t.Helper()
	m, err := ProjectSequentialMetrics(run, a)
	var typed *SequentialMetricsError
	if !errors.As(err, &typed) || typed.Kind != "invalid-sequential-metrics" {
		t.Fatalf("want typed refusal %q, got %+v, %v", field, m, err)
	}
	if typed.Field != field || typed.Index != index {
		t.Fatalf("refusal = %q at %d, want %q at %d", typed.Field, typed.Index, field, index)
	}
	if !reflect.DeepEqual(m, SequentialMetrics{}) {
		t.Fatalf("partial metrics escaped a refusal: %+v", m)
	}
}

// Three trades, six events, six marks. Credits and fees are dyadic.
func ht250Three() (engine.RunResult, engine.SequentialAccounting) {
	return ht250Ledger(100, ht250Trade{fee: 1, credit: 3, hold: 1}, ht250Trade{fee: .5, credit: .25}, ht250Trade{credit: -2, hold: 2})
}

func TestHT250ControlLedger(t *testing.T) {
	// Hand derivation, start 100: nets 3-1=2, 0.25-0.5=-0.25, -2-0=-2; account
	// net -0.25. Marks: 99, 102, 101.75, 101.75, 101.75, 99.75. Peak 102, low
	// 99.75: drawdown 2.25. Streaks: win, then two non-winners.
	run, a := ht250Three()
	h := ht250Project(t, run, a).Headline
	if h.Net != -.25 || h.EndEquity != 99.75 || h.Trades != 3 || h.WorstLoss != -2 || h.MaxWinStreak != 1 || h.MaxLossStreak != 2 || h.MaxDD != 2.25 {
		t.Fatalf("control headline: %+v", h)
	}
}

// inj.realized_chain_break: the second snapshot starts at a value other than
// the first snapshot's result. Break each of the six links in turn by one ulp,
// by changing a "before" and, separately, an "after".
func TestHT250RealizedChainBreakAtEveryLink(t *testing.T) {
	for k := 0; k < 6; k++ {
		t.Run("before", func(t *testing.T) {
			run, a := ht250Three()
			a.Events[k].RealizedBefore = math.Nextafter(a.Events[k].RealizedBefore, math.Inf(1))
			ht250RequireRefusal(t, run, a, "observed event chain", k)
		})
		t.Run("after", func(t *testing.T) {
			run, a := ht250Three()
			a.Events[k].RealizedAfter = math.Nextafter(a.Events[k].RealizedAfter, math.Inf(1))
			if k == 5 {
				// No successor event; the break shows against the terminal state.
				ht250RequireRefusal(t, run, a, "terminal observed state", -1)
			} else {
				ht250RequireRefusal(t, run, a, "observed event chain", k+1)
			}
		})
	}
}

// inj.nonfinite_equity_sum: finite cash and finite unrealized P&L whose sum
// overflows. A mark may not carry Infinity, a clamped maximum, or 0. Other mark
// checks also reject a nonfinite value, so the engine test is the primary
// check of the overflow guard; this one pins the substitute-value defects.
func TestHT250NonfiniteEquitySumNeverBecomesZeroOrInfinity(t *testing.T) {
	const big = 1.7e308
	for name, equity := range map[string]float64{
		"Infinity": math.Inf(1), "clamped to max": math.MaxFloat64, "zero": 0, "NaN": math.NaN(),
	} {
		t.Run(name, func(t *testing.T) {
			run, a := ht250Ledger(big, ht250Trade{hold: 1})
			a.Marks[0].Unrealized, a.Marks[0].Equity = big, equity
			field := "equity mark reconciliation"
			if math.IsInf(equity, 0) || math.IsNaN(equity) {
				field = "raw equity mark"
			}
			ht250RequireRefusal(t, run, a, field, 0)
		})
	}
	t.Run("sum just below overflow is accepted", func(t *testing.T) {
		const half = 8e307 // half+half = 1.6e308 exactly
		run, a := ht250Ledger(half, ht250Trade{hold: 1})
		a.Marks[0].Unrealized, a.Marks[0].Equity = half, 2*half
		m := ht250Project(t, run, a)
		// Peak 1.6e308, then 8e307: drawdown 8e307 is exactly 50 percent.
		if m.Equity[0].Equity != 2*half || m.Headline.MaxDD != half || m.Headline.MaxDDpct != 50 {
			t.Fatalf("finite large sum: %+v", m)
		}
	})
}

// inj.drawdown_nonfinite. The field name is pinned because a later headline
// check would also refuse this ledger; the contract outcome is the refusal and
// the pin shows the drawdown guard itself fired. Reachable from a consistent ledger: a tiny positive
// start and a large loss make the percentage overflow while the currency
// drawdown is finite. Peak 0 is refused earlier (startEquity) and -Infinity
// equity is refused as a raw mark, so those two never reach the division.
func TestHT250DrawdownPercentOverflowRefuses(t *testing.T) {
	run, a := ht250Ledger(1e-300, ht250Trade{credit: -1e10})
	ht250RequireRefusal(t, run, a, "drawdown", 0)
	// The same shape that stays finite: lose the whole account, 100 percent.
	run, a = ht250Ledger(1e-300, ht250Trade{credit: -1e-300})
	h := ht250Project(t, run, a).Headline
	if h.MaxDDpct != 100 || h.MaxDD != 1e-300 || h.EndEquity != 0 {
		t.Fatalf("whole-account loss: %+v", h)
	}
	run, a = ht250Ledger(100, ht250Trade{credit: -1})
	a.Marks[0].Equity, a.Marks[0].Realized = math.Inf(-1), math.Inf(-1)
	ht250RequireRefusal(t, run, a, "raw equity mark", 0)
}

// inj.alignment_mismatch: tradeAccounting, run.trades, events and marks must
// line up. Drop one record from each list in turn. The metrics projection has
// no bar count; marks versus barCount is checked at the collector (see the
// engine test) and the bridge.
func TestHT250AlignmentMismatchRefusesEachDroppedRecord(t *testing.T) {
	for k := 0; k < 3; k++ {
		run, a := ht250Three()
		run.Trades = append(run.Trades[:k:k], run.Trades[k+1:]...)
		ht250RequireRefusal(t, run, a, "accounting counts", -1) // TradeCount still says 3
		run.TradeCount = 2
		ht250RequireRefusal(t, run, a, "accounting counts", -1) // run self-consistent, accounting is not

		run, a = ht250Three()
		a.Trades = append(a.Trades[:k:k], a.Trades[k+1:]...)
		ht250RequireRefusal(t, run, a, "accounting counts", -1)
	}
	for k := 0; k < 6; k++ {
		run, a := ht250Three()
		a.Events = append(a.Events[:k:k], a.Events[k+1:]...)
		ht250RequireRefusal(t, run, a, "accounting counts", -1)

		// Which guard sees a dropped mark first depends on its position, so only
		// the typed whole-run refusal is asserted.
		run, a = ht250Three()
		a.Marks = append(a.Marks[:k:k], a.Marks[k+1:]...)
		m, err := ProjectSequentialMetrics(run, a)
		var typed *SequentialMetricsError
		if !errors.As(err, &typed) || !reflect.DeepEqual(m, SequentialMetrics{}) {
			t.Fatalf("dropping mark %d was accepted: %+v %v", k, m, err)
		}
	}
	// A surplus, well-formed entry/exit pair beyond the last mark: the event
	// count is even, so only the events-versus-trades comparison can refuse it.
	run, a := ht250Three()
	a.Events = append(a.Events,
		engine.SequentialAccountingEvent{Kind: "entry", TradeIndex: 3, Index: 6, T: 7, RealizedBefore: a.FinalRealized, RealizedAfter: a.FinalRealized},
		engine.SequentialAccountingEvent{Kind: "exit", TradeIndex: 3, Index: 7, T: 8, RealizedBefore: a.FinalRealized, RealizedAfter: a.FinalRealized})
	ht250RequireRefusal(t, run, a, "accounting counts", -1)
	// Same-length lists whose indices disagree with their position.
	run, a = ht250Three()
	run.TradeCount++
	ht250RequireRefusal(t, run, a, "accounting counts", -1)
	run, a = ht250Three()
	a.Marks[2].Index = 3
	ht250RequireRefusal(t, run, a, "raw equity mark", 2)
	run, a = ht250Three()
	a.Trades[1].TradeIndex = 2
	ht250RequireRefusal(t, run, a, "trade identity", 1)
	run, a = ht250Three()
	a.Events[3].TradeIndex = 2
	ht250RequireRefusal(t, run, a, "observed event chain", 3)
}

// ht250NegativeZeros collects every JSON number that is a negative zero.
func ht250NegativeZeros(t *testing.T, value any, path string, found *[]string) {
	t.Helper()
	switch v := value.(type) {
	case float64:
		if v == 0 && math.Signbit(v) {
			*found = append(*found, path)
		}
	case map[string]any:
		for k, child := range v {
			ht250NegativeZeros(t, child, path+"."+k, found)
		}
	case []any:
		for _, child := range v {
			ht250NegativeZeros(t, child, path+"[]", found)
		}
	}
}

// inj.negative_zero_fields: raw -0 reaches the fees, nets and realized chain,
// and through them every derived headline number (the time test below covers
// equity[].t). The output must equal the +0 projection byte for byte.
func TestHT250NegativeZeroIsNeverSerialized(t *testing.T) {
	nz := math.Copysign(0, -1)
	baseRun, baseA := ht250Ledger(100, ht250Trade{hold: 1})
	baseline, err := json.Marshal(ht250Project(t, baseRun, baseA))
	if err != nil {
		t.Fatal(err)
	}
	for name, inject := range map[string]func(*engine.SequentialAccounting){
		"entry fee": func(a *engine.SequentialAccounting) {
			a.Trades[0].EntryFee, a.Events[0].Amount = nz, nz
		},
		"exit fee": func(a *engine.SequentialAccounting) { a.Trades[0].ExitFee = nz },
		"exit credit and net": func(a *engine.SequentialAccounting) {
			a.Trades[0].ExitCredit, a.Trades[0].NetPnL, a.Events[1].Amount = nz, nz, nz
		},
		"whole chain": func(a *engine.SequentialAccounting) {
			a.Events[0].RealizedAfter = nz
			a.Events[1].RealizedBefore, a.Events[1].RealizedAfter, a.Events[1].Amount = nz, nz, nz
			a.Trades[0].ExitCredit, a.Trades[0].NetPnL = nz, nz
			a.Marks[0].Realized, a.Marks[1].Realized, a.FinalRealized = nz, nz, nz
		},
	} {
		t.Run(name, func(t *testing.T) {
			run, a := ht250Ledger(100, ht250Trade{hold: 1})
			inject(&a)
			encoded, err := json.Marshal(ht250Project(t, run, a))
			if err != nil {
				t.Fatal(err)
			}
			var tree any
			if err := json.Unmarshal(encoded, &tree); err != nil {
				t.Fatal(err)
			}
			var found []string
			ht250NegativeZeros(t, tree, "$", &found)
			if len(found) != 0 || string(encoded) != string(baseline) {
				t.Fatalf("negative zero escaped at %v\n got  %s\n want %s", found, encoded, baseline)
			}
		})
	}
}

// inj.tiny_positive_net: +/-5e-324 is a win or a loss, never flat. No epsilon.
func TestHT250SubnormalNetClassification(t *testing.T) {
	const tiny = math.SmallestNonzeroFloat64
	win := ht250Trade{fee: tiny, credit: 2 * tiny}  // net +tiny
	loss := ht250Trade{fee: 2 * tiny, credit: tiny} // net -tiny
	t.Run("positive", func(t *testing.T) {
		run, a := ht250Ledger(100, win)
		h := ht250Project(t, run, a).Headline
		if h.Net != tiny || *h.WinRate != 100 || h.MaxWinStreak != 1 || h.MaxLossStreak != 0 || *h.AvgWin != tiny || h.AvgLoss != nil || h.AvgLossReason != "no-nonwinning-trades" || h.ProfitFactor != nil || h.ProfitFactorReason != "no-losses" || h.WorstLoss != 0 {
			t.Fatalf("+5e-324: %+v", h)
		}
	})
	t.Run("negative", func(t *testing.T) {
		run, a := ht250Ledger(100, loss)
		h := ht250Project(t, run, a).Headline
		if h.Net != -tiny || *h.WinRate != 0 || h.MaxWinStreak != 0 || h.MaxLossStreak != 1 || *h.AvgLoss != -tiny || h.AvgWin != nil || h.AvgWinReason != "no-winning-trades" || h.WorstLoss != -tiny || *h.ProfitFactor != 0 || h.ProfitFactorReason != "" {
			t.Fatalf("-5e-324: %+v", h)
		}
	})
	t.Run("equal and opposite", func(t *testing.T) {
		run, a := ht250Ledger(100, win, loss)
		h := ht250Project(t, run, a).Headline
		// One winner of 5e-324, one loser of 5e-324: PF is exactly 1, net 0.
		if h.Net != 0 || *h.WinRate != 50 || *h.ProfitFactor != 1 || *h.AvgWin != tiny || *h.AvgLoss != -tiny || h.WorstLoss != -tiny || h.MaxWinStreak != 1 || h.MaxLossStreak != 1 {
			t.Fatalf("+/-5e-324: %+v", h)
		}
	})
}

// inj.invalid_effective_start at the projection boundary: the start equity must
// be finite and strictly positive.
func TestHT250InvalidEffectiveStartRefused(t *testing.T) {
	for name, start := range map[string]float64{
		"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1), "zero": 0, "negative zero": math.Copysign(0, -1), "negative": -1,
	} {
		t.Run(name, func(t *testing.T) {
			run, a := ht250Ledger(100, ht250Trade{credit: 1})
			a.StartEquity = start
			ht250RequireRefusal(t, run, a, "startEquity", -1)
		})
	}
}

func TestHT250NegativeZeroTimestampIsNeverSerialized(t *testing.T) {
	nz := math.Copysign(0, -1)
	encode := func(zero float64) string {
		run, a := ht250Ledger(100, ht250Trade{hold: 1})
		a.Marks[0].T, a.Trades[0].EntryT, a.Events[0].T = zero, zero, zero
		run.Trades[0].EntryT = zero
		encoded, err := json.Marshal(ht250Project(t, run, a))
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	if got, want := encode(nz), encode(0); got != want {
		t.Fatalf("-0 timestamp serialized differently:\n got  %s\n want %s", got, want)
	}
}
