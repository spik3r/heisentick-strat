package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
)

type flagParams struct {
	SetupType                  string
	TradeWindowUnrestricted    bool
	TradeWindowSegments        []string
	TradeWindowMinuteFrom      float64
	TradeWindowMinuteTo        float64
	TradeWindowMinuteRangeSet  bool
	NewYorkHours               []int
	BlockedNewYorkHours        []int
	EntryMode                  string
	EntryExpireCandles         int
	PoleBars                   int
	PoleATR                    float64
	FreshBreakBars             int
	MinFlagBars                int
	MaxFlagBars                int
	MaxFlagDepth               float64
	MinPoleNetFrac             float64
	FlagCloseFrac              float64
	LevelToleranceATR          float64
	TargetR                    float64
	TargetEdge                 string
	UseHTFBias                 bool
	UseAsiaWindow              bool
	UseMidWindow               bool
	UseLondonWindow            bool
	UseNYWindow                bool
	AdmitAsiaWindow            bool
	AdmitMidWindow             bool
	AdmitLondonWindow          bool
	AdmitNYWindow              bool
	AllowLong                  bool
	AllowShort                 bool
	FirstPullbackOnly          bool
	ImpulseCloseLocationMin    float64
	PullbackVolatilityContract bool
	AvoidLunchBreakouts        bool
	MinStopATR                 float64
	MaxStopATR                 float64
	StopBufferATR              float64
	StopType                   string
	StopFibRatio               float64
	StopLookbackCandles        int
	MinER                      float64
	MaxER                      float64
	BreakevenR                 float64
	BreakevenOffsetATR         float64
	CooldownBars               int
	MaxHoldBars                float64
	RiskUSD                    float64
	Partial                    partialParams
	Trail                      trailParams
	SupplyDemand               supplyDemandParams
	DoubleTopBottom            doubleTopBottomParams
	FibContinuation            fibContinuationParams
	ChannelBreakHold           channelBreakHoldParams
	TriplePush                 triplePushParams
	VWAPExtensionFade          vwapExtensionFadeParams
	VolumeAnomalyExhaustion    volumeAnomalyExhaustionParams
	ElderTripleScreen          elderTripleScreenParams
	PriceMomentum              priceMomentumParams
	DailyFlushFailure          dailyFlushFailureParams
	FairValueGap               fairValueGapParams
	WeekendExtremeFade         weekendExtremeFadeParams
	IntraHourRunExhaustion     intraHourRunExhaustionParams
	Keltner                    keltnerParams
	RangeActiveWithinCandles   int
	LevelSweepHighGrade        string
	LevelSweepLowGrade         string
	RBFPierceATR               float64
	RBFReclaimCandles          int
	RBFRequireCloseBackInside  bool
	RBFRetestFailedEdge        bool
	RBFRetestCandles           int
	RBFMaxEntryDistanceATR     float64
	RBFTargetMode              string
	RBFTargetR                 float64
	MinTargetR                 float64
	MaxEntryDistanceATR        float64
	TailRejectionMin           float64
	CloseLocationMin           float64
	ORBFirstCandles            int
	ORBFirstMinutes            int
	ORBUTCSlotMinutes          int
	ORBOpeningSessions         []string
	ORBOpeningSessionsOverride bool
	ORBBreakBufferATR          float64
	ORBHoldCandles             int
	ORBRequireRetest           bool
	ORBRetestCandles           int
	ORBRetestToleranceATR      float64
	ORBStopATRMult             float64
	ORBFixedStopPips           float64
	ORBTargetR                 float64
	ORBFixedTargetPips         float64
	ORBTargetRangeMultiple     float64
	IDEHoldCandles             int
	IDEInsideToleranceATR      float64
	IDEUseTrigger              bool
	IDETargetR                 float64
	DORStretchATR              float64
	DORReclaimCandles          int
	DORLevelPriority           []string
	DORLevelDistanceATR        float64
	DORUseTrigger              bool
	DORTargetR                 float64
	LevelPriority              []string
	FBRequireLevel             bool
	LevelDistanceATR           float64
	LevelPriorityExplicit      bool
	BRFreshBreakBars           int
	BRMinBreakATR              float64
	BRRetestBars               int
	BRLevelTolerance           float64
	BRDisplacementATR          float64
	BRStopType                 string
	BRStopFibRatio             float64
	BRFibExtension             float64
	BRTargetR                  float64
	BRMinER                    float64
	BRMaxER                    float64
	SBHHoldCandles             int
	SBHStopInsideATR           float64
	SBHMinStopATR              float64
	SBHMaxStopATR              float64
	SBHTargetR                 float64
	SBHAllowedLevels           []string
	TPBEMALen                  int
	TPBEMASlopeLen             int
	TPBPullbackATR             float64
	TPBPullbackWithin          int
	TPBMaxPullbackDepth        float64
	TPBImpulseLookbackBars     int
	TPBTargetR                 float64
	TPBMinER                   float64
	TPBMaxER                   float64
	TPBMinStopATR              float64
	TPBMaxStopATR              float64
	TPBStopBufferATR           float64
	TPBAttemptCount            int
	TPBUseTrigger              bool
	TPBVWAPTouch               string
	TPBVWAPFirst               bool
	TPBVWAPReclaim             bool
	DualEMA                    dualEMAResumptionParams
	SMAGoldenCross             smaGoldenCrossParams
	SessionPhases              []string
	PriorDayTypes              []string
	DayTypes                   []string
	DayThemes                  []string
	BlockedPriorDayTypes       []string
	DayTypeEREscape            float64
	MaxMovementER              float64
}

