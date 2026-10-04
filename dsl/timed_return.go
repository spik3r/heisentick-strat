package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TimedAnchor distinguishes a session opening price from a completed boundary close.
type TimedAnchor struct {
	Session  string `json:"session"`
	Boundary string `json:"boundary"`
	Clock    string `json:"clock,omitempty"`
	Price    string `json:"price"`
}

// TimedReturnSpec is an opt-in offline bar-price execution contract.
type TimedReturnSpec struct {
	Timezone  string      `json:"timezone"`
	Start     TimedAnchor `json:"start"`
	End       TimedAnchor `json:"end"`
	Entry     string      `json:"entry"`
	Exit      string      `json:"exit"`
	Direction string      `json:"direction"`
	Quantity  int         `json:"quantity"`
	Execution string      `json:"execution"`
	Calendar  string      `json:"calendar"`
}

var timedClockPattern = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)
var timedSymbolPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{2,15}$`)

// TimedClockMinute validates an exact HH:MM spelling.
func TimedClockMinute(clock string) (int, error) {
	if !timedClockPattern.MatchString(clock) {
		return 0, fmt.Errorf("timed clock must be HH:MM")
	}
	h, _ := strconv.Atoi(clock[:2])
	m, _ := strconv.Atoi(clock[3:])
	return h*60 + m, nil
}

func validateTimedAnchor(a TimedAnchor) error {
	if a.Session != "current" && a.Session != "previous" {
		return fmt.Errorf("timed anchor session must be current or previous")
	}
	switch a.Boundary {
	case "open", "close":
		if a.Clock != "" || a.Price != a.Boundary {
			return fmt.Errorf("session anchor price must match its open/close boundary")
		}
	case "clock":
		if a.Session != "current" {
			return fmt.Errorf("clock anchor requires current session date")
		}
		if _, err := TimedClockMinute(a.Clock); err != nil {
			return err
		}
		if a.Price != "open" && a.Price != "close" {
			return fmt.Errorf("clock anchor requires open or completed-close price")
		}
	default:
		return fmt.Errorf("unknown timed anchor boundary")
	}
	return nil
}

// DecodeTimedReturn rejects incompatible hand-built configs as well as malformed DSL.
func DecodeTimedReturn(cfg Config) (TimedReturnSpec, error) {
	var spec TimedReturnSpec
	if cfg["setupType"] != string(FamilyTimedReturn) {
		return spec, fmt.Errorf("timed return setup required")
	}
	raw, err := json.Marshal(cfg["timedReturn"])
	if err != nil {
		return spec, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return spec, fmt.Errorf("timed return config: %w", err)
	}
	if spec.Timezone != "UTC" && spec.Timezone != "America/New_York" {
		return spec, fmt.Errorf("unsupported timed timezone")
	}
	if err := validateTimedAnchor(spec.Start); err != nil {
		return spec, err
	}
	if err := validateTimedAnchor(spec.End); err != nil {
		return spec, err
	}
	entry, err := TimedClockMinute(spec.Entry)
	if err != nil {
		return spec, err
	}
	exit, err := TimedClockMinute(spec.Exit)
	if err != nil || entry >= exit {
		return spec, fmt.Errorf("timed entry must precede exit on the same date")
	}
	if spec.Direction != "with" && spec.Direction != "against" {
		return spec, fmt.Errorf("timed direction must be with or against return")
	}
	if spec.Quantity != 1 || spec.Execution != "delayed-open" || spec.Calendar != "required" {
		return spec, fmt.Errorf("timed return requires one unit, delayed-open execution and calendar required")
	}
	allowed := map[string]bool{"setupType": true, "timedReturn": true, "name": true, "description": true, "slices": true, "allowLong": true, "allowShort": true}
	defaults := defaultConfig()
	for key, value := range cfg {
		if allowed[key] {
			continue
		}
		want, exists := defaults[key]
		gotJSON, e1 := json.Marshal(value)
		wantJSON, e2 := json.Marshal(want)
		if !exists || e1 != nil || e2 != nil || !bytes.Equal(gotJSON, wantJSON) {
			return spec, fmt.Errorf("timed return rejects nondefault config field %q", key)
		}
	}
	for _, key := range []string{"name", "description"} {
		if value, exists := cfg[key]; exists {
			if _, ok := value.(string); !ok {
				return spec, fmt.Errorf("invalid timed %s", key)
			}
		}
	}
	long, lok := timedBoolean(cfg["allowLong"])
	short, sok := timedBoolean(cfg["allowShort"])
	if !lok || !sok || (!long && !short) {
		return spec, fmt.Errorf("timed side flags must be zero/one with an enabled side")
	}
	type route struct {
		Symbol string `json:"symbol"`
		TF     string `json:"tf"`
	}
	var routes []route
	raw, err = json.Marshal(cfg["slices"])
	if err != nil {
		return spec, err
	}
	dec = json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&routes); err != nil || len(routes) != 1 {
		return spec, fmt.Errorf("timed return requires one explicit slice")
	}
	if !timedSymbolPattern.MatchString(routes[0].Symbol) || !timedTimeframe(routes[0].TF) {
		return spec, fmt.Errorf("invalid timed symbol/timeframe route")
	}
	return spec, nil
}

func timedBoolean(v any) (bool, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return false, false
	}
	if string(raw) == "0" {
		return false, true
	}
	if string(raw) == "1" {
		return true, true
	}
	return false, false
}

func timedTimeframe(tf string) bool {
	switch tf {
	case "1m", "5m", "15m", "30m", "1h":
		return true
	}
	return false
}

func (p *parser) parseTimedReturn(line logicalLine, tokens []string) {
	if p.dslVersion != 7 || line.section != "setup" || p.config["setupType"] != string(FamilyTimedReturn) {
		p.err(line, "timed directives require dsl v7 and preceding type: timed return in setup", "")
		return
	}
	if len(tokens) < 3 {
		p.err(line, "incomplete timed directive", "")
		return
	}
	values := copyMap(p.config["timedReturn"])
	key := strings.ToLower(tokens[1])
	tail := tokens[2:]
	var value any
	switch key {
	case "signal":
		if len(tail) < 4 || (tail[0] != "start" && tail[0] != "end") {
			p.err(line, "timed signal requires start/end and an explicit anchor", "")
			return
		}
		key = tail[0]
		tail = tail[1:]
		a := TimedAnchor{Session: tail[0]}
		if len(tail) == 3 && tail[1] == "session" && (tail[2] == "open" || tail[2] == "close") {
			a.Boundary, a.Price = tail[2], tail[2]
		} else if len(tail) == 4 && tail[1] == "clock" && (tail[3] == "open" || tail[3] == "completed-close") {
			a.Boundary, a.Clock, a.Price = "clock", tail[2], "close"
			if tail[3] == "open" {
				a.Price = "open"
			}
		} else {
			p.err(line, "invalid timed anchor spelling", "")
			return
		}
		if err := validateTimedAnchor(a); err != nil {
			p.err(line, err.Error(), "")
			return
		}
		value = map[string]any{"session": a.Session, "boundary": a.Boundary, "price": a.Price}
		if a.Clock != "" {
			value.(map[string]any)["clock"] = a.Clock
		}
	case "timezone", "entry", "exit", "execution", "calendar":
		if len(tail) != 1 {
			p.err(line, "timed "+key+" requires exactly one value", "")
			return
		}
		value = tail[0]
	case "direction":
		if len(tail) != 2 || tail[1] != "return" || (tail[0] != "with" && tail[0] != "against") {
			p.err(line, "timed direction must be with return or against return", "")
			return
		}
		value = tail[0]
	case "quantity":
		if len(tail) != 2 || tail[0] != "1" || tail[1] != "unit" {
			p.err(line, "timed quantity must be exactly 1 unit", "")
			return
		}
		value = 1
	default:
		p.err(line, "unknown timed directive "+key, "")
		return
	}
	if _, exists := values[key]; exists {
		p.err(line, "duplicate timed directive "+key, "")
		return
	}
	values[key] = value
	p.config["timedReturn"] = values
}

func (p *parser) validateTimedReturn() {
	if p.config["setupType"] != string(FamilyTimedReturn) {
		if p.config["timedReturn"] != nil {
			p.errorAt(nil, nil, "timed return directives cannot be discarded by changing family", "")
		}
		return
	}
	if p.dslVersion != 7 {
		p.errorAt(nil, nil, "timed return requires dsl v7", "")
	}
	p.validateTimedPhysicalSource()
	allowed := map[string]bool{"|dsl": true, "|strategy": true, "strategy|strategy": true, "strategy|description": true, "market|slices": true, "setup|type": true, "setup|timed": true, "filters|side": true}
	counts := map[string]int{}
	for _, line := range joinWhenLines(expandPhysicalLines(p.source)) {
		tokens := tokenize(normalizeLine(line.text))
		if len(tokens) == 0 {
			continue
		}
		head := strings.ToLower(tokens[0])
		key := line.section + "|" + head
		if !allowed[key] {
			p.err(line, "timed return does not support authored directive "+key, "")
			continue
		}
		counts[head]++
		if (head == "slices" || head == "type" || head == "side" || head == "dsl") && counts[head] > 1 {
			p.err(line, "duplicate timed-return directive "+head, "")
		}
		if head == "slices" && len(tokens) != 3 {
			p.err(line, "timed return requires exactly one symbol/timeframe slice", "")
		}
		if head == "side" && !(len(tokens) == 2 && tokens[1] == "both" || len(tokens) == 3 && (tokens[1] == "long" || tokens[1] == "short") && tokens[2] == "only") {
			p.err(line, "invalid timed-return side directive", "")
		}
	}
	if _, err := DecodeTimedReturn(p.config); err != nil {
		p.errorAt(nil, nil, err.Error(), "")
	}
}

// validateTimedPhysicalSource audits original authored text independently of
// the legacy expander, which can discard inline text. This opt-in family
// intentionally uses one directive per physical line and nonnested block lines.
// Other families retain their existing grammar and conformance output.
func (p *parser) validateTimedPhysicalSource() {
	inBlock := false
	for i, raw := range strings.Split(p.source, "\n") {
		text := strings.TrimSpace(stripComment(raw))
		if text == "" {
			continue
		}
		line := logicalLine{text: text, line: i + 1, headCol: firstNonSpaceCol(raw)}
		if text == "}" {
			if !inBlock {
				p.err(line, "timed return has an unmatched closing brace", "")
			}
			inBlock = false
			continue
		}
		if strings.ContainsAny(text, "{}") {
			section, ok := sectionOpener(text)
			if strings.Count(text, "{") != 1 || strings.Contains(text, "}") || !strings.HasSuffix(text, "{") || !ok || inBlock {
				p.err(line, "timed return requires nonnested blocks with braces on separate directive-free lines", "")
				continue
			}
			if section != "strategy" && section != "market" && section != "setup" && section != "filters" {
				p.err(line, "timed return does not support section "+section, "")
			}
			if section == "strategy" {
				header := strings.TrimSpace(strings.TrimSuffix(text, "{"))
				if !strings.EqualFold(header, "strategy") && !timedQuotedMetadata.MatchString(header) {
					p.err(line, "timed strategy header must contain only one quoted name", "")
				}
			}
			inBlock = true
			continue
		}
		if _, ok := sectionOpener(text); ok {
			p.err(line, "timed return section headers require an opening brace", "")
			continue
		}
		tokens := tokenize(normalizeLine(text))
		if len(tokens) == 0 {
			p.err(line, "timed return contains an empty authored directive", "")
			continue
		}
		switch strings.ToLower(tokens[0]) {
		case "dsl":
			if len(tokens) != 2 || tokens[1] != "v7" {
				p.err(line, "timed version header must be exactly dsl v7", "")
			}
		case "type":
			if len(tokens) != 3 || !strings.EqualFold(tokens[1], "timed") || !strings.EqualFold(tokens[2], "return") {
				p.err(line, "timed setup type must be exactly type: timed return", "")
			}
		case "strategy", "description":
			if !timedQuotedMetadata.MatchString(text) {
				p.err(line, "timed metadata must contain only one quoted string", "")
			}
		}
	}
	if inBlock {
		p.errorAt(nil, nil, "timed return has an unclosed section block", "")
	}
}

var timedQuotedMetadata = regexp.MustCompile(`(?i)^(?:strategy|description)\s+(?:"[^"{}#]*"|'[^'{}#]*')$`)
