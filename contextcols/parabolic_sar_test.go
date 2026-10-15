package contextcols

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func sarHandSeries() marketdata.Series {
	s := marketdata.NewSeries(9)
	for i, row := range [][3]float64{{10, 8, 9}, {12, 10, 11}, {14, 12, 13}, {16, 14, 15}, {15, 9, 10}, {13, 7, 8}, {10, 6, 7}, {18, 16, 17}, {19, 17, 18}} {
		s.T[i], s.H[i], s.L[i], s.C[i], s.O[i] = float64(i)*60000, row[0], row[1], row[2], row[2]
	}
	return s
}

func TestParabolicSARHandDerivedTrendClampsReversalsAndCap(t *testing.T) {
	s := sarHandSeries()
	// See spec/parabolic-sar.md: binary-exact .25/.25/.5 arithmetic.
	want := []float64{math.NaN(), 8, 8, 10, 16, 16, 15, 6, 6}
	trends := []int8{0, 1, 1, 1, -1, -1, -1, 1, 1}
	flips := []int8{0, 0, 0, 0, -1, 0, 0, 1, 0}
	for _, mirror := range []bool{false, true} {
		if mirror {
			for i := range s.T {
				s.H[i], s.L[i], s.C[i] = 30-s.L[i], 30-s.H[i], 30-s.C[i]
			}
		}
		got := ComputeParabolicSAR(s, ParabolicSAROptions{.25, .25, .5})
		for i, expected := range want {
			trend, flip := trends[i], flips[i]
			if mirror {
				expected, trend, flip = 30-expected, -trend, -flip
			}
			if !(math.IsNaN(expected) && math.IsNaN(got.SAR[i])) && got.SAR[i] != expected || got.Trend[i] != trend || got.Flip[i] != flip {
				t.Fatalf("mirror %v row %d: SAR %v trend %d flip %d; want %v %d %d", mirror, i, got.SAR[i], got.Trend[i], got.Flip[i], expected, trend, flip)
			}
		}
	}
}

func TestParabolicSAREqualityFlatAndExtremeTie(t *testing.T) {
	s := sarHandSeries()
	// At index 3 the provisional SAR is exactly 11. A touch does not reverse.
	s.L[3], s.C[3] = 11, 12
	got := ComputeParabolicSAR(s, ParabolicSAROptions{.25, .25, .5})
	if got.Trend[3] != 1 || got.SAR[3] != 10 || got.Flip[3] != 0 {
		t.Fatal(got)
	}
	for i := range s.T {
		s.H[i], s.L[i], s.C[i] = 10, 10, 10
	}
	got = ComputeParabolicSAR(s, ParabolicSAROptions{.02, .02, .2})
	for i := 1; i < s.Len(); i++ {
		if got.SAR[i] != 10 || got.Trend[i] != -1 || got.Flip[i] != 0 {
			t.Fatal("flat/tied close", got)
		}
	}
	// EP ties do not accelerate: repeated high 12 retains AF .25.
	s = marketdata.Series{T: []float64{0, 1, 2, 3, 4}, H: []float64{10, 12, 12, 12, 12}, L: []float64{8, 10, 10, 10, 10}, C: []float64{9, 11, 11, 11, 11}}
	got = ComputeParabolicSAR(s, ParabolicSAROptions{.25, .25, .5})
	if got.SAR[3] != 9 || got.SAR[4] != 9.75 {
		t.Fatal("EP tie accelerated", got)
	}
}

func TestParabolicSARPrefixAndGapCausality(t *testing.T) {
	s := sarHandSeries()
	options := ParabolicSAROptions{.02, .02, .2}
	full := ComputeParabolicSAR(s, options)
	for n := 0; n <= s.Len(); n++ {
		prefix := marketdata.Series{T: s.T[:n], H: s.H[:n], L: s.L[:n], C: s.C[:n]}
		got := ComputeParabolicSAR(prefix, options)
		for i := 0; i < n; i++ {
			if math.Float64bits(got.SAR[i]) != math.Float64bits(full.SAR[i]) || got.Trend[i] != full.Trend[i] || got.Flip[i] != full.Flip[i] {
				t.Fatalf("prefix %d differs at %d", n, i)
			}
		}
	}
	for i := 4; i < s.Len(); i++ {
		s.T[i] += 7 * 24 * 3600000
	}
	gap := ComputeParabolicSAR(s, options)
	for i := range s.T {
		if math.Float64bits(gap.SAR[i]) != math.Float64bits(full.SAR[i]) || gap.Trend[i] != full.Trend[i] {
			t.Fatal("timestamp gap changed observed-bar recurrence")
		}
	}
}

