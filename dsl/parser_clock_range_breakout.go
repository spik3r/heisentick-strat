package dsl

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Clock range breakout parsing. The family's phrases are recorded as the
// document is read and validated once at the end, so the result does not
// depend on the order of sections or on whether `type:` comes first. Existing
// families keep their previous handling of the shared heads (`range`, `close`,
// `stop`); the family directives are errors anywhere else.

var (
	clockWallPattern   = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])$`)
	clockOffsetPattern = regexp.MustCompile(`(?i)^clock\s+UTC([+-])([0-9]{1,2})(?::([0-9]{2}))?$`)
	clockNumberPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)
)

// clockRangeInlineStarts are directive heads that only begin a new directive
// in an inline `{ ... }` body when followed by their family-specific keyword.
// The gate keeps other families' inline splitting unchanged.
var clockRangeInlineStarts = []struct {
	head string
	next *regexp.Regexp
}{
	{"clock", regexp.MustCompile(`(?i)^\s*utc`)},
	{"orders", regexp.MustCompile(`(?i)^\s*expire\b`)},
	{"buffer", regexp.MustCompile(`(?i)^\s*\S+\s+pips\b`)},
}

type clockRangeDirective struct {
	line   logicalLine
	tokens []string
}

type clockRangeParse struct {
	clock   []clockRangeDirective
	rng     []clockRangeDirective
	expire  []clockRangeDirective
	buffer  []clockRangeDirective
	closeAt []clockRangeDirective
	stop    []clockRangeDirective
}

func (c clockRangeParse) familyLines() []logicalLine {
	var lines []logicalLine
	for _, group := range [][]clockRangeDirective{c.clock, c.rng, c.expire, c.buffer, c.closeAt} {
		for _, directive := range group {
			lines = append(lines, directive.line)
		}
	}
	return lines
}

// recordClockRangeDirective stores a family phrase for end-of-document
// validation. It reports whether the directive belongs to the family's
// exclusive spellings (the shared heads `range`, `close` and `stop` fall
// through to their existing handlers only when they are not the family form).
func (p *parser) recordClockRangeDirective(line logicalLine, tokens []string) bool {
	directive := clockRangeDirective{line: line, tokens: tokens}
	head := strings.ToLower(tokens[0])
	switch {
	case head == "clock":
		p.clockRange.clock = append(p.clockRange.clock, directive)
	case head == "orders":
		p.clockRange.expire = append(p.clockRange.expire, directive)
	case head == "buffer":
		p.clockRange.buffer = append(p.clockRange.buffer, directive)
	case head == "range" && len(tokens) > 1 && startsWithDigit(tokens[1]):
		p.clockRange.rng = append(p.clockRange.rng, directive)
	case head == "close" && len(tokens) > 1 && strings.EqualFold(tokens[1], "positions"):
		p.clockRange.closeAt = append(p.clockRange.closeAt, directive)
	case head == "stop" && len(tokens) == 3 && strings.EqualFold(tokens[2], "percent"):
		// Not family-exclusive: other families treat this line as before.
		p.clockRange.stop = append(p.clockRange.stop, directive)
		return false
	default:
		return false
	}
	return true
}

func startsWithDigit(token string) bool {
	return token != "" && token[0] >= '0' && token[0] <= '9'
}

func parseClockWall(token string) (int, bool) {
	match := clockWallPattern.FindStringSubmatch(token)
	if match == nil {
		return 0, false
	}
	hours, _ := strconv.Atoi(match[1])
	minutes, _ := strconv.Atoi(match[2])
	return hours*60 + minutes, true
}

type clockNumberKind int

const (
	clockNumberOK clockNumberKind = iota
	clockNumberMalformed
	clockNumberNonfinite
)

func parseClockNumber(token string) (float64, clockNumberKind) {
	lower := strings.ToLower(strings.TrimLeft(token, "+-"))
	switch lower {
	case "nan", "inf", "infinity":
		return 0, clockNumberNonfinite
	}
	negative := strings.HasPrefix(token, "-")
	digits := strings.TrimPrefix(strings.TrimPrefix(token, "-"), "+")
	if !clockNumberPattern.MatchString(digits) {
		return 0, clockNumberMalformed
	}
	value, err := strconv.ParseFloat(digits, 64)
	if err != nil || math.IsInf(value, 0) {
		return 0, clockNumberNonfinite
	}
	if negative {
		value = -value
	}
	return value, clockNumberOK
}

// validateClockRangeBreakout runs after all lines were applied.
func (p *parser) validateClockRangeBreakout() {
	if p.config["setupType"] != string(FamilyClockRangeBreakout) {
		for _, line := range p.clockRange.familyLines() {
			p.err(line, "clock range breakout directives require type: clock range breakout", "")
		}
		return
	}
	p.scanClockRangeSource()
	p.applyClockRangeNeutralConfig()

	clock, clockOK := p.parseClockOffset()
	start, end, rangeOK := p.parseClockRangeWindow()
	closeMinute, closeOK := p.parseClockWallDirective(p.clockRange.closeAt, "close positions at", "close positions at <HH:MM>")
	expire, expireOK := closeMinute, closeOK
	if len(p.clockRange.expire) > 0 {
		expire, expireOK = p.parseClockExpire()
	}
	buffer, bufferOK := p.parseClockBuffer()
	percent, percentOK := p.parseClockStopPercent()
	if !clockOK || !rangeOK || !closeOK || !expireOK || !bufferOK || !percentOK {
		return
	}

	p.config["clock"] = map[string]any{"utcOffsetMinutes": clock}
	p.config["clockRangeBreakout"] = map[string]any{
		"rangeStartMinute": start, "rangeEndMinute": end,
		"expireMinute": expire, "closeMinute": closeMinute, "bufferPips": buffer,
	}
	p.config["stop"] = map[string]any{
		"type": "percent", "percent": percent,
		"extremeCandles": 0, "maxAtr": nil, "minAtr": 0, "paddingAtr": 0,
	}
	spec, err := DecodeClockRangeBreakout(p.config)
	if err != nil {
		p.errorAt(nil, nil, "clock range breakout: "+err.Error(), "")
		return
	}
	p.validateClockRangeRoutes(spec)
}

func (p *parser) applyClockRangeNeutralConfig() {
	p.config["sessions"] = map[string]any{"asia": 1, "london": 1, "mid": 1, "ny": 1}
	p.config["tradeWindowMode"] = "unrestricted"
	p.config["dayTypes"] = []any{}
	p.config["maxMovementEr"] = 1
	p.config["triggerCandles"] = []any{"any"}
	p.config["cooldownCandles"] = 0
	p.config["maxHoldCandles"] = 0
	p.config["closeLocationMin"] = 0
	p.config["breakeven"] = map[string]any{"atR": nil, "offsetAtr": 0}
	p.config["target"] = map[string]any{}
	p.config["stop"] = map[string]any{"extremeCandles": 0, "maxAtr": nil, "minAtr": 0, "paddingAtr": 0}
	p.config["clockRangeBreakout"] = map[string]any{}
}

// scanClockRangeSource rejects every authored directive outside the family's
// v1 surface, whatever section or order it appears in.
func (p *parser) scanClockRangeSource() {
	p.auditClockRangeInlineBodies()
	types := 0
	for _, line := range joinWhenLines(expandPhysicalLines(p.source)) {
		tokens := tokenize(normalizeLine(line.text))
		if len(tokens) == 0 {
			continue
		}
		head := strings.ToLower(tokens[0])
		reject := func(detail string) {
			p.err(line, fmt.Sprintf("clock range breakout does not support %s", detail), "")
		}
		switch head {
		case "dsl", "strategy", "name", "description", "clock", "orders", "buffer":
		case "slices":
			if len(tokens) < 3 || len(tokens)%2 == 0 {
				reject("slices that are not symbol timeframe pairs")
			}
		case "symbols", "timeframes":
			if len(tokens) < 2 {
				reject("an empty " + head + " list")
			}
		case "type":
			types++
			if len(tokens) != 4 || !strings.EqualFold(tokens[1], "clock") || !strings.EqualFold(tokens[2], "range") || !strings.EqualFold(tokens[3], "breakout") || types > 1 {
				reject("setup type directive " + strconv.Quote(strings.Join(tokens, " ")) + "; use exactly one type: clock range breakout")
			}
		case "range":
			if len(tokens) < 2 || !startsWithDigit(tokens[1]) {
				reject(`directive "range ` + strings.Join(tokens[1:], " ") + `"; use range <HH:MM> to <HH:MM>`)
			}
		case "close":
			if len(tokens) < 2 || !strings.EqualFold(tokens[1], "positions") {
				reject(`directive "close"; use close positions at <HH:MM>`)
			}
		case "stop":
			if !(len(tokens) == 3 && strings.EqualFold(tokens[2], "percent")) {
				reject(`stop "` + strings.Join(tokens[1:], " ") + `"; use stop <N> percent`)
			}
		case "side":
			text := strings.ToLower(strings.Join(tokens[1:], " "))
			if text != "both" && text != "long only" && text != "short only" {
				reject(`side "` + text + `"; use side both, side long only or side short only`)
			}
		case "risk":
			if !(len(tokens) == 3 && strings.EqualFold(tokens[2], "usd") && clockRiskOK(tokens[1])) {
				reject(`risk "` + strings.Join(tokens[1:], " ") + `"; use risk: <N> USD with N finite and nonnegative`)
			}
		case "riskusd":
			if !(len(tokens) == 2 && clockRiskOK(tokens[1])) {
				reject(`riskUsd "` + strings.Join(tokens[1:], " ") + `"; use a finite nonnegative number`)
			}
		default:
			section := line.section
			if section == "" {
				section = "top level"
			}
			reject(fmt.Sprintf("directive %q (section %s)", head, section))
		}
	}
}

func clockRiskOK(token string) bool {
	value, kind := parseClockNumber(token)
	return kind == clockNumberOK && value >= 0
}

func (p *parser) singleClockDirective(group []clockRangeDirective, name string) (clockRangeDirective, bool) {
	if len(group) == 0 {
		return clockRangeDirective{}, false
	}
	for _, extra := range group[1:] {
		p.err(extra.line, "duplicate "+name+" directive", "")
	}
	return group[0], len(group) == 1
}

func (p *parser) parseClockOffset() (int, bool) {
	directive, ok := p.singleClockDirective(p.clockRange.clock, "clock")
	if len(p.clockRange.clock) == 0 {
		p.errorAt(nil, nil, "clock range breakout requires clock UTC<±H[:MM]> in market conditions", "")
		return 0, false
	}
	if !ok {
		return 0, false
	}
	match := clockOffsetPattern.FindStringSubmatch(directive.line.text)
	if match == nil {
		p.err(directive.line, "clock must be written clock UTC<±H[:MM]>, for example clock UTC+10 or clock UTC-3:30", "")
		return 0, false
	}
	hours, _ := strconv.Atoi(match[2])
	minutes := 0
	if match[3] != "" {
		minutes, _ = strconv.Atoi(match[3])
		if minutes > 59 {
			p.err(directive.line, "clock offset minutes must be 00-59", "")
			return 0, false
		}
	}
	total := hours*60 + minutes
	if match[1] == "-" {
		total = -total
	}
	if total < ClockMinOffsetMinutes || total > ClockMaxOffsetMinutes {
		p.err(directive.line, "clock offset must be within UTC-12:00 and UTC+14:00", "")
		return 0, false
	}
	return total, true
}

func (p *parser) parseClockRangeWindow() (int, int, bool) {
	directive, ok := p.singleClockDirective(p.clockRange.rng, "range")
	if len(p.clockRange.rng) == 0 {
		p.errorAt(nil, nil, "clock range breakout requires range <HH:MM> to <HH:MM>", "")
		return 0, 0, false
	}
	if !ok {
		return 0, 0, false
	}
	tokens := directive.tokens
	if len(tokens) != 4 || !strings.EqualFold(tokens[2], "to") {
		p.err(directive.line, "range must be written range <HH:MM> to <HH:MM>", "")
		return 0, 0, false
	}
	start, startOK := parseClockWall(tokens[1])
	end, endOK := parseClockWall(tokens[3])
	if !startOK || !endOK {
		bad := tokens[1]
		if startOK {
			bad = tokens[3]
		}
		p.err(directive.line, fmt.Sprintf("range time %q must be HH:MM from 00:00 to 23:59", bad), "")
		return 0, 0, false
	}
	if start >= end {
		p.err(directive.line, "range start must be before range end on the same clock date", "")
		return 0, 0, false
	}
	return start, end, true
}

func (p *parser) parseClockWallDirective(group []clockRangeDirective, name, usage string) (int, bool) {
	directive, ok := p.singleClockDirective(group, name)
	if len(group) == 0 {
		p.errorAt(nil, nil, "clock range breakout requires "+usage, "")
		return 0, false
	}
	if !ok {
		return 0, false
	}
	tokens := directive.tokens
	if len(tokens) != 4 || !strings.EqualFold(tokens[2], "at") {
		p.err(directive.line, name+" must be written "+usage, "")
		return 0, false
	}
	minute, valid := parseClockWall(tokens[3])
	if !valid {
		p.err(directive.line, fmt.Sprintf("%s time %q must be HH:MM from 00:00 to 23:59", name, tokens[3]), "")
		return 0, false
	}
	return minute, true
}

func (p *parser) parseClockExpire() (int, bool) {
	directive, ok := p.singleClockDirective(p.clockRange.expire, "orders expire")
	if !ok {
		return 0, false
	}
	tokens := directive.tokens
	if len(tokens) != 3 || !strings.EqualFold(tokens[1], "expire") {
		p.err(directive.line, "orders must be written orders expire <HH:MM>", "")
		return 0, false
	}
	minute, valid := parseClockWall(tokens[2])
	if !valid {
		p.err(directive.line, fmt.Sprintf("orders expire time %q must be HH:MM from 00:00 to 23:59", tokens[2]), "")
		return 0, false
	}
	return minute, true
}

func (p *parser) parseClockBuffer() (float64, bool) {
	if len(p.clockRange.buffer) == 0 {
		return 0, true
	}
	directive, ok := p.singleClockDirective(p.clockRange.buffer, "buffer")
	if !ok {
		return 0, false
	}
	tokens := directive.tokens
	if len(tokens) != 3 || !strings.EqualFold(tokens[2], "pips") {
		p.err(directive.line, "buffer must be written buffer <N> pips", "")
		return 0, false
	}
	value, kind := parseClockNumber(tokens[1])
	switch {
	case kind == clockNumberMalformed:
		p.err(directive.line, fmt.Sprintf("buffer %q is not a number", tokens[1]), "")
	case kind == clockNumberNonfinite:
		p.err(directive.line, "buffer must be finite", "")
	case value < 0:
		p.err(directive.line, "buffer must be nonnegative", "")
	default:
		return value, true
	}
	return 0, false
}

func (p *parser) parseClockStopPercent() (float64, bool) {
	directive, ok := p.singleClockDirective(p.clockRange.stop, "stop")
	if len(p.clockRange.stop) == 0 {
		p.errorAt(nil, nil, "clock range breakout requires stop <N> percent", "")
		return 0, false
	}
	if !ok {
		return 0, false
	}
	value, kind := parseClockNumber(directive.tokens[1])
	switch {
	case kind == clockNumberMalformed:
		p.err(directive.line, fmt.Sprintf("stop percent %q is not a number", directive.tokens[1]), "")
	case kind == clockNumberNonfinite:
		p.err(directive.line, "stop percent must be finite", "")
	case !(value > 0) || value > 100:
		p.err(directive.line, "stop percent must be greater than 0 and at most 100", "")
	default:
		return value, true
	}
	return 0, false
}

// validateClockRangeRoutes checks every effective route: the four boundaries
// must sit on its UTC grid and a named symbol must have a reviewed pip size.
func (p *parser) validateClockRangeRoutes(spec ClockRangeSpec) {
	for _, route := range ClockRangeRoutes(p.config) {
		if err := spec.AlignmentError(route.Timeframe); err != nil {
			p.errorAt(nil, nil, fmt.Sprintf("clock range breakout route %s: %s", route, err), "")
		}
		if route.Symbol != "" {
			if _, ok := ReviewedInstrumentPip(route.Symbol); !ok {
				p.errorAt(nil, nil, fmt.Sprintf("clock range breakout route %s: no reviewed pip size for %s", route, route.Symbol), "")
			}
		}
	}
}

var clockAllowedHeads = map[string]bool{
	"dsl": true, "strategy": true, "name": true, "description": true, "slices": true, "symbols": true,
	"timeframes": true, "type": true, "range": true, "orders": true, "buffer": true, "clock": true,
	"close": true, "stop": true, "side": true, "risk": true, "riskusd": true,
}

// clockInlineKnownHeads are the words that start a directive in an inline
// `{ ... }` body for the purpose of the family's audit: every allowed head plus
// every head any other family accepts, so none can hide in text the ordinary
// splitter would drop or glue to a neighbor.
var clockInlineKnownHeads = func() map[string]bool {
	known := map[string]bool{}
	for head := range clockAllowedHeads {
		known[head] = true
	}
	for _, head := range generatedDirectiveHeads {
		known[strings.ToLower(head)] = true
	}
	for _, head := range directiveStartsForSection("setup") {
		known[strings.ToLower(head)] = true
	}
	for _, head := range []string{"local", "trade", "weekday", "hour", "new", "open", "seasonality", "micro", "rmv", "approach", "allow", "entrytf", "timed", "cooldown", "windows", "sessions", "session"} {
		known[head] = true
	}
	return known
}()

var clockQuotedMetadata = regexp.MustCompile(`^(?:"[^"]*"|'[^']*')$`)

