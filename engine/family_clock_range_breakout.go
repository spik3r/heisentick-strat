package engine

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Clock range breakout (spec/dsl-spec-families/clockRangeBreakout.md).
//
// After a fixed-clock range closes, place a buy stop above its high and a sell
// stop below its low. The first modeled fill wins and cancels the other order,
// a percent stop protects the trade, and a required clock close liquidates it.
// One entry per range day. The family runs its own bar loop on the shared
// broker's position, cost and serialization code; it reads no context column,
// session, window or management setting.

const (
	clockRangeTag       = "DSL-CRB"
	clockRangeMetaKey   = "clockRangeBreakout"
	clockRangeCloseRule = "clock-range-close"
	// clockRangeMaxTimestamp is the last millisecond of year 9999. Larger
	// values would make the per-day loop unbounded for no real data.
	clockRangeMaxTimestamp = 253402300799999.0
	msPerClockDay          = int64(24 * 60 * 60 * 1000)
	msPerClockMinute       = int64(60 * 1000)
)

// Outcomes of a range day, observable through ClockRangeDays.
const (
	ClockDayEntered  = "entered"
	ClockDayRejected = "rejected"
	ClockDayExpired  = "expired"
	// ClockDayPending marks orders still live when the data ended.
	ClockDayPending = "pending"
)

// Stable reasons for a rejected day or a cancelled fill.
const (
	ClockReasonEmptyRange        = "empty-range"
	ClockReasonInsufficient      = "insufficient-coverage"
	ClockReasonMissingFinalSlot  = "missing-final-slot"
	ClockReasonNonpositiveWidth  = "nonpositive-range-width"
	ClockReasonInvalidPrice      = "invalid-price"
	ClockReasonBrokerBlocked     = "broker-blocked"
	ClockReasonInvalidFill       = "invalid-fill"
	ClockReasonInvalidStop       = "invalid-stop-distance"
	ClockReasonInvalidSize       = "invalid-size"
	ClockReasonOutsideExecWindow = "outside-execution-window"
)

// ClockRangeDay records what happened to one declared-clock range day so
// rejected days and expired orders are observable without inventing trades.
type ClockRangeDay struct {
	DayKey       string  `json:"dayKey"`
	RangeStartT  float64 `json:"rangeStartT"`
	RangeEndT    float64 `json:"rangeEndT"`
	ExpectedBars int     `json:"expectedBars"`
	RangeBars    int     `json:"rangeBars"`
	Outcome      string  `json:"outcome"`
	Reason       string  `json:"reason,omitempty"`
	Side         string  `json:"side,omitempty"`
}

type clockPendingOrders struct {
	auditIndex int
	day        int64
	expiryT    float64
	closeT     float64
	rangeHigh  float64
	rangeLow   float64
	rangeBars  int
	dayKey     string
	rangeEndT  float64
	buy, sell  float64
	hasLong    bool
	hasShort   bool
}

type clockRangeState struct {
	days    []ClockRangeDay
	pending *clockPendingOrders
	// closeT is the resolved clock-close instant of the open position.
	closeT float64
}

func (s *clockRangeState) reset() {
	s.days = s.days[:0]
	s.pending = nil
	s.closeT = 0
}

type clockSchedule struct {
	spec      dsl.ClockRangeSpec
	offsetMs  int64
	timeframe float64
}

func (c clockSchedule) dayStart(day int64) int64 { return day*msPerClockDay - c.offsetMs }

func (c clockSchedule) at(day int64, minute int) float64 {
	return float64(c.dayStart(day) + int64(minute)*msPerClockMinute)
}

func (c clockSchedule) rangeStart(day int64) float64 { return c.at(day, c.spec.RangeStartMinute) }
func (c clockSchedule) rangeEnd(day int64) float64   { return c.at(day, c.spec.RangeEndMinute) }
func (c clockSchedule) expiry(day int64) float64     { return c.at(day, c.spec.ResolvedExpiryMinute()) }
func (c clockSchedule) close(day int64) float64      { return c.at(day, c.spec.ResolvedCloseMinute()) }

// clockDayOf is the declared-clock calendar day (days since 1970-01-01 on that
// clock) containing the UTC instant t. It depends only on the fixed offset.
func (c clockSchedule) clockDayOf(t float64) int64 {
	return floorDivInt64(int64(t)+c.offsetMs, msPerClockDay)
}

