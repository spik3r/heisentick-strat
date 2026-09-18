package engine

import "math"

// elderTripleScreenParamsFrom builds elderTripleScreenParams from the parsed
// "elderTripleScreen", "htf", "stop", and "target" config maps. Colocated
// with the family's runtime rather than left inline in config.go's builder.
func elderTripleScreenParamsFrom(elderTripleScreen, cfg, htf, stop, target map[string]any, triggerExplicit bool, triggerCandles []string) elderTripleScreenParams {
	return elderTripleScreenParams{
		EMALen:              intValue(elderTripleScreen, "emaLen", intFromAny(cfg["emaLen"], 21)),
		EMASlopeLen:         intValue(elderTripleScreen, "emaSlopeLen", 5),
		PullbackATR:         numberValue(elderTripleScreen, "pullbackAtr", 0.3),
		PullbackWithin:      intValue(elderTripleScreen, "pullbackWithin", 8),
		MaxPullbackDepth:    numberValue(elderTripleScreen, "maxPullbackDepth", 0.6),
		ImpulseLookbackBars: intValue(elderTripleScreen, "impulseLookbackBars", 12),
		TargetR:             numberValue(target, "fallbackR", 2),
		MinStopATR:          numberValue(stop, "minAtr", 0.5),
		StopBufferATR:       numberValue(stop, "paddingAtr", 0.3),
		TrailATR:            numberValue(elderTripleScreen, "trailAtr", 1.5),
		TrailTriggerR:       numberValue(elderTripleScreen, "trailR", 1.5),
		UseHTFBias:          stringValue(htf, "mode", "off") == "notAgainst",
		UseTrigger:          !triggerExplicit || !containsString(triggerCandles, "any"),
	}
}

func (b *broker) onElderTripleScreenBar(i int) {
	if b.hasPosition {
		b.moveStopToBreakeven(i)
		b.applyTrailWith(i, trailParams{
			ATR:      b.params.ElderTripleScreen.TrailATR,
			TriggerR: b.params.ElderTripleScreen.TrailTriggerR,
		})
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasElderEntry && i-b.elderLastEntry < p.CooldownBars {
		return
	}
	if !inAnyTradeWindow(b.series.T[i]) {
		return
	}
	for _, s := range []side{sideLong, sideShort} {
		if s == sideLong && !p.AllowLong {
			continue
		}
		if s == sideShort && !p.AllowShort {
			continue
		}
		setup, ok := b.elderSetup(i, s)
		if !ok {
			continue
		}
		b.enterSetup(i, setup)
		b.elderLastEntry = i
		b.hasElderEntry = true
		break
	}
}

func (b *broker) elderSetup(i int, s side) (setupPlan, bool) {
	p := b.params.ElderTripleScreen
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 || i >= len(b.cols.EMAFast) || !isFinite(b.cols.EMAFast[i]) {
		return setupPlan{}, false
	}
	if p.UseHTFBias && !b.htfAllows(i, s) {
		return setupPlan{}, false
	}
	slope := b.cols.EMAFastSlope[i]
	if s == sideLong {
		if !(slope > 0) {
			return setupPlan{}, false
		}
	} else if !(slope < 0) {
		return setupPlan{}, false
	}
	tagIdx := b.elderRecentEMATag(i, s, atr)
	if tagIdx < 0 {
		return setupPlan{}, false
	}
	hasTrigger := longTrigger(b.series, i) || b.emaReclaim(i, sideLong)
	if s == sideShort {
		hasTrigger = shortTrigger(b.series, i) || b.emaReclaim(i, sideShort)
	}
	if p.UseTrigger && !hasTrigger {
		return setupPlan{}, false
	}
	priorStart := maxInt(0, tagIdx-p.ImpulseLookbackBars)
	priorEnd := maxInt(priorStart, tagIdx-1)
	priorHi, priorLo := hiLo(b.series, priorStart, priorEnd)
	pullHi, pullLo := hiLo(b.series, tagIdx, i)
	impulseRange := priorHi - priorLo
	if !(impulseRange > 0) {
		return setupPlan{}, false
	}
	depth := (priorHi - pullLo) / impulseRange
	stopRaw := pullLo - atr*p.StopBufferATR
	if s == sideShort {
		depth = (pullHi - priorLo) / impulseRange
		stopRaw = pullHi + atr*p.StopBufferATR
	}
	if depth > p.MaxPullbackDepth {
		return setupPlan{}, false
	}
	entry := b.series.C[i]
	stopDist := math.Abs(entry - stopRaw)
	if stopDist < atr*p.MinStopATR {
		return setupPlan{}, false
	}
	trigger := "none"
	if hasTrigger {
		trigger = "candle"
	}
	return setupPlan{
		Side:   s,
		Stop:   stopRaw,
		Target: entry + float64(s)*stopDist*p.TargetR,
		Tag:    "DSL-ETS",
		Meta: gradeMeta(TradeMeta{
			"emaPrice":      b.cols.EMAFast[i],
			"emaSlope":      slope,
			"pullbackDepth": depth,
			"setup":         "elder-triple-screen",
			"side":          s.String(),
			"tagIdx":        tagIdx,
			"trigger":       trigger,
		}),
	}, true
}

func (b *broker) elderRecentEMATag(i int, s side, atr float64) int {
	p := b.params.ElderTripleScreen
	from := maxInt(0, i-p.PullbackWithin+1)
	tolerance := atr * p.PullbackATR
	for j := i; j >= from; j-- {
		if !isFinite(b.cols.EMAFast[j]) {
			continue
		}
		if s == sideLong && b.series.L[j] <= b.cols.EMAFast[j]+tolerance {
			return j
		}
		if s == sideShort && b.series.H[j] >= b.cols.EMAFast[j]-tolerance {
			return j
		}
	}
	return -1
}

func (b *broker) emaReclaim(i int, s side) bool {
	if i == 0 || !isFinite(b.cols.EMAFast[i]) {
		return false
	}
	if s == sideLong {
		return b.series.C[i-1] < b.cols.EMAFast[i] && b.series.C[i] > b.cols.EMAFast[i]
	}
	return b.series.C[i-1] > b.cols.EMAFast[i] && b.series.C[i] < b.cols.EMAFast[i]
}

func inAnyTradeWindow(t float64) bool {
	return inFlagTradeWindow(t, flagParams{
		UseAsiaWindow:   true,
		UseMidWindow:    true,
		UseLondonWindow: true,
		UseNYWindow:     true,
	}, 0)
}
