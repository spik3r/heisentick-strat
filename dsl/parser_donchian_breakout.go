package dsl

import (
	"math"
	"regexp"
	"strings"
)

// donchianBreakoutAudit records the authored directives the family's
// post-parse step needs: which market gates the author wrote, and whether any
// stop or target phrase falls outside the family's supported shapes.
type donchianBreakoutAudit struct {
	sessionsAuthored bool
	dayTypeAuthored  bool
	movementAuthored bool
	otherStop        bool
	otherTarget      bool
}

var donchianTargetR = regexp.MustCompile(`(?i)^[\d.]+R$`)

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
	case "stop":
		if !isDonchianATRStop(tokens) {
			audit.otherStop = true
		}
	}
	if line.section == "target" && !(head == "target" && len(tokens) == 2 && donchianTargetR.MatchString(tokens[1])) {
		audit.otherTarget = true
	}
}

// isDonchianATRStop matches exactly `stop N ATR` with positive N.
func isDonchianATRStop(tokens []string) bool {
	if len(tokens) != 3 || !strings.EqualFold(tokens[2], "atr") || !isNumberToken(tokens[1]) {
		return false
	}
	return parseNumber(tokens[1], 0) > 0
}

// parseDonchianBreakoutLine handles the setup-section phrases `channel N
// candles` and `trend ema N`. It reports whether the head belongs to the family.
func (p *parser) parseDonchianBreakoutLine(line logicalLine, tokens []string, head string) bool {
	if p.config["setupType"] != string(FamilyDonchianBreakout) || line.section != "setup" {
		return false
	}
	switch head {
	case "channel":
		value := firstNumber(tokens, 1, math.NaN())
		if len(tokens) != 3 || tokens[2] != "candles" || !isPositiveInteger(value) {
			p.err(line, "donchian-breakout channel must be exactly: channel N candles, with positive integer N", "")
			return true
		}
		p.setDonchianBreakoutValue("channelBars", value)
		return true
	case "trend":
		value := firstNumber(tokens, 2, math.NaN())
		if len(tokens) != 3 || tokens[1] != "ema" || !isPositiveInteger(value) {
			p.err(line, "donchian-breakout trend gate must be exactly: trend ema N, with positive integer N", "")
			return true
		}
		p.setDonchianBreakoutValue("trendEmaLen", value)
		return true
	}
	return false
}

// parseDonchianBreakoutStop writes the family's fixed ATR stop in the shape
// the JavaScript parser produces. Other stop shapes fall through to the shared
// parser and are refused after parsing.
func (p *parser) parseDonchianBreakoutStop(tokens []string) bool {
	if p.config["setupType"] != string(FamilyDonchianBreakout) || !isDonchianATRStop(tokens) {
		return false
	}
	stop := copyMap(p.config["stop"])
	stop["type"] = "atr"
	stop["atrMult"] = parseNumber(tokens[1], 0)
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
	if audit.otherStop || stop["type"] != "atr" || !(numberFromValue(stop["atrMult"]) > 0) {
		p.errorAt(nil, nil, "donchian breakout supports only a fixed ATR stop: stop N ATR.", "")
	}
	if audit.otherTarget {
		p.errorAt(nil, nil, "donchian breakout supports only target NR.", "")
	}
	if entry := copyMap(p.config["entryMode"]); entry["type"] != "market" {
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
