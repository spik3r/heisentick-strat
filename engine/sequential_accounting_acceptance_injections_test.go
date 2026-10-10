package engine

// HT-250 injected-only cases for the Sequential accounting collector. Each test
// drives a private seam with a state that valid OHLC input cannot produce.
// Expected values come from the written contract in
// docs/sequential-metrics-contract.md and hand arithmetic on invented, dyadic
// numbers; none was copied from implementation output except the sized quantity in the
// blocked-decision test, which is read from the audit and only multiplied by an
// exact power-of-two fee.

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/engine/seqcore"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func ht250Broker(costs Costs, closes ...float64) (*broker, *sequentialAccountingCollector) {
	bars := make([]marketdata.Bar, len(closes))
	for i, close := range closes {
		bars[i] = marketdata.Bar{T: float64(i), O: close, H: close, L: close, C: close, V: 1}
	}
	b := &broker{}
	b.reset(marketdata.SeriesFromBars(bars), contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: costs}, nil)
	collector := newSequentialAccountingCollector(b.costs.StartEquity)
	b.sequentialAccounting = collector
	return b, collector
}

// ht250RequireRefusal asserts the typed refusal and that nothing partial escapes.
func ht250RequireRefusal(t *testing.T, got SequentialAccounting, err error, field string) {
	t.Helper()
	var typed *SequentialAccountingError
	if !errors.As(err, &typed) || typed.Kind != "invalid-sequential-accounting" {
		t.Fatalf("want typed accounting refusal, got %v", err)
	}
	if typed.Field != field {
		t.Fatalf("refusal field = %q, want %q (a different guard fired)", typed.Field, field)
	}
	if !reflect.DeepEqual(got, SequentialAccounting{}) {
		t.Fatalf("partial accounting escaped a refusal: %+v", got)
	}
}

// Two trades on four bars. Hand derivation, start 1000, fee 0.25 per unit:
//
//	bar 0 long 2 @ 10, entry fee 0.5          realized -0.5, mark 999.5
//	bar 1 close @ 11: 1*2 - 0.5 = 1.5 credit  realized 1.0, net 1.0, mark 1001
//	bar 2 short 1 @ 11, entry fee 0.25        realized 0.75, mark 1000.75
//	bar 3 close @ 10.5: 0.5*1 - 0.25 = 0.25   realized 1.0, net 0, mark 1001
//
// tamper runs between the named steps so one broken link is injected per run.
func ht250TwoTradeFlow(tamper func(step string, b *broker)) (*sequentialAccountingCollector, *broker) {
	b, c := ht250Broker(Costs{StartEquity: 1000, FeePerUnit: .25}, 10, 11, 11, 10.5)
	at := func(step string) {
		if tamper != nil {
			tamper(step, b)
		}
	}
	b.openPosition(sideLong, 10, order{Size: 2, HasSize: true}, 0)
	c.mark(b, 0, false)
	at("before first exit")
	b.closePosition(11, 1, "t", "")
	at("after first exit")
	c.mark(b, 1, false)
	at("before second entry")
	b.openPosition(sideShort, 11, order{Size: 1, HasSize: true}, 2)
	c.mark(b, 2, false)
	b.closePosition(10.5, 3, "t", "")
	c.mark(b, 3, false)
	return c, b
}

func TestHT250RealizedChainControl(t *testing.T) {
	c, _ := ht250TwoTradeFlow(nil)
	got, err := c.finish(4, 2)
	if err != nil {
		t.Fatal(err)
	}
	wantEquity := []float64{999.5, 1001, 1000.75, 1001}
	for i, want := range wantEquity {
		if got.Marks[i].Equity != want {
			t.Fatalf("mark %d equity = %g, want %g", i, got.Marks[i].Equity, want)
		}
	}
	if got.FinalRealized != 1 || got.EndEquity != 1001 || got.Trades[0].NetPnL != 1 || got.Trades[1].NetPnL != 0 {
		t.Fatalf("control accounting: %+v", got)
	}
	// The chain itself: each snapshot starts at the previous snapshot's result.
	want := [][2]float64{{0, -.5}, {-.5, 1}, {1, .75}, {.75, 1}}
	for i, e := range got.Events {
		if e.RealizedBefore != want[i][0] || e.RealizedAfter != want[i][1] {
			t.Fatalf("event %d = %+v, want %v", i, e, want[i])
		}
	}
}

