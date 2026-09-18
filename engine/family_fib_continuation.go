package engine

import "math"

type fibImpulse struct {
	StartIdx int
	EndIdx   int
	Start    float64
	End      float64
}

func (b *broker) onFibContinuationBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasFibEntry && i-b.fibLastEntry < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	for _, s := range []side{sideLong, sideShort} {
		if s == sideLong && !p.AllowLong {
			continue
		}
		if s == sideShort && !p.AllowShort {
			continue
		}
		if !b.htfAllows(i, s) {
			continue
		}
		stop, target, meta, ok := b.fibContinuationSetup(i, s)
		if !ok {
			continue
		}
		b.fibLastEntry = i
		b.hasFibEntry = true
		if !b.marketGatesOK(i) {
			break
		}
		b.enterSetup(i, setupPlan{Side: s, Stop: stop, Target: target, Tag: "DSL-FIB", Meta: meta})
		break
	}
}

func (b *broker) fibContinuationSetup(i int, s side) (float64, float64, TradeMeta, bool) {
	p := b.params.FibContinuation
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return 0, 0, nil, false
	}
	impulse, ok := b.fibImpulse(i, s, p)
	if !ok {
		return 0, 0, nil, false
	}
	move := math.Abs(impulse.End - impulse.Start)
	if move < atr*p.ImpulseATR {
		return 0, 0, nil, false
	}
	pullHi, pullLo := hiLo(b.series, impulse.EndIdx+1, i)
	pullExtreme := pullLo
	if s == sideShort {
		pullExtreme = pullHi
	}
	depth := (impulse.End - pullExtreme) / move
	if s == sideShort {
		depth = (pullExtreme - impulse.End) / move
	}
	if depth < p.RetraceMin || depth > p.RetraceMax {
		return 0, 0, nil, false
	}
	if s == sideLong {
		if closeLocation(b.series, i, sideLong) < p.ConfirmCloseLocation {
			return 0, 0, nil, false
		}
		if p.UseTrigger && !(longTrigger(b.series, i) || b.series.C[i] > b.series.O[i]) {
			return 0, 0, nil, false
		}
	} else {
		if closeLocation(b.series, i, sideShort) < p.ConfirmCloseLocation {
			return 0, 0, nil, false
		}
		if p.UseTrigger && !(shortTrigger(b.series, i) || b.series.C[i] < b.series.O[i]) {
			return 0, 0, nil, false
		}
	}
	entry := b.series.C[i]
	dir := float64(s)
	stop := impulse.End - dir*move*p.StopFibRatio - dir*atr*p.StopPaddingATR
	minDist := atr * b.params.MinStopATR
	if math.Abs(entry-stop) < minDist {
		stop = entry - dir*minDist
	}
	if !stopOK(entry, stop, atr, b.params.MinStopATR, b.params.MaxStopATR) {
		return 0, 0, nil, false
	}
	risk := math.Abs(entry - stop)
	fibTarget := impulse.Start + dir*move*p.FibExtension
	rTarget := entry + dir*risk*b.params.TargetR
	target := math.Max(fibTarget, rTarget)
	if s == sideShort {
		target = math.Min(fibTarget, rTarget)
	}
	return stop, target, gradeMeta(TradeMeta{
		"fibExtension":      p.FibExtension,
		"impulseEnd":        impulse.EndIdx,
		"impulseEndPrice":   impulse.End,
		"impulseStart":      impulse.StartIdx,
		"impulseStartPrice": impulse.Start,
		"retraceDepth":      depth,
		"setup":             "fibContinuation",
		"side":              s.String(),
	}), true
}

func (b *broker) fibImpulse(i int, s side, p fibContinuationParams) (fibImpulse, bool) {
	from := i - p.ImpulseLookbackBars
	if from < 0 {
		from = 0
	}
	if s == sideLong {
		low := math.Inf(1)
		lowIdx := -1
		for j := from; j < i; j++ {
			if b.series.L[j] < low {
				low = b.series.L[j]
				lowIdx = j
			}
		}
		if lowIdx < 0 || lowIdx >= i-1 {
			return fibImpulse{}, false
		}
		high := math.Inf(-1)
		highIdx := -1
		for j := lowIdx + 1; j < i; j++ {
			if b.series.H[j] > high {
				high = b.series.H[j]
				highIdx = j
			}
		}
		if highIdx > lowIdx {
			return fibImpulse{StartIdx: lowIdx, EndIdx: highIdx, Start: low, End: high}, true
		}
		return fibImpulse{}, false
	}
	high := math.Inf(-1)
	highIdx := -1
	for j := from; j < i; j++ {
		if b.series.H[j] > high {
			high = b.series.H[j]
			highIdx = j
		}
	}
	if highIdx < 0 || highIdx >= i-1 {
		return fibImpulse{}, false
	}
	low := math.Inf(1)
	lowIdx := -1
	for j := highIdx + 1; j < i; j++ {
		if b.series.L[j] < low {
			low = b.series.L[j]
			lowIdx = j
		}
	}
	if lowIdx > highIdx {
		return fibImpulse{StartIdx: highIdx, EndIdx: lowIdx, Start: high, End: low}, true
	}
	return fibImpulse{}, false
}
