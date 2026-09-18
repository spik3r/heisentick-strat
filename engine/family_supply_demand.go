package engine

import (
	"math"
	"strconv"
)

type sdZone struct {
	Key             string
	Type            string
	OriginalType    string
	Pattern         string
	Lo              float64
	Hi              float64
	BaseWickLow     float64
	BaseWickHigh    float64
	BaseBodyLow     float64
	BaseBodyHigh    float64
	Start           int
	End             int
	CreatedAt       int
	TouchCount      int
	LastTouchAt     int
	HasLastTouch    bool
	Used            bool
	Flipped         bool
	PendingTouchAt  int
	HasPendingTouch bool
	ChochTouchAt    int
	HasChochTouch   bool
	ChochLevel      float64
	HasChochLevel   bool
}

type sdBaseStats struct {
	Hi             float64
	Lo             float64
	BodyHi         float64
	BodyLo         float64
	MaxBodyATR     float64
	MaxBodyToRange float64
}

func (b *broker) onSupplyDemandBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		b.detectSupplyDemandZones(i)
		b.pruneSupplyDemandZones(i)
		return
	}
	b.detectSupplyDemandZones(i)
	b.pruneSupplyDemandZones(i)
	p := b.params
	if b.hasFlagEntry && i-b.flagLastEntry < p.CooldownBars {
		return
	}
	if len(p.DayTypes) > 0 && !regimeAllowed(b.cols.Regime[i], p.DayTypes) && finiteOrZero(b.cols.ER[i]) > p.DayTypeEREscape {
		return
	}
	if finiteOrZero(b.cols.ER[i]) > p.MaxMovementER {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	lastCreatedAt := math.MaxInt
	lastIdx := len(b.sdZones)
	for {
		idx := b.nextSupplyDemandZone(lastCreatedAt, lastIdx)
		if idx < 0 {
			break
		}
		lastCreatedAt = b.sdZones[idx].CreatedAt
		lastIdx = idx
		stop, target, meta, ok := b.supplyDemandRetestSetup(i, idx)
		if !ok {
			continue
		}
		b.sdZones[idx].Used = true
		s := sideLong
		if b.sdZones[idx].Type == "supply" {
			s = sideShort
		}
		b.enterSetup(i, setupPlan{Side: s, Stop: stop, Target: target, Tag: "DSL-SD:" + b.sdZones[idx].Type, Meta: meta})
		b.flagLastEntry = i
		b.hasFlagEntry = true
		break
	}
}

func (b *broker) nextSupplyDemandZone(lastCreatedAt int, lastIdx int) int {
	best := -1
	for idx := range b.sdZones {
		zone := &b.sdZones[idx]
		if zone.Used {
			continue
		}
		if zone.CreatedAt > lastCreatedAt || (zone.CreatedAt == lastCreatedAt && idx >= lastIdx) {
			continue
		}
		if best < 0 ||
			zone.CreatedAt > b.sdZones[best].CreatedAt ||
			(zone.CreatedAt == b.sdZones[best].CreatedAt && idx < best) {
			best = idx
		}
	}
	return best
}

