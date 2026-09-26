package engine

import (
	"fmt"
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/dsl"
)

// named level sweep (spec/dsl-spec-families/namedLevelSweep.md; plan
// heisentick-backlog/plans/2026-09-26-dsl-named-level-sweep.md §3.2): a
// wick through, or a close approach to, a single *named* level (from the
// shared `priority(...)` universe — PDH/PDL, WH/WL, LH/LL, ... via the same
// `keyLevel` resolver `failed breakout` already uses), followed by a close
// back on the level's origin side.
//
// This is the family's Phase A/B boundary implementation: the geometry, the
// unified stop formula, and the one-signal-per-level-per-day dedup are
// implemented (mirroring the JS engine's engine/dsl/setups/namedLevelSweep.js
// phrase-for-phrase). Deliberately out of scope here, left for a follow-up
// pass: HTF-trend agreement gating specific to this family (the shared
// `marketGatesOK` non-session gates — sessions, prior day type/range, day
// type/ER — already apply, since every family shares them) and a dedicated
// trigger-candle path beyond the shared `triggerOK`.

type namedLevelSweepRule struct {
	Level          string
	Mode           string // "sweep" | "test"
	HasSweepAtr    bool
	SweepAtr       float64
	ReclaimAtr     float64
	ReclaimCandles int
	Side           side
	Grade          string
}

type namedLevelSweepStopSide struct {
	HasPadding bool
	PaddingATR float64
}

type namedLevelSweepParams struct {
	Rules     map[string]namedLevelSweepRule
	StopLong  namedLevelSweepStopSide
	StopShort namedLevelSweepStopSide
}

func namedLevelSweepParamsFromConfig(cfg dsl.Config) namedLevelSweepParams {
	nls := mapValue(cfg, "namedLevelSweep")
	rulesRaw, _ := nls["rules"].(map[string]any)
	rules := map[string]namedLevelSweepRule{}
	for level, raw := range rulesRaw {
		ruleMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		mode, _ := ruleMap["mode"].(string)
		s := sideLong
		if sideStr, _ := ruleMap["side"].(string); sideStr == "short" {
			s = sideShort
		}
		rule := namedLevelSweepRule{
			Level:          level,
			Mode:           mode,
			ReclaimAtr:     numberFromAny(ruleMap["reclaimAtr"], 0),
			ReclaimCandles: intFromAny(ruleMap["reclaimCandles"], 1),
			Side:           s,
		}
		if v, ok := ruleMap["sweepAtr"]; ok && v != nil {
			rule.HasSweepAtr = true
			rule.SweepAtr = numberFromAny(v, 0)
		}
		if grade, ok := ruleMap["grade"].(string); ok {
			rule.Grade = grade
		}
		rules[level] = rule
	}
	stop := mapValue(nls, "stop")
	result := namedLevelSweepParams{Rules: rules}
	if long := mapValue(stop, "long"); long != nil {
		result.StopLong = namedLevelSweepStopSide{HasPadding: true, PaddingATR: numberValue(long, "paddingAtr", 0)}
	}
	if short := mapValue(stop, "short"); short != nil {
		result.StopShort = namedLevelSweepStopSide{HasPadding: true, PaddingATR: numberValue(short, "paddingAtr", 0)}
	}
	return result
}

// sortedNamedLevelSweepLevels returns the rule map's keys in a fixed,
// deterministic order (map iteration in Go is randomized) so that a bar with
// more than one eligible rule always evaluates them in the same order across
// runs — required for byte-identical conformance trades.
func sortedNamedLevelSweepLevels(rules map[string]namedLevelSweepRule) []string {
	levels := make([]string, 0, len(rules))
	for level := range rules {
		levels = append(levels, level)
	}
	sort.Strings(levels)
	return levels
}

func (b *broker) onNamedLevelSweepBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	if !b.marketGatesOK(i) {
		return
	}
	p := b.params
	if b.hasNLSEntry && i-b.nlsLastEntry < p.CooldownBars {
		return
	}
	atr := finiteOrZero(b.cols.ATR[i])
	if atr == 0 {
		return
	}
	for _, level := range sortedNamedLevelSweepLevels(p.NamedLevelSweep.Rules) {
		rule := p.NamedLevelSweep.Rules[level]
		s := rule.Side
		if s == sideLong && !p.AllowLong {
			continue
		}
		if s == sideShort && !p.AllowShort {
			continue
		}
		setup, ok := b.namedLevelSweepSetup(i, rule, atr)
		if !ok {
			continue
		}
		day := localDayKey(b.series.T[i])
		seenKey := fmt.Sprintf("%s:%s", s.String(), level)
		if b.seen.nls.seen(day, seenKey) {
			continue
		}
		b.seen.nls.add(day, seenKey)
		b.enterSetup(i, setup)
		b.nlsLastEntry = i
		b.hasNLSEntry = true
		break
	}
}

