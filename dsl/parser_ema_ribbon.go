package dsl

import (
	"math"
	"strconv"
	"strings"
)

// emaRibbonAudit records authored directives the family's post-parse step
// refuses. The family owns its own entry window and trend filter, so the
// shared session, day-type, movement and higher-timeframe gates are refused
// rather than silently combined with them. Refusals stay sticky.
type emaRibbonAudit struct {
	marketGateAuthored bool
	otherStop          bool
	otherTarget        bool
	enterAuthored      bool
	managementAuthored bool
}

// emaRibbonHTFTimeframes are the trend-filter buckets the family builds from
// its own bars (UTC-aligned, so a 4h bucket starts at 00, 04, ... UTC).
var emaRibbonHTFTimeframes = map[string]float64{"2h": 2, "4h": 4, "8h": 8, "1d": 24}

// recordEMARibbonAuthored audits every authored line, including market
// conditions written before the setup type, so a refused gate cannot be
// silently overridden by the family defaults.
func (p *parser) recordEMARibbonAuthored(line logicalLine, tokens []string, head string) {
	audit := &p.emaRibbonAudit
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
	case "move", "trail", "breakeven", "maxholdcandles", "wait", "partial":
		audit.managementAuthored = true
	}
	if (line.section == "target" || head == "target") && !(head == "target" && len(tokens) == 2 && isDonchianTargetR(tokens[1])) {
		audit.otherTarget = true
	}
}

// emaRibbonCount parses a positive integer token.
func emaRibbonCount(token string) (int, bool) {
	value, ok := donchianCount(token)
	if !ok || value > 100000 {
		return 0, false
	}
	return int(value), true
}

// emaRibbonHour parses a whole UTC hour 0..23.
func emaRibbonHour(token string) (int, bool) {
	value, err := strconv.Atoi(token)
	return value, err == nil && value >= 0 && value <= 23
}

// parseEMARibbonLine handles the setup-section phrases:
//
//	ribbon ema N1 N2 ... Nk          (2..8 strictly increasing lengths)
//	pullback ema N                   (optional re-entry while aligned)
//	trend filter TF ribbon           (optional, TF in 2h 4h 8h 1d)
//	trend filter TF ema F above S    (optional, F < S)
//	shorts below daily ema N         (optional short-side daily gate)
//	act hours A to B utc             (optional entry and signal-exit window)
//
// It reports whether the head belongs to the family.
func (p *parser) parseEMARibbonLine(line logicalLine, tokens []string, head string) bool {
	if p.config["setupType"] != string(FamilyEMARibbon) {
		return false
	}
	if line.section != "setup" {
		// A family phrase in another section would otherwise reach a shared
		// handler and be dropped without effect.
		if isEMARibbonPhrase(tokens, head) {
			p.err(line, "ema-ribbon phrases (ribbon, pullback ema, trend filter, shorts below, act hours) belong in the setup section", "")
			return true
		}
		return false
	}
	switch head {
	case "ribbon":
		if len(tokens) < 4 || len(tokens) > 10 || !strings.EqualFold(tokens[1], "ema") {
			p.err(line, "ema-ribbon ribbon must be exactly: ribbon ema N1 N2 ..., with 2 to 8 strictly increasing positive integers", "")
			return true
		}
		lengths := make([]any, 0, len(tokens)-2)
		previous := 0
		for _, token := range tokens[2:] {
			value, ok := emaRibbonCount(token)
			if !ok || value <= previous {
				p.err(line, "ema-ribbon ribbon must be exactly: ribbon ema N1 N2 ..., with 2 to 8 strictly increasing positive integers", "")
				return true
			}
			previous = value
			lengths = append(lengths, float64(value))
		}
		p.setEMARibbonValue("emas", lengths)
		return true
	case "pullback":
		value, ok := emaRibbonCount(tokenAt(tokens, 2))
		if len(tokens) != 3 || !strings.EqualFold(tokens[1], "ema") || !ok {
			p.err(line, "ema-ribbon pullback must be exactly: pullback ema N, with positive integer N", "")
			return true
		}
		p.setEMARibbonValue("pullbackEma", float64(value))
		return true
	case "trend":
		tf := strings.ToLower(tokenAt(tokens, 2))
		_, tfOK := emaRibbonHTFTimeframes[tf]
		if len(tokens) < 4 || !strings.EqualFold(tokens[1], "filter") || !tfOK {
			p.err(line, "ema-ribbon trend filter must be: trend filter TF ribbon, or trend filter TF ema F above S, with TF one of 2h 4h 8h 1d", "")
			return true
		}
		if len(tokens) == 4 && strings.EqualFold(tokens[3], "ribbon") {
			p.setEMARibbonValue("trendFilter", map[string]any{"timeframe": tf, "mode": "ribbon"})
			return true
		}
		fast, fastOK := emaRibbonCount(tokenAt(tokens, 4))
		slow, slowOK := emaRibbonCount(tokenAt(tokens, 6))
		if len(tokens) == 7 && strings.EqualFold(tokens[3], "ema") && strings.EqualFold(tokens[5], "above") && fastOK && slowOK && fast < slow {
			p.setEMARibbonValue("trendFilter", map[string]any{"timeframe": tf, "mode": "pair", "fast": float64(fast), "slow": float64(slow)})
			return true
		}
		p.err(line, "ema-ribbon trend filter must be: trend filter TF ribbon, or trend filter TF ema F above S, with TF one of 2h 4h 8h 1d and F < S", "")
		return true
	case "shorts":
		value, ok := emaRibbonCount(tokenAt(tokens, 4))
		if len(tokens) != 5 || !strings.EqualFold(tokens[1], "below") || !strings.EqualFold(tokens[2], "daily") || !strings.EqualFold(tokens[3], "ema") || !ok {
			p.err(line, "ema-ribbon short gate must be exactly: shorts below daily ema N, with positive integer N", "")
			return true
		}
		p.setEMARibbonValue("shortDailyEma", float64(value))
		return true
	case "act":
		from, fromOK := emaRibbonHour(tokenAt(tokens, 2))
		to, toOK := emaRibbonHour(tokenAt(tokens, 4))
		if len(tokens) != 6 || !strings.EqualFold(tokens[1], "hours") || !strings.EqualFold(tokens[3], "to") || !strings.EqualFold(tokens[5], "utc") || !fromOK || !toOK || from == to {
			p.err(line, "ema-ribbon window must be exactly: act hours A to B utc, with whole hours 0-23 and A different from B", "")
			return true
		}
		p.setEMARibbonValue("actFromUtc", float64(from))
		p.setEMARibbonValue("actToUtc", float64(to))
		return true
	}
	return false
}

