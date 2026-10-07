package engine

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/dsl"
)

func rejectDedicatedAdaptiveFlagExecution(cfg dsl.Config) error {
	if dsl.IsAdaptiveVolumeFlagReserved(cfg) {
		return fmt.Errorf("%s", dsl.AdaptiveFlagDedicatedRunnerRequired)
	}
	return nil
}
