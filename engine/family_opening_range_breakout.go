package engine

import (
	"fmt"
	"math"
	"time"
)

type openingRange struct {
	Session    string
	Day        int64
	StartIndex int
	EndIndex   int
	High       float64
	Low        float64
	Size       float64
}

type windowBar struct {
	Index            int
	MinutesFromStart int
}

func (b *broker) onOpeningRangeBreakoutBar(i int) {
	if b.hasPosition {
		if b.params.ORBUTCSlotMinutes > 0 {
			progress, _ := utcSlotProgress(b.series.T[i], b.params.ORBUTCSlotMinutes)
			if key, ok := b.position.Meta["slotKey"].(string); ok && key != progress.Key {
				b.closePosition(b.series.C[i], i, "window-close")
				return
			}
		}
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasORBEntry && i-b.orbLastEntry < p.CooldownBars {
		return
	}
	r, ok := b.openingRangeFor(i)
	if !ok {
		return
	}
	seenKey := r.Session
	if b.seen.orb.seen(r.Day, seenKey) {
		return
	}
	for _, s := range []side{sideLong, sideShort} {
		setup, ok := b.openingRangeBreakoutSetup(i, r, s)
		if !ok {
			continue
		}
		b.seen.orb.add(r.Day, seenKey)
		b.enterSetup(i, setup)
		b.orbLastEntry = i
		b.hasORBEntry = true
		break
	}
}

func (b *broker) openingRangeFor(i int) (openingRange, bool) {
	allowed := openingAllowedWindows(b.params)
	var progress windowProgress
	var ok bool
	if b.params.ORBUTCSlotMinutes > 0 {
		progress, ok = utcSlotProgress(b.series.T[i], b.params.ORBUTCSlotMinutes)
	} else {
		progress, ok = tradeWindowProgress(b.series.T[i], allowed)
	}
	if !ok {
		return openingRange{}, false
	}
	prior := b.sameWindowBars(i, progress, allowed)
	var rangeBars []windowBar
	if b.params.ORBFirstMinutes > 0 {
		if progress.MinutesFromStart < b.params.ORBFirstMinutes {
			return openingRange{}, false
		}
		rangeBars = make([]windowBar, 0, len(prior))
		for _, bar := range prior {
			if bar.MinutesFromStart < b.params.ORBFirstMinutes {
				rangeBars = append(rangeBars, bar)
			}
		}
	} else {
		if b.params.ORBFirstCandles <= 0 || b.params.ORBFirstCandles > len(prior) {
			return openingRange{}, false
		}
		rangeBars = make([]windowBar, 0, b.params.ORBFirstCandles)
		rangeBars = append(rangeBars, prior[:b.params.ORBFirstCandles]...)
	}
	if len(rangeBars) == 0 {
		return openingRange{}, false
	}
	hi := math.Inf(-1)
	lo := math.Inf(1)
	for _, item := range rangeBars {
		hi = math.Max(hi, b.series.H[item.Index])
		lo = math.Min(lo, b.series.L[item.Index])
	}
	if !isFinite(hi) || !isFinite(lo) || hi <= lo {
		return openingRange{}, false
	}
	day := localDayKey(b.series.T[i])
	if progress.utcSlot {
		day = progress.SlotStart
	}
	return openingRange{
		Session:    progress.Key,
		Day:        day,
		StartIndex: rangeBars[0].Index,
		EndIndex:   rangeBars[len(rangeBars)-1].Index,
		High:       hi,
		Low:        lo,
		Size:       hi - lo,
	}, true
}

func (b *broker) sameWindowBars(i int, progress windowProgress, allowed map[string]bool) []windowBar {
	day := localDayKey(b.series.T[i])
	if progress.utcSlot {
		day = progress.SlotStart
	}
	out := []windowBar{}
	for j := i - 1; j >= 0; j-- {
		if !progress.utcSlot && localDayKey(b.series.T[j]) != day {
			break
		}
		p, ok := progressFor(b.series.T[j], progress, allowed)
		if !ok || p.Key != progress.Key {
			if len(out) > 0 {
				break
			}
			continue
		}
		out = append(out, windowBar{Index: j, MinutesFromStart: p.MinutesFromStart})
	}
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out
}

func utcSlotProgress(t float64, slotMinutes int) (windowProgress, bool) {
	if slotMinutes <= 0 {
		return windowProgress{}, false
	}
	slotMS := int64(slotMinutes) * 60_000
	start := floorDivInt64(int64(t), slotMS) * slotMS
	minutes := int(floorDivInt64(int64(t)-start, 60_000))
	return windowProgress{Key: fmt.Sprintf("utc:%d", start), MinutesFromStart: minutes, utcSlot: true, slotMinutes: slotMinutes, SlotStart: start}, true
}

func progressFor(t float64, want windowProgress, allowed map[string]bool) (windowProgress, bool) {
	if want.utcSlot {
		return utcSlotProgress(t, want.slotMinutes)
	}
	return tradeWindowProgress(t, allowed)
}

func (b *broker) openingHeldBeyond(i int, r openingRange, s side, buffer float64) bool {
	hold := b.params.ORBHoldCandles
	if i < hold || i <= r.EndIndex {
		return false
	}
	for j := i - hold + 1; j <= i; j++ {
		if j <= r.EndIndex {
			return false
		}
		c := b.series.C[j]
		if s == sideLong {
			if c <= r.High+buffer {
				return false
			}
		} else if c >= r.Low-buffer {
			return false
		}
	}
	pre := b.series.C[i-hold]
	if s == sideLong {
		return pre <= r.High+buffer
	}
	return pre >= r.Low-buffer
}

func (b *broker) orbRetestedEdge(i int, r openingRange, s side, atr float64) bool {
	p := b.params
	if !p.ORBRequireRetest {
		return true
	}
	tol := atr * p.ORBRetestToleranceATR
	edge := r.High
	if s == sideShort {
		edge = r.Low
	}
	from := maxInt(r.EndIndex+1, i-p.ORBRetestCandles+1)
	for j := from; j <= i; j++ {
		if s == sideLong && b.series.L[j] <= edge+tol && b.series.C[j] > edge {
			return true
		}
		if s == sideShort && b.series.H[j] >= edge-tol && b.series.C[j] < edge {
			return true
		}
	}
	return false
}

func (b *broker) openingRangeBreakoutSetup(i int, r openingRange, s side) (setupPlan, bool) {
	p := b.params
	pip := 0.0
	if p.ORBFixedStopPips > 0 || p.ORBFixedTargetPips > 0 {
		var reviewed bool
		pip, reviewed = reviewedInstrumentPip(b.fixture.Symbol)
		if !reviewed {
			return setupPlan{}, false
		}
	}
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 || r.Size == 0 {
		return setupPlan{}, false
	}
	if s == sideLong && !p.AllowLong {
		return setupPlan{}, false
	}
	if s == sideShort && !p.AllowShort {
		return setupPlan{}, false
	}
	if !b.htfAllows(i, s) {
		return setupPlan{}, false
	}
	buffer := atr * p.ORBBreakBufferATR
	if !b.openingHeldBeyond(i, r, s, buffer) || !b.orbRetestedEdge(i, r, s, atr) {
		return setupPlan{}, false
	}
	entry := b.series.C[i]
	var stop float64
	if p.ORBFixedStopPips > 0 {
		stop = entry - float64(s)*p.ORBFixedStopPips*pip
	} else if p.ORBStopATRMult > 0 {
		stop = entry - float64(s)*atr*p.ORBStopATRMult
	} else if s == sideLong {
		stop = r.Low - atr*p.StopBufferATR
	} else {
		stop = r.High + atr*p.StopBufferATR
	}
	if p.ORBFixedStopPips <= 0 && !stopOK(entry, stop, atr, p.MinStopATR, p.MaxStopATR) {
		return setupPlan{}, false
	}
	risk := math.Abs(entry - stop)
	target := entry + float64(s)*risk*p.ORBTargetR
	if p.ORBFixedTargetPips > 0 {
		target = entry + float64(s)*p.ORBFixedTargetPips*pip
	} else if p.ORBTargetRangeMultiple > 0 {
		target = entry + float64(s)*r.Size*p.ORBTargetRangeMultiple
	}
	meta := TradeMeta{
		"setup":      "openingRangeBreakout",
		"side":       s.String(),
		"session":    r.Session,
		"rangeHi":    r.High,
		"rangeLo":    r.Low,
		"rangeStart": r.StartIndex,
		"rangeEnd":   r.EndIndex,
	}
	if p.ORBUTCSlotMinutes > 0 {
		meta["slotKey"] = r.Session
		meta["slotStart"] = r.Day
		meta["slotMinutes"] = p.ORBUTCSlotMinutes
		meta["slotEnd"] = r.Day + int64(p.ORBUTCSlotMinutes)*60_000
		meta["forwardExitAt"] = time.UnixMilli(meta["slotEnd"].(int64)).UTC().Format("2006-01-02T15:04:05.000Z")
		meta["forwardExitReason"] = "window-close"
	}
	if p.ORBFixedStopPips > 0 || p.ORBFixedTargetPips > 0 {
		meta["forwardBracketAnchor"] = "entry-fill"
		meta["forwardStopDistance"] = math.Abs(stop - entry)
		meta["forwardTargetDistance"] = p.ORBFixedTargetPips * pip
	}
	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: target,
		Tag:    "DSL-ORB:" + r.Session,
		Meta:   gradeMeta(meta),
	}, true
}
