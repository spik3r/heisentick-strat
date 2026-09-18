package engine

import (
	"fmt"
	"math"
)

func (b *broker) onVWAPExtensionFadeBar(i int) {
	if b.hasPosition {
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasVEFLast && i-b.vefLast < p.CooldownBars {
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
		setup, seenKey, ok := b.vwapExtensionFadeSetup(i, s)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		if b.seen.vef.seen(day, seenKey) {
			continue
		}
		b.seen.vef.add(day, seenKey)
		b.enterSetup(i, setup)
		b.vefLast = i
		b.hasVEFLast = true
		break
	}
}

func (b *broker) vwapExtensionFadeSetup(i int, s side) (setupPlan, string, bool) {
	p := b.params.VWAPExtensionFade
	atr := finiteOrZero(b.cols.ATR[i])
	vwap := b.cols.VWAP[i]
	distance := b.cols.VWAPDistanceATR[i]
	if atr == 0 || !isFinite(vwap) || !isFinite(distance) {
		return setupPlan{}, "", false
	}
	phase := sessionPhaseName(b.cols.SessionPhase[i])
	if len(p.SessionPhases) > 0 && !containsString(p.SessionPhases, phase) {
		return setupPlan{}, "", false
	}
	if p.RequireRangeDay && !(b.cols.Regime[i] == regimeRanging || b.cols.Regime[i] == regimeChoppy) {
		return setupPlan{}, "", false
	}
	if s == sideShort && distance < p.MinDistanceATR {
		return setupPlan{}, "", false
	}
	if s == sideLong && distance > -p.MinDistanceATR {
		return setupPlan{}, "", false
	}
	if !candleQualityOK(b.series, i, s, p.TailRejectionMin, p.CloseLocationMin) {
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
	target := b.vwapFadeTarget(i, s, entry, risk, p)
	targetR := math.Abs(target-entry) / risk
	if (s == sideLong && target <= entry) || (s == sideShort && target >= entry) || targetR < p.MinTargetR {
		target = entry + float64(s)*risk*p.TargetR
		targetR = math.Abs(target-entry) / risk
	}
	if (s == sideLong && target <= entry) || (s == sideShort && target >= entry) || targetR < p.MinTargetR {
		return setupPlan{}, "", false
	}
	meta := gradeMeta(TradeMeta{
		"levelKey":        "VWAP",
		"levelPrice":      vwap,
		"regime":          regimeName(b.cols.Regime[i]),
		"sessionPhase":    phase,
		"setup":           "vwapExtensionFade",
		"setupAgeCandles": float64(0),
		"side":            s.String(),
		"targetMode":      p.TargetMode,
		"targetR":         targetR,
		"vwap":            vwap,
		"vwapDistanceAtr": distance,
	})
	return setupPlan{Side: s, Stop: stop, Target: target, Tag: "DSL-VEF:VWAP", Meta: meta},
		fmt.Sprintf("%s:%s:%.0f", s.String(), phase, math.Round(vwap*10)), true
}

func (b *broker) vwapFadeTarget(i int, s side, entry float64, risk float64, p vwapExtensionFadeParams) float64 {
	if p.TargetMode == "vwap" && isFinite(b.cols.VWAP[i]) {
		return b.cols.VWAP[i]
	}
	return entry + float64(s)*risk*p.TargetR
}
