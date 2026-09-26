package dsl

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type microFeatureDefinition struct {
	phrase     []string
	feature    string
	capability string
}

var microFeatureDefinitions = []microFeatureDefinition{
	{[]string{"spread", "ticks"}, "spreadTicks", "tickSize"},
	{[]string{"spread", "bps"}, "spreadBps", "bidAsk"},
	{[]string{"liquidity", "imbalance"}, "liquidityImbalance", "l1Size"},
	{[]string{"weighted", "mid", "displacement"}, "weightedMidDisplacement", "l1Size"},
	{[]string{"quote", "flow", "imbalance"}, "quoteFlowImbalance", "l1Size"},
	{[]string{"quote", "rate"}, "quoteRate", "eventTicks"},
	{[]string{"realised", "volatility"}, "realisedVolatility", "eventTicks"},
}

func tokensMatch(tokens []string, start int, phrase []string) bool {
	if start+len(phrase) > len(tokens) {
		return false
	}
	for index, word := range phrase {
		if !strings.EqualFold(tokens[start+index], word) {
			return false
		}
	}
	return true
}

func microWindowMilliseconds(token string, unitToken string) (float64, bool) {
	valueToken := strings.ToLower(token)
	unit := strings.ToLower(unitToken)
	for _, suffix := range []string{"ms", "s", "m"} {
		if strings.HasSuffix(valueToken, suffix) {
			unit = suffix
			valueToken = strings.TrimSuffix(valueToken, suffix)
			break
		}
	}
	value, err := strconv.ParseFloat(valueToken, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	scale := float64(0)
	switch unit {
	case "ms", "millisecond", "milliseconds":
		scale = 1
	case "s", "second", "seconds":
		scale = 1000
	case "m", "minute", "minutes":
		scale = 60000
	}
	if scale == 0 {
		return 0, false
	}
	return value * scale, true
}

func (p *parser) parseMicrostructure(line logicalLine, tokens []string) {
	var definition *microFeatureDefinition
	for index := range microFeatureDefinitions {
		candidate := &microFeatureDefinitions[index]
		if tokensMatch(tokens, 1, candidate.phrase) {
			definition = candidate
			break
		}
	}
	if definition == nil {
		p.err(line, "micro filter must name spread ticks, spread bps, liquidity imbalance, weighted mid displacement, quote flow imbalance, quote rate, or realised volatility", "")
		return
	}
	start := 1 + len(definition.phrase)
	op := ""
	valueIndex := -1
	if start+1 < len(tokens) && strings.EqualFold(tokens[start], "at") && strings.EqualFold(tokens[start+1], "most") {
		op, valueIndex = "atMost", start+2
	} else if start+1 < len(tokens) && strings.EqualFold(tokens[start], "at") && strings.EqualFold(tokens[start+1], "least") {
		op, valueIndex = "atLeast", start+2
	} else if start < len(tokens) {
		switch strings.ToLower(tokens[start]) {
		case "above", "over", "greater":
			op, valueIndex = "above", start+1
		case "below", "under", "less":
			op, valueIndex = "below", start+1
		}
	}
	if op == "" || valueIndex >= len(tokens) {
		p.err(line, "micro filter comparison must be above N, below N, at least N, or at most N", "")
		return
	}
	value, err := strconv.ParseFloat(tokens[valueIndex], 64)
	if err != nil {
		p.err(line, "micro filter needs a finite threshold", "")
		return
	}
	if (definition.feature == "liquidityImbalance" || definition.feature == "weightedMidDisplacement") && (value < -1 || value > 1) {
		p.err(line, fmt.Sprintf("%s threshold must be between -1 and 1", strings.Join(definition.phrase, " ")), "")
		return
	}
	filter := map[string]any{
		"feature": definition.feature, "capability": definition.capability, "op": op, "value": value,
	}
	overIndex := -1
	for index := valueIndex + 1; index < len(tokens); index++ {
		if strings.EqualFold(tokens[index], "over") {
			overIndex = index
			break
		}
	}
	if overIndex >= 0 {
		unit := ""
		if overIndex+2 < len(tokens) {
			unit = tokens[overIndex+2]
		}
		window, ok := microWindowMilliseconds(tokenAt(tokens, overIndex+1), unit)
		if !ok {
			p.err(line, "micro filter window must be a positive duration such as 5s or 1 minute", "")
			return
		}
		filter["windowMs"] = window
	}
	if (definition.feature == "quoteFlowImbalance" || definition.feature == "quoteRate" || definition.feature == "realisedVolatility") && overIndex < 0 {
		p.err(line, fmt.Sprintf("%s requires an explicit window, for example \"over 5s\"", strings.Join(definition.phrase, " ")), "")
		return
	}
	filters, _ := p.config["microstructureFilters"].([]any)
	p.config["microstructureFilters"] = append(filters, filter)
}

func (p *parser) parseSourceTimeframe(line logicalLine, tokens []string) bool {
	if len(tokens) < 2 || !strings.EqualFold(tokens[1], "timeframe") {
		return false
	}
	if p.dslVersion < 7 {
		p.err(line, "source timeframe is available only in dsl v7", "")
		return true
	}
	if line.section != "setup" && line.section != "filters" {
		p.err(line, "source timeframe is only allowed in setup { ... } or filters { ... }", "")
		return true
	}
	if len(tokens) != 3 || !isTimeframe(tokens[2]) {
		p.err(line, "source timeframe must be exactly one of 1m, 5m, 15m, 30m, 1h, 4h, 1d", "")
		return true
	}
	p.config["sourceTimeframe"] = strings.ToLower(tokens[2])
	return true
}

func (p *parser) parseSlices(tokens []string) {
	slices := []any{}
	seen := map[string]bool{}
	for i := 0; i+1 < len(tokens); i += 2 {
		symbol := strings.ToUpper(tokens[i])
		tf := strings.ToLower(tokens[i+1])
		key := symbol + "\x00" + tf
		if seen[key] {
			continue
		}
		seen[key] = true
		slices = append(slices, map[string]any{"symbol": symbol, "tf": tf})
	}
	p.config["slices"] = slices
}

func (p *parser) parseSessions(tokens []string) {
	sessions := map[string]any{"asia": 0, "mid": 0, "london": 0, "ny": 0}
	for _, token := range tokens {
		key := strings.ToLower(token)
		if _, ok := sessions[key]; ok {
			sessions[key] = 1
		}
	}
	p.setUserSessions(sessions)
}

func (p *parser) parseTradeWindow(line logicalLine, tokens []string) {
	if len(tokens) < 3 || !strings.EqualFold(tokens[1], "window") {
		return
	}
	if strings.EqualFold(tokens[2], "unrestricted") {
		p.config["tradeWindowMode"] = "unrestricted"
		return
	}
	if strings.EqualFold(tokens[2], "minutes") {
		if len(tokens) != 6 || !strings.EqualFold(tokens[4], "to") {
			p.err(line, "trade window minutes must use an increasing range, e.g. trade window minutes 0 to 90", "")
			return
		}
		from, fromErr := strconv.ParseFloat(tokens[3], 64)
		to, toErr := strconv.ParseFloat(tokens[5], 64)
		if fromErr != nil || toErr != nil || from < 0 || to <= from {
			p.err(line, "trade window minutes must use an increasing range, e.g. trade window minutes 0 to 90", "")
			return
		}
		p.config["tradeWindowMinuteRange"] = map[string]any{"from": from, "to": to}
		return
	}
	if !strings.EqualFold(tokens[2], "in") {
		if strings.EqualFold(tokens[2], "minute") {
			p.err(line, "trade window minutes must use the plural form and an increasing range, e.g. trade window minutes 0 to 90", "")
		}
		return
	}
	segments := make([]any, 0, len(tokens)-3)
	seen := map[string]bool{}
	for index := 3; index < len(tokens); index++ {
		segment := normalizeTradeWindowSegment(tokens[index])
		if segment == "" && index+1 < len(tokens) {
			segment = normalizeTradeWindowSegment(tokens[index] + "." + tokens[index+1])
			if segment != "" {
				index++
			}
		}
		if segment == "" {
			p.err(line, fmt.Sprintf("trade window segment %q must be one of asia/mid/london/ny plus .open/.middle/.close/.all", tokens[index]), "")
			continue
		}
		if seen[segment] {
			continue
		}
		seen[segment] = true
		segments = append(segments, segment)
	}
	if len(segments) > 0 {
		p.config["tradeWindowSegments"] = segments
	}
}

func normalizeTradeWindowSegment(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "_", ".")
	value = strings.ReplaceAll(value, "-", ".")
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return ""
	}
	validWindow := map[string]bool{"asia": true, "mid": true, "london": true, "ny": true}
	aliases := map[string]string{"first": "open", "second": "middle", "third": "close", "full": "all"}
	part := parts[1]
	if alias := aliases[part]; alias != "" {
		part = alias
	}
	validPart := map[string]bool{"open": true, "middle": true, "close": true, "all": true}
	if !validWindow[parts[0]] || !validPart[part] {
		return ""
	}
	return parts[0] + "." + part
}

