package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
)

func (b *broker) slippageAt(price float64) float64 {
	return b.costs.Slippage + math.Abs(price)*b.costs.SlippageBps/10_000
}

func (b *broker) runSpecialSetup() ([]Trade, bool) {
	switch b.params.SetupType {
	case string(dsl.FamilyDailyFlushFailure):
		return b.runDailyFlushFailure(), true
	case string(dsl.FamilyWeekendExtremeFade):
		return b.runWeekendExtremeFade(), true
	case string(dsl.FamilyIntraHourRunExhaustion):
		return b.runIntraHourRunExhaustion(), true
	default:
		return nil, false
	}
}
