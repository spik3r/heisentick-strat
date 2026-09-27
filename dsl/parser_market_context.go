package dsl

import (
	"fmt"
	"strconv"
	"strings"
)

var localWeekdayNames = map[string]string{
	"sun": "Sun", "mon": "Mon", "tue": "Tue", "wed": "Wed",
	"thu": "Thu", "fri": "Fri", "sat": "Sat",
}

var openLocationNames = map[string]string{
	"nearpdh": "nearPDH", "nearpdl": "nearPDL", "nearpdo": "nearPDO", "nearpdc": "nearPDC",
	"neardo": "nearDayOpen", "neardayopen": "nearDayOpen",
	"nearrangemid": "nearRangeMid", "nearrangemidpoint": "nearRangeMid",
	"nearah": "nearAH", "nearal": "nearAL", "nearlh": "nearLH", "nearll": "nearLL",
	"nearnh": "nearOvernightHigh", "overnighthigh": "nearOvernightHigh", "nearovernighthigh": "nearOvernightHigh",
	"nearnl": "nearOvernightLow", "overnightlow": "nearOvernightLow", "nearovernightlow": "nearOvernightLow",
	"nearcurrentopen": "nearCurrentOpen", "currentopen": "nearCurrentOpen", "other": "other",
}

func (p *parser) parseLocalWeekday(line logicalLine, tokens []string) bool {
	start := -1
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], "local") && strings.EqualFold(tokens[1], "weekday") {
		start = 2
	} else if len(tokens) >= 1 && strings.EqualFold(tokens[0], "weekday") {
		start = 1
	}
	if start < 0 {
		return false
	}
	values, negated, ok := gateList(tokens, start)
	if !ok || len(values) == 0 {
		p.err(line, "local weekday filter must use in (Mon, Tue) or not in (Fri)", "")
		return true
	}
	normalized := make([]any, 0, len(values))
	for _, value := range values {
		weekday, found := localWeekdayNames[strings.ToLower(value[:min(3, len(value))])]
		if !found {
			p.err(line, fmt.Sprintf("unknown local weekday %q; use Sun, Mon, Tue, Wed, Thu, Fri, or Sat", value), "")
			return true
		}
		normalized = append(normalized, weekday)
	}
	key := "localWeekdays"
	if negated {
		key = "blockedLocalWeekdays"
	}
	p.config[key] = normalized
	return true
}

func (p *parser) parseLocalHour(line logicalLine, tokens []string) bool {
	start := -1
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], "local") && strings.EqualFold(tokens[1], "hour") {
		start = 2
	} else if len(tokens) >= 1 && strings.EqualFold(tokens[0], "hour") {
		start = 1
	}
	if start < 0 {
		return false
	}
	values, negated, ok := gateList(tokens, start)
	if !ok || len(values) == 0 {
		p.err(line, "local hour filter must use 0-23 hours, for example: local hour not in (18)", "")
		return true
	}
	hours := make([]any, 0, len(values))
	for _, value := range values {
		hourToken := strings.TrimSuffix(strings.ToLower(value), ":00")
		hour, err := strconv.Atoi(hourToken)
		if err != nil || hour < 0 || hour > 23 {
			p.err(line, "local hour filter must use 0-23 hours, for example: local hour not in (18)", "")
			return true
		}
		hours = append(hours, hour)
	}
	key := "localHours"
	if negated {
		key = "blockedLocalHours"
	}
	p.config[key] = hours
	return true
}

func (p *parser) parseOpenLocation(line logicalLine, tokens []string) bool {
	if len(tokens) < 2 || !strings.EqualFold(tokens[1], "location") {
		return false
	}
	values, negated, ok := gateList(tokens, 2)
	if !ok || negated || len(values) == 0 {
		p.err(line, "open location filter must use in (nearPDH, nearDO, ...)", "")
		return true
	}
	normalized := make([]any, 0, len(values))
	for index := 0; index < len(values); index++ {
		value := values[index]
		if strings.EqualFold(value, "near") {
			if index+1 >= len(values) {
				p.err(line, `open location "near" needs a level name`, "")
				return true
			}
			index++
			value = "near" + values[index]
		}
		compact := strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "", ".", "").Replace(value))
		name, found := openLocationNames[compact]
		if !found {
			p.err(line, fmt.Sprintf("unknown open location %q", value), "")
			return true
		}
		normalized = append(normalized, name)
	}
	p.config["openLocations"] = normalized
	return true
}

func gateList(tokens []string, start int) (values []string, negated bool, ok bool) {
	in := -1
	not := -1
	for index := start; index < len(tokens); index++ {
		if strings.EqualFold(tokens[index], "in") && in < 0 {
			in = index
		}
		if strings.EqualFold(tokens[index], "not") && not < 0 {
			not = index
		}
	}
	if in < 0 {
		return nil, false, false
	}
	values = make([]string, 0, len(tokens)-in-1)
	for _, token := range tokens[in+1:] {
		if token != "" {
			values = append(values, token)
		}
	}
	return values, not >= 0 && not < in, true
}
