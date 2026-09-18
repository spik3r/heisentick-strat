package dsl

import "strings"

func (p *parser) parseEnter(tokens []string) {
	entry := copyMap(p.config["entryMode"])
	if containsLower(tokens, "limit") {
		entry["type"] = "limitSweptEdge"
		entry["expireCandles"] = firstNumber(tokens, 1, 5)
	} else {
		entry["type"] = "market"
	}
	p.config["entryMode"] = entry
}

func (p *parser) parseStop(line logicalLine, tokens []string) {
	stop := copyMap(p.config["stop"])
	if containsLower(tokens, "pips") {
		if p.config["setupType"] != string(FamilyOpeningRangeBreakout) {
			p.err(line, "fixed-pip stops are currently supported only for opening range breakout", "stop N ATR")
			return
		}
		pips := firstNumber(tokens, 1, 0)
		if !(pips > 0) {
			p.err(line, "stop pips must be positive", "stop 10 pips")
			return
		}
		stop["type"] = "pips"
		stop["pips"] = pips
	} else if len(tokens) >= 2 && isNumberToken(tokens[1]) {
		stop["type"] = "fixedAtr"
		stop["atr"] = parseNumber(tokens[1], 0)
	} else if containsLower(tokens, "size") {
		if idx := indexOfLower(tokens, "between"); idx >= 0 {
			stop["minAtr"] = firstNumber(tokens, idx+1, 0)
			if and := indexOfLower(tokens, "and"); and >= 0 {
				stop["maxAtr"] = firstNumber(tokens, and+1, 1.5)
			}
		}
		if idx := indexOfLower(tokens, "min"); idx >= 0 {
			stop["minAtr"] = firstNumber(tokens, idx+1, 0)
		}
		if idx := indexOfLower(tokens, "max"); idx >= 0 {
			stop["maxAtr"] = firstNumber(tokens, idx+1, 1.5)
		}
		if idx := indexOfLower(tokens, "<="); idx >= 0 {
			stop["maxAtr"] = firstNumber(tokens, idx+1, 1.5)
		}
	} else {
		if p.config["setupType"] == string(FamilyFibContinuation) && len(tokens) > 2 && isNumberToken(tokens[2]) {
			stop["type"] = "fibRetrace"
			stop["fibRatio"] = parseNumber(tokens[2], 0)
		}
		if idx := indexOfLower(tokens, "last"); idx >= 0 && idx+1 < len(tokens) && isNumberToken(tokens[idx+1]) {
			stop["extremeCandles"] = firstNumber(tokens, idx+1, 3)
		} else if idx := indexOfLower(tokens, "extreme"); idx >= 0 && idx+1 < len(tokens) && isNumberToken(tokens[idx+1]) {
			stop["extremeCandles"] = firstNumber(tokens, idx+1, 3)
		}
		if idx := indexOfLower(tokens, "by"); idx >= 0 {
			stop["paddingAtr"] = firstNumber(tokens, idx+1, 0.25)
		} else if idx := indexOfLower(tokens, "plus"); idx >= 0 {
			stop["paddingAtr"] = firstNumber(tokens, idx+1, 0.25)
		} else if idx := indexOf(tokens, "+"); idx >= 0 {
			stop["paddingAtr"] = firstNumber(tokens, idx+1, 0.25)
		} else if containsLower(tokens, "opposite") && containsLower(tokens, "range") {
			stop["paddingAtr"] = firstNumber(tokens, 1, 0.25)
		}
		if len(tokens) >= 4 && strings.EqualFold(tokens[1], "inside") {
			stop["paddingAtr"] = firstNumber(tokens, 3, 0.5)
			if p.config["setupType"] != string(FamilyInsideDayExpansion) {
				stop["minAtr"] = firstNumber(tokens, 3, 0.5)
			}
		}
		if idx := indexOfLower(tokens, "min"); idx >= 0 {
			stop["minAtr"] = firstNumber(tokens, idx+1, 0)
		}
		if idx := indexOfLower(tokens, "max"); idx >= 0 {
			stop["maxAtr"] = firstNumber(tokens, idx+1, 1.5)
		}
	}
	p.config["stop"] = stop
}

const rangeBreakFakeVWAPTargetWarning = "range-break-fake target vwap currently falls back to the range midpoint when VWAP is unavailable; future v7 semantics may reject missing VWAP instead."

func (p *parser) parseTarget(line logicalLine, tokens []string) {
	if p.parseSetupTarget(line, tokens) {
		return
	}
	if containsLower(tokens, "pips") {
		if p.config["setupType"] != string(FamilyOpeningRangeBreakout) {
			p.err(line, "fixed-pip targets are currently supported only for opening range breakout", "target N R")
			return
		}
		target := copyMap(p.config["target"])
		pips := firstNumber(tokens, 1, 0)
		if !(pips > 0) {
			p.err(line, "target pips must be positive", "target 20 pips")
			return
		}
		target["pips"] = pips
		p.config["target"] = target
		return
	}
	if len(tokens) >= 2 && isNumberToken(tokens[1]) {
		target := copyMap(p.config["target"])
		value := parseNumber(tokens[1], 1)
		switch p.config["setupType"] {
		case string(FamilyElderTripleScreen):
			target["fallbackR"] = value
		case string(FamilyTriplePushExhaustion):
			target["fallbackR"] = value
			target["tpeR"] = value
		default:
			key := setupTargetKey(p.config["setupType"])
			if key == "" {
				key = "r"
			}
			target[key] = value
		}
		p.config["target"] = target
		return
	}
	if containsLower(tokens, "oppositerangeedge") {
		target := copyMap(p.config["target"])
		target["edge"] = "range"
		p.config["target"] = target
	}
}

