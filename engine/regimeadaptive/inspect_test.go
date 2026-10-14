package regimeadaptive

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > 1e-11 {
		t.Fatalf("got %.17g want %.17g", got, want)
	}
}
func fixture(n int) Request {
	bars := make([]Bar, n)
	for i := range bars {
		c := 100 + float64(i)
		bars[i] = Bar{T: int64(i) * 60000, CloseT: int64(i+1) * 60000, Open: c, High: c + 1, Low: c - 1, Close: c}
	}
	return Request{Schema: Schema, Mode: "indicators-signals-only", Profile: SourceProfile, SourceSHA256: SourceHash, DataKind: "synthetic", FixtureID: "hand-ramp-v1", Timeframe: Timeframe{"minute", 1}, Bars: bars}
}
func TestWilderSeedAndRSIHandArithmetic(t *testing.T) {
	// ATR/RMA(3): mean(2,4,6)=4; next (2*4+8)/3=16/3.
	a := rma([]float64{2, 4, 6, 8}, 3)
	if finite(a[0]) || finite(a[1]) {
		t.Fatal("early RMA")
	}
	near(t, a[2], 4)
	near(t, a[3], 16./3)
	// RSI(3): changes +1,-1,+2 -> avgUp1 avgDown1/3 =>75.
	// Next -2 -> avgUp2/3 avgDown8/9 =>300/7.
	r := rsi([]float64{100, 101, 100, 102, 100}, 3)
	if finite(r[2]) {
		t.Fatal("RSI needs three deltas")
	}
	near(t, r[3], 75)
	near(t, r[4], 300./7)
	for _, tc := range []struct {
		close []float64
		want  float64
	}{{[]float64{1, 2, 3, 4}, 100}, {[]float64{4, 3, 2, 1}, 0}} {
		near(t, rsi(tc.close, 3)[3], tc.want)
	}
	if finite(rsi([]float64{2, 2, 2, 2}, 3)[3]) {
		t.Fatal("flat is explicitly unresolved, not neutral fabricated")
	}
}
func TestDefaultWarmupBBPopulationAndFirstTR(t *testing.T) {
	req := fixture(30)
	rows := calculate(req.Bars)
	near(t, *rows[0].TR, 2)
	if rows[12].ATR != nil {
		t.Fatal("partial ATR")
	}
	near(t, *rows[13].ATR, 2)
	if rows[13].RSI != nil || rows[26].RSIMA != nil {
		t.Fatal("early RSI/SMA")
	}
	near(t, *rows[14].RSI, 100)
	near(t, *rows[27].RSIMA, 100)
	if rows[18].BBBasis != nil {
		t.Fatal("early bands")
	}
	// Closes100..119: mean109.5, sum squared deviation665, population variance33.25.
	near(t, *rows[19].BBBasis, 109.5)
	near(t, *rows[19].BBUpper, 109.5+2*math.Sqrt(33.25))
	near(t, *rows[19].BBLower, 109.5-2*math.Sqrt(33.25))
	// TR2*14=28; high114-low99=15. First-row TR contributes at index13.
	near(t, *rows[13].CHOP, 100*math.Log10(28./15)/math.Log10(14))
}
func TestZeroRangeAndGapTR(t *testing.T) {
	req := fixture(16)
	for i := range req.Bars {
		req.Bars[i].Open = 100
		req.Bars[i].High = 100
		req.Bars[i].Low = 100
		req.Bars[i].Close = 100
	}
	rows := calculate(req.Bars)
	for _, r := range rows {
		near(t, *r.CHOP, 100)
	}
	near(t, *rows[13].ATR, 0)
	req.Bars[1].Open = 110
	req.Bars[1].High = 112
	req.Bars[1].Low = 109
	req.Bars[1].Close = 111
	near(t, *calculate(req.Bars)[1].TR, 12)
}
func TestPSARInitializationAccelerationAndReversal(t *testing.T) {
	bars := []Bar{{High: 11, Low: 9, Close: 10}, {High: 12, Low: 10, Close: 11}, {High: 14, Low: 11, Close: 13}, {High: 15, Low: 12, Close: 14}, {High: 13, Low: 8, Close: 9}, {High: 10, Low: 7, Close: 8}, {High: 10, Low: 6, Close: 7}}
	got := psar(bars, .1, .1, .2)
	if finite(got[0]) {
		t.Fatal("PSAR starts second bar")
	}
	// Previous two lows clamp9,9; then9+.2*(14-9)=10.
	// Reversal resets to max(previous EP15,current high13)=15.
	// Downside clamps prior two highs15, then15+.2*(7-15)=13.4.
	for i, v := range []float64{9, 9, 10, 15, 15, 13.4} {
		near(t, got[i+1], v)
	}
	up := append(bars[:4:4], Bar{High: 16, Low: 13, Close: 15}, Bar{High: 17, Low: 14, Close: 16}, Bar{High: 18, Low: 15, Close: 17})
	cap := psar(up, .1, .1, .2)
	near(t, cap[6], 13)
}
func TestCrossRegimeAndTimeframeBoundaries(t *testing.T) {
	if classify(ptr(58), ptr(47), ptr(99), 100, true) != 1 || classify(ptr(58.001), ptr(80), ptr(99), 100, true) != 0 {
		t.Fatal("strict CHOP/RSI precedence")
	}
	if classify(ptr(58), ptr(48), ptr(99), 100, false) != 3 || classify(ptr(58), ptr(52), ptr(101), 100, false) != 3 {
		t.Fatal("RSI equalities")
	}
	if !cross(ptr(2), ptr(1), ptr(1), ptr(1), true) || cross(ptr(1), ptr(1), ptr(0), ptr(1), true) || cross(nil, ptr(1), ptr(0), ptr(1), true) {
		t.Fatal("cross boundaries")
	}
	for _, tc := range []struct {
		tf           Timeframe
		source, flat bool
	}{{Timeframe{"second", 15}, true, false}, {Timeframe{"second", 45}, false, false}, {Timeframe{"minute", 30}, true, true}, {Timeframe{"minute", 60}, false, false}, {Timeframe{"day", 1}, false, false}} {
		if sensitive(tc.tf, SourceProfile) != tc.source || sensitive(tc.tf, FlatProfile) != tc.flat {
			t.Fatal(tc)
		}
	}
}
func TestRepeatedWicksBothBandsAndWrongSideTargetRetained(t *testing.T) {
	row := Row{Regime: 3, CHOP: ptr(50), ATR: ptr(1), RSI: ptr(50), RSIMA: ptr(50), PSAR: ptr(100), BBBasis: ptr(100), BBLower: ptr(90), BBUpper: ptr(110)}
	b := Bar{Close: 105, Low: 90, High: 110}
	previous := row
	for i := 0; i < 2; i++ {
		got := signals(row, &previous, b, &b, true, SourceProfile)
		if !got.RangeLong || !got.RangeShort || !got.BothBands {
			t.Fatal("source retains both independent branches/repeated wick")
		}
	}
	flat := signals(row, &previous, b, &b, true, FlatProfile)
	if flat.FlatEligible {
		t.Fatal("variant skips both bands")
	}
	b.High = 106
	got := signals(row, &previous, b, &b, true, SourceProfile)
	if !got.RangeLong || *row.BBBasis >= b.Close {
		t.Fatal("wick long target is below close; no source validity filter")
	}
}
func TestInitialAndScaleCanOverlapWithoutPositionAdmission(t *testing.T) {
	prev := Row{RSI: ptr(49), RSIMA: ptr(50)}
	row := Row{Regime: 1, RSI: ptr(51), RSIMA: ptr(50), PSAR: ptr(99)}
	b := Bar{Close: 100}
	got := signals(row, &prev, b, &b, true, SourceProfile)
	if !got.InitialLong || !got.ScaleLong {
		t.Fatal("both source triggers preserved")
	}
	if signals(row, &prev, b, &b, true, FlatProfile).ScaleLong {
		t.Fatal("flat has no scaling")
	}
}
func TestFuturePrefixInvarianceAndExecutionRefusal(t *testing.T) {
	req := fixture(90)
	full, err := Inspect(req)
	if err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= len(req.Bars); n++ {
		prefix := req
		prefix.Bars = req.Bars[:n]
		got, err := Inspect(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Rows, full.Rows[:n]) {
			t.Fatalf("future leak at%d", n)
		}
	}
	if full.ExecutionStatus != "unsupported-not-run" || full.Trades != nil || full.PendingOrders != nil || full.TerminalPositions != nil {
		t.Fatal("must not imply empty execution proof")
	}
	for _, change := range []func(*Request){func(r *Request) { r.Mode = "execute" }, func(r *Request) { r.DataKind = "historical" }, func(r *Request) { r.SourceSHA256 = FlatHash }, func(r *Request) { r.Timeframe.Unit = "hour" }, func(r *Request) { r.Bars[0].Low = 0 }, func(r *Request) { r.Bars[0].CloseT++ }} {
		r := fixture(30)
		change(&r)
		if _, err := Inspect(r); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}
func TestClosedJSONRejectsUnknownDuplicateMissingNullAndCoercion(t *testing.T) {
	raw, _ := json.Marshal(fixture(1))
	text := string(raw)
	for _, bad := range []string{strings.Replace(text, `"mode":`, `"mode":"execute","mode":`, 1), strings.Replace(text, `"mode":`, `"properties":{},"mode":`, 1), strings.Replace(text, `"open":100`, `"open":null`, 1), strings.Replace(text, `"open":100`, `"open":true`, 1), strings.Replace(text, `"t":0,`, ``, 1), text + `{}`, strings.Replace(text, `"t":0`, `"t":0.5`, 1)} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Fatalf("accepted%s", bad)
		}
	}
	if _, err := Decode(raw); err != nil {
		t.Fatal(err)
	}
}

