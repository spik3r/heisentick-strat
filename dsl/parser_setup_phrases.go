package dsl

import "strings"

func (p *parser) parseFlagPole(tokens []string) {
	flag := copyMap(p.config["flag"])
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "at") {
		flag["poleAtr"] = firstNumber(tokens, 2, 0)
	}
	if idx := indexOfLower(tokens, "over"); idx >= 0 {
		flag["poleBars"] = firstNumber(tokens, idx+1, 0)
	}
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "net") {
		flag["minPoleNetFrac"] = firstNumber(tokens, 2, 0)
	}
	p.config["flag"] = flag
}

func (p *parser) parseInside(tokens []string) {
	if p.config["setupType"] != string(FamilyInsideDayExpansion) {
		return
	}
	inside := copyMap(p.config["insideDay"])
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "tolerance") {
		inside["insideToleranceAtr"] = firstNumber(tokens, 2, 0)
	}
	p.config["insideDay"] = inside
}

func (p *parser) parseFlag(tokens []string) {
	flag := copyMap(p.config["flag"])
	if len(tokens) >= 4 && isNumberToken(tokens[1]) && strings.EqualFold(tokens[2], "to") {
		flag["minFlagBars"] = parseNumber(tokens[1], 0)
		flag["maxFlagBars"] = parseNumber(tokens[3], 0)
	} else if len(tokens) >= 4 && strings.EqualFold(tokens[1], "depth") {
		flag["maxFlagDepth"] = firstNumber(tokens, 2, 0)
	} else if len(tokens) >= 4 && strings.EqualFold(tokens[1], "close") {
		flag["flagCloseFrac"] = firstNumber(tokens, 2, 0)
	}
	p.config["flag"] = flag
}

func (p *parser) parseBreak(tokens []string) {
	switch p.config["setupType"] {
	case string(FamilyFlagContinuation):
		flag := copyMap(p.config["flag"])
		if len(tokens) >= 3 && strings.EqualFold(tokens[1], "within") {
			flag["freshBreakBars"] = firstNumber(tokens, 2, 0)
		}
		p.config["flag"] = flag
	case string(FamilyBreakRetest):
		br := copyMap(p.config["breakRetest"])
		if len(tokens) >= 3 && strings.EqualFold(tokens[1], "within") {
			br["freshBreakBars"] = firstNumber(tokens, 2, 0)
		}
		if containsLower(tokens, "distance") {
			br["minBreakAtr"] = firstNumber(tokens, 2, 0)
		}
		p.config["breakRetest"] = br
	case string(FamilyOpeningRangeBreakout):
		orb := copyMap(p.config["openingRangeBreakout"])
		if containsLower(tokens, "beyond") {
			orb["breakBufferAtr"] = firstNumber(tokens, 1, 0)
		}
		p.config["openingRangeBreakout"] = orb
	}
}

func (p *parser) parseLevel(tokens []string) {
	if p.config["setupType"] != string(FamilyFlagContinuation) {
		return
	}
	flag := copyMap(p.config["flag"])
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "tolerance") {
		flag["levelToleranceAtr"] = firstNumber(tokens, 2, 0)
	}
	p.config["flag"] = flag
}

func (p *parser) parseRetest(tokens []string) {
	switch p.config["setupType"] {
	case string(FamilyBreakRetest):
		br := copyMap(p.config["breakRetest"])
		if len(tokens) >= 3 && strings.EqualFold(tokens[1], "within") {
			br["retestBars"] = firstNumber(tokens, 2, 0)
		}
		if len(tokens) >= 3 && strings.EqualFold(tokens[1], "tolerance") {
			br["levelTolerance"] = firstNumber(tokens, 2, 0)
		}
		p.config["breakRetest"] = br
	case string(FamilySupplyDemand):
		sd := copyMap(p.config["supplyDemand"])
		if len(tokens) >= 3 && strings.EqualFold(tokens[1], "tolerance") {
			sd["retestToleranceAtr"] = firstNumber(tokens, 2, 0)
		}
		if len(tokens) >= 3 && strings.EqualFold(tokens[1], "within") {
			sd["maxZoneAgeCandles"] = firstNumber(tokens, 2, 0)
		}
		if containsLower(tokens, "first") {
			sd["maxRetests"] = 1
		}
		if containsLower(tokens, "must") && containsLower(tokens, "reject") {
			sd["requireRejectionClose"] = 1
		}
		p.config["supplyDemand"] = sd
	case string(FamilyFairValueGap):
		fvg := copyMap(p.config["fairValueGap"])
		if len(tokens) >= 3 && strings.EqualFold(tokens[1], "within") {
			fvg["retestCandles"] = firstNumber(tokens, 2, 8)
		}
		p.config["fairValueGap"] = fvg
	}
}