func TestParabolicSARInvalidParametersShapeAndSuffix(t *testing.T) {
	s := sarHandSeries()
	for _, o := range []ParabolicSAROptions{{}, {0, .02, .2}, {.02, 0, .2}, {.3, .02, .2}, {.02, .3, .2}, {.02, .02, 1.1}, {math.NaN(), .02, .2}, {.02, math.Inf(1), .2}, {.02, .02, math.NaN()}, {-.02, .02, .2}} {
		got := ComputeParabolicSAR(s, o)
		for i := range got.SAR {
			if !math.IsNaN(got.SAR[i]) || got.Trend[i] != 0 || got.Flip[i] != 0 {
				t.Fatalf("invalid options %+v", o)
			}
		}
	}
	badShape := s
	badShape.H = s.H[:3]
	for _, v := range ComputeParabolicSAR(badShape, ParabolicSAROptions{.02, .02, .2}).SAR {
		if !math.IsNaN(v) {
			t.Fatal("bad shape accepted")
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), 0} {
		bad := sarHandSeries()
		bad.H[4] = value
		got := ComputeParabolicSAR(bad, ParabolicSAROptions{.25, .25, .5})
		if got.SAR[3] != 10 {
			t.Fatal("earlier valid prefix changed")
		}
		for i := 4; i < bad.Len(); i++ {
			if !math.IsNaN(got.SAR[i]) || got.Trend[i] != 0 {
				t.Fatal("invalid suffix recovered silently", got)
			}
		}
	}
	// Finite operands whose difference overflows cannot be hidden by clamps.
	big := marketdata.Series{T: []float64{0, 1}, H: []float64{-1e308, 1e308}, L: []float64{-1e308, 1e308}, C: []float64{-1e308, 1e308}}
	got := ComputeParabolicSAR(big, ParabolicSAROptions{.02, .02, .2})
	if !math.IsNaN(got.SAR[1]) || got.Trend[1] != 0 {
		t.Fatal("overflow accepted", got)
	}
}

func TestParabolicSARInitializationReversalAndPreClampOrder(t *testing.T) {
	s := sarHandSeries()
	s.L[1] = 7
	got := ComputeParabolicSAR(s, ParabolicSAROptions{.25, .25, .5})
	if got.SAR[1] != 12 || got.Trend[1] != -1 || got.Flip[1] != 0 {
		t.Fatal("initial reversal was published as flip", got)
	}
	s = sarHandSeries()
	s.L[3] = 10.5
	got = ComputeParabolicSAR(s, ParabolicSAROptions{.25, .25, .5})
	// Candidate11 penetrates10.5, though clamping first would reduce it to10.
	if got.Trend[3] != -1 || got.SAR[3] != 16 || got.Flip[3] != -1 {
		t.Fatal("clamp preceded penetration", got)
	}
}

func TestParabolicSARDecimalRawBits(t *testing.T) {
	// Independent RN-even rational derivation (the existing arithmetic proof
	// helper's add/sub/mul, not this indicator): AF=.02,.04,.06 has bits
	// 3f947ae147ae147b,3fa47ae147ae147b,3faeb851eb851eb8.
	// SAR3=RN(8+RN(.04*RN(14-8))); SAR4=RN(SAR3+RN(.06*RN(16-SAR3)));
	// SAR6=RN(16+RN(.02*RN(7-16))). Clamps/reversals give the integer rows.
	// Running this same fixed-bit fixture natively and under Go/WASM pins raw
	// decimal arithmetic, beyond the bridge's rounded trade serialization.
	want := []uint64{0, 0x4020000000000000, 0x4020000000000000, 0x40207ae147ae147b, 0x4021694467381d7e, 0x4030000000000000, 0x402fa3d70a3d70a4, 0x4018000000000000, 0x4018000000000000}
	got := ComputeParabolicSAR(sarHandSeries(), ParabolicSAROptions{.02, .02, .2})
	if !math.IsNaN(got.SAR[0]) {
		t.Fatal("first output available")
	}
	for i := 1; i < len(want); i++ {
		if bits := math.Float64bits(got.SAR[i]); bits != want[i] {
			t.Fatalf("row%d: %016x want%016x", i, bits, want[i])
		}
	}
}
