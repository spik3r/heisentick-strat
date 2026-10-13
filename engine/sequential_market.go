package engine

import (
	"math"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
)

// This file is the v2 "market" companion route for Sequential Full E1/E2. It
// reuses the v1 scheduler, the unchanged Go broker and the v1 accounting
// collector. What it changes is admission only: the source hash pin and the
// SYNTH/5m pin of RunSequentialFixtureWithAccounting are replaced by the
// source's own authored route. Timestamp gaps and a held terminal position are
// still refused with the v1 typed errors; their settlement is an owner decision
// (docs/sequential-market-contract.md).

// SequentialMarketMaxBars bounds one request, as in the v1 companion.
const SequentialMarketMaxBars = sequentialAccountingMaxBars

// SequentialMarketRoute is the route and policy that the authored source fixed.
type SequentialMarketRoute struct {
	Symbol         string  `json:"symbol"`
	Timeframe      string  `json:"timeframe"`
	TimeframeMS    int64   `json:"timeframeMs"`
	Profile        string  `json:"profile"`
	Policy         string  `json:"policy"`
	RiskUSD        float64 `json:"riskUsd"`
	MaxNotionalUSD float64 `json:"maxNotionalUsd"`
}

// SequentialMarketRun is one admitted execution with its raw accounting.
type SequentialMarketRun struct {
	Route      SequentialMarketRoute
	Run        RunResult
	Accounting SequentialAccounting
}

// SequentialMarketTimeGap describes one observed-bar interval longer than one
// timeframe step. The engine does not decide whether it is a closure.
type SequentialMarketTimeGap struct {
	Index          int     `json:"index"`
	PreviousOpenMS float64 `json:"previousOpenMs"`
	OpenMS         float64 `json:"openMs"`
	MissingBars    float64 `json:"missingBars"`
}

// SequentialMarketTimeline is a read-only scan of bar spacing for diagnostics.
// Gaps lists at most SequentialMarketMaxListedGaps entries in time order.
type SequentialMarketTimeline struct {
	Bars          int                       `json:"bars"`
	GapCount      int                       `json:"gapCount"`
	Gaps          []SequentialMarketTimeGap `json:"gaps"`
	GapsTruncated bool                      `json:"gapsTruncated"`
}

const SequentialMarketMaxListedGaps = 32

// ScanSequentialMarketTimeline reports forward gaps only. A duplicate,
// out-of-order or non-integral timestamp is a validation error, not a gap.
func ScanSequentialMarketTimeline(fixture RunFixture, timeframeMS int64) SequentialMarketTimeline {
	timeline := SequentialMarketTimeline{Bars: len(fixture.Bars), Gaps: []SequentialMarketTimeGap{}}
	step := float64(timeframeMS)
	for i := 1; i < len(fixture.Bars); i++ {
		previous, current := fixture.Bars[i-1].T, fixture.Bars[i].T
		if !isFinite(previous) || !isFinite(current) || current <= previous+step {
			continue
		}
		timeline.GapCount++
		if len(timeline.Gaps) == SequentialMarketMaxListedGaps {
			timeline.GapsTruncated = true
			continue
		}
		timeline.Gaps = append(timeline.Gaps, SequentialMarketTimeGap{i, previous, current, math.Round((current-previous)/step) - 1})
	}
	return timeline
}

// ParseSequentialMarketSource decodes the authored route. Only Full E1/E2
// sources are accepted; the generic and legacy families never reach the
// private accounting collector through this entry point.
func ParseSequentialMarketSource(source string) (dsl.SequentialFullSpec, error) {
	parsed, err := dsl.Parse(source)
	if err != nil {
		return dsl.SequentialFullSpec{}, sequentialAccountingError("source.parse", -1)
	}
	if err := dsl.SequentialFullParseError(parsed); err != nil {
		return dsl.SequentialFullSpec{}, err
	}
	if len(parsed.Errors) != 0 {
		return dsl.SequentialFullSpec{}, sequentialAccountingError("source.parse", -1)
	}
	if !dsl.IsSequentialFullReserved(parsed.Config) {
		return dsl.SequentialFullSpec{}, sequentialAccountingError("source.family", -1)
	}
	return dsl.DecodeSequentialFullConfig(parsed.Config)
}

