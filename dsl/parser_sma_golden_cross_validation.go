package dsl

import (
	"math"
	"strings"
)

type smaGoldenCrossParseAudit struct {
	authored        []string
	setupBeforeType []string
	sawSetupType    bool
}

func (p *parser) recordSMAGoldenCrossAuthored(line logicalLine, head string) {
	audit := &p.smaGoldenCrossAudit
	audit.authored = append(audit.authored, line.section+"|"+head)
	if line.section == "setup" && !audit.sawSetupType && head == "sma" {
		for _, existing := range audit.setupBeforeType {
			if existing == head {
				return
			}
		}
		audit.setupBeforeType = append(audit.setupBeforeType, head)
	}
	if line.section == "setup" && head == "type" {
		audit.sawSetupType = true
	}
}

func (p *parser) validateSMAGoldenCross() {
	if p.config["setupType"] != string(FamilySMAGoldenCross) {
		return
	}
	audit := p.smaGoldenCrossAudit
	if len(audit.setupBeforeType) > 0 {
		p.errorAt(nil, nil, "sma golden cross requires its type directive before family phrase(s): "+strings.Join(audit.setupBeforeType, ", ")+".", "")
	}
	allowed := map[string]bool{
		"|dsl": true, "|strategy": true, "strategy|strategy": true, "strategy|description": true,
		"market|slices": true, "setup|type": true, "setup|sma": true,
		"filters|side": true,
	}
	seen := map[string]bool{}
	for _, authored := range audit.authored {
		if allowed[authored] || seen[authored] {
			continue
		}
		seen[authored] = true
		p.errorAt(nil, nil, "sma golden cross does not support authored directive: "+strings.ReplaceAll(authored, "|", ":")+".", "")
	}
	allowLong, _ := p.config["allowLong"].(int)
	allowShort, _ := p.config["allowShort"].(int)
	if allowLong == 0 || allowShort != 0 {
		p.errorAt(nil, nil, "sma golden cross supports side long only.", "")
	}
	pCfg := copyMap(p.config["smaGoldenCross"])
	fast, fastOk := pCfg["fastSmaLen"].(float64)
	slow, slowOk := pCfg["slowSmaLen"].(float64)
	if !fastOk || !slowOk || fast <= 0 || slow <= 0 || fast != float64(int(fast)) || slow != float64(int(slow)) {
		p.errorAt(nil, nil, "sma golden cross requires positive integer fast and slow SMA lengths.", "")
	} else if fast >= slow {
		p.errorAt(nil, nil, "sma golden cross requires the fast SMA length to be shorter than the slow SMA length.", "")
	}
	protectedKeys := []string{"atrLen", "stopAtr", "targetAtr"}
	protectedAuthored := 0
	for _, key := range protectedKeys {
		if _, ok := pCfg[key]; ok {
			protectedAuthored++
		}
	}
	if protectedAuthored == 0 {
		return
	}
	if protectedAuthored != len(protectedKeys) {
		p.errorAt(nil, nil, "sma golden cross protected execution requires sma atr, sma stop, and sma target together.", "")
	}
	atr, atrOk := pCfg["atrLen"].(float64)
	stop, stopOk := pCfg["stopAtr"].(float64)
	target, targetOk := pCfg["targetAtr"].(float64)
	if !atrOk || atr <= 0 || atr != math.Trunc(atr) {
		p.errorAt(nil, nil, "sma golden cross protected execution requires a positive integer ATR length.", "")
	}
	if !stopOk || !isFiniteNumber(stop) || stop <= 0 || !targetOk || !isFiniteNumber(target) || target <= 0 {
		p.errorAt(nil, nil, "sma golden cross protected execution requires finite positive stop and target ATR multiples.", "")
	}
}
