package engine

import (
	"fmt"
	"math"
)

func (b *broker) onTrendPullbackBar(i int) {
	inWindow := inSetupTradeWindow(b.series.T[i], b.params, 0)
	if inWindow {
		b.updateTrendPullbackSessionVWAPTouch(i)
	}
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.cols.Regime[i] != regimeTrending {
		b.tpbAttemptsLong = 0
		b.tpbAttemptsShort = 0
		b.hasTPBLastAttemptLong = false
		b.hasTPBLastAttemptShort = false
	}
	if !inWindow {
		return
	}
	if b.hasTPBEntry && i-b.tpbLastEntry < p.CooldownBars {
		return
	}
	if b.cols.Regime[i] != regimeTrending {
		return
	}
	er := b.cols.ER[i]
	if er < p.TPBMinER || er > p.TPBMaxER {
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
		setup, ok := b.trendPullbackSetup(i, s)
		if !ok {
			continue
		}
		attempts := b.recordTrendPullbackAttempt(i, s)
		if attempts < p.TPBAttemptCount {
			continue
		}
		setup.Meta["attempt"] = attempts
		b.enterSetup(i, setup)
		b.tpbLastEntry = i
		b.hasTPBEntry = true
		b.tpbAttemptsLong = 0
		b.tpbAttemptsShort = 0
		break
	}
}

func (b *broker) trendPullbackSetup(i int, s side) (setupPlan, bool) {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 || i >= len(b.ema) || i >= len(b.emaSlope) || !isFinite(b.ema[i]) {
		return setupPlan{}, false
	}
	ema := b.ema[i]
	slope := b.emaSlope[i]
	useSessionVWAP := p.TPBVWAPTouch != ""
	vwap := b.cols.VWAP[i]
	if useSessionVWAP && !isFinite(vwap) {
		return setupPlan{}, false
	}
	if s == sideLong {
		if (useSessionVWAP && b.series.C[i] <= vwap) || (!useSessionVWAP && b.series.C[i] <= ema) || !(slope > 0) {
			return setupPlan{}, false
		}
		if p.TPBUseTrigger && !longTrigger(b.series, i) {
			return setupPlan{}, false
		}
	} else {
		if (useSessionVWAP && b.series.C[i] >= vwap) || (!useSessionVWAP && b.series.C[i] >= ema) || !(slope < 0) {
			return setupPlan{}, false
		}
		if p.TPBUseTrigger && !shortTrigger(b.series, i) {
			return setupPlan{}, false
		}
	}

	tagIdx := b.recentEMATag(i, s, atr)
	if useSessionVWAP {
		tagIdx = b.recentTrendPullbackSessionVWAPTouch(i)
	}
	if tagIdx < 0 {
		return setupPlan{}, false
	}
	priorStart := maxInt(0, tagIdx-p.TPBImpulseLookbackBars)
	priorEnd := maxInt(priorStart, tagIdx-1)
	priorHi, priorLo := hiLo(b.series, priorStart, priorEnd)
	pullbackHi, pullbackLo := hiLo(b.series, tagIdx, i)
	impulseRange := priorHi - priorLo
	if !(impulseRange > 0) {
		return setupPlan{}, false
	}
	depth := (priorHi - pullbackLo) / impulseRange
	pullbackExtreme := pullbackLo
	if s == sideShort {
		depth = (pullbackHi - priorLo) / impulseRange
		pullbackExtreme = pullbackHi
	}
	if depth > p.TPBMaxPullbackDepth {
		return setupPlan{}, false
	}
	entry := b.series.C[i]
	stop := pullbackExtreme - float64(s)*atr*p.TPBStopBufferATR
	minDist := atr * p.TPBMinStopATR
	if math.Abs(entry-stop) < minDist {
		stop = entry - float64(s)*minDist
	}
	if !stopOK(entry, stop, atr, p.TPBMinStopATR, p.TPBMaxStopATR) {
		return setupPlan{}, false
	}
	risk := math.Abs(entry - stop)
	meta := TradeMeta{
		"setup":           "trendPullback",
		"side":            s.String(),
		"emaLen":          p.TPBEMALen,
		"emaPrice":        ema,
		"emaSlope":        slope,
		"tagIdx":          tagIdx,
		"pullbackExtreme": pullbackExtreme,
		"pullbackDepth":   depth,
		"impulseStart":    priorStart,
		"impulseEnd":      priorEnd,
	}
	if useSessionVWAP {
		meta["sessionVwap"] = vwap
		meta["sessionVwapTouch"] = p.TPBVWAPTouch
		meta["sessionVwapFirstTouchIdx"] = tagIdx
	}
	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: entry + float64(s)*risk*p.TPBTargetR,
		Tag:    "DSL-TPB",
		Meta:   gradeMeta(meta),
	}, true
}

