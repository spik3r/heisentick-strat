package engine

import "github.com/spik3r/heisentick-strat/dsl"

type smaGoldenCrossParams struct {
	FastLen   int
	SlowLen   int
	AllowLong bool
	ATRLen    int
	StopATR   float64
	TargetATR float64
}

func (p smaGoldenCrossParams) protected() bool {
	return p.ATRLen > 0 && p.StopATR > 0 && p.TargetATR > 0
}

func smaGoldenCrossParamsFromConfig(cfg dsl.Config) smaGoldenCrossParams {
	values := mapValue(cfg, "smaGoldenCross")
	return smaGoldenCrossParams{
		FastLen:   intValue(values, "fastSmaLen", 50),
		SlowLen:   intValue(values, "slowSmaLen", 200),
		AllowLong: numberFromAny(cfg["allowLong"], 1) != 0,
		ATRLen:    intValue(values, "atrLen", 14),
		StopATR:   numberValue(values, "stopAtr", 0),
		TargetATR: numberValue(values, "targetAtr", 0),
	}
}
