package dsl

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrParserUnimplemented = errors.New("dsl parser is not implemented")

func parse(source string) (ParseResult, error) {
	parser := newParser(source)
	parser.parse()
	return parser.result(), nil
}

type parser struct {
	source              string
	config              Config
	errors              []string
	warnings            []string
	diagnostics         []Diagnostic
	section             string
	userSessionsSet     bool
	userSideSet         bool
	elderTrailSet       bool
	dslVersion          int
	entryTfLine         *logicalLine
	dualEMAAudit        dualEMAParseAudit
	smaGoldenCrossAudit smaGoldenCrossParseAudit
}

type logicalLine struct {
	text    string
	line    int
	headCol int
	section string
}

func newParser(source string) *parser {
	return &parser{
		source:     source,
		config:     defaultConfig(),
		dslVersion: 6,
	}
}

func (p *parser) result() ParseResult {
	if p.config["setupType"] == string(FamilyElderTripleScreen) && p.elderTrailSet {
		trail := copyMap(p.config["trail"])
		elder := copyMap(p.config["elderTripleScreen"])
		elder["trailAtr"] = trail["atr"]
		elder["trailR"] = trail["triggerR"]
		p.config["elderTripleScreen"] = elder
	}
	p.config["sessions"] = normalizeSessionsValue(p.config["sessions"])
	p.validateTrendPullbackSessionVWAP()
	return ParseResult{
		Config:      p.config,
		Errors:      nonNilStrings(p.errors),
		Warnings:    nonNilStrings(p.warnings),
		Diagnostics: nonNilDiagnostics(p.diagnostics),
	}
}

func (p *parser) validateTrendPullbackSessionVWAP() {
	if p.config["setupType"] != string(FamilyTrendPullback) {
		return
	}
	tp := copyMap(p.config["trendPullback"])
	touch, _ := tp["sessionVwapTouch"].(string)
	firstOnly := firstNumber([]string{"", fmt.Sprint(tp["sessionVwapFirstTouchOnly"])}, 1, 0) != 0
	reclaim := firstNumber([]string{"", fmt.Sprint(tp["sessionVwapTrendSideReclaim"])}, 1, 0) != 0
	if touch != "" && (!firstOnly || !reclaim) {
		p.errorAt(nil, nil, "trend-pullback session VWAP touch requires both \"pullback first session VWAP touch only\" and \"reclaim session VWAP on trend side\".", "")
	}
	if touch == "" && (firstOnly || reclaim) {
		p.errorAt(nil, nil, "trend-pullback session VWAP first-touch/reclaim phrases require \"pullback session VWAP wick touch\" or \"pullback session VWAP close touch\".", "")
	}
}

func (p *parser) parse() {
	lines := joinWhenLines(expandPhysicalLines(p.source))
	for _, line := range lines {
		tokens := tokenize(normalizeLine(line.text))
		if len(tokens) == 0 {
			continue
		}
		p.apply(line, tokens)
	}
	p.validateLevels()
	p.validateEntryTimeframe()
	p.validatePriceMomentum()
	p.validateDailyFlushFailure()
	p.validateFairValueGap()
	p.validateDualEMAResumption()
	p.validateSMAGoldenCross()
}

