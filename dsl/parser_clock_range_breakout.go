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