func (b *broker) updateTrendPullbackSessionVWAPTouch(i int) {
	p := b.params
	if p.TPBVWAPTouch == "" || i >= len(b.cols.VWAP) || !isFinite(b.cols.VWAP[i]) {
		return
	}
	progress, ok := tradeWindowProgress(b.series.T[i], allowedWindows(p))
	if !ok {
		return
	}
	key := fmt.Sprintf("%d:%s", localDayKey(b.series.T[i]), progress.Key)
	if b.tpbVWAPSessionKey != key {
		b.tpbVWAPSessionKey = key
		b.tpbVWAPTouchCount = 0
		b.tpbVWAPFirstTouch = -1
	}
	vwap := b.cols.VWAP[i]
	touched := b.series.L[i] <= vwap && vwap <= b.series.H[i]
	if p.TPBVWAPTouch == "close" {
		touched = math.Min(b.series.O[i], b.series.C[i]) <= vwap && vwap <= math.Max(b.series.O[i], b.series.C[i])
	}
	if touched {
		b.tpbVWAPTouchCount++
		if b.tpbVWAPTouchCount == 1 {
			b.tpbVWAPFirstTouch = i
		}
	}
}

func (b *broker) recentTrendPullbackSessionVWAPTouch(i int) int {
	p := b.params
	progress, ok := tradeWindowProgress(b.series.T[i], allowedWindows(p))
	if !ok {
		return -1
	}
	key := fmt.Sprintf("%d:%s", localDayKey(b.series.T[i]), progress.Key)
	if key != b.tpbVWAPSessionKey || b.tpbVWAPFirstTouch < 0 || i-b.tpbVWAPFirstTouch > p.TPBPullbackWithin {
		return -1
	}
	if p.TPBVWAPFirst && b.tpbVWAPTouchCount != 1 {
		return -1
	}
	return b.tpbVWAPFirstTouch
}

func (b *broker) recentEMATag(i int, s side, atr float64) int {
	p := b.params
	from := maxInt(0, i-p.TPBPullbackWithin+1)
	tol := atr * p.TPBPullbackATR
	for j := i; j >= from; j-- {
		if j >= len(b.ema) || !isFinite(b.ema[j]) {
			continue
		}
		if s == sideLong && b.series.L[j] <= b.ema[j]+tol {
			return j
		}
		if s == sideShort && b.series.H[j] >= b.ema[j]-tol {
			return j
		}
	}
	return -1
}

func (b *broker) recordTrendPullbackAttempt(i int, s side) int {
	if s == sideLong {
		isNew := !b.hasTPBLastAttemptLong || i > b.tpbLastAttemptLong+1
		if isNew {
			b.tpbAttemptsLong++
		}
		b.tpbLastAttemptLong = i
		b.hasTPBLastAttemptLong = true
		return b.tpbAttemptsLong
	}
	isNew := !b.hasTPBLastAttemptShort || i > b.tpbLastAttemptShort+1
	if isNew {
		b.tpbAttemptsShort++
	}
	b.tpbLastAttemptShort = i
	b.hasTPBLastAttemptShort = true
	return b.tpbAttemptsShort
}
