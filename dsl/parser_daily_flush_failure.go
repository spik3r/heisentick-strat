package dsl

import (
	"math"
	"strings"
)

func (p *parser) validateDailyFlushFailure() {
	if p.config["setupType"] != string(FamilyDailyFlushFailure) {
		return
	}
	family := copyMap(p.config["dailyFlushFailure"])
	if _, ok := family["minFlushRangeAtr"]; !ok {
		p.errorAt(nil, nil, `daily flush failure requires "flush range at least X ATR".`, "")
	}
	if _, ok := family["maxFlushCloseLocation"]; !ok {
		p.errorAt(nil, nil, `daily flush failure requires "flush close in bottom X percent".`, "")
	}
}

func (p *parser) parseDailyFlushFailure(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyDailyFlushFailure) {
		p.unknownDirective(line, tokens[0])
		return
	}
	family := copyMap(p.config["dailyFlushFailure"])
	if len(tokens) == 6 && strings.EqualFold(tokens[1], "range") &&
		strings.EqualFold(tokens[2], "at") && strings.EqualFold(tokens[3], "least") &&
		strings.EqualFold(tokens[5], "ATR") {
		value := firstNumber(tokens, 4, math.NaN())
		if !isFiniteNumber(value) || value <= 0 {
			p.err(line, "flush range ATR multiple must be finite and positive", "")
			return
		}
		family["minFlushRangeAtr"] = value
		p.config["dailyFlushFailure"] = family
		return
	}
	if len(tokens) == 6 && strings.EqualFold(tokens[1], "close") &&
		strings.EqualFold(tokens[2], "in") && strings.EqualFold(tokens[3], "bottom") &&
		strings.EqualFold(tokens[5], "percent") {
		value := firstNumber(tokens, 4, math.NaN())
		if !isFiniteNumber(value) || value <= 0 || value > 100 {
			p.err(line, "flush close percent must be finite, positive, and at most 100", "")
			return
		}
		family["maxFlushCloseLocation"] = value / 100
		p.config["dailyFlushFailure"] = family
		return
	}
	p.err(line, "daily-flush-failure phrases are: flush range at least X ATR | flush close in bottom X percent", "")
}
