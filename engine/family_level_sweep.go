package engine

import (
	"fmt"
	"math"
)

type levelRef struct {
	Key   string
	Price float64
}

func (b *broker) onLevelSweepBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if !b.marketGatesOK(i) {
		return
	}
	if b.hasLSEntry && i-b.lsLastEntry < p.CooldownBars {
		return
	}
	for _, edge := range []string{"high", "low"} {
		s := sideShort
		if edge == "low" {
			s = sideLong
		}
		if s == sideLong && !p.AllowLong {
			continue
		}
		if s == sideShort && !p.AllowShort {
			continue
		}
		setup, seenKey, ok := b.levelSweepSetup(i, edge, s)
		if !ok {
			continue
		}
		theme, themeOK := b.dayThemeAdmission(i, s)
		if !themeOK {
			continue
		}
		setup.Meta = annotateDayTheme(setup.Meta, theme)
		day := localDayKey(b.series.T[i])
		if b.seen.ls.seen(day, seenKey) {
			continue
		}
		b.seen.ls.add(day, seenKey)
		levelKey, _ := setup.Meta["levelKey"].(string)
		grade, _ := setup.Meta["grade"].(string)
		setup.Tag = "DSL:" + levelKey + ":" + grade
		if p.EntryMode == "limit" {
			b.enterLimit(i, rbfRangeEdge(setup, s), setup, p.EntryExpireCandles)
		} else {
			b.enterSetup(i, setup)
		}
		b.lsLastEntry = i
		b.hasLSEntry = true
		break
	}
}

func rbfRangeEdge(setup setupPlan, s side) float64 {
	key := "rangeLo"
	if s == sideShort {
		key = "rangeHi"
	}
	limit, _ := setup.Meta[key].(float64)
	return limit
}

func (b *broker) levelSweepSetup(i int, edge string, s side) (setupPlan, string, bool) {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return setupPlan{}, "", false
	}
	r, ok := b.currentRBFRange(i)
	if !ok {
		return setupPlan{}, "", false
	}
	edgePrice := r.High
	if edge == "low" {
		edgePrice = r.Low
	}
	key, ok := b.nearestConfiguredLevel(i, edgePrice, p.LevelPriority, p.LevelDistanceATR)
	if p.FBRequireLevel && !ok {
		return setupPlan{}, "", false
	}
	if !b.levelSweptAndReclaimed(i, edge, edgePrice) {
		return setupPlan{}, "", false
	}
	if p.UseHTFBias && !b.htfAllows(i, s) {
		return setupPlan{}, "", false
	}
	if !b.sustainedApproach(i, edgePrice, s, 3) {
		return setupPlan{}, "", false
	}
	if !triggerOK(b.series, i, s, true) {
		return setupPlan{}, "", false
	}
	if !candleQualityOK(b.series, i, s, p.TailRejectionMin, p.CloseLocationMin) {
		return setupPlan{}, "", false
	}
	stop := recentExtreme(b.series, i, p.StopLookbackCandles, s)
	if s == sideLong {
		stop -= atr * p.StopBufferATR
	} else {
		stop += atr * p.StopBufferATR
	}
	if !stopOK(b.series.C[i], stop, atr, p.MinStopATR, p.MaxStopATR) {
		return setupPlan{}, "", false
	}
	entry := b.series.C[i]
	risk := math.Abs(entry - stop)
	target, targetOK := b.levelSweepTarget(i, r, s, entry, risk)
	if !targetOK {
		return setupPlan{}, "", false
	}
	rangeStart := maxInt(0, i-p.RangeActiveWithinCandles-3-3)
	approachHi, approachLo := hiLo(b.series, rangeStart, i)
	levelKey := "range"
	levelPrice := edgePrice
	if ok {
		levelKey = key.Key
		levelPrice = key.Price
	}
	grade := p.LevelSweepHighGrade
	if edge == "low" {
		grade = p.LevelSweepLowGrade
	}
	meta := gradeMeta(TradeMeta{
		"approachHi":      approachHi,
		"approachLo":      approachLo,
		"edge":            edge,
		"grade":           grade,
		"levelKey":        levelKey,
		"levelPrice":      levelPrice,
		"rangeEnd":        i,
		"rangeHi":         r.High,
		"rangeLo":         r.Low,
		"rangeStart":      rangeStart,
		"setup":           "rangeSweepReclaim",
		"setupAgeCandles": r.SinceActive,
		"side":            s.String(),
		"source":          "range",
	})
	return setupPlan{Side: s, Stop: stop, Target: target, Meta: meta}, fmt.Sprintf("%s:%.0f", s.String(), edgePrice*10), true
}

func (b *broker) levelSweepTarget(i int, r rbfRange, s side, entry float64, risk float64) (float64, bool) {
	p := b.params
	target := b.levelSweepEdgeTarget(i, r, s)
	targetR := math.Abs(target-entry) / risk
	edgeTargetOK := (s == sideLong && target > entry) || (s == sideShort && target < entry)
	if !edgeTargetOK || targetR < p.MinTargetR {
		target = entry + float64(s)*risk*p.TargetR
		targetR = math.Abs(target-entry) / risk
	}
	return target, targetR >= p.MinTargetR
}

