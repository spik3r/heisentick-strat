package dsl

import (
	"fmt"
	"math"
	"strings"
)

// named level sweep (spec §2.4, plan 2026-09-26-dsl-named-level-sweep.md):
//
//	when price sweeps <LEVEL> [by at least X ATR] and closes back above|below it [by at least Y ATR] [within N candles] then signal <side> [grade G]
//	when price tests  <LEVEL> within X ATR         and closes       above|below it [by at least Y ATR] [within N candles] then signal <side> [grade G]
//
// Both verbs write into cfg.namedLevelSweep.rules[<LEVEL>]. The JS compiler's
// strat/implementations/browser-runtime/compiler/parseSetups/namedLevelSweepPhrases.js
// (parseNamedLevelSweepWhenLine, dispatched from compile/setupLowering.js's
// parseWhenLine) mirrors this function phrase-for-phrase and
// message-for-message.

// namedLevelSweepHighLevels / namedLevelSweepLowLevels are the named-level
// keys whose polarity is fixed by the family's semantics (spec §1.2): a
// high-type level defaults to short, a low-type level defaults to long. Keys
// outside both sets (VWAP, EMA, POC, VAH, VAL, DO, CAM_*, RN<step>, a custom
// S/R name, ...) have no default polarity to warn against.
var namedLevelSweepHighLevels = map[string]bool{"PDH": true, "WH": true, "AH": true, "LH": true, "NH": true, "DH": true}
var namedLevelSweepLowLevels = map[string]bool{"PDL": true, "WL": true, "AL": true, "LL": true, "NL": true, "DL": true}

func namedLevelSweepLevelPolarity(level string) (string, bool) {
	if namedLevelSweepHighLevels[level] {
		return "high", true
	}
	if namedLevelSweepLowLevels[level] {
		return "low", true
	}
	return "", false
}

