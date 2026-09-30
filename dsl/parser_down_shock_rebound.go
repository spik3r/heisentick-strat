package dsl

import (
	"fmt"
	"math"
	"strings"
)

func (p *parser) validateDownShockRebound() {
	if p.config["setupType"] != string(FamilyDownShockRebound) {
		return
	}
	allowed := map[string]bool{"dsl": true, "strategy": true, "name": true, "description": true, "slices": true, "symbols": true, "timeframes": true, "type": true, "source": true, "entrytf": true, "shock": true, "risk": true, "riskusd": true}
	for _, line := range joinWhenLines(expandPhysicalLines(p.source)) {
		tokens := tokenize(normalizeLine(line.text))
		if len(tokens) == 0 {
			continue
		}
		head := strings.ToLower(tokens[0])
		if !allowed[head] {
			p.errorAt(nil, nil, "down shock rebound does not support authored directive: "+head, "")
		}
	}
	risk, ok := p.config["riskUsd"].(float64)
	if !ok {
		risk = firstNumber([]string{fmt.Sprint(p.config["riskUsd"])}, 0, math.NaN())
	}
	if !isFiniteNumber(risk) || risk <= 0 {
		p.errorAt(nil, nil, "down shock rebound risk must be a positive finite USD amount", "")
	}
	if p.config["sourceTimeframe"] != "15m" || p.config["entryTf"] != "1m" {
		p.errorAt(nil, nil, `down shock rebound requires source timeframe 15m and entryTf 1m`, "")
	}
}

func (p *parser) parseDownShockRebound(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyDownShockRebound) {
		p.unknownDirective(line, tokens[0])
		return
	}
	family := copyMap(p.config["downShockRebound"])
	phrase := strings.ToLower(strings.Join(tokens, " "))
	switch {
	case phrase == "shock entry immediate":
		family["entry"] = "immediate"
	case phrase == "shock entry reversal":
		family["entry"] = "reversal"
	case phrase == "shock target off":
		family["targetMode"] = "off"
		family["targetValue"] = 0.0
	case len(tokens) == 4 && strings.EqualFold(tokens[1], "target") && (strings.EqualFold(tokens[3], "ATR") || strings.EqualFold(tokens[3], "bp")):
		value := firstNumber(tokens, 2, math.NaN())
		if !isFiniteNumber(value) || value <= 0 {
			p.err(line, "shock target must be finite and positive", "")
			return
		}
		family["targetMode"] = strings.ToLower(tokens[3])
		family["targetValue"] = value
	default:
		p.err(line, "shock phrases are: shock entry immediate | shock entry reversal | shock target off | shock target X ATR | shock target X bp", "")
		return
	}
	p.config["downShockRebound"] = family
}