// inj.realized_chain_break: the second snapshot starts at a value other than
// the first snapshot's result. The break is injected at each link in turn,
// including the link between two trades and the link from the first exit to its
// own mark. Each must refuse the whole run.
func TestHT250RealizedChainBreakRefusesWholeRun(t *testing.T) {
	bump := func(b *broker) { b.realized = math.Nextafter(b.realized, math.Inf(1)) }
	for _, tc := range []struct {
		name, at, field string
	}{
		{"entry to exit of first trade", "before first exit", "realized continuity"},
		{"between trades", "before second entry", "realized continuity"},
		{"exit to its own mark", "after first exit", "mark realized continuity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := ht250TwoTradeFlow(func(step string, b *broker) {
				if step == tc.at {
					bump(b) // one ulp is enough: the chain is compared bitwise
				}
			})
			got, err := c.finish(4, 2)
			ht250RequireRefusal(t, got, err, tc.field)
		})
	}
}

// inj.nonfinite_equity_sum: finite cash and finite unrealized P&L whose sum
// overflows. The refusal must happen in the collector, before any mark exists.
func TestHT250NonfiniteEquitySumRefusesBeforeAnyMark(t *testing.T) {
	const big = 1.7e308
	for _, direction := range []side{sideLong, sideShort} {
		t.Run(direction.String(), func(t *testing.T) {
			// close 0 vs entry -direction*big gives unrealized +big either way.
			b, c := ht250Broker(Costs{StartEquity: big}, 0)
			b.openPosition(direction, -float64(direction)*big, order{Size: 1, HasSize: true}, 0)
			c.mark(b, 0, false)
			if len(c.data.Marks) != 0 {
				t.Fatalf("a mark was stored for an overflowing equity: %+v", c.data.Marks)
			}
			got, err := c.finish(1, 0)
			ht250RequireRefusal(t, got, err, "mark")
		})
	}
	t.Run("negative overflow", func(t *testing.T) {
		// Realized -big from a closed trade, then an open leg at -big. Cash is
		// 1 - big, so cash + unrealized is below -MaxFloat64.
		b, c := ht250Broker(Costs{StartEquity: 1}, 0)
		b.openPosition(sideLong, big, order{Size: 1, HasSize: true}, 0)
		b.closePosition(0, 0, "t", "")
		b.openPosition(sideLong, big, order{Size: 1, HasSize: true}, 0)
		c.mark(b, 0, false)
		got, err := c.finish(1, 1)
		ht250RequireRefusal(t, got, err, "mark")
	})
	t.Run("just below overflow is accepted", func(t *testing.T) {
		// 8e307 + 8e307 = 1.6e308 exactly (doubling), which is finite.
		const half = 8e307
		b, c := ht250Broker(Costs{StartEquity: half}, 0)
		b.openPosition(sideLong, -half, order{Size: 1, HasSize: true}, 0)
		c.mark(b, 0, false)
		if c.err != nil || len(c.data.Marks) != 1 || c.data.Marks[0].Equity != 2*half {
			t.Fatalf("finite sum refused or wrong: marks=%+v err=%v", c.data.Marks, c.err)
		}
	})
}

