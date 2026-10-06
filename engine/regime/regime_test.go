package regime

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func closeTo(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-10*math.Max(1, math.Abs(want)) {
		t.Fatalf("got %.16g want %.16g", got, want)
	}
}
func profile(t *testing.T, mode string) dsl.Config {
	t.Helper()
	source := `dsl v7
strategy "Invented synthetic case" { description "No market data" }
market { regime timeframe M30 from M5 }
setup { type: regime engine
 regime profile v9-floor-half-v1
 regime mode ` + mode + `
}`
	p, e := dsl.Parse(source)
	if e != nil || len(p.Errors) > 0 {
		t.Fatalf("parse:%v %v", e, p.Errors)
	}
	return p.Config
}
func costs(spread float64) Costs {
	return Costs{Spread: spread, FeePerUnitSide: .5, InitialEquity: 10000}
}
func row(i int, o, h, l, c, atr, st float64, regime, candidate int) IndicatorRow {
	b := int64(i) * M30MS
	return IndicatorRow{NativeBar: NativeBar{BucketT: b, FirstObservedT: b, CloseT: b + M30MS, Open: o, High: h, Low: l, Close: c, Volume: 200, Count: 6, Complete: true}, ATR14: pointer(atr), Supertrend: pointer(st), VolumeMean: pointer(100), Regime: regime, Candidate: candidate}
}
func lifecycleRequest(n int) Request {
	return Request{WarmupFromT: 0, TradeFromT: M30MS, TradeToT: int64(n) * M30MS, Costs: costs(0)}
}
func m5(n int) marketdata.Series {
	s := marketdata.NewSeries(n * 6)
	for i := range s.T {
		s.T[i] = float64(int64(i) * M5MS)
		s.O[i] = 100
		s.H[i] = 101
		s.L[i] = 99
		s.C[i] = 100
		s.V[i] = 10
	}
	return s
}
func removeM5(s marketdata.Series, index int) marketdata.Series {
	bars := s.Bars()
	return marketdata.SeriesFromBars(append(bars[:index], bars[index+1:]...))
}

