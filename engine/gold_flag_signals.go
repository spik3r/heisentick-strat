package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

func goldFlagPointer[T any](v T) *T { return &v }

func goldFlagSource(r GoldFlagReferenceRequest) ([]GoldFlagBar, int, error) {
	s := r.M15
	n := s.Len()
	if n == 0 || len(s.O) != n || len(s.H) != n || len(s.L) != n || len(s.C) != n || len(s.V) != n {
		return nil, 0, fmt.Errorf("gold flag requires nonempty equal-length M15 OHLCV")
	}
	out := []GoldFlagBar{}
	used := 0
	previous := -1.
	for i, t := range s.T {
		if !isFinite(t) || t < 0 || t > 9007199254740991 || math.Trunc(t) != t || int64(t)%goldFlagM15 != 0 || t <= previous {
			return nil, 0, fmt.Errorf("invalid or unordered gold flag M15 timestamp at row %d", i)
		}
		previous = t
		if int64(t) < r.FromT || int64(t) >= r.ToT {
			continue
		}
		o, h, l, c, v := s.O[i], s.H[i], s.L[i], s.C[i], s.V[i]
		if !isFinite(o) || !isFinite(h) || !isFinite(l) || !isFinite(c) || !isFinite(v) || l <= 0 || o < l || o > h || c < l || c > h || v < 0 {
			return nil, 0, fmt.Errorf("invalid gold flag OHLCV at row %d", i)
		}
		bucket := int64(t) / goldFlagM30 * goldFlagM30
		if len(out) == 0 || out[len(out)-1].OpenT != bucket {
			out = append(out, GoldFlagBar{Index: len(out), OpenT: bucket, CloseT: bucket + goldFlagM30, FirstObservedT: int64(t), Open: o, High: h, Low: l})
		}
		b := &out[len(out)-1]
		b.High = math.Max(b.High, h)
		b.Low = math.Min(b.Low, l)
		b.Close = c
		b.Volume += v
		b.SourceCount++
		b.Complete = b.SourceCount == 2
		b.LastObservedCloseT = int64(t) + goldFlagM15
		if !isFinite(b.Volume) {
			return nil, 0, fmt.Errorf("gold flag volume overflow")
		}
		used++
	}
	if used == 0 {
		return nil, 0, fmt.Errorf("gold flag selection has no observed source rows")
	}
	return out, used, nil
}

func goldFlagAggregate(bars []GoldFlagBar, size int64) []GoldFlagBar {
	out := []GoldFlagBar{}
	for _, b := range bars {
		bucket := b.OpenT / size * size
		if len(out) == 0 || out[len(out)-1].OpenT != bucket {
			out = append(out, GoldFlagBar{Index: len(out), OpenT: bucket, CloseT: bucket + size, FirstObservedT: b.FirstObservedT, Open: b.Open, High: b.High, Low: b.Low})
		}
		a := &out[len(out)-1]
		a.High = math.Max(a.High, b.High)
		a.Low = math.Min(a.Low, b.Low)
		a.Close = b.Close
		a.Volume += b.Volume
		a.SourceCount += b.SourceCount
		a.LastObservedCloseT = b.LastObservedCloseT
		a.Complete = int64(a.SourceCount) == size/goldFlagM15
	}
	return out
}

// PR388 differs from the generic SMA ATR and from v9's TR0 seed. Preserve the
// exact Wilder arithmetic: first output at index14 averages TR1 through TR14.
func goldFlagATR(bars []GoldFlagBar) []*float64 {
	out := make([]*float64, len(bars))
	a := 0.
	for i := 1; i < len(bars); i++ {
		b := bars[i]
		tr := math.Max(b.High-b.Low, math.Max(math.Abs(b.High-bars[i-1].Close), math.Abs(b.Low-bars[i-1].Close)))
		if i < 14 {
			a += tr
		} else if i == 14 {
			a = (a + tr) / 14
			out[i] = goldFlagPointer(a)
		} else {
			a = (a*13 + tr) / 14
			out[i] = goldFlagPointer(a)
		}
	}
	return out
}

func goldFlagExtreme(bars []GoldFlagBar, count int, decision int64) *GoldFlagExtreme {
	end := sort.Search(len(bars), func(i int) bool { return bars[i].CloseT > decision })
	if end < count {
		return nil
	}
	hi, lo := math.Inf(-1), math.Inf(1)
	for _, b := range bars[end-count : end] {
		hi = math.Max(hi, b.High)
		lo = math.Min(lo, b.Low)
	}
	return &GoldFlagExtreme{FirstBucketT: bars[end-count].OpenT, LastBucketT: bars[end-1].OpenT, AvailableT: bars[end-1].CloseT, High: hi, Low: lo}
}

func goldFlagDetect(bars []GoldFlagBar, i int, a *float64, h4, day *GoldFlagExtreme) *GoldFlagSignal {
	if a == nil || !(*a > 0) || i-11 < 1 {
		return nil
	}
	fs, ps := i-5, i-11
	fH, fL, pH, pL := math.Inf(-1), math.Inf(1), math.Inf(-1), math.Inf(1)
	pHi, pLi := -1, -1
	for j := fs; j <= i; j++ {
		fH = math.Max(fH, bars[j].High)
		fL = math.Min(fL, bars[j].Low)
	}
	for j := ps; j < fs; j++ {
		if bars[j].High > pH {
			pH = bars[j].High
			pHi = j
		}
		if bars[j].Low < pL {
			pL = bars[j].Low
			pLi = j
		}
	}
	pole, flag := pH-pL, fH-fL
	if pole < 2**a || flag > .6*pole || flag > 1.5**a {
		return nil
	}
	side := 0
	if pLi < pHi && fL >= pH-.5*pole {
		side = 1
	}
	if pHi < pLi && fH <= pL+.5*pole {
		if side != 0 {
			return nil
		}
		side = -1
	}
	if side == 0 {
		return nil
	}
	levels := []string{}
	if h4 != nil && ((side == 1 && fH >= h4.High-*a) || (side == -1 && fL <= h4.Low+*a)) {
		levels = append(levels, "h4")
	}
	if day != nil && ((side == 1 && fH >= day.High-*a) || (side == -1 && fL <= day.Low+*a)) {
		levels = append(levels, "day")
	}
	if len(levels) == 0 {
		return nil
	}
	return &GoldFlagSignal{SignalIdx: i, Side: side, ATR: *a, EdgeHi: fH, EdgeLo: fL, Pole: pole, Level: strings.Join(levels, "+")}
}

func goldFlagSnapshots(bars []GoldFlagBar) []GoldFlagSnapshot {
	atr := goldFlagATR(bars)
	h4 := goldFlagAggregate(bars, 4*3600000)
	days := goldFlagAggregate(bars, 24*3600000)
	rows := make([]GoldFlagSnapshot, len(bars))
	for i, b := range bars {
		h, d := goldFlagExtreme(h4, 12, b.CloseT), goldFlagExtreme(days, 5, b.CloseT)
		rows[i] = GoldFlagSnapshot{GoldFlagBar: b, ATR: atr[i], H4: h, Day: d, Candidate: goldFlagDetect(bars, i, atr[i], h, d)}
	}
	return rows
}
