package engine

import (
	"errors"
	"math"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
)

type rmvFilter struct {
	Op    string
	Value float64
}

func rmvFilterFromConfig(cfg dsl.Config) rmvFilter {
	rmv := mapValue(cfg, "relativeMeasuredVolatility")
	return rmvFilter{Op: stringValue(rmv, "op", ""), Value: numberValue(rmv, "value", 0)}
}

func validateRMVConfig(cfg dsl.Config) error {
	raw := cfg["relativeMeasuredVolatility"]
	if raw == nil {
		return nil
	}
	switch raw.(type) {
	case map[string]any, dsl.Config:
	default:
		return errors.New("engine config relativeMeasuredVolatility must be an object")
	}
	rmv := mapValue(cfg, "relativeMeasuredVolatility")
	for key, fallback := range map[string]float64{"atrPeriod": 14, "lookback": 100} {
		value := fallback
		if rawValue, present := rmv[key]; present {
			value = numberFromAny(rawValue, math.NaN())
		}
		if !isFinite(value) || value < 1 || value != math.Trunc(value) || value >= float64(math.MaxInt) {
			return errors.New("engine RMV periods must be positive integers")
		}
	}
	if _, hasValue := rmv["value"]; hasValue && rmv["op"] == nil {
		return errors.New("engine RMV value requires a comparison")
	}
	if rawOp, exists := rmv["op"]; exists {
		op, ok := rawOp.(string)
		if !ok || op != "above" && op != "below" && op != "atLeast" && op != "atMost" {
			return errors.New("engine RMV comparison is invalid")
		}
		value := numberValue(rmv, "value", math.NaN())
		if !isFinite(value) || value < 0 || value > 100 {
			return errors.New("engine RMV threshold must be between 0 and 100")
		}
	}
	entryTf := strings.ToLower(stringValue(cfg, "entryTf", "current"))
	if sourceTimeframeFromConfig(cfg) != "" && (entryTf == "" || entryTf == "current") {
		return errors.New("RMV with a source timeframe requires a supported source-entry route")
	}
	if setupTypeFromAny(cfg["setupType"]) == string(dsl.FamilyDownShockRebound) {
		return errors.New("RMV is not supported by the down-shock source-entry runner")
	}
	return nil
}

func validateRMVSourceEntryRoute(symbol, timeframe string, cfg dsl.Config) error {
	if cfg["relativeMeasuredVolatility"] == nil || sourceTimeframeFromConfig(cfg) == "" {
		return nil
	}
	entryTf := strings.ToLower(stringValue(cfg, "entryTf", "current"))
	if symbol != "XAUUSD" || timeframe != entryTf || !supportedSourceEntryRoute(symbol, sourceTimeframeFromConfig(cfg), entryTf) {
		return errors.New("RMV with a source timeframe requires a supported source-entry route")
	}
	return nil
}

func (b *broker) rmvGateOK(i int) bool {
	f := b.params.RMV
	if f.Op == "" {
		return true
	}
	if i < 0 || i >= len(b.cols.RMV) || !isFinite(b.cols.RMV[i]) {
		return false
	}
	value := b.cols.RMV[i]
	switch f.Op {
	case "above":
		return value > f.Value
	case "below":
		return value < f.Value
	case "atLeast":
		return value >= f.Value
	case "atMost":
		return value <= f.Value
	default:
		return false
	}
}