func (p *parser) apply(line logicalLine, tokens []string) {
	head := strings.ToLower(tokens[0])
	p.recordDualEMAAuthored(line, tokens, head)
	p.recordSMAGoldenCrossAuthored(line, head)
	if head != "dsl" {
		p.reportMalformedNumbers(line, tokens)
		p.reportDeprecatedSpellings(line, tokens)
	}
	if p.tryKeltnerLine(line, head, tokens) {
		return
	}
	switch head {
	case "dsl":
		p.parseVersion(line, tokens)
	case "strategy", "name":
		p.setString("name", quotedTail(line.text, 1))
	case "description":
		p.setString("description", cleanFreeText(quotedTail(line.text, 0)))
	case "slices":
		p.parseSlices(tokens[1:])
	case "symbols":
		p.config["symbols"] = upperList(tokens[1:])
	case "timeframes":
		p.config["timeframes"] = lowerList(tokens[1:])
	case "sessions":
		p.parseSessions(tokens[1:])
	case "trade":
		p.parseTradeWindow(line, tokens)
	case "new":
		p.parseNewYorkHour(tokens)
	case "session":
		p.parseSession(tokens[1:])
	case "day":
		p.parseDay(tokens[1:])
	case "prior":
		p.parsePriorDay(tokens[1:])
	case "movement", "maxmovementer", "trendiness":
		p.parseMovement(tokens)
	case "micro":
		p.parseMicrostructure(line, tokens)
	case "approach":
		p.parseApproach(tokens)
	case "priority":
		p.parsePriority(tokens[1:])
	case "range":
		p.parseRange(line, tokens)
	case "channel":
		p.parseChannel(line, tokens)
	case "higher":
		p.parseHigherTimeframe(tokens)
	case "side", "direction":
		p.parseSide(tokens[1:])
	case "trigger", "rejection", "candle":
		p.parseTrigger(tokens)
	case "tail":
		p.parseTail(tokens)
	case "close":
		p.setNumber("closeLocationMin", firstNumber(tokens, 1, 0))
	case "entry":
		p.parseEntry(tokens)
	case "entrytf":
		p.parseEntryTimeframe(line, tokens)
	case "when":
		p.parseWhen(line, tokens)
	case "enter":
		p.parseEnter(tokens)
	case "stop":
		p.parseStopDirective(line, tokens)
	case "target":
		p.parseTargetDirective(line, tokens)
	case "take":
		p.parseTakeProfit(tokens)
	case "minimum", "minr":
		p.setTargetNumber("minR", firstNumber(tokens, 1, 0.6))
	case "fallback", "fallbackr":
		if p.config["setupType"] == string(FamilyDualEMAResumption) && !p.parseDualEMAResumption(tokens) {
			p.err(line, "unrecognized fallback line — dual-ema-resumption phrases only", "")
		} else if p.config["setupType"] != string(FamilyDualEMAResumption) {
			p.setTargetNumber("fallbackR", firstNumber(tokens, 1, 1))
		}
	case "move":
		p.parseBreakeven(tokens)
	case "breakeven":
		p.parseBreakevenAlias(tokens)
	case "partial":
		p.parsePartial(tokens)
	case "trail":
		if p.config["setupType"] == string(FamilyDualEMAResumption) && !p.parseDualEMAResumption(tokens) {
			p.err(line, "unrecognized trail line — dual-ema-resumption phrases only", "")
		} else if p.config["setupType"] != string(FamilyDualEMAResumption) {
			p.parseTrail(tokens)
		}
	case "wait":
		p.parseWait(tokens)
	case "maxholdcandles":
		p.setNumber("maxHoldCandles", firstNumber(tokens, 1, 0))
	case "maxholdbars":
		p.setNumber("maxHoldCandles", firstNumber(tokens, 1, 0))
	case "risk":
		p.parseRisk(tokens)
	case "riskusd":
		p.setNumber("riskUsd", firstNumber(tokens, 1, 200))
	case "context", "location", "require":
		p.parseGrade(tokens)
	case "size":
		p.parseGradeSize(tokens)
	case "near", "nearkeylevel":
		p.parseNear(tokens)
	case "recent":
		p.parseRecent(tokens)
	case "type":
		p.parseSetupType(tokens[1:])
	case "gap":
		p.parseFairValueGapGap(line, tokens)
	case "sweep":
		p.parseFairValueGapSweep(line, tokens)
	case "displacement":
		p.parseFairValueGapDisplacement(line, tokens)
	case "opening":
		p.parseOpening(line, tokens)
	case "inside":
		p.parseInside(tokens)
	case "pole":
		p.parseFlagPole(tokens)
	case "flag":
		p.parseFlag(tokens)
	case "break":
		p.parseBreak(tokens)
	case "retest":
		p.parseRetest(tokens)
	case "stretch":
		p.parseStretch(tokens)
	case "reclaim":
		p.parseReclaim(tokens)
	case "pivot":
		p.parsePivot(tokens)
	case "peak":
		p.parsePeak(tokens)
	case "neckline":
		p.parseNeckline(tokens)
	case "breakout":
		p.parseBreakout(tokens)
	case "confirm":
		p.parseConfirm(tokens)
	case "impulse", "departure":
		p.parseImpulse(tokens)
	case "retrace":
		p.parseRetrace(tokens)
	case "patterns":
		p.parsePatterns(tokens)
	case "base":
		p.parseBase(tokens)
	case "consolidation":
		p.parseConsolidation(tokens)
	case "source":
		if !p.parseSourceTimeframe(line, tokens) {
			p.parseSource(tokens)
		}
	case "choch", "structure":
		p.parseChoch(tokens)
	case "zone":
		p.parseZone(tokens)
	case "pierce":
		p.parsePierce(tokens)
	case "lookback":
		p.parseLookback(line, tokens)
	case "neutral":
		p.parsePriceMomentumNeutral(line, tokens)
	case "flush":
		p.parseDailyFlushFailure(line, tokens)
	case "weekend":
		p.parseWeekendExtremeFade(line, tokens)
	case "run":
		p.parseIntraHourRunExhaustionRun(line, tokens)
	case "exhaustion":
		p.parseIntraHourRunExhaustionExhaustion(line, tokens)
	case "min":
		p.parseMin(tokens)
	case "push":
		p.parsePush(tokens)
	case "decel":
		p.parseDecel(tokens)
	case "distance":
		p.parseDistance(tokens)
	case "volume":
		p.parseVolume(tokens)
	case "anomaly":
		p.parseAnomaly(tokens)
	case "high":
		p.parseHighEffort(tokens)
	case "or":
		p.parseOr(tokens)
	case "level":
		p.parseLevel(tokens)
	case "ema":
		if p.config["setupType"] == string(FamilyDualEMAResumption) && !p.parseDualEMAResumption(tokens) {
			p.err(line, "unrecognized ema line — dual-ema-resumption phrases only", "")
		} else if p.config["setupType"] != string(FamilyDualEMAResumption) {
			p.parseEMA(tokens)
		}
	case "sma":
		if line.section != "setup" {
			p.unknownDirective(line, tokens[0])
		} else if p.config["setupType"] == string(FamilySMAGoldenCross) && !p.parseSMAGoldenCross(tokens) {
			p.err(line, "unrecognized sma line — sma-golden-cross phrases only", "")
		} else if p.config["setupType"] != string(FamilySMAGoldenCross) {
			p.unknownDirective(line, tokens[0])
		}
	case "swing":
		p.parseSwing(tokens)
	case "pullback":
		p.parsePullback(tokens)
	case "hold":
		p.parseHoldDirective(line, tokens)
	default:
		p.unknownDirective(line, tokens[0])
	}
}