// isEMARibbonPhrase reports whether a line has the shape of an ema-ribbon
// setup phrase.
func isEMARibbonPhrase(tokens []string, head string) bool {
	second := strings.ToLower(tokenAt(tokens, 1))
	switch head {
	case "ribbon":
		return second == "ema"
	case "pullback":
		return second == "ema"
	case "trend":
		return second == "filter"
	case "shorts":
		return second == "below"
	case "act":
		return second == "hours"
	}
	return false
}

// parseEMARibbonStop writes the family's fixed ATR stop. It mirrors the
// donchian-breakout stop handling: exact `stop N ATR` only, and an invalid
// exact phrase is consumed so the shared fallback cannot write it.
func (p *parser) parseEMARibbonStop(tokens []string) bool {
	if p.config["setupType"] != string(FamilyEMARibbon) {
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

func (p *parser) setEMARibbonValue(key string, value any) {
	family := copyMap(p.config["emaRibbon"])
	family[key] = value
	p.config["emaRibbon"] = family
}

// finalizeEMARibbon applies the family defaults and refusals. The family runs
// on every bar of its route (its own act-hours window is the only time gate)
// and owns its exits, so shared market gates and management phrases are
// refused instead of being combined with rules they were not tested with.
func (p *parser) finalizeEMARibbon() {
	if p.config["setupType"] != string(FamilyEMARibbon) {
		return
	}
	audit := p.emaRibbonAudit
	p.config["tradeWindowMode"] = "unrestricted"
	p.config["dayTypes"] = []any{"trending", "ranging", "choppy"}
	p.config["maxMovementEr"] = 1

	family := copyMap(p.config["emaRibbon"])
	emas, _ := family["emas"].([]any)
	if len(emas) < 2 {
		p.errorAt(nil, nil, `ema ribbon requires "ribbon ema N1 N2 ...".`, "")
	}
	stop := copyMap(p.config["stop"])
	stopATR := numberFromValue(stop["atrMult"])
	if audit.otherStop || stop["type"] != "atr" || !isFiniteNumber(stopATR) || !(stopATR > 0) {
		p.errorAt(nil, nil, "ema ribbon supports only a fixed ATR stop: stop N ATR.", "")
	}
	if audit.otherTarget {
		p.errorAt(nil, nil, "ema ribbon supports only target NR.", "")
	}
	if audit.marketGateAuthored {
		p.errorAt(nil, nil, `ema ribbon sets its own time window ("act hours A to B utc") and trend filter; session, day-type, movement and higher-timeframe gates are not supported.`, "")
	}
	if audit.managementAuthored {
		p.errorAt(nil, nil, "ema ribbon exits when the ribbon loses alignment; breakeven, trail, partial, wait and max-hold phrases are not supported.", "")
	}
	if entry := copyMap(p.config["entryMode"]); audit.enterAuthored || entry["type"] != "market" {
		p.errorAt(nil, nil, "ema ribbon enters at the next bar open; enter phrases are not supported.", "")
	}
	if source, ok := p.config["sourceTimeframe"].(string); ok && source != "" {
		p.errorAt(nil, nil, "ema ribbon does not support source timeframe execution.", "")
	}
	if _, hasFrom := family["actFromUtc"]; hasFrom {
		if math.IsNaN(numberFromValue(family["actToUtc"])) {
			p.errorAt(nil, nil, "ema ribbon act hours needs both hours.", "")
		}
	}
}
