package engine

import (
	"math"
	"reflect"
	"regexp"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
)

// This is technical option admission, not a source/strategy approval list.
// The ordinary Go parser remains the sole producer of execution configuration.
func validatePaperVWAP(source string, cfg dsl.Config, route PaperRoute, units PaperUnits, costs PaperCosts) error {
	if costs.FillOn != "close" {
		return paperError("FILL_UNSUPPORTED")
	}
	if route.Timeframe != "15m" || route.RangeMethod != "zone" || units.QuoteCurrency != "USD" {
		return paperError("ADMISSION")
	}
	if !paperVWAPAuthoredSupported(source) {
		return paperError("ADMISSION")
	}
	neutral, err := dsl.Parse("dsl v7\nsetup { type: vwap extension fade }\n")
	if err != nil || len(neutral.Errors) != 0 || !paperVWAPConfigSupported(map[string]any(cfg), map[string]any(neutral.Config), "") {
		return paperError("ADMISSION")
	}
	p := paramsFromConfig(cfg)
	v := p.VWAPExtensionFade
	for _, n := range []float64{p.RiskUSD, p.BreakevenR, p.BreakevenOffsetATR, p.MaxHoldBars, p.MaxMovementER, p.DayTypeEREscape, v.MinDistanceATR, v.TailRejectionMin, v.CloseLocationMin, v.StopBufferATR, v.MinStopATR, v.MaxStopATR, v.TargetR, v.MinTargetR} {
		if !isFinite(n) {
			return paperError("ADMISSION")
		}
	}
	if p.RiskUSD <= 0 || p.BreakevenR < 0 || p.BreakevenOffsetATR < 0 || p.MaxMovementER < 0 || (p.DayTypeEREscape < 0 && p.DayTypeEREscape != -1) ||
		v.MinDistanceATR <= 0 || v.TailRejectionMin < 0 || v.TailRejectionMin > 1 || v.CloseLocationMin < 0 || v.CloseLocationMin > 1 ||
		v.StopBufferATR < 0 || v.MinStopATR < 0 || v.MaxStopATR <= 0 || v.MinStopATR > v.MaxStopATR || v.TargetR <= 0 || v.MinTargetR <= 0 || v.TargetMode != "vwap" || !v.RequireRangeDay {
		return paperError("ADMISSION")
	}
	// Test the parsed values, before legacy integer conversion/rounding.
	for _, n := range []float64{numberValue(mapValue(cfg, "stop"), "extremeCandles", 0), numberFromAny(cfg["cooldownCandles"], -1), p.MaxHoldBars} {
		if !isFinite(n) || n < 0 || n > PaperPrefixMaxBars || math.Trunc(n) != n {
			return paperError("ADMISSION")
		}
	}
	return nil
}

var paperVWAPVariableLeaves = map[string]bool{
	"name": true, "description": true, "symbols": true, "timeframes": true, "slices": true,
	"allowLong": true, "allowShort": true, "sessions.asia": true, "sessions.mid": true, "sessions.london": true, "sessions.ny": true,
	"sessionPhases": true, "dayTypes": true, "dayTypeErEscape": true, "maxMovementEr": true,
	"vwapExtensionFade.minDistanceAtr": true, "vwapExtensionFade.requireRangeDay": true, "vwapExtensionFade.targetMode": true,
	"tailRejectionMin": true, "closeLocationMin": true, "stop.extremeCandles": true, "stop.paddingAtr": true, "stop.minAtr": true, "stop.maxAtr": true,
	"target.vefR": true, "target.minR": true,
	"riskUsd": true, "breakeven.atR": true, "breakeven.offsetAtr": true, "cooldownCandles": true, "maxHoldCandles": true,
}

func paperVWAPConfigSupported(actual, neutral map[string]any, prefix string) bool {
	for key, value := range actual {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		base, exists := neutral[key]
		if paperVWAPVariableLeaves[path] {
			if !paperVWAPLeafShape(path, value) {
				return false
			}
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			baseline, _ := base.(map[string]any)
			if !exists || !paperVWAPConfigSupported(nested, baseline, path) {
				return false
			}
		} else if !exists || !reflect.DeepEqual(value, base) {
			return false
		}
	}
	for key := range neutral {
		if _, ok := actual[key]; !ok {
			return false
		}
	}
	return true
}

// Allowed leaves remain leaves: no future nested option can hide below one.
func paperVWAPLeafShape(path string, value any) bool {
	switch path {
	case "slices":
		list, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok || len(m) != 2 {
				return false
			}
			if _, ok := m["symbol"].(string); !ok {
				return false
			}
			if _, ok := m["tf"].(string); !ok {
				return false
			}
		}
		return true
	case "symbols", "timeframes", "sessionPhases", "dayTypes":
		switch list := value.(type) {
		case []string:
			return true
		case []any:
			for _, item := range list {
				if _, ok := item.(string); !ok {
					return false
				}
			}
			return true
		}
	case "name", "description", "vwapExtensionFade.targetMode":
		_, ok := value.(string)
		return ok
	default:
		switch value.(type) {
		case int, float64:
			return true
		}
	}
	return false
}