func (p *parser) parseStretch(tokens []string) {
	if p.config["setupType"] != string(FamilyDayOpenReclaim) {
		return
	}
	dayOpen := copyMap(p.config["dayOpen"])
	dayOpen["stretchAtr"] = firstNumber(tokens, 1, 0)
	p.config["dayOpen"] = dayOpen
}

func (p *parser) parseReclaim(tokens []string) {
	switch p.config["setupType"] {
	case string(FamilyTrendPullback):
		if len(tokens) >= 6 && strings.EqualFold(tokens[1], "session") && strings.EqualFold(tokens[2], "vwap") && strings.EqualFold(tokens[3], "on") && strings.EqualFold(tokens[4], "trend") && strings.EqualFold(tokens[5], "side") {
			tp := copyMap(p.config["trendPullback"])
			tp["sessionVwapTrendSideReclaim"] = 1
			p.config["trendPullback"] = tp
		}
	case string(FamilyDayOpenReclaim):
		dayOpen := copyMap(p.config["dayOpen"])
		if idx := indexOfLower(tokens, "within"); idx >= 0 {
			dayOpen["reclaimCandles"] = firstNumber(tokens, idx+1, 0)
		}
		p.config["dayOpen"] = dayOpen
	case string(FamilyRangeBreakFake):
		rbf := copyMap(p.config["rangeBreakFake"])
		if idx := indexOfLower(tokens, "within"); idx >= 0 {
			rbf["reclaimCandles"] = firstNumber(tokens, idx+1, 0)
		}
		p.config["rangeBreakFake"] = rbf
	}
}

func (p *parser) parsePivot(tokens []string) {
	if p.config["setupType"] != string(FamilyDoubleTopBottom) {
		return
	}
	dtb := copyMap(p.config["doubleTopBottom"])
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "window") {
		dtb["pivotWindow"] = firstNumber(tokens, 2, 0)
	}
	p.config["doubleTopBottom"] = dtb
}

func (p *parser) parsePeak(tokens []string) {
	if p.config["setupType"] != string(FamilyDoubleTopBottom) {
		return
	}
	dtb := copyMap(p.config["doubleTopBottom"])
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "gap") {
		dtb["minPeakGap"] = firstNumber(tokens, 2, 0)
		if idx := indexOfLower(tokens, "to"); idx >= 0 {
			dtb["maxPeakGap"] = firstNumber(tokens, idx+1, 0)
		}
	}
	if containsLower(tokens, "difference") {
		dtb["maxPeakDiffAtr"] = firstNumber(tokens, 2, 0)
	}
	p.config["doubleTopBottom"] = dtb
}

func (p *parser) parseNeckline(tokens []string) {
	if p.config["setupType"] != string(FamilyDoubleTopBottom) {
		return
	}
	dtb := copyMap(p.config["doubleTopBottom"])
	dtb["necklineTouchAtr"] = firstNumber(tokens, 1, 0)
	p.config["doubleTopBottom"] = dtb
}

func (p *parser) parseBreakout(tokens []string) {
	if p.config["setupType"] != string(FamilyDoubleTopBottom) {
		return
	}
	dtb := copyMap(p.config["doubleTopBottom"])
	dtb["breakoutBufferAtr"] = firstNumber(tokens, 1, 0)
	p.config["doubleTopBottom"] = dtb
}

