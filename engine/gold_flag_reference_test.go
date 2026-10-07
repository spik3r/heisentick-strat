package engine

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
	"math"
	"reflect"
	"strings"
	"testing"
)

const goldFlagTestSource = `dsl v7
strategy "Invented flag mechanics" { description "Synthetic reference only" }
market { goldflag timeframe M30 from M15 }
setup { type: gold flag reference
 goldflag policy PR388_CAUSAL_STRESS_V1
}`

func goldFlagConfig(t *testing.T) dsl.Config {
	t.Helper()
	p, e := dsl.Parse(goldFlagTestSource)
	if e != nil || len(p.Errors) > 0 {
		t.Fatalf("parse %v %v", e, p.Errors)
	}
	return p.Config
}
func goldFlagTestBar(i int, o, h, l, c float64) GoldFlagBar {
	ts := int64(i) * goldFlagM30
	return GoldFlagBar{Index: i, OpenT: ts, CloseT: ts + goldFlagM30, FirstObservedT: ts, LastObservedCloseT: ts + goldFlagM30, Open: o, High: h, Low: l, Close: c, SourceCount: 2, Complete: true}
}
func goldFlagTestBars(n int) []GoldFlagBar {
	bs := make([]GoldFlagBar, n)
	for i := range bs {
		bs[i] = goldFlagTestBar(i, 95, 96, 94, 95)
	}
	return bs
}
func goldFlagTestSignal(i, d int) GoldFlagSignal {
	s := GoldFlagSignal{SignalIdx: i, Side: d, ATR: 10, EdgeHi: 100, EdgeLo: 90, Pole: 30, Level: "h4"}
	if d < 0 {
		s.EdgeHi = 110
		s.EdgeLo = 100
	}
	return s
}
func goldFlagMirror(b []GoldFlagBar, d int) []GoldFlagBar {
	out := append([]GoldFlagBar{}, b...)
	if d < 0 {
		for i := range out {
			x := out[i]
			out[i].Open = 200 - x.Open
			out[i].High = 200 - x.Low
			out[i].Low = 200 - x.High
			out[i].Close = 200 - x.Close
		}
	}
	return out
}

func TestGoldFlagMirroredCanonicalLifecycle(t *testing.T) {
	cases := []struct {
		name           string
		fill           GoldFlagBar
		later          *GoldFlagBar
		reason, status string
		exitIndex      int
		alternatives   int
	}{
		{"dual entry", goldFlagTestBar(2, 100, 103, 88, 90), nil, "sl-same-bar", "closed", 2, 2},
		{"fill target", goldFlagTestBar(2, 100, 126, 99, 120), nil, "tp-same-bar", "closed", 2, 0},
		{"opening target", goldFlagTestBar(2, 100, 104, 99, 103), goldFlagPointer(goldFlagTestBar(3, 126, 127, 88, 90)), "tp-open-capped", "closed", 3, 0},
		{"opening stop", goldFlagTestBar(2, 100, 104, 99, 103), goldFlagPointer(goldFlagTestBar(3, 80, 127, 79, 90)), "sl-open", "closed", 3, 0},
		{"opening cancel", goldFlagTestBar(2, 88, 140, 87, 130), nil, "", "cancelled-open-opposite", 0, 0},
		{"no retroactive cancel", goldFlagTestBar(2, 102, 103, 88, 90), nil, "sl-same-bar", "closed", 2, 0},
		{"later dual bracket", goldFlagTestBar(2, 100, 104, 99, 103), goldFlagPointer(goldFlagTestBar(3, 103, 127, 88, 90)), "sl", "closed", 3, 2},
		{"triple alternatives", goldFlagTestBar(2, 100, 127, 88, 90), nil, "sl-same-bar", "closed", 2, 3},
		{"opening entry dual bracket", goldFlagTestBar(2, 102, 129, 88, 90), nil, "sl-same-bar", "closed", 2, 2},
		{"opposite alone", goldFlagTestBar(2, 95, 99, 88, 90), nil, "", "cancelled-opposite", 0, 0},
	}
	for _, d := range []int{1, -1} {
		for _, c := range cases {
			t.Run(c.name+string(rune('2'+d)), func(t *testing.T) {
				bs := goldFlagTestBars(4)
				bs[2] = c.fill
				if c.later != nil {
					bs[3] = *c.later
				}
				o := goldFlagOrder(goldFlagMirror(bs, d), goldFlagTestSignal(1, d), .06)
				if o.Status != c.status || o.Reason != c.reason {
					t.Fatalf("got %+v", o)
				}
				if o.ExitIdx != nil && *o.ExitIdx != c.exitIndex {
					t.Fatal("exit row")
				}
				alts := 0
				if len(o.Ambiguities) > 0 {
					alts = len(o.Ambiguities[0].Alternatives)
				}
				if alts != c.alternatives {
					t.Fatalf("alternatives %d", alts)
				}
				if o.Status == "closed" {
					want := (float64(d)*(*o.Exit-*o.Entry) - 2*.06) / *o.Risk
					if *o.NetR != want {
						t.Fatal("cost math")
					}
				}
				if c.name == "opening target" && *o.Exit != *o.TP {
					t.Fatal("target must be capped")
				}
				if c.name == "opening stop" && *o.GrossR >= -1 {
					t.Fatal("stop gap must lose more than R")
				}
				if c.name == "no retroactive cancel" && *o.EntryGap != 1 {
					t.Fatal("actual gap fill")
				}
			})
		}
	}
}