func TestTimestampOverflowAndUnsafeValuesRefused(t *testing.T) {
	for _, pair := range [][2]int64{{9223372036854775800, -9223372036854715816}, {9007199254740992, 9007199254800992}, {0, -1}, {60000, 0}} {
		req := fixture(1)
		req.Bars[0].T = pair[0]
		req.Bars[0].CloseT = pair[1]
		if _, err := Inspect(req); err == nil {
			t.Fatal("unsafe timestamp pair accepted", pair)
		}
	}
}

func TestDefaultPSARAndEqualCloseDescendingInitialization(t *testing.T) {
	bars := []Bar{{High: 11, Low: 9, Close: 10}, {High: 12, Low: 10, Close: 11}, {High: 14, Low: 11, Close: 13}, {High: 15, Low: 12, Close: 14}, {High: 13, Low: 8, Close: 9}}
	values := psar(bars, .018, .018, .20)
	// Initial clamp9 twice, EP14 and acceleration.036:9+.036*(14-9)=9.18.
	// Next projects9.18+.054*(15-9.18)=9.49428, then reverses above low8 to EP15.
	near(t, values[1], 9)
	near(t, values[2], 9)
	near(t, values[3], 9.18)
	near(t, values[4], 15)
	for _, c := range []float64{9, 10} { // strict close>previous; equality initializes descending.
		descending := []Bar{{High: 11, Low: 9, Close: 10}, {High: 10, Low: 8, Close: c}, {High: 9, Low: 7, Close: 8}}
		got := psar(descending, .018, .018, .20)
		near(t, got[1], 11)
		near(t, got[2], 11)
	}
}
func TestMultiRegimeFixturePrefixInvariance(t *testing.T) {
	raw, err := os.ReadFile("testdata/ramp-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	req, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	full, err := Inspect(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{SourceProfile, FlatProfile} {
		req.Profile = profile
		req.SourceSHA256 = SourceHash
		if profile == FlatProfile {
			req.SourceSHA256 = FlatHash
		}
		full, err = Inspect(req)
		if err != nil {
			t.Fatal(err)
		}
		for n := 1; n <= len(req.Bars); n++ {
			prefix := req
			prefix.Bars = req.Bars[:n]
			got, err := Inspect(prefix)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Rows, full.Rows[:n]) {
				t.Fatalf("future leak profile%s prefix%d", profile, n)
			}
		}
	}
}
