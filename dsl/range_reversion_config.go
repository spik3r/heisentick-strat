package dsl

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

const (
	RangeReversionDelayedPinePolicy       = "DELAYED_PINE_OHLC_V1"
	RangeReversionImmediatePolicy         = "IMMEDIATE_STOP_FIRST_V1"
	RangeReversionSourceAvailability      = "NEXT_NATIVE_ROW"
	RangeReversionDedicatedRunnerRequired = "range-reversion-native-dedicated-runner-required"
)

type RangeReversionRules struct {
	Policy               string  `json:"policy"`
	Timeframe            string  `json:"timeframe"`
	SourceTimeframe      string  `json:"sourceTimeframe"`
	Bounds               string  `json:"bounds"`
	BoundsLookback       int     `json:"boundsLookback"`
	SourceAvailability   string  `json:"sourceAvailability"`
	RequireCandleColor   bool    `json:"requireCandleColor"`
	UseHTFEMA            bool    `json:"useHtfEma"`
	HTFEMALength         int     `json:"htfEmaLength"`
	UseVectorGates       bool    `json:"useVectorGates"`
	VectorLength         int     `json:"vectorLength"`
	MaxADX               float64 `json:"maxAdx"`
	MinCHOP              float64 `json:"minChop"`
	UseRangeExpansion    bool    `json:"useRangeExpansion"`
	RangeATRMultiple     float64 `json:"rangeAtrMultiple"`
	ATRLength            int     `json:"atrLength"`
	StopATRMultiple      float64 `json:"stopAtrMultiple"`
	TargetR              float64 `json:"targetR"`
	CooldownBars         int     `json:"cooldownBars"`
	BreakEvenEnabled     bool    `json:"breakEvenEnabled"`
	BreakEvenTriggerR    float64 `json:"breakEvenTriggerR"`
	BreakEvenOffsetTicks int     `json:"breakEvenOffsetTicks"`
	TickSize             float64 `json:"tickSize"`
}

func (r RangeReversionRules) projection() map[string]any {
	return map[string]any{
		"policy": r.Policy, "timeframe": r.Timeframe, "sourceTimeframe": r.SourceTimeframe,
		"bounds": r.Bounds, "boundsLookback": int64(r.BoundsLookback), "sourceAvailability": r.SourceAvailability,
		"requireCandleColor": r.RequireCandleColor, "useHtfEma": r.UseHTFEMA, "htfEmaLength": int64(r.HTFEMALength),
		"useVectorGates": r.UseVectorGates, "vectorLength": int64(r.VectorLength), "maxAdx": r.MaxADX, "minChop": r.MinCHOP,
		"useRangeExpansion": r.UseRangeExpansion, "rangeAtrMultiple": r.RangeATRMultiple, "atrLength": int64(r.ATRLength),
		"stopAtrMultiple": r.StopATRMultiple, "targetR": r.TargetR, "cooldownBars": int64(r.CooldownBars),
		"breakEvenEnabled": r.BreakEvenEnabled, "breakEvenTriggerR": r.BreakEvenTriggerR,
		"breakEvenOffsetTicks": int64(r.BreakEvenOffsetTicks), "tickSize": r.TickSize,
	}
}

type RangeReversionSpec struct {
	Rules RangeReversionRules `json:"rules"`
}