func (p *parser) parseSetupTarget(line logicalLine, tokens []string) bool {
	if p.config["setupType"] == string(FamilyFairValueGap) && containsLower(tokens, "extension") {
		p.err(line, "fair value gap does not support Fibonacci target extensions; use target N R", "target N R")
		return true
	}
	if p.config["setupType"] == string(FamilyFibContinuation) && containsLower(tokens, "extension") {
		target := copyMap(p.config["target"])
		target["fibExt"] = firstNumber(tokens, 1, 1.618)
		target["fibR"] = 2
		p.config["target"] = target
		return true
	}
	if p.config["setupType"] == string(FamilyOpeningRangeBreakout) && containsLower(tokens, "range") {
		target := copyMap(p.config["target"])
		target["orbRange"] = firstNumber(tokens, 1, 1)
		target["orbR"] = 1
		p.config["target"] = target
		return true
	}
	if p.config["setupType"] == string(FamilyRangeBreakFake) && containsLower(tokens, "midpoint") {
		rbf := copyMap(p.config["rangeBreakFake"])
		rbf["targetMode"] = "rangeMid"
		p.config["rangeBreakFake"] = rbf
		return true
	}
	if p.config["setupType"] == string(FamilyRangeBreakFake) && containsLower(tokens, "vwap") {
		rbf := copyMap(p.config["rangeBreakFake"])
		rbf["targetMode"] = "vwap"
		p.config["rangeBreakFake"] = rbf
		p.warn(line, rangeBreakFakeVWAPTargetWarning, "")
		return true
	}
	if p.config["setupType"] == string(FamilyVWAPExtensionFade) && containsLower(tokens, "vwap") {
		vef := copyMap(p.config["vwapExtensionFade"])
		vef["targetMode"] = "vwap"
		p.config["vwapExtensionFade"] = vef
		return true
	}
	if p.config["setupType"] == string(FamilyVolumeAnomalyExhaustion) && containsLower(tokens, "vwap") {
		vae := copyMap(p.config["volumeAnomalyExhaustion"])
		vae["targetMode"] = "vwapElseFixedR"
		p.config["volumeAnomalyExhaustion"] = vae
		return true
	}
	return false
}

func (p *parser) parseTakeProfit(tokens []string) {
	target := copyMap(p.config["target"])
	if containsLower(tokens, "channel") {
		target["edge"] = "channel"
	} else {
		target["edge"] = "range"
	}
	p.config["target"] = target
}

func (p *parser) setTargetNumber(key string, value float64) {
	target := copyMap(p.config["target"])
	target[key] = value
	p.config["target"] = target
}

func (p *parser) parseBreakeven(tokens []string) {
	be := copyMap(p.config["breakeven"])
	if idx := indexOfLower(tokens, "after"); idx >= 0 {
		be["atR"] = firstNumber(tokens, idx+1, 0.75)
	}
	if idx := indexOfLower(tokens, "plus"); idx >= 0 {
		be["offsetAtr"] = firstNumber(tokens, idx+1, 0.02)
	}
	p.config["breakeven"] = be
}

func (p *parser) parseBreakevenAlias(tokens []string) {
	be := copyMap(p.config["breakeven"])
	if containsLower(tokens, "off") {
		be["atR"] = 0
		be["offsetAtr"] = 0
	} else {
		if idx := indexOfLower(tokens, "atr"); idx >= 0 {
			be["atR"] = firstNumber(tokens, idx+1, 0.75)
		}
		if idx := indexOfLower(tokens, "offsetatr"); idx >= 0 {
			be["offsetAtr"] = firstNumber(tokens, idx+1, 0.02)
		}
	}
	p.config["breakeven"] = be
}

func (p *parser) parsePartial(tokens []string) {
	partial := copyMap(p.config["partial"])
	partial["enabled"] = 1
	partial["fraction"] = parseFraction(tokenAt(tokens, 1), 0)
	if idx := indexOfLower(tokens, "at"); idx >= 0 {
		partial["triggerR"] = firstNumber(tokens, idx+1, 1)
	}
	if containsLower(tokens, "breakeven") {
		partial["moveBreakeven"] = 1
	}
	p.config["partial"] = partial
}

func (p *parser) parseTrail(tokens []string) {
	trail := copyMap(p.config["trail"])
	if containsLower(tokens, "off") {
		trail["atr"] = 0
	} else {
		trail["atr"] = firstNumber(tokens, 1, 0)
		if idx := indexOfLower(tokens, "after"); idx >= 0 {
			trail["triggerR"] = firstNumber(tokens, idx+1, 1)
		}
	}
	p.config["trail"] = trail
	p.elderTrailSet = true
}

func (p *parser) parseWait(tokens []string) {
	if p.config["setupType"] == string(FamilySupplyDemand) && containsLower(tokens, "zone") {
		sd := copyMap(p.config["supplyDemand"])
		sd["minWaitCandles"] = firstNumber(tokens, 1, 1)
		p.config["supplyDemand"] = sd
		return
	}
	value := firstNumber(tokens, 1, 3)
	p.config["cooldownCandles"] = value
	if p.config["setupType"] == string(FamilyElderTripleScreen) {
		p.config["maxHoldCandles"] = value
	}
}