func (p *parser) parseVersion(line logicalLine, tokens []string) {
	if len(tokens) < 2 {
		return
	}
	version := strings.ToLower(tokens[1])
	if version == "v6" || version == "6" || version == "v7" || version == "7" {
		if version == "v7" || version == "7" {
			p.dslVersion = 7
		} else {
			p.dslVersion = 6
		}
		return
	}
	p.err(line, fmt.Sprintf("unsupported DSL version %q — this build supports v6/v7", tokens[1]), "")
}

func (p *parser) reportMalformedNumbers(line logicalLine, tokens []string) {
	if len(tokens) == 0 || strings.ToLower(tokens[0]) != "stop" {
		return
	}
	cues := map[string]bool{"last": true, "by": true, "+": true, "plus": true, "min": true, "max": true}
	reported := map[string]bool{}
	for i := 0; i+1 < len(tokens); i++ {
		if !cues[strings.ToLower(tokens[i])] {
			continue
		}
		token := tokens[i+1]
		if isNumberToken(token) || reported[token] {
			continue
		}
		reported[token] = true
		p.malformedNumber(line, token)
	}
}

func (p *parser) malformedNumber(line logicalLine, token string) {
	message := fmt.Sprintf("malformed number %q", token)
	if p.dslVersion >= 7 {
		p.err(line, message, "")
		return
	}
	p.warn(line, message, "")
}

