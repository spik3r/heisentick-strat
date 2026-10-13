package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// V3 of the market companion adds two owner-approved conventions
// (HT-228, 2026-10-13) to the v2 route and leaves v2 untouched:
//
//   - A hole in the bars is accepted only if a caller-supplied, hashed closure
//     calendar declares exactly that hole. The scheduler then counts observed
//     bars across it: indicators, Setup and Countdown, warm-up, eligibility,
//     order age and holding periods are all index based, so the only change is
//     that the sequential core steps across the hole instead of resetting. A
//     pending decision fills at the first observed open after the hole with the
//     usual slippage, sizing, wrong-side check and bracket priority. Any hole
//     the calendar does not equal refuses the whole run.
//   - A position still open after the last bar stays open. Nothing is closed,
//     and the run is not refused. Its mark is in the equity curve like any other
//     bar; it is not a completed trade.
//
// No candle and no equity mark is invented for a closure.

// SequentialMarketMaxListedClosures bounds the applied-closure list in a result.
const SequentialMarketMaxListedClosures = 256

// SequentialMarketOpenTrade is a position that was still open after the last
// observed bar. All prices are raw broker price units.
type SequentialMarketOpenTrade struct {
	OpportunityID string  `json:"opportunityId"`
	Side          string  `json:"side"`
	EntryIndex    int     `json:"entryIndex"`
	EntryT        float64 `json:"entryT"`
	Entry         float64 `json:"entry"`
	Size          float64 `json:"size"`
	Stop          float64 `json:"stop"`
	Target        float64 `json:"target"`
	// BarsHeld counts observed bars from the entry bar (held bar 1) to the last bar.
	BarsHeld      int     `json:"barsHeld"`
	MarkPrice     float64 `json:"markPrice"`
	UnrealizedPnL float64 `json:"unrealizedPnl"`
	EntryFeePaid  float64 `json:"entryFeePaid"`
}

// SequentialMarketCalendarUse identifies the calendar that was applied.
type SequentialMarketCalendarUse struct {
	CalendarID       string                     `json:"calendarId"`
	Provider         string                     `json:"provider"`
	Symbol           string                     `json:"symbol"`
	Timeframe        string                     `json:"timeframe"`
	Version          string                     `json:"version"`
	SHA256           string                     `json:"sha256"`
	DeclaredClosures int                        `json:"declaredClosures"`
	AppliedCount     int                        `json:"appliedCount"`
	Applied          []SequentialAppliedClosure `json:"applied"`
	AppliedTruncated bool                       `json:"appliedTruncated"`
}

// SequentialMarketV3Run is one admitted execution. Accounting holds a mark for
// every observed bar, completed trades only in Trades, and one extra entry event
// when a position is open at the end.
type SequentialMarketV3Run struct {
	Route                SequentialMarketRoute
	Run                  RunResult
	Accounting           SequentialAccounting
	Open                 *SequentialMarketOpenTrade
	PendingOrderRejected bool
	Calendar             *SequentialMarketCalendarUse
}

func sequentialMarketCalendarUse(c *SequentialClosureCalendar, applied []SequentialAppliedClosure) *SequentialMarketCalendarUse {
	if c == nil {
		return nil
	}
	use := &SequentialMarketCalendarUse{CalendarID: c.CalendarID, Provider: c.Provider, Symbol: c.Symbol, Timeframe: c.Timeframe, Version: c.Version, SHA256: c.SHA256, DeclaredClosures: len(c.Closures), AppliedCount: len(applied), Applied: applied}
	if len(applied) > SequentialMarketMaxListedClosures {
		use.Applied, use.AppliedTruncated = applied[:SequentialMarketMaxListedClosures], true
	}
	return use
}

