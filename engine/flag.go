package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	regimeTrending = int8(1)
	regimeRanging  = int8(2)
	regimeChoppy   = int8(3)
	trendUp        = int8(1)
	trendDown      = int8(2)
	phaseLunch     = int8(3)
	// HTF projection codes. A nil htfTrend slice means HTF was not requested
	// for the run; within a non-nil slice, 0 means the completed HTF bar is
	// unavailable (fail closed) and trendFlat is a genuine neutral completed bar.
	htfUnavailable = int8(0)
	trendFlat      = int8(4)
)

type flagSetup struct {
	Stop   float64
	Target float64
	Meta   TradeMeta
}

func (b *broker) onFlagBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasFlagEntry && i-b.flagLastEntry < p.CooldownBars {
		return
	}
	if !inFlagTradeWindow(b.series.T[i], p, 45) {
		return
	}
	if p.AvoidLunchBreakouts && b.cols.SessionPhase[i] == phaseLunch {
		return
	}
	if b.cols.Regime[i] != regimeTrending {
		return
	}
	er := b.cols.ER[i]
	if er < p.MinER || er > p.MaxER {
		return
	}
	if p.UseHTFBias {
		// Fail closed when HTF bias is requested but no completed bar is
		// available (absent series or a gap); a genuine flat bar does not oppose.
		if len(b.htfTrend) == 0 {
			return
		}
		htfTrend := b.htfTrend[i]
		if htfTrend == htfUnavailable {
			return
		}
		if htfTrend != trendFlat && htfTrend != b.cols.TrendDir[i] {
			return
		}
	}
	if b.cols.TrendDir[i] == trendUp && !p.AllowLong {
		return
	}
	if b.cols.TrendDir[i] == trendDown && !p.AllowShort {
		return
	}

	var setup flagSetup
	var ok bool
	var s side
	switch b.cols.TrendDir[i] {
	case trendUp:
		setup, ok = b.longFlag(i)
		s = sideLong
	case trendDown:
		setup, ok = b.shortFlag(i)
		s = sideShort
	default:
		return
	}
	if !ok {
		return
	}
	setup = applyFlagCompiledStop(setup, finiteOrZero(b.cols.ATR[i]), s, p)
	b.enter(i, s, setup)
	b.flagLastEntry = i
	b.hasFlagEntry = true
}

func applyFlagCompiledStop(setup flagSetup, atr float64, s side, p flagParams) flagSetup {
	if p.StopType != "fibRetrace" {
		return setup
	}
	poleHi, hiOK := setup.Meta["poleHi"].(float64)
	poleLo, loOK := setup.Meta["poleLo"].(float64)
	if !hiOK || !loOK {
		return setup
	}
	ratio := p.StopFibRatio
	if ratio == 0 {
		ratio = 0.618
	}
	if s == sideLong {
		setup.Stop = poleHi - (poleHi-poleLo)*ratio - p.StopBufferATR*atr
	} else {
		setup.Stop = poleLo + (poleHi-poleLo)*ratio + p.StopBufferATR*atr
	}
	return setup
}

