package dsl

func (p *parser) parseSetupType(tokens []string) {
	family := canonicalSetupFamily(tokens)
	if family == "" {
		return
	}
	p.config["setupType"] = family
	p.config["breakeven"] = map[string]any{"atR": 0.5, "offsetAtr": 0.05}
	p.config["cooldownCandles"] = 12
	switch family {
	case string(FamilyFlagContinuation):
		p.config["flag"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": nil, "minAtr": 0.5, "paddingAtr": 0.25}
		target := copyMap(p.config["target"])
		target["flagR"] = 1
		p.config["target"] = target
	case string(FamilySessionBreakHold):
		p.config["sessionBreakHold"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.5, "paddingAtr": 0.5}
		target := copyMap(p.config["target"])
		target["sbhR"] = 1
		p.config["target"] = target
	case string(FamilyTrendPullback):
		p.config["trendPullback"] = map[string]any{"emaLen": 21, "emaSlopeLen": 5}
		p.config["emaLen"] = 21
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": nil, "minAtr": 0.5, "paddingAtr": 0.25}
		target := copyMap(p.config["target"])
		target["tpbR"] = 2
		p.config["target"] = target
	case string(FamilyDualEMAResumption):
		p.config["breakeven"] = map[string]any{"atR": 0.75, "offsetAtr": 0.02}
		p.config["cooldownCandles"] = 3
		p.config["dualEmaResumption"] = map[string]any{
			"fastEmaLen": 20, "slowEmaLen": 80, "slowRiseBars": 12,
			"atrLen": 20, "stopAtr": 2.5, "trailAtr": 3.0,
		}
	case string(FamilySMAGoldenCross):
		p.config["breakeven"] = map[string]any{"atR": 0.75, "offsetAtr": 0.02}
		p.config["cooldownCandles"] = 3
		p.config["smaGoldenCross"] = map[string]any{"fastSmaLen": 50.0, "slowSmaLen": 200.0}
		p.config["allowLong"] = 1
		p.config["allowShort"] = 0
	case string(FamilyBreakRetest):
		p.config["breakRetest"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": nil, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("brR", 1)
	case string(FamilyChannelBreakHold):
		p.config["channelBreakHold"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.5, "paddingAtr": 0.5}
		p.setDefaultTarget("cbhR", 1)
	case string(FamilyDayOpenReclaim):
		p.config["dayOpen"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.5, "paddingAtr": 0.25}
		p.setDefaultTarget("dorR", 1)
	case string(FamilyDoubleTopBottom):
		p.config["doubleTopBottom"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("dtbR", 1)
	case string(FamilyElderTripleScreen):
		p.config["elderTripleScreen"] = map[string]any{"emaLen": 21}
		p.config["emaLen"] = 21
		p.projectSliceDimensions()
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.5, "paddingAtr": 0.25}
	case string(FamilyPriceMomentum):
		p.config["priceMomentum"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 3, "maxAtr": 3, "minAtr": 0.5, "paddingAtr": 0.25}
	case string(FamilyDailyFlushFailure):
		p.config["dailyFlushFailure"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 2, "maxAtr": 3, "minAtr": 0.5, "paddingAtr": 0.1}
		p.config["breakeven"] = map[string]any{"atR": 0, "offsetAtr": 0}
		p.config["cooldownCandles"] = 0
		p.config["maxHoldCandles"] = 10
		p.config["allowLong"] = 1
		p.config["allowShort"] = 0
	case string(FamilyFibContinuation):
		p.config["fibContinuation"] = map[string]any{"impulseLookbackBars": 24}
		p.config["emaLen"] = 21
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("fibR", 2)
	case string(FamilyInsideDayExpansion):
		p.config["insideDay"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.5, "paddingAtr": 0.25}
		p.setDefaultTarget("ideR", 1)
	case string(FamilyOpeningRangeBreakout):
		p.config["openingRangeBreakout"] = map[string]any{"firstMinutes": 0}
		p.applyDefaultSessions(map[string]any{"asia": 0, "london": 1, "ny": 1})
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("orbR", 1)
	case string(FamilyRangeBreakFake):
		p.config["rangeBreakFake"] = map[string]any{"requireCloseBackInside": 1}
		p.setDefaultTarget("rbfR", 1)
	case string(FamilySupplyDemand):
		p.config["supplyDemand"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("sdR", 1)
	case string(FamilyTriplePushExhaustion):
		p.config["triplePush"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("tpeR", 1)
	case string(FamilyVWAPExtensionFade):
		p.config["vwapExtensionFade"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 2, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("vefR", 1)
	case string(FamilyVolumeAnomalyExhaustion):
		p.config["volumeAnomalyExhaustion"] = map[string]any{}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 2, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("vaeR", 1)
		p.applyDefaultSessions(map[string]any{"asia": 0, "london": 1, "ny": 1})
	case string(FamilyFairValueGap):
		p.config["fairValueGap"] = map[string]any{
			"minGapAtr":          0.1,
			"minDisplacementAtr": 0.8,
			"retestCandles":      8,
			"entryReference":     "midpoint",
		}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 3, "minAtr": 0.4, "paddingAtr": 0.25}
		p.setDefaultTarget("fvgR", 1)
	case string(FamilyWeekendExtremeFade):
		p.config["weekendExtremeFade"] = map[string]any{
			"atrLen": 20, "maxWeekendRangeAtr": 5.0, "closeExtremePct": 0.3,
			"stopAtr": 0.75, "targetAtr": 2.0,
		}
		p.config["stop"] = map[string]any{"type": "atr", "atrMult": 0.75, "extremeCandles": 0, "maxAtr": nil, "minAtr": 0, "paddingAtr": 0}
		p.config["breakeven"] = map[string]any{"atR": nil, "offsetAtr": 0}
		p.config["cooldownCandles"] = 0
		p.config["maxHoldCandles"] = 12
	case string(FamilyIntraHourRunExhaustion):
		if timeframesAreDefault(p.config["timeframes"]) {
			p.config["timeframes"] = []any{"15m"}
		}
		p.config["intraHourRunExhaustion"] = map[string]any{
			"atrLen": 14, "hourCandles": 4, "runCandles": 3,
			"minRunAtr": 0.8, "exhaustLocationPct": 0.35,
			"requirePoke": 0, "stopPadAtr": 0.25,
		}
		p.config["stop"] = map[string]any{"type": "structure", "extremeCandles": 0, "maxAtr": nil, "minAtr": 0, "paddingAtr": 0.25}
		p.config["breakeven"] = map[string]any{"atR": nil, "offsetAtr": 0}
		p.config["cooldownCandles"] = 4
		p.config["maxHoldCandles"] = 8
		p.setDefaultTarget("ihreR", 1.5)
	case string(FamilyKeltnerReversion), string(FamilyKeltnerExpansion):
		if timeframesAreDefault(p.config["timeframes"]) {
			p.config["timeframes"] = []any{"15m"}
		}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": 2, "minAtr": 0.4, "paddingAtr": 0.25}
		p.config["maxHoldCandles"] = 24
		target := copyMap(p.config["target"])
		target["krR"] = 1
		target["minR"] = 0.5
		p.config["target"] = target
		// Both keltner families deliberately share the "keltnerReversion"
		// config key — keltner expansion does not get its own key.
		p.config["keltnerReversion"] = map[string]any{}
	case string(FamilyNamedLevelSweep):
		p.config["namedLevelSweep"] = map[string]any{"rules": map[string]any{}, "stop": map[string]any{}}
		p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": nil, "minAtr": 0, "paddingAtr": 0}
		p.config["breakeven"] = map[string]any{"atR": 0.75, "offsetAtr": 0.05}
		p.setDefaultTarget("nlsR", 1)
	}
}

// timeframesAreDefault reports whether value still holds the DSL's base
// default timeframe list (["5m", "15m"]), regardless of whether it is
// represented as []any (defaultConfig()'s shape) or []string (the shape the
// "timeframes" directive assigns). Mirrors the JS check
// `cfg.timeframes.join(',') === DEFAULT_CFG.timeframes.join(',')`, which
// compares by content rather than by whether the user explicitly wrote it.
func timeframesAreDefault(value any) bool {
	var list []string
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				return false
			}
			list = append(list, s)
		}
	case []string:
		list = typed
	default:
		return false
	}
	return len(list) == 2 && list[0] == "5m" && list[1] == "15m"
}

func (p *parser) setDefaultTarget(key string, value float64) {
	target := copyMap(p.config["target"])
	target[key] = value
	p.config["target"] = target
}

func (p *parser) projectSliceDimensions() {
	rawSlices, ok := p.config["slices"].([]any)
	if !ok || len(rawSlices) == 0 {
		return
	}
	symbolSeen := map[string]bool{}
	tfSeen := map[string]bool{}
	symbols := []any{}
	timeframes := []any{}
	for _, raw := range rawSlices {
		slice, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		symbol, _ := slice["symbol"].(string)
		tf, _ := slice["tf"].(string)
		if symbol != "" && !symbolSeen[symbol] {
			symbolSeen[symbol] = true
			symbols = append(symbols, symbol)
		}
		if tf != "" && !tfSeen[tf] {
			tfSeen[tf] = true
			timeframes = append(timeframes, tf)
		}
	}
	p.config["symbols"] = symbols
	p.config["timeframes"] = timeframes
}
