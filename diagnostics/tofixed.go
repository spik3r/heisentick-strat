package diagnostics

import (
	"math"
	"math/big"
	"strings"
)

// jsToFixed formats x as Number.prototype.toFixed(digits) does for the
// values that reach a warning message: it rounds the exact binary value of
// x and, on an exact tie, picks the larger magnitude (0.125 -> "0.13"),
// where Go's strconv would round to even. The sign is handled separately,
// so -0 formats as "0.00" and -0.001 as "-0.00", as in JS. Values at or
// above 1e21 are not expected here and fall back to Go's shortest form.
func jsToFixed(x float64, digits int) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	case math.Abs(x) >= 1e21:
		return big.NewFloat(x).Text('g', -1)
	}
	negative := x < 0
	// 2100 bits hold any float64 scaled by 10^digits exactly for small digits.
	const prec = 2100
	scaled := new(big.Float).SetPrec(prec).SetFloat64(math.Abs(x))
	pow := new(big.Float).SetPrec(prec).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil))
	scaled.Mul(scaled, pow)
	whole, _ := scaled.Int(nil)
	frac := new(big.Float).SetPrec(prec).Sub(scaled, new(big.Float).SetPrec(prec).SetInt(whole))
	if frac.Cmp(big.NewFloat(0.5)) >= 0 {
		whole.Add(whole, big.NewInt(1))
	}
	text := whole.String()
	if digits > 0 {
		if len(text) <= digits {
			text = strings.Repeat("0", digits-len(text)+1) + text
		}
		text = text[:len(text)-digits] + "." + text[len(text)-digits:]
	}
	if negative {
		text = "-" + text
	}
	return text
}
