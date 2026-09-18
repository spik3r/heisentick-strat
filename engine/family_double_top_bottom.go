package engine

import (
	"fmt"
	"math"
)

type dtbPivot struct {
	Idx   int
	Price float64
}

type dtbPair struct {
	First  dtbPivot
	Second dtbPivot
}

func (b *broker) onDoubleTopBottomBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	b.captureDTBSwing(i)
	p := b.params
	if b.hasDTBCool && i-b.dtbCooldown < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	if p.AllowShort && i > 1 {
		if b.tryDoubleTopBottomSide(i, sideShort) {
			return
		}
	}
	if p.AllowLong && i > 1 {
		b.tryDoubleTopBottomSide(i, sideLong)
	}
}

func (b *broker) captureDTBSwing(i int) {
	p := b.params.DoubleTopBottom
	idx := i - p.PivotWindow
	if idx < p.PivotWindow {
		return
	}
	if b.dtbPivotHigh(idx, p.PivotWindow) {
		last := len(b.dtbHighs) - 1
		if last < 0 || b.dtbHighs[last].Idx != idx {
			b.dtbHighs = append(b.dtbHighs, dtbPivot{Idx: idx, Price: b.series.H[idx]})
		}
	}
	if b.dtbPivotLow(idx, p.PivotWindow) {
		last := len(b.dtbLows) - 1
		if last < 0 || b.dtbLows[last].Idx != idx {
			b.dtbLows = append(b.dtbLows, dtbPivot{Idx: idx, Price: b.series.L[idx]})
		}
	}
	if len(b.dtbHighs) > 40 {
		b.dtbHighs = b.dtbHighs[len(b.dtbHighs)-40:]
	}
	if len(b.dtbLows) > 40 {
		b.dtbLows = b.dtbLows[len(b.dtbLows)-40:]
	}
}

func (b *broker) tryDoubleTopBottomSide(i int, s side) bool {
	stop, target, seenKey, pattern, meta, ok := b.doubleTopBottomSetup(i, s)
	if !ok {
		return false
	}
	if containsString(b.dtbSeen, seenKey) {
		return true
	}
	b.dtbSeen = append(b.dtbSeen, seenKey)
	b.enterSetup(i, setupPlan{Side: s, Stop: stop, Target: target, Tag: "DSL-DTB:" + pattern, Meta: meta})
	b.dtbCooldown = i
	b.hasDTBCool = true
	return true
}

func (b *broker) doubleTopBottomSetup(i int, s side) (float64, float64, string, string, TradeMeta, bool) {
	p := b.params.DoubleTopBottom
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return 0, 0, "", "", nil, false
	}
	if !b.htfAllows(i, s) {
		return 0, 0, "", "", nil, false
	}
	if s == sideShort {
		pair, ok := b.dtbPairForSide(b.dtbHighs, i, atr)
		if !ok {
			return 0, 0, "", "", nil, false
		}
		neckline := b.dtbRangeExtreme(pair.First.Idx, pair.Second.Idx, false)
		if !isFinite(neckline) {
			return 0, 0, "", "", nil, false
		}
		if p.NecklineDepthMinATR != 0 && math.Max(pair.First.Price, pair.Second.Price)-neckline < atr*p.NecklineDepthMinATR {
			return 0, 0, "", "", nil, false
		}
		trigger := pair.Second.Price - atr*p.NecklineTouchATR
		breakout := neckline - atr*p.BreakoutBufferATR
		if i-pair.Second.Idx > p.ConfirmCandles || b.series.C[i] > trigger || b.series.C[i] > breakout {
			return 0, 0, "", "", nil, false
		}
		entry := b.series.C[i]
		stop := math.Max(pair.First.Price, pair.Second.Price) + atr*b.params.StopBufferATR
		if !stopOK(entry, stop, atr, b.params.MinStopATR, b.params.MaxStopATR) {
			return 0, 0, "", "", nil, false
		}
		if !b.dtbChochConfirmed(i, pair, s) {
			return 0, 0, "", "", nil, false
		}
		risk := stop - entry
		return stop, entry - risk*b.params.TargetR, fmt.Sprintf("dtb:short:%d:%d", pair.First.Idx, pair.Second.Idx), "doubleTop", gradeMeta(TradeMeta{
			"leftIdx":    pair.First.Idx,
			"leftPrice":  pair.First.Price,
			"neckline":   neckline,
			"pattern":    "doubleTop",
			"rightIdx":   pair.Second.Idx,
			"rightPrice": pair.Second.Price,
			"setup":      "doubleTopBottom",
			"side":       "short",
		}), true
	}
	pair, ok := b.dtbPairForSide(b.dtbLows, i, atr)
	if !ok {
		return 0, 0, "", "", nil, false
	}
	neckline := b.dtbRangeExtreme(pair.First.Idx, pair.Second.Idx, true)
	if !isFinite(neckline) {
		return 0, 0, "", "", nil, false
	}
	if p.NecklineDepthMinATR != 0 && neckline-math.Min(pair.First.Price, pair.Second.Price) < atr*p.NecklineDepthMinATR {
		return 0, 0, "", "", nil, false
	}
	trigger := pair.Second.Price + atr*p.NecklineTouchATR
	breakout := neckline + atr*p.BreakoutBufferATR
	if i-pair.Second.Idx > p.ConfirmCandles || b.series.C[i] < trigger || b.series.C[i] < breakout {
		return 0, 0, "", "", nil, false
	}
	entry := b.series.C[i]
	stop := math.Min(pair.First.Price, pair.Second.Price) - atr*b.params.StopBufferATR
	if !stopOK(entry, stop, atr, b.params.MinStopATR, b.params.MaxStopATR) {
		return 0, 0, "", "", nil, false
	}
	if !b.dtbChochConfirmed(i, pair, s) {
		return 0, 0, "", "", nil, false
	}
	risk := entry - stop
	return stop, entry + risk*b.params.TargetR, fmt.Sprintf("dtb:long:%d:%d", pair.First.Idx, pair.Second.Idx), "doubleBottom", gradeMeta(TradeMeta{
		"leftIdx":    pair.First.Idx,
		"leftPrice":  pair.First.Price,
		"neckline":   neckline,
		"pattern":    "doubleBottom",
		"rightIdx":   pair.Second.Idx,
		"rightPrice": pair.Second.Price,
		"setup":      "doubleTopBottom",
		"side":       "long",
	}), true
}

