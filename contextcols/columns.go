// Package contextcols builds causal context columns from market data.
package contextcols

import (
	"math/big"

	"github.com/spik3r/heisentick-strat/marketdata"
)

type columnNeeds struct {
	SessionPhase bool
	InSession    bool
	Day          bool
	PriorDay     bool
	PriorDayType bool
	Camarilla    bool
	PriorSession bool
	Overnight    bool
	OpenLocation bool
	DayProgress  bool
	RangeStats   bool
	SessionBias  bool
	VWAP         bool
	VolumeStats  bool
	EMA          bool
	RegimeTrend  bool
	Swing        bool
	Range        bool
	RangeActive  bool
	Channel      bool
}

func (o Options) needs() columnNeeds {
	if !o.Selective {
		return columnNeeds{
			SessionPhase: true,
			InSession:    true,
			Day:          true,
			PriorDay:     true,
			PriorDayType: true,
			Camarilla:    true,
			PriorSession: true,
			Overnight:    true,
			OpenLocation: true,
			DayProgress:  true,
			RangeStats:   true,
			SessionBias:  true,
			VWAP:         true,
			VolumeStats:  true,
			EMA:          true,
			RegimeTrend:  true,
			Swing:        true,
			Range:        true,
			RangeActive:  true,
			Channel:      true,
		}
	}
	needs := columnNeeds{
		SessionPhase: o.NeedSessionPhase,
		Day:          true,
		PriorDay:     o.NeedPriorDay,
		PriorDayType: o.NeedPriorDay,
		RegimeTrend:  o.NeedRegimeTrend,
		RangeActive:  o.NeedRangeActive || o.NeedRegimeTrend,
		OpenLocation: o.NeedOpenLocation,
		EMA:          o.EMAFastLen > 0,
	}
	if o.NeedReportTradeContext {
		needs.PriorDay = true
		needs.PriorDayType = true
		needs.OpenLocation = true
	}
	return needs
}

// Columns contains the slice-4 core causal columns. Later context families
// should extend this package in focused files instead of growing build.go.
type Columns struct {
	Length int

	ATR []float64
	ER  []float64

	HourUTC      []float64
	SessionPhase []int8
	OpenLocation []int8
	PriorDayType []int8
	InSession    SessionMembership

	DayHigh  []float64
	DayLow   []float64
	DayOpen  []float64
	IsNewDay []int8

	Camarilla        CamarillaColumns
	PriorSession     PriorSessionColumns
	OvernightRange   OvernightRangeColumns
	DayRangeProgress []float64
	RangeStats       RangeStatColumns
	SessionBias      SessionBiasColumns

	PriorDayO []float64
	PriorDayH []float64
	PriorDayL []float64
	PriorDayC []float64

	VWAP            []float64
	VWAPDistanceATR []float64

	VolumeSMA20       []float64
	VolumeRatio20     []float64
	VolumeZ50         []float64
	SpreadATR         []float64
	BodyATR           []float64
	EffortResultRatio []float64

	EMAFast            []float64
	EMAFastSlope       []float64
	EMAFastDistanceATR []float64

	Regime      []int8
	TrendDir    []int8
	SwingHigh   []float64
	SwingLow    []float64
	Range       RangeColumns
	LastRange   LastRangeColumns
	Channel     ChannelColumns
	LastChannel ChannelColumns
}

// SessionMembership stores data-session membership using the JS session names.
type SessionMembership struct {
	Asia   []int8
	London []int8
	NY     []int8
}

