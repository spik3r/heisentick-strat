package dsl

func setupTargetKey(raw any) string {
	switch raw {
	case string(FamilyFlagContinuation):
		return "flagR"
	case string(FamilySessionBreakHold):
		return "sbhR"
	case string(FamilyTrendPullback):
		return "tpbR"
	case string(FamilyBreakRetest):
		return "brR"
	case string(FamilyChannelBreakHold):
		return "cbhR"
	case string(FamilyDayOpenReclaim):
		return "dorR"
	case string(FamilyDoubleTopBottom):
		return "dtbR"
	case string(FamilyInsideDayExpansion):
		return "ideR"
	case string(FamilyOpeningRangeBreakout):
		return "orbR"
	case string(FamilyRangeBreakFake):
		return "rbfR"
	case string(FamilySupplyDemand):
		return "sdR"
	case string(FamilyFibContinuation):
		return "fibR"
	case string(FamilyTriplePushExhaustion):
		return "tpeR"
	case string(FamilyVWAPExtensionFade):
		return "vefR"
	case string(FamilyVolumeAnomalyExhaustion):
		return "vaeR"
	case string(FamilyPriceMomentum):
		return "fallbackR"
	case string(FamilyFairValueGap):
		return "fvgR"
	case string(FamilyIntraHourRunExhaustion):
		return "ihreR"
	default:
		return ""
	}
}
