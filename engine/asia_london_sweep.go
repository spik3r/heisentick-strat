package engine

import (
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const millisecondsPerDay = 86_400_000.0
const millisecondsPerHour = 3_600_000.0

// AsiaLondonSweepParams defines the session profile and first-sweep rules used
// by the VP Asia-London continuation strategies.
type AsiaLondonSweepParams struct {
	TickSize                         float64
	RowsLayout                       string
	SessionRows                      float64
	ValueAreaPercent                 float64
	SourceStartHour                  float64
	AsiaEndHour                      float64
	LondonStartHour                  float64
	LondonEndHour                    float64
	MinAsiaRangePercentile           float64
	AsiaRangeLookback                int
	MinAsiaRangeHistory              int
	MaxTargetDistanceRange           float64
	StopBufferRange                  float64
	RMultiple                        float64
	AllowedSide                      int
	RequireOpenOutsideCompletedValue bool
}

// AsiaLondonSweepSignal is a causal entry decision made on the current bar.
type AsiaLondonSweepSignal struct {
	Side                   string
	Entry                  float64
	Stop                   float64
	Target                 float64
	PredictedSweep         string
	OpenOutsideCompletedVA bool
	FirstRaidTargetR       float64
	SourceRange            float64
	AsiaHigh               float64
	AsiaLow                float64
	AsiaPOC                float64
	AsiaVAH                float64
	AsiaVAL                float64
	Risk                   float64
}

// AsiaLondonSweepState advances one daily Asia profile and the London
// first-sweep decision. Call Process once for each bar in timestamp order.
// Completed Asia ranges are carried into the next UTC day, so the range filter
// never includes the current day in its own threshold.
type AsiaLondonSweepState struct {
	dayKey       int64
	initialized  bool
	rangeHistory []float64

	profileBars   []marketdata.Bar
	profile       *VolumeProfile
	profileOK     bool
	asiaHigh      float64
	asiaLow       float64
	asiaBars      int
	rangeRecorded bool
	predicted     string
	firstSweep    string
	done          bool
	traded        bool
}

// Process records source-session bars and advances the London setup. It
// returns a signal at most once per UTC day. Bars outside the configured
// source and trade windows only advance the day boundary.
func (s *AsiaLondonSweepState) Process(bar marketdata.Bar, p AsiaLondonSweepParams) *AsiaLondonSweepSignal {
	dayKey := int64(math.Floor(bar.T / millisecondsPerDay))
	if !s.initialized || s.dayKey != dayKey {
		s.resetDay(dayKey)
	}
	hour := utcHourOfDay(bar.T)
	if hour >= p.SourceStartHour && hour < p.AsiaEndHour {
		s.profileBars = append(s.profileBars, bar)
		s.asiaHigh = math.Max(s.asiaHigh, bar.H)
		s.asiaLow = math.Min(s.asiaLow, bar.L)
		s.asiaBars++
		return nil
	}
	if hour < p.LondonStartHour || hour >= p.LondonEndHour || s.done || s.traded {
		return nil
	}
	if !s.profileOK {
		profile, ok := ComputeVolumeProfile(s.profileBars, p.TickSize, p.RowsLayout, p.SessionRows, p.ValueAreaPercent)
		if !ok || !isFinite(profile.POC) || !isFinite(profile.VAH) || !isFinite(profile.VAL) {
			return nil
		}
		s.profile, s.profileOK = &profile, true
	}
	if s.asiaBars == 0 || !(s.asiaHigh > s.asiaLow) {
		return nil
	}
	if s.predicted == "" {
		s.predicted = predictAsiaSweep(bar.O, *s.profile)
		if s.predicted == "" {
			s.done = true
			return nil
		}
		pass := s.passesRangeFilter(p) && s.passesTargetDistanceFilter(bar.O, p)
		s.recordRange()
		if !pass {
			s.done = true
		}
	}
	if s.done || s.predicted == "" {
		return nil
	}
	hitHigh, hitLow := bar.H > s.asiaHigh, bar.L < s.asiaLow
	if hitHigh && hitLow {
		s.done, s.firstSweep = true, "both"
		return nil
	}
	if s.predicted == "high" {
		if hitLow {
			s.done, s.firstSweep = true, "low"
			return nil
		}
		if hitHigh {
			s.firstSweep = "high"
			if bar.C > s.asiaHigh {
				return s.enter(bar, "high", p)
			}
			s.done = true
		}
		return nil
	}
	if hitHigh {
		s.done, s.firstSweep = true, "high"
		return nil
	}
	if hitLow {
		s.firstSweep = "low"
		if bar.C < s.asiaLow {
			return s.enter(bar, "low", p)
		}
		s.done = true
	}
	return nil
}

// RangeHistory returns a copy of the ranges already recorded by the state.
func (s *AsiaLondonSweepState) RangeHistory() []float64 {
	return append([]float64(nil), s.rangeHistory...)
}

// FirstSweep reports the invalidating or expected edge first swept today.
func (s *AsiaLondonSweepState) FirstSweep() string { return s.firstSweep }

func (s *AsiaLondonSweepState) resetDay(dayKey int64) {
	s.dayKey = dayKey
	s.initialized = true
	s.profileBars = nil
	s.profile = nil
	s.profileOK = false
	s.asiaHigh, s.asiaLow = math.Inf(-1), math.Inf(1)
	s.asiaBars = 0
	s.rangeRecorded = false
	s.predicted = ""
	s.firstSweep = ""
	s.done = false
	s.traded = false
}

func (s *AsiaLondonSweepState) passesRangeFilter(p AsiaLondonSweepParams) bool {
	if !(p.MinAsiaRangePercentile > 0) {
		return true
	}
	lookback := p.AsiaRangeLookback
	if lookback < 1 {
		lookback = 120
	}
	minHistory := p.MinAsiaRangeHistory
	if minHistory < 1 {
		minHistory = 30
	}
	start := max(0, len(s.rangeHistory)-lookback)
	history := s.rangeHistory[start:]
	if len(history) < minHistory {
		return false
	}
	threshold := trailingPercentileFloor(history, p.MinAsiaRangePercentile)
	rangeSize := s.asiaHigh - s.asiaLow
	return isFinite(threshold) && rangeSize >= threshold
}

func (s *AsiaLondonSweepState) passesTargetDistanceFilter(open float64, p AsiaLondonSweepParams) bool {
	if !(p.MaxTargetDistanceRange > 0) {
		return true
	}
	rangeSize := s.asiaHigh - s.asiaLow
	if !(rangeSize > 0) {
		return false
	}
	edge := s.asiaLow
	if s.predicted == "high" {
		edge = s.asiaHigh
	}
	distance := math.Abs(edge-open) / rangeSize
	return distance <= p.MaxTargetDistanceRange+1e-12
}

func (s *AsiaLondonSweepState) recordRange() {
	if s.rangeRecorded {
		return
	}
	rangeSize := s.asiaHigh - s.asiaLow
	if isFinite(rangeSize) && rangeSize > 0 {
		s.rangeHistory = append(s.rangeHistory, rangeSize)
		if len(s.rangeHistory) > 500 {
			s.rangeHistory = append([]float64(nil), s.rangeHistory[len(s.rangeHistory)-500:]...)
		}
	}
	s.rangeRecorded = true
}

func (s *AsiaLondonSweepState) enter(bar marketdata.Bar, sideKey string, p AsiaLondonSweepParams) *AsiaLondonSweepSignal {
	openOutside := predictAsiaSweep(bar.O, *s.profile) == sideKey
	if p.RequireOpenOutsideCompletedValue && !openOutside {
		s.done = true
		return nil
	}
	if p.AllowedSide == 1 && sideKey != "high" || p.AllowedSide == -1 && sideKey != "low" {
		s.done = true
		return nil
	}
	side := "short"
	sign := -1.0
	edge := s.asiaLow
	if sideKey == "high" {
		side, sign, edge = "long", 1, s.asiaHigh
	}
	rangeSize := s.asiaHigh - s.asiaLow
	stop := edge + sign*(-rangeSize*p.StopBufferRange)
	risk := math.Abs(bar.C - stop)
	if !(risk > 0) {
		s.done = true
		return nil
	}
	target := bar.C + sign*risk*p.RMultiple
	s.traded, s.done = true, true
	return &AsiaLondonSweepSignal{
		Side: side, Entry: bar.C, Stop: stop, Target: target, PredictedSweep: sideKey,
		OpenOutsideCompletedVA: openOutside, FirstRaidTargetR: math.Abs(edge-bar.O) / rangeSize,
		SourceRange: rangeSize, AsiaHigh: s.asiaHigh, AsiaLow: s.asiaLow,
		AsiaPOC: s.profile.POC, AsiaVAH: s.profile.VAH, AsiaVAL: s.profile.VAL, Risk: risk,
	}
}

func predictAsiaSweep(open float64, profile VolumeProfile) string {
	if open > profile.VAH {
		return "high"
	}
	if open < profile.VAL {
		return "low"
	}
	return ""
}

func trailingPercentileFloor(values []float64, p float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	index := int(math.Floor(float64(len(copyValues)-1) * p))
	index = min(len(copyValues)-1, max(0, index))
	return copyValues[index]
}

func utcHourOfDay(milliseconds float64) float64 {
	withinDay := math.Mod(milliseconds, millisecondsPerDay)
	if withinDay < 0 {
		withinDay += millisecondsPerDay
	}
	return withinDay / millisecondsPerHour
}
