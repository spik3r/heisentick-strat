package engine

import "github.com/spik3r/heisentick-strat/dsl"

func mapValue(m map[string]any, key string) map[string]any {
	switch child := m[key].(type) {
	case map[string]any:
		return child
	case dsl.Config:
		return map[string]any(child)
	default:
		return nil
	}
}

func boolValue(m map[string]any, key string, fallback bool) bool {
	if m == nil {
		return fallback
	}
	return boolFromAny(m[key], fallback)
}

func boolFromAny(value any, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	}
	return fallback
}

func stringValue(m map[string]any, key string, fallback string) string {
	if m == nil {
		return fallback
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return fallback
}

func filterStrings(values []string, allowed map[string]bool) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if allowed[value] {
			out = append(out, value)
		}
	}
	return out
}

func intContains(values []int, needle int) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
