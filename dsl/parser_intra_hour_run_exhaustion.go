package dsl

import (
	"math"
	"strings"
)

const intraHourRunExhaustionPhrases = "intra-hour-run-exhaustion phrases are: run N candles | run hour candles N | run size min X ATR | run atr length N | exhaustion close location X percent | exhaustion require poke | exhaustion stop pad X ATR"

// parseIntraHourRunExhaustionRun handles the "run ..." directives of the
// intra-hour run exhaustion family.
func (p *parser) parseIntraHourRunExhaustionRun(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyIntraHourRunExhaustion) {
		p.unknownDirective(line, tokens[0])
		return
	}
	family := copyMap(p.config["intraHourRunExhaustion"])
	switch {
	case len(tokens) == 3 && strings.EqualFold(tokens[2], "candles"):
		value := firstNumber(tokens, 1, math.NaN())
		if !isPositiveInteger(value) {
			p.err(line, "run candles must be a positive integer", "")
			return
		}
		family["runCandles"] = value
	case len(tokens) == 4 && strings.EqualFold(tokens[1], "hour") && strings.EqualFold(tokens[2], "candles"):
		value := firstNumber(tokens, 3, math.NaN())
		if !isPositiveInteger(value) || value < 2 {
			p.err(line, "run hour candles must be an integer above 1", "")
			return
		}
		family["hourCandles"] = value
	case len(tokens) == 5 && strings.EqualFold(tokens[1], "size") && strings.EqualFold(tokens[2], "min") && strings.EqualFold(tokens[4], "ATR"):
		value := firstNumber(tokens, 3, math.NaN())
		if !isFiniteNumber(value) || value < 0 {
			p.err(line, "run size min ATR multiple must be finite and not negative", "")
			return
		}
		family["minRunAtr"] = value
	case len(tokens) == 4 && strings.EqualFold(tokens[1], "ATR") && strings.EqualFold(tokens[2], "length"):
		value := firstNumber(tokens, 3, math.NaN())
		if !isPositiveInteger(value) || value < 2 {
			p.err(line, "run atr length must be an integer above 1", "")
			return
		}
		family["atrLen"] = value
	default:
		p.err(line, intraHourRunExhaustionPhrases, "")
		return
	}
	p.config["intraHourRunExhaustion"] = family
}

// parseIntraHourRunExhaustionExhaustion handles the "exhaustion ..." directives
// of the intra-hour run exhaustion family.
func (p *parser) parseIntraHourRunExhaustionExhaustion(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyIntraHourRunExhaustion) {
		p.unknownDirective(line, tokens[0])
		return
	}
	family := copyMap(p.config["intraHourRunExhaustion"])
	switch {
	case len(tokens) == 5 && strings.EqualFold(tokens[1], "close") && strings.EqualFold(tokens[2], "location") && strings.EqualFold(tokens[4], "percent"):
		value := firstNumber(tokens, 3, math.NaN())
		if !isFiniteNumber(value) || value <= 0 || value >= 50 {
			p.err(line, "exhaustion close location percent must be finite, positive, and below 50", "")
			return
		}
		// Author-facing percentages accept both the canonical whole-percent
		// spelling (35 percent) and the fractional decimal spelling
		// (0.35 percent), matching the JS compiler.
		family["exhaustLocationPct"] = value
		if value > 1 {
			family["exhaustLocationPct"] = value / 100
		}
	case len(tokens) == 3 && strings.EqualFold(tokens[1], "require") && strings.EqualFold(tokens[2], "poke"):
		family["requirePoke"] = 1
	case len(tokens) == 5 && strings.EqualFold(tokens[1], "stop") && strings.EqualFold(tokens[2], "pad") && strings.EqualFold(tokens[4], "ATR"):
		value := firstNumber(tokens, 3, math.NaN())
		if !isFiniteNumber(value) || value < 0 {
			p.err(line, "exhaustion stop pad ATR multiple must be finite and not negative", "")
			return
		}
		family["stopPadAtr"] = value
		stop := copyMap(p.config["stop"])
		stop["paddingAtr"] = value
		p.config["stop"] = stop
	default:
		p.err(line, intraHourRunExhaustionPhrases, "")
		return
	}
	p.config["intraHourRunExhaustion"] = family
}

func isPositiveInteger(value float64) bool {
	return isFiniteNumber(value) && value > 0 && math.Trunc(value) == value
}
