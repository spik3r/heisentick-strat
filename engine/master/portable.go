package master

import fp "github.com/spik3r/heisentick-strat/internal/float64contract"

const PortableArithmeticContract = "master-binary64-separated-v1"

// RunPortableV1 is explicit opt-in, not an architecture-dependent replacement
// of Run. It shares the existing chronology and selects checked numerical kernels.
func RunPortableV1(r Request) (out Result, err error) {
	defer func() {
		if err != nil {
			out = Result{}
		}
	}()
	defer fp.Recover(&err)
	out, err = runArithmetic(r, true)
	if err == nil {
		out.Schema = "strat-master-structural-portable-report-v1"
		out.ArithmeticContract = PortableArithmeticContract
	}
	return
}

// These numerical comparison kernels preserve the full original legacy
// expression; only the portable branch introduces operation barriers.
func directionalDifference(a, b float64, d int, portable bool) float64 {
	if portable {
		return fp.Mul(float64(d), fp.Sub(a, b))
	}
	return float64(d) * (a - b)
}
func plainDifference(a, b float64, portable bool) float64 {
	if portable {
		return fp.Sub(a, b)
	}
	return a - b
}
