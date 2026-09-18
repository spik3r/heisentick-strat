package engine

import (
	"fmt"
	"math"
)

func (b *broker) onSessionBreakHoldBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasSBHEntry && i-b.sbhLastEntry < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}

	for _, level := range b.sessionBreakHoldLevels(i) {
		if len(p.SBHAllowedLevels) > 0 && !containsString(p.SBHAllowedLevels, level.Key) {
			continue
		}
		if level.Side == sideLong && !p.AllowLong {
			continue
		}
		if level.Side == sideShort && !p.AllowShort {
			continue
		}
		if !b.htfAllows(i, level.Side) {
			continue
		}
		setup, ok := b.sessionBreakHoldSetup(i, level)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		key := fmt.Sprintf("%s:%s", level.Side.String(), level.Key)
		if b.seen.sbh.seen(day, key) {
			continue
		}
		b.seen.sbh.add(day, key)
		b.enterSetup(i, setup)
		b.sbhLastEntry = i
		b.hasSBHEntry = true
		break
	}
}

type sessionLevel struct {
	Key   string
	Price float64
	Side  side
}

func (b *broker) sessionBreakHoldLevels(i int) []sessionLevel {
	return []sessionLevel{
		{Key: "AH", Price: b.cols.PriorSession.Asia.H[i], Side: sideLong},
		{Key: "AL", Price: b.cols.PriorSession.Asia.L[i], Side: sideShort},
		{Key: "LH", Price: b.cols.PriorSession.London.H[i], Side: sideLong},
		{Key: "LL", Price: b.cols.PriorSession.London.L[i], Side: sideShort},
	}
}

func (b *broker) sessionBreakHoldSetup(i int, level sessionLevel) (setupPlan, bool) {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 || !isFinite(level.Price) || i < p.SBHHoldCandles+1 {
		return setupPlan{}, false
	}
	for j := i - p.SBHHoldCandles + 1; j <= i; j++ {
		if level.Side == sideLong && b.series.C[j] <= level.Price {
			return setupPlan{}, false
		}
		if level.Side == sideShort && b.series.C[j] >= level.Price {
			return setupPlan{}, false
		}
	}
	pre := b.series.C[i-p.SBHHoldCandles]
	if level.Side == sideLong && pre > level.Price {
		return setupPlan{}, false
	}
	if level.Side == sideShort && pre < level.Price {
		return setupPlan{}, false
	}
	if level.Side == sideLong {
		if !longTrigger(b.series, i) {
			return setupPlan{}, false
		}
	} else if !shortTrigger(b.series, i) {
		return setupPlan{}, false
	}

	entry := b.series.C[i]
	stop := level.Price - float64(level.Side)*atr*p.SBHStopInsideATR
	if !stopOK(entry, stop, atr, p.SBHMinStopATR, p.SBHMaxStopATR) {
		return setupPlan{}, false
	}
	risk := math.Abs(entry - stop)
	return setupPlan{
		Side:   level.Side,
		Stop:   stop,
		Target: entry + float64(level.Side)*risk*p.SBHTargetR,
		Tag:    "DSL-SBH:" + level.Key,
		Meta: gradeMeta(TradeMeta{
			"setup":      "sessionBreakHold",
			"side":       level.Side.String(),
			"levelKey":   level.Key,
			"levelPrice": level.Price,
		}),
	}, true
}