func (p *parser) parseNewYorkHour(tokens []string) {
	if len(tokens) < 4 || !strings.EqualFold(tokens[1], "york") || !strings.EqualFold(tokens[2], "hour") {
		return
	}
	values := valuesAfterIn(tokens[3:])
	if len(values) == 0 {
		return
	}
	hours := make([]any, 0, len(values))
	for _, value := range values {
		hour, err := strconv.Atoi(value)
		if err != nil || hour < 0 || hour >= 24 {
			return
		}
		hours = append(hours, hour)
	}
	if containsLower(tokens, "not") {
		p.config["blockedNewYorkHours"] = hours
	} else {
		p.config["newYorkHours"] = hours
	}
}

func (p *parser) setUserSessions(sessions map[string]any) {
	p.config["sessions"] = normalizeSessionsValue(sessions)
	p.userSessionsSet = true
}

func (p *parser) applyDefaultSessions(sessions map[string]any) {
	if p.userSessionsSet {
		return
	}
	p.config["sessions"] = normalizeSessionsValue(sessions)
}

func (p *parser) parseSession(tokens []string) {
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], "phase") {
		p.config["sessionPhases"] = stringsToAny(lowerList(valuesAfterIn(tokens[1:])))
	}
}

func (p *parser) parseDay(tokens []string) {
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], "type") {
		values := valuesAfterIn(tokens[1:])
		if len(values) > 0 {
			p.config["dayTypes"] = lowerListUntil(values, "or", "strictly")
		}
		if idx := indexOfLower(tokens, "movement"); idx >= 0 {
			p.config["dayTypeErEscape"] = firstNumber(tokens[idx:], 0, 0.55)
		} else if containsLower(tokens, "strictly") {
			p.config["dayTypeErEscape"] = -1
		}
		return
	}
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], "theme") {
		p.config["dayThemes"] = lowerList(valuesAfterIn(tokens[1:]))
	}
}

