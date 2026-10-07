package master

import (
	"encoding/json"
	"errors"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/regime"
	"github.com/spik3r/heisentick-strat/marketdata"
	"math"
	"reflect"
	"strings"
	"testing"
)

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-10*math.Max(1, math.Abs(want)) {
		t.Fatalf("got %.16g want %.16g", got, want)
	}
}
func config(t *testing.T, mode string) dsl.Config {
	t.Helper()
	p, e := dsl.Parse(`dsl v7
strategy "Invented Master test" {description "Synthetic OHLC only"}
market {master timeframe M30 from M5}
setup {type: master structural master profile v10-phase0-floor-half-reference-v1 master mode ` + mode + `}`)
	if e != nil || len(p.Errors) > 0 {
		t.Fatalf("parse %v %v", e, p.Errors)
	}
	return p.Config
}
func costs(sp float64) Costs { return Costs{Spread: sp, FeePerUnitSide: .5, InitialEquity: 10000} }
func bars(n int) marketdata.Series {
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
func request(t *testing.T, n int, mode string) Request {
	return Request{Config: config(t, mode), M5: bars(n), WarmupFromT: 0, TradeFromT: 40 * M30MS, TradeToT: int64(n) * M30MS, Costs: costs(0)}
}
func row(i int, o, h, l, c, atr, st float64, dr, candidate int) IndicatorRow {
	b := int64(i) * M30MS
	snap := &H4Snapshot{AvailableT: b / H4MS * H4MS, Regime: -candidate}
	return IndicatorRow{IndicatorRow: regime.IndicatorRow{NativeBar: NativeBar{BucketT: b, FirstObservedT: b, CloseT: b + M30MS, Open: o, High: h, Low: l, Close: c, Volume: 200, Count: 6, Complete: true}, ATR14: pointer(atr), Supertrend: pointer(st), VolumeMean: pointer(100), Regime: dr, Candidate: candidate}, ATRPercent: pointer(100 * atr / c), HistoricalH4: snap, StableH4: snap, SourceCandidate: candidate, ProtectedCandidate: candidate}
}
func lifecycle(n int) Request {
	return Request{WarmupFromT: 0, TradeFromT: M30MS, TradeToT: int64(n) * M30MS, Costs: costs(0)}
}
func signal(d int, anchor, atr, st, hi, lo float64) Signal {
	return Signal{Signal: regime.Signal{Time: M30MS, Direction: d, Anchor: anchor, ATR: atr, Supertrend: st}, High: hi, Low: lo}
}

func TestMasterSharedPrimitiveHandDerived(t *testing.T) {
	native := make([]NativeBar, 40)
	for i := range native {
		v := float64(i + 1)
		native[i] = NativeBar{Open: v, High: v + 1, Low: v - 1, Close: v, Volume: 60, Complete: true}
	}
	x, e := regime.ReferenceBarIndicators(native)
	if e != nil {
		t.Fatal(e)
	}
	if x[9].HMA9 != nil || x[23].HMA21 != nil || x[27].HMA25 != nil || x[8].ATR10 != nil || x[12].ATR14 != nil {
		t.Fatal("early warmup")
	}
	near(t, *x[10].HMA9, 11)
	near(t, *x[24].HMA21, 24+1./3)
	near(t, *x[28].HMA25, 28+1./3)
	near(t, *x[9].ATR10, 2)
	near(t, *x[13].ATR14, 2)
	native[39].Volume = 120
	x, e = regime.ReferenceBarIndicators(native)
	if e != nil {
		t.Fatal(e)
	}
	near(t, *x[39].VolumeMean, 63)
	if !x[39].VolumeOK || !x[39].Complete40 || x[38].Complete40 {
		t.Fatal("readiness/current volume")
	}
	native[39].Volume = 60
	x, _ = regime.ReferenceBarIndicators(native)
	if x[39].VolumeOK {
		t.Fatal("volume equality")
	}
	native[0].Complete = false
	x, _ = regime.ReferenceBarIndicators(native)
	if x[39].Complete40 {
		t.Fatal("partial completeness")
	}
	native[0].Complete = true
	for i := 20; i < len(native); i++ {
		native[i].BucketT += 2 * 86400000
	}
	x, _ = regime.ReferenceBarIndicators(native)
	if !x[39].Complete40 {
		t.Fatal("whole missing bars reset inherited readiness")
	}
}

func TestMasterH4BoundaryAndDefaultDirection(t *testing.T) {
	r := request(t, 96, SourceMode)
	for i := 80 * 6; i < 88*6; i++ {
		r.M5.O[i] = 110
		r.M5.H[i] = 111
		r.M5.L[i] = 109
		r.M5.C[i] = 110
	}
	rows, h4, used, e := buildIndicators(r)
	if e != nil {
		t.Fatal(e)
	}
	if used != 576 || len(h4) != 12 || h4[0].Count != 48 || h4[0].M30Count != 8 {
		t.Fatal("aggregation")
	}
	if h4[8].ATR10 != nil || h4[8].Supertrend != nil || h4[8].Regime != 1 {
		t.Fatal("default H4 regime changed")
	}
	if rows[6].HistoricalH4 != nil || rows[7].HistoricalH4.AvailableT != H4MS || rows[7].StableH4 != nil || rows[8].StableH4.AvailableT != H4MS {
		t.Fatal("first snapshot horizon")
	}
	b := rows[87]
	if b.HistoricalH4.Regime != -1 || b.StableH4.Regime != 1 || b.HistoricalH4.AvailableT != 11*H4MS || b.StableH4.AvailableT != 10*H4MS || rows[88].StableH4.Regime != -1 {
		t.Fatalf("boundary collapsed: %+v %+v", b.HistoricalH4, b.StableH4)
	}
	// The short H4 reference is accepted without an invented full-session filter.
	r.M5 = marketdata.SeriesFromBars(append(r.M5.Bars()[:2], r.M5.Bars()[3:]...))
	_, h4, _, e = buildIndicators(r)
	if e != nil || h4[0].Count != 47 || h4[0].Complete {
		t.Fatal("short H4 silently dropped or certified", e)
	}
}

func TestMasterGateThresholdAndUnavailableSnapshot(t *testing.T) {
	for _, c := range []struct {
		base, dr int
		pct      float64
		want     int
	}{{1, -1, .08, 1}, {-1, 1, .08, -1}, {1, -1, math.Nextafter(.08, 0), 0}, {1, 1, 1, 0}, {0, -1, 1, 0}} {
		if got := gatedCandidate(c.base, pointer(c.pct), &H4Snapshot{Regime: c.dr}); got != c.want {
			t.Fatalf("gate %+v got%d", c, got)
		}
	}
	if gatedCandidate(1, pointer(1), nil) != 0 || gatedCandidate(1, nil, &H4Snapshot{Regime: -1}) != 0 {
		t.Fatal("unavailable data passed")
	}
	// Python gates on direction, not a fabricated readiness of the H4 line.
	if gatedCandidate(-1, pointer(.08), &H4Snapshot{Regime: 1}) != -1 {
		t.Fatal("added H4 line gate")
	}
}

func TestMasterSourceReversibleLockWideningAndPreentry(t *testing.T) {
	for _, d := range []int{1, -1} {
		st := 70.
		if d == -1 {
			st = 130
		}
		s := signal(d, 100, 10, st, 110, 90)
		p, _, e := openPosition(s, row(2, 100, 110, 90, 100, 10, st, -d, 0).NativeBar, SourceMode, costs(0), 10000)
		if e != nil {
			t.Fatal(e)
		}
		first := row(2, 100, 110, 90, 100, 10, st, -d, 0)
		a, e := manage(p, first, -d, SourceMode, 0)
		if e != nil {
			t.Fatal(e)
		}
		if a.Locked || p.SL == nil {
			t.Fatal("early activation")
		}
		near(t, *p.SL, 100-float64(d)*20)
		second := row(3, 100, 120, 80, 100, 10, st, -d, 0)
		a, e = manage(p, second, -d, SourceMode, 0)
		if e != nil {
			t.Fatal(e)
		}
		if !a.BecameLocked || !a.HypotheticalLockRelaxation || !a.StopWidened || a.PreentryOnlyActivation {
			t.Fatalf("source diagnostics %+v", a)
		}
		near(t, *p.SL, st)
		third := row(4, 100, 110, 90, 100, 20, st, -d, 0)
		a, e = manage(p, third, -d, SourceMode, 0)
		if e != nil {
			t.Fatal(e)
		}
		if !a.Deactivated || p.LockDeactivations != 1 || p.Locked {
			t.Fatal("ATR activation was latched")
		}
		near(t, *p.TP, 100+float64(d)*80)
		// A signal extreme alone may arm SOURCE before the position experiences it.
		s.High = 130
		s.Low = 70
		p, _, _ = openPosition(s, first.NativeBar, SourceMode, costs(0), 10000)
		a, e = manage(p, first, -d, SourceMode, 0)
		if e != nil || !a.PreentryOnlyActivation || p.PreentryOnlyActivations != 1 {
			t.Fatal("signal extreme lost", e)
		}
	}
}

func TestMasterProtectedFrozenStickyNoWidenAndFractionalPrices(t *testing.T) {
	for _, d := range []int{1, -1} {
		flip := func(v float64) float64 {
			if d == 1 {
				return v
			}
			return 200 - v
		}
		s := signal(d, 100, 2, flip(90), 200, 1)
		b := row(2, flip(103), 108, 92, flip(104), 2, flip(100), -d, 0).NativeBar
		p, _, e := openPosition(s, b, ProtectedMode, costs(0), 10000)
		if e != nil {
			t.Fatal(e)
		}
		near(t, *p.SL, flip(99))
		near(t, *p.TP, flip(111))
		r := row(2, flip(103), 106.9, 93.1, flip(104), 50, flip(102.123456), -d, 0)
		a, e := manage(p, r, -d, ProtectedMode, 0)
		if e != nil {
			t.Fatal(e)
		}
		if a.Locked || a.PreentryOnlyActivation {
			t.Fatal("pre-fill extreme/ATR influenced protected activation")
		}
		near(t, *p.SL, flip(99))
		r.High = 107
		r.Low = 93
		a, e = manage(p, r, -d, ProtectedMode, 0)
		if e != nil || !a.BecameLocked {
			t.Fatal("exact frozen threshold", e)
		}
		near(t, *p.SL, flip(102.123456))
		r.Supertrend = pointer(flip(101))
		r.High = 105
		r.Low = 95
		r.ATR14 = pointer(500)
		a, e = manage(p, r, -d, ProtectedMode, 0)
		if e != nil {
			t.Fatal(e)
		}
		if !p.Locked || a.StopWidened || a.HypotheticalLockRelaxation || !a.RejectedWorseST || p.StopWidenings != 0 || p.LockDeactivations != 0 {
			t.Fatal("protected diagnostic/execution conflated")
		}
		near(t, *p.SL, flip(102.123456))
		near(t, *p.TP, flip(111))
	}
}

func TestMasterDiagnosticToleranceDoesNotMoveExecution(t *testing.T) {
	p, _, _ := openPosition(signal(1, 100, 2, 90, 101, 99), row(2, 100, 104, 99, 103, 2, 96, -1, 0).NativeBar, ProtectedMode, costs(0), 10000)
	p.Locked = true
	p.SL = pointer(96)
	r := row(3, 100, 104, 99, 103, 2, 96+5e-10, -1, 0)
	a, e := manage(p, r, -1, ProtectedMode, 0)
	if e != nil {
		t.Fatal(e)
	}
	if *p.SL != 96+5e-10 || a.StopWidened {
		t.Fatal("execution used diagnostic epsilon")
	}
}

func TestMasterEntryBarBracketAndSourceNaked(t *testing.T) {
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 0), row(1, 100, 101, 99, 100, 2, 90, -1, 1), row(2, 103, 106, 95, 104, 2, 90, -1, 0)}
	for _, mode := range []string{SourceMode, ProtectedMode} {
		out, e := execute(lifecycle(3), mode, rows)
		if e != nil {
			t.Fatal(e)
		}
		if mode == SourceMode {
			if len(out.Trades) != 0 || out.OpenPosition == nil || out.OpenPosition.FirstLiveT != 3*M30MS {
				t.Fatal("source entry bar protected")
			}
			near(t, *out.OpenPosition.TP, 108)
		} else {
			if len(out.Trades) != 1 || out.Trades[0].Reason != "stop" || out.Trades[0].Exit != 99 || out.OpenPosition != nil {
				t.Fatal("protected bracket missing")
			}
		}
	}
}

