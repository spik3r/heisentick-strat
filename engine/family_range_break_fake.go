package engine

import (
	"fmt"
	"math"
)

type rbfRange struct {
	High        float64
	Low         float64
	Mid         float64
	SinceActive int
}

func (b *broker) onRangeBreakFakeBar(i int) {
	if b.hasPosition {
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasRBFEntry && i-b.rbfLastEntry < p.CooldownBars {
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
		setup, ok := b.rangeBreakFakeSetup(i, s)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		seenKey := fmt.Sprintf("%s:%.0f:%d", s.String(), setup.Meta["levelPrice"].(float64)*10, i)
		if b.seen.rbf.seen(day, seenKey) {
			continue
		}
		b.seen.rbf.add(day, seenKey)
		b.enterSetup(i, setup)
		b.rbfLastEntry = i
		b.hasRBFEntry = true
		break
	}
}

func (b *broker) currentRBFRange(i int) (rbfRange, bool) {
	since := int(b.cols.LastRange.SinceActive[i])
	hi := b.cols.LastRange.High[i]
	lo := b.cols.LastRange.Low[i]
	if since < 0 || since > b.params.RangeActiveWithinCandles || !isFinite(hi) || !isFinite(lo) {
		return rbfRange{}, false
	}
	return rbfRange{High: hi, Low: lo, Mid: (hi + lo) / 2, SinceActive: since}, true
}

func (b *broker) rangePiercedAndReclaimed(i int, r rbfRange, s side) bool {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	buffer := atr * p.RBFPierceATR
	from := maxInt(0, i-p.RBFReclaimCandles+1)
	pierced := false
	for j := from; j <= i; j++ {
		if s == sideShort && b.series.H[j] > r.High+buffer {
			pierced = true
		}
		if s == sideLong && b.series.L[j] < r.Low-buffer {
			pierced = true
		}
	}
	if !pierced {
		return false
	}
	if !p.RBFRequireCloseBackInside {
		return true
	}
	return b.series.C[i] < r.High && b.series.C[i] > r.Low
}

func (b *broker) rbfRetestOK(i int, r rbfRange, s side) bool {
	p := b.params
	if !p.RBFRetestFailedEdge {
		return true
	}
	edge := r.Low
	if s == sideShort {
		edge = r.High
	}
	from := maxInt(0, i-p.RBFRetestCandles+1)
	for j := from; j <= i; j++ {
		if s == sideShort && b.series.H[j] >= edge && b.series.C[j] < edge {
			return true
		}
		if s == sideLong && b.series.L[j] <= edge && b.series.C[j] > edge {
			return true
		}
	}
	return false
}

func (b *broker) rangeBreakFakeSetup(i int, s side) (setupPlan, bool) {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return setupPlan{}, false
	}
	r, ok := b.currentRBFRange(i)
	if !ok {
		return setupPlan{}, false
	}
	if !b.rangePiercedAndReclaimed(i, r, s) || !b.rbfRetestOK(i, r, s) {
		return setupPlan{}, false
	}
	if !candleQualityOK(b.series, i, s, p.TailRejectionMin, p.CloseLocationMin) {
		return setupPlan{}, false
	}
	entry := b.series.C[i]
	levelPrice := r.Low
	levelKey := "range.low"
	edge := "low"
	if s == sideShort {
		levelPrice = r.High
		levelKey = "range.high"
		edge = "high"
	}
	if p.RBFMaxEntryDistanceATR != 0 && math.Abs(entry-levelPrice) > atr*p.RBFMaxEntryDistanceATR {
		return setupPlan{}, false
	}
	if p.StopLookbackCandles <= 0 {
		return setupPlan{}, false
	}
	stop := recentExtreme(b.series, i, p.StopLookbackCandles, s)
	if s == sideLong {
		stop -= atr * p.StopBufferATR
	} else {
		stop += atr * p.StopBufferATR
	}
	if !stopOK(entry, stop, atr, p.MinStopATR, p.MaxStopATR) {
		return setupPlan{}, false
	}
	risk := math.Abs(entry - stop)
	target := b.rbfTarget(i, r, s, entry, risk)
	targetR := math.Abs(target-entry) / risk
	if (s == sideLong && target <= entry) || (s == sideShort && target >= entry) || targetR < p.MinTargetR {
		return setupPlan{}, false
	}
	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: target,
		Tag:    "DSL-RBF:" + levelKey,
		Meta: gradeMeta(TradeMeta{
			"setup":           "rangeBreakFake",
			"side":            s.String(),
			"edge":            edge,
			"levelKey":        levelKey,
			"levelPrice":      levelPrice,
			"rangeHi":         r.High,
			"rangeLo":         r.Low,
			"rangeMid":        r.Mid,
			"setupAgeCandles": r.SinceActive,
			"targetMode":      p.RBFTargetMode,
		}),
	}, true
}

func (b *broker) rbfTarget(i int, r rbfRange, s side, entry float64, risk float64) float64 {
	p := b.params
	dir := float64(s)
	switch p.RBFTargetMode {
	case "oppositeEdge":
		if s == sideLong {
			return r.High
		}
		return r.Low
	case "vwap":
		if isFinite(b.cols.VWAP[i]) {
			return b.cols.VWAP[i]
		}
	case "fixedR":
		return entry + dir*risk*p.RBFTargetR
	}
	return r.Mid
}
