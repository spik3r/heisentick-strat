package dsl

import (
	"math"
	"strings"
)

type dualEMAParseAudit struct {
	authored        []string
	riskAuthored    bool
	riskValid       bool
	sawSetupType    bool
	setupBeforeType []string
}

func (p *parser) recordDualEMAAuthored(line logicalLine, tokens []string, head string) {
	audit := &p.dualEMAAudit
	audit.authored = append(audit.authored, line.section+"|"+head)
	if line.section == "setup" && !audit.sawSetupType && (head == "ema" || head == "stop" || head == "trail" || head == "fallback") {
		for _, existing := range audit.setupBeforeType {
			if existing == head {
				return
			}
		}
		audit.setupBeforeType = append(audit.setupBeforeType, head)
	}
	if head == "type" {
		audit.sawSetupType = true
	}
	if line.section == "execution" && (head == "risk" || head == "riskusd") {
		value := firstNumber(tokens, 1, 0)
		exactShape := (head == "risk" && len(tokens) == 3 && strings.EqualFold(tokens[2], "usd")) || (head == "riskusd" && len(tokens) == 2)
		valid := exactShape && isNumberToken(tokens[1]) && value > 0 && !math.IsInf(value, 0)
		audit.riskValid = (!audit.riskAuthored || audit.riskValid) && valid
		audit.riskAuthored = true
	}
}

func (p *parser) validateDualEMAResumption() {
	if p.config["setupType"] != string(FamilyDualEMAResumption) {
		return
	}
	audit := p.dualEMAAudit
	if len(audit.setupBeforeType) > 0 {
		p.errorAt(nil, nil, "dual ema resumption requires its type directive before family phrase(s): "+strings.Join(audit.setupBeforeType, ", ")+".", "")
	}
	allowed := map[string]bool{
		"|dsl": true, "strategy|strategy": true, "strategy|description": true, "market|slices": true,
		"setup|type": true, "setup|ema": true, "setup|stop": true,
		"setup|trail": true, "setup|fallback": true, "filters|side": true,
		"execution|risk": true, "execution|riskusd": true,
	}
	for _, authored := range audit.authored {
		if !allowed[authored] {
			p.errorAt(nil, nil, "dual ema resumption does not support authored directive: "+strings.ReplaceAll(authored, "|", ":")+".", "")
		}
	}
	if source, ok := p.config["sourceTimeframe"].(string); ok && source != "" {
		p.errorAt(nil, nil, "dual ema resumption does not support source timeframe execution.", "")
	}
	allowLong, _ := p.config["allowLong"].(int)
	allowShort, _ := p.config["allowShort"].(int)
	if allowLong == 0 || allowShort != 0 {
		p.errorAt(nil, nil, "dual ema resumption supports side long only.", "")
	}
	if audit.riskAuthored && !audit.riskValid {
		p.errorAt(nil, nil, "dual ema resumption risk USD must be finite and greater than zero.", "")
	}
}
