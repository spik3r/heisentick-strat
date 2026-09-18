package contextcols

import (
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	regimeTrending int8 = 1
	regimeRanging  int8 = 2
	regimeChoppy   int8 = 3
	trendFlat      int8 = 0
	trendUp        int8 = 1
	trendDown      int8 = 2
)

type RangeOptions struct {
	Method       string
	Disabled     bool
	Enabled      *bool
	MinBars      int
	MaxBars      int
	MaxWidthATR  float64
	MinTouches   int
	TouchATR     float64
	Containment  float64
	MinCrossings int
	MaxDriftFrac float64
	BufferATR    float64
	BandSource   string
	MinWidthATR  float64
}

type RangeColumns struct {
	High   []float64
	Low    []float64
	Active []int8
}

type LastRangeColumns struct {
	High        []float64
	Low         []float64
	SinceActive []int32
}

type Pivot struct {
	Idx   int
	Price float64
}

type Swings struct {
	LastHigh   []float64
	LastLow    []float64
	PivotHighs []Pivot
	PivotLows  []Pivot
}

type RangeDetection struct {
	Active []int8
	High   []float64
	Low    []float64
}

func (o RangeOptions) normalized() RangeOptions {
	if o.Method == "" {
		o.Method = "pivot"
	}
	if o.MinBars <= 0 {
		o.MinBars = 24
	}
	if o.MaxBars <= 0 {
		o.MaxBars = 160
	}
	if o.MaxWidthATR <= 0 {
		o.MaxWidthATR = 3
	}
	if o.MinTouches <= 0 {
		o.MinTouches = 3
	}
	if o.TouchATR <= 0 {
		o.TouchATR = 0.5
	}
	if o.Containment <= 0 {
		o.Containment = 0.85
	}
	if o.MinCrossings <= 0 {
		o.MinCrossings = 4
	}
	if o.MaxDriftFrac <= 0 {
		o.MaxDriftFrac = 0.5
	}
	if o.BufferATR <= 0 {
		o.BufferATR = 0.25
	}
	if o.BandSource == "" {
		o.BandSource = "body"
	}
	if o.MinWidthATR <= 0 {
		o.MinWidthATR = 1
	}
	return o
}

func makeRangeColumns(n int) RangeColumns {
	return RangeColumns{High: nanSlice(n), Low: nanSlice(n), Active: make([]int8, n)}
}

func makeLastRangeColumns(n int) LastRangeColumns {
	cols := LastRangeColumns{High: nanSlice(n), Low: nanSlice(n), SinceActive: make([]int32, n)}
	for i := range cols.SinceActive {
		cols.SinceActive[i] = -1
	}
	return cols
}

func ComputeSwings(series marketdata.Series, k int) Swings {
	n := series.Len()
	swings := Swings{LastHigh: nanSlice(n), LastLow: nanSlice(n)}
	var curHigh, curLow float64
	hasHigh := false
	hasLow := false
	for i := 0; i < n; i++ {
		p := i - k
		if p >= k {
			isHigh := true
			isLow := true
			for j := p - k; j <= p+k; j++ {
				if j == p {
					continue
				}
				if series.H[j] >= series.H[p] {
					isHigh = false
				}
				if series.L[j] <= series.L[p] {
					isLow = false
				}
			}
			if isHigh {
				curHigh = series.H[p]
				hasHigh = true
				swings.PivotHighs = append(swings.PivotHighs, Pivot{Idx: p, Price: series.H[p]})
			}
			if isLow {
				curLow = series.L[p]
				hasLow = true
				swings.PivotLows = append(swings.PivotLows, Pivot{Idx: p, Price: series.L[p]})
			}
		}
		if hasHigh {
			swings.LastHigh[i] = curHigh
		}
		if hasLow {
			swings.LastLow[i] = curLow
		}
	}
	return swings
}

func DetectRanges(series marketdata.Series, atr []float64, swings Swings, options RangeOptions, pivotK int) RangeDetection {
	options = options.normalized()
	n := series.Len()
	out := RangeDetection{Active: make([]int8, n), High: nanSlice(n), Low: nanSlice(n)}
	if options.Disabled || (options.Enabled != nil && !*options.Enabled) {
		return out
	}

	upper, lower := rangeBandFuncs(options)
	for i := options.MinBars - 1; i < n; i++ {
		bound := options.MaxWidthATR * atr[i]
		start := i
		hi := upper(series, i)
		lo := lower(series, i)
		for start-1 >= 0 && i-(start-1)+1 <= options.MaxBars {
			nh := max(hi, upper(series, start-1))
			nl := min(lo, lower(series, start-1))
			if nh-nl > bound {
				break
			}
			start--
			hi = nh
			lo = nl
		}
		if qualifiesRange(series, atr, swings, options, pivotK, start, i, hi, lo) {
			buf := edgeBuffer(options, atr[i])
			out.Active[i] = 1
			out.High[i] = hi + buf
			out.Low[i] = lo - buf
		}
	}
	return out
}

func rangeBandFuncs(options RangeOptions) (func(marketdata.Series, int) float64, func(marketdata.Series, int) float64) {
	if options.Method == "zone" && options.BandSource == "body" {
		return func(s marketdata.Series, i int) float64 { return max(s.O[i], s.C[i]) },
			func(s marketdata.Series, i int) float64 { return min(s.O[i], s.C[i]) }
	}
	return func(s marketdata.Series, i int) float64 { return s.H[i] },
		func(s marketdata.Series, i int) float64 { return s.L[i] }
}

func edgeBuffer(options RangeOptions, atr float64) float64 {
	if options.Method == "zone" {
		return options.BufferATR * atr
	}
	return 0
}