// firstUndeclaredGap is the v1 refusal for a run with no calendar.
func firstUndeclaredGap(bars []marketdata.Bar, step int64) error {
	for i := 1; i < len(bars); i++ {
		if int64(bars[i].T)-int64(bars[i-1].T) > step {
			return &SequentialFullExecutionError{Kind: "unsupported-sequential-time-gap", Field: "series.timestamp", BarIndex: i}
		}
	}
	return nil
}

// RunSequentialMarketV3 runs one Full E1/E2 execution. calendar may be nil, in
// which case every hole is refused exactly as in v2.
func RunSequentialMarketV3(fixture RunFixture, source string, calendar *SequentialClosureCalendar) (SequentialMarketV3Run, error) {
	route, err := ValidateSequentialMarketFixture(fixture, source)
	if err != nil {
		return SequentialMarketV3Run{}, err
	}
	spec, err := ParseSequentialMarketSource(source)
	if err != nil {
		return SequentialMarketV3Run{}, err
	}
	var after []bool
	var applied []SequentialAppliedClosure
	if calendar != nil {
		if err := calendar.BindRoute(route); err != nil {
			return SequentialMarketV3Run{}, err
		}
		if after, applied, err = calendar.MatchBars(fixture.Bars, route.TimeframeMS); err != nil {
			return SequentialMarketV3Run{}, err
		}
	} else if err := firstUndeclaredGap(fixture.Bars, route.TimeframeMS); err != nil {
		return SequentialMarketV3Run{}, err
	}
	series := marketdata.SeriesFromBars(fixture.Bars)
	series = marketdata.Series{
		T: append([]float64(nil), series.T...), O: append([]float64(nil), series.O...),
		H: append([]float64(nil), series.H...), L: append([]float64(nil), series.L...),
		C: append([]float64(nil), series.C...), V: append([]float64(nil), series.V...),
	}
	scheduling := fixtureFromRequest(RunRequest{StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs})
	s, err := newSequentialFullScheduler(spec, series, scheduling)
	if err != nil {
		return SequentialMarketV3Run{}, err
	}
	s.afterClosure = after
	capture := newSequentialAccountingCollector(scheduling.Costs.StartEquity)
	capture.data.Profile, capture.data.Policy = route.Profile, route.Policy
	s.broker.sequentialAccounting = capture
	for i := 0; i < series.Len(); i++ {
		if err := s.step(i); err != nil {
			return SequentialMarketV3Run{}, err
		}
		capture.mark(&s.broker, i, false)
		if capture.err != nil {
			return SequentialMarketV3Run{}, capture.err
		}
	}
	// A decision on the last bar has no next open. A held position stays open.
	if s.pending >= 0 {
		s.reject(s.pending, SequentialFullNoNextBar)
		s.pending = -1
	}
	trades := append([]Trade{}, s.broker.trades...)
	result, err := checkedResultEnvelope(scheduling, trades)
	if err != nil {
		return SequentialMarketV3Run{}, sequentialAdmissionError("invalid-sequential-derived-value", "trade", -1)
	}
	result.Case = fixture.Case
	result.SequentialFull = &s.audit
	out := SequentialMarketV3Run{Route: route, Run: result, Calendar: sequentialMarketCalendarUse(calendar, applied)}
	for _, o := range s.audit.Opportunities {
		out.PendingOrderRejected = out.PendingOrderRejected || o.Reason == SequentialFullNoNextBar
	}
	events := 2 * len(trades)
	if s.broker.hasPosition {
		last := series.Len() - 1
		pos := s.broker.position
		if capture.open == nil || last < 0 {
			return SequentialMarketV3Run{}, sequentialAccountingError("open position identity", last)
		}
		id, _ := pos.Meta["opportunityId"].(string)
		mark := capture.data.Marks[last]
		out.Open = &SequentialMarketOpenTrade{
			OpportunityID: id, Side: pos.Side.String(), EntryIndex: pos.EntryIndex, EntryT: pos.EntryT, Entry: pos.Entry,
			Size: pos.Size, Stop: pos.SL, Target: pos.TP, BarsHeld: last - pos.EntryIndex + 1, MarkPrice: series.C[last],
			UnrealizedPnL: mark.Unrealized, EntryFeePaid: capture.open.EntryFee,
		}
		for _, v := range []float64{pos.Entry, pos.Size, pos.SL, pos.TP, mark.Unrealized, capture.open.EntryFee} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return SequentialMarketV3Run{}, sequentialAccountingError("open position value", last)
			}
		}
		events++
	} else if capture.open != nil {
		return SequentialMarketV3Run{}, sequentialAccountingError("open position identity", series.Len()-1)
	}
	data := capture.data
	if len(data.Marks) != series.Len() || len(data.Trades) != len(trades) || len(data.Events) != events {
		return SequentialMarketV3Run{}, sequentialAccountingError("terminal counts", series.Len()-1)
	}
	out.Accounting = data
	return out, nil
}

