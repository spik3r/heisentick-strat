package dsl

import (
	"strconv"
	"strings"
)

// clockFadeAudit records authored directives the family's post-parse step
// refuses. The family owns its clock gate and its exit, so shared session,
// day-type, movement and higher-timeframe gates and the shared management
// phrases are refused rather than combined with rules they were not tested
// with. Refusals stay sticky.
type clockFadeAudit struct {
	marketGateAuthored bool
	otherStop          bool
	targetAuthored     bool
	enterAuthored      bool
	managementAuthored bool
}

// recordClockFadeAuthored audits every authored line, including market
// conditions written before the setup type.
func (p *parser) recordClockFadeAuthored(line logicalLine, tokens []string, head string) {
	audit := &p.clockFadeAudit
	switch head {
	case "sessions", "windows", "trade", "regime", "movement", "maxmovementer", "maxer", "trendiness", "higher", "htf", "session", "new", "local", "weekday", "hour", "prior", "open":
		audit.marketGateAuthored = true
	case "day":
		if len(tokens) > 1 && strings.EqualFold(tokens[1], "type") {
			audit.marketGateAuthored = true
		}
	case "enter":
		audit.enterAuthored = true
	case "stop":
		if !isDonchianATRStop(tokens) {
			audit.otherStop = true
		}
	case "move", "trail", "breakeven", "wait", "partial":
		audit.managementAuthored = true
	case "target":
		audit.targetAuthored = true
	}
	if line.section == "target" {
		audit.targetAuthored = true
	}
}

// clockFadeMinute parses a strict HH:MM wall-clock token into minutes after
// midnight.
func clockFadeMinute(token string) (int, bool) {
	if len(token) != 5 || token[2] != ':' {
		return 0, false
	}
	hour, hourErr := strconv.Atoi(token[:2])
	minute, minuteErr := strconv.Atoi(token[3:])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

// isClockFadePhrase reports whether a line has the shape of a clock-fade
// setup phrase.
func isClockFadePhrase(tokens []string, head string) bool {
	switch head {
	case "signal":
		return strings.EqualFold(tokenAt(tokens, 1), "at")
	case "lookback":
		return true
	}
	return false
}

// parseClockFadeLine handles the setup-section phrases:
//
//	lookback N candles               (trailing move, N >= 1)
//	signal at new york HH:MM         (open time of the signal bar)
//
// It reports whether the head belongs to the family.
func (p *parser) parseClockFadeLine(line logicalLine, tokens []string, head string) bool {
	if p.config["setupType"] != string(FamilyClockFade) {
		return false
	}
	if line.section != "setup" {
		if isClockFadePhrase(tokens, head) {
			p.err(line, "clock-fade phrases (lookback, signal at new york) belong in the setup section", "")
			return true
		}
		return false
	}
	switch head {
	case "lookback":
		value, ok := donchianCount(tokenAt(tokens, 1))
		if len(tokens) != 3 || !strings.EqualFold(tokens[2], "candles") || !ok || value > 100000 {
			p.err(line, "clock-fade lookback must be exactly: lookback N candles, with positive integer N", "")
			return true
		}
		p.setClockFadeValue("lookbackBars", value)
		return true
	case "signal":
		minute, ok := clockFadeMinute(tokenAt(tokens, 4))
		if len(tokens) != 5 || !strings.EqualFold(tokens[1], "at") || !strings.EqualFold(tokens[2], "new") || !strings.EqualFold(tokens[3], "york") || !ok {
			p.err(line, "clock-fade signal must be exactly: signal at new york HH:MM, with a 24-hour time", "")
			return true
		}
		p.setClockFadeValue("newYorkMinute", float64(minute))
		return true
	}
	return false
}

// parseClockFadeStop writes the family's fixed ATR stop. Only the exact
// `stop N ATR` phrase is accepted; an invalid exact phrase is consumed so the
// shared fallback cannot write it.
func (p *parser) parseClockFadeStop(tokens []string) bool {
	if p.config["setupType"] != string(FamilyClockFade) {
		return false
	}
	exactATR := len(tokens) == 3 && strings.EqualFold(tokens[2], "atr")
	separatedPlusATR := len(tokens) == 4 && tokens[1] == "+" && strings.EqualFold(tokens[3], "atr")
	if !exactATR && !separatedPlusATR {
		return false
	}
	if !isDonchianATRStop(tokens) {
		return true
	}
	value, _ := strconv.ParseFloat(tokens[1], 64) // checked by isDonchianATRStop
	stop := copyMap(p.config["stop"])
	stop["type"] = "atr"
	stop["atrMult"] = value
	p.config["stop"] = stop
	return true
}

func (p *parser) setClockFadeValue(key string, value any) {
	family := copyMap(p.config["clockFade"])
	family[key] = value
	p.config["clockFade"] = family
}

// finalizeClockFade applies the family defaults and refusals. The family
// evaluates every bar of its route; its own clock phrase is the only time
// gate.
func (p *parser) finalizeClockFade() {
	if p.config["setupType"] != string(FamilyClockFade) {
		return
	}
	audit := p.clockFadeAudit
	p.config["tradeWindowMode"] = "unrestricted"
	p.config["dayTypes"] = []any{"trending", "ranging", "choppy"}
	p.config["maxMovementEr"] = 1

	family := copyMap(p.config["clockFade"])
	if _, ok := family["lookbackBars"]; !ok {
		p.errorAt(nil, nil, `clock fade requires "lookback N candles".`, "")
	}
	if _, ok := family["newYorkMinute"]; !ok {
		p.errorAt(nil, nil, `clock fade requires "signal at new york HH:MM".`, "")
	}
	stop := copyMap(p.config["stop"])
	stopATR := numberFromValue(stop["atrMult"])
	if audit.otherStop || stop["type"] != "atr" || !isFiniteNumber(stopATR) || !(stopATR > 0) {
		p.errorAt(nil, nil, "clock fade supports only a fixed ATR stop: stop N ATR.", "")
	}
	if audit.targetAuthored {
		p.errorAt(nil, nil, "clock fade has no target; it exits after maxHoldCandles or at the stop.", "")
	}
	if audit.marketGateAuthored {
		p.errorAt(nil, nil, `clock fade sets its own time gate ("signal at new york HH:MM"); session, day-type, movement, clock and higher-timeframe gates are not supported.`, "")
	}
	if audit.managementAuthored {
		p.errorAt(nil, nil, "clock fade exits after maxHoldCandles; breakeven, trail, partial and wait phrases are not supported.", "")
	}
	if entry := copyMap(p.config["entryMode"]); audit.enterAuthored || entry["type"] != "market" {
		p.errorAt(nil, nil, "clock fade enters at market; enter phrases are not supported.", "")
	}
	if source, ok := p.config["sourceTimeframe"].(string); ok && source != "" {
		p.errorAt(nil, nil, "clock fade does not support source timeframe execution.", "")
	}
	hold := numberFromValue(p.config["maxHoldCandles"])
	if !isFiniteNumber(hold) || hold < 1 || hold != float64(int(hold)) {
		p.errorAt(nil, nil, "clock fade requires maxHoldCandles N with positive integer N (default 1).", "")
	}
}
