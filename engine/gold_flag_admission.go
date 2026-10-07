package engine

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/dsl"
)

func rejectDedicatedGoldFlagExecution(cfg dsl.Config) error {
	if dsl.IsGoldFlagReferenceReserved(cfg) {
		return fmt.Errorf("%s", dsl.GoldFlagReferenceDedicatedRunnerRequired)
	}
	return nil
}
