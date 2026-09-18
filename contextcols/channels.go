package contextcols

import (
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	channelNone       int8 = 0
	channelAscending  int8 = 1
	channelDescending int8 = 2
	channelFlat       int8 = 3
)

type ChannelOptions struct {
	Enabled              bool
	Lookback             int
	MinSpan              int
	MaxPivots            int
	MinPivots            int
	MinMidCrossings      int
	MinWidthATR          float64
	MaxWidthATR          float64
	BodyTolATR           float64
	MaxBodyViolationRate float64
	MaxSlopeGapATR       float64
	DirectionSlopeATR    float64
	Source               string
}

type ChannelColumns struct {
	Active         []int8
	Direction      []int8
	Upper          []float64
	Lower          []float64
	Mid            []float64
	Width          []float64
	WidthATR       []float64
	StartIdx       []int32
	EndIdx         []int32
	TouchesHigh    []int32
	TouchesLow     []int32
	Crossings      []int32
	SinceActive    []int32
	UpperSlope     []float64
	UpperIntercept []float64
	LowerSlope     []float64
	LowerIntercept []float64
}

type lineModel struct {
	Slope     float64
	Intercept float64
}

type channelState struct {
	Active      bool
	Direction   int8
	Upper       float64
	Lower       float64
	Mid         float64
	Width       float64
	WidthATR    float64
	StartIdx    int
	EndIdx      int
	TouchesHigh int
	TouchesLow  int
	Crossings   int
	SinceActive int
	UpperLine   lineModel
	LowerLine   lineModel
}

func (o ChannelOptions) normalized() ChannelOptions {
	if o.Lookback <= 0 {
		o.Lookback = 180
	}
	if o.MinSpan <= 0 {
		o.MinSpan = 70
	}
	if o.MaxPivots <= 0 {
		o.MaxPivots = 10
	}
	if o.MinPivots <= 0 {
		o.MinPivots = 2
	}
	if o.MinMidCrossings <= 0 {
		o.MinMidCrossings = 2
	}
	if o.MinWidthATR <= 0 {
		o.MinWidthATR = 1.8
	}
	if o.MaxWidthATR <= 0 {
		o.MaxWidthATR = 14
	}
	if o.BodyTolATR <= 0 {
		o.BodyTolATR = 0.35
	}
	if o.MaxBodyViolationRate <= 0 {
		o.MaxBodyViolationRate = 0.08
	}
	if o.MaxSlopeGapATR <= 0 {
		o.MaxSlopeGapATR = 0.035
	}
	if o.DirectionSlopeATR <= 0 {
		o.DirectionSlopeATR = 0.75
	}
	if o.Source == "" {
		o.Source = "wick"
	}
	return o
}

func makeChannelColumns(n int) ChannelColumns {
	cols := ChannelColumns{
		Active:         make([]int8, n),
		Direction:      make([]int8, n),
		Upper:          nanSlice(n),
		Lower:          nanSlice(n),
		Mid:            nanSlice(n),
		Width:          nanSlice(n),
		WidthATR:       nanSlice(n),
		StartIdx:       make([]int32, n),
		EndIdx:         make([]int32, n),
		TouchesHigh:    make([]int32, n),
		TouchesLow:     make([]int32, n),
		Crossings:      make([]int32, n),
		SinceActive:    make([]int32, n),
		UpperSlope:     nanSlice(n),
		UpperIntercept: nanSlice(n),
		LowerSlope:     nanSlice(n),
		LowerIntercept: nanSlice(n),
	}
	for i := range cols.StartIdx {
		cols.StartIdx[i] = -1
		cols.EndIdx[i] = -1
		cols.SinceActive[i] = -1
	}
	return cols
}

