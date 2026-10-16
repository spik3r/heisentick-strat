package dsl

import (
	"math"
	"strings"
)

func (p *parser) validatePriceMomentum() {
	if p.config["setupType"] != string(FamilyPriceMomentum) {
		return
	}
	momentum := copyMap(p.config["priceMomentum"])
	if _, ok := momentum["lookbackBars"]; !ok {
		p.errorAt(nil, nil, `price momentum requires "lookback N candles".`, "")
	}
	if _, ok := momentum["thresholdPct"]; !ok {
		p.errorAt(nil, nil, `price momentum requires "neutral zone X percent".`, "")
	}
	// An explicit `stop N ATR` must name a usable multiple. A zero, negative
	// or non-finite N would otherwise read as no fixed stop, and the family
	// would silently trade its recent-extreme stop instead.
	if stop := copyMap(p.config["stop"]); stop["type"] == "fixedAtr" {
		if multiple, ok := stop["atr"].(float64); !ok || !isFiniteNumber(multiple) || multiple <= 0 {
			p.errorAt(nil, nil, "price momentum stop N ATR requires a finite positive N.", "stop 2 ATR")
		}
	}
}

func (p *parser) parseLookback(line logicalLine, tokens []string) {
	if p.config["setupType"] == string(FamilyPriceMomentum) {
		p.parsePriceMomentumLookback(line, tokens)
	} else {
		p.parseTriplePush(tokens)
	}
}

func (p *parser) parsePriceMomentumLookback(line logicalLine, tokens []string) {
	momentum := copyMap(p.config["priceMomentum"])
	momentum["lookbackBars"] = nil
	p.config["priceMomentum"] = momentum
	value := firstNumber(tokens, 1, math.NaN())
	if len(tokens) != 3 || !strings.EqualFold(tokens[2], "candles") || !isFiniteNumber(value) || value <= 0 || value != math.Trunc(value) {
		p.err(line, "price-momentum lookback must be exactly: lookback N candles, with positive integer N", "")
		return
	}
	momentum["lookbackBars"] = value
}

func (p *parser) parsePriceMomentumNeutral(line logicalLine, tokens []string) {
	if p.config["setupType"] != string(FamilyPriceMomentum) {
		p.unknownDirective(line, tokens[0])
		return
	}
	momentum := copyMap(p.config["priceMomentum"])
	momentum["thresholdPct"] = nil
	p.config["priceMomentum"] = momentum
	value := firstNumber(tokens, 2, math.NaN())
	if len(tokens) != 4 || !strings.EqualFold(tokens[1], "zone") || !strings.EqualFold(tokens[3], "percent") || !isFiniteNumber(value) || value <= 0 {
		p.err(line, "price-momentum neutral zone must be exactly: neutral zone X percent, with finite positive X", "")
		return
	}
	momentum["thresholdPct"] = value
}

func isFiniteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