func (b *broker) levelSweepEdgeTarget(i int, r rbfRange, s side) float64 {
	target := r.Low
	if s == sideLong {
		target = r.High
	}
	if b.params.TargetEdge != "channel" || i < 0 || i >= len(b.cols.LastChannel.SinceActive) {
		return target
	}
	age := int(b.cols.LastChannel.SinceActive[i])
	if age < 0 || age > b.params.ChannelBreakHold.ActiveWithinCandles {
		return target
	}
	if s == sideLong {
		return b.cols.LastChannel.Upper[i]
	}
	return b.cols.LastChannel.Lower[i]
}

func (b *broker) levelSweptAndReclaimed(i int, edge string, edgePrice float64) bool {
	atr := finiteOrZero(b.cols.ATR[i])
	buffer := atr * 0.1
	from := maxInt(0, i-3+1)
	swept := false
	for j := from; j <= i; j++ {
		if edge == "high" && b.series.H[j] > edgePrice+buffer {
			swept = true
		}
		if edge == "low" && b.series.L[j] < edgePrice-buffer {
			swept = true
		}
	}
	return swept && b.series.C[i] < b.cols.LastRange.High[i] && b.series.C[i] > b.cols.LastRange.Low[i]
}

func (b *broker) sustainedApproach(i int, level float64, s side, bars int) bool {
	if i < bars {
		return false
	}
	progress := 0
	for j := i - bars + 1; j <= i; j++ {
		if s == sideShort && b.series.C[j] >= b.series.C[j-1] {
			progress++
		}
		if s == sideLong && b.series.C[j] <= b.series.C[j-1] {
			progress++
		}
	}
	prevDist := math.Abs(b.series.C[i-bars] - level)
	nowDist := math.Abs(b.series.C[i] - level)
	return progress >= bars-1 && nowDist < prevDist
}

func (b *broker) nearestConfiguredLevel(i int, refPrice float64, priority []string, maxDistanceATR float64) (levelRef, bool) {
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 || len(priority) == 0 {
		return levelRef{}, false
	}
	for _, key := range priority {
		level, ok := b.keyLevel(i, key, refPrice)
		if !ok {
			continue
		}
		if math.Abs(level.Price-refPrice) <= atr*maxDistanceATR {
			return level, true
		}
	}
	return levelRef{}, false
}

func (b *broker) keyLevel(i int, key string, refPrice float64) (levelRef, bool) {
	switch key {
	case "VWAP":
		if isFinite(b.cols.VWAP[i]) {
			return levelRef{Key: key, Price: b.cols.VWAP[i]}, true
		}
	case "CAM_R3":
		return finiteLevel(key, b.cols.Camarilla.R3[i])
	case "CAM_R4":
		return finiteLevel(key, b.cols.Camarilla.R4[i])
	case "CAM_S3":
		return finiteLevel(key, b.cols.Camarilla.S3[i])
	case "CAM_S4":
		return finiteLevel(key, b.cols.Camarilla.S4[i])
	case "PDH":
		return finiteLevel(key, b.cols.PriorDayH[i])
	case "PDL":
		return finiteLevel(key, b.cols.PriorDayL[i])
	case "PDO":
		return finiteLevel(key, b.cols.PriorDayO[i])
	case "PDC":
		return finiteLevel(key, b.cols.PriorDayC[i])
	case "DO":
		return finiteLevel(key, b.cols.DayOpen[i])
	case "DH":
		return finiteLevel(key, b.cols.DayHigh[i])
	case "DL":
		return finiteLevel(key, b.cols.DayLow[i])
	case "WH":
		hi, _, ok := priorWeekLevels(b.series, i)
		if ok {
			return levelRef{Key: key, Price: hi}, true
		}
	case "WL":
		_, lo, ok := priorWeekLevels(b.series, i)
		if ok {
			return levelRef{Key: key, Price: lo}, true
		}
	case "AH":
		return finiteLevel(key, b.cols.PriorSession.Asia.H[i])
	case "AL":
		return finiteLevel(key, b.cols.PriorSession.Asia.L[i])
	case "LH":
		return finiteLevel(key, b.cols.PriorSession.London.H[i])
	case "LL":
		return finiteLevel(key, b.cols.PriorSession.London.L[i])
	case "NH":
		return finiteLevel(key, b.cols.PriorSession.NY.H[i])
	case "NL":
		return finiteLevel(key, b.cols.PriorSession.NY.L[i])
	}
	return levelRef{}, false
}

func finiteLevel(key string, price float64) (levelRef, bool) {
	if !isFinite(price) {
		return levelRef{}, false
	}
	return levelRef{Key: key, Price: price}, true
}