func qualifiesRange(series marketdata.Series, atr []float64, swings Swings, options RangeOptions, pivotK int, start int, end int, hi float64, lo float64) bool {
	if options.Method == "zone" {
		return qualifiesZone(series, atr, options, start, end, hi, lo)
	}
	return qualifiesPivot(series, atr, swings, options, pivotK, start, end, hi, lo)
}

func qualifiesPivot(series marketdata.Series, atr []float64, swings Swings, options RangeOptions, pivotK int, start int, end int, hi float64, lo float64) bool {
	length := end - start + 1
	if length < options.MinBars {
		return false
	}
	width := hi - lo
	if width <= 0 || !driftOK(series, start, end, width, options.MaxDriftFrac) {
		return false
	}
	tol := max(options.TouchATR*atr[end], 0.12*width)
	if countTouches(swings.PivotHighs, start, end, hi, tol, pivotK) < options.MinTouches {
		return false
	}
	if countTouches(swings.PivotLows, start, end, lo, tol, pivotK) < options.MinTouches {
		return false
	}
	if !crossingsOK(series, start, end, hi, lo, options.MinCrossings) {
		return false
	}
	maxOut := length - int(math.Ceil(options.Containment*float64(length)))
	outside := 0
	for j := start; j <= end; j++ {
		if series.H[j] > hi+tol || series.L[j] < lo-tol {
			outside++
			if outside > maxOut {
				return false
			}
		}
	}
	return true
}

func qualifiesZone(series marketdata.Series, atr []float64, options RangeOptions, start int, end int, hi float64, lo float64) bool {
	length := end - start + 1
	if length < options.MinBars {
		return false
	}
	width := hi - lo
	if width <= 0 || width < options.MinWidthATR*atr[end] {
		return false
	}
	if !driftOK(series, start, end, width, options.MaxDriftFrac) || !crossingsOK(series, start, end, hi, lo, options.MinCrossings) {
		return false
	}
	upper, lower := rangeBandFuncs(options)
	tol := options.TouchATR * atr[end]
	touchHi := 0
	touchLo := 0
	lastHi := -2
	lastLo := -2
	for j := start; j <= end; j++ {
		if upper(series, j) >= hi-tol && j-lastHi > 1 {
			touchHi++
			lastHi = j
		}
		if lower(series, j) <= lo+tol && j-lastLo > 1 {
			touchLo++
			lastLo = j
		}
	}
	if touchHi < options.MinTouches || touchLo < options.MinTouches {
		return false
	}
	buf := max(options.TouchATR, options.BufferATR) * atr[end]
	maxOut := length - int(math.Ceil(options.Containment*float64(length)))
	outside := 0
	for j := start; j <= end; j++ {
		if series.H[j] > hi+buf || series.L[j] < lo-buf {
			outside++
			if outside > maxOut {
				return false
			}
		}
	}
	return true
}

func driftOK(series marketdata.Series, start int, end int, width float64, maxDriftFrac float64) bool {
	return math.Abs(series.C[end]-series.C[start]) <= maxDriftFrac*width
}

func crossingsOK(series marketdata.Series, start int, end int, hi float64, lo float64, minCrossings int) bool {
	mid := (hi + lo) / 2
	crossings := 0
	prevSide := sign(series.C[start] - mid)
	for j := start + 1; j <= end; j++ {
		side := sign(series.C[j] - mid)
		if side != 0 && side != prevSide && prevSide != 0 {
			crossings++
		}
		if side != 0 {
			prevSide = side
		}
	}
	return crossings >= minCrossings
}

func countTouches(pivots []Pivot, start int, end int, edge float64, tol float64, pivotK int) int {
	touches := 0
	for k := lowerBoundPivots(pivots, start); k < len(pivots); k++ {
		pivot := pivots[k]
		if pivot.Idx > end {
			break
		}
		if pivot.Idx+pivotK > end {
			continue
		}
		if math.Abs(pivot.Price-edge) <= tol {
			touches++
		}
	}
	return touches
}

func lowerBoundPivots(pivots []Pivot, target int) int {
	lo := 0
	hi := len(pivots)
	for lo < hi {
		mid := (lo + hi) >> 1
		if pivots[mid].Idx < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

func classifyRegimeTrend(series marketdata.Series, i int, er float64, erLen int, trendER float64, rangeActive bool) (int8, int8) {
	if rangeActive {
		return regimeRanging, trendFlat
	}
	if er >= trendER {
		ref := i - erLen
		if ref < 0 {
			ref = 0
		}
		if series.C[i] >= series.C[ref] {
			return regimeTrending, trendUp
		}
		return regimeTrending, trendDown
	}
	return regimeChoppy, trendFlat
}

func classifyRegimeTrendState(series marketdata.Series, i int, er float64, erLen int, enterER, exitER float64, persistenceBars int, rangeActive bool, active *bool, candidateBars *int) (int8, int8) {
	if rangeActive {
		*active = false
		*candidateBars = 0
		return regimeRanging, trendFlat
	}
	if *active {
		if er < exitER {
			*active = false
			*candidateBars = 0
		}
	} else if er >= enterER {
		*candidateBars++
		if *candidateBars >= persistenceBars {
			*active = true
		}
	} else {
		*candidateBars = 0
	}
	if !*active {
		return regimeChoppy, trendFlat
	}
	ref := i - erLen
	if ref < 0 {
		ref = 0
	}
	if series.C[i] >= series.C[ref] {
		return regimeTrending, trendUp
	}
	return regimeTrending, trendDown
}
