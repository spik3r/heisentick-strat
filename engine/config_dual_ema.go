package engine

import "github.com/spik3r/heisentick-strat/dsl"

type dualEMAResumptionParams struct {
	FastLen, SlowLen, RiseBars, ATRLen int
	StopATR, TrailATR                  float64
}

func dualEMAParams(cfg dsl.Config) dualEMAResumptionParams {
	values := mapValue(cfg, "dualEmaResumption")
	return dualEMAResumptionParams{
		FastLen: intValue(values, "fastEmaLen", 20), SlowLen: intValue(values, "slowEmaLen", 80),
		RiseBars: intValue(values, "slowRiseBars", 12), ATRLen: intValue(values, "atrLen", 20),
		StopATR: numberValue(values, "stopAtr", 2.5), TrailATR: numberValue(values, "trailAtr", 3),
	}
}
