package engine

import (
	"fmt"
	"math"
)

type volumeAnomaly struct {
	Idx  int
	Kind string
}

func (b *broker) onVolumeAnomalyExhaustionBar(i int) {
	if b.hasPosition {
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasVAELast && i-b.vaeLast < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	sides := []side{sideLong, sideShort}
	if b.series.C[i] < b.series.O[i] {
		sides = []side{sideShort, sideLong}
	}
	for _, s := range sides {
		if s == sideLong && !p.AllowLong {
			continue
		}
		if s == sideShort && !p.AllowShort {
			continue
		}
		setup, seenKey, ok := b.volumeAnomalySetup(i, s)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		if b.seen.vae.seen(day, seenKey) {
			continue
		}
		b.seen.vae.add(day, seenKey)
		levelKey, _ := setup.Meta["levelKey"].(string)
		setup.Tag = "DSL-VPA:" + levelKey
		b.enterSetup(i, setup)
		b.vaeLast = i
		b.hasVAELast = true
		break
	}
}

func (b *broker) volumeAnomalySetup(i int, s side) (setupPlan, string, bool) {
	p := b.params.VolumeAnomalyExhaustion
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return setupPlan{}, "", false
	}
	anomaly, ok := b.volumeAnomalyAt(i, s)
	if !ok {
		return setupPlan{}, "", false
	}
	refPrice := b.series.L[anomaly.Idx]
	if s == sideShort {
		refPrice = b.series.H[anomaly.Idx]
	}
	level, ok := b.nearestVolumeLevel(i, s, refPrice)
	if !ok {
		return setupPlan{}, "", false
	}
	if s == sideShort {
		if !(b.series.C[i] < level.Price) {
			return setupPlan{}, "", false
		}
	} else if !(b.series.C[i] > level.Price) {
		return setupPlan{}, "", false
	}
	if p.RejectionWickMin != 0 && tailRejection(b.series, i, s) < p.RejectionWickMin && anomaly.Kind == "rejection-wick" {
		return setupPlan{}, "", false
	}
	if p.CloseLocationMin != 0 && closeLocation(b.series, i, s) < p.CloseLocationMin {
		return setupPlan{}, "", false
	}
	entry := b.series.C[i]
	stop := recentExtreme(b.series, i, p.StopLookback, s)
	if s == sideShort {
		stop += atr * p.StopBufferATR
	} else {
		stop -= atr * p.StopBufferATR
	}
	if !stopOK(entry, stop, atr, p.MinStopATR, p.MaxStopATR) {
		return setupPlan{}, "", false
	}
	risk := math.Abs(entry - stop)
	target, targetMode, targetR := b.volumeAnomalyTarget(i, s, entry, risk)
	if s == sideLong {
		if target <= entry {
			return setupPlan{}, "", false
		}
	} else if target >= entry {
		return setupPlan{}, "", false
	}
	phase := sessionPhaseName(b.cols.SessionPhase[i])
	meta := gradeMeta(TradeMeta{
		"anomalyIndex":      anomaly.Idx,
		"anomalyKind":       anomaly.Kind,
		"bodyAtr":           b.cols.BodyATR[anomaly.Idx],
		"effortResultRatio": b.cols.EffortResultRatio[anomaly.Idx],
		"levelDistanceAtr":  math.Abs(refPrice-level.Price) / atr,
		"levelKey":          level.Key,
		"levelPrice":        level.Price,
		"openLocation":      openLocationName(b.cols.OpenLocation[i]),
		"priorDayType":      priorDayTypeName(b.cols.PriorDayType[i]),
		"sessionPhase":      phase,
		"setup":             "volumeAnomalyExhaustion",
		"setupAgeCandles":   i - anomaly.Idx,
		"side":              s.String(),
		"spreadAtr":         b.cols.SpreadATR[anomaly.Idx],
		"targetMode":        targetMode,
		"targetR":           targetR,
		"volumeRatio20":     b.cols.VolumeRatio20[anomaly.Idx],
		"volumeZ50":         b.cols.VolumeZ50[anomaly.Idx],
		"vwapDistanceAtr":   b.cols.VWAPDistanceATR[i],
	})
	seenKey := fmt.Sprintf("%s:%s:%.0f:%s", s.String(), level.Key, math.Round(level.Price*10), phase)
	return setupPlan{Side: s, Stop: stop, Target: target, Meta: meta}, seenKey, true
}

func (b *broker) volumeAnomalyAt(i int, s side) (volumeAnomaly, bool) {
	p := b.params.VolumeAnomalyExhaustion
	from := maxInt(0, i-p.AnomalyLookbackCandles+1)
	for j := i; j >= from; j-- {
		if !(b.cols.VolumeRatio20[j] >= p.VolumeRatioMin) {
			continue
		}
		narrow := b.cols.SpreadATR[j] <= p.MaxSpreadATR
		rejection := tailRejection(b.series, j, s) >= p.RejectionWickMin
		if !narrow && !rejection {
			continue
		}
		kind := "rejection-wick"
		if narrow && rejection {
			kind = "narrow+rejection"
		} else if narrow {
			kind = "narrow-spread"
		}
		return volumeAnomaly{Idx: j, Kind: kind}, true
	}
	return volumeAnomaly{}, false
}

func (b *broker) nearestVolumeLevel(i int, s side, refPrice float64) (levelRef, bool) {
	p := b.params.VolumeAnomalyExhaustion
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return levelRef{}, false
	}
	for _, key := range p.LevelPriority {
		level, ok := b.keyLevel(i, key, refPrice)
		if !ok || !sideAllowsLevel(s, level.Key, level.Price, refPrice) {
			continue
		}
		if math.Abs(refPrice-level.Price) <= atr*p.LevelDistanceATR {
			return level, true
		}
	}
	return levelRef{}, false
}

func sideAllowsLevel(s side, key string, price float64, refPrice float64) bool {
	if key == "VWAP" {
		if s == sideShort {
			return refPrice >= price
		}
		return refPrice <= price
	}
	upper := map[string]bool{"CAM_R3": true, "CAM_R4": true, "PDH": true, "PDO": true, "PDC": true, "DH": true, "WH": true, "AH": true, "LH": true, "NH": true}
	lower := map[string]bool{"CAM_S3": true, "CAM_S4": true, "PDL": true, "PDO": true, "PDC": true, "DL": true, "WL": true, "AL": true, "LL": true, "NL": true}
	if s == sideShort {
		return upper[key]
	}
	return lower[key]
}

func (b *broker) volumeAnomalyTarget(i int, s side, entry float64, risk float64) (float64, string, float64) {
	p := b.params.VolumeAnomalyExhaustion
	if p.TargetMode == "vwapElseFixedR" && isFinite(b.cols.VWAP[i]) {
		targetR := math.Abs(b.cols.VWAP[i]-entry) / risk
		valid := b.cols.VWAP[i] > entry
		if s == sideShort {
			valid = b.cols.VWAP[i] < entry
		}
		if valid && targetR >= p.MinVWAPTargetR {
			return b.cols.VWAP[i], "vwap", targetR
		}
	}
	return entry + float64(s)*risk*p.TargetR, "fixedR", p.TargetR
}