// SequentialMarketTimelineGapV3 adds the calendar verdict to a listed gap.
type SequentialMarketTimelineGapV3 struct {
	SequentialMarketTimeGap
	Declared     bool `json:"declared"`
	ClosureIndex *int `json:"closureIndex,omitempty"`
}

// SequentialMarketTimelineV3 is the bar-spacing scan with calendar coverage.
type SequentialMarketTimelineV3 struct {
	Bars               int                             `json:"bars"`
	GapCount           int                             `json:"gapCount"`
	UndeclaredGapCount int                             `json:"undeclaredGapCount"`
	Gaps               []SequentialMarketTimelineGapV3 `json:"gaps"`
	GapsTruncated      bool                            `json:"gapsTruncated"`
}

// ScanSequentialMarketTimelineV3 lists forward gaps and whether the calendar
// (which may be nil) declares each one exactly. Undeclared gaps are listed
// before declared ones are, so the 32-entry bound cannot hide a refusal cause.
func ScanSequentialMarketTimelineV3(fixture RunFixture, timeframeMS int64, calendar *SequentialClosureCalendar) SequentialMarketTimelineV3 {
	plain := ScanSequentialMarketTimeline(RunFixture{Bars: fixture.Bars}, timeframeMS)
	out := SequentialMarketTimelineV3{Bars: plain.Bars, GapCount: plain.GapCount, Gaps: []SequentialMarketTimelineGapV3{}}
	step := float64(timeframeMS)
	classify := func(i int) (int, bool) {
		if calendar == nil {
			return -1, false
		}
		index := calendar.closureIndexFor(int64(fixture.Bars[i-1].T), int64(fixture.Bars[i].T), timeframeMS)
		return index, index >= 0
	}
	declared := []SequentialMarketTimelineGapV3{}
	for i := 1; i < len(fixture.Bars); i++ {
		previous, current := fixture.Bars[i-1].T, fixture.Bars[i].T
		if !isFinite(previous) || !isFinite(current) || current <= previous+step {
			continue
		}
		gap := SequentialMarketTimeGap{Index: i, PreviousOpenMS: previous, OpenMS: current, DeltaMS: current - previous}
		if math.Mod(gap.DeltaMS, step) == 0 {
			missing := int(gap.DeltaMS/step) - 1
			gap.OnGrid, gap.MissingBars = true, &missing
		}
		entry := SequentialMarketTimelineGapV3{SequentialMarketTimeGap: gap}
		if index, ok := classify(i); ok {
			entry.Declared, entry.ClosureIndex = true, &index
			declared = append(declared, entry)
			continue
		}
		out.UndeclaredGapCount++
		if len(out.Gaps) < SequentialMarketMaxListedGaps {
			out.Gaps = append(out.Gaps, entry)
		} else {
			out.GapsTruncated = true
		}
	}
	for _, entry := range declared {
		if len(out.Gaps) < SequentialMarketMaxListedGaps {
			out.Gaps = append(out.Gaps, entry)
		} else {
			out.GapsTruncated = true
		}
	}
	return out
}
