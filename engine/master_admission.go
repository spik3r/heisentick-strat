package engine

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/dsl"
)

func rejectDedicatedMasterExecution(cfg dsl.Config) error {
	if dsl.IsMasterStructuralReserved(cfg) {
		return fmt.Errorf("%s", dsl.MasterStructuralDedicatedRunnerRequired)
	}
	return nil
}
