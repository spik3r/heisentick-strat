package engine

import (
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestAdaptiveInputValidationEveryColumn(t *testing.T) {
	r := adaptiveTestRules(t)
	cfg := dsl.AdaptiveFlagConfig("Invented", "Synthetic only", "M30", "INITIAL", r)
	good := func() marketdata.Series { return adaptiveTestSeries(adaptiveTestBars(2)) }
	for name, mutate := range map[string]func(*marketdata.Series){
		"empty":    func(s *marketdata.Series) { *s = marketdata.Series{} },
		"T length": func(s *marketdata.Series) { s.T = s.T[:1] }, "O length": func(s *marketdata.Series) { s.O = nil },
		"H length": func(s *marketdata.Series) { s.H = nil }, "L length": func(s *marketdata.Series) { s.L = nil },
		"C length": func(s *marketdata.Series) { s.C = nil }, "V length": func(s *marketdata.Series) { s.V = nil },
		"negative time": func(s *marketdata.Series) { s.T[0] = -1800000 }, "fraction": func(s *marketdata.Series) { s.T[0] = .5 },
		"offgrid": func(s *marketdata.Series) { s.T[0] = 1 }, "duplicate": func(s *marketdata.Series) { s.T[1] = s.T[0] },
		"reversed":     func(s *marketdata.Series) { s.T[0], s.T[1] = s.T[1], s.T[0] },
		"unsafe time":  func(s *marketdata.Series) { s.T[1] = 9007199254740992 },
		"unsafe close": func(s *marketdata.Series) { s.T[1] = 9007199253000000 },
		"zero price":   func(s *marketdata.Series) { s.L[0] = 0 }, "negative volume": func(s *marketdata.Series) { s.V[0] = -1 },
		"inverted range": func(s *marketdata.Series) { s.H[0] = 99 }, "open outside": func(s *marketdata.Series) { s.O[0] = 101 },
		"close outside": func(s *marketdata.Series) { s.C[0] = 99 },
	} {
		t.Run(name, func(t *testing.T) {
			s := good()
			mutate(&s)
			if _, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: s}); err == nil {
				t.Fatal("invalid series admitted")
			}
		})
	}
	for col := 0; col < 6; col++ {
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			s := good()
			columns := [][]float64{s.T, s.O, s.H, s.L, s.C, s.V}
			columns[col][0] = value
			if _, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: s}); err == nil {
				t.Fatalf("nonfinite column %d admitted", col)
			}
		}
	}
	s := good()
	s.V[0] = 0
	if _, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: s}); err != nil {
		t.Fatal("zero volume should be legal", err)
	}
	cfg = dsl.AdaptiveFlagConfig("Invented", "Synthetic only", "H1", "INITIAL", r)
	if _, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: s}); err == nil {
		t.Fatal("M30 odd boundary admitted as H1")
	}
	s.T[1] = 7200000
	out, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: cfg, Series: s})
	if err != nil {
		t.Fatal(err)
	}
	if out.TimeframeMS != 3600000 || len(out.Gaps) != 1 || out.Gaps[0].MissingSlots != 1 || out.Snapshots[0].CloseT != 3600000 {
		t.Fatal("H1 gap grid")
	}
}

func TestAdaptiveIndicatorsWarmupInitialTRAndCurrentVolume(t *testing.T) {
	bs := adaptiveTestBars(4)
	bs[0] = adaptiveTestBar(0, 10, 12, 8, 10)
	bs[0].Volume = 10
	bs[1] = adaptiveTestBar(1, 15, 16, 14, 15)
	bs[1].Volume = 20
	bs[2] = adaptiveTestBar(2, 15, 17, 14, 16)
	bs[2].Volume = 60
	bs[3] = adaptiveTestBar(3, 16, 18, 15, 17)
	bs[3].Volume = 90
	tr, atr := adaptiveFlagATR(bs, 3)
	if !reflect.DeepEqual(tr, []float64{4, 6, 3, 3}) || atr[0] != nil || atr[1] != nil || *atr[2] != float64(13.0/3.0) {
		t.Fatal("ATR must seed from TR0 through TR[length-1]")
	}
	want := float64(float64(float64(float64(13.0/3.0)*2)+3) / 3)
	if *atr[3] != want {
		t.Fatal("Wilder operation order")
	}
	vol := adaptiveFlagVolumeSMA(bs, 3)
	if vol[0] != nil || vol[1] != nil || *vol[2] != 30 || *vol[3] != float64(170.0/3.0) {
		t.Fatal("volume current-inclusive full SMA")
	}
	if got := adaptiveFlagEMA(bs, 1); !reflect.DeepEqual(got, []float64{10, 15, 16, 17}) {
		t.Fatal("EMA first close/length one")
	}
	_, short := adaptiveFlagATR(bs, 5)
	for _, v := range short {
		if v != nil {
			t.Fatal("ATR premature warmup")
		}
	}
}