func TestHandDerivedArithmetic(t *testing.T) {
	closeTo(t, wma([]float64{1, 2, 3}, 3)[2], 7./3)
	for _, test := range []struct {
		period, length int
		want           float64
	}{{9, 11, 11}, {21, 25, 24 + 1./3}, {25, 29, 28 + 1./3}} {
		v := make([]float64, test.length)
		for i := range v {
			v[i] = float64(i + 1)
		}
		h := hma(v, test.period)
		for _, x := range h[:len(h)-1] {
			if !math.IsNaN(x) {
				t.Fatal("HMA warmed early")
			}
		}
		closeTo(t, h[len(h)-1], test.want)
	}
	r := rma([]float64{1, 2, 3, 7}, 3)
	if !math.IsNaN(r[0]) || !math.IsNaN(r[1]) {
		t.Fatal("RMA warmed early")
	}
	closeTo(t, r[2], 2)
	closeTo(t, r[3], 11./3)
}
func TestSupertrendStrictBandCrossAndTR(t *testing.T) {
	b := make([]NativeBar, 14)
	for i := range b {
		b[i] = row(i, 100, 101, 99, 100, 0, 0, 0, 0).NativeBar
	}
	b[10].High = 106
	b[10].Close = 106
	b[11].High = 106.01
	b[11].Close = 106.01
	b[12].High = 106.01
	b[12].Low = 95
	b[12].Close = 95
	b[13].High = 106.01
	b[13].Low = 94.99
	b[13].Close = 94.99
	x, e := calculate(b)
	if e != nil {
		t.Fatal(e)
	}
	if x[8].ATR10 != nil {
		t.Fatal("early ATR")
	}
	closeTo(t, *x[9].ATR10, 2)
	closeTo(t, *x[10].ATR10, 2.5)
	for i, want := range map[int]int{9: 1, 10: 1, 11: -1, 12: -1, 13: 1} {
		if x[i].Regime != want {
			t.Fatalf("regime at%d=%d want%d", i, x[i].Regime, want)
		}
	}
	closeTo(t, *x[10].Supertrend, 106)
	closeTo(t, *x[12].Supertrend, 95)
	// First TR must be high-low, independent of the opening price.
	b[0].Open = 100.5
	x, e = calculate(b)
	if e != nil {
		t.Fatal(e)
	}
	closeTo(t, *x[9].ATR10, 2)
	b[1].Open = 110
	b[1].High = 111
	b[1].Low = 109
	b[1].Close = 110
	x, e = calculate(b)
	if e != nil {
		t.Fatal(e)
	}
	closeTo(t, *x[9].ATR10, 3.8)
}
func TestCrossAndSameBarStrictVolumeGate(t *testing.T) {
	for _, test := range []struct {
		current, previous float64
		want              int
	}{{1, 0, 1}, {-1, 0, -1}, {0, -1, 0}, {1, 1, 0}, {math.NaN(), 0, 0}} {
		if got := crossDirection(test.current, test.previous); got != test.want {
			t.Fatalf("cross %v got%d", test, got)
		}
	}
	if candidateSignal(1, -1, true, true) != 1 || candidateSignal(-1, 1, true, true) != -1 {
		t.Fatal("gate rejected cross")
	}
	for _, test := range []struct {
		cross, regime int
		vol, full     bool
	}{{1, 1, true, true}, {1, -1, false, true}, {1, -1, true, false}, {0, -1, true, true}} {
		if candidateSignal(test.cross, test.regime, test.vol, test.full) != 0 {
			t.Fatal("gate bypass/delayed cross")
		}
	}
	s := m5(40)
	bars, _, e := aggregate(s, 0, 40*M30MS)
	if e != nil {
		t.Fatal(e)
	}
	bars[39].Volume = 120
	x, e := calculate(bars)
	if e != nil {
		t.Fatal(e)
	}
	closeTo(t, *x[39].VolumeMean, 63)
	if !x[39].VolumeOK || x[38].Complete40 || !x[39].Complete40 {
		t.Fatal("volume/40 timing")
	}
	bars[39].Volume = 60
	x, _ = calculate(bars)
	if x[39].VolumeOK {
		t.Fatal("volume equality passed")
	}
	bars[10].Complete = false
	x, _ = calculate(bars)
	if x[39].Complete40 {
		t.Fatal("partial ignored")
	}
	// Observed-bar semantics: no fabricated weekend candles or freshness reset.
	for i := 20; i < len(bars); i++ {
		bars[i].BucketT += 2 * 86400000
		bars[i].FirstObservedT += 2 * 86400000
		bars[i].CloseT += 2 * 86400000
	}
	bars[10].Complete = true
	x, _ = calculate(bars)
	if !x[39].Complete40 {
		t.Fatal("weekend reset changed profile")
	}
}
func TestAggregateCompletenessTimestampAndWatermark(t *testing.T) {
	s := removeM5(m5(3), 6)
	b, n, e := aggregate(s, 0, 3*M30MS)
	if e != nil {
		t.Fatal(e)
	}
	if n != 17 || b[1].Complete || b[1].Count != 5 || b[1].FirstObservedT != M30MS+M5MS {
		t.Fatal("lost missing-open provenance")
	}
	if _, _, e = aggregate(removeM5(m5(3), 17), 0, 3*M30MS); e == nil {
		t.Fatal("unclosed tail accepted")
	}
	if _, _, e = aggregate(removeM5(m5(3), 0), 0, 3*M30MS); e == nil {
		t.Fatal("missing context start accepted")
	}
	for name, mutate := range map[string]func(*marketdata.Series){"absent volume": func(s *marketdata.Series) { s.V = nil }, "duplicate": func(s *marketdata.Series) { s.T[1] = s.T[0] }, "offgrid": func(s *marketdata.Series) { s.T[1]++ }, "nan": func(s *marketdata.Series) { s.V[1] = math.NaN() }, "negative volume": func(s *marketdata.Series) { s.V[1] = -1 }, "incoherent": func(s *marketdata.Series) { s.H[1] = 90 }, "zero": func(s *marketdata.Series) { s.L[1] = 0 }, "overflow": func(s *marketdata.Series) {
		for i := range s.V {
			s.V[i] = math.MaxFloat64
		}
	}} {
		t.Run(name, func(t *testing.T) {
			s := m5(3)
			mutate(&s)
			if _, _, e := aggregate(s, 0, 3*M30MS); e == nil {
				t.Fatal("invalid input admitted")
			}
		})
	}
}
func TestSourceDelayedProtectionAndDynamicSignalAnchor(t *testing.T) {
	r := lifecycleRequest(4)
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 0), row(1, 100, 101, 99, 100, 2, 90, -1, 1), row(2, 103, 108, 95, 104, 2, 90, -1, 0), row(3, 104, 106, 97, 104, 3, 90, -1, 0)}
	out, e := execute(r, SourceMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Trades) != 0 || out.OpenPosition == nil {
		t.Fatal("retroactive source entry-bar exit")
	}
	p := out.OpenPosition
	closeTo(t, p.Entry, 103)
	closeTo(t, *p.FirstLiveSL, 96)
	closeTo(t, *p.FirstLiveTP, 107)
	closeTo(t, *p.SL, 94)
	closeTo(t, *p.TP, 110.5)
	if !p.NakedEntryBar || !p.ShadowStopHit || p.StopWidenings != 1 || p.TargetChanges != 1 || p.FirstLiveT != 3*M30MS {
		t.Fatal("source audit wrong")
	}
	closeTo(t, p.Quantity, 1000./103)
	closeTo(t, out.Summary.CashEquity, 10000-p.EntryFee)
	closeTo(t, out.Summary.ClosedEquity, 10000)
}
func TestAuditFrozenBracketRatchetAndEntryBarProtection(t *testing.T) {
	r := lifecycleRequest(4)
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 0), row(1, 100, 101, 99, 100, 2, 90, -1, 1), row(2, 103, 108, 100, 104, 3, 100, -1, 0), row(3, 104, 106, 101, 104, 10, 95, -1, 0)}
	out, e := execute(r, AuditMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	p := out.OpenPosition
	if p == nil {
		t.Fatal("unexpected exit")
	}
	closeTo(t, *p.FirstLiveSL, 99)
	closeTo(t, *p.FirstLiveTP, 110)
	closeTo(t, *p.SL, 100)
	closeTo(t, *p.TP, 110)
	if p.StopWidenings != 0 || p.NakedEntryBar {
		t.Fatal("baseline policy changed")
	}
	rows[2].Low = 98
	out, e = execute(r, AuditMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Trades) != 1 || out.Trades[0].Reason != "stop" || out.Trades[0].Exit != 99 {
		t.Fatal("entry bracket inactive")
	}
}
func TestShortSourceAndAuditMirrors(t *testing.T) {
	for _, mode := range []string{SourceMode, AuditMode} {
		t.Run(mode, func(t *testing.T) {
			r := lifecycleRequest(3)
			rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 110, 1, 0), row(1, 100, 101, 99, 100, 2, 110, 1, -1), row(2, 97, 105, 92, 96, 2, 110, 1, 0)}
			out, e := execute(r, mode, rows)
			if e != nil {
				t.Fatal(e)
			}
			if mode == SourceMode {
				p := out.OpenPosition
				if p == nil {
					t.Fatal("short source retroactive fill")
				}
				closeTo(t, *p.SL, 104)
				closeTo(t, *p.TP, 93)
			} else {
				if len(out.Trades) != 1 || out.Trades[0].Exit != 101 || out.Trades[0].Reason != "stop" {
					t.Fatalf("audit short:%+v", out.Trades)
				}
			}
		})
	}
}
func TestBarrierGapPathAndTieRules(t *testing.T) {
	for _, x := range []struct {
		name            string
		d               int
		o, h, l, sl, tp float64
		price           float64
		reason          string
		gap             bool
	}{
		{"long stop gap", 1, 90, 101, 89, 95, 105, 90, "stop_gap", true}, {"short stop gap", -1, 110, 111, 99, 105, 95, 110, "stop_gap", true},
		{"long target improvement", 1, 110, 111, 109, 95, 105, 110, "target_gap", true}, {"short target improvement", -1, 90, 91, 89, 105, 95, 90, "target_gap", true},
		{"long highfirst", 1, 100, 106, 90, 95, 105, 105, "target_ambiguous", false}, {"long lowfirst", 1, 100, 110, 94, 95, 105, 95, "stop_ambiguous", false},
		{"long tie", 1, 100, 106, 94, 95, 105, 95, "stop_ambiguous", false}, {"short tie", -1, 100, 106, 94, 105, 95, 95, "target_ambiguous", false},
		{"short highfirst", -1, 100, 106, 90, 105, 95, 105, "stop_ambiguous", false},
	} {
		t.Run(x.name, func(t *testing.T) {
			b := row(1, x.o, x.h, x.l, x.o, 0, 0, 0, 0).NativeBar
			b.FirstObservedT += M5MS
			p, r, when := barrier(b, x.sl, x.tp, x.d, 0)
			closeTo(t, p, x.price)
			if r != x.reason {
				t.Fatal(r)
			}
			want := b.CloseT
			if x.gap {
				want = b.FirstObservedT
			}
			if when != want {
				t.Fatal("exit time")
			}
		})
	}
}
func TestForcedStopLatchAndFreshFlip(t *testing.T) {
	for _, mode := range []string{SourceMode, AuditMode} {
		r := lifecycleRequest(4)
		rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 0), row(1, 100, 101, 99, 100, 2, 90, -1, 1), row(2, 100, 101, 99, 100, 2, 102, 1, 0), row(3, 105, 111, 103, 106, 2, 102, 1, 0)}
		out, e := execute(r, mode, rows)
		if e != nil {
			t.Fatal(e)
		}
		reason := "wrong_side_stop"
		if mode == AuditMode {
			reason = "explicit_regime_flip"
		}
		if len(out.Trades) != 1 || out.Trades[0].Reason != reason || out.Trades[0].Exit != 105 || out.Trades[0].ExitT != 3*M30MS {
			t.Fatalf("%s forced exit:%+v", mode, out.Trades)
		}
	}
	p, _, _ := openPosition(Signal{Direction: 1, Anchor: 100, ATR: 2, Supertrend: 90}, row(1, 100, 101, 99, 100, 2, 90, 1, 0).NativeBar, AuditMode, costs(0), 10000)
	_, e := manage(p, row(1, 100, 101, 99, 100, 2, 102, 1, 0), 1, AuditMode, 0)
	if e != nil {
		t.Fatal(e)
	}
	if p.ForcedReason != "" {
		t.Fatal("pre-existing contrary regime treated as fresh flip")
	}
}
func TestSourceMarketableTargetIsExplicitlyNotLatched(t *testing.T) {
	p, _, _ := openPosition(Signal{Direction: 1, Anchor: 100, ATR: 2, Supertrend: 90}, row(1, 100, 110, 99, 109, 1, 90, -1, 0).NativeBar, SourceMode, costs(0), 10000)
	edit, e := manage(p, row(1, 100, 110, 99, 109, 1, 90, -1, 0), -1, SourceMode, 0)
	if e != nil {
		t.Fatal(e)
	}
	if !edit.TargetMarketable || p.ForcedReason != "" || p.MarketableTargetEdits != 1 {
		t.Fatal("changed offline target activation")
	}
	_, reason, _ := barrier(row(2, 101, 103, 99, 102, 1, 90, -1, 0).NativeBar, *p.SL, *p.TP, 1, 0)
	if reason != "" {
		t.Fatal("marketable target incorrectly latched across gap-back")
	}
}
func TestSpreadFeesSizingAndPartialTimestamp(t *testing.T) {
	for _, d := range []int{1, -1} {
		signal := Signal{Direction: d, Anchor: 100, ATR: 2, Supertrend: 90}
		b := row(1, 100, 101, 99, 100, 2, 90, -1, 0).NativeBar
		b.FirstObservedT += M5MS
		b.Count = 5
		b.Complete = false
		p, cash, e := openPosition(signal, b, SourceMode, costs(1), 10000)
		if e != nil {
			t.Fatal(e)
		}
		if p.EntryT != M30MS+M5MS {
			t.Fatal("backdated partial fill")
		}
		exit := 100.
		if d == -1 {
			exit++
		}
		net := float64(d)*(exit-p.Entry) - 1
		closeTo(t, net, -2)
		closeTo(t, cash, 10000-p.Quantity*.5)
	}
	p, cash, _ := openPosition(Signal{Direction: 1, Anchor: 100, ATR: 2, Supertrend: 90}, row(1, 100, 101, 99, 100, 2, 90, -1, 0).NativeBar, SourceMode, costs(0), 10000)
	closeTo(t, p.Quantity, 10)
	closeTo(t, cash, 9995)
	if _, _, e := openPosition(p.Signal, row(1, 100, 101, 99, 100, 2, 90, -1, 0).NativeBar, SourceMode, costs(0), 0); e == nil {
		t.Fatal("nonpositive sizing equity")
	}
	r := lifecycleRequest(3)
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 0), row(1, 100, 101, 99, 100, 2, 90, -1, 1), row(2, 100, 101, 99, 100, 2, 90, -1, 0)}
	rows[2].Complete = false
	rows[2].FirstObservedT += M5MS
	out, e := execute(r, SourceMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if out.OpenPosition == nil || out.Summary.PartialEntryFills != 1 || out.OpenPosition.EntryT != 2*M30MS+M5MS {
		t.Fatal("future partial entry censor returned")
	}
	closeTo(t, out.Summary.OpenEntryFee, 5)
	closeTo(t, out.Summary.MarkedEquity, 9995)
	closeTo(t, out.Summary.MarkedEquityAfterClosingFee, 9990)
}
func TestStartEndBoundariesAndFutureIsolation(t *testing.T) {
	r := lifecycleRequest(3)
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 1), row(1, 100, 101, 99, 100, 2, 90, -1, 0), row(2, 100, 101, 99, 100, 2, 90, -1, 1)}
	out, e := execute(r, SourceMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Signals) != 0 || out.OpenPosition != nil || out.PendingSignal != nil {
		t.Fatal("warmup/final-close signal leaked")
	}
	rows[1].Candidate = 1
	out, e = execute(r, SourceMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if out.OpenPosition == nil || out.OpenPosition.FirstLiveT != r.TradeToT || len(out.Edits) != 1 {
		t.Fatal("final close management lost")
	}
	// Full public API: all computation at this cutoff is independent of later prices.
	req := Request{Config: profile(t, SourceMode), M5: m5(50), WarmupFromT: 0, TradeFromT: 40 * M30MS, TradeToT: 50 * M30MS, Costs: costs(0)}
	before, e := Run(req)
	if e != nil {
		t.Fatal(e)
	}
	req.M5 = m5(55)
	for i := 50 * 6; i < 55*6; i++ {
		req.M5.O[i] = 1000
		req.M5.H[i] = 1001
		req.M5.L[i] = 999
		req.M5.C[i] = 1000
		req.M5.V[i] = 5000
	}
	after, e := Run(req)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("future observations changed fixed-cutoff output")
	}
	raw, e := json.Marshal(after)
	if e != nil || strings.Contains(string(raw), "NaN") {
		t.Fatal("non-JSON-safe indicators/PF")
	}
}
func TestFrozenRequestRefusals(t *testing.T) {
	r := Request{Config: profile(t, SourceMode), M5: m5(50), WarmupFromT: 0, TradeFromT: 40 * M30MS, TradeToT: 50 * M30MS, Costs: costs(0)}
	for name, change := range map[string]func(*Request){"bad mode": func(r *Request) { r.Config["regimeEngine"].(map[string]any)["mode"] = "improved" }, "cost": func(r *Request) { r.Costs.FeePerUnitSide = 0 }, "nan spread": func(r *Request) { r.Costs.Spread = math.NaN() }, "fraction spread": func(r *Request) { r.Costs.Spread = .2 }, "equity": func(r *Request) { r.Costs.InitialEquity = 1000 }, "misaligned": func(r *Request) { r.TradeToT++ }, "order": func(r *Request) { r.TradeFromT = 0 }, "tail": func(r *Request) { r.TradeToT += M30MS }} {
		t.Run(name, func(t *testing.T) {
			rr := r
			rr.Config = profile(t, SourceMode)
			change(&rr)
			if _, e := Run(rr); e == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}

// A deterministic invented price/volume path exercises the public pipeline;
// no real-market bars or observed strategy outcomes enter this fixture.
func waveM5(native int) marketdata.Series {
	s := m5(native)
	previous := 100.
	for i := 0; i < native; i++ {
		c := 100 + .015*float64(i) + 4*math.Sin(float64(i)*.13) + .8*math.Sin(float64(i)*.71)
		o := previous
		for j := 0; j < 6; j++ {
			k := i*6 + j
			s.O[k] = o + (c-o)*float64(j)/6
			s.C[k] = o + (c-o)*float64(j+1)/6
			s.H[k] = math.Max(s.O[k], s.C[k]) + .08
			s.L[k] = math.Min(s.O[k], s.C[k]) - .08
			s.V[k] = 10 + float64((i*17)%23)
		}
		previous = c
	}
	return s
}
func TestPublicPipelineInventedTradesAndFutureMutation(t *testing.T) {
	for _, mode := range []string{SourceMode, AuditMode} {
		for _, spread := range []float64{0, 1} {
			t.Run(mode+string(rune('0'+int(spread))), func(t *testing.T) {
				req := Request{Config: profile(t, mode), M5: waveM5(700), WarmupFromT: 0, TradeFromT: 40 * M30MS, TradeToT: 500 * M30MS, Costs: costs(spread)}
				first, e := Run(req)
				if e != nil {
					t.Fatal(e)
				}
				if len(first.Trades) == 0 || len(first.Signals) == 0 {
					t.Fatalf("invented fixture did not exercise trades: %d signals %d trades", len(first.Signals), len(first.Trades))
				}
				sum := 0.
				for _, tr := range first.Trades {
					closeTo(t, tr.Net, (float64(tr.Signal.Direction)*(tr.Exit-tr.Entry)-1)*tr.Quantity)
					closeTo(t, tr.EntryFee, tr.Quantity*.5)
					closeTo(t, tr.ExitFee, tr.Quantity*.5)
					if tr.EntryT < tr.Signal.Time || tr.ExitT < tr.EntryT || tr.ExitT > req.TradeToT {
						t.Fatal("noncausal trade times")
					}
					sum += tr.Net
				}
				closeTo(t, sum, first.Summary.Net)
				closeTo(t, first.Summary.CashEquity, 10000+sum-first.Summary.OpenEntryFee)
				for i := 500 * 6; i < req.M5.Len(); i++ {
					req.M5.O[i] *= 2
					req.M5.H[i] *= 2
					req.M5.L[i] *= 2
					req.M5.C[i] *= 2
					req.M5.V[i] *= 10
				}
				after, e := Run(req)
				if e != nil {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(first, after) {
					t.Fatal("future mutation changed genuine trade history/exposure")
				}
				req.M5 = waveM5(500)
				short, e := Run(req)
				if e != nil {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(first, short) {
					t.Fatal("future append affected history")
				}
			})
		}
	}
}