func paramsFromConfig(cfg dsl.Config) flagParams {
	flag := mapValue(cfg, "flag")
	stop := mapValue(cfg, "stop")
	target := mapValue(cfg, "target")
	breakeven := mapValue(cfg, "breakeven")
	sessions := mapValue(cfg, "sessions")
	partial := mapValue(cfg, "partial")
	trail := mapValue(cfg, "trail")
	htf := mapValue(cfg, "htf")
	rbf := mapValue(cfg, "rangeBreakFake")
	orb := mapValue(cfg, "openingRangeBreakout")
	inside := mapValue(cfg, "insideDay")
	dayOpen := mapValue(cfg, "dayOpen")
	br := mapValue(cfg, "breakRetest")
	sbh := mapValue(cfg, "sessionBreakHold")
	tpb := mapValue(cfg, "trendPullback")
	supplyDemand := mapValue(cfg, "supplyDemand")
	doubleTopBottom := mapValue(cfg, "doubleTopBottom")
	fibContinuation := mapValue(cfg, "fibContinuation")
	channelBreakHold := mapValue(cfg, "channelBreakHold")
	triplePush := mapValue(cfg, "triplePush")
	vwapExtensionFade := mapValue(cfg, "vwapExtensionFade")
	volumeAnomaly := mapValue(cfg, "volumeAnomalyExhaustion")
	elderTripleScreen := mapValue(cfg, "elderTripleScreen")
	priceMomentum := mapValue(cfg, "priceMomentum")
	dailyFlushFailure := mapValue(cfg, "dailyFlushFailure")
	keltner := mapValue(cfg, "keltnerReversion")
	fairValueGap := mapValue(cfg, "fairValueGap")
	trigger := mapValue(cfg, "trigger")
	tradeWindowMinuteRange := mapValue(cfg, "tradeWindowMinuteRange")
	entryMode := mapValue(cfg, "entryMode")
	rangeCfg := mapValue(cfg, "range")
	channel := mapValue(cfg, "channel")
	sweepRules := mapValue(cfg, "sweepRules")
	highSweepRule := mapValue(sweepRules, "high")
	lowSweepRule := mapValue(sweepRules, "low")
	setupType := setupTypeFromAny(cfg["setupType"])
	if setupType == "" {
		setupType = string(dsl.FamilyFlagContinuation)
	}
	_, stopLookbackExplicit := stop["extremeCandles"]
	preserveExplicitStopLookback := stopLookbackExplicit && setupType == string(dsl.FamilyRangeBreakFake)
	triggerExplicit := boolFromAny(cfg["triggerExplicit"], false)
	triggerCandles := stringSliceValue(cfg["triggerCandles"])
	supplyDemandPatterns, supplyDemandPatternsPresent := stringSliceMapValue(supplyDemand, "patterns")
	dorLevelPriority, dorLevelPriorityPresent := stringSliceMapValue(cfg, "levelPriority")
	levelPriority := stringSliceValue(cfg["levelPriority"])
	sbhAllowedLevels := filterStrings(levelPriority, map[string]bool{"AH": true, "AL": true, "LH": true, "LL": true})
	if len(sbhAllowedLevels) == 0 {
		sbhAllowedLevels = nil
	}
	vwapExtensionFadeSessionPhases := stringSliceValue(cfg["sessionPhases"])
	if setupType == string(dsl.FamilyVWAPExtensionFade) && len(vwapExtensionFadeSessionPhases) == 0 {
		vwapExtensionFadeSessionPhases = []string{"lunch", "close"}
	}
	channelDirectionRequired, channelDirectionText := false, ""
	if setupType == string(dsl.FamilyChannelBreakHold) {
		channelDirectionRequired, channelDirectionText = rawChannelDirectionFilter(channel["directions"])
	}
	p := flagParams{
		SetupType:                  setupType,
		TradeWindowUnrestricted:    stringValue(cfg, "tradeWindowMode", "") == "unrestricted",
		TradeWindowSegments:        stringSliceValue(cfg["tradeWindowSegments"]),
		TradeWindowMinuteFrom:      numberValue(tradeWindowMinuteRange, "from", 0),
		TradeWindowMinuteTo:        numberValue(tradeWindowMinuteRange, "to", 0),
		TradeWindowMinuteRangeSet:  len(tradeWindowMinuteRange) > 0,
		NewYorkHours:               intSliceValue(cfg, "newYorkHours"),
		BlockedNewYorkHours:        intSliceValue(cfg, "blockedNewYorkHours"),
		EntryMode:                  normalizedEntryMode(stringValue(entryMode, "type", "market")),
		EntryExpireCandles:         intValue(entryMode, "expireCandles", 5),
		PoleBars:                   intValue(flag, "poleBars", 12),
		PoleATR:                    numberValue(flag, "poleAtr", 1.5),
		FreshBreakBars:             intValue(flag, "freshBreakBars", 24),
		MinFlagBars:                intValue(flag, "minFlagBars", 3),
		MaxFlagBars:                intValue(flag, "maxFlagBars", 10),
		MaxFlagDepth:               numberValue(flag, "maxFlagDepth", 0.34),
		MinPoleNetFrac:             numberValue(flag, "minPoleNetFrac", 0.25),
		FlagCloseFrac:              numberValue(flag, "flagCloseFrac", 0.45),
		LevelToleranceATR:          numberValue(flag, "levelToleranceAtr", 0.5),
		TargetR:                    numberValue(target, "flagR", 0.8),
		TargetEdge:                 stringValue(target, "edge", "range"),
		UseHTFBias:                 stringValue(htf, "mode", "off") == "notAgainst",
		UseAsiaWindow:              boolValue(sessions, "asia", true),
		UseMidWindow:               true,
		UseLondonWindow:            boolValue(sessions, "london", true),
		UseNYWindow:                boolValue(sessions, "ny", false),
		AdmitAsiaWindow:            boolValue(sessions, "asia", true),
		AdmitMidWindow:             boolValue(sessions, "mid", false),
		AdmitLondonWindow:          boolValue(sessions, "london", true),
		AdmitNYWindow:              boolValue(sessions, "ny", false),
		AllowLong:                  numberFromAny(cfg["allowLong"], 1) != 0,
		AllowShort:                 numberFromAny(cfg["allowShort"], 1) != 0,
		FirstPullbackOnly:          boolValue(flag, "firstPullbackOnly", false),
		ImpulseCloseLocationMin:    numberValue(flag, "impulseCloseLocationMin", 0),
		PullbackVolatilityContract: boolValue(flag, "pullbackVolatilityContract", false),
		AvoidLunchBreakouts:        boolValue(flag, "avoidLunchBreakouts", false),
		MinStopATR:                 numberValue(stop, "minAtr", 0.5),
		MaxStopATR:                 numberValue(stop, "maxAtr", math.Inf(1)),
		StopBufferATR:              numberValue(stop, "paddingAtr", 0.1),
		StopType:                   stringValue(stop, "type", ""),
		StopFibRatio:               numberValue(stop, "fibRatio", 0.618),
		StopLookbackCandles:        intValue(stop, "extremeCandles", 0),
		MinER:                      numberValue(flag, "minEr", 0.5),
		MaxER:                      numberValue(flag, "maxEr", 0.75),
		BreakevenR:                 numberValue(breakeven, "atR", 0.5),
		BreakevenOffsetATR:         numberValue(breakeven, "offsetAtr", 0.05),
		CooldownBars:               intFromAny(cfg["cooldownCandles"], 12),
		MaxHoldBars:                numberFromAny(cfg["maxHoldCandles"], 0),
		RiskUSD:                    numberFromAny(cfg["riskUsd"], 200),
		DualEMA:                    dualEMAParams(cfg),
		SMAGoldenCross:             smaGoldenCrossParamsFromConfig(cfg),
		Partial: partialParams{
			Enabled:       boolValue(partial, "enabled", false),
			Fraction:      numberValue(partial, "fraction", 0),
			TriggerR:      numberValue(partial, "triggerR", 1),
			MoveBreakeven: boolValue(partial, "moveBreakeven", false),
		},
		Trail: trailParams{
			ATR:      numberValue(trail, "atr", 0),
			TriggerR: numberValue(trail, "triggerR", 1),
		},
		SupplyDemand: supplyDemandParams{
			Patterns:                supplyDemandPatterns,
			MinBaseCandles:          intValue(supplyDemand, "minBaseCandles", 2),
			MaxBaseCandles:          intValue(supplyDemand, "maxBaseCandles", 5),
			MaxBaseBodyATR:          numberValue(supplyDemand, "maxBaseBodyAtr", math.Inf(1)),
			MaxBaseBodyToRange:      numberValue(supplyDemand, "maxBaseBodyToRange", 1),
			MaxZoneATR:              numberValue(supplyDemand, "maxZoneAtr", 1.2),
			MinSourceATR:            numberValue(supplyDemand, "minSourceAtr", 0),
			SourceCandles:           intValue(supplyDemand, "sourceCandles", 4),
			ImpulseATR:              numberValue(supplyDemand, "impulseAtr", 1.5),
			ImpulseCandles:          intValue(supplyDemand, "impulseCandles", 4),
			MinDepartureBodyToRange: numberValue(supplyDemand, "minDepartureBodyToRange", 0),
			ZoneBounds:              stringValue(supplyDemand, "zoneBounds", "wick"),
			MaxZoneAgeCandles:       intValue(supplyDemand, "maxZoneAgeCandles", 72),
			RetestToleranceATR:      numberValue(supplyDemand, "retestToleranceAtr", 0.2),
			MaxRetests:              intValue(supplyDemand, "maxRetests", 1),
			RequireRejectionClose:   boolValue(supplyDemand, "requireRejectionClose", false),
			MaxReactionCandles:      intValue(supplyDemand, "maxReactionCandles", 0),
			ChochCandles:            intValue(supplyDemand, "chochCandles", 0),
			ChochLookbackCandles:    intValue(supplyDemand, "chochLookbackCandles", 3),
			InvalidationATR:         numberValue(supplyDemand, "invalidationAtr", 0.05),
			FlipBrokenZones:         boolValue(supplyDemand, "flipBrokenZones", false),
			MinWaitCandles:          intValue(supplyDemand, "minWaitCandles", 1),
			SkipOverlappingZones:    boolValue(supplyDemand, "skipOverlappingZones", false),
			UseTrigger:              triggerExplicit && !containsString(triggerCandles, "any"),
		},
		DoubleTopBottom: doubleTopBottomParams{
			PivotWindow:          intValue(doubleTopBottom, "pivotWindow", 2),
			MinPeakGap:           intValue(doubleTopBottom, "minPeakGap", 5),
			MaxPeakGap:           intValue(doubleTopBottom, "maxPeakGap", 72),
			MaxPeakDiffATR:       numberValue(doubleTopBottom, "maxPeakDiffAtr", 0.5),
			NecklineTouchATR:     numberValue(doubleTopBottom, "necklineTouchAtr", 0.25),
			NecklineDepthMinATR:  numberValue(doubleTopBottom, "necklineDepthMinAtr", 0),
			BreakoutBufferATR:    numberValue(doubleTopBottom, "breakoutBufferAtr", 0.05),
			ConfirmCandles:       intValue(doubleTopBottom, "confirmCandles", 12),
			ChochCandles:         intValue(doubleTopBottom, "chochCandles", 0),
			ChochLookbackCandles: intValue(doubleTopBottom, "chochLookbackCandles", 4),
		},
		FibContinuation: fibContinuationParams{
			ImpulseATR:           numberValue(fibContinuation, "impulseAtr", 2),
			ImpulseLookbackBars:  intValue(fibContinuation, "impulseLookbackBars", 24),
			RetraceMin:           numberValue(fibContinuation, "retraceMin", 0.5),
			RetraceMax:           numberValue(fibContinuation, "retraceMax", 0.618),
			ConfirmCloseLocation: numberValue(fibContinuation, "confirmCloseLocation", 0.6),
			FibExtension:         numberValue(target, "fibExt", 1.618),
			StopFibRatio:         numberValue(stop, "fibRatio", 0.786),
			StopPaddingATR:       numberValue(stop, "paddingAtr", 0.25),
			UseTrigger:           !triggerExplicit || !containsString(triggerCandles, "any"),
		},
		ChannelBreakHold: channelBreakHoldParams{
			HoldCandles:         intValue(channelBreakHold, "holdCandles", 3),
			ActiveWithinCandles: intValue(channel, "activeWithinCandles", 12),
			ChannelMinWidthATR:  numberValue(channel, "tradeMinWidthAtr", 0),
			ChannelMaxWidthATR:  numberValue(channel, "tradeMaxWidthAtr", 0),
			ChannelDirections:   stringSliceValue(channel["directions"]),
			DirectionRequired:   channelDirectionRequired,
			DirectionText:       channelDirectionText,
			StopInsideATR:       numberValue(stop, "paddingAtr", 0.5),
			UseTrigger:          !triggerExplicit || !containsString(triggerCandles, "any"),
		},
		TriplePush: triplePushParams{
			PivotK:               intValue(triplePush, "pivotK", 3),
			LookbackBars:         intValue(triplePush, "lookbackBars", 60),
			MinSeparation:        intValue(triplePush, "minSeparation", 5),
			PushDecay:            numberValue(triplePush, "pushDecay", 1),
			DecelMode:            stringValue(triplePush, "decelMode", "size"),
			TargetStructure:      boolValue(triplePush, "targetStructure", false),
			NearATR:              numberValue(triplePush, "nearAtr", 1.5),
			ConfirmCloseLocation: numberValue(triplePush, "confirmCloseLocation", 0.55),
			StopPaddingATR:       numberValue(stop, "paddingAtr", 0.25),
			MinStopATR:           numberValue(stop, "minAtr", 0.4),
			MaxStopATR:           numberValue(stop, "maxAtr", 3.5),
			TargetR:              numberValue(target, "tpeR", 2),
			MinER:                numberValue(triplePush, "minEr", 0),
			MaxER:                numberValue(triplePush, "maxEr", 1),
			UseHTFBias:           stringValue(htf, "mode", "off") == "notAgainst",
			UseTrigger:           !triggerExplicit || !containsString(triggerCandles, "any"),
		},
		VWAPExtensionFade: vwapExtensionFadeParams{
			MinDistanceATR:   numberValue(vwapExtensionFade, "minDistanceAtr", 1.2),
			SessionPhases:    vwapExtensionFadeSessionPhases,
			RequireRangeDay:  boolValue(vwapExtensionFade, "requireRangeDay", true),
			TailRejectionMin: numberValue(vwapExtensionFade, "tailRejectionMin", numberFromAny(cfg["tailRejectionMin"], 0.35)),
			CloseLocationMin: numberFromAny(cfg["closeLocationMin"], 0),
			StopLookback:     intValue(stop, "extremeCandles", 5),
			StopBufferATR:    numberValue(stop, "paddingAtr", 0.25),
			MinStopATR:       numberValue(stop, "minAtr", 0.4),
			MaxStopATR:       numberValue(stop, "maxAtr", 2),
			TargetMode:       stringValue(vwapExtensionFade, "targetMode", "vwap"),
			TargetR:          numberValue(target, "vefR", numberValue(target, "fallbackR", 1)),
			MinTargetR:       numberValue(target, "minR", 0.4),
		},
		VolumeAnomalyExhaustion: volumeAnomalyExhaustionParams{
			AnomalyLookbackCandles: intValue(volumeAnomaly, "anomalyLookbackCandles", 3),
			VolumeRatioMin:         numberValue(volumeAnomaly, "volumeRatioMin", 1.8),
			MaxSpreadATR:           numberValue(volumeAnomaly, "maxSpreadAtr", 0.65),
			RejectionWickMin:       numberValue(volumeAnomaly, "rejectionWickMin", 0.45),
			LevelPriority:          volumeLevelPriority(cfg, volumeAnomaly, levelPriority),
			LevelDistanceATR:       numberValue(volumeAnomaly, "levelDistanceAtr", numberFromAny(cfg["levelDistanceAtr"], 0.6)),
			StopLookback:           intValue(stop, "extremeCandles", 5),
			StopBufferATR:          numberValue(stop, "paddingAtr", 0.25),
			MinStopATR:             numberValue(stop, "minAtr", 0.4),
			MaxStopATR:             numberValue(stop, "maxAtr", 2),
			TargetMode:             stringValue(volumeAnomaly, "targetMode", "vwapElseFixedR"),
			TargetR:                numberValue(target, "vaeR", numberValue(target, "fallbackR", 1)),
			MinVWAPTargetR:         numberValue(target, "minR", 0.5),
			CloseLocationMin:       numberFromAny(cfg["closeLocationMin"], 0),
		},
		ElderTripleScreen: elderTripleScreenParamsFrom(elderTripleScreen, cfg, htf, stop, target, triggerExplicit, triggerCandles),
		PriceMomentum: priceMomentumParams{
			LookbackBars: intValue(priceMomentum, "lookbackBars", 0),
			ThresholdPct: numberValue(priceMomentum, "thresholdPct", 0),
		},
		DailyFlushFailure: dailyFlushFailureParams{
			ATRLength:             20,
			MinFlushRangeATR:      numberValue(dailyFlushFailure, "minFlushRangeAtr", 1.25),
			MaxFlushCloseLocation: numberValue(dailyFlushFailure, "maxFlushCloseLocation", 0.25),
		},
		FairValueGap: fairValueGapParams{
			MinGapATR:          numberValue(fairValueGap, "minGapAtr", 0.1),
			MinDisplacementATR: numberValue(fairValueGap, "minDisplacementAtr", 0.8),
			RetestCandles:      intValue(fairValueGap, "retestCandles", 8),
			EntryReference:     stringValue(fairValueGap, "entryReference", "midpoint"),
			EntryExpireCandles: intValue(fairValueGap, "entryExpireCandles", 1),
			SweepRequired:      numberValue(fairValueGap, "sweepRequired", 0) != 0,
			SweepLookback:      intValue(fairValueGap, "sweepLookbackCandles", 30),
			SweepPivotWidth:    intValue(fairValueGap, "sweepPivotWidth", 2),
		},
		Keltner:                   keltnerParamsFrom(keltner, target),
		RangeActiveWithinCandles:  intValue(rangeCfg, "activeWithinCandles", 8),
		LevelSweepHighGrade:       stringValue(highSweepRule, "grade", "A"),
		LevelSweepLowGrade:        stringValue(lowSweepRule, "grade", "A"),
		RBFPierceATR:              numberValue(rbf, "pierceAtr", 0.1),
		RBFReclaimCandles:         intValue(rbf, "reclaimCandles", 3),
		RBFRequireCloseBackInside: boolValue(rbf, "requireCloseBackInside", true),
		RBFRetestFailedEdge:       boolValue(rbf, "retestFailedEdge", false),
		RBFRetestCandles:          intValue(rbf, "retestCandles", 6),
		RBFMaxEntryDistanceATR:    numberValue(rbf, "maxEntryDistanceAtr", numberValue(trigger, "maxEntryDistanceAtr", 0.6)),
		RBFTargetMode:             stringValue(rbf, "targetMode", "rangeMid"),
		RBFTargetR:                numberValue(target, "rbfR", numberValue(target, "fallbackR", 1)),
		MinTargetR:                numberValue(target, "minR", 0.6),
		MaxEntryDistanceATR:       numberValue(trigger, "maxEntryDistanceAtr", 0),
		TailRejectionMin:          numberValue(rbf, "tailRejectionMin", numberFromAny(cfg["tailRejectionMin"], 0)),
		CloseLocationMin:          numberFromAny(cfg["closeLocationMin"], 0),
		ORBFirstCandles:           intValue(orb, "firstCandles", 4),
		ORBFirstMinutes:           intValue(orb, "firstMinutes", 0),
		ORBUTCSlotMinutes:         intValue(orb, "utcSlotMinutes", 0),
		ORBOpeningSessions:        stringSliceValue(orb["openingSessions"]),
		ORBBreakBufferATR:         numberValue(orb, "breakBufferAtr", 0),
		ORBHoldCandles:            intValue(orb, "holdCandles", 1),
		ORBRequireRetest:          boolValue(orb, "requireRetest", false),
		ORBRetestCandles:          intValue(orb, "retestCandles", 6),
		ORBRetestToleranceATR:     numberValue(orb, "retestToleranceAtr", 0.2),
		ORBStopATRMult:            numberValue(stop, "atrMult", numberValue(stop, "atr", 0)),
		ORBFixedStopPips:          numberValue(stop, "pips", 0),
		ORBTargetR:                numberValue(target, "orbR", 1),
		ORBFixedTargetPips:        numberValue(target, "pips", 0),
		ORBTargetRangeMultiple:    numberValue(target, "orbRange", 0),
		IDEHoldCandles:            intValue(inside, "holdCandles", 1),
		IDEInsideToleranceATR:     numberValue(inside, "insideToleranceAtr", 0.05),
		IDEUseTrigger:             triggerExplicit && !containsString(triggerCandles, "any"),
		IDETargetR:                numberValue(target, "ideR", 1),
		DORStretchATR:             numberValue(dayOpen, "stretchAtr", 1),
		DORReclaimCandles:         intValue(dayOpen, "reclaimCandles", 3),
		DORLevelPriority:          dorLevelPriority,
		DORLevelDistanceATR:       numberFromAny(cfg["levelDistanceAtr"], 1.5),
		DORUseTrigger:             triggerExplicit && !containsString(triggerCandles, "any"),
		DORTargetR:                numberValue(target, "dorR", 1),
		LevelPriority:             levelPriority,
		FBRequireLevel:            setupType == string(dsl.FamilyFailedBreakout) && rawLengthNonempty(cfg["levelPriority"]),
		LevelDistanceATR:          numberFromAny(cfg["levelDistanceAtr"], 1.5),
		LevelPriorityExplicit:     boolFromAny(cfg["levelPriorityExplicit"], false),
		BRFreshBreakBars:          intValue(br, "freshBreakBars", 12),
		BRMinBreakATR:             numberValue(br, "minBreakAtr", 1),
		BRRetestBars:              intValue(br, "retestBars", 6),
		BRLevelTolerance:          numberValue(br, "levelTolerance", 0.35),
		BRDisplacementATR:         numberValue(br, "displacementAtr", 0),
		BRStopType:                stringValue(stop, "type", ""),
		BRStopFibRatio:            numberValue(stop, "fibRatio", 0.618),
		BRFibExtension:            numberValue(target, "brFibExt", 0),
		BRTargetR:                 numberValue(target, "brR", 0.8),
		BRMinER:                   numberValue(br, "minEr", 0.4),
		BRMaxER:                   numberValue(br, "maxEr", 0.95),
		SBHHoldCandles:            intValue(sbh, "holdCandles", 3),
		SBHStopInsideATR:          numberValue(stop, "paddingAtr", 0.5),
		SBHMinStopATR:             numberValue(stop, "minAtr", 0.5),
		SBHMaxStopATR:             numberValue(stop, "maxAtr", 3),
		SBHTargetR:                numberValue(target, "sbhR", 1.2),
		SBHAllowedLevels:          sbhAllowedLevels,
		TPBEMALen:                 intValue(tpb, "emaLen", intFromAny(cfg["emaLen"], 21)),
		TPBEMASlopeLen:            intValue(tpb, "emaSlopeLen", 5),
		TPBPullbackATR:            numberValue(tpb, "pullbackAtr", 0.25),
		TPBPullbackWithin:         intValue(tpb, "pullbackWithin", 6),
		TPBMaxPullbackDepth:       numberValue(tpb, "maxPullbackDepth", 0.5),
		TPBImpulseLookbackBars:    intValue(tpb, "impulseLookbackBars", 12),
		TPBTargetR:                numberValue(target, "tpbR", 2),
		TPBMinER:                  numberValue(tpb, "minEr", 0.35),
		TPBMaxER:                  numberValue(tpb, "maxEr", 0.95),
		TPBMinStopATR:             numberValue(stop, "minAtr", 0.5),
		TPBMaxStopATR:             numberValue(stop, "maxAtr", math.Inf(1)),
		TPBStopBufferATR:          numberValue(stop, "paddingAtr", 0.25),
		TPBAttemptCount:           intValue(tpb, "attemptCount", 1),
		TPBUseTrigger:             !triggerExplicit || !containsString(triggerCandles, "any"),
		TPBVWAPTouch:              stringValue(tpb, "sessionVwapTouch", ""),
		TPBVWAPFirst:              boolValue(tpb, "sessionVwapFirstTouchOnly", false),
		TPBVWAPReclaim:            boolValue(tpb, "sessionVwapTrendSideReclaim", false),
		SessionPhases:             stringSliceValue(cfg["sessionPhases"]),
		PriorDayTypes:             stringSliceValue(cfg["priorDayTypes"]),
		DayTypes:                  stringSliceValue(cfg["dayTypes"]),
		DayThemes:                 stringSliceValue(cfg["dayThemes"]),
		BlockedPriorDayTypes:      stringSliceValue(cfg["blockedPriorDayTypes"]),
		DayTypeEREscape:           numberFromAny(cfg["dayTypeErEscape"], 0.55),
		MaxMovementER:             numberFromAny(cfg["maxMovementEr"], 0.8),
	}
	applyExtractedSetupParams(&p, cfg, stop, orb, setupType)
	if p.StopLookbackCandles == 0 && !preserveExplicitStopLookback {
		p.StopLookbackCandles = p.familyDefaultStopLookback()
	}
	if setupType != string(dsl.FamilyFlagContinuation) {
		p.UseMidWindow = boolValue(sessions, "mid", false)
	}
	if !dorLevelPriorityPresent {
		p.DORLevelPriority = []string{"PDH", "PDL", "WH", "WL"}
	}
	if !supplyDemandPatternsPresent {
		p.SupplyDemand.Patterns = []string{"DBR", "RBR", "RBD", "DBD"}
	}
	switch setupType {
	case string(dsl.FamilyFailedBreakout):
		p.TargetR = numberValue(target, "fallbackR", 1)
	case string(dsl.FamilySupplyDemand):
		p.TargetR = numberValue(target, "sdR", 1)
	case string(dsl.FamilyDoubleTopBottom):
		p.TargetR = numberValue(target, "dtbR", 1)
	case string(dsl.FamilyFibContinuation):
		p.TargetR = numberValue(target, "fibR", 2)
	case string(dsl.FamilyChannelBreakHold):
		p.TargetR = numberValue(target, "cbhR", 1)
	case string(dsl.FamilyTriplePushExhaustion):
		p.TargetR = p.TriplePush.TargetR
	case string(dsl.FamilyVWAPExtensionFade):
		p.TargetR = p.VWAPExtensionFade.TargetR
	case string(dsl.FamilyVolumeAnomalyExhaustion):
		p.TargetR = p.VolumeAnomalyExhaustion.TargetR
	case string(dsl.FamilyElderTripleScreen):
		p.TargetR = p.ElderTripleScreen.TargetR
	case string(dsl.FamilyPriceMomentum):
		p.TargetR = numberValue(target, "fallbackR", 1)
	case string(dsl.FamilyFairValueGap):
		p.TargetR = numberValue(target, "fvgR", 1)
	case string(dsl.FamilyKeltnerReversion), string(dsl.FamilyKeltnerExpansion):
		p.TargetR = numberValue(keltner, "targetR", numberValue(target, "krR", 1))
	}
	return p
}