func (p *parser) parseConfirm(tokens []string) {
	switch p.config["setupType"] {
	case string(FamilyDoubleTopBottom):
		dtb := copyMap(p.config["doubleTopBottom"])
		if idx := indexOfLower(tokens, "within"); idx >= 0 {
			dtb["confirmCandles"] = firstNumber(tokens, idx+1, 0)
		}
		p.config["doubleTopBottom"] = dtb
	case string(FamilyFibContinuation):
		fib := copyMap(p.config["fibContinuation"])
		fib["confirmCloseLocation"] = firstNumber(tokens, 1, 0)
		p.config["fibContinuation"] = fib
	case string(FamilyTriplePushExhaustion):
		triple := copyMap(p.config["triplePush"])
		triple["confirmCloseLocation"] = firstNumber(tokens, 1, 0)
		p.config["triplePush"] = triple
	}
}

func (p *parser) parseSwing(tokens []string) {
	if p.config["setupType"] != string(FamilyBreakRetest) || len(tokens) < 3 || !strings.EqualFold(tokens[1], "levels") {
		return
	}
	br := copyMap(p.config["breakRetest"])
	br["levelSource"] = "swing"
	if idx := indexOfLower(tokens, "lookback"); idx >= 0 {
		br["swingLookbackBars"] = firstNumber(tokens, idx+1, 60)
	}
	if idx := indexOfLower(tokens, "pivot"); idx >= 0 {
		br["swingPivotK"] = firstNumber(tokens, idx+1, 2)
	}
	if idx := indexOfLower(tokens, "count"); idx >= 0 {
		br["swingLevelCount"] = firstNumber(tokens, idx+1, 4)
	}
	p.config["breakRetest"] = br
}

func (p *parser) parsePullback(tokens []string) {
	if p.config["setupType"] != string(FamilyTrendPullback) && p.config["setupType"] != string(FamilyElderTripleScreen) {
		return
	}
	key := "trendPullback"
	if p.config["setupType"] == string(FamilyElderTripleScreen) {
		key = "elderTripleScreen"
	}
	tp := copyMap(p.config[key])
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "tag") {
		tp["pullbackAtr"] = firstNumber(tokens, 2, 0)
	} else if len(tokens) >= 3 && strings.EqualFold(tokens[1], "within") {
		tp["pullbackWithin"] = firstNumber(tokens, 2, 0)
		if p.config["setupType"] == string(FamilyElderTripleScreen) {
			stop := copyMap(p.config["stop"])
			stop["extremeCandles"] = firstNumber(tokens, 2, 0)
			p.config["stop"] = stop
		}
	} else if len(tokens) >= 4 && strings.EqualFold(tokens[1], "max") && strings.EqualFold(tokens[2], "depth") {
		tp["maxPullbackDepth"] = firstNumber(tokens, 3, 0)
	} else if p.config["setupType"] == string(FamilyTrendPullback) && len(tokens) >= 5 && strings.EqualFold(tokens[1], "session") && strings.EqualFold(tokens[2], "vwap") && strings.EqualFold(tokens[4], "touch") {
		mode := strings.ToLower(tokens[3])
		if mode == "wick" || mode == "close" {
			tp["sessionVwapTouch"] = mode
		}
	} else if p.config["setupType"] == string(FamilyTrendPullback) && len(tokens) >= 6 && strings.EqualFold(tokens[1], "first") && strings.EqualFold(tokens[2], "session") && strings.EqualFold(tokens[3], "vwap") && strings.EqualFold(tokens[4], "touch") && strings.EqualFold(tokens[5], "only") {
		tp["sessionVwapFirstTouchOnly"] = 1
	}
	p.config[key] = tp
}

func (p *parser) parseHold(tokens []string) {
	switch p.config["setupType"] {
	case string(FamilySessionBreakHold):
		p.config["sessionBreakHold"] = map[string]any{"holdCandles": firstNumber(tokens, 1, 3)}
	case string(FamilyChannelBreakHold):
		p.config["channelBreakHold"] = map[string]any{"holdCandles": firstNumber(tokens, 1, 2)}
	case string(FamilyOpeningRangeBreakout):
		orb := copyMap(p.config["openingRangeBreakout"])
		orb["holdCandles"] = firstNumber(tokens, 1, 1)
		p.config["openingRangeBreakout"] = orb
	case string(FamilyInsideDayExpansion):
		inside := copyMap(p.config["insideDay"])
		inside["holdCandles"] = firstNumber(tokens, 1, 2)
		p.config["insideDay"] = inside
	}
}

