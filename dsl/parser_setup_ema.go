package dsl

import "strings"

func (p *parser) parseEMA(tokens []string) {
	if len(tokens) < 3 {
		return
	}
	value := firstNumber(tokens, 2, 21)
	if p.config["setupType"] == string(FamilyTrendPullback) {
		tp := copyMap(p.config["trendPullback"])
		if strings.EqualFold(tokens[1], "length") {
			p.config["emaLen"] = value
			tp["emaLen"] = value
		} else if strings.EqualFold(tokens[1], "slope") {
			tp["emaSlopeLen"] = value
		}
		p.config["trendPullback"] = tp
	} else if p.config["setupType"] == string(FamilyElderTripleScreen) {
		elder := copyMap(p.config["elderTripleScreen"])
		if strings.EqualFold(tokens[1], "length") {
			p.config["emaLen"] = value
			elder["emaLen"] = value
		}
		p.config["elderTripleScreen"] = elder
	} else if p.config["setupType"] == string(FamilyBreakRetest) {
		br := copyMap(p.config["breakRetest"])
		if strings.EqualFold(tokens[1], "length") {
			p.config["emaLen"] = value
			br["emaLen"] = value
		} else if strings.EqualFold(tokens[1], "slope") {
			br["emaSlopeLen"] = value
		} else if strings.EqualFold(tokens[1], "confluence") {
			br["emaConfluenceAtr"] = firstNumber(tokens, 3, 0)
		}
		p.config["breakRetest"] = br
	} else {
		p.config["emaLen"] = value
	}
}
