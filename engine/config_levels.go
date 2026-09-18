package engine

import (
	"fmt"

	"github.com/spik3r/heisentick-strat/dsl"
)

var sharedAdmissionListFields = [...]string{
	"sessionPhases",
	"priorDayTypes",
	"dayTypes",
	"dayThemes",
	"blockedPriorDayTypes",
}

func validateSharedAdmissionLists(cfg dsl.Config) error {
	for _, field := range sharedAdmissionListFields {
		value, present := cfg[field]
		if present && !isStringList(value) {
			return fmt.Errorf("engine config %s must be a string list", field)
		}
	}
	return nil
}

func validateSessionBreakHoldLevelPriority(cfg dsl.Config) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilySessionBreakHold) {
		return nil
	}
	value, present := cfg["levelPriority"]
	if present && !isLevelPriorityList(value) {
		return fmt.Errorf("engine config levelPriority must be a list")
	}
	return nil
}

func isLevelPriorityList(value any) bool {
	switch value.(type) {
	case []string, []any:
		return true
	default:
		return false
	}
}

func isStringList(value any) bool {
	switch values := value.(type) {
	case []string:
		return true
	case []any:
		for _, value := range values {
			if _, ok := value.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func intSliceValue(m map[string]any, key string) []int {
	switch values := m[key].(type) {
	case []int:
		out := make([]int, len(values))
		copy(out, values)
		return out
	case []any:
		out := make([]int, 0, len(values))
		for _, value := range values {
			if n, ok := value.(int); ok {
				out = append(out, n)
			} else if n, ok := value.(float64); ok {
				out = append(out, int(n))
			}
		}
		return out
	}
	return nil
}

func stringSliceValue(value any) []string {
	switch values := value.(type) {
	case []string:
		out := make([]string, len(values))
		copy(out, values)
		return out
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func stringSliceMapValue(values map[string]any, key string) ([]string, bool) {
	value, present := values[key]
	if !present {
		return nil, false
	}
	return stringSliceValue(value), true
}

func stringSliceOr(primary []string, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	if len(fallback) > 0 {
		return fallback
	}
	return nil
}

func volumeLevelPriority(cfg dsl.Config, volumeAnomaly map[string]any, cfgPriority []string) []string {
	if boolFromAny(cfg["levelPriorityExplicit"], false) {
		return cfgPriority
	}
	if priority := stringSliceValue(volumeAnomaly["levelPriority"]); len(priority) > 0 {
		return priority
	}
	return []string{"CAM_R4", "CAM_R3", "CAM_S3", "CAM_S4", "PDH", "PDL", "VWAP"}
}
