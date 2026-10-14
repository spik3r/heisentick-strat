package regimeadaptive

import (
	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
	"math"
)

// Separate binary64 operations prevent native fused multiply-add from changing
// recurrence arithmetic relative to WASM. No existing ATR or family is altered.
func rma(values []float64, n int) []float64 {
	out := missing(len(values))
	count := 0
	sum := 0.
	last := math.NaN()
	for i, v := range values {
		if !finite(v) {
			continue
		}
		if !finite(last) {
			sum = fp.Add(sum, v)
			count++
			if count == n {
				last = fp.Div(sum, float64(n))
				out[i] = last
			}
		} else {
			last = fp.Div(fp.Add(fp.Mul(last, float64(n-1)), v), float64(n))
			out[i] = last
		}
	}
	return out
}
func sma(values []float64, n int) []float64 {
	out := missing(len(values))
	for i := n - 1; i < len(values); i++ {
		sum := 0.
		valid := true
		for j := i - n + 1; j <= i; j++ {
			if !finite(values[j]) {
				valid = false
				break
			}
			sum = fp.Add(sum, values[j])
		}
		if valid {
			out[i] = fp.Div(sum, float64(n))
		}
	}
	return out
}
func rsi(close []float64, n int) []float64 {
	up, down := missing(len(close)), missing(len(close))
	for i := 1; i < len(close); i++ {
		delta := fp.Sub(close[i], close[i-1])
		up[i] = math.Max(delta, 0)
		down[i] = math.Max(-delta, 0)
	}
	u, d := rma(up, n), rma(down, n)
	out := missing(len(close))
	for i := range close {
		if !finite(u[i]) || !finite(d[i]) {
			continue
		}
		// Both-zero RSI remains explicitly unavailable pending a Pine flat-series
		// witness. Do not manufacture a neutral value or claim built-in equivalence.
		if u[i] == 0 && d[i] == 0 {
			continue
		}
		if d[i] == 0 {
			out[i] = 100
		} else {
			out[i] = fp.Sub(100, fp.Div(100, fp.Add(1, fp.Div(u[i], d[i]))))
		}
	}
	return out
}
func psar(bars []Bar, start, inc, maximum float64) []float64 {
	out := missing(len(bars))
	if len(bars) < 2 {
		return out
	}
	below := bars[1].Close > bars[0].Close
	ep, sar := bars[1].High, bars[0].Low
	if !below {
		ep, sar = bars[1].Low, bars[0].High
	}
	af := start
	for i := 1; i < len(bars); i++ {
		first := i == 1
		sar = fp.Add(sar, fp.Mul(af, fp.Sub(ep, sar)))
		if below && sar > bars[i].Low {
			first = true
			below = false
			sar = math.Max(bars[i].High, ep)
			ep = bars[i].Low
			af = start
		} else if !below && sar < bars[i].High {
			first = true
			below = true
			sar = math.Min(bars[i].Low, ep)
			ep = bars[i].High
			af = start
		}
		if !first {
			if below && bars[i].High > ep {
				ep = bars[i].High
				af = math.Min(fp.Add(af, inc), maximum)
			}
			if !below && bars[i].Low < ep {
				ep = bars[i].Low
				af = math.Min(fp.Add(af, inc), maximum)
			}
		}
		if below {
			sar = math.Min(sar, bars[i-1].Low)
			if i > 1 {
				sar = math.Min(sar, bars[i-2].Low)
			}
		} else {
			sar = math.Max(sar, bars[i-1].High)
			if i > 1 {
				sar = math.Max(sar, bars[i-2].High)
			}
		}
		out[i] = sar
	}
	return out
}
func calculate(bars []Bar) []Row {
	n := len(bars)
	close, tr := make([]float64, n), make([]float64, n)
	for i, b := range bars {
		close[i] = b.Close
		tr[i] = fp.Sub(b.High, b.Low)
		if i > 0 {
			tr[i] = math.Max(tr[i], math.Max(math.Abs(fp.Sub(b.High, bars[i-1].Close)), math.Abs(fp.Sub(b.Low, bars[i-1].Close))))
		}
	}
	atr, strength, basis, sar := rma(tr, 14), rsi(close, 14), sma(close, 20), psar(bars, .018, .018, .20)
	strengthMA := sma(strength, 14)
	rows := make([]Row, n)
	for i, b := range bars {
		chop, upper, lower := math.NaN(), math.NaN(), math.NaN()
		// Pine rolling highest/lowest includes available history on early bars.
		first := max(0, i-13)
		high, low := b.High, b.Low
		for j := first; j <= i; j++ {
			high = math.Max(high, bars[j].High)
			low = math.Min(low, bars[j].Low)
		}
		diff := fp.Sub(high, low)
		if diff == 0 {
			chop = 100
		} else if i >= 13 {
			sum := 0.
			for j := i - 13; j <= i; j++ {
				sum = fp.Add(sum, tr[j])
			}
			if sum > 0 {
				chop = fp.Div(fp.Mul(100, math.Log10(fp.Div(sum, diff))), math.Log10(14))
			}
		}
		if finite(basis[i]) {
			variance := 0.
			for j := i - 19; j <= i; j++ {
				d := fp.Sub(close[j], basis[i])
				variance = fp.Add(variance, fp.Mul(d, d))
			}
			dev := fp.Mul(2, math.Sqrt(fp.Div(variance, 20)))
			upper, lower = fp.Add(basis[i], dev), fp.Sub(basis[i], dev)
		}
		rows[i] = Row{T: b.T, CloseT: b.CloseT, TR: ptr(tr[i]), ATR: ptr(atr[i]), CHOP: ptr(chop), RSI: ptr(strength[i]), RSIMA: ptr(strengthMA[i]), PSAR: ptr(sar[i]), BBBasis: ptr(basis[i]), BBUpper: ptr(upper), BBLower: ptr(lower)}
	}
	return rows
}
