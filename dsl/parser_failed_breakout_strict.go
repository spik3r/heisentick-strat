package dsl

import (
	"regexp"
	"strings"
)

var strictFBName = regexp.MustCompile(`(?i)^strategy\s+"([^"{}]+)"$`)
var strictFBDescription = regexp.MustCompile(`(?i)^description\s+"([^"{}]+)"$`)
var strictFBType = regexp.MustCompile(`(?i)^type\s*:?\s+failed\s+breakout$`)
var strictFBRange = regexp.MustCompile(`(?i)^range\s+method\s*:?\s+zone$`)
var strictFBSlices = regexp.MustCompile(`(?i)^slices\s*\(\s*[a-z0-9._-]+\s+(?:1m|5m|15m|30m|1h|4h|1d)(?:\s*,?\s+[a-z0-9._-]+\s+(?:1m|5m|15m|30m|1h|4h|1d))*\s*\)$`)
var strictFBRisk = regexp.MustCompile(`(?i)^risk\s*:?\s+(\S+)\s+USD$`)
var strictFBRiskUSD = regexp.MustCompile(`(?i)^riskUsd\s*:?\s+(\S+)$`)
var strictFBDecimal = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$`)

// This profile deliberately exposes only risk sizing. Validate original physical
// lines: the historical inline lexer can discard prefixes and section suffixes.
// Ordinary Parse and the other strict families retain their existing grammar.
func (p *parser) validateStrictFailedBreakout() {
	if !p.strictSource || p.config["setupType"] != string(FamilyFailedBreakout) {
		return
	}
	seen := map[string]int{}
	sections := map[string]int{}
	section, directives := "", 0
	validatedRisk := 0.0
	consume := func(line logicalLine) {
		text := strings.TrimSpace(line.text)
		key, valid := "", false
		switch line.section {
		case "":
			key, valid = "dsl", strings.EqualFold(text, "dsl v7")
		case "strategy":
			key, valid = "description", strictFBDescription.MatchString(text)
			if valid {
				p.config["description"] = strictFBDescription.FindStringSubmatch(text)[1]
			}
		case "market":
			key, valid = "slices", strictFBSlices.MatchString(text)
			pairs := map[string]bool{}
			tokens := tokenize(normalizeLine(text))
			for i := 1; i+1 < len(tokens); i += 2 {
				pair := strings.ToUpper(tokens[i]) + "|" + strings.ToLower(tokens[i+1])
				if pairs[pair] {
					valid = false
				}
				pairs[pair] = true
			}
		case "setup":
			key, valid = "type", strictFBType.MatchString(text)
		case "filters":
			if strings.EqualFold(strings.Join(strings.Fields(text), " "), "side long only") {
				key, valid = "side", true
			} else {
				key, valid = "range", strictFBRange.MatchString(text)
			}
		case "execution":
			key = "risk"
			match := strictFBRisk.FindStringSubmatch(text)
			if match == nil {
				match = strictFBRiskUSD.FindStringSubmatch(text)
			}
			if match != nil {
				value, ok := parseSMAFiniteNumber(match[1])
				valid = strictFBDecimal.MatchString(match[1]) && ok && value > 0
				if valid {
					validatedRisk = value
				}
			}
		}
		if !valid {
			p.err(line, "strict failed breakout supports only explicit v7/type/slices, side long only, range method zone and finite positive risk USD", "")
			return
		}
		seen[key]++
		if seen[key] > 1 {
			p.err(line, "strict failed breakout rejects duplicate assignment: "+key, "")
		}
	}
	for i, raw := range strings.Split(p.source, "\n") {
		text := strings.TrimSpace(stripComment(raw))
		if text == "" {
			continue
		}
		line := logicalLine{text: text, line: i + 1, headCol: firstNonSpaceCol(raw), section: section}
		if text == "}" {
			if section == "" || directives == 0 {
				p.err(line, "strict failed breakout rejects unmatched or empty sections", "")
			}
			section, directives = "", 0
			continue
		}
		if strings.ContainsAny(text, "{}") {
			open := strings.IndexByte(text, '{')
			if section != "" || open < 0 || strings.Count(text, "{") != 1 || strings.Count(text, "}") > 1 ||
				(strings.Contains(text, "}") && !strings.HasSuffix(text, "}")) {
				p.err(line, "strict failed breakout rejects nested, malformed or trailing section content", "")
				continue
			}
			header := strings.TrimSpace(text[:open])
			switch strings.ToLower(strings.Join(strings.Fields(header), " ")) {
			case "market conditions":
				section = "market"
			case "setup", "filters", "execution":
				section = strings.ToLower(header)
			default:
				if strictFBName.MatchString(header) {
					section = "strategy"
					p.config["name"] = strictFBName.FindStringSubmatch(header)[1]
				} else {
					p.err(line, "strict failed breakout rejects unsupported section", "")
					continue
				}
			}
			sections[section]++
			if sections[section] > 1 {
				p.err(line, "strict failed breakout rejects duplicate section: "+section, "")
			}
			directives = 0
			if section == "strategy" {
				directives = 1 // The header supplies the optional name metadata.
			}
			body := strings.TrimSpace(text[open+1:])
			if strings.HasSuffix(body, "}") {
				body = strings.TrimSpace(strings.TrimSuffix(body, "}"))
				if body != "" {
					line.text, line.section = body, section
					consume(line)
					directives++
				}
				if directives == 0 {
					p.err(line, "strict failed breakout rejects empty sections", "")
				}
				section, directives = "", 0
			} else if body != "" {
				p.err(line, "strict failed breakout requires a section body on following lines or one complete inline phrase", "")
			}
			continue
		}
		consume(line)
		directives++
	}
	if section != "" {
		p.errorAt(nil, nil, "strict failed breakout requires closing every section", "")
	}
	for _, key := range []string{"dsl", "type", "slices", "side", "range", "risk"} {
		if seen[key] != 1 {
			p.errorAt(nil, nil, "strict failed breakout requires exactly one "+key+" assignment", "")
		}
	}
	if parsedRisk, ok := p.config["riskUsd"].(float64); !ok || parsedRisk != validatedRisk {
		p.errorAt(nil, nil, "strict failed breakout parsed risk must match its audited decimal value", "")
	}
}