func (p *parser) parsePriorDay(tokens []string) {
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], "day") && strings.EqualFold(tokens[1], "range") {
		value := firstNumber(tokens, 2, math.NaN())
		if !math.IsNaN(value) {
			priorDay := copyMap(p.config["priorDay"])
			priorDay["minRangeAtr"] = value
			p.config["priorDay"] = priorDay
		}
		return
	}
	if len(tokens) < 3 || !strings.EqualFold(tokens[0], "day") || !strings.EqualFold(tokens[1], "type") {
		return
	}
	values := lowerList(valuesAfterIn(tokens[2:]))
	if containsLower(tokens, "not") {
		p.config["blockedPriorDayTypes"] = values
	} else {
		p.config["priorDayTypes"] = values
	}
}

func (p *parser) parseMovement(tokens []string) {
	if p.parseSetupMovement(tokens) {
		return
	}
	if len(tokens) == 0 {
		return
	}
	p.config["maxMovementEr"] = firstNumber(tokens, 0, 0.8)
}

func (p *parser) parseApproach(tokens []string) {
	if containsLower(tokens, "candles") {
		p.config["approachCandles"] = firstNumber(tokens, 1, 3)
		return
	}
	p.config["approachDistanceAtr"] = firstNumber(tokens, 1, 0)
}

func (p *parser) parsePriority(tokens []string) {
	levels := []any{}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		levels = append(levels, normalizeLevel(token))
	}
	p.config["levelPriority"] = levels
	p.config["levelPriorityExplicit"] = true
}

func (p *parser) parseRange(line logicalLine, tokens []string) {
	rangeCfg := copyMap(p.config["range"])
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "method") {
		rangeCfg["method"] = strings.ToLower(tokens[2])
	}
	if len(tokens) >= 4 && strings.EqualFold(tokens[1], "active") {
		rangeCfg["activeWithinCandles"] = firstNumber(tokens, 2, 8)
	}
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "edge") {
		p.config["levelDistanceAtr"] = firstNumber(tokens, 2, 1.5)
	}
	p.config["range"] = rangeCfg
	_ = line
}

func (p *parser) parseChannel(line logicalLine, tokens []string) {
	channel := copyMap(p.config["channel"])
	channel["enabled"] = true
	if len(tokens) >= 4 && strings.EqualFold(tokens[1], "active") {
		channel["activeWithinCandles"] = firstNumber(tokens, 2, 12)
	} else if len(tokens) >= 3 && strings.EqualFold(tokens[1], "width") {
		switch strings.ToLower(tokens[2]) {
		case "between":
			channel["tradeMinWidthAtr"] = firstNumber(tokens, 3, 0)
			if idx := indexOfLower(tokens, "and"); idx >= 0 {
				channel["tradeMaxWidthAtr"] = firstNumber(tokens, idx+1, 0)
			}
		case "at":
			channel["tradeMinWidthAtr"] = firstNumber(tokens, 3, 0)
		case "below":
			channel["tradeMaxWidthAtr"] = firstNumber(tokens, 3, 0)
		default:
			p.err(line, "channel width must be: channel width between X and Y ATR | channel width at least X ATR | channel width below X ATR", "")
		}
	} else if len(tokens) >= 3 && strings.EqualFold(tokens[1], "direction") {
		directions := lowerList(valuesAfterIn(tokens[2:]))
		if len(directions) == 1 && directions[0] == "directional" {
			directions = []string{"ascending", "descending"}
		}
		channel["directions"] = stringsToAny(directions)
	}
	p.config["channel"] = channel
}