func (b *broker) detectSupplyDemandZones(i int) {
	p := b.params.SupplyDemand
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 || i < p.ImpulseCandles+p.MinBaseCandles-1 {
		return
	}
	impulseStart := i - p.ImpulseCandles + 1
	baseEnd := impulseStart - 1
	for n := p.MinBaseCandles; n <= p.MaxBaseCandles; n++ {
		baseStart := baseEnd - n + 1
		if baseStart < 0 {
			continue
		}
		stats := b.sdBaseStats(baseStart, baseEnd, atr)
		if stats.Hi-stats.Lo > atr*p.MaxZoneATR {
			continue
		}
		if stats.MaxBodyATR > p.MaxBaseBodyATR || stats.MaxBodyToRange > p.MaxBaseBodyToRange {
			continue
		}
		sourceDir := b.sdSourceDirection(baseStart, atr)
		if p.MinSourceATR > 0 && sourceDir == "flat" {
			continue
		}
		upMove := b.series.C[i] - stats.Hi
		downMove := stats.Lo - b.series.C[i]
		if upMove >= atr*p.ImpulseATR {
			if b.sdDepartureBodyStrength(impulseStart, i, "up") >= p.MinDepartureBodyToRange {
				pattern := sdPatternFor("demand", sourceDir)
				if containsString(p.Patterns, pattern) {
					b.addSupplyDemandZone(i, "demand", baseStart, baseEnd, stats, pattern)
				}
			}
		}
		if downMove >= atr*p.ImpulseATR {
			if b.sdDepartureBodyStrength(impulseStart, i, "down") >= p.MinDepartureBodyToRange {
				pattern := sdPatternFor("supply", sourceDir)
				if containsString(p.Patterns, pattern) {
					b.addSupplyDemandZone(i, "supply", baseStart, baseEnd, stats, pattern)
				}
			}
		}
	}
}

func (b *broker) addSupplyDemandZone(i int, zoneType string, baseStart int, baseEnd int, stats sdBaseStats, pattern string) {
	lo, hi := sdZoneBounds(zoneType, stats, b.params.SupplyDemand.ZoneBounds)
	key := sdZoneKey(zoneType, baseStart, baseEnd, lo, hi)
	for _, zone := range b.sdZones {
		if zone.Key == key {
			return
		}
		if b.params.SupplyDemand.SkipOverlappingZones && !zone.Used && zone.Type == zoneType && hi >= zone.Lo && lo <= zone.Hi {
			return
		}
	}
	b.sdZones = append(b.sdZones, sdZone{
		Key:          key,
		Type:         zoneType,
		OriginalType: zoneType,
		Pattern:      pattern,
		Lo:           lo,
		Hi:           hi,
		BaseWickLow:  stats.Lo,
		BaseWickHigh: stats.Hi,
		BaseBodyLow:  stats.BodyLo,
		BaseBodyHigh: stats.BodyHi,
		Start:        baseStart,
		End:          baseEnd,
		CreatedAt:    i,
		LastTouchAt:  -1,
	})
	if len(b.sdZones) > 80 {
		copy(b.sdZones, b.sdZones[len(b.sdZones)-80:])
		b.sdZones = b.sdZones[:80]
	}
}

func (b *broker) pruneSupplyDemandZones(i int) {
	p := b.params.SupplyDemand
	atr := finiteOrZero(b.cols.ATR[i])
	out := b.sdZones[:0]
	for _, zone := range b.sdZones {
		if zone.Used {
			continue
		}
		if i-zone.CreatedAt > p.MaxZoneAgeCandles {
			continue
		}
		if zone.Type == "demand" && b.series.C[i] < zone.Lo-atr*p.InvalidationATR {
			if !p.FlipBrokenZones || zone.Flipped {
				continue
			}
			zone.Type = "supply"
			zone.Flipped = true
			zone.Used = false
			zone.CreatedAt = i
		}
		if zone.Type == "supply" && b.series.C[i] > zone.Hi+atr*p.InvalidationATR {
			if !p.FlipBrokenZones || zone.Flipped {
				continue
			}
			zone.Type = "demand"
			zone.Flipped = true
			zone.Used = false
			zone.CreatedAt = i
		}
		out = append(out, zone)
	}
	b.sdZones = out
}

