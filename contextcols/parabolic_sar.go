package contextcols

import (
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// ParabolicSAROptions are acceleration factors, not percentages. Zero values
// are invalid; callers wanting defaults use .02, .02, .2 explicitly.
type ParabolicSAROptions struct {
	Start float64
	Step  float64
	Max   float64
}

func (o ParabolicSAROptions) valid() bool {
	return isFinite(o.Start) && isFinite(o.Step) && isFinite(o.Max) &&
		o.Start > 0 && o.Step > 0 && o.Start <= o.Max && o.Step <= o.Max && o.Max <= 1
}

// ParabolicSARColumns contains aligned causal outputs. SAR is NaN and Trend
// is zero while unavailable. Trend is +1 for rising and -1 for falling; Flip
// has the new trend's sign only when two published trends differ.
type ParabolicSARColumns struct {
	SAR   []float64
	Trend []int8
	Flip  []int8
}

// ComputeParabolicSAR implements the close-seeded, strict-penetration variant
// specified in spec/parabolic-sar.md. It uses the same recurrence convention as
// the private regimeadaptive PSAR without altering that historical kernel.
//
// Bar zero is unavailable. Invalid options/shapes produce unavailable outputs;
// an invalid H/L/C row or nonfinite intermediate makes that row and the entire
// remaining suffix unavailable. Earlier outputs are retained. Timestamp gaps
// are not filled and do not reset state: each supplied bar advances once.
func ComputeParabolicSAR(series marketdata.Series, o ParabolicSAROptions) ParabolicSARColumns {
	n := series.Len()
	out := ParabolicSARColumns{SAR: nanSlice(n), Trend: make([]int8, n), Flip: make([]int8, n)}
	if !o.valid() || len(series.H) != n || len(series.L) != n || len(series.C) != n || n < 2 {
		return out
	}
	validBar := func(i int) bool {
		return isFinite(series.H[i]) && isFinite(series.L[i]) && isFinite(series.C[i]) &&
			series.L[i] <= series.C[i] && series.C[i] <= series.H[i]
	}
	if !validBar(0) || !validBar(1) {
		return out
	}
	trend := int8(-1)
	sar, ep := series.H[0], series.L[1]
	if series.C[1] > series.C[0] {
		trend, sar, ep = 1, series.L[0], series.H[1]
	}
	af := o.Start
	for i := 1; i < n; i++ {
		if !validBar(i) {
			return out
		}
		// Explicit conversions are binary64 rounding barriers: no fused
		// multiply-add is permitted on any native or WASM target.
		delta := float64(ep - sar)
		advance := float64(af * delta)
		candidate := float64(sar + advance)
		if !isFinite(delta) || !isFinite(advance) || !isFinite(candidate) {
			return out
		}
		sar = candidate
		first := i == 1
		if trend == 1 && sar > series.L[i] {
			trend, sar, ep, af = -1, math.Max(series.H[i], ep), series.L[i], o.Start
			first = true
		} else if trend == -1 && sar < series.H[i] {
			trend, sar, ep, af = 1, math.Min(series.L[i], ep), series.H[i], o.Start
			first = true
		}
		if !first && (trend == 1 && series.H[i] > ep || trend == -1 && series.L[i] < ep) {
			if trend == 1 {
				ep = series.H[i]
			} else {
				ep = series.L[i]
			}
			af = math.Min(float64(af+o.Step), o.Max)
		}
		if trend == 1 {
			sar = math.Min(sar, series.L[i-1])
			if i > 1 {
				sar = math.Min(sar, series.L[i-2])
			}
		} else {
			sar = math.Max(sar, series.H[i-1])
			if i > 1 {
				sar = math.Max(sar, series.H[i-2])
			}
		}
		out.SAR[i], out.Trend[i] = sar, trend
		if i > 1 && out.Trend[i-1] != trend {
			out.Flip[i] = trend
		}
	}
	return out
}