func (b *broker) longFlag(i int) (flagSetup, bool) {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	pdh := b.cols.PriorDayH[i]
	if !isFinite(pdh) || !brokeRecently(b.series, i, pdh, sideLong, p.FreshBreakBars) {
		return flagSetup{}, false
	}
	breakIdx := breakIndexRecently(b.series, i, pdh, sideLong, p.FreshBreakBars)
	if !longTrigger(b.series, i) {
		return flagSetup{}, false
	}
	for length := p.MinFlagBars; length <= p.MaxFlagBars; length++ {
		flagStart := i - length + 1
		poleStart := flagStart - p.PoleBars
		if poleStart < 0 {
			continue
		}
		poleHi, poleLo := hiLo(b.series, poleStart, flagStart-1)
		flagHi, flagLo := hiLo(b.series, flagStart, i)
		poleRange := poleHi - poleLo
		if poleRange < atr*p.PoleATR {
			continue
		}
		if p.ImpulseCloseLocationMin != 0 && closeLocation(b.series, flagStart-1, sideLong) < p.ImpulseCloseLocationMin {
			continue
		}
		poleNet := b.series.C[flagStart-1] - b.series.O[poleStart]
		if poleNet < poleRange*p.MinPoleNetFrac {
			continue
		}
		if poleHi < pdh {
			continue
		}
		if p.FirstPullbackOnly && breakIdx >= 0 && flagStart-breakIdx > p.MaxFlagBars {
			continue
		}
		if (poleHi-flagLo)/poleRange > p.MaxFlagDepth {
			continue
		}
		if flagLo < pdh-atr*p.LevelToleranceATR {
			continue
		}
		if p.PullbackVolatilityContract && (flagHi-flagLo)/float64(length) >= poleRange/float64(p.PoleBars) {
			continue
		}
		if (b.series.C[i]-flagLo)/math.Max(0.00001, flagHi-flagLo) < p.FlagCloseFrac {
			continue
		}
		entry := b.series.C[i]
		stop := flagLo - atr*p.StopBufferATR
		if entry-stop < atr*p.MinStopATR {
			stop = entry - atr*p.MinStopATR
		}
		if !stopOK(entry, stop, atr, p.MinStopATR, math.Inf(1)) {
			continue
		}
		risk := entry - stop
		return flagSetup{
			Stop:   stop,
			Target: entry + math.Min(poleRange, risk*p.TargetR),
			Meta: flagTradeMeta(TradeMeta{
				"setup":      "flag",
				"side":       "long",
				"flagStart":  flagStart,
				"flagEnd":    i,
				"flagHi":     flagHi,
				"flagLo":     flagLo,
				"poleStart":  poleStart,
				"poleEnd":    flagStart - 1,
				"poleHi":     poleHi,
				"poleLo":     poleLo,
				"levelKey":   "PDH",
				"levelPrice": pdh,
			}),
		}, true
	}
	return flagSetup{}, false
}

func (b *broker) shortFlag(i int) (flagSetup, bool) {
	p := b.params
	atr := finiteOrZero(b.cols.ATR[i])
	pdl := b.cols.PriorDayL[i]
	if !isFinite(pdl) || !brokeRecently(b.series, i, pdl, sideShort, p.FreshBreakBars) {
		return flagSetup{}, false
	}
	breakIdx := breakIndexRecently(b.series, i, pdl, sideShort, p.FreshBreakBars)
	if !shortTrigger(b.series, i) {
		return flagSetup{}, false
	}
	for length := p.MinFlagBars; length <= p.MaxFlagBars; length++ {
		flagStart := i - length + 1
		poleStart := flagStart - p.PoleBars
		if poleStart < 0 {
			continue
		}
		poleHi, poleLo := hiLo(b.series, poleStart, flagStart-1)
		flagHi, flagLo := hiLo(b.series, flagStart, i)
		poleRange := poleHi - poleLo
		if poleRange < atr*p.PoleATR {
			continue
		}
		if p.ImpulseCloseLocationMin != 0 && closeLocation(b.series, flagStart-1, sideShort) < p.ImpulseCloseLocationMin {
			continue
		}
		poleNet := b.series.O[poleStart] - b.series.C[flagStart-1]
		if poleNet < poleRange*p.MinPoleNetFrac {
			continue
		}
		if poleLo > pdl {
			continue
		}
		if p.FirstPullbackOnly && breakIdx >= 0 && flagStart-breakIdx > p.MaxFlagBars {
			continue
		}
		if (flagHi-poleLo)/poleRange > p.MaxFlagDepth {
			continue
		}
		if flagHi > pdl+atr*p.LevelToleranceATR {
			continue
		}
		if p.PullbackVolatilityContract && (flagHi-flagLo)/float64(length) >= poleRange/float64(p.PoleBars) {
			continue
		}
		if (flagHi-b.series.C[i])/math.Max(0.00001, flagHi-flagLo) < p.FlagCloseFrac {
			continue
		}
		entry := b.series.C[i]
		stop := flagHi + atr*p.StopBufferATR
		if stop-entry < atr*p.MinStopATR {
			stop = entry + atr*p.MinStopATR
		}
		if !stopOK(entry, stop, atr, p.MinStopATR, math.Inf(1)) {
			continue
		}
		risk := stop - entry
		return flagSetup{
			Stop:   stop,
			Target: entry - math.Min(poleRange, risk*p.TargetR),
			Meta: flagTradeMeta(TradeMeta{
				"setup":      "flag",
				"side":       "short",
				"flagStart":  flagStart,
				"flagEnd":    i,
				"flagHi":     flagHi,
				"flagLo":     flagLo,
				"poleStart":  poleStart,
				"poleEnd":    flagStart - 1,
				"poleHi":     poleHi,
				"poleLo":     poleLo,
				"levelKey":   "PDL",
				"levelPrice": pdl,
			}),
		}, true
	}
	return flagSetup{}, false
}

