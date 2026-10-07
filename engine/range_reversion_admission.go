package engine

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/dsl"
)

func rejectDedicatedRangeReversionExecution(cfg dsl.Config) error {
	if dsl.IsRangeReversionReserved(cfg) {
		return fmt.Errorf("%s", dsl.RangeReversionDedicatedRunnerRequired)
	}
	return nil
}
