package engine

import "math"

type tpePivot struct {
	Idx   int
	Price float64
}

type triplePushSeq struct {
	Pushes  [3]tpePivot
	Leg2    float64
	Leg3    float64
	Extreme float64
	Base    float64
}

func (b *broker) onTriplePushExhaustionBar(i int) {
	b.updateTPEPivots(i)
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasTPEEntry && i-b.tpeLastEntry < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	for _, s := range []side{sideShort, sideLong} {
		if s == sideLong && !p.AllowLong {
			continue
		}
		if s == sideShort && !p.AllowShort {
			continue
		}
		if p.TriplePush.UseHTFBias && !b.htfAllowsFade(i, s) {
			continue
		}
		setup, ok := b.triplePushSetup(i, s)
		if !ok {
			continue
		}
		b.enterSetup(i, setup)
		b.tpeLastEntry = i
		b.hasTPEEntry = true
		break
	}
}

func (b *broker) updateTPEPivots(i int) {
	p := b.params.TriplePush
	k := p.PivotK
	j := i - k
	if j <= k || j <= b.tpeLastConfirmed {
		return
	}
	b.tpeLastConfirmed = j
	from := j - k
	to := j + k
	hj := b.series.H[j]
	lj := b.series.L[j]
	isHigh := true
	isLow := true
	for n := from; n <= to; n++ {
		if n == j {
			continue
		}
		if b.series.H[n] >= hj {
			isHigh = false
		}
		if b.series.L[n] <= lj {
			isLow = false
		}
		if !isHigh && !isLow {
			break
		}
	}
	if isHigh {
		b.tpeHighs = append(b.tpeHighs, tpePivot{Idx: j, Price: hj})
		if len(b.tpeHighs) > 24 {
			copy(b.tpeHighs, b.tpeHighs[1:])
			b.tpeHighs = b.tpeHighs[:24]
		}
	}
	if isLow {
		b.tpeLows = append(b.tpeLows, tpePivot{Idx: j, Price: lj})
		if len(b.tpeLows) > 24 {
			copy(b.tpeLows, b.tpeLows[1:])
			b.tpeLows = b.tpeLows[:24]
		}
	}
}

func (b *broker) triplePushSetup(i int, s side) (setupPlan, bool) {
	p := b.params.TriplePush
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return setupPlan{}, false
	}
	er := finiteOrZero(b.cols.ER[i])
	if er < p.MinER || er > p.MaxER {
		return setupPlan{}, false
	}
	seq, ok := b.findTriplePush(i, s)
	if !ok {
		return setupPlan{}, false
	}
	dist := seq.Extreme - b.series.C[i]
	if s == sideLong {
		dist = b.series.C[i] - seq.Extreme
	}
	if dist < -atr*0.1 || dist > atr*p.NearATR {
		return setupPlan{}, false
	}
	if closeLocation(b.series, i, s) < p.ConfirmCloseLocation {
		return setupPlan{}, false
	}
	if p.UseTrigger && !triggerOK(b.series, i, s, true) {
		return setupPlan{}, false
	}
	entry := b.series.C[i]
	dir := float64(s)
	stop := seq.Extreme - dir*atr*p.StopPaddingATR
	minDist := atr * p.MinStopATR
	if math.Abs(entry-stop) < minDist {
		stop = entry - dir*minDist
	}
	if !stopOK(entry, stop, atr, p.MinStopATR, p.MaxStopATR) {
		return setupPlan{}, false
	}
	risk := math.Abs(entry - stop)
	target := entry + dir*risk*p.TargetR
	if p.TargetStructure && isFinite(seq.Base) {
		profit := (seq.Base - entry) * dir
		if profit >= risk {
			target = seq.Base
		}
	}
	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: target,
		Tag:    "DSL-TPE",
		Meta: gradeMeta(TradeMeta{
			"leg2":           seq.Leg2,
			"leg3":           seq.Leg3,
			"push1":          seq.Pushes[0].Idx,
			"push2":          seq.Pushes[1].Idx,
			"push3":          seq.Pushes[2].Idx,
			"pushDecayRatio": seq.Leg3 / seq.Leg2,
			"setup":          "triplePushExhaustion",
			"side":           s.String(),
			"thirdPushPrice": seq.Extreme,
		}),
	}, true
}

func (b *broker) findTriplePush(i int, s side) (triplePushSeq, bool) {
	peaks := b.tpeHighs
	valleys := b.tpeLows
	if s == sideLong {
		peaks = b.tpeLows
		valleys = b.tpeHighs
	}
	return triplePushSequence(peaks, valleys, i, b.params.TriplePush, s)
}

func triplePushSequence(peaks []tpePivot, valleys []tpePivot, i int, p triplePushParams, s side) (triplePushSeq, bool) {
	if len(peaks) < 3 {
		return triplePushSeq{}, false
	}
	recent := make([]tpePivot, 0, len(peaks))
	for _, peak := range peaks {
		if i-peak.Idx <= p.LookbackBars {
			recent = append(recent, peak)
		}
	}
	if len(recent) < 3 {
		return triplePushSeq{}, false
	}
	recent = recent[len(recent)-3:]
	a, bb, c := recent[0], recent[1], recent[2]
	dir := 1.0
	if s == sideLong {
		dir = -1
	}
	if !((c.Price-bb.Price)*dir > 0 && (bb.Price-a.Price)*dir > 0) {
		return triplePushSeq{}, false
	}
	if bb.Idx-a.Idx < p.MinSeparation || c.Idx-bb.Idx < p.MinSeparation {
		return triplePushSeq{}, false
	}
	lowAB, okAB := lastPivotBetween(valleys, a.Idx, bb.Idx)
	lowBC, okBC := lastPivotBetween(valleys, bb.Idx, c.Idx)
	if !okAB || !okBC {
		return triplePushSeq{}, false
	}
	leg2 := math.Abs(bb.Price - lowAB.Price)
	leg3 := math.Abs(c.Price - lowBC.Price)
	if p.DecelMode == "slope" {
		slope2 := leg2 / math.Max(1, float64(bb.Idx-lowAB.Idx))
		slope3 := leg3 / math.Max(1, float64(c.Idx-lowBC.Idx))
		if !(slope3 <= slope2*p.PushDecay) {
			return triplePushSeq{}, false
		}
	} else if !(leg3 <= leg2*p.PushDecay) {
		return triplePushSeq{}, false
	}
	base := lowAB
	for _, valley := range valleys {
		if valley.Idx < a.Idx {
			base = valley
		}
	}
	return triplePushSeq{
		Pushes:  [3]tpePivot{a, bb, c},
		Leg2:    leg2,
		Leg3:    leg3,
		Extreme: c.Price,
		Base:    base.Price,
	}, true
}

func lastPivotBetween(pivots []tpePivot, from int, to int) (tpePivot, bool) {
	var out tpePivot
	ok := false
	for _, pivot := range pivots {
		if pivot.Idx > from && pivot.Idx < to {
			out = pivot
			ok = true
		}
	}
	return out, ok
}

func (b *broker) htfAllowsFade(i int, s side) bool {
	if len(b.htfTrend) == 0 {
		return true // HTF not requested for this run
	}
	htfTrend := b.htfTrend[i]
	if htfTrend == htfUnavailable {
		return false // HTF requested but no completed bar -> fail closed
	}
	if htfTrend == trendFlat {
		return true
	}
	if s == sideShort {
		return htfTrend == trendUp
	}
	return htfTrend == trendDown
}
