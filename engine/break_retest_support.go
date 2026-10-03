package engine

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
)

// The parser shares the v7 break-retest vocabulary with the browser runtime,
// but the Go executor currently implements only key-level break/retest without
// EMA confluence. Reject active unsupported settings before any execution path
// can silently run the key-level fallback under a different strategy meaning.
func validateBreakRetestSupport(cfg dsl.Config) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyBreakRetest) {
		return nil
	}
	br := mapValue(cfg, "breakRetest")
	if raw, present := cfg["breakRetest"]; present && raw != nil && br == nil {
		return fmt.Errorf("break retest config must be an object")
	}
	if raw, present := br["levelSource"]; present {
		source, ok := raw.(string)
		if !ok {
			return fmt.Errorf("break retest level source must be a string")
		}
		if source != "key" {
			return fmt.Errorf("break retest level source %q is not implemented by the Go engine", source)
		}
	}
	if raw, present := br["emaConfluenceAtr"]; present {
		value := numberFromAny(raw, math.NaN())
		if !isFinite(value) || value < 0 {
			return fmt.Errorf("break retest EMA confluence must be a nonnegative number")
		}
		if value > 0 {
			return fmt.Errorf("break retest EMA confluence is not implemented by the Go engine")
		}
	}
	return nil
}
