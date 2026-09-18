package dsl

import (
	"math"
	"strconv"
	"strings"
)

func valuesAfterIn(tokens []string) []string {
	if idx := indexOfLower(tokens, "in"); idx >= 0 {
		return tokens[idx+1:]
	}
	return tokens
}

func lowerList(tokens []string) []string {
	values := []string{}
	for _, token := range tokens {
		if token != "" {
			values = append(values, strings.ToLower(token))
		}
	}
	return values
}

func lowerListUntil(tokens []string, stops ...string) []any {
	stopSet := map[string]bool{}
	for _, stop := range stops {
		stopSet[strings.ToLower(stop)] = true
	}
	values := []any{}
	for _, token := range tokens {
		lower := strings.ToLower(token)
		if stopSet[lower] {
			break
		}
		values = append(values, lower)
	}
	return values
}

func sessionMapFromNames(names []string) map[string]any {
	sessions := map[string]any{"asia": 0, "mid": 0, "london": 0, "ny": 0}
	for _, name := range names {
		key := strings.ToLower(name)
		if _, ok := sessions[key]; ok {
			sessions[key] = 1
		}
	}
	return sessions
}

func normalizeSessionsValue(value any) map[string]any {
	return sessionMapFromNames(sessionNamesFromValue(value))
}

func sessionNamesFromValue(value any) []string {
	switch sessions := value.(type) {
	case map[string]any:
		names := []string{}
		for _, key := range []string{"asia", "mid", "london", "ny"} {
			if sessionValueOn(sessions[key]) {
				names = append(names, key)
			}
		}
		return names
	case map[string]int:
		names := []string{}
		for _, key := range []string{"asia", "mid", "london", "ny"} {
			if sessions[key] != 0 {
				names = append(names, key)
			}
		}
		return names
	}
	return nil
}

func sessionValueOn(value any) bool {
	switch typed := value.(type) {
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case bool:
		return typed
	default:
		return false
	}
}

func upperList(tokens []string) []any {
	values := []any{}
	for _, token := range tokens {
		if token != "" {
			values = append(values, strings.ToUpper(token))
		}
	}
	return values
}

func stringsToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func firstNumber(tokens []string, start int, fallback float64) float64 {
	for i := start; i < len(tokens); i++ {
		if isNumberToken(tokens[i]) {
			return parseNumber(tokens[i], fallback)
		}
	}
	return fallback
}

func parseNumber(token string, fallback float64) float64 {
	cleaned := strings.TrimSuffix(strings.TrimSuffix(token, "R"), "r")
	cleaned = strings.TrimSuffix(cleaned, "%")
	value, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || math.IsNaN(value) {
		return fallback
	}
	return value
}

func parseFraction(token string, fallback float64) float64 {
	switch strings.ToLower(token) {
	case "half":
		return 0.5
	case "quarter":
		return 0.25
	}
	value := parseNumber(token, fallback)
	if strings.HasSuffix(token, "%") {
		return value / 100
	}
	return value
}

func isNumberToken(token string) bool {
	_, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(token, "R"), "r"), "%"), 64)
	return err == nil
}

func indexOfLower(tokens []string, needle string) int {
	for i, token := range tokens {
		if strings.EqualFold(token, needle) {
			return i
		}
	}
	return -1
}

func indexOf(tokens []string, needle string) int {
	for i, token := range tokens {
		if token == needle {
			return i
		}
	}
	return -1
}

func containsLower(tokens []string, needle string) bool {
	return indexOfLower(tokens, needle) >= 0
}

func tokenAt(tokens []string, index int) string {
	if index < 0 || index >= len(tokens) {
		return ""
	}
	return tokens[index]
}

func isTimeframe(token string) bool {
	switch strings.ToLower(token) {
	case "1m", "5m", "15m", "30m", "1h", "4h", "1d":
		return true
	default:
		return false
	}
}
