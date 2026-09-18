package engine

import "github.com/spik3r/heisentick-strat/dsl"

func usesGenericTrail(setupType string) bool {
	switch setupType {
	case string(dsl.FamilyFlagContinuation),
		string(dsl.FamilyBreakRetest),
		string(dsl.FamilyOpeningRangeBreakout),
		string(dsl.FamilySessionBreakHold),
		string(dsl.FamilyChannelBreakHold),
		string(dsl.FamilyTriplePushExhaustion),
		string(dsl.FamilyFibContinuation),
		string(dsl.FamilyInsideDayExpansion),
		string(dsl.FamilyDayOpenReclaim),
		string(dsl.FamilySupplyDemand),
		string(dsl.FamilyDoubleTopBottom),
		string(dsl.FamilyTrendPullback),
		string(dsl.FamilyFailedBreakout),
		string(dsl.FamilyPriceMomentum),
		string(dsl.FamilyFairValueGap),
		string(dsl.FamilyKeltnerReversion),
		string(dsl.FamilyKeltnerExpansion):
		return true
	default:
		return false
	}
}
