package engine

import (
	"fmt"
	"math"
)

type dayRange struct {
	High float64
	Low  float64
}

func (b *broker) onInsideDayExpansionBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasIDEEntry && i-b.ideLastEntry < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	r, ok := b.insideDayRange(i)
	if !ok {
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
		setup, ok := b.insideDayExpansionSetup(i, r, s)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		level := r.Low
		if s == sideLong {
			level = r.High
		}
		seenKey := fmt.Sprintf("%s:%.0f", s.String(), math.Round(level*10))
		if b.seen.ide.seen(day, seenKey) {
			continue
		}
		b.seen.ide.add(day, seenKey)
		b.enterSetup(i, setup)
		b.ideLastEntry = i
		b.hasIDEEntry = true
		break
	}
}

func (b *broker) dayRangeFor(i int, day int64) (dayRange, bool) {
	hi := math.Inf(-1)
	lo := math.Inf(1)
	found := false
	for j := i - 1; j >= 0; j-- {
		key := localDayKey(b.series.T[j])
		if key > day {
			continue
		}
		if key < day {
			break
		}
		hi = math.Max(hi, b.series.H[j])
		lo = math.Min(lo, b.series.L[j])
		found = true
	}
	return dayRange{High: hi, Low: lo}, found
}

func (b *broker) insideDayRange(i int) (dayRange, bool) {
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return dayRange{}, false
	}
	today := localDayKey(b.series.T[i])
	prev, ok := b.dayRangeFor(i, today-1)
	if !ok {
		return dayRange{}, false
	}
	prior, ok := b.dayRangeFor(i, today-2)
	if !ok {
		return dayRange{}, false
	}
	tol := atr * b.params.IDEInsideToleranceATR
	if prev.High <= prior.High+tol && prev.Low >= prior.Low-tol {
		return prev, true
	}
	return dayRange{}, false
}

func (b *broker) insideHeldBeyond(i int, level float64, s side) bool {
	hold := b.params.IDEHoldCandles
	if i < hold {
		return false
	}
	for j := i - hold + 1; j <= i; j++ {
		if s == sideLong {
			if b.series.C[j] <= level {
				return false
			}
		} else if b.series.C[j] >= level {
			return false
		}
	}
	pre := b.series.C[i-hold]
	if s == sideLong {
		return pre <= level
	}
	return pre >= level
}

func (b *broker) insideDayExpansionSetup(i int, r dayRange, s side) (setupPlan, bool) {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return setupPlan{}, false
	}
	level := r.Low
	levelKey := "PDL"
	if s == sideLong {
		level = r.High
		levelKey = "PDH"
	}
	if !b.insideHeldBeyond(i, level, s) || !triggerOK(b.series, i, s, p.IDEUseTrigger) {
		return setupPlan{}, false
	}
	entry := b.series.C[i]
	stop := level + atr*p.StopBufferATR
	if s == sideLong {
		stop = level - atr*p.StopBufferATR
	}
	if !stopOK(entry, stop, atr, p.MinStopATR, p.MaxStopATR) {
		return setupPlan{}, false
	}
	risk := math.Abs(entry - stop)
	target := entry + float64(s)*risk*p.IDETargetR
	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: target,
		Tag:    "DSL-IDE",
		Meta: gradeMeta(TradeMeta{
			"setup":      "insideDayExpansion",
			"side":       s.String(),
			"levelKey":   levelKey,
			"levelPrice": level,
			"rangeHi":    r.High,
			"rangeLo":    r.Low,
		}),
	}, true
}