func TestGoldFlagObservedClockTerminalAndCosts(t *testing.T) {
	for _, d := range []int{1, -1} {
		bs := goldFlagTestBars(8)
		bs[6] = goldFlagTestBar(6, 100, 126, 99, 125)
		o := goldFlagOrder(goldFlagMirror(bs, d), goldFlagTestSignal(1, d), 0)
		if o.Status != "expired" || o.ResolutionIdx != 5 {
			t.Fatal("four-row expiry", o)
		}
		bs = goldFlagTestBars(3)
		o = goldFlagOrder(goldFlagMirror(bs, d), goldFlagTestSignal(1, d), 0)
		if o.Status != "working-at-end" {
			t.Fatal(o)
		}
		bs[2] = goldFlagTestBar(2, 100, 104, 99, 103)
		o = goldFlagOrder(goldFlagMirror(bs, d), goldFlagTestSignal(1, d), 0)
		if o.Status != "open-at-end" || o.Exit != nil {
			t.Fatal("fabricated terminal close", o)
		}
		bs = goldFlagTestBars(28)
		for i := 2; i < len(bs); i++ {
			bs[i] = goldFlagTestBar(i, 103, 104, 99, 103)
		}
		bs[2] = goldFlagTestBar(2, 100, 104, 99, 103)
		o = goldFlagOrder(goldFlagMirror(bs, d), goldFlagTestSignal(1, d), 0)
		if o.Reason != "time" || *o.ExitIdx != 26 {
			t.Fatal("fill+24", o)
		}
		for _, cost := range []float64{0, .06, .15, .25, .5} {
			v := goldFlagOrder(goldFlagMirror(bs, d), goldFlagTestSignal(1, d), cost)
			if *v.Entry != *o.Entry || *v.Exit != *o.Exit || *v.Risk != *o.Risk || *v.TP != *o.TP || math.Abs(*v.NetR-(*o.GrossR - 2*cost / *o.Risk)) > 1e-15 {
				t.Fatal("cost changed execution")
			}
		}
		bs[26] = goldFlagTestBar(26, 103, 126, 99, 125)
		o = goldFlagOrder(goldFlagMirror(bs, d), goldFlagTestSignal(1, d), 0)
		if o.Reason != "tp" {
			t.Fatal("bracket before time")
		}
	}
	bs := goldFlagTestBars(6)
	for j := 2; j < len(bs); j++ {
		bs[j].OpenT += 50 * 3600000
		bs[j].CloseT += 50 * 3600000
		bs[j].FirstObservedT += 50 * 3600000
		bs[j].LastObservedCloseT += 50 * 3600000
	}
	bs[3].High = 102
	o := goldFlagOrder(bs, goldFlagTestSignal(1, 1), 0)
	if o.FillIdx == nil || *o.FillIdx != 3 {
		t.Fatal("gap incorrectly expired order")
	}
	clock := goldFlagClock(bs, o)
	if len(clock.ActiveGapRows) != 1 || *clock.ExpiryCloseMS <= clock.ElapsedTwoHourMS {
		t.Fatal("observed clock lost gap", clock)
	}
}