func clockDayKey(day int64) string {
	return time.Unix(day*86400, 0).UTC().Format("2006-01-02")
}

func newClockSchedule(spec dsl.ClockRangeSpec, timeframe string) (clockSchedule, error) {
	minutes, ok := dsl.ClockRangeTimeframeMinutes(timeframe)
	if !ok {
		return clockSchedule{}, fmt.Errorf("clockRangeBreakout: unsupported timeframe %q", timeframe)
	}
	return clockSchedule{spec: spec, offsetMs: int64(spec.UTCOffsetMinutes) * msPerClockMinute, timeframe: float64(int64(minutes) * msPerClockMinute)}, nil
}

// validateClockRangeRun checks everything the family needs before a run: a
// well-formed config, a reviewed pip size, a supported route timeframe whose
// grid holds the four boundaries, and a series that is strictly increasing,
// grid-aligned and OHLC-consistent. It applies only to this family; the shared
// series checks of other families are unchanged.
func validateClockRangeRun(cfg dsl.Config, symbol, timeframe string, series marketdata.Series) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyClockRangeBreakout) {
		return nil
	}
	spec, err := dsl.DecodeClockRangeBreakout(cfg)
	if err != nil {
		return fmt.Errorf("clockRangeBreakout: invalid config: %w", err)
	}
	if _, ok := dsl.ReviewedInstrumentPip(symbol); !ok {
		return fmt.Errorf("clockRangeBreakout: no reviewed pip size for symbol %q", symbol)
	}
	schedule, err := newClockSchedule(spec, timeframe)
	if err != nil {
		return err
	}
	if err := spec.AlignmentError(timeframe); err != nil {
		return fmt.Errorf("clockRangeBreakout: %w", err)
	}
	for i := range series.T {
		t := series.T[i]
		switch {
		case t < 0 || t > clockRangeMaxTimestamp:
			return fmt.Errorf("clockRangeBreakout: malformed series: timestamp at index %d is outside the supported range", i)
		case math.Mod(t, schedule.timeframe) != 0:
			return fmt.Errorf("clockRangeBreakout: malformed series: timestamp at index %d is not aligned to the %s UTC grid", i, timeframe)
		case i > 0 && !(t > series.T[i-1]):
			return fmt.Errorf("clockRangeBreakout: malformed series: timestamps must be strictly increasing and distinct (index %d)", i)
		case !(series.L[i] <= math.Min(series.O[i], series.C[i])) || !(math.Max(series.O[i], series.C[i]) <= series.H[i]):
			return fmt.Errorf("clockRangeBreakout: malformed series: OHLC bounds violated at index %d", i)
		}
	}
	return nil
}

// ClockRangeDays returns the per-day outcomes of the last run of this
// prepared strategy. It is empty for other families.
func (r *PreparedRun) ClockRangeDays() []ClockRangeDay {
	if r == nil {
		return nil
	}
	return append([]ClockRangeDay(nil), r.broker.clock.days...)
}

func (b *broker) runClockRangeBreakout(liquidateAtEnd bool) []Trade {
	spec := b.params.ClockRange
	pipSize, pipOK := dsl.ReviewedInstrumentPip(b.fixture.Symbol)
	schedule, err := newClockSchedule(spec, b.fixture.Timeframe)
	n := b.series.Len()
	end := b.executionEnd()
	if end >= n {
		end = n - 1
	}
	// A run that skipped validateClockRangeRun cannot trade: fail closed.
	if err != nil || !pipOK || spec.Validate() != nil || end < 0 {
		return b.trades
	}
	nextDay := schedule.clockDayOf(b.series.T[0]) - 1
	for schedule.rangeEnd(nextDay) <= b.series.T[0] {
		nextDay++
	}
	for i := 0; i <= end; i++ {
		t := b.series.T[i]
		for schedule.rangeEnd(nextDay) <= t {
			b.placeClockRangeOrders(schedule, nextDay, i, pipSize)
			nextDay++
		}
		exited := false
		if b.hasPosition {
			switch {
			case t >= b.clock.closeT:
				b.closeClockRangePosition(b.series.O[i], i, ReasonRule, clockRangeCloseRule, "clock")
				exited = true
			case i > b.position.EntryIndex:
				exited = b.clockRangeStopOnLaterBar(i)
			}
		}
		if pending := b.clock.pending; pending != nil {
			if t >= pending.expiryT {
				b.finishClockRangeDay(pending, ClockDayExpired, "")
				b.clock.pending = nil
			} else if !exited && !b.hasPosition {
				b.fillClockRangeOrders(schedule, pending, i)
			}
		}
	}
	if liquidateAtEnd && b.hasPosition {
		b.closeClockRangePosition(b.series.C[end], end, ReasonEndOfTest, "", "end-of-data")
	}
	if pending := b.clock.pending; pending != nil {
		b.finishClockRangeDay(pending, ClockDayPending, "")
	}
	return b.trades
}

