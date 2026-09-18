package engine

import (
	"fmt"
	"math"
)

func (b *broker) onBreakRetestBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasBREntry && i-b.brLastEntry < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	if b.cols.Regime[i] != regimeTrending {
		return
	}
	er := b.cols.ER[i]
	if er < p.BRMinER || er > p.BRMaxER {
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
		setup, ok := b.breakRetestSetup(i, s)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		seenKey := fmt.Sprintf("%s:%s:%.0f", s.String(), setup.Meta["levelKey"].(string), math.Round(setup.Meta["levelPrice"].(float64)*10))
		if b.seen.br.seen(day, seenKey) {
			continue
		}
		b.seen.br.add(day, seenKey)
		b.enterSetup(i, setup)
		b.brLastEntry = i
		b.hasBREntry = true
		break
	}
}

func (b *broker) breakRetestSetup(i int, s side) (setupPlan, bool) {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return setupPlan{}, false
	}
	for _, level := range b.breakRetestLevels(i, s) {
		if !brokeRecently(b.series, i, level.Price, s, p.BRFreshBreakBars) {
			continue
		}
		ran := b.breakRunDistance(i, level.Price, s, p.BRFreshBreakBars)
		if ran < atr*p.BRMinBreakATR {
			continue
		}
		tol := atr * p.BRLevelTolerance
		if !b.retestedLevel(i, level.Price, s, p.BRRetestBars, tol) {
			continue
		}
		if s == sideLong {
			if b.series.C[i] <= level.Price || !longTrigger(b.series, i) {
				continue
			}
		} else if b.series.C[i] >= level.Price || !shortTrigger(b.series, i) {
			continue
		}
		if p.BRDisplacementATR > 0 && b.series.H[i]-b.series.L[i] < atr*p.BRDisplacementATR {
			continue
		}

		entry := b.series.C[i]
		extreme := b.retestExtreme(i, s, p.BRRetestBars)
		stop := extreme - float64(s)*atr*p.StopBufferATR
		minDist := atr * p.MinStopATR
		if math.Abs(entry-stop) < minDist {
			stop = entry - float64(s)*minDist
		}
		if !stopOK(entry, stop, atr, p.MinStopATR, p.MaxStopATR) {
			continue
		}
		risk := math.Abs(entry - stop)
		target := entry + float64(s)*risk*p.BRTargetR
		if p.BRFibExtension > 0 {
			target = entry + float64(s)*ran*p.BRFibExtension
		}
		breakExtreme := level.Price + float64(s)*ran
		if p.BRStopType == "fibRetrace" {
			stop = breakRetestFibStop(level.Price, breakExtreme, atr, s, p.BRStopFibRatio, p.StopBufferATR)
		}
		return setupPlan{
			Side:   s,
			Stop:   stop,
			Target: target,
			Tag:    "DSL-BR",
			Meta: gradeMeta(TradeMeta{
				"setup":         "breakRetest",
				"side":          s.String(),
				"levelKey":      level.Key,
				"levelPrice":    level.Price,
				"retestExtreme": extreme,
				"breakExtreme":  breakExtreme,
			}),
		}, true
	}
	return setupPlan{}, false
}

func breakRetestFibStop(levelPrice, breakExtreme, atr float64, s side, ratio, paddingATR float64) float64 {
	if ratio == 0 {
		ratio = 0.618
	}
	return breakExtreme - (breakExtreme-levelPrice)*ratio - float64(s)*paddingATR*atr
}

func (b *broker) breakRetestLevels(i int, s side) []keyLevel {
	priority := b.params.LevelPriority
	fallbackKey := "PDH"
	if s == sideShort {
		fallbackKey = "PDL"
	}
	if !b.params.LevelPriorityExplicit || len(priority) == 0 {
		priority = []string{fallbackKey}
	}
	weekH, weekL, hasWeek := priorWeekLevels(b.series, i)
	out := make([]keyLevel, 0, len(priority))
	for _, key := range priority {
		level, ok := b.breakRetestLevelByKey(i, s, key, weekH, weekL, hasWeek)
		if ok {
			out = append(out, level)
		}
	}
	if len(out) > 0 {
		return out
	}
	fallback, ok := b.breakRetestLevelByKey(i, s, fallbackKey, weekH, weekL, hasWeek)
	if !ok {
		return nil
	}
	return []keyLevel{fallback}
}

func (b *broker) breakRetestLevelByKey(i int, s side, key string, weekH float64, weekL float64, hasWeek bool) (keyLevel, bool) {
	switch key {
	case "PDH":
		if s == sideLong && isFinite(b.cols.PriorDayH[i]) {
			return keyLevel{Key: key, Price: b.cols.PriorDayH[i]}, true
		}
	case "PDL":
		if s == sideShort && isFinite(b.cols.PriorDayL[i]) {
			return keyLevel{Key: key, Price: b.cols.PriorDayL[i]}, true
		}
	case "WH":
		if s == sideLong && hasWeek {
			return keyLevel{Key: key, Price: weekH}, true
		}
	case "WL":
		if s == sideShort && hasWeek {
			return keyLevel{Key: key, Price: weekL}, true
		}
	case "AH":
		if s == sideLong && isFinite(b.cols.PriorSession.Asia.H[i]) {
			return keyLevel{Key: key, Price: b.cols.PriorSession.Asia.H[i]}, true
		}
	case "AL":
		if s == sideShort && isFinite(b.cols.PriorSession.Asia.L[i]) {
			return keyLevel{Key: key, Price: b.cols.PriorSession.Asia.L[i]}, true
		}
	case "LH":
		if s == sideLong && isFinite(b.cols.PriorSession.London.H[i]) {
			return keyLevel{Key: key, Price: b.cols.PriorSession.London.H[i]}, true
		}
	case "LL":
		if s == sideShort && isFinite(b.cols.PriorSession.London.L[i]) {
			return keyLevel{Key: key, Price: b.cols.PriorSession.London.L[i]}, true
		}
	}
	return keyLevel{}, false
}

func (b *broker) breakRunDistance(i int, level float64, s side, bars int) float64 {
	start := maxInt(0, i-bars+1)
	ran := 0.0
	for j := start; j <= i; j++ {
		d := b.series.H[j] - level
		if s == sideShort {
			d = level - b.series.L[j]
		}
		if d > ran {
			ran = d
		}
	}
	return ran
}

func (b *broker) retestedLevel(i int, level float64, s side, bars int, tol float64) bool {
	start := maxInt(0, i-bars+1)
	for j := start; j <= i; j++ {
		if s == sideLong && b.series.L[j] <= level+tol && b.series.L[j] >= level-tol*2 {
			return true
		}
		if s == sideShort && b.series.H[j] >= level-tol && b.series.H[j] <= level+tol*2 {
			return true
		}
	}
	return false
}

func (b *broker) retestExtreme(i int, s side, bars int) float64 {
	start := maxInt(0, i-bars+1)
	extreme := math.Inf(1)
	if s == sideShort {
		extreme = math.Inf(-1)
	}
	for j := start; j <= i; j++ {
		if s == sideLong {
			extreme = math.Min(extreme, b.series.L[j])
		} else {
			extreme = math.Max(extreme, b.series.H[j])
		}
	}
	return extreme
}
