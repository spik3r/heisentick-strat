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
	// Optional gates.
	ClockReasonNoDailyATR      = "no-daily-atr"
	ClockReasonRangeTooNarrow  = "range-too-narrow"
	ClockReasonNoPriorDay      = "no-prior-day"
	ClockReasonNoDirection     = "no-direction"
	ClockReasonOppositeTrigger = "opposite-triggered-first"
	clockRangeTargetExitReason = "target"
	clockTradingDayMinSpanMs   = int64(4 * 60 * 60 * 1000)
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
	// cancelLong / cancelShort mark a trigger that cancels the day instead of
	// filling (cancel when opposite side triggers first).
	cancelLong  bool
	cancelShort bool
}

type clockRangeState struct {
	days    []ClockRangeDay
	pending *clockPendingOrders
	// run replaces v1's closeT float64 with a pointer of the same size, so the
	// broker layout and the pinned arithmetic offsets after it do not move.
	run *clockRunState
}

// clockRunState is the per-run family state behind clockRangeState.run.
type clockRunState struct {
	// closeT is the resolved clock-close instant of the open position.
	closeT float64
	// gates are the decoded optional gates, installed after broker reset.
	gates dsl.ClockRangeGates
}

func (s *clockRangeState) reset() {
	s.days = s.days[:0]
	s.pending = nil
	s.run = nil
}

// installClockRangeGates gives the next run its decoded gates. gates is nil
// for other families, which then allocate nothing; a clock run without
// installed gates gets v1 behaviour.
func (s *clockRangeState) installClockRangeGates(gates *dsl.ClockRangeGates) {
	if gates != nil {
		s.run = &clockRunState{gates: *gates}
	}
}

// prepareClockRangeGates returns the decoded optional gates of a clock range
// breakout config, or nil for other families and invalid configs (public
// callers validate first; a nil runtime with gate config fails closed).
func prepareClockRangeGates(cfg dsl.Config) *dsl.ClockRangeGates {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyClockRangeBreakout) {
		return nil
	}
	_, gates, err := dsl.DecodeClockRangeBreakoutGates(cfg)
	if err != nil {
		return nil
	}
	return &gates
}

// clockTradingDays is the trading-day OHLC series the optional gates read.
// A trading day is a declared-clock date; a date whose bars span less than
// four hours (such as the weekend reopen) is merged into the next date that
// has bars. key is the clock day the trading day is filed under.
type clockTradingDays struct {
	key        []int64
	o, h, l, c []float64
}

// buildClockTradingDays aggregates every bar of the series. Lookups only read
// trading days filed before the range date, whose bars all precede the range,
// and whether a date is merged depends only on that date's own bars, so
// building the whole series up front reads no future data.
func buildClockTradingDays(schedule clockSchedule, t, o, h, l, c []float64) clockTradingDays {
	var days clockTradingDays
	var carry *[3]float64 // open, high, low of a merged short date
	tf := int64(schedule.timeframe)
	for first := 0; first < len(t); {
		day := schedule.clockDayOf(t[first])
		last := first
		for last+1 < len(t) && schedule.clockDayOf(t[last+1]) == day {
			last++
		}
		open, high, low := o[first], h[first], l[first]
		for j := first + 1; j <= last; j++ {
			high, low = math.Max(high, h[j]), math.Min(low, l[j])
		}
		if carry != nil {
			open, high, low = carry[0], math.Max(high, carry[1]), math.Min(low, carry[2])
			carry = nil
		}
		span := int64(t[last]) + tf - int64(t[first])
		if span < clockTradingDayMinSpanMs && last+1 < len(t) {
			carry = &[3]float64{open, high, low}
		} else {
			days.key = append(days.key, day)
			days.o, days.h, days.l, days.c = append(days.o, open), append(days.h, high), append(days.l, low), append(days.c, c[last])
		}
		first = last + 1
	}
	return days
}

// priorIndex is the last trading day filed before clock day, or -1.
func (d clockTradingDays) priorIndex(day int64) int {
	return sort.Search(len(d.key), func(i int) bool { return d.key[i] >= day }) - 1
}

// atr is the mean true range of the n trading days ending at index i. Each
// true range needs the trading day before it.
func (d clockTradingDays) atr(i, n int) (float64, bool) {
	if i < n {
		return 0, false
	}
	sum := 0.0
	for j := i - n + 1; j <= i; j++ {
		prevClose := d.c[j-1]
		sum += math.Max(d.h[j]-d.l[j], math.Max(math.Abs(d.h[j]-prevClose), math.Abs(d.l[j]-prevClose)))
	}
	return sum / float64(n), true
}

