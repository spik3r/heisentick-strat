package engine

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/seqcore"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func sequentialFullTestSpec(policy string) dsl.SequentialFullSpec {
	return dsl.SequentialFullSpec{Profile: seqcore.ProfileID, Policy: policy, Symbol: "SYN", Timeframe: "5m", RiskUSD: 100, MaxNotionalUSD: 100000}
}

// Producer regression inputs, not an independent acceptance corpus. The first
// comparison flips at bar 5; Setup 9 completes at 13, Countdown 13 at 25.
func sequentialFullSlideBars(n int) []marketdata.Bar {
	bars := make([]marketdata.Bar, n)
	for i := range bars {
		c := float64(104 - i)
		if i < 4 {
			c = 100
		}
		if i == 4 {
			c = 101
		}
		bars[i] = marketdata.Bar{T: float64(i) * 300000, O: c, H: c + 1, L: c - 1, C: c, V: 1}
	}
	return bars
}

func sequentialFullE1Bars() []marketdata.Bar {
	bars := sequentialFullSlideBars(19)
	sequentialFullFlatFrom(bars, 14)
	return bars
}

func sequentialFullE2Bars() []marketdata.Bar {
	bars := sequentialFullSlideBars(39)
	sequentialFullFlatFrom(bars, 26)
	return bars
}

func sequentialFullFlatFrom(bars []marketdata.Bar, start int) {
	for i := start; i < len(bars); i++ {
		bars[i].O, bars[i].C, bars[i].H, bars[i].L = 100, 100, 100.5, 99.5
	}
}

func sequentialFullMirror(bars []marketdata.Bar) []marketdata.Bar {
	out := append([]marketdata.Bar{}, bars...)
	for i, b := range bars {
		out[i].O, out[i].C, out[i].H, out[i].L = 200-b.O, 200-b.C, 200-b.L, 200-b.H
	}
	return out
}

func sequentialFullNear(t *testing.T, label string, got, want float64) {
	t.Helper()
	// Four result ULPs cover the small independent literal/rational and mirror
	// calculations in these regression expectations. This is a local binary64
	// comparison, with no relative-to-one floor near zero. Broker PnL uses the
	// separately documented operand error budget below instead.
	sequentialFullWithinULP(t, label, got, want, 4)
}

func sequentialFullWithinULP(t *testing.T, label string, got, want float64, limit uint64) {
	t.Helper()
	if got == want {
		return
	}
	rank := func(value float64) uint64 {
		bits := math.Float64bits(value)
		if bits>>63 != 0 {
			return ^bits
		}
		return bits | (1 << 63)
	}
	a, b := rank(got), rank(want)
	if a < b {
		a, b = b, a
	}
	if !isFinite(got) || !isFinite(want) || a-b > limit {
		t.Fatalf("%s = %.17g, want %.17g (distance %d ULP; limit %d)", label, got, want, a-b, limit)
	}
}

func sequentialFullOperandULP(value float64) float64 {
	value = math.Abs(value)
	return math.Nextafter(value, math.Inf(1)) - value
}

func sequentialFullBrokerPnL(t *testing.T, label string, got, want, gross, fee float64) {
	t.Helper()
	// The inherited broker may fuse gross multiplication with fee subtraction
	// on ARM64; it is deliberately unchanged. Bound that difference in the
	// rounded gross/fee operands, plus one result rounding, not relative to a
	// near-cancelled result. Two ULPs per term conservatively include both
	// multiplication roundings and subtraction. Zero fees remain well-defined.
	bound := 2 * (sequentialFullOperandULP(gross) + sequentialFullOperandULP(fee) + sequentialFullOperandULP(want))
	if !isFinite(got) || !isFinite(want) || math.Abs(got-want) > bound {
		t.Fatalf("%s = %.17g, want %.17g (absolute operand-ULP bound %.17g)", label, got, want, bound)
	}
}