func (p *parser) parseGrade(tokens []string) {
	if len(tokens) >= 3 && strings.EqualFold(tokens[0], "require") && strings.EqualFold(tokens[1], "range") {
		if p.config["setupType"] == string(FamilyVWAPExtensionFade) {
			vef := copyMap(p.config["vwapExtensionFade"])
			vef["requireRangeDay"] = 1
			p.config["vwapExtensionFade"] = vef
		}
		return
	}
	grade := copyMap(p.config["grade"])
	switch strings.ToLower(tokens[0]) {
	case "context", "location", "trigger":
		grade[strings.ToLower(tokens[0])] = firstNumber(tokens, 1, 0)
	case "require":
		grade["requireTotal"] = firstNumber(tokens, 1, 0)
	}
	p.config["grade"] = grade
}

func (p *parser) parseTail(tokens []string) {
	value := firstNumber(tokens, 1, 0)
	p.config["tailRejectionMin"] = value
	if p.config["setupType"] == string(FamilyRangeBreakFake) {
		rbf := copyMap(p.config["rangeBreakFake"])
		rbf["tailRejectionMin"] = value
		p.config["rangeBreakFake"] = rbf
	}
}

func (p *parser) parseNear(tokens []string) {
	if strings.EqualFold(tokens[0], "nearkeylevel") {
		p.config["levelDistanceAtr"] = firstNumber(tokens, 1, 0)
		return
	}
	if p.config["setupType"] == string(FamilyTriplePushExhaustion) {
		triple := copyMap(p.config["triplePush"])
		triple["nearAtr"] = firstNumber(tokens, 1, 0)
		p.config["triplePush"] = triple
	}
}

func (p *parser) parseRecent(tokens []string) {
	if containsLower(tokens, "within") {
		p.config["levelDistanceAtr"] = firstNumber(tokens, 1, 1.5)
	}
}

func (p *parser) parseImpulse(tokens []string) {
	switch p.config["setupType"] {
	case string(FamilyFibContinuation):
		fib := copyMap(p.config["fibContinuation"])
		fib["impulseAtr"] = firstNumber(tokens, 1, 0)
		p.config["fibContinuation"] = fib
	case string(FamilySupplyDemand):
		sd := copyMap(p.config["supplyDemand"])
		sd["impulseAtr"] = firstNumber(tokens, 1, 0)
		if idx := indexOfLower(tokens, "within"); idx >= 0 {
			sd["impulseCandles"] = firstNumber(tokens, idx+1, 0)
		}
		p.config["supplyDemand"] = sd
	}
}

func (p *parser) parseRetrace(tokens []string) {
	if p.config["setupType"] != string(FamilyFibContinuation) {
		return
	}
	fib := copyMap(p.config["fibContinuation"])
	if idx := indexOfLower(tokens, "into"); idx >= 0 {
		fib["retraceMin"] = firstNumber(tokens, idx+1, 0)
	}
	if idx := indexOfLower(tokens, "to"); idx >= 0 {
		fib["retraceMax"] = firstNumber(tokens, idx+1, 0)
	}
	p.config["fibContinuation"] = fib
}

func (p *parser) parsePatterns(tokens []string) {
	if p.config["setupType"] != string(FamilySupplyDemand) {
		return
	}
	sd := copyMap(p.config["supplyDemand"])
	sd["patterns"] = upperList(tokens[1:])
	p.config["supplyDemand"] = sd
}

func (p *parser) parseBase(tokens []string) {
	if p.config["setupType"] != string(FamilySupplyDemand) {
		return
	}
	sd := copyMap(p.config["supplyDemand"])
	if len(tokens) >= 4 && isNumberToken(tokens[1]) {
		sd["minBaseCandles"] = firstNumber(tokens, 1, 0)
		if idx := indexOfLower(tokens, "to"); idx >= 0 {
			sd["maxBaseCandles"] = firstNumber(tokens, idx+1, 0)
		}
	}
	if containsLower(tokens, "body") {
		sd["maxBaseBodyAtr"] = firstNumber(tokens, 2, 0)
	}
	p.config["supplyDemand"] = sd
}

