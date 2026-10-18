package engine

import "github.com/spik3r/heisentick-strat/dsl"

func smaRegimeHoldPeriodFromConfig(cfg dsl.Config) int {
	values := mapValue(cfg, "smaRegimeHold")
	return intValue(values, "period", 200)
}