// auditClockRangeInlineBodies checks the original text of every inline block.
// The ordinary splitter drops text before the first recognized directive and
// glues unrecognized words onto the previous one, so a forbidden gate written
// inline would otherwise vanish. Here each inline body must consist only of
// allowed directives, with nothing before the first one and metadata given as
// a single quoted string.
func (p *parser) auditClockRangeInlineBodies() {
	for index, raw := range strings.Split(p.source, "\n") {
		stripped := strings.TrimSpace(stripComment(raw))
		open, closeIndex := strings.Index(stripped, "{"), strings.LastIndex(stripped, "}")
		if open < 0 || closeIndex < open {
			continue
		}
		prefix := strings.TrimSpace(stripped[:open])
		if _, ok := sectionOpener(prefix + " {"); !ok && !isNamedStrategyHeader(prefix) {
			continue
		}
		line := logicalLine{text: stripped, line: index + 1, headCol: firstNonSpaceCol(raw)}
		body := strings.TrimSpace(stripped[open+1 : closeIndex])
		leading, segments := splitClockInlineBody(body)
		if leading != "" {
			p.err(line, fmt.Sprintf("clock range breakout does not support inline text %q before the first directive", leading), "")
		}
		for _, segment := range segments {
			fields := strings.Fields(strings.NewReplacer("(", " ", ")", " ").Replace(segment))
			head := strings.ToLower(strings.TrimRight(fields[0], ":"))
			if !clockAllowedHeads[head] {
				p.err(line, fmt.Sprintf("clock range breakout does not support inline directive %q", strings.TrimSpace(segment)), "")
				continue
			}
			if head == "description" || head == "name" || head == "strategy" {
				if rest := strings.TrimSpace(segment[len(fields[0]):]); !clockQuotedMetadata.MatchString(rest) {
					p.err(line, fmt.Sprintf("clock range breakout inline %s must be one quoted string", head), "")
				}
			}
		}
	}
}