func (p *parser) parseConsolidation(tokens []string) {
	if p.config["setupType"] != string(FamilySupplyDemand) {
		return
	}
	sd := copyMap(p.config["supplyDemand"])
	if containsLower(tokens, "width") {
		sd["maxZoneAtr"] = firstNumber(tokens, 3, 0)
	} else {
		sd["maxBaseBodyAtr"] = firstNumber(tokens, 3, 0)
	}
	p.config["supplyDemand"] = sd
}

func (p *parser) parseSource(tokens []string) {
	if p.config["setupType"] != string(FamilySupplyDemand) {
		return
	}
	sd := copyMap(p.config["supplyDemand"])
	sd["minSourceAtr"] = firstNumber(tokens, 1, 0)
	if idx := indexOfLower(tokens, "within"); idx >= 0 {
		sd["sourceCandles"] = firstNumber(tokens, idx+1, 0)
	}
	p.config["supplyDemand"] = sd
}

func (p *parser) parseChoch(tokens []string) {
	if p.config["setupType"] != string(FamilySupplyDemand) {
		return
	}
	sd := copyMap(p.config["supplyDemand"])
	if idx := indexOfLower(tokens, "within"); idx >= 0 {
		sd["chochCandles"] = firstNumber(tokens, idx+1, 0)
	}
	if idx := indexOfLower(tokens, "lookback"); idx >= 0 {
		sd["chochLookbackCandles"] = firstNumber(tokens, idx+1, 0)
	}
	p.config["supplyDemand"] = sd
}

func (p *parser) parseZone(tokens []string) {
	if p.config["setupType"] != string(FamilySupplyDemand) {
		return
	}
	sd := copyMap(p.config["supplyDemand"])
	if containsLower(tokens, "width") {
		sd["maxZoneAtr"] = firstNumber(tokens, 3, 0)
	}
	if containsLower(tokens, "bounds") {
		if containsLower(tokens, "body") {
			sd["zoneBounds"] = "body"
		} else if containsLower(tokens, "proximal") || containsLower(tokens, "distal") {
			sd["zoneBounds"] = "proximalDistal"
		} else {
			sd["zoneBounds"] = "wick"
		}
	}
	if containsLower(tokens, "break") {
		sd["invalidationAtr"] = firstNumber(tokens, 2, 0.05)
	}
	if containsLower(tokens, "flip") {
		sd["flipBrokenZones"] = 1
	}
	p.config["supplyDemand"] = sd
}

func (p *parser) parsePierce(tokens []string) {
	if p.config["setupType"] != string(FamilyRangeBreakFake) {
		return
	}
	rbf := copyMap(p.config["rangeBreakFake"])
	rbf["pierceAtr"] = firstNumber(tokens, 1, 0)
	p.config["rangeBreakFake"] = rbf
}

func (p *parser) parseTriplePush(tokens []string) {
	if p.config["setupType"] != string(FamilyTriplePushExhaustion) {
		return
	}
	triple := copyMap(p.config["triplePush"])
	triple["lookbackBars"] = firstNumber(tokens, 1, 0)
	p.config["triplePush"] = triple
}

func (p *parser) parseMin(tokens []string) {
	if p.config["setupType"] != string(FamilyTriplePushExhaustion) || len(tokens) < 2 || !strings.EqualFold(tokens[1], "separation") {
		return
	}
	triple := copyMap(p.config["triplePush"])
	triple["minSeparation"] = firstNumber(tokens, 2, 0)
	p.config["triplePush"] = triple
}

func (p *parser) parsePush(tokens []string) {
	if p.config["setupType"] != string(FamilyTriplePushExhaustion) || len(tokens) < 2 || !strings.EqualFold(tokens[1], "decay") {
		return
	}
	triple := copyMap(p.config["triplePush"])
	triple["pushDecay"] = firstNumber(tokens, 2, 0)
	p.config["triplePush"] = triple
}

func (p *parser) parseDecel(tokens []string) {
	if p.config["setupType"] != string(FamilyTriplePushExhaustion) {
		return
	}
	triple := copyMap(p.config["triplePush"])
	if len(tokens) >= 2 {
		triple["decelMode"] = strings.ToLower(tokens[1])
	}
	p.config["triplePush"] = triple
}

