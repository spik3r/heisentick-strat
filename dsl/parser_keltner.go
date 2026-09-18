package dsl

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// keltnerTargetRTokenPattern matches an NR-style target token such as "1R"
// or "0.5r", mirroring the JS regex in
// engine/dsl/spec/parseSetups/keltnerPhrases.js.
var keltnerTargetRTokenPattern = regexp.MustCompile(`(?i)^[0-9]*\.?[0-9]+r$`)

func isKeltnerSetupType(setupType any) bool {
	return setupType == string(FamilyKeltnerReversion) || setupType == string(FamilyKeltnerExpansion)
}

// keltnerNumber parses a plain numeric token, returning NaN for anything
// that is not a finite number. Unlike parseNumber it does not tolerate an
// "R"/"%" suffix — keltner's numeric phrases (ema length, distance, pierce,
// reclaim) are always plain numbers.
func keltnerNumber(token string) float64 {
	if token == "" {
		return math.NaN()
	}
	value, err := strconv.ParseFloat(token, 64)
	if err != nil {
		return math.NaN()
	}
	return value
}

// parseKeltnerLine mirrors parseKeltnerReversionLine in
// engine/dsl/spec/parseSetups/keltnerPhrases.js. Both the keltner-reversion
// and keltner-expansion families share this grammar and write into the same
// "keltnerReversion" config key. Returns false only if head is not one of
// the five recognized heads (defensive; callers already filter on head).
func (p *parser) parseKeltnerLine(line logicalLine, head string, tokens []string) bool {
	keltner := copyMap(p.config["keltnerReversion"])
	switch head {
	case "ema":
		value := keltnerNumber(tokenAt(tokens, 2))
		if len(tokens) != 3 || !strings.EqualFold(tokens[1], "length") || !isFiniteNumber(value) || value != math.Trunc(value) || value <= 1 {
			p.err(line, "keltner-reversion ema must be exactly: ema length N, with integer N greater than 1", "ema length N")
			return true
		}
		keltner["emaLen"] = value
	case "distance":
		value := keltnerNumber(tokenAt(tokens, 1))
		if len(tokens) != 3 || !strings.EqualFold(tokens[2], "atr") || !isFiniteNumber(value) || value <= 0 {
			p.err(line, "keltner-reversion band width must be exactly: distance X ATR, with finite positive X", "distance X ATR")
			return true
		}
		keltner["bandAtr"] = value
	case "pierce":
		value := keltnerNumber(tokenAt(tokens, 3))
		if len(tokens) != 5 || !strings.EqualFold(tokens[1], "at") || !strings.EqualFold(tokens[2], "least") ||
			!strings.EqualFold(tokens[4], "atr") || !isFiniteNumber(value) || value < 0 {
			p.err(line, "keltner-reversion pierce must be exactly: pierce at least X ATR, with finite non-negative X", "pierce at least X ATR")
			return true
		}
		keltner["minPierceAtr"] = value
	case "reclaim":
		value := keltnerNumber(tokenAt(tokens, 2))
		if len(tokens) != 4 || !strings.EqualFold(tokens[1], "within") || !strings.EqualFold(tokens[3], "candles") ||
			!isFiniteNumber(value) || value != math.Trunc(value) || value <= 0 {
			p.err(line, "keltner-reversion reclaim must be exactly: reclaim within N candles, with positive integer N", "reclaim within N candles")
			return true
		}
		keltner["reclaimCandles"] = value
	case "target":
		rToken := ""
		for _, token := range tokens {
			if keltnerTargetRTokenPattern.MatchString(token) {
				rToken = token
				break
			}
		}
		value := math.NaN()
		if rToken != "" {
			value = keltnerNumber(rToken[:len(rToken)-1])
		}
		if rToken == "" || !isFiniteNumber(value) || value <= 0 {
			p.err(line, "keltner-reversion target must be: target midline else NR | target NR, with finite positive N", "target NR")
			return true
		}
		if containsLower(tokens, "midline") {
			keltner["targetMode"] = "midlineElseFixedR"
		} else {
			keltner["targetMode"] = "fixedR"
		}
		keltner["targetR"] = value
	default:
		return false
	}
	p.config["keltnerReversion"] = keltner
	return true
}

// keltnerHeads are the directive heads a Keltner family claims outright. Every
// one of them is also a shared head owned by another family, so the claim is
// scoped to the setup type rather than to the head alone.
var keltnerHeads = map[string]bool{
	"ema": true, "distance": true, "pierce": true, "reclaim": true, "target": true,
}

// tryKeltnerLine routes a shared directive head to the Keltner phrase parser
// when a Keltner family is selected. It reports whether the head was consumed,
// so each call site keeps its non-Keltner behaviour unchanged.
func (p *parser) tryKeltnerLine(line logicalLine, head string, tokens []string) bool {
	if !isKeltnerSetupType(p.config["setupType"]) || !keltnerHeads[head] {
		return false
	}
	if !p.parseKeltnerLine(line, head, tokens) {
		p.err(line, "unrecognized "+head+" line — keltner-reversion phrases only", "")
	}
	return true
}
