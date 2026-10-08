package dsl

import (
	"math"
	"strings"
)

func (p *parser) parseEntry(tokens []string) {
	if p.config["setupType"] == string(FamilyFairValueGap) && len(tokens) >= 3 && strings.EqualFold(tokens[1], "at") {
		fvg := copyMap(p.config["fairValueGap"])
		switch strings.ToLower(tokens[2]) {
		case "midpoint":
			fvg["entryReference"] = "midpoint"
		case "proximal", "proximaledge":
			fvg["entryReference"] = "proximalEdge"
		case "0.618", "61.8%":
			fvg["entryReference"] = "fib618"
		}
		if len(tokens) >= 5 && strings.EqualFold(tokens[3], "within") {
			value := firstNumber(tokens, 4, 0)
			if value <= 0 || value != math.Trunc(value) {
				p.errorAt(nil, nil, "fair value gap entry window must be a positive integer", "entry at midpoint within N candles")
				return
			}
			fvg["entryExpireCandles"] = value
		}
		p.config["fairValueGap"] = fvg
		return
	}
	if len(tokens) >= 4 && strings.EqualFold(tokens[1], "distance") {
		if p.config["setupType"] == string(FamilyRangeBreakFake) {
			rbf := copyMap(p.config["rangeBreakFake"])
			rbf["maxEntryDistanceAtr"] = firstNumber(tokens, 2, 0)
			p.config["rangeBreakFake"] = rbf
		}
		trigger := copyMap(p.config["trigger"])
		trigger["maxEntryDistanceAtr"] = firstNumber(tokens, 2, 0)
		p.config["trigger"] = trigger
	}
}

func (p *parser) parseEntryTimeframe(line logicalLine, tokens []string) {
	value := "current"
	if len(tokens) > 1 {
		value = strings.ToLower(tokens[1])
	}
	p.config["entryTf"], p.entryTfLine = value, &line
	if value != "current" && value != "1m" && value != "5m" && value != "15m" && value != "30m" && value != "1h" {
		message := "entryTf " + value + " is unsupported; only entryTf current and source-entry pairs 4h -> 15m (legacy), 4h -> 30m, 1h -> 5m, and 15m -> 1m are implemented (plus 4h -> 1h with supply demand retest on entry timeframe)."
		if p.dslVersion >= 7 {
			p.err(line, message, "")
		} else {
			p.warn(line, message, "")
		}
	}
}

func (p *parser) validateEntryTimeframe() {
	entryTf, _ := p.config["entryTf"].(string)
	source, _ := p.config["sourceTimeframe"].(string)
	sd, _ := p.config["supplyDemand"].(map[string]any)
	if retest, _ := sd["retestOnEntryTimeframe"].(int); retest != 0 {
		if source != "4h" || (entryTf != "15m" && entryTf != "30m" && entryTf != "1h") {
			message := "retest on entry timeframe requires source timeframe 4h and entryTf 15m, 30m or 1h."
			if p.entryTfLine != nil {
				p.err(*p.entryTfLine, message, "")
			} else {
				p.errorAt(nil, nil, message, "")
			}
			return
		}
	}
	if entryTf == "current" {
		return
	}
	expected := map[string]string{"15m": "4h", "30m": "4h", "5m": "1h", "1m": "15m"}[entryTf]
	if retest, _ := sd["retestOnEntryTimeframe"].(int); retest != 0 && entryTf == "1h" {
		expected = "4h"
	}
	message := "entryTf " + entryTf + " is supported only with source timeframe " + expected + "."
	if expected == "" || source != expected {
		if p.entryTfLine != nil {
			p.err(*p.entryTfLine, message, "")
		} else {
			p.errorAt(nil, nil, message, "")
		}
		return
	}
	slices, _ := p.config["slices"].([]any)
	for _, raw := range slices {
		slice, _ := raw.(map[string]any)
		timeframe, _ := slice["tf"].(string)
		if strings.EqualFold(timeframe, entryTf) {
			return
		}
	}
	message = "entryTf " + entryTf + " requires a market slice at that entry timeframe."
	if p.entryTfLine != nil {
		p.err(*p.entryTfLine, message, "")
	} else {
		p.errorAt(nil, nil, message, "")
	}
}
