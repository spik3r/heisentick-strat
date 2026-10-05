package dsl

import (
	"math"
	"strconv"
	"strings"
)

func (p *parser) parseOpening(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyOpeningRangeBreakout) {
		return
	}
	orb := copyMap(p.config["openingRangeBreakout"])
	badShape := func() {
		p.err(line, "opening range must use first N candles/minutes or every N hours UTC, optionally followed by in (sessions)", "opening range first 15 minutes in (london)")
	}
	if len(tokens) < 5 || !strings.EqualFold(tokens[1], "range") {
		badShape()
		return
	}
	// Read the quantity at its grammar position, never a later number or a
	// number with an unrelated R/% suffix. Both engine range modes use ints.
	value, err := strconv.ParseFloat(tokens[3], 64)
	suffix := 5
	switch strings.ToLower(tokens[2]) {
	case "first":
		if err != nil || !isFiniteNumber(value) || value <= 0 || math.Trunc(value) != value || value >= math.Exp2(strconv.IntSize-1) {
			p.err(line, "opening range first must use a positive integer number of candles or minutes", "opening range first 15 minutes")
			return
		}
		switch strings.ToLower(tokens[4]) {
		case "minute", "minutes":
			orb["firstMinutes"] = value
			orb["firstCandles"] = float64(0)
		case "candle", "candles", "bar", "bars":
			orb["firstCandles"] = value
			orb["firstMinutes"] = float64(0)
		default:
			badShape()
			return
		}
	case "every":
		if len(tokens) < 6 || !strings.EqualFold(tokens[4], "hours") || !strings.EqualFold(tokens[5], "utc") {
			badShape()
			return
		}
		if err != nil || !isFiniteNumber(value) || math.Trunc(value) != value || value < 1 || value > 24 {
			p.err(line, "opening range every must use an integer from 1 to 24 hours", "opening range every 3 hours UTC")
			return
		}
		orb["utcSlotMinutes"] = value * 60
		orb["firstMinutes"] = float64(15)
		orb["firstCandles"] = float64(0)
		suffix = 6
	default:
		badShape()
		return
	}
	if len(tokens) > suffix {
		if len(tokens) < suffix+2 || !strings.EqualFold(tokens[suffix], "in") {
			badShape()
			return
		}
		sessions := lowerList(tokens[suffix+1:])
		orb["openingSessions"] = stringsToAny(sessions)
		p.setUserSessions(sessionMapFromNames(sessions))
	}
	p.config["openingRangeBreakout"] = orb
}
