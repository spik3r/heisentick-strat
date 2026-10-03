package engine

import (
	"fmt"
	"sort"

	"github.com/spik3r/heisentick-strat/dsl"
)

// The shared parser recognizes more named levels than the Go keyLevel
// resolver. Reject unsupported entry rules instead of silently producing no
// setup. The list mirrors the resolver in family_level_sweep.go.
func validateNamedLevelSweepSupport(cfg dsl.Config) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyNamedLevelSweep) {
		return nil
	}
	nls := mapValue(cfg, "namedLevelSweep")
	if _, present := cfg["namedLevelSweep"]; present && nls == nil {
		return fmt.Errorf("named level sweep config must be an object")
	}
	rules := mapValue(nls, "rules")
	if _, present := nls["rules"]; present && rules == nil {
		return fmt.Errorf("named level sweep rules must be an object")
	}
	keys := make([]string, 0, len(rules))
	for key := range rules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if mapValue(rules, key) == nil {
			return fmt.Errorf("named level sweep rule %q must be an object", key)
		}
		if !goNamedLevelSweepKeySupported(key) {
			return fmt.Errorf("named level sweep level %q is not implemented by the Go engine", key)
		}
	}
	return nil
}

func goNamedLevelSweepKeySupported(key string) bool {
	switch key {
	case "VWAP", "CAM_R3", "CAM_R4", "CAM_S3", "CAM_S4",
		"PDH", "PDL", "PDO", "PDC", "DO", "DH", "DL",
		"WH", "WL", "AH", "AL", "LH", "LL", "NH", "NL":
		return true
	}
	return false
}