func (b *broker) supplyDemandRetestSetup(i int, zoneIdx int) (float64, float64, TradeMeta, bool) {
	p := b.params.SupplyDemand
	zone := &b.sdZones[zoneIdx]
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 || i <= zone.CreatedAt+p.MinWaitCandles {
		return 0, 0, nil, false
	}
	s := sideLong
	if zone.Type == "supply" {
		s = sideShort
	}
	if s == sideLong && !b.params.AllowLong {
		return 0, 0, nil, false
	}
	if s == sideShort && !b.params.AllowShort {
		return 0, 0, nil, false
	}
	if !b.htfAllows(i, s) {
		return 0, 0, nil, false
	}
	chochConfirmed := false
	if p.ChochCandles > 0 && zone.HasChochTouch {
		age := i - zone.ChochTouchAt
		if age > p.ChochCandles {
			zone.HasChochTouch = false
			zone.HasChochLevel = false
			return 0, 0, nil, false
		}
		chochConfirmed = (s == sideLong && b.series.C[i] > zone.ChochLevel) ||
			(s == sideShort && b.series.C[i] < zone.ChochLevel)
		if !chochConfirmed {
			return 0, 0, nil, false
		}
	}
	if !chochConfirmed {
		tol := atr * p.RetestToleranceATR
		touched := (s == sideLong && b.series.L[i] <= zone.Hi+tol && b.series.C[i] > zone.Lo) ||
			(s == sideShort && b.series.H[i] >= zone.Lo-tol && b.series.C[i] < zone.Hi)
		rejected := (s == sideLong && b.series.C[i] > zone.Hi) ||
			(s == sideShort && b.series.C[i] < zone.Lo)
		pendingAge := math.MaxInt
		if zone.HasPendingTouch {
			pendingAge = i - zone.PendingTouchAt
		}
		withinReaction := p.MaxReactionCandles > 0 && pendingAge <= p.MaxReactionCandles
		if !touched {
			if p.RequireRejectionClose && withinReaction {
				if !rejected {
					return 0, 0, nil, false
				}
				zone.HasPendingTouch = false
			} else {
				if zone.HasPendingTouch && pendingAge > p.MaxReactionCandles {
					zone.HasPendingTouch = false
				}
				return 0, 0, nil, false
			}
		}
		if touched && (!zone.HasLastTouch || zone.LastTouchAt != i) && !withinReaction {
			zone.TouchCount++
			zone.LastTouchAt = i
			zone.HasLastTouch = true
		}
		if zone.TouchCount > p.MaxRetests {
			return 0, 0, nil, false
		}
		if p.RequireRejectionClose {
			if !rejected {
				if p.MaxReactionCandles > 0 && !zone.HasPendingTouch {
					zone.PendingTouchAt = i
					zone.HasPendingTouch = true
				}
				return 0, 0, nil, false
			}
			zone.HasPendingTouch = false
		}
		if p.ChochCandles > 0 {
			from := i - maxInt(1, p.ChochLookbackCandles)
			if from < 0 {
				from = 0
			}
			hi, lo := hiLo(b.series, from, i)
			zone.ChochTouchAt = i
			zone.HasChochTouch = true
			zone.ChochLevel = hi
			if s == sideShort {
				zone.ChochLevel = lo
			}
			zone.HasChochLevel = true
			return 0, 0, nil, false
		}
	}
	if p.UseTrigger && !((s == sideLong && longTrigger(b.series, i)) || (s == sideShort && shortTrigger(b.series, i))) {
		return 0, 0, nil, false
	}
	entry := b.series.C[i]
	stop := zone.Lo - atr*b.params.StopBufferATR
	if s == sideShort {
		stop = zone.Hi + atr*b.params.StopBufferATR
	}
	minDist := atr * b.params.MinStopATR
	if math.Abs(entry-stop) < minDist {
		if s == sideLong {
			stop = entry - minDist
		} else {
			stop = entry + minDist
		}
	}
	if !stopOK(entry, stop, atr, b.params.MinStopATR, b.params.MaxStopATR) {
		return 0, 0, nil, false
	}
	risk := math.Abs(entry - stop)
	target := entry + float64(s)*risk*b.params.TargetR
	var chochLevel any
	if zone.HasChochLevel {
		chochLevel = zone.ChochLevel
	}
	return stop, target, gradeMeta(TradeMeta{
		"baseBodyHigh":     zone.BaseBodyHigh,
		"baseBodyLow":      zone.BaseBodyLow,
		"baseEnd":          zone.End,
		"baseStart":        zone.Start,
		"baseWickHigh":     zone.BaseWickHigh,
		"baseWickLow":      zone.BaseWickLow,
		"chochLevel":       chochLevel,
		"flipped":          zone.Flipped,
		"originalZoneType": zone.OriginalType,
		"pattern":          zone.Pattern,
		"setup":            "supplyDemand",
		"side":             s.String(),
		"zoneCreatedAt":    zone.CreatedAt,
		"zoneHigh":         zone.Hi,
		"zoneLow":          zone.Lo,
		"zoneType":         zone.Type,
	}), true
}