// Recognize only qualified authored forms. These checks never parse values into
// a second config. They reject clauses that ordinary parsing can silently erase,
// including a default-equivalent take-profit or a later overwritten directive.
var paperVWAPForms = func() map[string][]*regexp.Regexp {
	n := `[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:e[+-]?[0-9]+)?`
	forms := map[string][]string{
		"|dsl":              {`dsl v7`},
		"market|symbols":    {`symbols [a-z0-9_./:-]+(?: [a-z0-9_./:-]+)*`},
		"market|timeframes": {`timeframes (?:1m|5m|15m|30m|1h|4h|1d)(?: (?:1m|5m|15m|30m|1h|4h|1d))*`},
		"market|slices":     {`slices [a-z0-9_./:-]+ (?:1m|5m|15m|30m|1h|4h|1d)(?: [a-z0-9_./:-]+ (?:1m|5m|15m|30m|1h|4h|1d))*`},
		"market|sessions":   {`sessions (?:asia|mid|london|ny)(?: (?:asia|mid|london|ny))*`},
		"market|session":    {`session phase in (?:open|middle|lunch|close|overnight|other)(?: (?:open|middle|lunch|close|overnight|other))*`},
		"market|day":        {`day type in (?:trending|ranging|choppy)(?: (?:trending|ranging|choppy))*(?: strictly)?`},
		"market|movement":   {`movement below ` + n}, "market|trendiness": {`trendiness below ` + n},
		"setup|type":     {`type vwap extension fade`},
		"setup|distance": {`distance from vwap at least ` + n + ` atr`},
		"setup|require":  {`require range day`}, "setup|target": {`target vwap`},
		"filters|side": {`side (?:both|long|short)(?: only)?`},
		"filters|tail": {`tail rejection at least ` + n}, "filters|close": {`close location at least ` + n},
		"risk|stop":            {`stop beyond last ` + n + ` candle(?:s)? extreme by ` + n + ` atr`, `stop size <= ` + n + ` atr`, `stop size between ` + n + ` and ` + n + ` atr`},
		"target|target":        {`target ` + n + ` ?r`, `target vwap`},
		"target|minimum":       {`minimum reward ` + n + ` ?r`},
		"management|move":      {`move stop to breakeven after ` + n + ` ?r plus ` + n + ` atr`},
		"management|breakeven": {`breakeven off`}, "management|trail": {`trail off`},
		"management|maxholdcandles": {`maxholdcandles ` + n},
		"management|wait":           {`wait ` + n + ` candles after trade`},
		"execution|risk":            {`risk ` + n + ` usd`},
	}
	out := map[string][]*regexp.Regexp{}
	for key, patterns := range forms {
		for _, pattern := range patterns {
			out[key] = append(out[key], regexp.MustCompile(`(?i)^`+pattern+`$`))
		}
	}
	return out
}()

var paperVWAPMetadata = regexp.MustCompile(`(?i)^(?:strategy|name|description) (?:"[^"]*"|'[^']*')$`)

func paperVWAPAuthoredSupported(source string) bool {
	audit := dsl.AuditOrdinarySource(source)
	if len(audit.Issues) != 0 {
		return false
	}
	typeSeen := false
	for _, directive := range audit.Directives {
		head := strings.ToLower(directive.Tokens[0])
		if (directive.Section == "strategy" || directive.Section == "") && head == "strategy" {
			if !paperVWAPMetadata.MatchString(strings.Join(directive.Tokens, " ")) {
				return false
			}
			continue
		}
		if directive.Section == "strategy" && (head == "name" || head == "description") {
			if !paperVWAPMetadata.MatchString(strings.Join(directive.Tokens, " ")) {
				return false
			}
			continue
		}
		if head == "type" {
			if typeSeen {
				return false
			}
			typeSeen = true
		} else if !typeSeen && (directive.Section == "setup" || directive.Section == "risk" || directive.Section == "target" || directive.Section == "management") {
			return false
		}
		matched := false
		for _, pattern := range paperVWAPForms[directive.Section+"|"+head] {
			if pattern.MatchString(strings.Join(directive.Tokens, " ")) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return typeSeen
}

type paperVWAPState struct {
	Last     int      `json:"last"`
	HasLast  bool     `json:"hasLast"`
	SeenDay  int64    `json:"seenDay"`
	SeenKeys []string `json:"seenKeys"`
}

func paperVWAPStateFromBroker(b *broker) *paperVWAPState {
	if b.params.SetupType != string(dsl.FamilyVWAPExtensionFade) {
		return nil
	}
	return &paperVWAPState{b.vefLast, b.hasVEFLast, b.seen.vef.day, append([]string(nil), b.seen.vef.keys...)}
}