func (p *parser) reportDeprecatedSpellings(line logicalLine, tokens []string) {
	if len(tokens) == 0 {
		return
	}
	head := strings.ToLower(tokens[0])
	if head == "description" || head == "name" || head == "strategy" {
		return
	}
	reported := map[string]bool{}
	report := func(token string, suggestion string) {
		if reported[token] {
			return
		}
		reported[token] = true
		p.deprecated(line, token, suggestion)
	}
	if head == "stop" && len(tokens) > 1 {
		switch tokens[1] {
		case "extreme":
			report("stop extreme", "stop beyond last N candle extreme by X ATR")
		case "recentExtreme":
			report("stop recentExtreme", "stop beyond last N candle extreme by X ATR")
		}
	}
	for _, token := range tokens {
		if suggestion, ok := generatedExactDeprecatedSpellings[token]; ok {
			report(token, suggestion)
			continue
		}
		if suggestion, ok := generatedDeprecatedSpellings[strings.ToLower(token)]; ok {
			report(token, suggestion)
		}
	}
}

func (p *parser) deprecated(line logicalLine, token string, suggestion string) {
	if p.dslVersion >= 7 {
		p.err(line, fmt.Sprintf("%q is deprecated in dsl v7 — did you mean %q?", token, suggestion), suggestion)
		return
	}
	p.warn(line, fmt.Sprintf("%q is deprecated — prefer %s.", token, suggestion), "")
}

func (p *parser) parseRisk(tokens []string) {
	if len(tokens) >= 2 && strings.EqualFold(tokens[1], "usd") {
		return
	}
	p.config["riskUsd"] = firstNumber(tokens, 1, 200)
}

func (p *parser) setString(key string, value string) {
	if value != "" {
		p.config[key] = value
	}
}

func (p *parser) setNumber(key string, value float64) {
	p.config[key] = value
}

func (p *parser) validateLevels() {
	levels, ok := p.config["levelPriority"].([]any)
	if !ok {
		return
	}
	for _, raw := range levels {
		level, _ := raw.(string)
		if !isKnownLevel(level) {
			p.errorAt(nil, nil, fmt.Sprintf("unknown level %q in priority(...) — known: PDH, PDL, PDO, PDC, DO, DH, DL, WH, WL, AH, AL, LH, LL, NH, NL, range.high, range.low, CAM_R3, CAM_R4, CAM_S3, CAM_S4, channel.high, channel.low, VWAP, EMA, POC, VAH, VAL, RN<step> (round numbers), or a defined S/R/TL name", level), "")
		}
	}
}

func (p *parser) unknownDirective(line logicalLine, head string) {
	suggestion := nearestDirective(head)
	message := fmt.Sprintf("unknown directive %q", head)
	if suggestion != "" {
		message += fmt.Sprintf(" — did you mean %q?", suggestion)
	}
	p.err(line, message, suggestion)
}

func (p *parser) err(line logicalLine, message string, suggestion string) {
	lineNo := line.line
	col := line.headCol
	p.errorAt(&lineNo, &col, message, suggestion)
}

func (p *parser) warn(line logicalLine, message string, suggestion string) {
	lineNo := line.line
	col := line.headCol
	full := fmt.Sprintf("line %d: %s", lineNo, message)
	p.warnings = append(p.warnings, full)
	p.diagnostics = append(p.diagnostics, Diagnostic{
		Severity:   DiagnosticWarning,
		Message:    message,
		Line:       &lineNo,
		Column:     &col,
		Suggestion: suggestion,
	})
}