func TestMasterPendingFlipKeepsBracketAndOpeningPriority(t *testing.T) {
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 0), row(1, 100, 101, 99, 100, 2, 90, -1, 1), row(2, 100, 102, 98, 100, 2, 110, 1, 0), row(3, 90, 92, 88, 91, 2, 110, 1, 0)}
	partial := lifecycle(3)
	out, e := execute(partial, ProtectedMode, rows[:3])
	if e != nil {
		t.Fatal(e)
	}
	if out.OpenPosition == nil || out.OpenPosition.ForcedReason != "regime_flip_market_exit" || out.OpenPosition.SL == nil || out.OpenPosition.TP == nil {
		t.Fatal("pending close canceled bracket")
	}
	// Next actually observed open can be separated by a scheduled/session gap.
	rows[3].BucketT += 2 * 86400000
	rows[3].FirstObservedT = rows[3].BucketT + M5MS
	rows[3].CloseT = rows[3].BucketT + M30MS
	r := lifecycle(4)
	r.TradeToT = rows[3].CloseT
	out, e = execute(r, ProtectedMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Trades) != 1 || out.Trades[0].Reason != "regime_flip_market_exit" || out.Trades[0].ExitT != rows[3].FirstObservedT || out.Trades[0].Exit != 90 {
		t.Fatal("market priority or gap timestamp")
	}
}