func (b *broker) dtbPairForSide(list []dtbPivot, i int, atr float64) (dtbPair, bool) {
	p := b.params.DoubleTopBottom
	if len(list) < 2 {
		return dtbPair{}, false
	}
	second := list[len(list)-1]
	first := list[len(list)-2]
	gap := second.Idx - first.Idx
	if gap < p.MinPeakGap || gap > p.MaxPeakGap {
		return dtbPair{}, false
	}
	if math.Abs(first.Price-second.Price) > atr*p.MaxPeakDiffATR {
		return dtbPair{}, false
	}
	if second.Idx >= i {
		return dtbPair{}, false
	}
	return dtbPair{First: first, Second: second}, true
}

func (b *broker) dtbRangeExtreme(fromIdx int, toIdx int, pickHigh bool) float64 {
	if fromIdx+1 >= toIdx {
		return math.NaN()
	}
	lo := math.Inf(1)
	hi := math.Inf(-1)
	for j := fromIdx + 1; j < toIdx; j++ {
		lo = math.Min(lo, b.series.L[j])
		hi = math.Max(hi, b.series.H[j])
	}
	if pickHigh {
		return hi
	}
	return lo
}

func (b *broker) dtbChochConfirmed(i int, pair dtbPair, s side) bool {
	p := b.params.DoubleTopBottom
	if p.ChochCandles == 0 {
		return true
	}
	lookbackStart := pair.Second.Idx - maxInt(1, p.ChochLookbackCandles)
	if lookbackStart < p.PivotWindow {
		lookbackStart = p.PivotWindow
	}
	if s == sideLong {
		for j := pair.Second.Idx - 1; j >= lookbackStart; j-- {
			if i-j > p.ChochCandles {
				return false
			}
			if b.dtbPivotHigh(j, p.PivotWindow) {
				return b.series.C[i] > b.series.H[j]
			}
		}
		return false
	}
	for j := pair.Second.Idx - 1; j >= lookbackStart; j-- {
		if i-j > p.ChochCandles {
			return false
		}
		if b.dtbPivotLow(j, p.PivotWindow) {
			return b.series.C[i] < b.series.L[j]
		}
	}
	return false
}

func (b *broker) dtbPivotHigh(idx int, window int) bool {
	h := b.series.H[idx]
	for j := 1; j <= window; j++ {
		if b.series.H[idx-j] >= h || b.series.H[idx+j] >= h {
			return false
		}
	}
	return true
}

func (b *broker) dtbPivotLow(idx int, window int) bool {
	l := b.series.L[idx]
	for j := 1; j <= window; j++ {
		if b.series.L[idx-j] <= l || b.series.L[idx+j] <= l {
			return false
		}
	}
	return true
}
