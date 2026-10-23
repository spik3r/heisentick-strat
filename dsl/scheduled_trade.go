package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ScheduledTradeSpec is the closed configuration for a fixed-clock trade.
type ScheduledTradeSpec struct {
	Timezone string  `json:"timezone"`
	Calendar string  `json:"calendar"`
	Entry    string  `json:"entry"`
	Exit     string  `json:"exit"`
	Side     string  `json:"side"`
	StopPips float64 `json:"stopPips"`
	Quantity int     `json:"quantity"`
}

func DecodeScheduledTrade(cfg Config) (ScheduledTradeSpec, error) {
	var spec ScheduledTradeSpec
	if cfg["setupType"] != string(FamilyScheduledTrade) {
		return spec, fmt.Errorf("scheduled trade setup required")
	}
	raw, err := json.Marshal(cfg["scheduledTrade"])
	if err != nil {
		return spec, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return spec, fmt.Errorf("scheduled trade config: %w", err)
	}
	if spec.Timezone != "UTC" || spec.Calendar != "japan-settlement" {
		return spec, fmt.Errorf("scheduled trade requires UTC and japan-settlement calendar")
	}
	entry, err := TimedClockMinute(spec.Entry)
	if err != nil {
		return spec, err
	}
	exit, err := TimedClockMinute(spec.Exit)
	if err != nil || exit <= entry {
		return spec, fmt.Errorf("scheduled exit must follow entry on the same UTC date")
	}
	if spec.Side != "long" && spec.Side != "short" {
		return spec, fmt.Errorf("scheduled side must be long or short")
	}
	if !finiteScheduled(spec.StopPips) || spec.StopPips <= 0 || spec.Quantity != 1 {
		return spec, fmt.Errorf("scheduled trade requires a positive pip stop and quantity 1 unit")
	}
	allowed := map[string]bool{"setupType": true, "scheduledTrade": true, "name": true, "description": true, "slices": true}
	defaults := defaultConfig()
	for key, value := range cfg {
		if allowed[key] {
			continue
		}
		want, ok := defaults[key]
		gotJSON, e1 := json.Marshal(value)
		wantJSON, e2 := json.Marshal(want)
		if !ok || e1 != nil || e2 != nil || !bytes.Equal(gotJSON, wantJSON) {
			return spec, fmt.Errorf("scheduled trade rejects nondefault config field %q", key)
		}
	}
	for _, key := range []string{"name", "description"} {
		if v, ok := cfg[key]; ok {
			if _, isString := v.(string); !isString {
				return spec, fmt.Errorf("invalid scheduled %s", key)
			}
		}
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
	if err = dec.Decode(&routes); err != nil || len(routes) != 1 {
		return spec, fmt.Errorf("scheduled trade requires one explicit slice")
	}
	if !ScheduledPipKnownSymbol(routes[0].Symbol) || (routes[0].TF != "1m" && routes[0].TF != "5m") {
		return spec, fmt.Errorf("scheduled trade route requires a supported FX pip map and 1m or 5m timeframe")
	}
	step := 1
	if routes[0].TF == "5m" {
		step = 5
	}
	if entry%step != 0 || exit%step != 0 {
		return spec, fmt.Errorf("scheduled entry and exit must align to the declared timeframe")
	}
	return spec, nil
}

func finiteScheduled(v float64) bool { return v > 0 && v < 1e6 }

// ScheduledPipKnownSymbol limits this family to its reviewed FX quotation map.
func ScheduledPipKnownSymbol(symbol string) bool {
	switch symbol {
	case "USDJPY", "EURJPY", "GBPJPY", "AUDJPY", "NZDJPY", "CADJPY", "CHFJPY", "EURUSD", "GBPUSD", "AUDUSD", "NZDUSD", "USDCAD", "USDCHF":
		return true
	}
	return false
}

func (p *parser) parseScheduledTrade(line logicalLine, tokens []string) {
	if p.dslVersion != 7 || line.section != "setup" || p.config["setupType"] != string(FamilyScheduledTrade) {
		p.err(line, "scheduled directives require dsl v7 and preceding type: scheduled trade in setup", "")
		return
	}
	if len(tokens) < 3 {
		p.err(line, "incomplete scheduled directive", "")
		return
	}
	key := strings.ToLower(tokens[1])
	tail := tokens[2:]
	values := copyMap(p.config["scheduledTrade"])
	var value any
	switch key {
	case "timezone", "calendar", "entry", "exit":
		if len(tail) != 1 {
			p.err(line, "scheduled "+key+" requires exactly one value", "")
			return
		}
		value = tail[0]
	case "side":
		if len(tail) != 1 || (tail[0] != "long" && tail[0] != "short") {
			p.err(line, "scheduled side must be long or short", "")
			return
		}
		value = tail[0]
	case "stop":
		if len(tail) != 2 || tail[1] != "pips" {
			p.err(line, "scheduled stop requires a positive number of pips", "")
			return
		}
		n := parseNumber(tail[0], 0)
		if n <= 0 || !finiteScheduled(n) {
			p.err(line, "scheduled stop must be positive", "")
			return
		}
		key = "stopPips"
		value = n
	case "quantity":
		if len(tail) != 2 || tail[0] != "1" || tail[1] != "unit" {
			p.err(line, "scheduled quantity must be exactly 1 unit", "")
			return
		}
		value = 1
	default:
		p.err(line, "unknown scheduled directive "+key, "")
		return
	}
	if _, exists := values[key]; exists {
		p.err(line, "duplicate scheduled directive "+key, "")
		return
	}
	values[key] = value
	p.config["scheduledTrade"] = values
}

func (p *parser) validateScheduledTrade() {
	if p.config["setupType"] != string(FamilyScheduledTrade) {
		if p.config["scheduledTrade"] != nil {
			p.errorAt(nil, nil, "scheduled trade directives cannot be discarded by changing family", "")
		}
		return
	}
	if p.dslVersion != 7 {
		p.errorAt(nil, nil, "scheduled trade requires dsl v7", "")
	}
	allowed := map[string]bool{"|dsl": true, "|strategy": true, "strategy|strategy": true, "strategy|description": true, "market|slices": true, "setup|type": true, "setup|scheduled": true}
	seen := map[string]bool{}
	for _, line := range joinWhenLines(expandPhysicalLines(p.source)) {
		tokens := tokenize(normalizeLine(line.text))
		if len(tokens) == 0 {
			continue
		}
		head := strings.ToLower(tokens[0])
		k := line.section + "|" + head
		if !allowed[k] {
			p.err(line, "scheduled trade does not support authored directive "+k, "")
			continue
		}
		if (head == "strategy" || head == "description") && !timedQuotedMetadata.MatchString(strings.TrimSpace(line.text)) {
			p.err(line, "scheduled metadata must contain only one quoted string", "")
		}
		if head == "dsl" && (len(tokens) != 2 || tokens[1] != "v7") {
			p.err(line, "scheduled version header must be exactly dsl v7", "")
		}
		if head == "type" || head == "slices" || head == "dsl" {
			if seen[head] {
				p.err(line, "duplicate scheduled directive "+head, "")
			}
			seen[head] = true
		}
		if head == "slices" && len(tokens) != 3 {
			p.err(line, "scheduled trade requires exactly one symbol/timeframe slice", "")
		}
	}
	if _, err := DecodeScheduledTrade(p.config); err != nil {
		p.errorAt(nil, nil, err.Error(), "")
	}
}