func TestAdaptiveStrictConfirmedPivotBothSides(t *testing.T) {
	bs := adaptiveTestBars(7)
	for i, h := range []float64{11, 12, 13, 20, 13, 12, 11} {
		bs[i] = adaptiveTestBar(i, 5, h, 1, 5)
	}
	for _, high := range []bool{true, false} {
		candidate := bs
		if !high {
			candidate = adaptiveTestReflect(bs)
		}
		if adaptiveFlagConfirmedPivot(candidate, 5, 3, high) != nil {
			t.Fatal("pivot used before right-side confirmation")
		}
		p := adaptiveFlagConfirmedPivot(candidate, 6, 3, high)
		if p == nil || p.Index != 3 || p.ConfirmationIdx != 6 {
			t.Fatal("pivot occurrence/confirmation")
		}
		tied := append([]AdaptiveFlagBar{}, candidate...)
		if high {
			tied[2].High = tied[3].High
		} else {
			tied[2].Low = tied[3].Low
		}
		if adaptiveFlagConfirmedPivot(tied, 6, 3, high) != nil {
			t.Fatal("left plateau must be rejected")
		}
		tied = append([]AdaptiveFlagBar{}, candidate...)
		if high {
			tied[5].High = tied[3].High
		} else {
			tied[5].Low = tied[3].Low
		}
		if adaptiveFlagConfirmedPivot(tied, 6, 3, high) != nil {
			t.Fatal("right plateau must be rejected")
		}
	}
}

func TestAdaptiveMovingPoleEndpointsAndHighAnchoredClampedWindow(t *testing.T) {
	r := adaptiveTestRules(t)
	r.PivotSensitivity = 1
	r.ATRLen = 1
	r.MinFlagBars = 1
	r.MaxFlagBars = 8
	r.UseVolumeFilter = false
	r.UseEMATrend = false
	r.MinPoleATR = .1
	highs := []float64{101, 103, 102, 104, 110, 108, 109, 107}
	lows := []float64{99, 100, 98, 102, 105, 106, 106, 106}
	bs := adaptiveTestBars(len(highs))
	for i := range bs {
		bs[i] = adaptiveTestBar(i, lows[i], highs[i], lows[i], highs[i])
	}
	rows := adaptiveFlagSnapshots(bs, r)
	if rows[5].LastLow.Index != 2 || rows[5].LastHigh.Index != 4 || rows[5].BullHeight != 12 || rows[6].BullHeight != 10 || rows[6].PoleEndpointIdx != 5 {
		t.Fatalf("moving source pole: %+v %+v", rows[5], rows[6])
	}
	if rows[5].FlagBars != 1 || rows[5].FlagStartIdx != 5 || rows[6].FlagBars != 2 || rows[6].FlagStartIdx != 5 || rows[6].FlagHigh != 109 || rows[6].FlagLow != 106 {
		t.Fatal("window must include current row and exclude the high pivot at row4")
	}
	// The initial clamp is a window length, not a required count of elapsed bars.
	r.MinFlagBars = 3
	r.MaxFlagBars = 3
	rows = adaptiveFlagSnapshots(bs, r)
	if rows[0].FlagBars != 3 || rows[0].FlagStartIdx != 0 || rows[6].FlagBars != 3 || rows[6].FlagStartIdx != 4 {
		t.Fatal("source clamping/available prefix")
	}
}