// inj.alignment_mismatch (engine side): marks vs bar count, trades vs the
// broker's closed trades, and events vs trades. One record is dropped from each
// list in turn.
func TestHT250AlignmentMismatchRefusesEachDroppedRecord(t *testing.T) {
	const field = "terminal count or position"
	control, _ := ht250TwoTradeFlow(nil)
	if _, err := control.finish(4, 2); err != nil {
		t.Fatalf("control flow refused: %v", err)
	}
	for _, tc := range []struct {
		name string
		drop func(*sequentialAccountingCollector)
	}{
		{"first mark", func(c *sequentialAccountingCollector) { c.data.Marks = c.data.Marks[1:] }},
		{"last mark", func(c *sequentialAccountingCollector) { c.data.Marks = c.data.Marks[:3] }},
		{"first trade", func(c *sequentialAccountingCollector) { c.data.Trades = c.data.Trades[1:] }},
		{"last trade", func(c *sequentialAccountingCollector) { c.data.Trades = c.data.Trades[:1] }},
		{"entry event", func(c *sequentialAccountingCollector) { c.data.Events = c.data.Events[1:] }},
		{"last event", func(c *sequentialAccountingCollector) { c.data.Events = c.data.Events[:3] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := ht250TwoTradeFlow(nil)
			tc.drop(c)
			got, err := c.finish(4, 2)
			ht250RequireRefusal(t, got, err, field)
		})
	}
	// Extra record on the other side of the comparison.
	t.Run("expected bar count larger", func(t *testing.T) {
		c, _ := ht250TwoTradeFlow(nil)
		got, err := c.finish(5, 2)
		ht250RequireRefusal(t, got, err, field)
	})
	t.Run("expected trade count larger", func(t *testing.T) {
		c, _ := ht250TwoTradeFlow(nil)
		got, err := c.finish(4, 3)
		ht250RequireRefusal(t, got, err, field)
	})
}

func ht250LegacySource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "conformance", "parse", "setup-legacy-setup9.strat"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// inj.invalid_effective_start: NaN or Infinity at the Go call boundary. Zero
// (either sign) is "omitted" and becomes 10000 before the finite check, so it
// is accepted; every other nonpositive or nonfinite value is refused with the
// startEquity field, not a neighbouring guard.
func TestHT250InvalidEffectiveStartAtCallBoundary(t *testing.T) {
	source := ht250LegacySource(t)
	base := RunFixture{Schema: runFixtureSchema, Case: "ht250-start", RangeMethod: "zone", StrategyID: "dslSequentialLegacySetup9", Symbol: "XAUUSD", Timeframe: "1h", Bars: []marketdata.Bar{{T: 0, O: 1, H: 1, L: 1, C: 1, V: 1}}}
	for name, start := range map[string]float64{
		"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1), "negative": -1, "negative subnormal": -math.SmallestNonzeroFloat64,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := base
			fixture.Costs.StartEquity = start
			run, got, err := RunSequentialFixtureWithAccounting(fixture, source)
			ht250RequireRefusal(t, got, err, "startEquity")
			if !reflect.DeepEqual(run, RunResult{}) {
				t.Fatalf("run escaped a refusal: %+v", run)
			}
		})
	}
	for name, start := range map[string]float64{"zero": 0, "negative zero": math.Copysign(0, -1)} {
		t.Run(name+" normalizes", func(t *testing.T) {
			fixture := base
			fixture.Costs.StartEquity = start
			_, got, err := RunSequentialFixtureWithAccounting(fixture, source)
			if err != nil || got.StartEquity != 10000 || got.EndEquity != 10000 {
				t.Fatalf("zero start: %+v %v", got, err)
			}
		})
	}
}