func (p *parser) parseNamedLevelSweepWhen(line logicalLine, tokens []string) {
	if len(tokens) < 4 || !strings.EqualFold(tokens[1], "price") {
		p.err(line, `named level sweep entry rule must start with "when price sweeps ..." or "when price tests ..."`, "")
		return
	}
	verb := strings.ToLower(tokens[2])
	if verb != "sweeps" && verb != "tests" {
		p.err(line, `named level sweep entry rule must use "sweeps" or "tests"`, "")
		return
	}
	level := normalizeLevel(tokens[3])
	if !isKnownLevel(level) {
		p.err(line, unknownLevelMessage("entry rule", level), "")
		return
	}

	andIdx := indexOfLower(tokens, "and")
	if andIdx < 0 {
		p.err(line, `named level sweep entry rule requires "and closes ..."`, "")
		return
	}
	between := tokens[4:andIdx]

	var sweepAtr *float64
	if verb == "sweeps" {
		if len(between) > 0 {
			if !strings.EqualFold(between[0], "by") {
				p.err(line, `"sweeps" distance clause must be "by at least X ATR"`, "")
				return
			}
			value := firstNumber(between, 1, math.NaN())
			if math.IsNaN(value) {
				p.err(line, `"sweeps ... by at least X ATR" must include a number`, "")
				return
			}
			if value < 0 {
				p.err(line, "sweep/test distance must be >= 0", "")
				return
			}
			sweepAtr = &value
		}
	} else {
		if len(between) == 0 || !strings.EqualFold(between[0], "within") {
			p.err(line, `"tests" entry rule requires "within X ATR" — there is no default tolerance for "tests"`, "")
			return
		}
		value := firstNumber(between, 1, math.NaN())
		if math.IsNaN(value) {
			p.err(line, `"tests <LEVEL> within X ATR" must include a number`, "")
			return
		}
		if value < 0 {
			p.err(line, "sweep/test distance must be >= 0", "")
			return
		}
		sweepAtr = &value
	}

	rest := tokens[andIdx+1:]
	if len(rest) == 0 || !strings.EqualFold(rest[0], "closes") {
		p.err(line, `named level sweep entry rule requires "and closes above|below it"`, "")
		return
	}
	idx := 1
	if idx < len(rest) && strings.EqualFold(rest[idx], "back") {
		idx++
	}
	if idx >= len(rest) {
		p.err(line, `named level sweep entry rule requires a close direction: "above" or "below"`, "")
		return
	}
	closeSideWord := strings.ToLower(rest[idx])
	if closeSideWord != "above" && closeSideWord != "below" {
		p.err(line, `named level sweep close direction must be "above" or "below"`, "")
		return
	}
	idx++
	if idx >= len(rest) || !strings.EqualFold(rest[idx], "it") {
		p.err(line, `named level sweep entry rule requires "it" after the close direction`, "")
		return
	}
	idx++

	reclaimAtr := 0.0
	if idx < len(rest) && strings.EqualFold(rest[idx], "by") {
		value := firstNumber(rest, idx+1, math.NaN())
		if math.IsNaN(value) {
			p.err(line, `"by at least Y ATR" must include a number`, "")
			return
		}
		if value < 0 {
			p.err(line, "sweep/test distance must be >= 0", "")
			return
		}
		reclaimAtr = value
		for idx < len(rest) && !strings.EqualFold(rest[idx], "within") && !strings.EqualFold(rest[idx], "then") {
			idx++
		}
	}

	reclaimCandles := 1.0
	if idx < len(rest) && strings.EqualFold(rest[idx], "within") {
		value := firstNumber(rest, idx+1, math.NaN())
		if !math.IsNaN(value) {
			reclaimCandles = value
		}
		for idx < len(rest) && !strings.EqualFold(rest[idx], "then") {
			idx++
		}
	}

	if idx >= len(rest) || !strings.EqualFold(rest[idx], "then") {
		p.err(line, `named level sweep entry rule requires "then signal <side>"`, "")
		return
	}
	idx++
	if idx >= len(rest) || !strings.EqualFold(rest[idx], "signal") {
		p.err(line, `named level sweep entry rule requires "then signal <side>"`, "")
		return
	}
	idx++
	if idx >= len(rest) {
		p.err(line, `named level sweep entry rule requires a side after "signal"`, "")
		return
	}
	var side string
	switch strings.ToLower(rest[idx]) {
	case "long", "buy":
		side = "long"
	case "short", "sell":
		side = "short"
	default:
		p.err(line, `named level sweep signal side must be "long" or "short"`, "")
		return
	}
	idx++
	grade := ""
	if idx < len(rest) && strings.EqualFold(rest[idx], "grade") && idx+1 < len(rest) {
		grade = rest[idx+1]
	}

	// Compile error (spec §2.4/§3.5): the close direction must agree with the
	// signalled side's polarity — a long reclaims upward (closes above the
	// level), a short reclaims downward (closes below it).
	wantClose := "above"
	if side == "short" {
		wantClose = "below"
	}
	if closeSideWord != wantClose {
		p.err(line, fmt.Sprintf("close direction %q does not match \"then signal %s\" — use %q", closeSideWord, side, wantClose), "")
		return
	}

	// Compile warning (spec §3.5): the level's own polarity contradicts the
	// signalled side. Not an error — a future strategy may legitimately want
	// the countertrend direction for a custom level.
	if polarity, ok := namedLevelSweepLevelPolarity(level); ok {
		defaultSide := "long"
		if polarity == "high" {
			defaultSide = "short"
		}
		if defaultSide != side {
			p.warn(line, fmt.Sprintf("%s is a %s-type level; 'then signal %s' trades against its default polarity — confirm this is intended", level, polarity, side), "")
		}
	}

	nls := copyMap(p.config["namedLevelSweep"])
	rules := copyMap(nls["rules"])
	rule := map[string]any{
		"level":          level,
		"mode":           map[string]string{"sweeps": "sweep", "tests": "test"}[verb],
		"reclaimAtr":     reclaimAtr,
		"reclaimCandles": reclaimCandles,
		"side":           side,
	}
	if sweepAtr != nil {
		rule["sweepAtr"] = *sweepAtr
	} else {
		rule["sweepAtr"] = nil
	}
	if grade != "" {
		rule["grade"] = grade
	}
	rules[level] = rule
	nls["rules"] = rules
	p.config["namedLevelSweep"] = nls
}