func TestMasterNativeBarrierLongShortTiesGapsAndImprovement(t *testing.T) {
	b := row(1, 100, 110, 90, 100, 2, 90, -1, 0).NativeBar
	for _, c := range []struct {
		d              int
		sl, tp, spread float64
		reason         string
		price          float64
	}{{1, 95, 105, 0, "stop_ambiguous", 95}, {-1, 105, 95, 0, "target_ambiguous", 95}, {-1, 106, 96, 1, "target_ambiguous", 96}, {1, 101, 110, 0, "stop_gap", 100}, {1, 90, 99, 0, "target_gap", 100}, {-1, 99, 90, 0, "stop_gap", 100}, {-1, 110, 101, 0, "target_gap", 100}} {
		p, reason, when := regime.ReferenceBarrier(b, c.sl, c.tp, c.d, c.spread)
		if reason != c.reason || p != c.price {
			t.Fatalf("barrier %+v got %g %s", c, p, reason)
		}
		want := b.CloseT
		if strings.Contains(reason, "gap") {
			want = b.FirstObservedT
		}
		if when != want {
			t.Fatal("barrier time")
		}
	}
	b.High = 103
	p, reason, _ := regime.ReferenceBarrier(b, 95, 102, 1, 0)
	if p != 102 || reason != "target_ambiguous" {
		t.Fatal("strict nearer-high path")
	}
}