// clockSchedule is local to one run. gates and daily carry the optional
// placement inputs so no broker field changes layout.
type clockSchedule struct {
	spec      dsl.ClockRangeSpec
	offsetMs  int64
	timeframe float64
	gates     dsl.ClockRangeGates
	daily     clockTradingDays
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
	// A config the parser would reject must not trade through a direct API.
	// Explicit zero risk is valid; negative or non-finite risk is not.
	if raw, present := cfg["riskUsd"]; present {
		var risk float64
		switch typed := raw.(type) {
		case int:
			risk = float64(typed)
		case int64:
			risk = float64(typed)
		case float64:
			risk = typed
		default:
			risk = math.NaN()
		}
		if !isFinite(risk) || risk < 0 {
			return fmt.Errorf("clockRangeBreakout: invalid config: riskUsd must be a finite nonnegative number")
		}
	}
	if err := validateSeriesShape("market", series); err != nil {
		return fmt.Errorf("clockRangeBreakout: malformed series: %w", err)
	}
	if err := validateSeriesValues("market", series); err != nil {
		return fmt.Errorf("clockRangeBreakout: malformed series: %w", err)
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

// validateClockRangeFixtureRows checks the fixture's original rows before the
// lossy row-to-bar adapters ran: a short row would otherwise be dropped and a
// long one truncated, and the coverage rule could hide the loss. Typed callers
// that supply only Bars have no rows to check.
func validateClockRangeFixtureRows(cfg dsl.Config, fixture RunFixture) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyClockRangeBreakout) {
		return nil
	}
	if fixture.rawRowDefect != "" {
		return fmt.Errorf("clockRangeBreakout: malformed series: %s", fixture.rawRowDefect)
	}
	if fixture.RawBars == nil {
		return nil
	}
	if len(fixture.RawBars) != len(fixture.Bars) {
		return fmt.Errorf("clockRangeBreakout: malformed series: %d bar rows were reduced to %d bars", len(fixture.RawBars), len(fixture.Bars))
	}
	for i, row := range fixture.RawBars {
		if len(row) != 6 {
			return fmt.Errorf("clockRangeBreakout: malformed series: bar row %d has %d values, want exactly 6", i, len(row))
		}
		for _, value := range row {
			if !isFinite(value) {
				return fmt.Errorf("clockRangeBreakout: malformed series: bar row %d has a non-finite value", i)
			}
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
	if b.clock.run == nil {
		b.clock.run = &clockRunState{}
	}
	gates := b.clock.run.gates
	pipSize, pipOK := dsl.ReviewedInstrumentPip(b.fixture.Symbol)
	schedule, err := newClockSchedule(spec, b.fixture.Timeframe)
	schedule.gates = gates
	n := b.series.Len()
	end := b.executionEnd()
	if end >= n {
		end = n - 1
	}
	// A run that skipped validateClockRangeRun cannot trade: fail closed.
	if err != nil || !pipOK || dsl.ValidateClockRange(spec, gates) != nil || end < 0 {
		return b.trades
	}
	if gates.UsesDailyInputs() {
		schedule.daily = buildClockTradingDays(schedule, b.series.T, b.series.O, b.series.H, b.series.L, b.series.C)
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
			case t >= b.clock.run.closeT:
				b.closeClockRangePosition(b.series.O[i], i, ReasonRule, clockRangeCloseRule, "clock")
				exited = true
			case i > b.position.EntryIndex:
				exited = b.clockRangeExitOnLaterBar(i)
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
		// A range closes when its final bar does: place then, without waiting
		// for another quote. Bars up to i are all that is known.
		for schedule.rangeEnd(nextDay) <= t+schedule.timeframe {
			b.placeClockRangeOrders(schedule, nextDay, i+1, pipSize)
			nextDay++
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
// opened before it. barIndex is the count of bars known at that instant (the
// index of the first bar at or after it, which may not exist yet), so the
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
	if reason := applyClockRangeGates(schedule, day, pending); reason != "" {
		reject(reason)
		return
	}
	needBuy := pending.hasLong || pending.cancelLong
	needSell := pending.hasShort || pending.cancelShort
	if (needBuy && !(isFinite(pending.buy) && pending.buy > 0)) || (needSell && !(isFinite(pending.sell) && pending.sell > 0)) {
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

// applyClockRangeGates applies the optional placement-time gates to a day
// whose range is valid. It returns a rejection reason, or "" when the day
// keeps its orders. Only trading days filed before the range date are read.
func applyClockRangeGates(schedule clockSchedule, day int64, pending *clockPendingOrders) string {
	gates, daily := schedule.gates, schedule.daily
	if !gates.UsesDailyInputs() {
		return ""
	}
	prior := daily.priorIndex(day)
	if gates.WidthATRDays > 0 {
		atr, ok := daily.atr(prior, gates.WidthATRDays)
		if !ok || !isFinite(atr) || !(atr > 0) {
			return ClockReasonNoDailyATR
		}
		if pending.rangeHigh-pending.rangeLow < gates.WidthMinATR*atr {
			return ClockReasonRangeTooNarrow
		}
	}
	if gates.PriorDayCandle {
		if prior < 0 {
			return ClockReasonNoPriorDay
		}
		colour := daily.c[prior] - daily.o[prior]
		if colour == 0 {
			return ClockReasonNoDirection
		}
		pending.hasLong, pending.hasShort = colour > 0, colour < 0
		if gates.CancelOnOpposite {
			pending.cancelLong, pending.cancelShort = colour < 0, colour > 0
		}
	}
	return ""
}

// fillClockRangeOrders tests the pending stop orders against bar i.
func (b *broker) fillClockRangeOrders(schedule clockSchedule, pending *clockPendingOrders, i int) {
	open, high, low := b.series.O[i], b.series.H[i], b.series.L[i]
	longTouched := (pending.hasLong || pending.cancelLong) && high >= pending.buy
	shortTouched := (pending.hasShort || pending.cancelShort) && low <= pending.sell
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
	if (long && pending.cancelLong) || (!long && pending.cancelShort) {
		b.finishClockRangeDay(pending, ClockDayRejected, ClockReasonOppositeTrigger)
		return
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
	gates := schedule.gates
	distance := fill * schedule.spec.StopPercent / 100
	stop := fill - sign*distance
	if gates.RangeEdgeStop() {
		stop = pending.rangeLow
		if s == sideShort {
			stop = pending.rangeHigh
		}
		distance = sign * (fill - stop)
	}
	if !isFinite(distance) || !(distance > 0) || !isFinite(stop) || !(math.Abs(fill-stop) > 0) {
		reject(ClockReasonInvalidStop)
		return
	}
	target := 0.0
	if gates.TargetR > 0 {
		target = fill + sign*gates.TargetR*distance
		if !isFinite(target) || !(target > 0) {
			reject(ClockReasonInvalidStop)
			return
		}
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
		Side: s, SL: stop, TP: target, Size: size, HasSize: true, RiskUSD: b.params.RiskUSD, HasRisk: true, Tag: clockRangeTag,
		Meta: meta, Index: i, NoSlip: true, NoTarget: target == 0,
	}, i)
	if !b.hasPosition {
		reject(ClockReasonOutsideExecWindow)
		return
	}
	b.finishClockRangeDay(pending, ClockDayEntered, "")
	b.clock.days[pending.auditIndex].Side = s.String()
	b.clock.run.closeT = pending.closeT
	// The whole entry bar is tested against the stop, even when its extreme
	// may have come before the fill. The target is not tested on the entry
	// bar: a favourable extreme there may also have come before the fill.
	if pos := b.position; (pos.Side == sideLong && b.series.L[i] <= pos.SL) || (pos.Side == sideShort && b.series.H[i] >= pos.SL) {
		b.closeClockRangePosition(pos.SL, i, "sl", "", "stop")
	}
}

// clockRangeExitOnLaterBar exits at the open when the bar opens through the
// stop and at the stop on an ordinary touch. The stop is tested before the
// target. A target touch, including a bar opening beyond it, fills at the
// target level.
func (b *broker) clockRangeExitOnLaterBar(i int) bool {
	pos := b.position
	if (pos.Side == sideLong && b.series.O[i] <= pos.SL) || (pos.Side == sideShort && b.series.O[i] >= pos.SL) {
		b.closeClockRangePosition(b.series.O[i], i, "sl", "", "stop")
		return true
	}
	if (pos.Side == sideLong && b.series.L[i] <= pos.SL) || (pos.Side == sideShort && b.series.H[i] >= pos.SL) {
		b.closeClockRangePosition(pos.SL, i, "sl", "", "stop")
		return true
	}
	if !pos.NoTarget && ((pos.Side == sideLong && b.series.H[i] >= pos.TP) || (pos.Side == sideShort && b.series.L[i] <= pos.TP)) {
		b.closeClockRangePosition(pos.TP, i, "tp", "", clockRangeTargetExitReason)
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