// ValidateSequentialMarketFixture performs every check that does not need to
// execute the strategy. Start equity must be explicit: the omitted/zero to
// 10000 normalization of the v1 and generic routes is not an approved default
// for this surface.
func ValidateSequentialMarketFixture(fixture RunFixture, source string) (SequentialMarketRoute, error) {
	spec, err := ParseSequentialMarketSource(source)
	if err != nil {
		return SequentialMarketRoute{}, err
	}
	step, ok := dsl.SequentialFullTimeframeMS(spec.Timeframe)
	if !ok {
		return SequentialMarketRoute{}, sequentialAccountingError("timeframe", -1)
	}
	route := SequentialMarketRoute{spec.Symbol, spec.Timeframe, step, spec.Profile, spec.Policy, spec.RiskUSD, spec.MaxNotionalUSD}
	if fixture.Schema != runFixtureSchema {
		return route, sequentialAccountingError("schema", -1)
	}
	if strings.TrimSpace(fixture.Case) == "" {
		return route, sequentialAccountingError("case", -1)
	}
	if strings.TrimSpace(fixture.StrategyID) == "" {
		return route, sequentialAccountingError("strategyId", -1)
	}
	if fixture.RangeMethod != "zone" {
		return route, sequentialAccountingError("rangeMethod", -1)
	}
	if len(fixture.Bars) < 1 || len(fixture.Bars) > SequentialMarketMaxBars {
		return route, sequentialAccountingError("bars.count", -1)
	}
	if fixture.Symbol != spec.Symbol || fixture.Timeframe != spec.Timeframe {
		return route, sequentialAccountingError("route", -1)
	}
	if fixture.SourceTimeframe != "" || fixture.HigherTimeframe != "" || fixture.TimedCalendar != nil || len(fixture.SourceBars)+len(fixture.HTFBars)+len(fixture.SourceHTFBars)+len(fixture.RawSourceBars)+len(fixture.RawHTFBars)+len(fixture.RawSourceHTFBars) != 0 {
		return route, sequentialAccountingError("additional source or calendar", -1)
	}
	if !isFinite(fixture.Costs.StartEquity) || fixture.Costs.StartEquity <= 0 {
		return route, sequentialAccountingError("costs.startEquity", -1)
	}
	if err := validateSequentialFullCosts(fixture.Costs); err != nil {
		return route, err
	}
	if fixture.sequentialEnvelopeError != nil {
		return route, fixture.sequentialEnvelopeError
	}
	if fixture.rawRowDefect != "" {
		return route, sequentialAccountingError("bars.raw", -1)
	}
	if fixture.RawBars != nil && len(fixture.RawBars) != len(fixture.Bars) {
		return route, sequentialAccountingError("bars.shape", -1)
	}
	const maxExactMS = float64(1<<53 - 1)
	for i, bar := range fixture.Bars {
		if !isFinite(bar.T) || bar.T < 0 || bar.T != math.Trunc(bar.T) || bar.T > maxExactMS-float64(step) {
			return route, sequentialAccountingError("bars.timestamp", i)
		}
		if !isFinite(bar.O) || !isFinite(bar.H) || !isFinite(bar.L) || !isFinite(bar.C) || !isFinite(bar.V) || bar.V < 0 || bar.H < bar.L || bar.H < bar.O || bar.H < bar.C || bar.L > bar.O || bar.L > bar.C {
			return route, sequentialAccountingError("bars", i)
		}
		// Spacing below one step is a mislabelled timeframe or a bad timestamp,
		// not a hole. Spacing above one step is left to the shared scheduler
		// admission, which refuses it as a typed time gap.
		if i > 0 && bar.T < fixture.Bars[i-1].T+float64(step) {
			return route, sequentialAccountingError("bars.timestamp", i)
		}
		if fixture.RawBars != nil {
			row := fixture.RawBars[i]
			if len(row) != 6 || row[0] != bar.T || row[1] != bar.O || row[2] != bar.H || row[3] != bar.L || row[4] != bar.C || row[5] != bar.V {
				return route, sequentialAccountingError("bars.binding", i)
			}
		}
	}
	return route, nil
}

// RunSequentialMarketFixture runs one Full E1/E2 execution on a caller-authored
// route. It never relaxes the gap or terminal-position refusals of the shared
// scheduler, so a success always has a flat terminal state and contiguous bars.
func RunSequentialMarketFixture(fixture RunFixture, source string) (SequentialMarketRun, error) {
	route, err := ValidateSequentialMarketFixture(fixture, source)
	if err != nil {
		return SequentialMarketRun{}, err
	}
	capture := newSequentialAccountingCollector(fixture.Costs.StartEquity)
	capture.data.Profile, capture.data.Policy = route.Profile, route.Policy
	result, err := runFixtureCase(fixture, source, capture)
	if capture.err != nil {
		return SequentialMarketRun{}, capture.err
	}
	if err != nil {
		return SequentialMarketRun{}, err
	}
	accounting, err := capture.finish(len(fixture.Bars), result.TradeCount)
	if err != nil {
		return SequentialMarketRun{}, err
	}
	if result.SequentialFull == nil || result.SequentialFull.Symbol != route.Symbol || result.SequentialFull.Timeframe != route.Timeframe {
		return SequentialMarketRun{}, sequentialAccountingError("audit route", -1)
	}
	return SequentialMarketRun{Route: route, Run: result, Accounting: accounting}, nil
}