func TestMasterRoundedQuantityFeesAndTypedFailure(t *testing.T) {
	for _, raw := range []float64{.2, .2 - 5e-13, .2 - 2e-12} {
		entry := 1000 / raw
		p, cash, e := openPosition(signal(1, entry, 10, entry-30, entry+1, entry-1), NativeBar{Open: entry}, SourceMode, costs(0), 10000)
		if e != nil {
			t.Fatal(e)
		}
		want := .2
		if raw < .2-1e-12 {
			want = .1
		}
		near(t, p.Quantity, want)
		near(t, p.EntryFee, want*.5)
		near(t, cash, 10000-want*.5)
	}
	p, cash, e := openPosition(signal(1, 100000, 10, 99970, 100001, 99999), NativeBar{Open: 100000}, SourceMode, costs(0), 10000)
	var typed *QuantityBelowStepError
	if !errors.As(e, &typed) || p != nil || cash != 10000 {
		t.Fatal("zero quantity not typed/pre-fee", e)
	}
	for _, d := range []int{1, -1} {
		p, _, e := openPosition(signal(d, 5000, 10, 4900, 5001, 4999), NativeBar{Open: 5000}, SourceMode, costs(1), 10000)
		if e != nil {
			t.Fatal(e)
		}
		want := 5000.
		q := .2
		if d == 1 {
			want++
			q = .1
		}
		near(t, p.Entry, want)
		near(t, p.Quantity, q)
	}
}