func TestGoldFlagCooldownOccupancyAndExitRow(t *testing.T) {
	bars := goldFlagTestBars(35)
	rows := make([]GoldFlagSnapshot, len(bars))
	for i, b := range bars {
		rows[i] = GoldFlagSnapshot{GoldFlagBar: b, Candidate: goldFlagPointer(goldFlagTestSignal(i, 1))}
	}
	// Rejected risk consumes the same six-row cooldown; row8 becomes next signal.
	rows[1].Candidate.EdgeLo = 100
	rows[1].Candidate.EdgeHi = 101
	os, _ := goldFlagPortfolio(bars, rows, 0)
	if os[0].Status != "invalid-risk" || os[1].SignalIdx != 8 {
		t.Fatalf("cooldown %+v", os)
	}
	// Fill row2, no bracket until time row26. Row26 cannot emit a new signal.
	for i := 2; i < len(bars); i++ {
		bars[i] = goldFlagTestBar(i, 103, 104, 99, 103)
	}
	bars[2] = goldFlagTestBar(2, 100, 104, 99, 103)
	rows[1].Candidate = goldFlagPointer(goldFlagTestSignal(1, 1))
	os, _ = goldFlagPortfolio(bars, rows, 0)
	if os[0].Status != "closed" || *os[0].ExitIdx != 26 || os[1].SignalIdx != 27 {
		t.Fatalf("occupancy/exit row %+v", os)
	}
}

func TestGoldFlagBrokerOptInDoesNotChangeLegacy(t *testing.T) {
	bars := goldFlagTestBars(3)
	bars[2] = goldFlagTestBar(2, 126, 127, 88, 90)
	for _, opt := range []bool{false, true} {
		b := goldFlagBroker(bars)
		b.openPosition(sideLong, 101, order{SL: 89, TP: 125, NoSlip: true, Size: 1, HasSize: true}, 1)
		b.position.OpeningTargetPrecedence = opt
		b.resolveIntrabarExit(2)
		want := "sl"
		if opt {
			want = "tp"
		}
		if b.trades[0].Reason != want {
			t.Fatalf("opt=%v changed rule", opt)
		}
	}
}

func TestGoldFlagSourceAggregationIndicatorsAndCausality(t *testing.T) {
	raw := []marketdata.Bar{}
	for i := 0; i < 420; i++ {
		raw = append(raw, marketdata.Bar{T: float64(int64(i) * goldFlagM15), O: 100, H: 101, L: 99, C: 100, V: 1})
	}
	r := GoldFlagReferenceRequest{Config: goldFlagConfig(t), M15: marketdata.SeriesFromBars(raw), FromT: 0, ToT: 210 * goldFlagM30, CostPerFill: .06}
	a, e := RunGoldFlagReference(r)
	if e != nil {
		t.Fatal(e)
	}
	if a.Snapshots[13].ATR != nil || *a.Snapshots[14].ATR != 2 || a.Snapshots[95].H4 == nil || a.Snapshots[94].H4 != nil {
		t.Fatal("ATR seed/HTF readiness")
	}
	// A same-time H4 close is available, never the developing next bucket.
	if a.Snapshots[95].H4.AvailableT != a.Snapshots[95].CloseT {
		t.Fatal("boundary visibility")
	}
	raw = append(raw[:5:5], raw[6:]...)
	r.M15 = marketdata.SeriesFromBars(raw)
	b, e := RunGoldFlagReference(r)
	if e != nil {
		t.Fatal(e)
	}
	if b.Snapshots[2].Complete || b.Snapshots[2].SourceCount != 1 {
		t.Fatal("partial discarded")
	}
	raw = append(raw[:4:4], raw[5:]...)
	r.M15 = marketdata.SeriesFromBars(raw)
	b, e = RunGoldFlagReference(r)
	if e != nil {
		t.Fatal(e)
	}
	if len(b.Gaps) != 1 {
		t.Fatal("missing bucket hidden")
	}
	// Future price mutations after the explicit watermark cannot change results.
	r.ToT = 100 * goldFlagM30
	before, e := RunGoldFlagReference(r)
	if e != nil {
		t.Fatal(e)
	}
	r.M15.H[len(r.M15.H)-1] = 99999
	after, e := RunGoldFlagReference(r)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("future price leak")
	}
}

func TestGoldFlagInputRefusal(t *testing.T) {
	raw := []marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: float64(goldFlagM15), O: 100, H: 101, L: 99, C: 100, V: 1}}
	base := GoldFlagReferenceRequest{Config: goldFlagConfig(t), M15: marketdata.SeriesFromBars(raw), FromT: 0, ToT: goldFlagM30, CostPerFill: 0}
	for _, mutate := range []func(*GoldFlagReferenceRequest){func(r *GoldFlagReferenceRequest) { r.CostPerFill = .07 }, func(r *GoldFlagReferenceRequest) { r.FromT = 1 }, func(r *GoldFlagReferenceRequest) { r.ToT = 0 }, func(r *GoldFlagReferenceRequest) { r.M15.V = nil }, func(r *GoldFlagReferenceRequest) { r.M15.T[1] = 0 }, func(r *GoldFlagReferenceRequest) { r.M15.H[0] = math.Inf(1) }} {
		r := base
		r.M15 = marketdata.SeriesFromBars(raw)
		mutate(&r)
		if _, e := RunGoldFlagReference(r); e == nil {
			t.Fatal("malformed request accepted")
		}
	}
	if _, e := Run(RunRequest{Config: base.Config}); e == nil || !strings.Contains(e.Error(), dsl.GoldFlagReferenceDedicatedRunnerRequired) {
		t.Fatal("generic route admitted", e)
	}
}

