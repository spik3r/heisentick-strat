package regime

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func finite(x float64) bool      { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func pointer(x float64) *float64 { v := x; return &v }
func nullable(x float64) *float64 {
	if !finite(x) {
		return nil
	}
	return pointer(x)
}
func nanSlice(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	return out
}

func wma(values []float64, n int) []float64 {
	out := nanSlice(len(values))
	denominator := float64(n*(n+1)) / 2
	for i := n - 1; i < len(values); i++ {
		sum := 0.
		for j := 0; j < n; j++ {
			sum += values[i-n+1+j] * (float64(j+1) / denominator)
		}
		out[i] = sum
	}
	return out
}
func hma(values []float64, n int) []float64 {
	half := wma(values, max(1, n/2))
	whole := wma(values, n)
	for i := range half {
		half[i] = 2*half[i] - whole[i]
	}
	return wma(half, max(1, int(math.Floor(math.Sqrt(float64(n))+.5))))
}
func rma(values []float64, n int) []float64 {
	out := nanSlice(len(values))
	count := 0
	sum := 0.
	last := math.NaN()
	for i, v := range values {
		if !finite(v) {
			continue
		}
		if !finite(last) {
			sum += v
			count++
			if count == n {
				last = sum / float64(n)
				out[i] = last
			}
		} else {
			last = (last*float64(n-1) + v) / float64(n)
			out[i] = last
		}
	}
	return out
}

func aggregate(s marketdata.Series, warmup, until int64) ([]NativeBar, int, error) {
	n := s.Len()
	if n == 0 || len(s.O) != n || len(s.H) != n || len(s.L) != n || len(s.C) != n || len(s.V) != n {
		return nil, 0, fmt.Errorf("regime requires nonempty equal-length OHLCV including volume")
	}
	bars := []NativeBar{}
	used := 0
	previous := -1.
	for i, t := range s.T {
		if !finite(t) || t < 0 || t > 9007199254740991 || math.Trunc(t) != t || int64(t)%M5MS != 0 || t <= previous {
			return nil, 0, fmt.Errorf("invalid or unordered M5 timestamp at row %d", i)
		}
		previous = t
		if int64(t) < warmup || int64(t) >= until {
			continue
		}
		o, h, l, c, v := s.O[i], s.H[i], s.L[i], s.C[i], s.V[i]
		if !finite(o) || !finite(h) || !finite(l) || !finite(c) || !finite(v) || l <= 0 || o < l || o > h || c < l || c > h || v < 0 {
			return nil, 0, fmt.Errorf("invalid M5 OHLCV at row %d", i)
		}
		bt := int64(t) / M30MS * M30MS
		if len(bars) == 0 || bars[len(bars)-1].BucketT != bt {
			bars = append(bars, NativeBar{BucketT: bt, FirstObservedT: int64(t), CloseT: bt + M30MS, Open: o, High: h, Low: l})
		}
		b := &bars[len(bars)-1]
		b.High = math.Max(b.High, h)
		b.Low = math.Min(b.Low, l)
		b.Close = c
		b.Volume += v
		b.Count++
		if !finite(b.Volume) {
			return nil, 0, fmt.Errorf("native volume overflow")
		}
		// Strict unique grid timestamps imply count6 contains exactly all slots.
		b.Complete = b.Count == 6
		used++
	}
	if used == 0 || bars[0].FirstObservedT != warmup {
		return nil, 0, fmt.Errorf("regime requires observed first M5 open at warmup boundary")
	}
	// End is an explicit complete-history watermark, not inferred from a
	// bucket's label. Requiring the last M5 close prevents an unclosed tail.
	last := -1
	for i, t := range s.T {
		if int64(t) < until && int64(t) >= warmup {
			last = i
		}
	}
	if last < 0 || int64(s.T[last])+M5MS != until {
		return nil, 0, fmt.Errorf("regime input does not reach declared complete-history watermark")
	}
	return bars, used, nil
}

func calculate(bars []NativeBar) ([]IndicatorRow, error) {
	n := len(bars)
	close := make([]float64, n)
	volume := make([]float64, n)
	tr := make([]float64, n)
	for i, b := range bars {
		close[i] = b.Close
		volume[i] = b.Volume
		tr[i] = b.High - b.Low
		if i > 0 {
			tr[i] = math.Max(tr[i], math.Max(math.Abs(b.High-close[i-1]), math.Abs(b.Low-close[i-1])))
		}
	}
	h9, h21, h25 := hma(close, 9), hma(close, 21), hma(close, 25)
	a10, a14 := rma(tr, 10), rma(tr, 14)
	upper, lower, st := nanSlice(n), nanSlice(n), nanSlice(n)
	direction := make([]int, n)
	out := make([]IndicatorRow, n)
	fullCount := 0
	for i, b := range bars {
		direction[i] = 1
		if finite(a10[i]) {
			bu, bl := (b.High+b.Low)/2+3*a10[i], (b.High+b.Low)/2-3*a10[i]
			upper[i], lower[i] = bu, bl
			if i > 0 {
				if finite(upper[i-1]) && bu >= upper[i-1] && close[i-1] <= upper[i-1] {
					upper[i] = upper[i-1]
				}
				if finite(lower[i-1]) && bl <= lower[i-1] && close[i-1] >= lower[i-1] {
					lower[i] = lower[i-1]
				}
			}
			if i > 0 && finite(a10[i-1]) {
				if st[i-1] == upper[i-1] {
					if close[i] > upper[i] {
						direction[i] = -1
					}
				} else {
					direction[i] = -1
					if close[i] < lower[i] {
						direction[i] = 1
					}
				}
			}
			st[i] = upper[i]
			if direction[i] == -1 {
				st[i] = lower[i]
			}
		}
		mean := math.NaN()
		if i >= 19 {
			sum := 0.
			for j := i - 19; j <= i; j++ {
				sum += volume[j]
			}
			mean = sum / 20
		}
		cross := 0
		if i > 0 {
			cross = crossDirection(h9[i]-h21[i], h9[i-1]-h21[i-1])
		}
		if b.Complete {
			fullCount++
		} else {
			fullCount = 0
		}
		volOK := b.Volume > mean
		candidate := candidateSignal(cross, direction[i], volOK, fullCount >= 40)
		// Warm-up NaNs are expected; once each field is mathematically ready,
		// nonfinite values mean overflow and must not silently suppress signals.
		for _, field := range []struct {
			v     float64
			ready bool
		}{{h9[i], i >= 10}, {h21[i], i >= 24}, {h25[i], i >= 28}, {a10[i], i >= 9}, {a14[i], i >= 13}, {st[i], i >= 9}, {mean, i >= 19}} {
			if field.ready && !finite(field.v) {
				return nil, fmt.Errorf("nonfinite derived indicator at bucket %d", b.BucketT)
			}
		}
		out[i] = IndicatorRow{NativeBar: b, HMA9: nullable(h9[i]), HMA21: nullable(h21[i]), HMA25: nullable(h25[i]), ATR10: nullable(a10[i]), ATR14: nullable(a14[i]), Supertrend: nullable(st[i]), Regime: direction[i], VolumeMean: nullable(mean), VolumeOK: volOK, Complete40: fullCount >= 40, Cross: cross, Candidate: candidate}
	}
	return out, nil
}

func crossDirection(current, previous float64) int {
	if current > 0 && previous <= 0 {
		return 1
	}
	if current < 0 && previous >= 0 {
		return -1
	}
	return 0
}
func candidateSignal(cross, regime int, volumeOK, complete bool) int {
	if complete && volumeOK && cross == -regime {
		return cross
	}
	return 0
}