func (b *broker) moveStopToBreakeven(i int) {
	p := b.params
	if p.BreakevenR == 0 || !b.hasPosition {
		return
	}
	pos := &b.position
	risk := math.Abs(pos.Entry - pos.SL)
	if risk <= 0 {
		return
	}
	sign := float64(pos.Side)
	favorable := (b.series.C[i] - pos.Entry) * sign
	if favorable < risk*p.BreakevenR {
		return
	}
	b.tightenStop(pos.Entry + sign*finiteOrZero(b.cols.ATR[i])*p.BreakevenOffsetATR)
}

func (b *broker) applyPartialManagement(i int) {
	p := b.params.Partial
	if !p.Enabled || !b.hasPosition || b.position.Size == 0 || b.position.PartialTaken || p.Fraction <= 0 {
		return
	}
	pos := &b.position
	risk := math.Abs(pos.Entry - pos.SL)
	if risk <= 0 || p.TriggerR <= 0 {
		return
	}
	sign := float64(pos.Side)
	favorable := (b.series.C[i] - pos.Entry) * sign
	if favorable < risk*p.TriggerR {
		return
	}
	if pos.Size < 0 {
		if p.MoveBreakeven {
			b.tightenStop(pos.Entry)
		}
		pos.PartialTaken = true
		return
	}
	fraction := math.Min(1, math.Max(0, p.Fraction))
	if fraction >= 1 {
		b.closePosition(b.series.C[i], i, "partial")
		return
	}
	closedSize := pos.Size * fraction
	px := b.series.C[i] - sign*b.slippageAt(b.series.C[i])
	points := (px - pos.Entry) * sign
	pnl := points*closedSize - b.costs.FeePerUnit*closedSize
	b.realized += pnl
	b.trades = append(b.trades, Trade{
		Side:       pos.Side.String(),
		Entry:      pos.Entry,
		Exit:       px,
		SL:         pos.SL,
		TP:         pos.TP,
		InitialSL:  pos.InitialSL,
		InitialTP:  pos.InitialTP,
		Meta:       pos.Meta,
		Partial:    true,
		Size:       closedSize,
		EntryIndex: pos.EntryIndex,
		ExitIndex:  i,
		EntryT:     pos.EntryT,
		ExitT:      b.series.T[i],
		Points:     points,
		PnL:        pnl,
		Reason:     "partial",
		Tag:        pos.Tag,
	})
	pos.Size -= closedSize
	pos.PartialTaken = true
	if p.MoveBreakeven {
		b.tightenStop(pos.Entry)
	}
}

func (b *broker) applyTrail(i int) {
	b.applyTrailWith(i, b.params.Trail)
}

func (b *broker) applyTrailWith(i int, p trailParams) {
	if p.ATR <= 0 || !b.hasPosition {
		return
	}
	pos := &b.position
	risk := math.Abs(pos.Entry - pos.SL)
	if risk <= 0 {
		return
	}
	sign := float64(pos.Side)
	favorable := (b.series.C[i] - pos.Entry) * sign
	if favorable < risk*p.TriggerR {
		return
	}
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return
	}
	b.tightenStop(b.series.C[i] - sign*p.ATR*atr)
}

func (b *broker) exitAfterBars(i int) {
	if !b.hasPosition || b.params.MaxHoldBars == 0 {
		return
	}
	if float64(i-b.position.EntryIndex) < b.params.MaxHoldBars {
		return
	}
	b.closePosition(b.series.C[i], i, "time")
}

