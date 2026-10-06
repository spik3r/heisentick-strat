package engine

import (
	"fmt"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
)

// Reserved objects cannot be hidden by an old-family discriminator, a case
// alias, null, an off-route result, or empty market columns. Compilation has no
// execution permission: even a fully valid root is refused here.
func rejectFrozenLevelExecution(cfg dsl.Config) error {
	reserved := false
	for key, value := range cfg {
		if strings.EqualFold(key, string(dsl.FamilyFrozenLevelBreakout)) {
			reserved = true
		}
		if strings.EqualFold(key, "setupType") && strings.EqualFold(setupTypeFromAny(value), string(dsl.FamilyFrozenLevelBreakout)) {
			reserved = true
		}
	}
	if !reserved {
		return nil
	}
	if _, err := dsl.NormalizeFrozenLevelConfig(cfg); err != nil {
		return fmt.Errorf("%s: invalid reserved config: %w", dsl.FrozenLevelExecutionUnimplemented, err)
	}
	return fmt.Errorf("%s", dsl.FrozenLevelExecutionUnimplemented)
}
