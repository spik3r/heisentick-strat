package engine

import "github.com/spik3r/heisentick-strat/dsl"

type smaRegimeHoldParams struct {
	Period int
}

func smaRegimeHoldParamsFromConfig(cfg dsl.Config) smaRegimeHoldParams {
	values := mapValue(cfg, "smaRegimeHold")
	return smaRegimeHoldParams{Period: intValue(values, "period", 200)}
}
