package dsl

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// donchianBreakoutAudit records the authored directives the family's
// post-parse step needs: which market gates the author wrote, and whether any
// stop or target phrase falls outside the family's supported shapes, or
// any enter directive was authored. Refusals remain sticky across later lines.
type donchianBreakoutAudit struct {
	sessionsAuthored bool
	dayTypeAuthored  bool
	movementAuthored bool
	otherStop        bool
	otherTarget      bool
	enterAuthored    bool
}

var donchianTargetR = regexp.MustCompile(`(?i)^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)R$`)

func (p *parser) recordDonchianBreakoutAuthored(line logicalLine, tokens []string, head string) {
	audit := &p.donchianAudit
	switch head {
	case "sessions", "windows", "trade":
		audit.sessionsAuthored = true
	case "regime":
		audit.dayTypeAuthored = true
	case "day":
		if len(tokens) > 1 && strings.EqualFold(tokens[1], "type") {
			audit.dayTypeAuthored = true
		}
	case "movement", "maxmovementer", "maxer", "trendiness":
		audit.movementAuthored = true
	case "enter":
		audit.enterAuthored = true
	case "stop":
		if !isDonchianATRStop(tokens) {
			audit.otherStop = true
		}
	}
	if (line.section == "target" || head == "target") && !(head == "target" && len(tokens) == 2 && isDonchianTargetR(tokens[1])) {
		audit.otherTarget = true
	}
}

// isDonchianTargetR admits one unsigned decimal component and an R suffix.
// Literal zero disables the target; a nonzero decimal that underflows must not.
func isDonchianTargetR(token string) bool {
	if !donchianTargetR.MatchString(token) {
		return false
	}
	value, err := strconv.ParseFloat(token[:len(token)-1], 64)
	if err != nil || !isFiniteNumber(value) {
		return false
	}
	if value == 0 {
		return !strings.ContainsAny(token, "123456789")
	}
	return value > 0
}

// isDonchianATRStop matches exactly `stop N ATR` with finite, positive N.
func isDonchianATRStop(tokens []string) bool {
	if len(tokens) != 3 || !strings.EqualFold(tokens[2], "atr") {
		return false
	}
	value, err := strconv.ParseFloat(tokens[1], 64)
	return err == nil && isFiniteNumber(value) && value > 0
}

// donchianCount parses the exact unsuffixed token. The exclusive power-of-two
// bound is representable even when float64(MaxInt) rounds up to that bound.
func donchianCount(token string) (float64, bool) {
	value, err := strconv.ParseFloat(token, 64)
	return value, err == nil && isFiniteNumber(value) && value >= 1 &&
		math.Trunc(value) == value && value < math.Ldexp(1, strconv.IntSize-1)
}

// parseDonchianBreakoutLine handles the setup-section phrases `channel N
// candles` and `trend ema N`. It reports whether the head belongs to the family.
func (p *parser) parseDonchianBreakoutLine(line logicalLine, tokens []string, head string) bool {
	if p.config["setupType"] != string(FamilyDonchianBreakout) || line.section != "setup" {
		return false
	}
	switch head {
	case "channel":
		value, valid := donchianCount(tokenAt(tokens, 1))
		if len(tokens) != 3 || tokens[2] != "candles" || !valid {
			p.err(line, "donchian-breakout channel must be exactly: channel N candles, with positive integer N", "")
			return true
		}
		p.setDonchianBreakoutValue("channelBars", value)
		return true
	case "trend":
		value, valid := donchianCount(tokenAt(tokens, 2))
		if len(tokens) != 3 || tokens[1] != "ema" || !valid {
			p.err(line, "donchian-breakout trend gate must be exactly: trend ema N, with positive integer N", "")
			return true
		}
		p.setDonchianBreakoutValue("trendEmaLen", value)
		return true
	}
	return false
}

// parseDonchianBreakoutStop writes the family's fixed ATR stop in the shape
// the JavaScript parser produces. Invalid exact ATR phrases are consumed so
// shared fallback parsing cannot put nonfinite values in the refused config.
// Other stop shapes retain their shared projections and post-parse refusal.
func (p *parser) parseDonchianBreakoutStop(tokens []string) bool {
	if p.config["setupType"] != string(FamilyDonchianBreakout) {
		return false
	}
	exactATR := len(tokens) == 3 && strings.EqualFold(tokens[2], "atr")
	// normalizeLine separates a leading plus. The phrase is still invalid,
	// but must not reach parseStop's `plus`/`+` padding fallback (e.g. +Inf).
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

func (p *parser) setDonchianBreakoutValue(key string, value float64) {
	family := copyMap(p.config["donchianBreakout"])
	family[key] = value
	p.config["donchianBreakout"] = family
}

// finalizeDonchianBreakout applies the family's market-gate defaults and its
// refusals. A breakout trend rule should not inherit the generic session
// windows, the ranging/choppy day-type gate or the 0.8 movement-efficiency
// cap, so unless the author writes one of those gates the family runs on
// every bar of its route.
func (p *parser) finalizeDonchianBreakout() {
	if p.config["setupType"] != string(FamilyDonchianBreakout) {
		return
	}
	audit := p.donchianAudit
	if _, utcRange := p.config["tradeWindowUTCHourRange"]; !audit.sessionsAuthored && !utcRange {
		p.config["tradeWindowMode"] = "unrestricted"
	}
	if !audit.dayTypeAuthored {
		p.config["dayTypes"] = []any{"trending", "ranging", "choppy"}
	}
	if !audit.movementAuthored {
		p.config["maxMovementEr"] = 1
	}

	family := copyMap(p.config["donchianBreakout"])
	if !(numberFromValue(family["channelBars"]) > 0) {
		p.errorAt(nil, nil, `donchian breakout requires "channel N candles".`, "")
	}
	stop := copyMap(p.config["stop"])
	stopATR := numberFromValue(stop["atrMult"])
	if audit.otherStop || stop["type"] != "atr" || !isFiniteNumber(stopATR) || !(stopATR > 0) {
		p.errorAt(nil, nil, "donchian breakout supports only a fixed ATR stop: stop N ATR.", "")
	}
	if audit.otherTarget {
		p.errorAt(nil, nil, "donchian breakout supports only target NR.", "")
	}
	if entry := copyMap(p.config["entryMode"]); audit.enterAuthored || entry["type"] != "market" {
		p.errorAt(nil, nil, "donchian breakout enters at the signal close; enter phrases are not supported.", "")
	}
	if source, ok := p.config["sourceTimeframe"].(string); ok && source != "" {
		p.errorAt(nil, nil, "donchian breakout does not support source timeframe execution.", "")
	}
}

func numberFromValue(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return math.NaN()
	}
}
