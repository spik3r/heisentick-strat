package engine

import "math"

type partialParams struct {
	Enabled       bool
	Fraction      float64
	TriggerR      float64
	MoveBreakeven bool
}

type trailParams struct {
	ATR      float64
	TriggerR float64
}

func numberValue(m map[string]any, key string, fallback float64) float64 {
	if m == nil {
		return fallback
	}
	return numberFromAny(m[key], fallback)
}

func numberFromAny(value any, fallback float64) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float32:
		converted := float64(v)
		if math.IsNaN(converted) {
			return fallback
		}
		return converted
	case float64:
		if math.IsNaN(v) {
			return fallback
		}
		return v
	}
	return fallback
}

func intValue(m map[string]any, key string, fallback int) int {
	return int(math.Round(numberValue(m, key, float64(fallback))))
}

func intFromAny(value any, fallback int) int {
	return int(math.Round(numberFromAny(value, float64(fallback))))
}