func TestAdaptiveNegativeRetraceAndEMAOrGates(t *testing.T) {
	r := adaptiveTestRules(t)
	r.MinFlagBars = 1
	r.MaxFlagBars = 1
	bs := adaptiveTestBreakout()[:29]
	bs[28] = adaptiveTestBar(28, 109.5, 110, 109, 109.5)
	bs[28].Volume = 200
	s := adaptiveFlagSnapshots(bs, r)[28]
	if s.BullRetrace == nil || *s.BullRetrace != -1 || !s.BullValid || s.Candidate == nil {
		t.Fatalf("negative retrace is a literal source comparison: %+v", s)
	}
	r.FastEMALen = 2
	r.SlowEMALen = 10
	for _, mirror := range []bool{false, true} {
		bs = adaptiveTestBars(6)
		for i, c := range []float64{10, 10, 10, 2, 3, 4} {
			bs[i] = adaptiveTestBar(i, c, float64(c+.5), float64(c-.5), c)
		}
		if mirror {
			bs = adaptiveTestReflect(bs)
		}
		s = adaptiveFlagSnapshots(bs, r)[5]
		if !mirror && !(s.FastEMA < s.SlowEMA && s.Close > s.FastEMA && s.TrendBull) {
			t.Fatal("bull OR gate")
		}
		if mirror && !(s.FastEMA > s.SlowEMA && s.Close < s.FastEMA && s.TrendBear) {
			t.Fatal("bear OR gate")
		}
	}
}

func TestAdaptiveSourceThresholdsAndLongPriority(t *testing.T) {
	r := adaptiveTestRules(t)
	bs := adaptiveTestBreakout()
	r.VolumeSMALen = 1
	r.VolumeSMAMult = 1
	rows := adaptiveFlagSnapshots(bs, r)
	if rows[27].VolumeOK || rows[27].Candidate != nil {
		t.Fatal("volume equality must fail strict >")
	}
	r.VolumeSMAMult = math.Nextafter(1, 0)
	rows = adaptiveFlagSnapshots(bs, r)
	if !rows[27].VolumeOK || rows[27].Candidate == nil {
		t.Fatal("volume below-equality boundary")
	}
	s := AdaptiveFlagSnapshot{AdaptiveFlagBar: adaptiveTestBar(3, 100, 102, 98, 100), ATR: adaptiveFlagPointer(2.), FlagHigh: 102, FlagLow: 98, BullValid: true, BearValid: true}
	if got := adaptiveFlagCandidate(s, r); got.Side != "long" {
		t.Fatal("source long branch must take precedence")
	}
	// Pole recency includes exactly maxFlagBars+sensitivity+6 observed rows.
	limit := r.MaxFlagBars + r.PivotSensitivity + 6
	if !adaptiveFlagPoleRecent(limit, r) || adaptiveFlagPoleRecent(limit+1, r) {
		t.Fatal("source pole age boundary")
	}
}

func TestAdaptiveArithmeticRoundingBarriers(t *testing.T) {
	bs := adaptiveTestBreakout()
	length := 50
	out := adaptiveFlagEMA(bs, length)
	alpha := float64(2.0 / float64(float64(length)+1))
	weight := float64(1 - alpha)
	foundFusedDifference := false
	for i := 1; i < len(bs); i++ {
		left := float64(alpha * bs[i].Close)
		right := float64(weight * out[i-1])
		want := float64(left + right)
		if out[i] != want {
			t.Fatalf("EMA operation rounding at row%d", i)
		}
		if math.FMA(alpha, bs[i].Close, right) != want || math.FMA(weight, out[i-1], left) != want {
			foundFusedDifference = true
		}
	}
	if !foundFusedDifference {
		t.Fatal("fixture must exercise a fused versus unfused rounding difference")
	}
	// Mirrored prices are separately evaluated with explicit rounding barriers.
	mirror := adaptiveTestReflect(bs)
	mirrored := adaptiveFlagEMA(mirror, length)
	for i := 1; i < len(mirror); i++ {
		want := float64(float64(alpha*mirror[i].Close) + float64(weight*mirrored[i-1]))
		if mirrored[i] != want {
			t.Fatal("mirrored EMA rounding")
		}
	}
}

func TestAdaptiveDerivedOverflowFailsClosed(t *testing.T) {
	r := adaptiveTestRules(t)
	r.ATRLen = 1
	r.VolumeSMALen = 1
	bs := adaptiveTestBars(2)
	bs[0].Volume = math.MaxFloat64
	bs[1].Volume = math.MaxFloat64
	if _, err := RunAdaptiveVolumeFlag(AdaptiveFlagRequest{Config: dsl.AdaptiveFlagConfig("Invented", "overflow control", "M30", "CUSTOM", r), Series: adaptiveTestSeries(bs)}); err == nil {
		t.Fatal("intermediate volume sum overflow must not become valid finite output")
	}
}