func (b *broker) namedLevelSweepSetup(i int, rule namedLevelSweepRule, atr float64) (setupPlan, bool) {
	level, ok := b.keyLevel(i, rule.Level, b.series.C[i])
	if !ok {
		return setupPlan{}, false
	}
	if !b.namedLevelSweptOrTested(i, rule, level.Price, atr) {
		return setupPlan{}, false
	}
	if !triggerOK(b.series, i, rule.Side, b.params.NLSUseTrigger) {
		return setupPlan{}, false
	}
	entry := b.series.C[i]
	stop := b.namedLevelSweepStop(i, level.Price, rule.Side, atr)
	risk := math.Abs(entry - stop)
	if risk <= 0 {
		return setupPlan{}, false
	}
	target := entry + float64(rule.Side)*risk*b.params.TargetR
	meta := gradeMeta(TradeMeta{
		"setup":      "namedLevelSweep",
		"side":       rule.Side.String(),
		"levelKey":   level.Key,
		"levelPrice": level.Price,
		"mode":       rule.Mode,
	})
	if rule.Grade != "" {
		meta["grade"] = rule.Grade
	}
	return setupPlan{
		Side:   rule.Side,
		Stop:   stop,
		Target: target,
		Tag:    "DSL-NLS:" + level.Key,
		Meta:   meta,
	}, true
}

// namedLevelSweptOrTested implements both entry-trigger verbs (spec §1.2 /
// §2.4): "sweeps" requires the wick to clear the level (by at least
// SweepAtr*ATR if given, any strict crossing otherwise); "tests" requires
// only that the wick reach within SweepAtr*ATR of the level. Either way the
// signal bar's own close must then sit past the level's origin side by at
// least ReclaimAtr*ATR, and the wick must have occurred within the last
// ReclaimCandles bars (default 1 = the signal bar itself).
func (b *broker) namedLevelSweptOrTested(i int, rule namedLevelSweepRule, levelPrice float64, atr float64) bool {
	window := rule.ReclaimCandles
	if window < 1 {
		window = 1
	}
	from := maxInt(0, i-window+1)
	wicked := false
	for j := from; j <= i; j++ {
		if rule.Side == sideLong {
			if rule.Mode == "test" {
				tol := rule.SweepAtr * atr
				if b.series.L[j] <= levelPrice+tol {
					wicked = true
				}
			} else {
				if !rule.HasSweepAtr {
					if b.series.L[j] < levelPrice {
						wicked = true
					}
				} else if b.series.L[j] <= levelPrice-rule.SweepAtr*atr {
					wicked = true
				}
			}
		} else {
			if rule.Mode == "test" {
				tol := rule.SweepAtr * atr
				if b.series.H[j] >= levelPrice-tol {
					wicked = true
				}
			} else {
				if !rule.HasSweepAtr {
					if b.series.H[j] > levelPrice {
						wicked = true
					}
				} else if b.series.H[j] >= levelPrice+rule.SweepAtr*atr {
					wicked = true
				}
			}
		}
	}
	if !wicked {
		return false
	}
	margin := rule.ReclaimAtr * atr
	if rule.Side == sideLong {
		return b.series.C[i] >= levelPrice+margin
	}
	return b.series.C[i] <= levelPrice-margin
}

// namedLevelSweepStop is the family's unified stop formula (spec §2.4): the
// further-away of the signal bar's own extreme and the level price, padded
// by the declared per-side amount. A long's "stop below" uses
// min(low, level) - padding; a short's "stop above" uses
// max(high, level) + padding. A side with no declared stop phrase falls back
// to the shared generic ATR stop bounds (0 padding) rather than refusing the
// setup outright.
func (b *broker) namedLevelSweepStop(i int, levelPrice float64, s side, atr float64) float64 {
	padding := 0.0
	if s == sideLong && b.params.NamedLevelSweep.StopLong.HasPadding {
		padding = b.params.NamedLevelSweep.StopLong.PaddingATR
	} else if s == sideShort && b.params.NamedLevelSweep.StopShort.HasPadding {
		padding = b.params.NamedLevelSweep.StopShort.PaddingATR
	}
	if s == sideLong {
		return math.Min(b.series.L[i], levelPrice) - padding*atr
	}
	return math.Max(b.series.H[i], levelPrice) + padding*atr
}
