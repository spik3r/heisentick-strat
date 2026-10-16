package engine

import (
	"errors"
	"math"
	"reflect"

	"github.com/spik3r/heisentick-strat/dsl"
)

type supplyDemandParams struct {
	Patterns                []string
	MinBaseCandles          int
	MaxBaseCandles          int
	MaxBaseBodyATR          float64
	MaxBaseBodyToRange      float64
	MaxZoneATR              float64
	MinSourceATR            float64
	SourceCandles           int
	ImpulseATR              float64
	ImpulseCandles          int
	MinDepartureBodyToRange float64
	ZoneBounds              string
	MaxZoneAgeCandles       int
	RetestToleranceATR      float64
	MaxRetests              int
	RequireRejectionClose   bool
	MaxReactionCandles      int
	ChochCandles            int
	ChochLookbackCandles    int
	InvalidationATR         float64
	FlipBrokenZones         bool
	MinWaitCandles          int
	SkipOverlappingZones    bool
	RetestOnEntryTimeframe  bool
	UseTrigger              bool
}

type doubleTopBottomParams struct {
	PivotWindow          int
	MinPeakGap           int
	MaxPeakGap           int
	MaxPeakDiffATR       float64
	NecklineTouchATR     float64
	NecklineDepthMinATR  float64
	BreakoutBufferATR    float64
	ConfirmCandles       int
	ChochCandles         int
	ChochLookbackCandles int
}

type fibContinuationParams struct {
	ImpulseATR           float64
	ImpulseLookbackBars  int
	RetraceMin           float64
	RetraceMax           float64
	ConfirmCloseLocation float64
	FibExtension         float64
	StopFibRatio         float64
	StopPaddingATR       float64
	UseTrigger           bool
}

type channelBreakHoldParams struct {
	HoldCandles         int
	ActiveWithinCandles int
	ChannelMinWidthATR  float64
	ChannelMaxWidthATR  float64
	ChannelDirections   []string
	DirectionRequired   bool
	DirectionText       string
	StopInsideATR       float64
	UseTrigger          bool
}

func rawChannelDirectionFilter(value any) (required bool, text string) {
	raw := reflect.ValueOf(value)
	if !raw.IsValid() {
		return false, ""
	}
	switch raw.Kind() {
	case reflect.String:
		return raw.Len() > 0, raw.String()
	case reflect.Slice, reflect.Array:
		return raw.Len() > 0, ""
	default:
		return false, ""
	}
}

type triplePushParams struct {
	PivotK               int
	LookbackBars         int
	MinSeparation        int
	PushDecay            float64
	DecelMode            string
	TargetStructure      bool
	NearATR              float64
	ConfirmCloseLocation float64
	StopPaddingATR       float64
	MinStopATR           float64
	MaxStopATR           float64
	TargetR              float64
	MinER                float64
	MaxER                float64
	UseHTFBias           bool
	UseTrigger           bool
}

type vwapExtensionFadeParams struct {
	MinDistanceATR   float64
	SessionPhases    []string
	RequireRangeDay  bool
	TailRejectionMin float64
	CloseLocationMin float64
	StopLookback     int
	StopBufferATR    float64
	MinStopATR       float64
	MaxStopATR       float64
	TargetMode       string
	TargetR          float64
	MinTargetR       float64
}

type volumeAnomalyExhaustionParams struct {
	AnomalyLookbackCandles int
	VolumeRatioMin         float64
	MaxSpreadATR           float64
	RejectionWickMin       float64
	LevelPriority          []string
	LevelDistanceATR       float64
	StopLookback           int
	StopBufferATR          float64
	MinStopATR             float64
	MaxStopATR             float64
	TargetMode             string
	TargetR                float64
	MinVWAPTargetR         float64
	CloseLocationMin       float64
}

type elderTripleScreenParams struct {
	EMALen              int
	EMASlopeLen         int
	PullbackATR         float64
	PullbackWithin      int
	MaxPullbackDepth    float64
	ImpulseLookbackBars int
	TargetR             float64
	MinStopATR          float64
	StopBufferATR       float64
	TrailATR            float64
	TrailTriggerR       float64
	UseHTFBias          bool
	UseTrigger          bool
}

type priceMomentumParams struct {
	LookbackBars int
	ThresholdPct float64
	// StopATR > 0 places the stop a fixed ATR multiple from entry
	// (`stop N ATR`) instead of beyond the recent candle extreme.
	StopATR float64
}

