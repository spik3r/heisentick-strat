package engine

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/dsl"
)

// Reserve both discriminator and object before any off-route, empty-data or
// legacy-family dispatcher can discard the dedicated offline configuration.
func rejectDedicatedRegimeExecution(cfg dsl.Config) error {
	if dsl.IsRegimeEngineReserved(cfg) {
		return fmt.Errorf("%s", dsl.RegimeEngineDedicatedRunnerRequired)
	}
	return nil
}