// inj.blocked_in_position: two Full decisions closer together than the hold.
// The scheduler's private acceptEvents seam takes the decisions; the rest is the
// ordinary run loop with a collector. Flat bars produce no natural decision, so
// every decision here is injected. Hand derivation (E1, hold 4):
//
//	decision A at bar 14 queues; the fill is bar 15's open; time exit is bar 19
//	decision C at bar 14, after A in a later call: A pending -> blocked_in_position
//	decision B at bar 15 after the fill: position open       -> blocked_in_position
//	decision D at bar 17: position open                      -> blocked_in_position
//
// The accounting contract says a blocked decision queues nothing, so the
// collector must equal an otherwise identical run that only had decision A.
func TestHT250BlockedDecisionsAddNoMarkTradeOrEvent(t *testing.T) {
	bars := make([]marketdata.Bar, 40)
	for i := range bars {
		bars[i] = marketdata.Bar{T: float64(i) * 300000, O: 100, H: 100.5, L: 99.5, C: 100, V: 1}
	}
	decision := func(i int, id string) []seqcore.Event {
		events := []seqcore.Event{}
		for _, kind := range []string{"setup_complete", "perfection"} {
			events = append(events, seqcore.Event{Type: kind, Side: "buy", EpisodeID: id, BarIndex: int64(i), BarOpenMS: int64(bars[i].T), DecisionMS: int64(bars[i].T) + 300000})
		}
		return events
	}
	run := func(t *testing.T, extra map[int][]string) (SequentialAccounting, SequentialFullAudit) {
		t.Helper()
		const fee = .015625
		s, err := newSequentialFullScheduler(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open", StartEquity: 1000, FeePerUnit: fee}})
		if err != nil {
			t.Fatal(err)
		}
		c := newSequentialAccountingCollector(1000)
		s.broker.sequentialAccounting = c
		for i := range bars {
			if err := s.step(i); err != nil {
				t.Fatal(err)
			}
			ids := extra[i]
			if i == 14 {
				ids = append([]string{"A"}, ids...)
			}
			for _, id := range ids {
				if err := s.acceptEvents(i, decision(i, id)); err != nil {
					t.Fatal(err)
				}
			}
			c.mark(&s.broker, i, false)
		}
		got, err := c.finish(len(bars), len(s.broker.trades))
		if err != nil {
			t.Fatal(err)
		}
		return got, s.audit
	}
	baseline, baseAudit := run(t, nil)
	if len(baseline.Trades) != 1 || baseline.Trades[0].EntryIndex != 15 || baseline.Trades[0].ExitIndex != 19 || len(baseline.Marks) != 40 || len(baseline.Events) != 2 {
		t.Fatalf("baseline is not one 15->19 trade over 40 marks: %+v", baseline)
	}
	injected, audit := run(t, map[int][]string{14: {"C"}, 15: {"B"}, 17: {"D"}})
	if len(audit.Opportunities) != 4 {
		t.Fatalf("expected 4 opportunities, got %+v", audit.Opportunities)
	}
	for i, o := range audit.Opportunities {
		switch o.ID {
		case "A:perf":
			if o.Status != SequentialFullFilled || o.FillIndex == nil || *o.FillIndex != 15 {
				t.Fatalf("decision A not filled at bar 15: %+v", o)
			}
		default:
			if o.Status != SequentialFullRejected || o.Reason != SequentialFullOccupied || o.FillIndex != nil || o.Fill != nil {
				t.Fatalf("opportunity %d (%s) must be blocked with no fill: %+v", i, o.ID, o)
			}
		}
	}
	if !reflect.DeepEqual(baseAudit.Opportunities[0], audit.Opportunities[0]) {
		t.Fatalf("blocked decisions changed the filled opportunity")
	}
	if !reflect.DeepEqual(baseline, injected) {
		t.Fatalf("blocked decisions changed the accounting:\nbaseline=%+v\ninjected=%+v", baseline, injected)
	}
	// Direct arithmetic: flat bars, no slippage, so the only cost is two fees.
	size := *audit.Opportunities[0].Size
	entryFee := .015625 * size
	if injected.Trades[0].EntryFee != entryFee || injected.Trades[0].Points != 0 || injected.Trades[0].NetPnL != float64(-entryFee-entryFee) {
		t.Fatalf("trade fees: %+v (size %g)", injected.Trades[0], size)
	}
	for i := 15; i < 19; i++ {
		if injected.Marks[i].Realized != -entryFee || injected.Marks[i].Unrealized != 0 {
			t.Fatalf("mark %d while held: %+v", i, injected.Marks[i])
		}
	}
	if injected.Marks[39].Realized != injected.FinalRealized || injected.Marks[39].Unrealized != 0 {
		t.Fatalf("terminal mark: %+v", injected.Marks[39])
	}
}