func (p *parser) errorAt(line *int, col *int, message string, suggestion string) {
	if line != nil {
		p.errors = append(p.errors, fmt.Sprintf("line %d: %s", *line, message))
	} else {
		p.errors = append(p.errors, message)
	}
	p.diagnostics = append(p.diagnostics, Diagnostic{
		Severity:   DiagnosticError,
		Message:    message,
		Line:       line,
		Column:     col,
		Suggestion: suggestion,
	})
}

func defaultConfig() Config {
	return Config{
		"allowLong":             1,
		"allowShort":            1,
		"approachCandles":       3,
		"approachDistanceAtr":   0,
		"blockedLocalHours":     []any{},
		"blockedLocalWeekdays":  []any{},
		"blockedPriorDayTypes":  []any{},
		"blockedSessionPhases":  []any{},
		"breakeven":             map[string]any{"atR": 0.75, "offsetAtr": 0.02},
		"channel":               map[string]any{"activeWithinCandles": 12, "directions": []any{}, "enabled": false, "tradeMaxWidthAtr": 0, "tradeMinWidthAtr": 0},
		"channelBreakHold":      map[string]any{},
		"closeLocationMin":      0,
		"cooldownCandles":       3,
		"customLevels":          []any{},
		"dayOpen":               map[string]any{},
		"dayThemes":             []any{},
		"dayTypeErEscape":       0.55,
		"dayTypes":              []any{"ranging", "choppy"},
		"description":           "",
		"doubleTopBottom":       map[string]any{},
		"elderTripleScreen":     map[string]any{},
		"emaLen":                0,
		"entryMode":             map[string]any{"expireCandles": 5, "type": "market"},
		"entryTf":               "current",
		"fairValueGap":          map[string]any{},
		"fibContinuation":       map[string]any{},
		"flag":                  map[string]any{},
		"grade":                 map[string]any{"context": 0, "location": 0, "requireTotal": 0, "riskReward": 0, "sizeTiers": []any{}, "trigger": 0},
		"htf":                   map[string]any{"mode": "off", "timeframe": "auto"},
		"levelConfluenceAtr":    0,
		"levelDistanceAtr":      1.5,
		"levelPriority":         []any{"PDH", "PDL", "WH", "WL"},
		"levelPriorityExplicit": false,
		"localHours":            []any{},
		"localWeekdays":         []any{},
		"newYorkHours":          []any{},
		"blockedNewYorkHours":   []any{},
		"maxHoldCandles":        0,
		"maxMovementEr":         0.8,
		"name":                  "",
		"openLocations":         []any{},
		"openingRangeBreakout":  map[string]any{},
		"priceMomentum":         map[string]any{},
		"partial":               map[string]any{"enabled": 0, "fraction": 0, "moveBreakeven": 0, "triggerR": 1},
		"priorDayTypes":         []any{},
		"range":                 map[string]any{"activeWithinCandles": 8, "method": nil},
		"rangeBreakFake":        map[string]any{},
		"rangeStatFilters":      []any{},
		"riskUsd":               200,
		"seasonalityFilters":    []any{},
		"sessionBiasFilters":    []any{},
		"sessionPhases":         []any{},
		"sessions":              map[string]any{"asia": 1, "london": 1, "mid": 0, "ny": 1},
		"setupType":             string(FamilyFailedBreakout),
		"slices":                []any{},
		"stop":                  map[string]any{"extremeCandles": 3, "maxAtr": 1.5, "minAtr": 0, "paddingAtr": 0.25},
		"supplyDemand":          map[string]any{},
		"sweepRules": map[string]any{
			"high": map[string]any{"reclaimCandles": 3, "side": "short", "sweepDistanceAtr": 0.1},
			"low":  map[string]any{"reclaimCandles": 3, "side": "long", "sweepDistanceAtr": 0.1},
		},
		"symbols":                 []any{},
		"tailRejectionMin":        0,
		"target":                  map[string]any{"edge": "range", "fallbackR": 1, "minR": 0.6},
		"timeframes":              []any{"5m", "15m"},
		"tradeWindowMinuteRange":  nil,
		"tradeWindowSegments":     []any{},
		"trail":                   map[string]any{"atr": 0, "triggerR": 1},
		"trendPullback":           map[string]any{},
		"trigger":                 map[string]any{"maxEntryDistanceAtr": 0, "maxSetupAgeCandles": 0},
		"triggerCandles":          []any{"pin", "engulf", "outside"},
		"triggerExplicit":         false,
		"triplePush":              map[string]any{},
		"volumeAnomalyExhaustion": map[string]any{},
		"vp":                      []any{},
		"vwapExtensionFade":       map[string]any{},
	}
}