func makeColumns(n int, needs columnNeeds) Columns {
	cols := Columns{
		Length: n,
		ATR:    make([]float64, n),
		ER:     make([]float64, n),
	}
	if needs.InSession || needs.OpenLocation {
		cols.HourUTC = make([]float64, n)
	}
	if needs.SessionPhase {
		cols.SessionPhase = make([]int8, n)
	}
	if needs.OpenLocation {
		cols.OpenLocation = make([]int8, n)
	}
	if needs.PriorDayType {
		cols.PriorDayType = make([]int8, n)
	}
	if needs.InSession {
		cols.InSession = makeSessionMembership(n)
	}
	if needs.Day || needs.OpenLocation {
		cols.DayHigh = make([]float64, n)
		cols.DayLow = make([]float64, n)
		cols.DayOpen = make([]float64, n)
		cols.IsNewDay = make([]int8, n)
	}
	if needs.Camarilla {
		cols.Camarilla = makeCamarillaColumns(n)
	}
	if needs.PriorSession || needs.OpenLocation {
		cols.PriorSession = makePriorSessionColumns(n)
	}
	if needs.Overnight || needs.OpenLocation {
		cols.OvernightRange = makeOvernightRangeColumns(n)
	}
	if needs.DayProgress {
		cols.DayRangeProgress = nanSlice(n)
	}
	if needs.RangeStats {
		cols.RangeStats = makeRangeStatColumns(n)
	}
	if needs.SessionBias {
		cols.SessionBias = makeSessionBiasColumns(n)
	}
	if needs.PriorDay || needs.Camarilla || needs.PriorDayType || needs.OpenLocation {
		cols.PriorDayO = nanSlice(n)
		cols.PriorDayH = nanSlice(n)
		cols.PriorDayL = nanSlice(n)
		cols.PriorDayC = nanSlice(n)
	}
	if needs.VWAP {
		cols.VWAP = nanSlice(n)
		cols.VWAPDistanceATR = nanSlice(n)
	}
	if needs.VolumeStats {
		cols.VolumeSMA20 = nanSlice(n)
		cols.VolumeRatio20 = nanSlice(n)
		cols.VolumeZ50 = nanSlice(n)
		cols.SpreadATR = nanSlice(n)
		cols.BodyATR = nanSlice(n)
		cols.EffortResultRatio = nanSlice(n)
	}
	if needs.EMA {
		cols.EMAFast = nanSlice(n)
		cols.EMAFastSlope = nanSlice(n)
		cols.EMAFastDistanceATR = nanSlice(n)
	}
	if needs.RegimeTrend {
		cols.Regime = make([]int8, n)
		cols.TrendDir = make([]int8, n)
	}
	if needs.Swing {
		cols.SwingHigh = nanSlice(n)
		cols.SwingLow = nanSlice(n)
	}
	if needs.Range {
		cols.Range = makeRangeColumns(n)
		cols.LastRange = makeLastRangeColumns(n)
	}
	if needs.Channel {
		cols.Channel = makeChannelColumns(n)
		cols.LastChannel = makeChannelColumns(n)
	}
	return cols
}

func makeSessionMembership(n int) SessionMembership {
	return SessionMembership{
		Asia:   make([]int8, n),
		London: make([]int8, n),
		NY:     make([]int8, n),
	}
}

