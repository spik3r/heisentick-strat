package dsl

import (
	"math"
	"strings"
)

func (p *parser) parseStopDirective(line logicalLine, tokens []string) {
	if p.config["setupType"] == string(FamilyDualEMAResumption) && !p.parseDualEMAResumption(tokens) {
		p.err(line, "unrecognized stop line — dual-ema-resumption phrases only", "")
	} else if p.config["setupType"] == string(FamilyWeekendExtremeFade) {
		p.parseWeekendExtremeFade(line, tokens)
	} else if p.config["setupType"] == string(FamilyNamedLevelSweep) && p.parseNamedLevelSweepStopLine(line, tokens) {
		// handled
	} else if p.config["setupType"] != string(FamilyDualEMAResumption) {
		p.parseStop(line, tokens)
	}
}

func (p *parser) parseTargetDirective(line logicalLine, tokens []string) {
	if p.config["setupType"] == string(FamilyWeekendExtremeFade) {
		p.parseWeekendExtremeFade(line, tokens)
	} else {
		p.parseTarget(line, tokens)
	}
}

func (p *parser) parseHoldDirective(line logicalLine, tokens []string) {
	if p.config["setupType"] == string(FamilyWeekendExtremeFade) {
		p.parseWeekendExtremeFade(line, tokens)
	} else {
		p.parseHold(tokens)
	}
}

func (p *parser) parseWeekendExtremeFade(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyWeekendExtremeFade) {
		p.unknownDirective(line, tokens[0])
		return
	}
	family := copyMap(p.config["weekendExtremeFade"])
	if len(tokens) == 5 && strings.EqualFold(tokens[1], "range") && strings.EqualFold(tokens[2], "max") && strings.EqualFold(tokens[4], "ATR") {
		value := firstNumber(tokens, 3, math.NaN())
		if !isFiniteNumber(value) || value <= 0 {
			p.err(line, "weekend range ATR multiple must be finite and positive", "")
			return
		}
		family["maxWeekendRangeAtr"] = value
		p.config["weekendExtremeFade"] = family
		return
	}
	if len(tokens) == 5 && strings.EqualFold(tokens[1], "close") && (strings.EqualFold(tokens[2], "extreme") || strings.EqualFold(tokens[2], "location")) && strings.EqualFold(tokens[4], "percent") {
		value := firstNumber(tokens, 3, math.NaN())
		if !isFiniteNumber(value) || value <= 0 || value >= 50 {
			p.err(line, "weekend close extreme percent must be finite, positive, and below 50", "")
			return
		}
		// Author-facing percentages accept both the canonical whole-percent
		// spelling (30 percent) and the fractional decimal spelling
		// (0.3 percent). Keep values already below one as fractions so the Go
		// parser matches the JS compiler.
		family["closeExtremePct"] = value
		if value > 1 {
			family["closeExtremePct"] = value / 100
		}
		p.config["weekendExtremeFade"] = family
		return
	}
	if len(tokens) == 3 && strings.EqualFold(tokens[1], "ATR") {
		value := firstNumber(tokens, 2, math.NaN())
		if !isFiniteNumber(value) || value <= 0 {
			p.err(line, "weekend ATR value must be finite and positive", "")
			return
		}
		switch strings.ToLower(tokens[0]) {
		case "stop":
			family["stopAtr"] = value
			stop := copyMap(p.config["stop"])
			stop["atrMult"] = value
			p.config["stop"] = stop
		case "target":
			family["targetAtr"] = value
		default:
			p.err(line, "weekend-extreme-fade ATR phrases only", "")
			return
		}
		p.config["weekendExtremeFade"] = family
		return
	}
	if len(tokens) == 4 && strings.EqualFold(tokens[1], "max") && strings.EqualFold(tokens[3], "candles") {
		value := firstNumber(tokens, 2, math.NaN())
		if !isFiniteNumber(value) || value <= 0 || math.Trunc(value) != value {
			p.err(line, "weekend hold bars must be a positive integer", "")
			return
		}
		p.config["maxHoldCandles"] = value
		family["maxHoldBars"] = value
		p.config["weekendExtremeFade"] = family
		return
	}
	p.err(line, "weekend-extreme-fade phrases are: weekend range max X ATR | weekend close extreme X percent | stop ATR X | target ATR X | hold max X candles", "")
}