func TestMasterPartialPendingFillUsesActualOpen(t *testing.T) {
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 0), row(1, 100, 101, 99, 100, 2, 90, -1, 1), row(2, 103, 104, 100, 103, 2, 90, -1, 0)}
	rows[2].Complete = false
	rows[2].Count = 5
	rows[2].FirstObservedT += M5MS
	out, e := execute(lifecycle(3), SourceMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if out.OpenPosition == nil || out.OpenPosition.EntryT != rows[2].FirstObservedT || out.Summary.PartialEntryFills != 1 || out.Summary.PartialPositionBars != 1 {
		t.Fatal("future completeness canceled/backdated entry")
	}
}

func TestMasterSourceMarketableTargetNotLatched(t *testing.T) {
	p, _, _ := openPosition(signal(1, 100, 2, 90, 101, 99), NativeBar{Open: 100}, SourceMode, costs(0), 10000)
	a, e := manage(p, row(2, 100, 120, 99, 110, 1, 95, -1, 0), -1, SourceMode, 0)
	if e != nil {
		t.Fatal(e)
	}
	if !a.TargetMarketable || p.ForcedReason != "" {
		t.Fatal("marketable target was latched")
	}
	b := row(3, 100, 103, 96, 101, 1, 95, -1, 0).NativeBar
	_, reason, _ := regime.ReferenceBarrier(b, *p.SL, *p.TP, 1, 0)
	if reason != "" {
		t.Fatal("gap-back retro target fill")
	}
}

func TestMasterPublicPipelineWatermarkAndFutureInvariance(t *testing.T) {
	r := request(t, 160, SourceMode)
	// Invented triangular price cycle with deterministic variable volume; not history.
	for i := range r.M5.T {
		j := i / 6
		k := j % 32
		v := 100 + float64(k)
		if k > 16 {
			v = 100 + float64(32-k)
		}
		r.M5.O[i] = v
		r.M5.C[i] = v + .2
		r.M5.H[i] = v + 1
		r.M5.L[i] = v - 1
		r.M5.V[i] = float64(10 + j%7)
	}
	for _, mode := range []string{SourceMode, ProtectedMode} {
		r.Config = config(t, mode)
		out, e := Run(r)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := json.Marshal(out)
		if e != nil || len(raw) == 0 || out.PineParityVerified || out.CostComplete || len(out.Indicators) != 160 {
			t.Fatal("public envelope", e)
		}
		future := r
		more := r.M5.Bars()
		for i := 0; i < 6; i++ {
			b := more[len(more)-1]
			b.T += float64((i + 1) * 300000)
			b.O = 999
			b.H = 1000
			b.L = 998
			b.C = 999
			more = append(more, b)
		}
		future.M5 = marketdata.SeriesFromBars(more)
		after, e := Run(future)
		if e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(out, after) {
			t.Fatal("post-watermark future affected result")
		}
	}
	r.Config = config(t, SourceMode)
	r.M5 = marketdata.SeriesFromBars(r.M5.Bars()[:len(r.M5.T)-1])
	if _, e := Run(r); e == nil {
		t.Fatal("unclosed tail accepted")
	}
}