type keltnerParams struct {
	EMALen         int
	BandATR        float64
	MinPierceATR   float64
	ReclaimCandles int
	TargetMode     string
	MinR           float64
}

type dailyFlushFailureParams struct {
	ATRLength             int
	MinFlushRangeATR      float64
	MaxFlushCloseLocation float64
}

type fairValueGapParams struct {
	MinGapATR          float64
	MinDisplacementATR float64
	RetestCandles      int
	EntryReference     string
	EntryExpireCandles int
	SweepRequired      bool
	SweepLookback      int
	SweepPivotWidth    int
}

type weekendExtremeFadeParams struct {
	ATRLength          int
	MaxWeekendRangeATR float64
	CloseExtremePct    float64
	StopATR            float64
	TargetATR          float64
}

func weekendExtremeFadeParamsFromConfig(cfg dsl.Config, stop dsl.Config) weekendExtremeFadeParams {
	weekend := mapValue(cfg, "weekendExtremeFade")
	return weekendExtremeFadeParams{
		ATRLength:          intValue(weekend, "atrLen", 20),
		MaxWeekendRangeATR: numberValue(weekend, "maxWeekendRangeAtr", 5),
		CloseExtremePct:    numberValue(weekend, "closeExtremePct", 0.3),
		StopATR:            numberValue(weekend, "stopAtr", numberValue(stop, "atrMult", 0.75)),
		TargetATR:          numberValue(weekend, "targetAtr", 2),
	}
}

type intraHourRunExhaustionParams struct {
	ATRLength          int
	HourCandles        int
	RunCandles         int
	MinRunATR          float64
	ExhaustLocationPct float64
	RequirePoke        bool
	StopPadATR         float64
	TargetR            float64
}

func intraHourRunExhaustionParamsFromConfig(cfg dsl.Config, stop dsl.Config, target dsl.Config) intraHourRunExhaustionParams {
	family := mapValue(cfg, "intraHourRunExhaustion")
	return intraHourRunExhaustionParams{
		ATRLength:          intValue(family, "atrLen", 14),
		HourCandles:        intValue(family, "hourCandles", 4),
		RunCandles:         intValue(family, "runCandles", 3),
		MinRunATR:          numberValue(family, "minRunAtr", 0.8),
		ExhaustLocationPct: numberValue(family, "exhaustLocationPct", 0.35),
		RequirePoke:        numberValue(family, "requirePoke", 0) != 0,
		StopPadATR:         numberValue(family, "stopPadAtr", numberValue(stop, "paddingAtr", 0.25)),
		TargetR:            numberValue(target, "ihreR", 1.5),
	}
}

func applyExtractedSetupParams(p *flagParams, cfg, stop, openingRange dsl.Config, setupType string) {
	p.DownShockRebound = downShockReboundParamsFromConfig(cfg)
	p.WeekendExtremeFade = weekendExtremeFadeParamsFromConfig(cfg, stop)
	p.IntraHourRunExhaustion = intraHourRunExhaustionParamsFromConfig(cfg, stop, mapValue(cfg, "target"))
	p.ORBOpeningSessionsOverride = setupType == string(dsl.FamilyOpeningRangeBreakout) && rawLengthNonempty(openingRange["openingSessions"])
}

// fixedATRStop returns the `stop N ATR` multiple, or 0 when the program
// declares another stop type.
func fixedATRStop(stop map[string]any) float64 {
	if stringValue(stop, "type", "") != "fixedAtr" {
		return 0
	}
	return numberValue(stop, "atr", 0)
}

// validatePriceMomentumStopConfig refuses an explicit price-momentum `stop N
// ATR` whose multiple is not a finite positive number. Without it a zero,
// negative or non-numeric multiple reads as "no fixed stop" and the family
// would trade the recent-extreme stop instead of the declared one. Absent
// fixed stops and other families are unaffected.
func validatePriceMomentumStopConfig(cfg dsl.Config) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyPriceMomentum) {
		return nil
	}
	stop := mapValue(cfg, "stop")
	if stringValue(stop, "type", "") != "fixedAtr" {
		return nil
	}
	var multiple float64
	switch v := stop["atr"].(type) {
	case float64:
		multiple = v
	case float32:
		multiple = float64(v)
	case int:
		multiple = float64(v)
	case int64:
		multiple = float64(v)
	default:
		return errors.New("price momentum stop N ATR requires a numeric multiple")
	}
	if math.IsNaN(multiple) || math.IsInf(multiple, 0) || multiple <= 0 {
		return errors.New("price momentum stop N ATR requires a finite positive multiple")
	}
	return nil
}
