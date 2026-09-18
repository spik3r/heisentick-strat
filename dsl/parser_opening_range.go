package dsl

import (
	"math"
	"strings"
)

func (p *parser) parseOpening(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyOpeningRangeBreakout) {
		return
	}
	orb := copyMap(p.config["openingRangeBreakout"])
	if len(tokens) >= 4 && strings.EqualFold(tokens[1], "range") {
		if containsLower(tokens, "every") && containsLower(tokens, "hours") && containsLower(tokens, "utc") {
			hours := firstNumber(tokens, 3, 0)
			if math.Trunc(hours) != hours || hours < 1 || hours > 24 {
				p.err(line, "opening range every must use an integer from 1 to 24 hours", "opening range every 3 hours UTC")
				return
			}
			orb["utcSlotMinutes"] = hours * 60
			orb["firstMinutes"] = float64(15)
			orb["firstCandles"] = float64(0)
		} else {
			orb["firstCandles"] = firstNumber(tokens, 2, 0)
		}
		if containsLower(tokens, "in") {
			sessions := lowerList(valuesAfterIn(tokens[3:]))
			orb["openingSessions"] = stringsToAny(sessions)
			p.setUserSessions(sessionMapFromNames(sessions))
		}
	}
	p.config["openingRangeBreakout"] = orb
}
