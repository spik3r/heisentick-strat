package dsl

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var smaDecimalToken = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

func (p *parser) parseSMAGoldenCross(tokens []string) bool {
	if p.config["setupType"] != string(FamilySMAGoldenCross) || len(tokens) < 2 {
		return false
	}
	if !strings.EqualFold(tokens[0], "sma") {
		return false
	}
	key := map[string]string{
		"fast": "fastSmaLen",
		"slow": "slowSmaLen",
	}[strings.ToLower(tokens[1])]
	if key != "" {
		if len(tokens) != 3 {
			return false
		}
		// Keep the original fast/slow numeric grammar unchanged. The
		// protected execution fields below use stricter finite parsing.
		value := firstNumber(tokens, 2, 0)
		if value <= 0 || value != float64(int(value)) {
			return false
		}
		cfg := copyMap(p.config["smaGoldenCross"])
		cfg[key] = value
		p.config["smaGoldenCross"] = cfg
		return true
	}

	if strings.EqualFold(tokens[1], "atr") {
		if len(tokens) != 3 {
			return false
		}
		value, ok := parseSMAFiniteNumber(tokens[2])
		if !ok || value <= 0 || value != math.Trunc(value) {
			return false
		}
		cfg := copyMap(p.config["smaGoldenCross"])
		cfg["atrLen"] = value
		p.config["smaGoldenCross"] = cfg
		return true
	}

	protectedKey := map[string]string{"stop": "stopAtr", "target": "targetAtr"}[strings.ToLower(tokens[1])]
	if protectedKey == "" || len(tokens) != 4 || !strings.EqualFold(tokens[3], "ATR") {
		return false
	}
	value, ok := parseSMAFiniteNumber(tokens[2])
	if !ok || value <= 0 {
		return false
	}
	cfg := copyMap(p.config["smaGoldenCross"])
	cfg[protectedKey] = value
	p.config["smaGoldenCross"] = cfg
	return true
}

func parseSMAFiniteNumber(token string) (float64, bool) {
	if !smaDecimalToken.MatchString(token) {
		return 0, false
	}
	value, err := strconv.ParseFloat(token, 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}