func ComputeSwingsClose(series marketdata.Series, k int) Swings {
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
				if series.C[j] >= series.C[p] {
					isHigh = false
				}
				if series.C[j] <= series.C[p] {
					isLow = false
				}
			}
			if isHigh {
				curHigh = series.C[p]
				hasHigh = true
				swings.PivotHighs = append(swings.PivotHighs, Pivot{Idx: p, Price: series.C[p]})
			}
			if isLow {
				curLow = series.C[p]
				hasLow = true
				swings.PivotLows = append(swings.PivotLows, Pivot{Idx: p, Price: series.C[p]})
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

func DetectChannels(series marketdata.Series, atr []float64, swings Swings, options ChannelOptions, pivotK int) ChannelColumns {
	options = options.normalized()
	n := series.Len()
	out := makeChannelColumns(n)
	if !options.Enabled {
		return out
	}
	confirmedHighs := make([]Pivot, 0, options.MaxPivots)
	confirmedLows := make([]Pivot, 0, options.MaxPivots)
	hp, lp := 0, 0
	for i := 0; i < n; i++ {
		for hp < len(swings.PivotHighs) && swings.PivotHighs[hp].Idx+pivotK <= i {
			confirmedHighs = append(confirmedHighs, swings.PivotHighs[hp])
			hp++
		}
		for lp < len(swings.PivotLows) && swings.PivotLows[lp].Idx+pivotK <= i {
			confirmedLows = append(confirmedLows, swings.PivotLows[lp])
			lp++
		}
		startLimit := maxInt(0, i-options.Lookback+1)
		for len(confirmedHighs) > 0 && confirmedHighs[0].Idx < startLimit {
			confirmedHighs = confirmedHighs[1:]
		}
		for len(confirmedLows) > 0 && confirmedLows[0].Idx < startLimit {
			confirmedLows = confirmedLows[1:]
		}
		if len(confirmedHighs) < options.MinPivots || len(confirmedLows) < options.MinPivots {
			continue
		}
		highs := lastPivots(confirmedHighs, options.MaxPivots)
		lows := lastPivots(confirmedLows, options.MaxPivots)
		startIdx := minInt(highs[0].Idx, lows[0].Idx)
		span := i - startIdx
		if span < options.MinSpan {
			continue
		}
		upperLine := fitLine(highs)
		lowerLine := fitLine(lows)
		upperStart := linePrice(upperLine, startIdx)
		lowerStart := linePrice(lowerLine, startIdx)
		upperNow := linePrice(upperLine, i)
		lowerNow := linePrice(lowerLine, i)
		if !(upperStart > lowerStart && upperNow > lowerNow) {
			continue
		}
		curATR := math.Max(atr[i], 1e-9)
		widthStart := upperStart - lowerStart
		widthNow := upperNow - lowerNow
		width := (widthStart + widthNow) / 2
		widthATR := width / curATR
		if widthATR < options.MinWidthATR || widthATR > options.MaxWidthATR {
			continue
		}
		if math.Abs(upperLine.Slope-lowerLine.Slope) > curATR*options.MaxSlopeGapATR {
			continue
		}
		bodyTol := curATR * options.BodyTolATR
		maxViolations := int(math.Floor(float64(span+1) * options.MaxBodyViolationRate))
		violations := 0
		useClose := options.Source == "close"
		for j := startIdx; j <= i; j++ {
			bodyHi := max(series.O[j], series.C[j])
			bodyLo := min(series.O[j], series.C[j])
			if useClose {
				bodyHi = series.C[j]
				bodyLo = series.C[j]
			}
			if bodyHi > linePrice(upperLine, j)+bodyTol || bodyLo < linePrice(lowerLine, j)-bodyTol {
				violations++
				if violations > maxViolations {
					break
				}
			}
		}
		if violations > maxViolations {
			continue
		}
		crossings := midlineCrossingsForChannel(series, startIdx, i, upperLine, lowerLine, curATR*0.15)
		if crossings < options.MinMidCrossings {
			continue
		}
		slope := (upperLine.Slope + lowerLine.Slope) / 2
		slopeThreshold := curATR / float64(maxInt(options.MinSpan, span)) * options.DirectionSlopeATR
		direction := channelFlat
		if slope > slopeThreshold {
			direction = channelAscending
		} else if slope < -slopeThreshold {
			direction = channelDescending
		}
		writeChannel(out, i, projectChannel(channelState{
			Direction:   direction,
			UpperLine:   upperLine,
			LowerLine:   lowerLine,
			WidthATR:    widthATR,
			StartIdx:    startIdx,
			EndIdx:      i,
			TouchesHigh: len(highs),
			TouchesLow:  len(lows),
			Crossings:   crossings,
		}, i, 0))
	}
	return out
}

func (cols ChannelColumns) At(i int) channelState {
	return channelState{
		Active:      cols.Active[i] == 1,
		Direction:   cols.Direction[i],
		Upper:       cols.Upper[i],
		Lower:       cols.Lower[i],
		Mid:         cols.Mid[i],
		Width:       cols.Width[i],
		WidthATR:    cols.WidthATR[i],
		StartIdx:    int(cols.StartIdx[i]),
		EndIdx:      int(cols.EndIdx[i]),
		TouchesHigh: int(cols.TouchesHigh[i]),
		TouchesLow:  int(cols.TouchesLow[i]),
		Crossings:   int(cols.Crossings[i]),
		SinceActive: int(cols.SinceActive[i]),
		UpperLine:   lineModel{Slope: cols.UpperSlope[i], Intercept: cols.UpperIntercept[i]},
		LowerLine:   lineModel{Slope: cols.LowerSlope[i], Intercept: cols.LowerIntercept[i]},
	}
}

func writeChannel(cols ChannelColumns, i int, ch channelState) {
	if ch.Active {
		cols.Active[i] = 1
	}
	cols.Direction[i] = ch.Direction
	cols.Upper[i] = ch.Upper
	cols.Lower[i] = ch.Lower
	cols.Mid[i] = ch.Mid
	cols.Width[i] = ch.Width
	cols.WidthATR[i] = ch.WidthATR
	cols.StartIdx[i] = int32(ch.StartIdx)
	cols.EndIdx[i] = int32(ch.EndIdx)
	cols.TouchesHigh[i] = int32(ch.TouchesHigh)
	cols.TouchesLow[i] = int32(ch.TouchesLow)
	cols.Crossings[i] = int32(ch.Crossings)
	cols.SinceActive[i] = int32(ch.SinceActive)
	cols.UpperSlope[i] = ch.UpperLine.Slope
	cols.UpperIntercept[i] = ch.UpperLine.Intercept
	cols.LowerSlope[i] = ch.LowerLine.Slope
	cols.LowerIntercept[i] = ch.LowerLine.Intercept
}

func projectChannel(ch channelState, idx int, sinceActive int) channelState {
	upper := linePrice(ch.UpperLine, idx)
	lower := linePrice(ch.LowerLine, idx)
	ch.Active = sinceActive == 0
	ch.Upper = upper
	ch.Lower = lower
	ch.Mid = (upper + lower) / 2
	ch.Width = upper - lower
	ch.SinceActive = sinceActive
	return ch
}

func fitLine(points []Pivot) lineModel {
	n := float64(len(points))
	var sx, sy, sxx, sxy float64
	for _, point := range points {
		x := float64(point.Idx)
		sx += x
		sy += point.Price
		sxx += x * x
		sxy += x * point.Price
	}
	denom := n*sxx - sx*sx
	slope := 0.0
	if math.Abs(denom) > 1e-9 {
		slope = (n*sxy - sx*sy) / denom
	}
	return lineModel{Slope: slope, Intercept: (sy - slope*sx) / n}
}

func linePrice(line lineModel, idx int) float64 {
	return line.Intercept + line.Slope*float64(idx)
}

func midlineCrossingsForChannel(series marketdata.Series, start int, end int, upper lineModel, lower lineModel, tol float64) int {
	prev := 0
	crossings := 0
	for i := start; i <= end; i++ {
		mid := (linePrice(upper, i) + linePrice(lower, i)) / 2
		delta := series.C[i] - mid
		side := 0
		if delta > tol {
			side = 1
		} else if delta < -tol {
			side = -1
		}
		if side == 0 {
			continue
		}
		if prev != 0 && side != prev {
			crossings++
		}
		prev = side
	}
	return crossings
}

func lastPivots(pivots []Pivot, maxCount int) []Pivot {
	if len(pivots) <= maxCount {
		return pivots
	}
	return pivots[len(pivots)-maxCount:]
}

func ChannelDirectionName(direction int8) string {
	switch direction {
	case channelAscending:
		return "ascending"
	case channelDescending:
		return "descending"
	default:
		return "flat"
	}
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