func TestGoldFlagDetectorExactRecipe(t *testing.T) {
	for _, d := range []int{1, -1} {
		bars := goldFlagTestBars(14)
		for j := 1; j <= 6; j++ {
			x := 100 + float64(j-1)*2
			bars[j] = goldFlagTestBar(j, x, x+2, x, x+1)
		}
		for j := 7; j <= 12; j++ {
			bars[j] = goldFlagTestBar(j, 110, 112, 109, 111)
		}
		bars = goldFlagMirror(bars, d)
		atr := 6.
		ext := &GoldFlagExtreme{High: 118, Low: 82}
		s := goldFlagDetect(bars, 12, &atr, ext, nil)
		if s == nil || s.Side != d || s.Pole != 12 || s.Level != "h4" {
			t.Fatalf("exact pole and tolerance %+v", s)
		}
		tooHigh := 6.0000001
		if goldFlagDetect(bars, 12, &tooHigh, ext, nil) != nil {
			t.Fatal("pole ATR inequality")
		}
		if goldFlagDetect(bars, 12, &atr, nil, nil) != nil {
			t.Fatal("location unavailable")
		}
		if goldFlagDetect(bars, 12, nil, ext, nil) != nil {
			t.Fatal("ATR unavailable")
		}
		// Expanding the location threshold by one representable amount fails.
		bad := *ext
		if d == 1 {
			bad.High = math.Nextafter(ext.High, math.Inf(1))
		} else {
			bad.Low = math.Nextafter(ext.Low, math.Inf(-1))
		}
		if goldFlagDetect(bars, 12, &atr, &bad, nil) != nil {
			t.Fatal("one-sided tolerance must be exact")
		}
		// Equal extrema keep their first occurrence, rather than the last.
		tied := append([]GoldFlagBar{}, bars...)
		if d == 1 {
			tied[6].Low = tied[1].Low
		} else {
			tied[6].High = tied[1].High
		}
		if goldFlagDetect(tied, 12, &atr, ext, nil) == nil {
			t.Fatal("first tied pole extremum lost")
		}
	}
}

func TestGoldFlagSignalPrefixAndKnownEventTimes(t *testing.T) {
	bars := goldFlagTestBars(160)
	for i := range bars {
		bars[i] = goldFlagTestBar(i, 100, 101, 99, 100)
	}
	for j := 144; j < 150; j++ {
		x := 100 + float64(j-144)*4
		bars[j] = goldFlagTestBar(j, x, x+4, x, x+2)
	}
	for j := 150; j < 156; j++ {
		bars[j] = goldFlagTestBar(j, 122, 124, 121, 123)
	}
	full := goldFlagSnapshots(bars)
	if full[155].Candidate == nil {
		t.Fatal("invented signal fixture must exercise detector")
	}
	prefix := goldFlagSnapshots(bars[:156])
	if !reflect.DeepEqual(full[155], prefix[155]) {
		t.Fatal("developing HTF leaked future")
	}
	bars[159].High = 99999
	if !reflect.DeepEqual(goldFlagSnapshots(bars)[155], full[155]) {
		t.Fatal("future extrema altered signal")
	}
	bs := goldFlagTestBars(4)
	bs[2] = goldFlagTestBar(2, 102, 103, 88, 90)
	bs[2].FirstObservedT += goldFlagM15
	bs[2].SourceCount = 1
	bs[2].Complete = false
	o := goldFlagOrder(bs, goldFlagTestSignal(1, 1), 0)
	if o.Events[1].Time.LowerMS != bs[2].FirstObservedT || o.Events[1].Time.UpperMS != bs[2].FirstObservedT {
		t.Fatal("partial actual open timestamp lost")
	}
	if o.Events[2].Time.LowerInclusive || o.Events[2].Time.UpperInclusive {
		t.Fatal("nonopening crossing endpoints must be exclusive")
	}
	if o.Events[2].AfterEventID == nil || *o.Events[2].AfterEventID != 1 {
		t.Fatal("entry-before-exit constraint lost")
	}
}