// parseNamedLevelSweepStopLine handles the family's two stop phrases (spec
// §2.4): "stop below the signal candle and the level by X ATR" (longs) and
// "stop above the signal candle and the level by X ATR" (shorts). It returns
// false for any other "stop ..." line so parseStopDirective falls through to
// the generic stop parser (e.g. "stop size min A max B", used alongside this
// phrase in the family's own examples).
func (p *parser) parseNamedLevelSweepStopLine(line logicalLine, tokens []string) bool {
	if len(tokens) < 2 {
		return false
	}
	dirWord := strings.ToLower(tokens[1])
	if dirWord != "below" && dirWord != "above" {
		return false
	}
	phrase := strings.ToLower(strings.Join(tokens[2:], " "))
	if !strings.HasPrefix(phrase, "the signal candle and the level by ") {
		return false
	}
	value := firstNumber(tokens, 2, math.NaN())
	if math.IsNaN(value) {
		p.err(line, `named level sweep stop must include a number: "stop below the signal candle and the level by X ATR"`, "")
		return true
	}
	if value < 0 {
		p.err(line, "named level sweep stop padding must be >= 0", "")
		return true
	}
	nls := copyMap(p.config["namedLevelSweep"])
	stop := copyMap(nls["stop"])
	if dirWord == "below" {
		stop["long"] = map[string]any{"paddingAtr": value}
	} else {
		stop["short"] = map[string]any{"paddingAtr": value}
	}
	nls["stop"] = stop
	p.config["namedLevelSweep"] = nls
	return true
}

// parseAllowDirective handles `allow re-entry on the same level after a
// trade closes` (spec §2.4 / A2): the family's explicit opt-out of its
// default one-signal-per-level-per-day cap. It removes the daily dedup for
// every rule but leaves the "no open position" gate untouched — this is a
// single-position engine (broker.hasPosition), so that gate already holds
// unconditionally in onNamedLevelSweepBar/onNamedLevelSweepBarJS regardless
// of this phrase. Named-level-sweep-only: the phrase is meaningless for any
// other family, since no other family's dedup is keyed the same way.
func (p *parser) parseAllowDirective(line logicalLine, tokens []string) {
	phrase := strings.ToLower(strings.Join(tokens, " "))
	if phrase != "allow re-entry on the same level after a trade closes" {
		p.unknownDirective(line, tokens[0])
		return
	}
	// Deferred to result()/validateNamedLevelSweepReentry: this phrase's own
	// examples put it in `filters { }`, ahead of `setup { type: ... }` later
	// in the source, so the setupType check cannot run here without making
	// the phrase order-sensitive.
	saved := line
	p.allowReentryLine = &saved
	// Nested under namedLevelSweep (not top-level): a top-level key would
	// show up, always false, in every other family's golden config —
	// breaking the v6/v7 byte-identical-goldens requirement, since
	// defaultConfig() has no per-family scoping. setup { type: named level
	// sweep }'s own dispatch (parser_setups.go) preserves any key already
	// set here when this phrase (in `filters { }`, per its own example)
	// is parsed before that block, as the family's worked examples order
	// them — see the FamilyNamedLevelSweep case there.
	nls := copyMap(p.config["namedLevelSweep"])
	nls["allowSameLevelReentry"] = true
	p.config["namedLevelSweep"] = nls
}

// validateNamedLevelSweepReentry reports the deferred setupType check for
// parseAllowDirective (see its comment): the phrase only means something for
// `type: named level sweep`.
func (p *parser) validateNamedLevelSweepReentry() {
	if p.allowReentryLine == nil {
		return
	}
	if p.config["setupType"] != string(FamilyNamedLevelSweep) {
		p.err(*p.allowReentryLine, `"allow re-entry on the same level after a trade closes" is only valid for setup type: named level sweep`, "")
	}
}

// validateNamedLevelSweep warns when `type: named level sweep` is declared
// with no `when price sweeps|tests ...` entry rule at all — the setup would
// then never signal (spec §3.5).
func (p *parser) validateNamedLevelSweep() {
	if p.config["setupType"] != string(FamilyNamedLevelSweep) {
		return
	}
	nls, _ := p.config["namedLevelSweep"].(map[string]any)
	rules, _ := nls["rules"].(map[string]any)
	if len(rules) == 0 {
		p.warnAt(nil, nil, `named level sweep declares no entry rule — add a "when price sweeps ..." or "when price tests ..." line, or nothing will ever signal`, "")
	}
}