func (b *broker) finishClockRangeDay(pending *clockPendingOrders, outcome, reason string) {
	day := &b.clock.days[pending.auditIndex]
	day.Outcome, day.Reason = outcome, reason
}

// placeClockRangeOrders evaluates day at its range end using only bars that
// opened before it. barIndex is the first bar at or after that instant, so the
// range bars are the contiguous run of bars before it.
func (b *broker) placeClockRangeOrders(schedule clockSchedule, day int64, barIndex int, pipSize float64) {
	spec := schedule.spec
	rangeStart, rangeEnd := schedule.rangeStart(day), schedule.rangeEnd(day)
	// Orders left from an earlier day expired before this range ended.
	if old := b.clock.pending; old != nil {
		b.finishClockRangeDay(old, ClockDayExpired, "")
		b.clock.pending = nil
	}
	expected := int((rangeEnd - rangeStart) / schedule.timeframe)
	audit := ClockRangeDay{DayKey: clockDayKey(day), RangeStartT: rangeStart, RangeEndT: rangeEnd, ExpectedBars: expected, Outcome: ClockDayRejected}
	reject := func(reason string) {
		audit.Reason = reason
		b.clock.days = append(b.clock.days, audit)
	}
	lo := sort.SearchFloat64s(b.series.T[:barIndex], rangeStart)
	audit.RangeBars = barIndex - lo
	required := (9*expected + 9) / 10 // ceil(0.9 * expected) in integers
	switch {
	case audit.RangeBars == 0:
		reject(ClockReasonEmptyRange)
		return
	case audit.RangeBars < required:
		reject(ClockReasonInsufficient)
		return
	case b.series.T[barIndex-1] != rangeEnd-schedule.timeframe:
		reject(ClockReasonMissingFinalSlot)
		return
	}
	high, low := b.series.H[lo], b.series.L[lo]
	for j := lo + 1; j < barIndex; j++ {
		high = math.Max(high, b.series.H[j])
		low = math.Min(low, b.series.L[j])
	}
	if !(high > low) {
		reject(ClockReasonNonpositiveWidth)
		return
	}
	pending := &clockPendingOrders{
		day: day, expiryT: schedule.expiry(day), closeT: schedule.close(day),
		rangeHigh: high, rangeLow: low, rangeBars: audit.RangeBars, dayKey: audit.DayKey, rangeEndT: rangeEnd,
		buy: high + spec.BufferPips*pipSize, sell: low - spec.BufferPips*pipSize,
		hasLong: spec.AllowLong, hasShort: spec.AllowShort,
	}
	if (pending.hasLong && !(isFinite(pending.buy) && pending.buy > 0)) || (pending.hasShort && !(isFinite(pending.sell) && pending.sell > 0)) {
		reject(ClockReasonInvalidPrice)
		return
	}
	if b.hasPosition {
		reject(ClockReasonBrokerBlocked)
		return
	}
	audit.Outcome = ClockDayPending
	b.clock.days = append(b.clock.days, audit)
	pending.auditIndex = len(b.clock.days) - 1
	b.clock.pending = pending
}