func (p *parser) parseDistance(tokens []string) {
	if p.config["setupType"] != string(FamilyVWAPExtensionFade) {
		return
	}
	vef := copyMap(p.config["vwapExtensionFade"])
	vef["minDistanceAtr"] = firstNumber(tokens, 1, 0)
	p.config["vwapExtensionFade"] = vef
}

func (p *parser) parseVolume(tokens []string) {
	if p.config["setupType"] != string(FamilyVolumeAnomalyExhaustion) {
		return
	}
	vae := copyMap(p.config["volumeAnomalyExhaustion"])
	if containsLower(tokens, "ratio") {
		vae["volumeRatioMin"] = firstNumber(tokens, 1, 0)
	}
	p.config["volumeAnomalyExhaustion"] = vae
}

func (p *parser) parseAnomaly(tokens []string) {
	if p.config["setupType"] != string(FamilyVolumeAnomalyExhaustion) || len(tokens) < 2 || !strings.EqualFold(tokens[1], "lookback") {
		return
	}
	vae := copyMap(p.config["volumeAnomalyExhaustion"])
	vae["anomalyLookbackCandles"] = firstNumber(tokens, 2, 0)
	p.config["volumeAnomalyExhaustion"] = vae
}

func (p *parser) parseHighEffort(tokens []string) {
	if p.config["setupType"] != string(FamilyVolumeAnomalyExhaustion) {
		return
	}
	vae := copyMap(p.config["volumeAnomalyExhaustion"])
	if containsLower(tokens, "spread") {
		vae["maxSpreadAtr"] = firstNumber(tokens, 1, 0)
	}
	p.config["volumeAnomalyExhaustion"] = vae
}

func (p *parser) parseOr(tokens []string) {
	if p.config["setupType"] != string(FamilyVolumeAnomalyExhaustion) {
		return
	}
	vae := copyMap(p.config["volumeAnomalyExhaustion"])
	if containsLower(tokens, "rejection") {
		vae["rejectionWickMin"] = firstNumber(tokens, 1, 0)
	}
	p.config["volumeAnomalyExhaustion"] = vae
}

func (p *parser) parseGradeSize(tokens []string) {
	if len(tokens) < 4 || !strings.EqualFold(tokens[1], "by") || !strings.EqualFold(tokens[2], "score") {
		return
	}
	tiers := []any{}
	for i := 3; i+1 < len(tokens); i += 2 {
		tiers = append(tiers, map[string]any{"threshold": parseNumber(tokens[i], 0), "multiplier": parseNumber(tokens[i+1], 1)})
	}
	grade := copyMap(p.config["grade"])
	grade["sizeTiers"] = tiers
	p.config["grade"] = grade
}

func (p *parser) parseSetupMovement(tokens []string) bool {
	switch p.config["setupType"] {
	case string(FamilyFlagContinuation):
		flag := copyMap(p.config["flag"])
		if len(tokens) >= 4 && strings.EqualFold(tokens[1], "between") {
			flag["minEr"] = firstNumber(tokens, 2, 0)
			if idx := indexOfLower(tokens, "and"); idx >= 0 {
				flag["maxEr"] = firstNumber(tokens, idx+1, 0)
			}
			p.config["flag"] = flag
			return true
		}
	case string(FamilyTrendPullback):
		tp := copyMap(p.config["trendPullback"])
		if len(tokens) >= 4 && strings.EqualFold(tokens[1], "between") {
			tp["minEr"] = firstNumber(tokens, 2, 0)
			if idx := indexOfLower(tokens, "and"); idx >= 0 {
				tp["maxEr"] = firstNumber(tokens, idx+1, 0)
			}
			p.config["trendPullback"] = tp
			return true
		}
	case string(FamilyBreakRetest):
		br := copyMap(p.config["breakRetest"])
		if len(tokens) >= 4 && strings.EqualFold(tokens[1], "between") {
			br["minEr"] = firstNumber(tokens, 2, 0)
			if idx := indexOfLower(tokens, "and"); idx >= 0 {
				br["maxEr"] = firstNumber(tokens, idx+1, 0)
			}
			p.config["breakRetest"] = br
			return true
		}
	}
	return false
}
