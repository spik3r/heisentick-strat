package dsl

import "strings"

func (p *parser) parseDualEMAResumption(tokens []string) bool {
	if p.config["setupType"] != string(FamilyDualEMAResumption) || len(tokens) < 2 {
		return false
	}
	cfg := copyMap(p.config["dualEmaResumption"])
	head := strings.ToLower(tokens[0])
	setInt := func(key string, index int) bool {
		value := firstNumber(tokens, index, 0)
		if value <= 0 || value != float64(int(value)) {
			return false
		}
		cfg[key] = value
		p.config["dualEmaResumption"] = cfg
		return true
	}
	setPositive := func(key string, index int) bool {
		value := firstNumber(tokens, index, 0)
		if value <= 0 {
			return false
		}
		cfg[key] = value
		p.config["dualEmaResumption"] = cfg
		return true
	}
	if head == "ema" && len(tokens) == 3 && strings.EqualFold(tokens[1], "fast") {
		return setInt("fastEmaLen", 2)
	}
	if head == "ema" && len(tokens) == 4 && strings.EqualFold(tokens[1], "slow") && strings.EqualFold(tokens[2], "rise") {
		return setInt("slowRiseBars", 3)
	}
	if head == "ema" && len(tokens) == 3 && strings.EqualFold(tokens[1], "slow") {
		return setInt("slowEmaLen", 2)
	}
	if head == "ema" && len(tokens) == 4 && strings.EqualFold(tokens[1], "wilder") && strings.EqualFold(tokens[2], "atr") {
		return setInt("atrLen", 3)
	}
	if head == "stop" && len(tokens) == 4 && strings.EqualFold(tokens[1], "initial") && strings.EqualFold(tokens[3], "atr") {
		return setPositive("stopAtr", 2)
	}
	if head == "trail" && len(tokens) == 4 && strings.EqualFold(tokens[1], "close") && strings.EqualFold(tokens[3], "atr") {
		return setPositive("trailAtr", 2)
	}
	return len(tokens) == 4 && head == "fallback" && strings.EqualFold(strings.Join(tokens[1:], " "), "below slow ema")
}
