package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func adaptiveTestRules(t *testing.T) dsl.AdaptiveFlagRules {
	t.Helper()
	r, err := dsl.AdaptiveFlagPreset("INITIAL")
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func adaptiveTestBar(i int, o, h, l, c float64) AdaptiveFlagBar {
	return AdaptiveFlagBar{Index: i, OpenT: int64(i) * 1800000, CloseT: int64(i+1) * 1800000, Open: o, High: h, Low: l, Close: c, Volume: 100}
}
func adaptiveTestBars(n int) []AdaptiveFlagBar {
	out := make([]AdaptiveFlagBar, n)
	for i := range out {
		out[i] = adaptiveTestBar(i, 100, 100.5, 99.5, 100)
	}
	return out
}
func adaptiveTestSeries(bs []AdaptiveFlagBar) marketdata.Series {
	out := marketdata.NewSeries(len(bs))
	for i, b := range bs {
		out.T[i] = float64(b.OpenT)
		out.O[i] = b.Open
		out.H[i] = b.High
		out.L[i] = b.Low
		out.C[i] = b.Close
		out.V[i] = b.Volume
	}
	return out
}
func adaptiveTestReflect(bs []AdaptiveFlagBar) []AdaptiveFlagBar {
	out := append([]AdaptiveFlagBar{}, bs...)
	for i, b := range out {
		out[i].Open = float64(200 - b.Open)
		out[i].High = float64(200 - b.Low)
		out[i].Low = float64(200 - b.High)
		out[i].Close = float64(200 - b.Close)
	}
	return out
}
func adaptiveTestSignal(i int, side string) *AdaptiveFlagSignal {
	s := &AdaptiveFlagSignal{SignalIdx: i, Side: side, Trigger: 101, Stop: 99, Target: 110, PlannedTriggerToStopDistance: 2}
	if side == "short" {
		s.Trigger = 99
		s.Stop = 101
		s.Target = 90
	}
	return s
}
func adaptiveTestPortfolio(bs []AdaptiveFlagBar, r dsl.AdaptiveFlagRules, signals map[int]*AdaptiveFlagSignal) AdaptiveFlagResult {
	rows := make([]AdaptiveFlagSnapshot, len(bs))
	for i, b := range bs {
		rows[i] = AdaptiveFlagSnapshot{AdaptiveFlagBar: b, Candidate: signals[i]}
	}
	o, e, s, z := adaptiveFlagPortfolio(adaptiveTestSeries(bs), bs, rows, r)
	return AdaptiveFlagResult{Orders: o, Events: e, States: s, Terminal: z}
}
func adaptiveTestRun(t *testing.T, bs []AdaptiveFlagBar, bundle string, r dsl.AdaptiveFlagRules) AdaptiveFlagResult {
	t.Helper()
	out, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: dsl.AdaptiveFlagConfig("Invented adaptive flag", "Synthetic mechanics only", "M30", bundle, r), Series: adaptiveTestSeries(bs)})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// These invented observations reproduce the reviewed Python reference's frozen
// source fixture. They are not provider, historical, account or trade data.
func adaptiveTestBreakout() []AdaptiveFlagBar {
	bs := adaptiveTestBars(20)
	for i := range bs {
		bs[i] = adaptiveTestBar(i, 100, 101, 99, 100)
	}
	bs = append(bs,
		adaptiveTestBar(20, 100, 100, 97, 99), adaptiveTestBar(21, 100, 103, 99, 102),
		adaptiveTestBar(22, 103, 105, 102, 104), adaptiveTestBar(23, 105, 107, 104, 106),
		adaptiveTestBar(24, 107, 108, 106, 107.5), adaptiveTestBar(25, 107, 107.5, 106.5, 107),
		adaptiveTestBar(26, 107, 107.4, 106.4, 107), adaptiveTestBar(27, 107, 107.6, 106.2, 107.3),
		adaptiveTestBar(28, 107, 120, 102, 119), adaptiveTestBar(29, 119, 120, 118, 119))
	bs[27].Volume = 200
	return bs
}

