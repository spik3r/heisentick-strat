package engine

import (
	"math"
	"math/big"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func adaptiveTestSeedBoundaryBars() []AdaptiveFlagBar {
	bs := adaptiveTestBars(12)
	for i := range bs {
		width := float64(float64(30.1+float64((i*7)%31)) + float64(1.0/float64(3+i%7)))
		bs[i] = adaptiveTestBar(i, 100, float64(100+float64(width/2)), float64(100-float64(width/2)), 100)
	}
	return append(bs, adaptiveTestBar(12, 90, 100, 70, 90), adaptiveTestBar(13, 107, 110, 105, 107), adaptiveTestBar(14, 108, 109, 106, 108))
}

// Independent exact-bit oracle: math/big performs each source addition at
// precision 53 with nearest-even rounding, then the one final division. The
// alternative first accumulates exact rationals and rounds the total once,
// before division. For this fixture it equals CPython 3.12.14's float-only
// Neumaier sum; it is evidence of a boundary difference, never an engine mode.
func TestAdaptiveNumericalOrderedSeedVersusCompensatedBoundary(t *testing.T) {
	bs := adaptiveTestSeedBoundaryBars()
	r := adaptiveTestRules(t)
	r.PivotSensitivity = 1
	r.MinFlagBars = 1
	r.ATRLen = 15
	r.UseVolumeFilter = false
	r.UseEMATrend = false
	r.MinPoleATR = 0x1.0e2076a4eecb0p+0
	const orderedSeed = 0x1.2f43d54e65f77p+5
	const compensatedSeed = 0x1.2f43d54e65f76p+5
	tr, atr := adaptiveFlagATR(bs, r.ATRLen)
	ordered := new(big.Float).SetPrec(53).SetMode(big.ToNearestEven)
	exact := new(big.Rat)
	for _, v := range tr {
		ordered.Add(ordered, new(big.Float).SetPrec(53).SetFloat64(v))
		exact.Add(exact, new(big.Rat).SetFloat64(v))
	}
	ordered.Quo(ordered, new(big.Float).SetPrec(53).SetInt64(15))
	orderedOracle, _ := ordered.Float64()
	compensated := new(big.Float).SetPrec(53).SetMode(big.ToNearestEven).SetRat(exact)
	compensated.Quo(compensated, new(big.Float).SetPrec(53).SetInt64(15))
	compensatedOracle, _ := compensated.Float64()
	if orderedOracle != orderedSeed || compensatedOracle != compensatedSeed || *atr[14] != orderedSeed || math.Nextafter(compensatedSeed, math.Inf(1)) != orderedSeed {
		t.Fatalf("independent seed bits: ordered=%x alternative=%x actual=%x", math.Float64bits(orderedOracle), math.Float64bits(compensatedOracle), math.Float64bits(*atr[14]))
	}
	orderedThreshold := float64(r.MinPoleATR * orderedSeed)
	compensatedThreshold := float64(r.MinPoleATR * compensatedSeed)
	if compensatedThreshold != 40 || orderedThreshold != math.Nextafter(40, math.Inf(1)) {
		t.Fatal("fixture must straddle a literal one-ULP pole threshold")
	}
	out := adaptiveTestRun(t, bs, "CUSTOM", r)
	if out.NumericalPolicy != dsl.AdaptiveFlagNumericalPolicy || out.EffectiveConfig.NumericalPolicy != out.NumericalPolicy {
		t.Fatal("fixed numerical provenance missing")
	}
	s := out.Snapshots[14]
	if s.BullHeight != 40 || s.BullImpulse || s.BullValid || s.Candidate != nil || len(out.Orders) != 0 {
		t.Fatal("ordered policy must reject the threshold-adjacent setup")
	}
	// Explicit counterfactual evidence only. Warmup bars have no ATR and no
	// candidate; replacing this one seed is sufficient to expose the divergence.
	alternative := append([]AdaptiveFlagSnapshot{}, out.Snapshots...)
	s.ATR = adaptiveFlagPointer(compensatedSeed)
	s.BullImpulse = s.BullHeight >= compensatedThreshold
	s.BullValid = s.BullImpulse && s.BullRetrace != nil && *s.BullRetrace <= float64(s.BullHeight*r.MaxFlagRetrace) && s.FlagWidth <= float64(s.BullHeight*r.FlagWidthPoleMult) && s.TrendBull && s.VolumeOK
	s.Candidate = adaptiveFlagCandidate(s, r)
	alternative[14] = s
	orders, _, _, terminal := adaptiveFlagPortfolio(adaptiveTestSeries(bs), bs, alternative, r)
	if len(orders) != 1 || orders[0].SignalIdx != 14 || orders[0].Side != "long" || terminal.Status != "pending" {
		t.Fatal("compensated alternative must expose the distinct row14 pending state")
	}
}

func TestAdaptiveNumericalDirectAdjacentBoundaryComparisons(t *testing.T) {
	// This tests only the named Go numerical contract. It asserts no unknown
	// Pine-version precision, automatic decimal quantization, or Python parity.
	r := adaptiveTestRules(t)
	r.ATRLen = 1
	r.VolumeSMALen = 1
	r.VolumeSMAMult = 1
	bs := adaptiveTestBars(1)
	for _, mult := range []float64{math.Nextafter(1, 0), 1, math.Nextafter(1, math.Inf(1))} {
		r.VolumeSMAMult = mult
		s := adaptiveFlagSnapshots(bs, r)[0]
		if s.VolumeOK != (mult < 1) {
			t.Fatalf("strict volume comparison rounded adjacent multiplier %x", math.Float64bits(mult))
		}
	}
	bs = adaptiveTestSeedBoundaryBars()
	r = adaptiveTestRules(t)
	r.PivotSensitivity = 1
	r.MinFlagBars = 1
	r.ATRLen = 15
	r.UseVolumeFilter = false
	r.UseEMATrend = false
	const boundary = 0x1.0e2076a4eecb0p+0
	r.MinPoleATR = boundary
	if adaptiveFlagSnapshots(bs, r)[14].BullValid {
		t.Fatal("direct >40 threshold must reject")
	}
	r.MinPoleATR = math.Nextafter(boundary, 0)
	s := adaptiveFlagSnapshots(bs, r)[14]
	if !s.BullValid || s.Candidate == nil {
		t.Fatal("adjacent lower multiplier must admit; no epsilon or decimal rounding")
	}
}