func TestSequentialFullMirroredNaturalTimeExits(t *testing.T) {
	for _, policy := range []string{"E1", "E2"} {
		for _, mirror := range []bool{false, true} {
			t.Run(policy+map[bool]string{false: "-long", true: "-short"}[mirror], func(t *testing.T) {
				bars, decision, entry, exit, stop := sequentialFullE1Bars(), 13, 14, 18, 90-0.1*(29.0/14)
				if policy == "E2" {
					bars, decision, entry, exit, stop = sequentialFullE2Bars(), 25, 26, 38, 77.8
				}
				sign := 1.0
				if mirror {
					bars, stop, sign = sequentialFullMirror(bars), 200-stop, -1
				}
				trades, audit, err := runSequentialFull(sequentialFullTestSpec(policy), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open"}})
				if err != nil || len(trades) != 1 || len(audit.Opportunities) != 1 {
					t.Fatalf("trades=%+v audit=%+v err=%v", trades, audit, err)
				}
				trade, opportunity := trades[0], audit.Opportunities[0]
				if trade.EntryIndex != entry || trade.ExitIndex != exit || trade.Reason != "time" || opportunity.DecisionIndex != decision || opportunity.SetupIndex != 13 || opportunity.SetupFirstIndex != 5 || opportunity.Status != SequentialFullFilled {
					t.Fatalf("trade=%+v opportunity=%+v", trade, opportunity)
				}
				sequentialFullNear(t, "stop", trade.InitialSL, stop)
				sequentialFullNear(t, "target", trade.InitialTP, 100+sign*2*math.Abs(100-stop))
				sequentialFullNear(t, "size", trade.Size, 100/math.Abs(100-stop))
			})
		}
	}
}

func TestSequentialFullEntryBarStopTargetAndCollision(t *testing.T) {
	for _, side := range []string{"long", "short"} {
		for _, kind := range []string{"stop", "target", "both"} {
			t.Run(side+"-"+kind, func(t *testing.T) {
				bars := sequentialFullE1Bars()[:15]
				if kind != "target" {
					bars[14].L = 80
				}
				if kind != "stop" {
					bars[14].H = 125
				}
				if side == "short" {
					bars = sequentialFullMirror(bars)
				}
				trades, _, err := runSequentialFull(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open"}})
				if err != nil || len(trades) != 1 {
					t.Fatalf("trades=%+v err=%v", trades, err)
				}
				want := "sl"
				if kind == "target" {
					want = "tp"
				}
				if trades[0].EntryIndex != 14 || trades[0].ExitIndex != 14 || trades[0].Reason != want {
					t.Fatalf("trade=%+v, want same-entry-bar %s", trades[0], want)
				}
			})
		}
	}
}

func TestSequentialFullTimeExitHasOpenPriority(t *testing.T) {
	for _, policy := range []string{"E1", "E2"} {
		for _, mirror := range []bool{false, true} {
			bars := sequentialFullE1Bars()
			if policy == "E2" {
				bars = sequentialFullE2Bars()
			}
			last := len(bars) - 1
			bars[last].O, bars[last].H, bars[last].L, bars[last].C = 70, 180, 60, 100
			if mirror {
				bars = sequentialFullMirror(bars)
			}
			trades, _, err := runSequentialFull(sequentialFullTestSpec(policy), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open", Slippage: 0.1}})
			if err != nil || len(trades) != 1 || trades[0].Reason != "time" || trades[0].ExitIndex != last {
				t.Fatalf("policy=%s mirrored=%v trades=%+v err=%v", policy, mirror, trades, err)
			}
			want := 69.9
			if mirror {
				want = 130.1
			}
			sequentialFullNear(t, "time open", trades[0].Exit, want)
		}
	}
}

func TestSequentialFullCarriedPriceGaps(t *testing.T) {
	for _, kind := range []string{"stop", "target"} {
		for _, mirror := range []bool{false, true} {
			bars := sequentialFullE1Bars()[:16]
			price := 80.0
			if kind == "target" {
				price = 140
			}
			bars[15].O, bars[15].H, bars[15].L, bars[15].C = price, price+1, price-1, price
			if mirror {
				bars = sequentialFullMirror(bars)
			}
			trades, _, err := runSequentialFull(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open"}})
			if err != nil || len(trades) != 1 {
				t.Fatalf("trades=%+v err=%v", trades, err)
			}
			want := bars[15].O
			if kind == "target" {
				want = trades[0].TP
			}
			sequentialFullNear(t, "gap exit", trades[0].Exit, want)
		}
	}
}

func TestSequentialFullFillSizingCostsAndWrongSide(t *testing.T) {
	for _, mirror := range []bool{false, true} {
		bars := sequentialFullE1Bars()
		if mirror {
			bars = sequentialFullMirror(bars)
		}
		spec := sequentialFullTestSpec("E1")
		spec.MaxNotionalUSD = 500
		sign := 1.0
		if mirror {
			sign = -1
		}
		for _, fee := range []float64{0, 0.2} {
			costs := Costs{FillOn: "nextOpen", Slippage: 0.25, SlippageBps: 10, FeePerUnit: fee}
			s, err := newSequentialFullScheduler(spec, marketdata.SeriesFromBars(bars), RunFixture{Costs: costs})
			if err != nil {
				t.Fatal(err)
			}
			for i := range bars {
				if err := s.step(i); err != nil {
					t.Fatal(err)
				}
			}
			trade := s.broker.trades[0]
			fill := 100 + float64(sign*0.35)
			exit := 100 - float64(sign*0.35)
			size := 500 / math.Abs(fill)
			points := float64(exit-fill) * sign
			gross, perFillFee := float64(points*size), float64(fee*size)
			sequentialFullWithinULP(t, "one entry slippage", trade.Entry, fill, 0)
			sequentialFullWithinULP(t, "one exit slippage", trade.Exit, exit, 0)
			sequentialFullWithinULP(t, "cap size", trade.Size, size, 0)
			sequentialFullNear(t, "fill-relative target", trade.InitialTP, fill+sign*2*math.Abs(fill-trade.SL))
			sequentialFullBrokerPnL(t, "trade exit fee", trade.PnL, gross-perFillFee, gross, perFillFee)
			// Once PnL is independently bounded, this checks the separate entry
			// fee debit without multiplying the same error budget by two.
			sequentialFullBrokerPnL(t, "realized entry fee", s.broker.realized, trade.PnL-perFillFee, trade.PnL, perFillFee)
			if !s.audit.Opportunities[0].CapBinds {
				t.Fatal("cap reduction not audited")
			}
		}
		for _, equal := range []bool{false, true} {
			fresh, err := newSequentialFullScheduler(spec, marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open", FeePerUnit: 2}})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i <= 13; i++ {
				if err := fresh.step(i); err != nil {
					t.Fatal(err)
				}
			}
			stop := fresh.audit.Opportunities[0].Stop
			fresh.series.O[14] = stop
			if !equal {
				fresh.series.O[14] -= sign
			}
			if err := fresh.fill(0, 14); err != nil {
				t.Fatal(err)
			}
			if fresh.broker.hasPosition || fresh.broker.realized != 0 || len(fresh.broker.trades) != 0 || fresh.audit.Opportunities[0].Reason != SequentialFullWrongSide {
				t.Fatalf("wrong-side/equality entered or charged: %+v", fresh.audit)
			}
		}
	}
}

func TestSequentialFullTerminalAndPrefixBehavior(t *testing.T) {
	spec, fixture := sequentialFullTestSpec("E1"), RunFixture{Costs: Costs{FillOn: "open"}}
	bars := sequentialFullE1Bars()
	_, audit, err := runSequentialFull(spec, marketdata.SeriesFromBars(bars[:14]), fixture)
	if err != nil || len(audit.Opportunities) != 1 || audit.Opportunities[0].Reason != SequentialFullNoNextBar {
		t.Fatalf("terminal decision audit=%+v err=%v", audit, err)
	}
	trades, audit, err := runSequentialFull(spec, marketdata.SeriesFromBars(bars[:15]), fixture)
	var typed *SequentialFullExecutionError
	if !errors.As(err, &typed) || typed.Kind != "unsupported-incomplete-terminal-run" || len(trades) != 0 || audit.Opportunities[0].Status != SequentialFullFilled {
		t.Fatalf("held terminal trades=%+v audit=%+v err=%v", trades, audit, err)
	}
	_, audit, err = runSequentialFullMode(spec, marketdata.SeriesFromBars(bars[:14]), fixture, false)
	if err != nil || audit.Opportunities[0].Status != SequentialFullPending {
		t.Fatalf("prefix pending audit=%+v err=%v", audit, err)
	}
	for n := 1; n <= len(bars); n++ {
		_, prefix, err := runSequentialFullMode(spec, marketdata.SeriesFromBars(bars[:n]), fixture, false)
		if err != nil {
			t.Fatal(err)
		}
		mutated := append([]marketdata.Bar{}, bars...)
		for i := n; i < len(mutated); i++ {
			mutated[i].O, mutated[i].H, mutated[i].L, mutated[i].C = 101, 200, 1, 102
		}
		s, err := newSequentialFullScheduler(spec, marketdata.SeriesFromBars(mutated), fixture)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < n; i++ {
			if err := s.step(i); err != nil {
				t.Fatal(err)
			}
		}
		if !reflect.DeepEqual(prefix, s.audit) {
			t.Fatalf("future suffix changed prefix at %d: %+v versus %+v", n, prefix, s.audit)
		}
	}
}

// Existing immutable core fixture inputs exercise adapter lifecycle seams. No
// expected execution result is copied from or written to the core corpus.
func sequentialFullCoreBars(t *testing.T, file, id string) []marketdata.Bar {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("seqcore", "testdata", "seq-core-fixtures.v2", file+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			ID     string        `json:"id"`
			Bars   []seqcore.Bar `json:"bars"`
			Series struct {
				StartOpenMS int64   `json:"start_open_ms"`
				TimeframeMS int64   `json:"timeframe_ms"`
				OpenMS      []int64 `json:"open_ms"`
			} `json:"series"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		if c.ID != id {
			continue
		}
		bars := make([]marketdata.Bar, len(c.Bars))
		for i, b := range c.Bars {
			open := c.Series.StartOpenMS + int64(i)*c.Series.TimeframeMS
			if len(c.Series.OpenMS) != 0 {
				open = c.Series.OpenMS[i]
			}
			bars[i] = marketdata.Bar{T: float64(open), O: b.O, H: b.H, L: b.L, C: b.C, V: 1}
		}
		return bars
	}
	t.Fatal("missing core input " + id)
	return nil
}

func TestSequentialFullDelayedWindowAndFrozenSetupRange(t *testing.T) {
	for _, side := range []string{"buy", "sell"} {
		for _, age := range []string{"4", "5"} {
			bars := sequentialFullCoreBars(t, "window_policy", "window_policy.setup_age"+age+"."+side)
			_, audit, err := runSequentialFullMode(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open"}}, false)
			if err != nil || len(audit.Opportunities) != 1 {
				t.Fatalf("audit=%+v err=%v", audit, err)
			}
			o := audit.Opportunities[0]
			if age == "5" {
				if o.Reason != SequentialFullExpired {
					t.Fatalf("age five was not expired: %+v", o)
				}
				continue
			}
			if o.Status != SequentialFullPending || o.DecisionIndex != 17 || o.NextOpenIndex != 18 {
				t.Fatalf("age four last-eligible decision: %+v", o)
			}
			// The decision's ATR is current, but the structural extreme remains
			// the completed Setup 1..9 range rather than the perfection bar.
			extreme := math.Inf(1)
			if side == "sell" {
				extreme = math.Inf(-1)
			}
			for _, b := range bars[5:14] {
				if side == "buy" {
					extreme = math.Min(extreme, b.L)
				} else {
					extreme = math.Max(extreme, b.H)
				}
			}
			want := extreme - 0.1*o.SignalATR
			if side == "sell" {
				want = extreme + 0.1*o.SignalATR
			}
			sequentialFullNear(t, "frozen setup range", o.Stop, want)
			fill := bars[len(bars)-1]
			fill.T += 300000
			fill.O, fill.C = o.Stop+2, o.Stop+2
			if side == "sell" {
				fill.O, fill.C = o.Stop-2, o.Stop-2
			}
			fill.H, fill.L = fill.C+0.1, fill.C-0.1
			bars = append(bars, fill)
			_, after, err := runSequentialFullMode(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open"}}, false)
			if err != nil || after.Opportunities[0].Status != SequentialFullFilled || *after.Opportunities[0].FillIndex != 18 || after.Opportunities[0].Stop != o.Stop || after.Opportunities[0].SignalATR != o.SignalATR {
				t.Fatalf("fill outside decision window changed decision: %+v err=%v", after, err)
			}
		}
	}
}

func TestSequentialFullCountdownOriginalEpisodeAndRecycle(t *testing.T) {
	for _, c := range []struct {
		file, id string
		decision int
	}{
		{"countdown_overlap", "countdown_overlap.active_preserved.buy", 27},
		{"countdown_overlap", "countdown_overlap.deferred_preserved.buy", 34},
		{"terminal_13_setup_22_collision", "terminal_13_setup_22_collision.main.buy", 26},
	} {
		bars := sequentialFullCoreBars(t, c.file, c.id)
		// A distinct original Setup-1 low makes using only Countdown bars or
		// the replacement Setup observably wrong without changing closes.
		bars[5].L = 1
		_, audit, err := runSequentialFullMode(sequentialFullTestSpec("E2"), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open"}}, false)
		if err != nil || len(audit.Opportunities) != 1 {
			t.Fatalf("%s: audit=%+v err=%v", c.id, audit, err)
		}
		o := audit.Opportunities[0]
		if o.SetupIndex != 13 || o.SetupFirstIndex != 5 || o.DecisionIndex != c.decision {
			t.Fatalf("%s: admitting episode replaced or event lost: %+v", c.id, o)
		}
		extreme := math.Inf(1)
		for _, b := range bars[5 : c.decision+1] {
			extreme = math.Min(extreme, b.L)
		}
		sequentialFullNear(t, "admitting Setup through CD13", o.Stop, extreme-0.1*o.SignalATR)
	}
}

func TestSequentialFullExecutableCollisionOccupancyAndDedup(t *testing.T) {
	for _, occupied := range []bool{false, true} {
		s, err := newSequentialFullScheduler(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(sequentialFullE1Bars()), RunFixture{Costs: Costs{FillOn: "open"}})
		if err != nil {
			t.Fatal(err)
		}
		s.broker.hasPosition = occupied
		events := []seqcore.Event{}
		for _, side := range []string{"sell", "buy"} {
			for _, kind := range []string{"setup_complete", "perfection"} {
				events = append(events, seqcore.Event{Type: kind, Side: side, EpisodeID: side, BarIndex: 13, BarOpenMS: 3900000, DecisionMS: 4200000})
			}
		}
		if err := s.acceptEvents(13, events); err != nil {
			t.Fatal(err)
		}
		if err := s.acceptEvents(13, events); err != nil {
			t.Fatal(err)
		}
		want := SequentialFullSimultaneous
		if occupied {
			want = SequentialFullOccupied
		}
		if len(s.audit.Opportunities) != 2 || s.pending != -1 {
			t.Fatalf("duplicate or queued collision: %+v", s.audit)
		}
		for _, o := range s.audit.Opportunities {
			if o.Reason != want {
				t.Fatalf("reason=%s want=%s", o.Reason, want)
			}
		}
	}
}

func TestSequentialFullFiniteDerivedRefusals(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*sequentialFullScheduler)
	}{
		{"fill", func(s *sequentialFullScheduler) {
			s.broker.costs.Slippage = math.MaxFloat64
			s.series.O[14] = math.MaxFloat64
		}},
		{"target", func(s *sequentialFullScheduler) { s.audit.Opportunities[0].Stop = -math.MaxFloat64 }},
		{"size", func(s *sequentialFullScheduler) {
			s.spec.RiskUSD, s.spec.MaxNotionalUSD = math.MaxFloat64, math.MaxFloat64
			s.series.O[14], s.audit.Opportunities[0].Stop = 0.001, 0.0009
		}},
		{"fee", func(s *sequentialFullScheduler) { s.broker.costs.FeePerUnit = math.MaxFloat64 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, err := newSequentialFullScheduler(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(sequentialFullE1Bars()), RunFixture{Costs: Costs{FillOn: "open"}})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i <= 13; i++ {
				if err := s.step(i); err != nil {
					t.Fatal(err)
				}
			}
			test.mutate(s)
			err = s.fill(0, 14)
			var typed *SequentialFullExecutionError
			if !errors.As(err, &typed) || typed.Kind != "invalid-sequential-derived-value" || s.broker.hasPosition || s.broker.realized != 0 {
				t.Fatalf("derived error=%v position=%v realized=%v", err, s.broker.hasPosition, s.broker.realized)
			}
		})
	}
}

func TestSequentialFullCoreContinuesWhileOccupied(t *testing.T) {
	bars := sequentialFullCoreBars(t, "countdown_overlap", "countdown_overlap.active_preserved.buy")
	s, err := newSequentialFullScheduler(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(bars), RunFixture{Costs: Costs{FillOn: "open"}})
	if err != nil {
		t.Fatal(err)
	}
	// Keep an explicitly seeded broker position throughout both Setup episodes;
	// this isolates the core/occupied-book seam from natural bracket lifetimes.
	s.broker.openPosition(sideLong, 100, order{SL: -10000, TP: 10000, Size: 1, HasSize: true, NoSlip: true}, 0)
	s.hold = 100
	for i := range bars {
		if err := s.step(i); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.audit.Opportunities) != 2 || s.audit.Opportunities[0].DecisionIndex != 13 || s.audit.Opportunities[1].DecisionIndex != 24 {
		t.Fatalf("occupied book stopped the core: %+v", s.audit)
	}
	for _, o := range s.audit.Opportunities {
		if o.Reason != SequentialFullOccupied {
			t.Fatalf("occupied opportunity=%+v", o)
		}
	}
}

func TestSequentialFullTimeExitThenSameBarDecision(t *testing.T) {
	s, err := newSequentialFullScheduler(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(sequentialFullE1Bars()), RunFixture{Costs: Costs{FillOn: "open"}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 13; i++ {
		if err := s.step(i); err != nil {
			t.Fatal(err)
		}
	}
	s.broker.openPosition(sideLong, 100, order{SL: -10000, TP: 10000, Size: 1, HasSize: true, NoSlip: true}, 9)
	for i := 13; i < s.series.Len(); i++ {
		if err := s.step(i); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.broker.trades) != 2 || s.broker.trades[0].ExitIndex != 13 || s.broker.trades[1].EntryIndex != 14 || s.broker.trades[1].ExitIndex != 18 || s.audit.Opportunities[0].Status != SequentialFullFilled {
		t.Fatalf("exit/decision/fill ordering trades=%+v audit=%+v", s.broker.trades, s.audit)
	}
}

func TestSequentialFullATRFullWarmupGuard(t *testing.T) {
	s, err := newSequentialFullScheduler(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(sequentialFullE1Bars()), RunFixture{Costs: Costs{FillOn: "open"}})
	if err != nil {
		t.Fatal(err)
	}
	// Core cannot complete a canonical flip-gated Setup this early. Injecting a
	// policy-boundary event verifies it cannot consume the legacy partial ATR.
	events := []seqcore.Event{{Type: "setup_complete", Side: "buy", EpisodeID: "early"}, {Type: "perfection", Side: "buy", EpisodeID: "early"}}
	if err := s.acceptEvents(12, events); err != nil {
		t.Fatal(err)
	}
	if len(s.audit.Opportunities) != 1 || s.audit.Opportunities[0].Reason != SequentialFullWarmup || s.pending != -1 {
		t.Fatalf("partial ATR was admitted: %+v", s.audit)
	}
}

func TestSequentialFullE1UsesWholeCompletedSetup(t *testing.T) {
	bars := sequentialFullE1Bars()
	bars[5].L = 50
	for _, mirror := range []bool{false, true} {
		input := bars
		if mirror {
			input = sequentialFullMirror(bars)
		}
		trades, audit, err := runSequentialFull(sequentialFullTestSpec("E1"), marketdata.SeriesFromBars(input), RunFixture{Costs: Costs{FillOn: "open"}})
		if err != nil || len(trades) != 1 {
			t.Fatalf("trades=%+v audit=%+v err=%v", trades, audit, err)
		}
		// TR is 51 at bar 5, 2 on the other thirteen seed bars.
		want := 50 - 0.1*(77.0/14)
		if mirror {
			want = 200 - want
		}
		sequentialFullNear(t, "Setup-1 extreme", trades[0].InitialSL, want)
	}
}

func TestSequentialFullPublicPositiveQuotientBoundsAndZeroFill(t *testing.T) {
	for _, test := range []struct {
		name     string
		shift    float64
		scale    float64
		cap      float64
		wantBind bool
		riskInf  bool
		capInf   bool
		wantZero bool
	}{
		{name: "finite cap rescues risk quotient overflow", scale: 1e-309, cap: 1, wantBind: true, riskInf: true},
		{name: "nonbinding cap quotient overflow", scale: 0.001, cap: math.MaxFloat64, capInf: true},
		{name: "translated zero fill has zero notional", shift: -100, scale: 1, cap: 100000, capInf: true, wantZero: true},
		{name: "translated negative fill uses absolute notional", shift: -200, scale: 1, cap: 500, wantBind: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bars := sequentialFullE1Bars()
			for i := range bars {
				b := &bars[i]
				b.O, b.H = (b.O+test.shift)*test.scale, (b.H+test.shift)*test.scale
				b.L, b.C = (b.L+test.shift)*test.scale, (b.C+test.shift)*test.scale
			}
			config := dsl.Config{
				"dslVersion": int64(7), "name": "Producer numeric regression", "description": "", "setupType": "sequentialFull",
				"sequentialFull": map[string]any{"contractVersion": "sequential-full-config-v1", "profile": seqcore.ProfileID, "policy": "E1", "symbol": "SYN", "timeframe": "5m", "riskUsd": 100.0, "maxNotionalUsd": test.cap},
			}
			request := RunRequest{Config: config, Symbol: "SYN", Timeframe: "5m", Series: marketdata.SeriesFromBars(bars), Costs: Costs{FillOn: "open"}}
			prepared, err := PrepareRun(request)
			if err != nil {
				t.Fatalf("finite operands failed public admission: %v", err)
			}
			result, err := prepared.RunChecked(request.Costs)
			if err != nil || len(result.Trades) != 1 || result.SequentialFull == nil || len(result.SequentialFull.Opportunities) != 1 {
				t.Fatalf("finite capped execution: result=%+v err=%v", result, err)
			}
			o := result.SequentialFull.Opportunities[0]
			if o.Status != SequentialFullFilled || o.Fill == nil || o.Size == nil || o.RiskDistance == nil || o.CapBinds != test.wantBind {
				t.Fatalf("fill/cap audit=%+v", o)
			}
			riskBound := 100 / *o.RiskDistance
			capBound := math.Inf(1)
			if *o.Fill != 0 {
				capBound = test.cap / math.Abs(*o.Fill)
			}
			if math.IsInf(riskBound, 1) != test.riskInf || math.IsInf(capBound, 1) != test.capInf || (*o.Fill == 0) != test.wantZero {
				t.Fatalf("probe did not reach required numerical boundary: risk=%v cap=%v fill=%v", riskBound, capBound, *o.Fill)
			}
			sequentialFullWithinULP(t, "finite minimum of positive bounds", *o.Size, math.Min(riskBound, capBound), 0)
			if !isFinite(*o.Size) || *o.Size <= 0 || result.Trades[0].Reason != "time" || result.Trades[0].PnL != 0 {
				t.Fatalf("unexpected finite execution result: %+v", result.Trades[0])
			}
			if _, err := json.Marshal(result); err != nil {
				t.Fatalf("intermediate infinity escaped audit: %v", err)
			}
			direct, err := Run(request)
			if err != nil || !reflect.DeepEqual(result, direct) {
				t.Fatalf("direct/prepared public parity: %v", err)
			}
		})
	}
}

func TestSequentialFullPublicFiniteTargetWithOverflowingDoubleRisk(t *testing.T) {
	for _, mirror := range []bool{false, true} {
		bars := sequentialFullE1Bars()
		for i, b := range bars {
			if i <= 13 {
				bars[i].O = 6e307 + float64((200-b.O)*1e306)
				bars[i].H = 6e307 + float64((200-b.L)*1e306)
				bars[i].L = 6e307 + float64((200-b.H)*1e306)
				bars[i].C = 6e307 + float64((200-b.C)*1e306)
			} else {
				bars[i].O, bars[i].H, bars[i].L, bars[i].C = 7e307, 7.1e307, 6.9e307, 7e307
			}
		}
		if mirror {
			for i, b := range bars {
				bars[i].O, bars[i].H, bars[i].L, bars[i].C = -b.O, -b.L, -b.H, -b.C
			}
		}
		config := dsl.Config{
			"dslVersion": int64(7), "name": "Producer target regression", "description": "", "setupType": "sequentialFull",
			"sequentialFull": map[string]any{"contractVersion": "sequential-full-config-v1", "profile": seqcore.ProfileID, "policy": "E1", "symbol": "SYN", "timeframe": "5m", "riskUsd": 100.0, "maxNotionalUsd": 100000.0},
		}
		request := RunRequest{Config: config, Symbol: "SYN", Timeframe: "5m", Series: marketdata.SeriesFromBars(bars), Costs: Costs{FillOn: "open"}}
		result, err := Run(request)
		if err != nil || len(result.Trades) != 1 || result.SequentialFull == nil || len(result.SequentialFull.Opportunities) != 1 {
			t.Fatalf("mirrored=%v finite target result=%+v err=%v", mirror, result, err)
		}
		o := result.SequentialFull.Opportunities[0]
		if o.Status != SequentialFullFilled || o.RiskDistance == nil || !math.IsInf(2**o.RiskDistance, 1) || o.Target == nil || !isFinite(*o.Target) {
			t.Fatalf("did not exercise a finite final target after doubled-risk overflow: %+v", o)
		}
		directionalRisk := -*o.RiskDistance
		if mirror {
			directionalRisk = *o.RiskDistance
		}
		want := float64(*o.Fill+directionalRisk) + directionalRisk
		sequentialFullWithinULP(t, "overflow-only distributed 2R", *o.Target, want, 0)
		if result.Trades[0].Reason != "time" || result.Trades[0].PnL != 0 {
			t.Fatalf("unexpected huge finite trade: %+v", result.Trades[0])
		}
		prepared, err := PrepareRun(request)
		if err != nil {
			t.Fatal(err)
		}
		other, err := prepared.RunChecked(request.Costs)
		if err != nil || !reflect.DeepEqual(result, other) {
			t.Fatalf("prepared finite target parity: %v", err)
		}
	}
}