func DecodeRangeReversion(cfg Config) (RangeReversionSpec, error) {
	var spec RangeReversionSpec
	if len(cfg) != 5 {
		return spec, fmt.Errorf("range-reversion config has unknown or missing top-level keys")
	}
	if version, ok := cfg["dslVersion"].(int64); !ok || version != 7 {
		return spec, fmt.Errorf("range-reversion requires DSL version 7")
	}
	if family, _ := cfg["setupType"].(string); family != string(FamilyRangeReversion) {
		return spec, fmt.Errorf("range-reversion setup type required")
	}
	for _, key := range []string{"name", "description"} {
		value, ok := cfg[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return spec, fmt.Errorf("range-reversion %s is required", key)
		}
	}
	obj, ok := cfg["rangeReversion"].(map[string]any)
	if !ok {
		return spec, fmt.Errorf("rangeReversion object required")
	}
	allowed := map[string]bool{"policy": true, "timeframe": true, "sourceTimeframe": true, "bounds": true, "boundsLookback": true, "sourceAvailability": true, "requireCandleColor": true, "useHtfEma": true, "htfEmaLength": true, "useVectorGates": true, "vectorLength": true, "maxAdx": true, "minChop": true, "useRangeExpansion": true, "rangeAtrMultiple": true, "atrLength": true, "stopAtrMultiple": true, "targetR": true, "cooldownBars": true, "breakEvenEnabled": true, "breakEvenTriggerR": true, "breakEvenOffsetTicks": true, "tickSize": true}
	if len(obj) != len(allowed) {
		return spec, fmt.Errorf("rangeReversion has unknown or missing keys")
	}
	for key := range obj {
		if !allowed[key] {
			return spec, fmt.Errorf("rangeReversion has unknown key %s", key)
		}
	}
	raw, err := json.Marshal(obj)
	if err != nil {
		return spec, err
	}
	if err = json.Unmarshal(raw, &spec.Rules); err != nil {
		return spec, err
	}
	r := spec.Rules
	if r.Policy != RangeReversionDelayedPinePolicy && r.Policy != RangeReversionImmediatePolicy {
		return spec, fmt.Errorf("rangeReversion.policy is unsupported")
	}
	if r.Timeframe != "M30" && r.Timeframe != "H1" {
		return spec, fmt.Errorf("rangeReversion.timeframe must be M30 or H1")
	}
	if r.SourceTimeframe != "H4" || r.SourceAvailability != RangeReversionSourceAvailability {
		return spec, fmt.Errorf("rangeReversion requires H4 source with NEXT_NATIVE_ROW availability")
	}
	if (r.Bounds != "CHART" && r.Bounds != "SOURCE") || r.BoundsLookback < 1 || r.BoundsLookback > 10000 {
		return spec, fmt.Errorf("rangeReversion bounds must be CHART or SOURCE with lookback 1..10000")
	}
	if r.Policy == RangeReversionDelayedPinePolicy && (r.Bounds != "CHART" || !r.RequireCandleColor) {
		return spec, fmt.Errorf("DELAYED_PINE_OHLC_V1 requires chart bounds and candle color")
	}
	for _, v := range []struct {
		name      string
		n         float64
		min       float64
		allowZero bool
	}{
		{"maxAdx", r.MaxADX, 0, true}, {"minChop", r.MinCHOP, 0, true}, {"rangeAtrMultiple", r.RangeATRMultiple, 0, false},
		{"stopAtrMultiple", r.StopATRMultiple, 0, false}, {"targetR", r.TargetR, 0, false},
		{"breakEvenTriggerR", r.BreakEvenTriggerR, 0, true}, {"tickSize", r.TickSize, 0, false},
	} {
		if math.IsNaN(v.n) || math.IsInf(v.n, 0) || (v.allowZero && v.n < v.min) || (!v.allowZero && v.n <= v.min) {
			return spec, fmt.Errorf("rangeReversion.%s must be finite and %s", v.name, map[bool]string{true: "nonnegative", false: "positive"}[v.allowZero])
		}
	}
	if r.HTFEMALength < 1 || r.VectorLength < 2 || r.ATRLength < 1 || r.CooldownBars < 0 || r.CooldownBars > 10000 {
		return spec, fmt.Errorf("rangeReversion lengths or cooldown are outside supported bounds")
	}
	if r.BreakEvenOffsetTicks < 0 || r.BreakEvenOffsetTicks > 100000 {
		return spec, fmt.Errorf("rangeReversion break-even offset ticks are outside supported bounds")
	}
	return spec, nil
}

func IsRangeReversionReserved(cfg Config) bool {
	var inspect func(map[string]any) bool
	inspect = func(values map[string]any) bool {
		for key, value := range values {
			if strings.Contains(strings.ToLower(key), "rangereversion") || strings.Contains(strings.ToLower(key), "range-reversion") {
				return true
			}
			if nested, ok := value.(map[string]any); ok && inspect(nested) {
				return true
			}
			identity := strings.Map(func(r rune) rune {
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					return unicode.ToLower(r)
				}
				return -1
			}, key)
			if identity != "setuptype" {
				continue
			}
			var family string
			switch v := value.(type) {
			case string:
				family = v
			case FamilyID:
				family = string(v)
			}
			family = strings.Map(func(r rune) rune {
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					return unicode.ToLower(r)
				}
				return -1
			}, family)
			if family == "rangereversion" {
				return true
			}
		}
		return false
	}
	return inspect(cfg)
}

func rangeReversionNumber(token string, integer bool) (float64, error) {
	if integer {
		if !frozenDSLInteger.MatchString(token) {
			return 0, fmt.Errorf("invalid integer %q", token)
		}
		n, err := strconv.ParseInt(token, 10, 64)
		return float64(n), err
	}
	if !frozenDSLDecimal.MatchString(token) {
		return 0, fmt.Errorf("invalid decimal %q", token)
	}
	n, err := strconv.ParseFloat(token, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid finite decimal %q", token)
	}
	return n, nil
}