// fillClockRangeOrders tests the pending stop orders against bar i.
func (b *broker) fillClockRangeOrders(schedule clockSchedule, pending *clockPendingOrders, i int) {
	open, high, low := b.series.O[i], b.series.H[i], b.series.L[i]
	longTouched := pending.hasLong && high >= pending.buy
	shortTouched := pending.hasShort && low <= pending.sell
	if !longTouched && !shortTouched {
		return
	}
	// The pair is cancelled by the first fill, rejected or not: the day is used.
	b.clock.pending = nil
	// With both touched the trigger nearer the open wins; an exact tie is long.
	// This is an OHLC modeling convention, not the real intrabar order.
	long := longTouched
	if longTouched && shortTouched {
		long = math.Abs(open-pending.buy) <= math.Abs(open-pending.sell)
	}
	side, fillBase := sideShort, math.Min(pending.sell, open)
	if long {
		side, fillBase = sideLong, math.Max(pending.buy, open)
	}
	b.openClockRangePosition(schedule, pending, side, fillBase, i)
}

func (b *broker) openClockRangePosition(schedule clockSchedule, pending *clockPendingOrders, s side, fillBase float64, i int) {
	reject := func(reason string) { b.finishClockRangeDay(pending, ClockDayRejected, reason) }
	sign := float64(s)
	fill := fillBase + sign*b.slippageAt(fillBase)
	if !isFinite(fill) || !(fill > 0) {
		reject(ClockReasonInvalidFill)
		return
	}
	distance := fill * schedule.spec.StopPercent / 100
	stop := fill - sign*distance
	if !isFinite(distance) || !(distance > 0) || !isFinite(stop) || !(math.Abs(fill-stop) > 0) {
		reject(ClockReasonInvalidStop)
		return
	}
	// Size is the risk over the stated distance, not over fill - stop, which
	// loses digits to cancellation. An explicit zero risk gives size zero.
	size := b.params.RiskUSD / distance
	if !isFinite(size) {
		reject(ClockReasonInvalidSize)
		return
	}
	meta := TradeMeta{clockRangeMetaKey: map[string]any{
		"rangeHigh":     numberForJSON(pending.rangeHigh),
		"rangeLow":      numberForJSON(pending.rangeLow),
		"rangeBars":     pending.rangeBars,
		"side":          s.String(),
		"dayKey":        pending.dayKey,
		"orderPlacedAt": numberForJSON(pending.rangeEndT),
	}}
	// The slippage above is the single entry slippage; NoSlip keeps the shared
	// broker from applying it a second time.
	b.openPosition(s, fill, order{
		Side: s, SL: stop, Size: size, HasSize: true, RiskUSD: b.params.RiskUSD, HasRisk: true, Tag: clockRangeTag,
		Meta: meta, Index: i, NoSlip: true, NoTarget: true,
	}, i)
	if !b.hasPosition {
		reject(ClockReasonOutsideExecWindow)
		return
	}
	b.finishClockRangeDay(pending, ClockDayEntered, "")
	b.clock.days[pending.auditIndex].Side = s.String()
	b.clock.closeT = pending.closeT
	// The whole entry bar is tested against the stop, even when its extreme
	// may have come before the fill.
	if pos := b.position; (pos.Side == sideLong && b.series.L[i] <= pos.SL) || (pos.Side == sideShort && b.series.H[i] >= pos.SL) {
		b.closeClockRangePosition(pos.SL, i, "sl", "", "stop")
	}
}

// clockRangeStopOnLaterBar exits at the open when the bar opens through the
// stop and at the stop on an ordinary touch.
func (b *broker) clockRangeStopOnLaterBar(i int) bool {
	pos := b.position
	if (pos.Side == sideLong && b.series.O[i] <= pos.SL) || (pos.Side == sideShort && b.series.O[i] >= pos.SL) {
		b.closeClockRangePosition(b.series.O[i], i, "sl", "", "stop")
		return true
	}
	if (pos.Side == sideLong && b.series.L[i] <= pos.SL) || (pos.Side == sideShort && b.series.H[i] >= pos.SL) {
		b.closeClockRangePosition(pos.SL, i, "sl", "", "stop")
		return true
	}
	return false
}

func (b *broker) closeClockRangePosition(price float64, index int, reason, rule, exitReason string) {
	nested := map[string]any{}
	if existing, ok := b.position.Meta[clockRangeMetaKey].(map[string]any); ok {
		for key, value := range existing {
			nested[key] = value
		}
	}
	nested["exitReason"] = exitReason
	b.position.Meta = TradeMeta{clockRangeMetaKey: nested}
	b.closePosition(price, index, reason, rule)
}
