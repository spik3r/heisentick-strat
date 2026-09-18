package engine

// onBar dispatches a single completed bar to the per-setup-type bar handler.
// Moved out of broker.go as a pure relocation: no dispatch logic changed.
func (b *broker) onBar(i int) {
	b.applyPreHandlerTrail(i)
	switch b.params.SetupType {
	case "rangeBreakFake":
		b.onRangeBreakFakeBar(i)
	case "openingRangeBreakout":
		b.onOpeningRangeBreakoutBar(i)
	case "insideDayExpansion":
		b.onInsideDayExpansionBar(i)
	case "dayOpenReclaim":
		b.onDayOpenReclaimBar(i)
	case "breakRetest":
		b.onBreakRetestBar(i)
	case "sessionBreakHold":
		b.onSessionBreakHoldBar(i)
	case "trendPullback":
		b.onTrendPullbackBar(i)
	case "dualEmaResumption":
		b.onDualEMAResumptionBar(i)
	case "smaGoldenCross":
		b.onSMAGoldenCrossBar(i)
	case "supplyDemand":
		b.onSupplyDemandBar(i)
	case "doubleTopBottom":
		b.onDoubleTopBottomBar(i)
	case "fibContinuation":
		b.onFibContinuationBar(i)
	case "channelBreakHold":
		b.onChannelBreakHoldBar(i)
	case "failedBreakout":
		b.onLevelSweepBar(i)
	case "triplePushExhaustion":
		b.onTriplePushExhaustionBar(i)
	case "vwapExtensionFade":
		b.onVWAPExtensionFadeBar(i)
	case "volumeAnomalyExhaustion":
		b.onVolumeAnomalyExhaustionBar(i)
	case "elderTripleScreen":
		b.onElderTripleScreenBar(i)
	case "priceMomentum":
		b.onPriceMomentumBar(i)
	case "fairValueGap":
		b.onFairValueGapBar(i)
	case "keltnerReversion", "keltnerExpansion":
		b.onKeltnerBar(i, b.params.SetupType)
	default:
		b.onFlagBar(i)
	}
}