func TestMasterStartEndAndOpenAccountMark(t *testing.T) {
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, 2, 90, -1, 1), row(1, 100, 101, 99, 100, 2, 90, -1, 0), row(2, 100, 101, 99, 100, 2, 90, -1, 1)}
	out, e := execute(lifecycle(3), SourceMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if out.OpenPosition != nil || out.PendingSignal != nil || len(out.Signals) != 0 {
		t.Fatal("warmup/end signal leaked")
	}
	rows[1].SourceCandidate = 1
	rows[2].SourceCandidate = 0
	out, e = execute(lifecycle(3), SourceMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	p := out.OpenPosition
	if p == nil || out.Summary.Trades != 0 || out.Summary.PF != nil {
		t.Fatal("terminal liquidation fabricated")
	}
	near(t, p.Quantity, 10)
	near(t, out.Summary.OpenEntryFee, 5)
	near(t, out.Summary.ClosedEquity, 10000)
	near(t, out.Summary.CashEquity, 9995)
	near(t, out.Summary.MarkedEquity, 9995)
	near(t, out.Summary.MarkedEquityAfterClosingFee, 9990)
}

func TestMasterRequestRefusals(t *testing.T) {
	for name, mutate := range map[string]func(*Request){"cost": func(r *Request) { r.Costs.Spread = .2 }, "equity": func(r *Request) { r.Costs.InitialEquity = 20000 }, "endpoint": func(r *Request) { r.TradeToT++ }, "order": func(r *Request) { r.TradeFromT = 0 }, "volume": func(r *Request) { r.M5.V = nil }, "duplicate": func(r *Request) { r.M5.T[1] = 0 }, "offgrid": func(r *Request) { r.M5.T[1]++ }, "OHLC": func(r *Request) { r.M5.H[1] = 90 }, "nonfinite": func(r *Request) { r.M5.C[1] = math.NaN() }, "negative volume": func(r *Request) { r.M5.V[1] = -1 }, "source start": func(r *Request) { r.M5 = marketdata.SeriesFromBars(r.M5.Bars()[1:]) }} {
		t.Run(name, func(t *testing.T) {
			r := request(t, 48, SourceMode)
			mutate(&r)
			if _, e := Run(r); e == nil {
				t.Fatal("bad request admitted")
			}
		})
	}
}

func waveM5(native int) marketdata.Series {
	s := bars(native)
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
func TestMasterPublicPipelineActualInventedTrades(t *testing.T) {
	for _, mode := range []string{SourceMode, ProtectedMode} {
		for _, spread := range []float64{0, 1} {
			t.Run(mode+string(rune('0'+int(spread))), func(t *testing.T) {
				req := Request{Config: config(t, mode), M5: waveM5(700), WarmupFromT: 0, TradeFromT: 40 * M30MS, TradeToT: 500 * M30MS, Costs: costs(spread)}
				first, e := Run(req)
				if e != nil {
					t.Fatal(e)
				}
				if len(first.Trades) == 0 || len(first.Signals) == 0 {
					t.Fatalf("invented fixture did not exercise trades: %d signals %d trades", len(first.Signals), len(first.Trades))
				}
				sum := 0.
				for _, tr := range first.Trades {
					near(t, tr.Net, (float64(tr.Signal.Direction)*(tr.Exit-tr.Entry)-1)*tr.Quantity)
					near(t, tr.EntryFee, tr.Quantity*.5)
					near(t, tr.ExitFee, tr.Quantity*.5)
					if tr.EntryT < tr.Signal.Time || tr.ExitT < tr.EntryT || tr.ExitT > req.TradeToT {
						t.Fatal("noncausal trade times")
					}
					sum += tr.Net
				}
				near(t, sum, first.Summary.Net)
				near(t, first.Summary.CashEquity, 10000+sum-first.Summary.OpenEntryFee)
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

func TestMasterPublicZeroQuantityAbortsWithoutPartialResult(t *testing.T) {
	r := request(t, 500, SourceMode)
	r.M5 = waveM5(500)
	for i := range r.M5.T {
		r.M5.O[i] *= 10000
		r.M5.H[i] *= 10000
		r.M5.L[i] *= 10000
		r.M5.C[i] *= 10000
	}
	for _, mode := range []string{SourceMode, ProtectedMode} {
		r.Config = config(t, mode)
		out, e := Run(r)
		var typed *QuantityBelowStepError
		if !errors.As(e, &typed) || !reflect.DeepEqual(out, Result{}) {
			t.Fatalf("public zero quantity did not abort: %T %v", e, e)
		}
	}
}

func TestMasterNoM5IntrabarTraversal(t *testing.T) {
	// Invented entry bar visits its low in its first M5 candle. Its aggregated
	// M30 high is closer to the open, so the declared native path visits high
	// first. The dedicated runner must use that M30 convention, not M5 ordering.
	s := bars(3)
	for i := 12; i < 18; i++ {
		s.O[i] = 100
		s.C[i] = 100
		s.H[i] = 101
		s.L[i] = 99
	}
	s.L[12] = 90
	s.H[17] = 103
	native, _, e := regime.ReferenceIndicators(s, 0, 3*M30MS)
	if e != nil {
		t.Fatal(e)
	}
	rows := []IndicatorRow{row(0, 100, 101, 99, 100, .5, 95, -1, 0), row(1, 100, 101, 99, 100, .5, 95, -1, 1), row(2, 100, 103, 90, 100, .5, 95, -1, 0)}
	rows[2].NativeBar = native[2].NativeBar
	out, e := execute(lifecycle(3), ProtectedMode, rows)
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Trades) != 1 || out.Trades[0].Reason != "target_ambiguous" || out.Trades[0].Exit != 102 {
		t.Fatal("M5 ordering replaced declared native path")
	}
}