// Build computes the currently ported causal context columns.
func Build(series marketdata.Series, options Options) Columns {
	options = options.normalized()
	n := series.Len()
	needs := options.needs()
	cols := makeColumns(n, needs)
	if n == 0 {
		return cols
	}

	cols.ATR = ComputeATR(series, options.ATRLen)
	if options.TrendERMode == "trueRange" {
		cols.ER = ComputeTrueRangeER(series, options.ERLen)
	} else {
		cols.ER = ComputeER(series, options.ERLen)
	}
	if needs.VolumeStats {
		volumeStats := ComputeVolumeAnomalyStats(series, cols.ATR, 20, 50)
		cols.VolumeSMA20 = volumeStats.SMA20
		cols.VolumeRatio20 = volumeStats.Ratio20
		cols.VolumeZ50 = volumeStats.Z50
		cols.SpreadATR = volumeStats.SpreadATR
		cols.BodyATR = volumeStats.BodyATR
		cols.EffortResultRatio = volumeStats.EffortResultRatio
	}
	if options.EMAFastLen > 0 {
		cols.EMAFast = ComputeEMA(series, options.EMAFastLen)
		slopeLen := options.EMAFastSlopeLen
		if slopeLen <= 0 {
			slopeLen = 5
		}
		cols.EMAFastSlope = ComputeSlopeFromSeries(cols.EMAFast, slopeLen)
	}
	days := summarizeDays(series)
	dayPrev := previousDays(days)
	dayPrevPrev := previousPreviousDays(days)
	dayRangeAvg := priorDayRangeAverages(days, options.RangeStatsLookback)
	var sessionRuns sessionRunsByName
	if needs.PriorSession || needs.OpenLocation {
		sessionRuns = buildSessionRuns(series, days)
	}
	var overnightRuns []sessionRun
	if needs.Overnight || needs.OpenLocation {
		overnightRuns = buildOvernightRuns(series, sortedDayKeys(days))
	}
	var rangeStats []RangeStatsRow
	if needs.RangeStats {
		rangeStats = ComputeRangeStats(series, options.RangeStatsLookback)
	}
	var sessionBias []SessionBiasRow
	if needs.SessionBias {
		sessionBias = ComputeSessionBias(series, cols.ATR)
	}
	var swings Swings
	if needs.Swing || needs.Range || (needs.RangeActive && options.Range.Method != "zone") || needs.Channel {
		swings = ComputeSwings(series, options.PivotK)
	}
	channelSwings := swings
	if needs.Channel && options.Channel.Source == "close" {
		channelSwings = ComputeSwingsClose(series, options.PivotK)
	}
	var ranges RangeDetection
	if needs.Range || needs.RangeActive {
		ranges = DetectRanges(series, cols.ATR, swings, options.Range, options.PivotK)
	}
	var channels ChannelColumns
	if needs.Channel {
		channels = DetectChannels(series, cols.ATR, channelSwings, options.Channel, options.PivotK)
	}

	var curDay int64 = -1
	var dayHigh, dayLow, dayOpen float64
	var vwapDen float64
	var vwapNumJS *big.Float
	if needs.VWAP {
		vwapNumJS = new(big.Float).SetPrec(53).SetMode(big.ToNearestEven)
	}
	var lastRangeHigh, lastRangeLow float64
	hasLastRange := false
	lastRangeSinceActive := 0
	var lastChannel channelState
	hasLastChannel := false
	lastChannelSinceActive := 0
	trendCandidateBars := 0
	trendActive := false
	sessionPtr := sessionRunPointers{Asia: -1, London: -1, NY: -1}
	overnightPtr := -1
	for i := 0; i < n; i++ {
		t := int64(series.T[i])
		day := UTCDayKey(t)
		if day != curDay {
			curDay = day
			dayHigh = series.H[i]
			dayLow = series.L[i]
			dayOpen = series.O[i]
			vwapDen = 0
			if vwapNumJS != nil {
				vwapNumJS.SetFloat64(0)
			}
			if len(cols.IsNewDay) > 0 {
				cols.IsNewDay[i] = 1
			}
		} else {
			dayHigh = max(dayHigh, series.H[i])
			dayLow = min(dayLow, series.L[i])
		}
		prior, hasPrior := dayPrev[day]
		priorPrev, hasPriorPrev := dayPrevPrev[day]

		if hasPrior {
			if len(cols.PriorDayO) > 0 {
				cols.PriorDayO[i] = prior.O
				cols.PriorDayH[i] = prior.H
				cols.PriorDayL[i] = prior.L
				cols.PriorDayC[i] = prior.C
			}
			if needs.Camarilla {
				writeCamarilla(cols.Camarilla, i, prior)
			}
			if len(cols.PriorDayType) > 0 {
				if hasPriorPrev {
					cols.PriorDayType[i] = classifyPriorDayType(prior, priorPrev)
				}
			}
		}

		if needs.PriorSession || needs.OpenLocation {
			sessionPtr = writePriorSessions(cols.PriorSession, i, t, sessionRuns, sessionPtr)
		}
		if needs.Overnight || needs.OpenLocation {
			overnightPtr = writeOvernightRange(cols.OvernightRange, i, t, cols.ATR[i], overnightRuns, overnightPtr)
		}
		if len(cols.DayRangeProgress) > 0 {
			if avg := dayRangeAvg[day]; avg > 0 && isFinite(avg) {
				cols.DayRangeProgress[i] = (dayHigh - dayLow) / avg
			}
		}
		if needs.RangeStats {
			writeRangeStats(cols.RangeStats, i, rangeStats[i])
		}
		if needs.SessionBias {
			writeSessionBias(cols.SessionBias, i, sessionBias[i])
		}

		if needs.VWAP && isFinite(series.V[i]) && isFinite(series.C[i]) && series.V[i] >= 0 {
			priceVolume := series.C[i] * series.V[i]
			vwapNumJS.Add(vwapNumJS, new(big.Float).SetPrec(53).SetMode(big.ToNearestEven).SetFloat64(priceVolume))
			vwapDen += series.V[i]
		}
		if vwapDen > 0 {
			vwapNumRounded, _ := vwapNumJS.Float64()
			cols.VWAP[i] = vwapNumRounded / vwapDen
			if cols.ATR[i] > 0 {
				cols.VWAPDistanceATR[i] = (series.C[i] - cols.VWAP[i]) / cols.ATR[i]
			}
		}
		if options.EMAFastLen > 0 && isFinite(cols.EMAFast[i]) && cols.ATR[i] > 0 {
			cols.EMAFastDistanceATR[i] = (series.C[i] - cols.EMAFast[i]) / cols.ATR[i]
		}

		hourUTC := HourUTC(t)
		if len(cols.HourUTC) > 0 {
			cols.HourUTC[i] = hourUTC
		}
		if len(cols.SessionPhase) > 0 {
			cols.SessionPhase[i] = SessionPhaseCode(SessionPhaseAtHour(LocalHour(t)))
		}
		if needs.InSession {
			cols.InSession.Asia[i] = sessionFlag(hourUTC, sessionAsia)
			cols.InSession.London[i] = sessionFlag(hourUTC, sessionLondon)
			cols.InSession.NY[i] = sessionFlag(hourUTC, sessionNY)
		}
		if needs.OpenLocation {
			cols.OpenLocation[i] = classifyOpenLocation(series.C[i], openLocationRefs{
				PDH:           cols.PriorDayH[i],
				PDL:           cols.PriorDayL[i],
				PDO:           cols.PriorDayO[i],
				PDC:           cols.PriorDayC[i],
				DayOpen:       dayOpen,
				RangeMid:      (dayHigh + dayLow) / 2,
				AsiaHigh:      cols.PriorSession.Asia.H[i],
				AsiaLow:       cols.PriorSession.Asia.L[i],
				LondonHigh:    cols.PriorSession.London.H[i],
				LondonLow:     cols.PriorSession.London.L[i],
				OvernightHigh: cols.OvernightRange.High[i],
				OvernightLow:  cols.OvernightRange.Low[i],
				CurrentOpen:   dayOpen,
			}, cols.ATR[i])
		}
		if len(cols.DayHigh) > 0 {
			cols.DayHigh[i] = dayHigh
			cols.DayLow[i] = dayLow
			cols.DayOpen[i] = dayOpen
		}
		if needs.Swing {
			cols.SwingHigh[i] = swings.LastHigh[i]
			cols.SwingLow[i] = swings.LastLow[i]
		}
		rangeActive := needs.RangeActive && ranges.Active[i] == 1
		if needs.Range && rangeActive {
			cols.Range.High[i] = ranges.High[i]
			cols.Range.Low[i] = ranges.Low[i]
			cols.Range.Active[i] = 1
			lastRangeHigh = ranges.High[i]
			lastRangeLow = ranges.Low[i]
			hasLastRange = true
			lastRangeSinceActive = 0
		} else if hasLastRange {
			lastRangeSinceActive++
		}
		if needs.Range && hasLastRange {
			cols.LastRange.High[i] = lastRangeHigh
			cols.LastRange.Low[i] = lastRangeLow
			cols.LastRange.SinceActive[i] = int32(lastRangeSinceActive)
		}
		if needs.Channel && channels.Active[i] == 1 {
			writeChannel(cols.Channel, i, channels.At(i))
			lastChannel = channels.At(i)
			hasLastChannel = true
			lastChannelSinceActive = 0
		} else if hasLastChannel {
			lastChannelSinceActive++
		}
		if needs.Channel && hasLastChannel {
			projected := projectChannel(lastChannel, i, lastChannelSinceActive)
			writeChannel(cols.LastChannel, i, projected)
		}
		if needs.RegimeTrend {
			cols.Regime[i], cols.TrendDir[i] = classifyRegimeTrendState(series, i, cols.ER[i], options.ERLen, options.TrendEnterER, options.TrendExitER, options.TrendPersistenceBars, rangeActive, &trendActive, &trendCandidateBars)
		}
	}
	return cols
}

type daySummary struct {
	O        float64
	H        float64
	L        float64
	C        float64
	FirstIdx int
	LastIdx  int
}

func summarizeDays(series marketdata.Series) map[int64]daySummary {
	days := make(map[int64]daySummary)
	for i := 0; i < series.Len(); i++ {
		key := UTCDayKey(int64(series.T[i]))
		day, ok := days[key]
		if !ok {
			days[key] = daySummary{O: series.O[i], H: series.H[i], L: series.L[i], C: series.C[i], FirstIdx: i, LastIdx: i}
			continue
		}
		day.H = max(day.H, series.H[i])
		day.L = min(day.L, series.L[i])
		day.C = series.C[i]
		day.LastIdx = i
		days[key] = day
	}
	return days
}