func TestAdaptiveReferencePinnedSourceAndIdentity(t *testing.T) {
	r := adaptiveTestRules(t)
	bs := adaptiveTestBreakout()
	out := adaptiveTestRun(t, bs, "INITIAL", r)
	if len(out.Orders) != 1 {
		t.Fatalf("orders: %+v", out.Orders)
	}
	o := out.Orders[0]
	if o.SignalIdx != 27 || *o.FillIdx != 28 || *o.ExitIdx != 29 || *o.Exit != 119 || o.Reason != "target-gap" {
		t.Fatalf("source lifecycle: %+v", o)
	}
	s := out.Snapshots[27]
	// Full binary64 values independently evaluated by the corrected Python
	// reference named in AdaptiveFlagReferenceSHA256, before display rounding.
	for name, pair := range map[string][2]float64{
		"ATR": {*s.ATR, 0x1.086a326098ffcp+1}, "fast": {s.FastEMA, 0x1.95b376c1ec151p+6},
		"slow": {s.SlowEMA, 0x1.918c42d9a2ad5p+6}, "volume": {*s.VolumeSMA, 105},
		"trigger": {o.Trigger, 0x1.aed02a7a8d0a0p+6}, "stop": {o.Stop, 0x1.9ee26ae92d767p+6}, "target": {o.Target, 0x1.d6a28965fbfaep+6},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s got %x want %x", name, math.Float64bits(pair[0]), math.Float64bits(pair[1]))
		}
	}
	if *o.ActualFillToStopDistance != o.PlannedTriggerToStopDistance || o.EntryAtOpen || *o.BracketCreationIdx != 28 {
		t.Fatal("raw distances or delayed bracket")
	}
	if len(out.ConfigSHA256) != 64 || len(out.InputSHA256) != 64 || out.ExecutionSemantics != AdaptiveFlagExecutionSemantics || !strings.Contains(out.ArithmeticQualification, "not-qualified") {
		t.Fatal("identity/qualification")
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"pnl":`, `"qty":`, `"quantity":`, `"equity":`, `"size":`, `"risk":`, `"grossR":`, `"netR":`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("economic field", forbidden)
		}
	}
	again := adaptiveTestRun(t, bs, "INITIAL", r)
	if !reflect.DeepEqual(out, again) {
		t.Fatal("nondeterministic result")
	}
	out.EffectiveConfig.Rules.ValidBars = 999
	out.Snapshots[27].Close = 999
	out.Assumptions[0] = "mutated"
	if !reflect.DeepEqual(again, adaptiveTestRun(t, bs, "INITIAL", r)) {
		t.Fatal("result mutated later runs")
	}
	for _, bundle := range []string{"TWEAKED", "SNAPSHOT_C"} {
		rules, _ := dsl.AdaptiveFlagPreset(bundle)
		other := adaptiveTestRun(t, bs, bundle, rules)
		if other.ConfigSHA256 == again.ConfigSHA256 || len(other.Orders) != 1 || other.Orders[0].Status != "open" {
			t.Fatalf("bundle identity or frozen source %s: %+v", bundle, other.Orders)
		}
	}
	changed := append([]AdaptiveFlagBar{}, bs...)
	changed[0].Volume = 101
	if adaptiveTestRun(t, changed, "INITIAL", r).InputSHA256 == again.InputSHA256 {
		t.Fatal("input hash ignored volume")
	}
}

func TestAdaptiveReferenceSourceShortAndAsymmetricWindow(t *testing.T) {
	r := adaptiveTestRules(t)
	bs := adaptiveTestReflect(adaptiveTestBreakout())
	out := adaptiveTestRun(t, bs, "INITIAL", r)
	if len(out.Orders) != 0 || out.Snapshots[27].FlagBars != 7 || out.Snapshots[27].FlagStartIdx != 21 {
		t.Fatal("both sides must use last HIGH age; source is not mirror invariant")
	}
	r.MaxFlagBars = 3
	out = adaptiveTestRun(t, bs, "CUSTOM", r)
	if len(out.Orders) != 1 {
		t.Fatalf("short source %+v", out.Orders)
	}
	o := out.Orders[0]
	if o.Side != "short" || o.SignalIdx != 27 || *o.FillIdx != 28 || *o.ExitIdx != 29 || *o.Exit != 81 || o.Reason != "target-gap" {
		t.Fatalf("short source %+v", o)
	}
}

func TestAdaptiveReferenceSourceCorrectedStopAndCreationVolume(t *testing.T) {
	bs := adaptiveTestBreakout()
	bs[28] = adaptiveTestBar(28, 107, 109, 100, 101)
	bs[28].Volume = 0
	bs[29] = adaptiveTestBar(29, 106, 109, 105, 108)
	out := adaptiveTestRun(t, bs, "INITIAL", adaptiveTestRules(t))
	o := out.Orders[0]
	if out.Snapshots[28].VolumeOK || *o.FillIdx != 28 || *o.ExitIdx != 29 || *o.Exit != 106 || o.Reason != "stop-activated-at-close" {
		t.Fatalf("creation-only filter and latch: %+v", o)
	}
	if out.States[28].QueuedExit == nil || out.States[28].QueuedExit.NextOpenIdx != 29 {
		t.Fatal("missing latch trace")
	}
	for i, e := range out.Events {
		if e.ID != i || e.OrderID != 0 {
			t.Fatal("event sequence")
		}
	}
}

func TestAdaptiveReferenceStopActivationMirrorsAndBoundaries(t *testing.T) {
	r := adaptiveTestRules(t)
	for _, side := range []string{"long", "short"} {
		for _, close := range []float64{98, 99, math.Nextafter(99, 100), 100} {
			for _, opening := range []float64{97, 99, 100, 112} {
				t.Run(fmt.Sprintf("%s-close-%g-open-%g", side, close, opening), func(t *testing.T) {
					bs := adaptiveTestBars(3)
					bs[1] = adaptiveTestBar(1, 100, 102, 96, close)
					bs[2] = adaptiveTestBar(2, opening, 113, 96, 100)
					if side == "short" {
						bs = adaptiveTestReflect(bs)
					}
					out := adaptiveTestPortfolio(bs, r, map[int]*AdaptiveFlagSignal{0: adaptiveTestSignal(0, side)})
					o := out.Orders[0]
					if *o.FillIdx != 1 || *o.ExitIdx != 2 {
						t.Fatalf("no entry-bar bracket: %+v", o)
					}
					if close <= 99 {
						want := opening
						if side == "short" {
							want = float64(200 - opening)
						}
						if o.Reason != "stop-activated-at-close" || *o.Exit != want {
							t.Fatalf("latch: %+v", o)
						}
					} else if o.Reason == "stop-activated-at-close" {
						t.Fatal("intrabar-only prior stop touch must not latch")
					}
				})
			}
		}
	}
}

func TestAdaptiveReferenceTargetDoesNotLatchMirrored(t *testing.T) {
	r := adaptiveTestRules(t)
	for _, side := range []string{"long", "short"} {
		for _, close := range []float64{110, 111} {
			for _, next := range []struct {
				b      AdaptiveFlagBar
				reason string
				exit   float64
			}{
				{adaptiveTestBar(2, 105, 109, 104, 108), "", 0},
				{adaptiveTestBar(2, 112, 113, 111, 112), "target-gap", 112},
				{adaptiveTestBar(2, 110, 111, 109, 110), "target", 110},
				{adaptiveTestBar(2, 105, 111, 104, 109), "target", 110},
			} {
				bs := adaptiveTestBars(3)
				bs[1] = adaptiveTestBar(1, 100, 112, 99.5, close)
				bs[2] = next.b
				if side == "short" {
					bs = adaptiveTestReflect(bs)
				}
				out := adaptiveTestPortfolio(bs, r, map[int]*AdaptiveFlagSignal{0: adaptiveTestSignal(0, side)})
				o := out.Orders[0]
				if out.States[1].QueuedExit != nil || o.Reason != next.reason {
					t.Fatalf("target policy %s/%g: %+v", side, close, o)
				}
				if next.reason == "" {
					if o.Status != "open" || out.Terminal.Status != "open" {
						t.Fatal("target recovery must stay open")
					}
				} else {
					want := next.exit
					if side == "short" {
						want = float64(200 - want)
					}
					if *o.Exit != want {
						t.Fatal("target fill")
					}
				}
			}
		}
	}
}

func TestAdaptiveReferencePendingOpportunitiesExpiryAndSameCloseRearm(t *testing.T) {
	for _, bundle := range []string{"INITIAL", "TWEAKED"} {
		r, _ := dsl.AdaptiveFlagPreset(bundle)
		end := r.ValidBars + 1
		bs := adaptiveTestBars(end + 1)
		signals := map[int]*AdaptiveFlagSignal{}
		for i := range bs {
			signals[i] = adaptiveTestSignal(i, "long")
		}
		before := adaptiveTestPortfolio(bs[:end], r, signals)
		if len(before.Orders) != 1 || before.Orders[0].FillOpportunities != r.ValidBars || before.Terminal.PendingAge != r.ValidBars {
			t.Fatal("early expiry", bundle)
		}
		expired := adaptiveTestPortfolio(bs, r, signals)
		if len(expired.Orders) != 2 || expired.Orders[0].Status != "expired" || expired.Orders[0].FillOpportunities != r.ValidBars+1 || expired.Orders[1].SignalIdx != end || expired.Terminal.PendingAge != 0 {
			t.Fatalf("same-close expiry/rearm %+v", expired.Orders)
		}
		bs[end] = adaptiveTestBar(end, 100, 102, 99.5, 101)
		filled := adaptiveTestPortfolio(bs, r, signals)
		if len(filled.Orders) != 1 || *filled.Orders[0].FillIdx != end || filled.Terminal.Status != "open" {
			t.Fatal("final opportunity must fill before expiry")
		}
	}
	// A previous close through the stop does not invalidate an unfilled entry.
	r := adaptiveTestRules(t)
	bs := adaptiveTestBars(3)
	bs[1] = adaptiveTestBar(1, 95, 100, 90, 95)
	bs[2] = adaptiveTestBar(2, 100, 102, 99.5, 101)
	out := adaptiveTestPortfolio(bs, r, map[int]*AdaptiveFlagSignal{0: adaptiveTestSignal(0, "long")})
	if len(out.Orders) != 1 || *out.Orders[0].FillIdx != 2 {
		t.Fatal("invented opposite-side invalidation")
	}
	// An exit permits a fresh signal on that same close, active next row only.
	bs = adaptiveTestBars(4)
	bs[1] = adaptiveTestBar(1, 100, 102, 99.5, 101)
	bs[2] = adaptiveTestBar(2, 112, 114, 99.5, 101)
	bs[3] = adaptiveTestBar(3, 100, 102, 99.5, 101)
	out = adaptiveTestPortfolio(bs, r, map[int]*AdaptiveFlagSignal{0: adaptiveTestSignal(0, "long"), 2: adaptiveTestSignal(2, "long")})
	if len(out.Orders) != 2 || *out.Orders[0].ExitIdx != 2 || out.Orders[1].SignalIdx != 2 || *out.Orders[1].FillIdx != 3 {
		t.Fatal("same-exit-close rearm")
	}
}

func TestAdaptiveReferenceFrozenLevelsEntryGapAndTerminalStates(t *testing.T) {
	r := adaptiveTestRules(t)
	bs := adaptiveTestBars(3)
	bs[1] = adaptiveTestBar(1, 120, 121, 100, 111)
	bs[2] = adaptiveTestBar(2, 105, 109, 104, 108)
	out := adaptiveTestPortfolio(bs, r, map[int]*AdaptiveFlagSignal{0: adaptiveTestSignal(0, "long")})
	o := out.Orders[0]
	if *o.Entry != 120 || o.Target != 110 || o.Stop != 99 || o.PlannedTriggerToStopDistance != 2 || *o.ActualFillToStopDistance != 21 || !o.EntryAtOpen || o.Status != "open" {
		t.Fatalf("frozen levels: %+v", o)
	}
	if len(adaptiveTestPortfolio(bs[:1], r, map[int]*AdaptiveFlagSignal{0: adaptiveTestSignal(0, "long")}).Orders) != 1 {
		t.Fatal("last close must be allowed to create terminal pending")
	}
	bs[1] = adaptiveTestBar(1, 100, 102, 96, 98)
	out = adaptiveTestPortfolio(bs[:2], r, map[int]*AdaptiveFlagSignal{0: adaptiveTestSignal(0, "long")})
	if out.Terminal.Status != "queued-exit" || out.Terminal.QueuedExit.NextOpenIdx != 2 || out.Orders[0].Exit != nil || out.Orders[0].Status != "queued-exit" {
		t.Fatal("terminal queued state must not liquidate")
	}
}

func TestAdaptiveReferenceMaxHoldQueueAndGapClock(t *testing.T) {
	r := adaptiveTestRules(t)
	r.MaxHold = 2
	bs := adaptiveTestBars(5)
	for i := 1; i < 4; i++ {
		bs[i] = adaptiveTestBar(i, 101, 102, 100, 101)
	}
	bs[4] = adaptiveTestBar(4, 120, 121, 119, 120)
	bs[4].OpenT = 100 * 1800000
	bs[4].CloseT = 101 * 1800000
	signals := map[int]*AdaptiveFlagSignal{0: adaptiveTestSignal(0, "long")}
	queued := adaptiveTestPortfolio(bs[:4], r, signals)
	q := queued.Terminal.QueuedExit
	if q == nil || q.CreationIdx != 3 || q.NextOpenIdx != 4 || q.CreationCloseMS != 4*1800000 || queued.Orders[0].Exit != nil {
		t.Fatal("E+H close counter / terminal queue")
	}
	out := adaptiveTestPortfolio(bs, r, signals)
	o := out.Orders[0]
	if o.Reason != "max-hold" || *o.ExitIdx != 4 || *o.Exit != 120 {
		t.Fatal("queued market must precede target gap")
	}
	if out.States[1].QueuedExit != nil || out.States[2].QueuedExit != nil {
		t.Fatal("hold count starts at zero on entry close")
	}
	// Public runner records the gap but keeps nominal close independent of it.
	full := adaptiveTestRun(t, bs, "CUSTOM", r)
	if len(full.Gaps) != 1 || full.Gaps[0].MissingSlots != 96 || full.Snapshots[3].CloseT != 4*1800000 {
		t.Fatal("gap/nominal close contract")
	}
}

func TestAdaptiveReferenceOHLCPathAndTouchBoundaries(t *testing.T) {
	long := AdaptiveFlagSignal{Side: "long", Stop: 95, Target: 105}
	short := AdaptiveFlagSignal{Side: "short", Stop: 105, Target: 95}
	for _, c := range []struct {
		s      AdaptiveFlagSignal
		b      AdaptiveFlagBar
		reason string
		price  float64
		atOpen bool
	}{
		{long, adaptiveTestBar(1, 100, 107, 93, 101), "target", 105, false},
		{short, adaptiveTestBar(1, 100, 107, 93, 101), "stop", 105, false},
		{long, adaptiveTestBar(1, 99, 107, 94, 101), "stop", 95, false},
		{short, adaptiveTestBar(1, 99, 107, 94, 101), "target", 95, false},
		{long, adaptiveTestBar(1, 93, 100, 90, 98), "stop-gap", 93, true},
		{long, adaptiveTestBar(1, 108, 110, 106, 109), "target-gap", 108, true},
		{short, adaptiveTestBar(1, 108, 110, 106, 109), "stop-gap", 108, true},
		{short, adaptiveTestBar(1, 93, 94, 90, 92), "target-gap", 93, true},
		{long, adaptiveTestBar(1, 95, 95, 95, 95), "stop", 95, true},
		{long, adaptiveTestBar(1, 105, 105, 105, 105), "target", 105, true},
	} {
		reason, price, atOpen := adaptiveFlagBracket(c.s, c.b)
		if reason != c.reason || price != c.price || atOpen != c.atOpen {
			t.Fatalf("path got %s/%g/%v want %+v", reason, price, atOpen, c)
		}
	}
	for _, side := range []string{"long", "short"} {
		sig := adaptiveTestSignal(0, side)
		bs := adaptiveTestBar(1, 101, 101, 101, 101)
		if side == "short" {
			bs = adaptiveTestReflect([]AdaptiveFlagBar{bs})[0]
		}
		price, atOpen, filled := adaptiveFlagEntry(*sig, bs)
		if !filled || !atOpen || price != sig.Trigger {
			t.Fatal("entry equality is open touch")
		}
	}
}

func TestAdaptiveReferenceSourceExpiryAndPrefixCausality(t *testing.T) {
	r := adaptiveTestRules(t)
	bs := adaptiveTestBreakout()[:28]
	for i := 28; i <= 41; i++ {
		bs = append(bs, adaptiveTestBar(i, 107, 107.4, 106.4, 107))
	}
	out := adaptiveTestRun(t, bs, "INITIAL", r)
	if len(out.Orders) != 2 || out.Orders[0].SignalIdx != 27 || out.Orders[0].Status != "expired" || out.Orders[1].SignalIdx != 40 || out.Terminal.PendingAge != 1 {
		t.Fatalf("pinned Python expiry %+v", out.Orders)
	}
	for _, n := range []int{10, 20, 27, 28, 40} {
		prefix := adaptiveTestRun(t, bs[:n], "INITIAL", r)
		if !reflect.DeepEqual(prefix.Snapshots, out.Snapshots[:n]) || !reflect.DeepEqual(prefix.States, out.States[:n]) {
			t.Fatalf("future leak at prefix %d", n)
		}
	}
}

func TestAdaptiveEvaluationWindowPreservesWarmupAndSuppressesBroker(t *testing.T) {
	bs := adaptiveTestBreakout()
	rules := adaptiveTestRules(t)
	cfg := dsl.AdaptiveFlagConfig("Window fixture", "", "M30", "INITIAL", rules)
	full, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: adaptiveTestSeries(bs)})
	if err != nil {
		t.Fatal(err)
	}
	for _, start := range []int{27, 28, 40} {
		window := &AdaptiveFlagExecutionWindow{TradeFromMS: int64(start) * 1800000, TradeToMS: 50 * 1800000}
		out, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: adaptiveTestSeries(bs), Window: window})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(full.Snapshots, out.Snapshots) {
			t.Fatal("trade start reset or altered warmup")
		}
		if out.PreTradeRows != min(start, len(bs)) || out.EligibleTradeRows != len(bs)-out.PreTradeRows {
			t.Fatal("window counts", out.PreTradeRows, out.EligibleTradeRows)
		}
		if out.FirstRetainedOpenMS != 0 || out.LastRetainedOpenMS != 29*1800000 || out.LastRetainedCloseMS != 30*1800000 {
			t.Fatal("retained clock metadata")
		}
		for i, state := range out.States {
			if i < start && (state.Status != "flat" || state.PendingOrderID != nil || state.PositionOrderID != nil) {
				t.Fatal("warmup carried broker state", i, state)
			}
		}
		if start == 27 {
			if len(out.Orders) != 1 || out.Orders[0].SignalIdx != 27 || *out.Orders[0].FillIdx != 28 {
				t.Fatal("first eligible close failed to use pre-start pivot")
			}
		} else {
			for _, o := range out.Orders {
				if o.SignalIdx < start {
					t.Fatal("pre-start order carried")
				}
			}
		}
		window.TradeFromMS = 0
		if out.ExecutionWindow.TradeFromMS != int64(start)*1800000 {
			t.Fatal("caller mutated result window")
		}
	}
}

func TestAdaptiveEvaluationEndTruncatesBeforeFillAndPreservesTerminal(t *testing.T) {
	bs := adaptiveTestBreakout()
	cfg := dsl.AdaptiveFlagConfig("Window fixture", "", "M30", "INITIAL", adaptiveTestRules(t))
	for _, end := range []int{28, 29, 30} {
		out, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: adaptiveTestSeries(bs), Window: &AdaptiveFlagExecutionWindow{TradeFromMS: 27 * 1800000, TradeToMS: int64(end) * 1800000}})
		if err != nil {
			t.Fatal(err)
		}
		if out.UsedSourceRows != end || out.ProvidedSourceRows != 30 || out.IgnoredSuffixRows != 30-end || len(out.Snapshots) != end || len(out.States) != end {
			t.Fatal("prefix metadata or truncation")
		}
		if out.LastRetainedCloseMS != int64(end)*1800000 {
			t.Fatal("nominal end close")
		}
		want := map[int]string{28: "pending", 29: "open", 30: "flat"}[end]
		if out.Terminal.Status != want {
			t.Fatalf("end%d terminal%s want%s", end, out.Terminal.Status, want)
		}
		if end == 28 && (len(out.Orders) != 1 || out.Orders[0].FillIdx != nil) {
			t.Fatal("fill at exact end escaped")
		}
		if end == 29 && out.Orders[0].ExitIdx != nil {
			t.Fatal("exit at exact end escaped")
		}
		for _, o := range out.Orders {
			if o.FillIdx != nil && *o.FillIdx >= end || o.ExitIdx != nil && *o.ExitIdx >= end {
				t.Fatal("post-end execution")
			}
		}
	}
}

func TestAdaptiveEvaluationWindowGapsFutureInvarianceAndValidation(t *testing.T) {
	bs := adaptiveTestBreakout()
	cfg := dsl.AdaptiveFlagConfig("Window fixture", "", "M30", "INITIAL", adaptiveTestRules(t))
	series := adaptiveTestSeries(bs)
	window := &AdaptiveFlagExecutionWindow{TradeFromMS: 27 * 1800000, TradeToMS: 28 * 1800000}
	want, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: series, Window: window})
	if err != nil {
		t.Fatal(err)
	}
	future := adaptiveTestSeries(bs)
	future.O[28] = math.NaN()
	future.H[29] = math.Inf(1)
	future.T[29] = math.NaN()
	got, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: future, Window: window})
	if err != nil {
		t.Fatal("ignored suffix was processed", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("future suffix changed prefix execution/identity")
	}
	for i := 27; i < len(bs); i++ {
		bs[i].OpenT += 5 * 1800000
		bs[i].CloseT += 5 * 1800000
	}
	gap, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: adaptiveTestSeries(bs), Window: &AdaptiveFlagExecutionWindow{TradeFromMS: 28 * 1800000, TradeToMS: 40 * 1800000}})
	if err != nil {
		t.Fatal(err)
	}
	if gap.PreTradeRows != 27 || gap.Orders[0].SignalIdx != 27 || gap.Events[0].Time.LowerMS != 33*1800000 {
		t.Fatal("first observed eligible row across gap", gap.PreTradeRows, gap.Orders)
	}
	for _, w := range []AdaptiveFlagExecutionWindow{{0, 0}, {1, 1800000}, {0, 1800001}, {1800000, 0}, {-1800000, 1800000}, {0, 9007199254740992}} {
		if _, e := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: series, Window: &w}); e == nil {
			t.Fatal("invalid window accepted", w)
		}
	}
	malformed := adaptiveTestSeries(adaptiveTestBreakout())
	malformed.T[10] = math.NaN()
	if _, e := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: malformed, Window: window}); e == nil {
		t.Fatal("invalid timestamp inside prefix hidden")
	}
	noPrefix := adaptiveTestSeries(adaptiveTestBreakout())
	for i := range noPrefix.T {
		noPrefix.T[i] += 100 * 1800000
	}
	if _, e := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: noPrefix, Window: window}); e == nil {
		t.Fatal("empty retained prefix must fail")
	}
}

func TestAdaptiveEvaluationWindowH1AndFullInputControl(t *testing.T) {
	bs := adaptiveTestBreakout()
	for i := range bs {
		bs[i].OpenT *= 2
		bs[i].CloseT *= 2
	}
	cfg := dsl.AdaptiveFlagConfig("H1 window fixture", "", "H1", "INITIAL", adaptiveTestRules(t))
	series := adaptiveTestSeries(bs)
	full, e := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: series})
	if e != nil {
		t.Fatal(e)
	}
	window, e := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: series, Window: &AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: 30 * 3600000}})
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(full.Snapshots, window.Snapshots) || !reflect.DeepEqual(full.Orders, window.Orders) || !reflect.DeepEqual(full.Events, window.Events) || !reflect.DeepEqual(full.States, window.States) || full.InputSHA256 != window.InputSHA256 || full.ConfigSHA256 != window.ConfigSHA256 {
		t.Fatal("full-range window changed full-input execution")
	}
	if _, e = RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: series, Window: &AdaptiveFlagExecutionWindow{TradeFromMS: 1800000, TradeToMS: 30 * 3600000}}); e == nil {
		t.Fatal("M30 boundary accepted as H1 window")
	}
}