func longTrigger(series marketdata.Series, i int) bool {
	return isBullPin(series, i) || isBullEngulf(series, i) || (isOutsideBar(series, i) && series.C[i] > series.O[i])
}

func shortTrigger(series marketdata.Series, i int) bool {
	return isBearPin(series, i) || isBearEngulf(series, i) || (isOutsideBar(series, i) && series.C[i] < series.O[i])
}

func isBullPin(series marketdata.Series, i int) bool {
	body := math.Abs(series.C[i] - series.O[i])
	rng := rangeSize(series, i)
	lower := math.Min(series.O[i], series.C[i]) - series.L[i]
	upper := series.H[i] - math.Max(series.O[i], series.C[i])
	return lower >= rng*0.45 && lower >= body*1.5 && upper <= rng*0.35 && series.C[i] >= series.O[i]
}

func isBearPin(series marketdata.Series, i int) bool {
	body := math.Abs(series.C[i] - series.O[i])
	rng := rangeSize(series, i)
	lower := math.Min(series.O[i], series.C[i]) - series.L[i]
	upper := series.H[i] - math.Max(series.O[i], series.C[i])
	return upper >= rng*0.45 && upper >= body*1.5 && lower <= rng*0.35 && series.C[i] <= series.O[i]
}

func isBullEngulf(series marketdata.Series, i int) bool {
	if i < 1 {
		return false
	}
	return series.C[i-1] < series.O[i-1] && series.C[i] > series.O[i] &&
		series.O[i] <= series.C[i-1] && series.C[i] >= series.O[i-1]
}

func isBearEngulf(series marketdata.Series, i int) bool {
	if i < 1 {
		return false
	}
	return series.C[i-1] > series.O[i-1] && series.C[i] < series.O[i] &&
		series.O[i] >= series.C[i-1] && series.C[i] <= series.O[i-1]
}

func isOutsideBar(series marketdata.Series, i int) bool {
	if i < 1 {
		return false
	}
	return series.H[i] > series.H[i-1] && series.L[i] < series.L[i-1]
}

func rangeSize(series marketdata.Series, i int) float64 {
	return math.Max(0.00001, series.H[i]-series.L[i])
}

func closeLocation(series marketdata.Series, i int, s side) float64 {
	rng := math.Max(0, series.H[i]-series.L[i])
	if rng == 0 {
		return 0.5
	}
	if s == sideShort {
		return (series.H[i] - series.C[i]) / rng
	}
	return (series.C[i] - series.L[i]) / rng
}

func brokeRecently(series marketdata.Series, i int, level float64, s side, bars int) bool {
	start := maxInt(1, i-bars+1)
	for j := start; j <= i; j++ {
		if s == sideLong && series.C[j-1] <= level && series.C[j] > level {
			return true
		}
		if s == sideShort && series.C[j-1] >= level && series.C[j] < level {
			return true
		}
	}
	return false
}

func breakIndexRecently(series marketdata.Series, i int, level float64, s side, bars int) int {
	start := maxInt(1, i-bars+1)
	for j := i; j >= start; j-- {
		if s == sideLong && series.C[j-1] <= level && series.C[j] > level {
			return j
		}
		if s == sideShort && series.C[j-1] >= level && series.C[j] < level {
			return j
		}
	}
	return -1
}

func hiLo(series marketdata.Series, from int, to int) (float64, float64) {
	hi := math.Inf(-1)
	lo := math.Inf(1)
	for j := maxInt(0, from); j <= to; j++ {
		hi = math.Max(hi, series.H[j])
		lo = math.Min(lo, series.L[j])
	}
	return hi, lo
}

func stopOK(entry float64, stop float64, atr float64, minATR float64, maxATR float64) bool {
	if !isFinite(stop) || atr == 0 {
		return false
	}
	dist := math.Abs(entry - stop)
	return dist >= atr*minATR && dist <= atr*maxATR
}

func finiteOrZero(v float64) float64 {
	if !isFinite(v) {
		return 0
	}
	return v
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
