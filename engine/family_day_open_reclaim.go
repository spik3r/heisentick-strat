package engine

import (
	"fmt"
	"math"
)

type keyLevel struct {
	Key   string
	Price float64
}

func (b *broker) onDayOpenReclaimBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasDOREntry && i-b.dorLastEntry < p.CooldownBars {
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
		setup, ok := b.dayOpenReclaimSetup(i, s)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		seenKey := fmt.Sprintf("%s:%s:%.0f", s.String(), setup.Meta["levelKey"].(string), math.Round(setup.Meta["dayOpen"].(float64)*10))
		if b.seen.dor.seen(day, seenKey) {
			continue
		}
		b.seen.dor.add(day, seenKey)
		b.enterSetup(i, setup)
		b.dorLastEntry = i
		b.hasDOREntry = true
		break
	}
}

func (b *broker) dayOpenReclaimSetup(i int, s side) (setupPlan, bool) {
	p := b.params
	if !b.reclaimedDayOpen(i, s) {
		return setupPlan{}, false
	}
	extreme, ok := b.stretchedExtreme(i, s)
	if !ok {
		return setupPlan{}, false
	}
	level, ok := b.nearestDORLevel(i, extreme)
	if !ok || !triggerOK(b.series, i, s, p.DORUseTrigger) {
		return setupPlan{}, false
	}
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return setupPlan{}, false
	}
	entry := b.series.C[i]
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
	target := entry + float64(s)*risk*p.DORTargetR
	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: target,
		Tag:    "DSL-DOR:" + level.Key,
		Meta: gradeMeta(TradeMeta{
			"setup":            "dayOpenReclaim",
			"side":             s.String(),
			"levelKey":         level.Key,
			"levelPrice":       level.Price,
			"dayOpen":          b.cols.DayOpen[i],
			"stretchedExtreme": extreme,
		}),
	}, true
}

func (b *broker) reclaimedDayOpen(i int, s side) bool {
	if i < 1 || !isFinite(b.cols.DayOpen[i]) {
		return false
	}
	dayOpen := b.cols.DayOpen[i]
	if s == sideLong {
		return b.series.C[i-1] <= dayOpen && b.series.C[i] > dayOpen
	}
	return b.series.C[i-1] >= dayOpen && b.series.C[i] < dayOpen
}

func (b *broker) stretchedExtreme(i int, s side) (float64, bool) {
	p := b.params
	dayOpen := b.cols.DayOpen[i]
	atr := finiteOrZero(b.cols.ATR[i])
	if !isFinite(dayOpen) || atr == 0 || i < p.DORReclaimCandles {
		return 0, false
	}
	from := maxInt(0, i-p.DORReclaimCandles)
	if s == sideLong {
		extreme := math.Inf(1)
		for j := from; j <= i; j++ {
			extreme = math.Min(extreme, b.series.L[j])
		}
		return extreme, dayOpen-extreme >= atr*p.DORStretchATR
	}
	extreme := math.Inf(-1)
	for j := from; j <= i; j++ {
		extreme = math.Max(extreme, b.series.H[j])
	}
	return extreme, extreme-dayOpen >= atr*p.DORStretchATR
}

func (b *broker) nearestDORLevel(i int, refPrice float64) (keyLevel, bool) {
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return keyLevel{}, false
	}
	wh, wl, hasWeek := priorWeekLevels(b.series, i)
	for _, key := range b.params.DORLevelPriority {
		level, ok := b.levelByKey(i, key, wh, wl, hasWeek)
		if !ok {
			continue
		}
		if math.Abs(level.Price-refPrice) <= atr*b.params.DORLevelDistanceATR {
			return level, true
		}
	}
	return keyLevel{}, false
}

func (b *broker) levelByKey(i int, key string, wh float64, wl float64, hasWeek bool) (keyLevel, bool) {
	switch key {
	case "PDH":
		if isFinite(b.cols.PriorDayH[i]) {
			return keyLevel{Key: key, Price: b.cols.PriorDayH[i]}, true
		}
	case "PDL":
		if isFinite(b.cols.PriorDayL[i]) {
			return keyLevel{Key: key, Price: b.cols.PriorDayL[i]}, true
		}
	case "WH":
		if hasWeek {
			return keyLevel{Key: key, Price: wh}, true
		}
	case "WL":
		if hasWeek {
			return keyLevel{Key: key, Price: wl}, true
		}
	}
	return keyLevel{}, false
}