func quotedTail(text string, skipLeadingTokens int) string {
	if start := strings.IndexAny(text, `"'`); start >= 0 {
		quote := text[start]
		if end := strings.LastIndexByte(text[start+1:], quote); end >= 0 {
			return text[start+1 : start+1+end]
		}
	}
	tokens := strings.Fields(normalizeLine(text))
	if len(tokens) <= skipLeadingTokens+1 {
		return ""
	}
	return strings.Join(tokens[skipLeadingTokens+1:], " ")
}

func cleanFreeText(text string) string {
	text = strings.ReplaceAll(text, ":", "")
	text = strings.ReplaceAll(text, ",", "")
	text = strings.ReplaceAll(text, "(", " ")
	text = strings.ReplaceAll(text, ")", " ")
	return strings.Join(strings.Fields(text), " ")
}

func canonicalSetupFamily(tokens []string) string {
	phrase := strings.ToLower(strings.Join(tokens, " "))
	if family := fairValueGapSetupFamilies[phrase]; family != "" {
		return family
	}
	return generatedCanonicalSetupFamilies[phrase]
}

func copyMap(value any) map[string]any {
	source, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	out := map[string]any{}
	for key, value := range source {
		out[key] = value
	}
	return out
}

func normalizeLevel(token string) string {
	upperLevels := map[string]bool{"pdh": true, "pdl": true, "pdo": true, "pdc": true, "do": true, "dh": true, "dl": true, "wh": true, "wl": true, "ah": true, "al": true, "lh": true, "ll": true, "nh": true, "nl": true, "vwap": true, "ema": true, "poc": true, "vah": true, "val": true}
	lower := strings.ToLower(token)
	if upperLevels[lower] || strings.HasPrefix(strings.ToUpper(token), "CAM_") || strings.HasPrefix(strings.ToUpper(token), "RN") {
		return strings.ToUpper(token)
	}
	return token
}

func isKnownLevel(level string) bool {
	known := map[string]bool{
		"PDH": true, "PDL": true, "PDO": true, "PDC": true, "DO": true, "DH": true, "DL": true,
		"WH": true, "WL": true, "AH": true, "AL": true, "LH": true, "LL": true, "NH": true, "NL": true,
		"range.high": true, "range.low": true, "CAM_R3": true, "CAM_R4": true, "CAM_S3": true, "CAM_S4": true,
		"channel.high": true, "channel.low": true, "VWAP": true, "EMA": true, "POC": true, "VAH": true, "VAL": true,
	}
	return known[level] || strings.HasPrefix(level, "RN")
}

func nearestDirective(head string) string {
	best := ""
	bestDistance := 3
	for _, known := range directiveStartsForSection("") {
		distance := levenshtein(strings.ToLower(head), strings.ToLower(known))
		if distance < bestDistance {
			bestDistance = distance
			best = known
		}
	}
	return best
}

func levenshtein(a string, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ar := range a {
		curr := make([]int, len(b)+1)
		curr[0] = i + 1
		for j, br := range b {
			cost := 0
			if ar != br {
				cost = 1
			}
			curr[j+1] = min(min(curr[j]+1, prev[j+1]+1), prev[j]+cost)
		}
		prev = curr
	}
	return prev[len(b)]
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilDiagnostics(values []Diagnostic) []Diagnostic {
	if values == nil {
		return []Diagnostic{}
	}
	return values
}

func canonicalJSON(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(bytes)
}
