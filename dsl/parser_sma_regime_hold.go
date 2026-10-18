package dsl

import (
	"math"
	"strings"
)

func (p *parser) recordSMARegimeHoldAuthored(line logicalLine, head string, tokens []string) {
	p.smaRegimeHoldAudit = append(p.smaRegimeHoldAudit, line.section+"|"+head)
	if head == "type" {
		p.smaRegimeHoldTypes++
		if canonicalSetupFamily(tokens[1:]) == string(FamilySMARegimeHold) {
			p.smaRegimeHoldSeen = true
		}
	}
}

func (p *parser) parseSMARegimeHold(tokens []string) bool {
	if p.config["setupType"] != string(FamilySMARegimeHold) || len(tokens) != 3 {
		return false
	}
	if !strings.EqualFold(tokens[0], "sma") || !strings.EqualFold(tokens[1], "period") {
		return false
	}
	value, ok := parseSMAFiniteNumber(tokens[2])
	if !ok || value < 1 || value > math.MaxInt32 || value != math.Trunc(value) {
		return false
	}
	config := copyMap(p.config["smaRegimeHold"])
	config["period"] = value
	p.config["smaRegimeHold"] = config
	return true
}

func (p *parser) validateSMARegimeHold() {
	if !p.smaRegimeHoldSeen && p.config["setupType"] != string(FamilySMARegimeHold) {
		return
	}
	if p.config["setupType"] != string(FamilySMARegimeHold) {
		p.errorAt(nil, nil, "sma regime hold must remain the selected setup family", "")
	}
	if p.smaRegimeHoldTypes != 1 {
		p.errorAt(nil, nil, "sma regime hold requires exactly one setup type directive", "")
	}
	for _, issue := range AuditOrdinarySource(p.source).Issues {
		p.errorAt(nil, nil, "sma regime hold does not support uncovered source: "+issue.Text, "")
	}
	config := copyMap(p.config["smaRegimeHold"])
	period, ok := config["period"].(float64)
	if !ok || period < 1 || period > math.MaxInt32 || period != math.Trunc(period) {
		p.errorAt(nil, nil, "sma-regime-hold requires a positive integer SMA period", "")
	}
	allowed := map[string]bool{
		"|dsl": true, "|strategy": true, "strategy|strategy": true, "strategy|description": true,
		"market|slices": true, "market|symbols": true, "market|timeframes": true,
		"setup|type": true, "setup|sma": true,
	}
	seen := map[string]bool{}
	for _, authored := range p.smaRegimeHoldAudit {
		if allowed[authored] || seen[authored] {
			continue
		}
		seen[authored] = true
		p.errorAt(nil, nil, "sma regime hold does not support authored directive: "+strings.ReplaceAll(authored, "|", ":")+".", "")
	}
}
