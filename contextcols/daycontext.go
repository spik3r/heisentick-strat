package contextcols

import "math"

const openLocationToleranceATR = 0.65

const (
	openLocationNearPDH         int8 = 1
	openLocationNearPDL         int8 = 2
	openLocationNearPDO         int8 = 3
	openLocationNearPDC         int8 = 4
	openLocationNearDO          int8 = 5
	openLocationRangeMid        int8 = 6
	openLocationNearAH          int8 = 7
	openLocationNearAL          int8 = 8
	openLocationNearLH          int8 = 9
	openLocationNearLL          int8 = 10
	openLocationNearNH          int8 = 11
	openLocationNearNL          int8 = 12
	openLocationNearCurrentOpen int8 = 15
	openLocationOther           int8 = 16

	priorDayTypeRange   int8 = 1
	priorDayTypeTrend   int8 = 2
	priorDayTypeWide    int8 = 3
	priorDayTypeNarrow  int8 = 4
	priorDayTypeOutside int8 = 5
)

type openLocationRefs struct {
	PDH           float64
	PDL           float64
	PDO           float64
	PDC           float64
	DayOpen       float64
	RangeMid      float64
	AsiaHigh      float64
	AsiaLow       float64
	LondonHigh    float64
	LondonLow     float64
	OvernightHigh float64
	OvernightLow  float64
	CurrentOpen   float64
}

func classifyOpenLocation(refPrice float64, refs openLocationRefs, atr float64) int8 {
	if !isFinite(refPrice) {
		return openLocationOther
	}
	budget := 0.0
	if isFinite(atr) && atr > 0 {
		budget = atr * openLocationToleranceATR
	}
	if budget == 0 {
		return openLocationOther
	}
	bestCode := openLocationOther
	bestDistance := math.Inf(1)
	candidates := []struct {
		code  int8
		price float64
	}{
		{openLocationNearPDH, refs.PDH},
		{openLocationNearPDL, refs.PDL},
		{openLocationNearPDO, refs.PDO},
		{openLocationNearPDC, refs.PDC},
		{openLocationNearDO, refs.DayOpen},
		{openLocationRangeMid, refs.RangeMid},
		{openLocationNearAH, refs.AsiaHigh},
		{openLocationNearAL, refs.AsiaLow},
		{openLocationNearLH, refs.LondonHigh},
		{openLocationNearLL, refs.LondonLow},
		{openLocationNearNH, refs.OvernightHigh},
		{openLocationNearNL, refs.OvernightLow},
		{openLocationNearCurrentOpen, refs.CurrentOpen},
	}
	for _, candidate := range candidates {
		if !isFinite(candidate.price) {
			continue
		}
		distance := math.Abs(refPrice - candidate.price)
		if distance > budget {
			continue
		}
		if distance < bestDistance {
			bestCode = candidate.code
			bestDistance = distance
		}
	}
	return bestCode
}

func classifyPriorDayType(current daySummary, prior daySummary) int8 {
	r := current.H - current.L
	if !isFinite(r) || r <= 0 {
		return 0
	}
	priorRange := prior.H - prior.L
	closeRangeRatio := math.Abs(current.C-current.O) / r
	if current.H > prior.H && current.L < prior.L {
		return priorDayTypeOutside
	}
	if closeRangeRatio >= 0.45 {
		return priorDayTypeTrend
	}
	if !isFinite(priorRange) || priorRange <= 0 {
		return priorDayTypeRange
	}
	relative := r / priorRange
	if relative >= 1.45 {
		return priorDayTypeWide
	}
	if relative <= 0.65 {
		return priorDayTypeNarrow
	}
	return priorDayTypeRange
}
