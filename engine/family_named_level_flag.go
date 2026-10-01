package engine

import (
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/dsl"
)

type namedLevelFlagParams struct {
	LevelPriority                          []string
	ImpulseATR, BreakDistanceATR           float64
	MaxBars, MinBars                       int
	StopPaddingATR, MinStopATR, MaxStopATR float64
	TargetMode                             string
	BodyATR                                float64
	MinStopPoints                          float64
	TargetR                                float64
}

type namedLevelFlagState struct {
	Key                        string
	Level, Mid, High, Low, ATR float64
	Side                       side
	Index, Bars                int
	Pullback                   bool
}

func namedLevelFlagParamsFromConfig(cfg dsl.Config) namedLevelFlagParams {
	p := mapValue(cfg, "namedLevelFlag")
	stop := mapValue(cfg, "stop")
	return namedLevelFlagParams{
		LevelPriority: stringSliceValue(cfg["levelPriority"]),
		ImpulseATR:    numberValue(p, "impulseAtr", .8), BreakDistanceATR: numberValue(p, "breakDistanceAtr", .1),
		MaxBars: intValue(p, "maxBars", 10), MinBars: intValue(p, "minBars", 2),
		StopPaddingATR: numberValue(stop, "paddingAtr", .2), MinStopATR: numberValue(stop, "minAtr", .4),
		MaxStopATR: numberValue(stop, "maxAtr", 2), TargetMode: stringValue(p, "targetMode", "fixed2R"),
		BodyATR:       numberValue(p, "breakoutBodyAtr", 0),
		MinStopPoints: numberValue(p, "minStopPoints", 0),
		TargetR:       numberValue(mapValue(cfg, "target"), "nlfR", 2),
	}
}

func (b *broker) namedLevelFlagPrices(i int) map[string]float64 {
	prices := make(map[string]float64, len(b.params.NamedLevelFlag.LevelPriority))
	var weekHigh, weekLow float64
	weekLoaded, weekPresent := false, false
	for _, key := range b.params.NamedLevelFlag.LevelPriority {
		var price float64
		var ok bool
		switch key {
		case "PDH":
			price, ok = namedLevelFlagAt(b.cols.PriorDayH, i)
		case "PDL":
			price, ok = namedLevelFlagAt(b.cols.PriorDayL, i)
		case "PDO":
			price, ok = namedLevelFlagAt(b.cols.PriorDayO, i)
		case "PDC":
			price, ok = namedLevelFlagAt(b.cols.PriorDayC, i)
		case "DO":
			price, ok = namedLevelFlagAt(b.cols.DayOpen, i)
		case "DH":
			price, ok = namedLevelFlagAt(b.cols.DayHigh, i)
		case "DL":
			price, ok = namedLevelFlagAt(b.cols.DayLow, i)
		case "AH":
			price, ok = namedLevelFlagAt(b.cols.PriorSession.Asia.H, i)
		case "AL":
			price, ok = namedLevelFlagAt(b.cols.PriorSession.Asia.L, i)
		case "LH":
			price, ok = namedLevelFlagAt(b.cols.PriorSession.London.H, i)
		case "LL":
			price, ok = namedLevelFlagAt(b.cols.PriorSession.London.L, i)
		case "NH":
			price, ok = namedLevelFlagAt(b.cols.PriorSession.NY.H, i)
		case "NL":
			price, ok = namedLevelFlagAt(b.cols.PriorSession.NY.L, i)
		case "WH", "WL":
			if !weekLoaded {
				weekHigh, weekLow, weekPresent = priorWeekLevels(b.series, i)
				weekLoaded = true
			}
			if key == "WH" {
				price = weekHigh
			} else {
				price = weekLow
			}
			ok = weekPresent
		case "CAM_R3":
			price, ok = namedLevelFlagAt(b.cols.Camarilla.R3, i)
		case "CAM_R4":
			price, ok = namedLevelFlagAt(b.cols.Camarilla.R4, i)
		case "CAM_S3":
			price, ok = namedLevelFlagAt(b.cols.Camarilla.S3, i)
		case "CAM_S4":
			price, ok = namedLevelFlagAt(b.cols.Camarilla.S4, i)
		case "VWAP":
			price, ok = namedLevelFlagAt(b.cols.VWAP, i)
		}
		if ok && isFinite(price) {
			prices[key] = price
		}
	}
	return prices
}

func namedLevelFlagAt(values []float64, i int) (float64, bool) {
	if i < 0 || i >= len(values) {
		return 0, false
	}
	return values[i], true
}