func (b *broker) sdBaseStats(from int, to int, atr float64) sdBaseStats {
	hi := math.Inf(-1)
	lo := math.Inf(1)
	bodyHi := math.Inf(-1)
	bodyLo := math.Inf(1)
	maxBody := 0.0
	maxBodyToRange := 0.0
	for j := from; j <= to; j++ {
		hi = math.Max(hi, b.series.H[j])
		lo = math.Min(lo, b.series.L[j])
		barBodyHi := math.Max(b.series.O[j], b.series.C[j])
		barBodyLo := math.Min(b.series.O[j], b.series.C[j])
		bodyHi = math.Max(bodyHi, barBodyHi)
		bodyLo = math.Min(bodyLo, barBodyLo)
		body := math.Abs(b.series.C[j] - b.series.O[j])
		maxBody = math.Max(maxBody, body)
		rng := b.series.H[j] - b.series.L[j]
		if rng > 0 {
			maxBodyToRange = math.Max(maxBodyToRange, body/rng)
		}
	}
	maxBodyATR := math.Inf(1)
	if atr != 0 {
		maxBodyATR = maxBody / atr
	}
	return sdBaseStats{Hi: hi, Lo: lo, BodyHi: bodyHi, BodyLo: bodyLo, MaxBodyATR: maxBodyATR, MaxBodyToRange: maxBodyToRange}
}

func (b *broker) sdSourceDirection(baseStart int, atr float64) string {
	p := b.params.SupplyDemand
	sourceStart := baseStart - p.SourceCandles
	if sourceStart < 0 || p.SourceCandles <= 0 {
		return "flat"
	}
	move := b.series.C[baseStart-1] - b.series.C[sourceStart]
	threshold := atr * p.MinSourceATR
	if move >= threshold {
		return "up"
	}
	if move <= -threshold {
		return "down"
	}
	return "flat"
}

func (b *broker) sdDepartureBodyStrength(from int, to int, dir string) float64 {
	best := 0.0
	for j := from; j <= to; j++ {
		rng := b.series.H[j] - b.series.L[j]
		if rng <= 0 {
			continue
		}
		aligned := (dir == "up" && b.series.C[j] > b.series.O[j]) ||
			(dir == "down" && b.series.C[j] < b.series.O[j])
		if aligned {
			best = math.Max(best, math.Abs(b.series.C[j]-b.series.O[j])/rng)
		}
	}
	return best
}

func sdPatternFor(zoneType string, sourceDir string) string {
	if zoneType == "demand" {
		if sourceDir == "down" {
			return "DBR"
		}
		return "RBR"
	}
	if sourceDir == "up" {
		return "RBD"
	}
	return "DBD"
}

func sdZoneBounds(zoneType string, stats sdBaseStats, mode string) (float64, float64) {
	if mode == "body" {
		return stats.BodyLo, stats.BodyHi
	}
	if mode == "proximalDistal" {
		if zoneType == "demand" {
			return stats.Lo, stats.BodyHi
		}
		return stats.BodyLo, stats.Hi
	}
	return stats.Lo, stats.Hi
}

func sdZoneKey(zoneType string, start int, end int, lo float64, hi float64) string {
	return zoneType + ":" + strconv.Itoa(start) + ":" + strconv.Itoa(end) + ":" + strconv.Itoa(int(math.Round(lo*10))) + ":" + strconv.Itoa(int(math.Round(hi*10)))
}