func (p *parser) parseHigherTimeframe(tokens []string) {
	htf := copyMap(p.config["htf"])
	htf["timeframe"] = "auto"
	if containsLower(tokens, "off") {
		htf["mode"] = "off"
	} else if containsLower(tokens, "agree") {
		htf["mode"] = "notAgainst"
	} else {
		htf["mode"] = "notAgainst"
	}
	for _, token := range tokens {
		if isTimeframe(token) || token == "daily" || token == "weekly" {
			htf["timeframe"] = strings.ToLower(token)
		}
	}
	p.config["htf"] = htf
}

func (p *parser) parseSide(tokens []string) {
	p.userSideSet = true
	text := strings.ToLower(strings.Join(tokens, " "))
	switch {
	case strings.Contains(text, "long"):
		p.config["allowLong"] = 1
		p.config["allowShort"] = 0
	case strings.Contains(text, "short"):
		p.config["allowLong"] = 0
		p.config["allowShort"] = 1
	case strings.Contains(text, "both"):
		p.config["allowLong"] = 1
		p.config["allowShort"] = 1
	}
}

func (p *parser) parseTrigger(tokens []string) {
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], "trigger") && strings.EqualFold(tokens[1], "any") {
		p.config["triggerCandles"] = []any{"any"}
		p.config["triggerExplicit"] = true
		return
	}
	if len(tokens) >= 3 && (strings.EqualFold(tokens[0], "trigger") || strings.EqualFold(tokens[0], "rejection")) {
		p.config["triggerCandles"] = stringsToAny(lowerList(valuesAfterIn(tokens[2:])))
		p.config["triggerExplicit"] = true
		return
	}
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], "candle") {
		if p.parseCandleMorphology(tokens) {
			return
		}
		p.config["triggerCandles"] = stringsToAny(lowerList(valuesAfterIn(tokens[1:])))
		p.config["triggerExplicit"] = true
		return
	}
	if len(tokens) >= 4 && strings.EqualFold(tokens[0], "setup") && strings.EqualFold(tokens[1], "expires") {
		trigger := copyMap(p.config["trigger"])
		trigger["maxSetupAgeCandles"] = firstNumber(tokens, 2, 0)
		p.config["trigger"] = trigger
	}
}

// parseNoConfirmationCandle handles `no confirmation candle` — a clearer
// synonym for `candle in (any)` / `trigger any` for a setup whose original
// has no candle-shape requirement at all, rather than "accepts any shape".
// Shared trigger vocabulary, not family-locked (spec §2.4).
func (p *parser) parseNoConfirmationCandle(line logicalLine, tokens []string) {
	if len(tokens) >= 3 && strings.EqualFold(tokens[1], "confirmation") && strings.EqualFold(tokens[2], "candle") {
		p.config["triggerCandles"] = []any{"any"}
		p.config["triggerExplicit"] = true
		return
	}
	p.unknownDirective(line, tokens[0])
}

func (p *parser) parseWhen(line logicalLine, tokens []string) {
	if p.config["setupType"] == string(FamilyNamedLevelSweep) {
		p.parseNamedLevelSweepWhen(line, tokens)
		return
	}
	if !containsLower(tokens, "range.high") && !containsLower(tokens, "range.low") &&
		!containsLower(tokens, "channel.high") && !containsLower(tokens, "channel.low") {
		p.err(line, "when rule must sweep range.high/range.low or channel.high/channel.low", "")
		return
	}
	if containsLower(tokens, "signal") && containsLower(tokens, "grade") {
		sweepRules := copyMap(p.config["sweepRules"])
		grade := tokenAt(tokens, indexOfLower(tokens, "grade")+1)
		if containsLower(tokens, "range.high") {
			rule := copyMap(sweepRules["high"])
			rule["source"] = "range"
			if grade != "" {
				rule["grade"] = grade
			}
			sweepRules["high"] = rule
		}
		if containsLower(tokens, "range.low") {
			rule := copyMap(sweepRules["low"])
			rule["source"] = "range"
			if grade != "" {
				rule["grade"] = grade
			}
			sweepRules["low"] = rule
		}
		p.config["sweepRules"] = sweepRules
	}
}