// splitClockInlineBody splits body at every known directive head outside
// quotes. It returns the text before the first head and the segments.
func splitClockInlineBody(body string) (string, []string) {
	var starts []int
	var open byte
	prevHeadEnd := 0
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case open == 0 && (c == '"' || c == '\''):
			open = c
			continue
		case open != 0:
			if c == open {
				open = 0
			}
			continue
		}
		if i > 0 && body[i-1] != ' ' && body[i-1] != '\t' {
			continue
		}
		end := i
		for end < len(body) && (body[end] == '_' || body[end] >= '0' && body[end] <= '9' || body[end] >= 'a' && body[end] <= 'z' || body[end] >= 'A' && body[end] <= 'Z') {
			end++
		}
		word := strings.ToLower(body[i:end])
		if end == i || !clockInlineKnownHeads[word] || (end < len(body) && body[end] != ' ' && body[end] != '\t' && body[end] != '(' && body[end] != ':') {
			continue
		}
		if len(starts) > 0 && skipsInsideTypeValue(body, starts[len(starts)-1], i) {
			continue
		}
		// A head directly after another head is part of a multi-word head
		// such as `local hour` or `trade window`.
		if len(starts) > 0 && strings.TrimSpace(body[prevHeadEnd:i]) == "" {
			continue
		}
		gated := false
		for _, extra := range clockRangeInlineStarts {
			if word == extra.head && !extra.next.MatchString(body[end:]) {
				gated = true
			}
		}
		if !gated {
			starts = append(starts, i)
			prevHeadEnd = end
		}
		i = end - 1
	}
	if len(starts) == 0 {
		return strings.TrimSpace(body), nil
	}
	segments := make([]string, 0, len(starts))
	for k, start := range starts {
		stop := len(body)
		if k+1 < len(starts) {
			stop = starts[k+1]
		}
		segments = append(segments, strings.TrimSpace(body[start:stop]))
	}
	return strings.TrimSpace(body[:starts[0]]), segments
}

// skipsInsideTypeValue reports whether position i lies in the family name
// following a `type` head at typeStart, whose words (clock, range, breakout)
// would otherwise look like directive heads.
func skipsInsideTypeValue(body string, typeStart, i int) bool {
	rest := strings.ToLower(body[typeStart:])
	if !strings.HasPrefix(rest, "type") {
		return false
	}
	cursor := len("type")
	for cursor < len(rest) && (rest[cursor] == ' ' || rest[cursor] == ':') {
		cursor++
	}
	const name = "clock range breakout"
	return strings.HasPrefix(rest[cursor:], name) && i > typeStart && i < typeStart+cursor+len(name)
}