func (b *broker) onNamedLevelFlagBar(i int) {
	// The research adapter does not refresh level snapshots while a trade is
	// open. Keep that cache boundary so setup eligibility remains identical.
	if b.hasPosition {
		b.exitAfterBars(i)
		return
	}
	day := floorDivInt64(int64(b.series.T[i]), 24*60*60*1000)
	if b.seen.nlf.day != day {
		b.nlfState = nil
	}
	prices := b.namedLevelFlagPrices(i)
	previous := b.nlfPrices
	b.nlfPrices = prices
	if !b.marketGatesOK(i) {
		b.nlfState = nil
		return
	}
	atr := finiteOrZero(b.cols.ATR[i])
	if atr <= 0 {
		return
	}
	if p := b.nlfState; p != nil {
		age := i - p.Index
		level, live := prices[p.Key]
		if age < 1 {
			return
		}
		bad := age > b.params.NamedLevelFlag.MaxBars || !live || level != p.Level
		if p.Side == sideLong && (b.series.C[i] < p.Level || b.series.L[i] < p.Mid) {
			bad = true
		}
		if p.Side == sideShort && (b.series.C[i] > p.Level || b.series.H[i] > p.Mid) {
			bad = true
		}
		if bad {
			b.nlfState = nil
			return
		}
		breakPrice := p.High
		if p.Side == sideShort {
			breakPrice = p.Low
		}
		if p.Bars >= b.params.NamedLevelFlag.MinBars && p.Pullback &&
			(p.Side == sideLong && b.series.C[i] > breakPrice || p.Side == sideShort && b.series.C[i] < breakPrice) {
			b.nlfState = nil
			if b.params.NamedLevelFlag.BodyATR > 0 && math.Abs(b.series.C[i]-b.series.O[i]) < b.params.NamedLevelFlag.BodyATR*atr {
				return
			}
			stop := p.Low - b.params.NamedLevelFlag.StopPaddingATR*atr
			if p.Side == sideShort {
				stop = p.High + b.params.NamedLevelFlag.StopPaddingATR*atr
			}
			if !stopOK(b.series.C[i], stop, atr, b.params.NamedLevelFlag.MinStopATR, b.params.NamedLevelFlag.MaxStopATR) {
				return
			}
			knownKey, knownPrice, known := b.nlfKnownRoom(i, p.Side, b.series.C[i], prices, previous)
			if b.params.NamedLevelFlag.TargetMode == "knownLevel2R" && !known {
				return
			}
			risk := math.Abs(b.series.C[i] - stop)
			if b.params.NamedLevelFlag.MinStopPoints > 0 && risk < b.params.NamedLevelFlag.MinStopPoints {
				return
			}
			meta := TradeMeta{"setup": "namedLevelFlag", "side": p.Side.String(), "levelKey": p.Key, "levelPrice": p.Level, "targetMode": b.params.NamedLevelFlag.TargetMode}
			if known {
				meta["targetLevelKey"] = knownKey
				meta["targetLevelPrice"] = knownPrice
			}
			b.enterSetup(i, setupPlan{Side: p.Side, Stop: stop, Target: b.series.C[i] + float64(p.Side)*risk*b.params.NamedLevelFlag.TargetR,
				Tag: "DSL-NLF:" + p.Key, Meta: meta})
			return
		}
		p.Pullback = p.Pullback || p.Side == sideLong && b.series.C[i] < b.series.O[i] || p.Side == sideShort && b.series.C[i] > b.series.O[i]
		p.High = math.Max(p.High, b.series.H[i])
		p.Low = math.Min(p.Low, b.series.L[i])
		p.Bars++
		return
	}
	if i == 0 || math.Abs(b.series.C[i]-b.series.O[i]) < b.params.NamedLevelFlag.ImpulseATR*atr {
		return
	}
	sideNow := sideLong
	if b.series.C[i] < b.series.O[i] {
		sideNow = sideShort
	}
	if sideNow == sideLong && !b.params.AllowLong || sideNow == sideShort && !b.params.AllowShort {
		return
	}
	keys := append([]string(nil), b.params.NamedLevelFlag.LevelPriority...)
	sort.Strings(keys)
	for _, key := range keys {
		if len(key) == 0 || (sideNow == sideLong) != (key[len(key)-1] == 'H') || b.seen.nlf.seen(day, key) {
			continue
		}
		level, live := prices[key]
		old, had := previous[key]
		if !live || !had || level != old {
			continue
		}
		if sideNow == sideLong && (b.series.C[i-1] > level || b.series.C[i] < level+b.params.NamedLevelFlag.BreakDistanceATR*atr) ||
			sideNow == sideShort && (b.series.C[i-1] < level || b.series.C[i] > level-b.params.NamedLevelFlag.BreakDistanceATR*atr) {
			continue
		}
		b.seen.nlf.add(day, key)
		b.nlfState = &namedLevelFlagState{Key: key, Level: level, Side: sideNow, Index: i, Mid: (b.series.H[i] + b.series.L[i]) / 2, High: math.Inf(-1), Low: math.Inf(1), ATR: atr}
		return
	}
}

func (b *broker) nlfKnownRoom(i int, side side, close float64, prices, previous map[string]float64) (string, float64, bool) {
	keys := append([]string(nil), b.params.NamedLevelFlag.LevelPriority...)
	sort.Strings(keys)
	bestKey, bestPrice, bestDistance := "", 0.0, math.Inf(1)
	for _, key := range keys {
		level, live := prices[key]
		old, had := previous[key]
		distance := math.Abs(level - close)
		if live && had && level == old && float64(side)*(level-close) > 0 && distance < bestDistance {
			bestKey, bestPrice, bestDistance = key, level, distance
		}
	}
	return bestKey, bestPrice, bestKey != ""
}
