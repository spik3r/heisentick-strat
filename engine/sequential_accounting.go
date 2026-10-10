package engine

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
)

// SequentialAccounting is raw, unrounded accounting from one admitted run.
// It is separate from the unchanged conformance RunResult serialization. Only
// the frozen Sequential fixture entry point can allocate its private collector.
type SequentialAccounting struct {
	Profile       string                      `json:"profile"`
	Policy        string                      `json:"policy,omitempty"`
	StartEquity   float64                     `json:"startEquity"`
	EndEquity     float64                     `json:"endEquity"`
	FinalRealized float64                     `json:"finalRealized"`
	Marks         []SequentialEquityMark      `json:"marks"`
	Trades        []SequentialTradeAccounting `json:"trades"`
	Events        []SequentialAccountingEvent `json:"events"`
}

// SequentialEquityMark observes the close after that original bar's execution.
// A legacy final liquidation replaces the last mark rather than adding a bar.
type SequentialEquityMark struct {
	Index      int     `json:"index"`
	T          float64 `json:"t"`
	Realized   float64 `json:"realized"`
	Unrealized float64 `json:"unrealized"`
	Equity     float64 `json:"equity"`
}

// SequentialTradeAccounting retains broker values before legacy rounding.
// ExitCredit is the existing broker PnL (already net of the exit fee); NetPnL
// also deducts the nominal entry fee, without subtracting large equity values.
type SequentialTradeAccounting struct {
	TradeIndex int     `json:"tradeIndex"`
	EntryIndex int     `json:"entryIndex"`
	ExitIndex  int     `json:"exitIndex"`
	EntryT     float64 `json:"entryT"`
	ExitT      float64 `json:"exitT"`
	Side       string  `json:"side"`
	Entry      float64 `json:"entry"`
	Exit       float64 `json:"exit"`
	Size       float64 `json:"size"`
	Points     float64 `json:"points"`
	EntryFee   float64 `json:"entryFee"`
	ExitFee    float64 `json:"exitFee"`
	ExitCredit float64 `json:"exitCredit"`
	NetPnL     float64 `json:"netPnl"`
}

// SequentialAccountingEvent records the actual mutation chain. Entry Amount
// is a nominal fee debit; exit Amount is the existing broker's raw exit credit.
// Entry before-minus-Amount is not an exact replay requirement: the original
// broker expression may fuse its fee multiplication and subtraction.
type SequentialAccountingEvent struct {
	Kind           string  `json:"kind"`
	TradeIndex     int     `json:"tradeIndex"`
	Index          int     `json:"index"`
	T              float64 `json:"t"`
	RealizedBefore float64 `json:"realizedBefore"`
	RealizedAfter  float64 `json:"realizedAfter"`
	Amount         float64 `json:"amount"`
}

// SequentialAccountingError refuses invalid or unrepresentable capture values.
// No partial run or accounting is returned by the public companion entry point.
type SequentialAccountingError struct {
	Kind     string `json:"kind"`
	Field    string `json:"field"`
	BarIndex int    `json:"barIndex"`
}

func (e *SequentialAccountingError) Error() string {
	return fmt.Sprintf("%s: %s at bar %d", e.Kind, e.Field, e.BarIndex)
}

func sequentialAccountingError(field string, index int) error {
	return &SequentialAccountingError{"invalid-sequential-accounting", field, index}
}

// RunSequentialFixtureWithAccounting runs exactly one existing Go execution,
// with private observation enabled only for the four source-frozen strategies.
// Generic fixture, column, direct and prepared runs cannot request capture.
func RunSequentialFixtureWithAccounting(fixture RunFixture, source string) (RunResult, SequentialAccounting, error) {
	if err := validateSequentialAccountingFixture(fixture, source); err != nil {
		return RunResult{}, SequentialAccounting{}, err
	}
	capture := newSequentialAccountingCollector(fixture.Costs.normalized().StartEquity)
	switch fixture.StrategyID {
	case "dslSequentialLegacySetup9":
		capture.data.Profile = dsl.LegacySetup9Profile
	case "dslSequentialLegacySetup9PerfSeasonal":
		capture.data.Profile = dsl.LegacySetup9PerfSeasonalProfile
	case "dslSequentialFullE1":
		capture.data.Profile, capture.data.Policy = dsl.SequentialFullProfileID, "E1"
	case "dslSequentialFullE2":
		capture.data.Profile, capture.data.Policy = dsl.SequentialFullProfileID, "E2"
	}
	result, err := runFixtureCase(fixture, source, capture)
	if capture.err != nil {
		return RunResult{}, SequentialAccounting{}, capture.err
	}
	if err != nil {
		return RunResult{}, SequentialAccounting{}, err
	}
	accounting, err := capture.finish(len(fixture.Bars), result.TradeCount)
	if err != nil {
		return RunResult{}, SequentialAccounting{}, err
	}
	return result, accounting, nil
}

const sequentialAccountingMaxBars = 500000

func validateSequentialAccountingFixture(fixture RunFixture, source string) error {
	// The dedicated companion surface is bounded independently of generic
	// fixture admission. Validate before allocating any capture state.
	if fixture.Schema != runFixtureSchema {
		return sequentialAccountingError("schema", -1)
	}
	if strings.TrimSpace(fixture.Case) == "" {
		return sequentialAccountingError("case", -1)
	}
	if fixture.RangeMethod != "zone" {
		return sequentialAccountingError("rangeMethod", -1)
	}
	if len(fixture.Bars) < 1 || len(fixture.Bars) > sequentialAccountingMaxBars {
		return sequentialAccountingError("bars.count", -1)
	}
	var digest, symbol, timeframe, family string
	switch fixture.StrategyID {
	case "dslSequentialLegacySetup9":
		digest = "e2995b9057bc091ce5061bb66d5a06b35c33ebd621a8bef4e00f4496d774dc44"
	case "dslSequentialLegacySetup9PerfSeasonal":
		digest = "f16afb623842d05ef85db179e25138bf75ac1778396a4cb72563333e043658eb"
	case "dslSequentialFullE1":
		digest = "6ffd40ee4ffc3ba5c606a1e23e203757972e5742263a694fcc373de8d92cf8ac"
	case "dslSequentialFullE2":
		digest = "94086f5b62ed6abd89265754572f838e3c024226666d99ceb730f4537e0c8ec2"
	default:
		return sequentialAccountingError("strategyId", -1)
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(source))) != digest {
		return sequentialAccountingError("source.sha256", -1)
	}
	parsed, err := dsl.Parse(source)
	if err != nil || len(parsed.Errors) != 0 {
		return sequentialAccountingError("source.parse", -1)
	}
	stepMS := float64(3600000)
	family = setupTypeFromAny(parsed.Config["setupType"])
	switch family {
	case string(dsl.FamilyLegacySetup9):
		symbol, timeframe = "XAUUSD", "1h"
	case string(dsl.FamilySequentialFull):
		symbol, timeframe = "SYNTH", "5m"
		stepMS = 300000
	default:
		return sequentialAccountingError("source.family", -1)
	}
	if fixture.Symbol != symbol || fixture.Timeframe != timeframe {
		return sequentialAccountingError("route", -1)
	}
	if fixture.SourceTimeframe != "" || fixture.HigherTimeframe != "" || fixture.TimedCalendar != nil || len(fixture.SourceBars)+len(fixture.HTFBars)+len(fixture.SourceHTFBars)+len(fixture.RawSourceBars)+len(fixture.RawHTFBars)+len(fixture.RawSourceHTFBars) != 0 {
		return sequentialAccountingError("additional source or calendar", -1)
	}
	costs := fixture.Costs.normalized()
	if !isFinite(costs.StartEquity) || costs.StartEquity <= 0 {
		return sequentialAccountingError("startEquity", -1)
	}
	for _, cost := range []float64{costs.FeePerUnit, costs.Slippage, costs.SlippageBps} {
		if !isFinite(cost) || cost < 0 {
			return sequentialAccountingError("costs", -1)
		}
	}
	if fixture.sequentialEnvelopeError != nil {
		return fixture.sequentialEnvelopeError
	}
	if fixture.rawRowDefect != "" {
		return sequentialAccountingError("bars.raw", -1)
	}
	if fixture.RawBars != nil && len(fixture.RawBars) != len(fixture.Bars) {
		return sequentialAccountingError("bars.shape", -1)
	}
	const maxExactMS = float64(1<<53 - 1)
	for i, bar := range fixture.Bars {
		if !isFinite(bar.T) || bar.T < 0 || bar.T != math.Trunc(bar.T) || bar.T > maxExactMS-stepMS {
			return sequentialAccountingError("bars.timestamp", i)
		}
		if !isFinite(bar.T) || !isFinite(bar.O) || !isFinite(bar.H) || !isFinite(bar.L) || !isFinite(bar.C) || !isFinite(bar.V) || bar.V < 0 || bar.H < bar.L || bar.H < bar.O || bar.H < bar.C || bar.L > bar.O || bar.L > bar.C {
			return sequentialAccountingError("bars", i)
		}
		if i > 0 && bar.T <= fixture.Bars[i-1].T {
			return sequentialAccountingError("bars.timestamp", i)
		}
		if fixture.RawBars != nil {
			row := fixture.RawBars[i]
			if len(row) != 6 || row[0] != bar.T || row[1] != bar.O || row[2] != bar.H || row[3] != bar.L || row[4] != bar.C || row[5] != bar.V {
				return sequentialAccountingError("bars.binding", i)
			}
		}
	}
	return nil
}

type sequentialAccountingCollector struct {
	data     SequentialAccounting
	open     *SequentialTradeAccounting
	realized float64
	err      error
}

func newSequentialAccountingCollector(start float64) *sequentialAccountingCollector {
	return &sequentialAccountingCollector{data: SequentialAccounting{
		StartEquity: start, EndEquity: start,
		Marks: []SequentialEquityMark{}, Trades: []SequentialTradeAccounting{}, Events: []SequentialAccountingEvent{},
	}}
}

func (c *sequentialAccountingCollector) fail(field string, index int) {
	if c.err == nil {
		c.err = sequentialAccountingError(field, index)
	}
}

func (c *sequentialAccountingCollector) finite(field string, index int, values ...float64) bool {
	for _, value := range values {
		if !isFinite(value) {
			c.fail(field, index)
			return false
		}
	}
	return true
}

func sequentialAccountingIdentical(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b)
}

func (c *sequentialAccountingCollector) mutation(kind string, tradeIndex, index int, t, before, after, amount float64) {
	if !c.finite("mutation", index, t, before, after, amount) {
		return
	}
	if !sequentialAccountingIdentical(before, c.realized) {
		c.fail("realized continuity", index)
	}
	c.data.Events = append(c.data.Events, SequentialAccountingEvent{kind, tradeIndex, index, t, before, after, amount})
	c.realized = after
}

func (c *sequentialAccountingCollector) entry(b *broker, before float64) {
	if c.err != nil {
		return
	}
	pos := b.position
	fee := float64(b.costs.FeePerUnit * pos.Size)
	if c.open != nil || len(b.trades) != len(c.data.Trades) {
		c.fail("entry identity", pos.EntryIndex)
		return
	}
	if !c.finite("entry", pos.EntryIndex, pos.Entry, pos.Size, fee) || pos.Size < 0 {
		c.fail("entry size", pos.EntryIndex)
		return
	}
	c.open = &SequentialTradeAccounting{TradeIndex: len(c.data.Trades), EntryIndex: pos.EntryIndex, EntryT: pos.EntryT, Side: pos.Side.String(), Entry: pos.Entry, Size: pos.Size, EntryFee: fee}
	c.mutation("entry", c.open.TradeIndex, pos.EntryIndex, pos.EntryT, before, b.realized, fee)
}

func (c *sequentialAccountingCollector) exit(b *broker, before, credit float64, index int) {
	if c.err != nil {
		return
	}
	if c.open == nil || len(b.trades) != len(c.data.Trades)+1 {
		c.fail("exit identity", index)
		return
	}
	trade := b.trades[len(b.trades)-1]
	entry := c.open
	if entry.EntryIndex != trade.EntryIndex || !sequentialAccountingIdentical(entry.EntryT, trade.EntryT) || entry.Side != trade.Side || !sequentialAccountingIdentical(entry.Entry, trade.Entry) || !sequentialAccountingIdentical(entry.Size, trade.Size) {
		c.fail("trade identity", index)
		return
	}
	fee := float64(b.costs.FeePerUnit * trade.Size)
	net := float64(credit - entry.EntryFee)
	if !c.finite("exit", index, trade.Exit, trade.Points, fee, credit, net) {
		return
	}
	entry.ExitIndex, entry.ExitT, entry.Exit = trade.ExitIndex, trade.ExitT, trade.Exit
	entry.Points, entry.ExitFee, entry.ExitCredit, entry.NetPnL = trade.Points, fee, credit, net
	c.mutation("exit", entry.TradeIndex, index, trade.ExitT, before, b.realized, credit)
	c.data.Trades = append(c.data.Trades, *entry)
	c.open = nil
}

func (c *sequentialAccountingCollector) mark(b *broker, index int, replace bool) {
	if c.err != nil {
		return
	}
	if index < 0 || index >= b.series.Len() || (!replace && index != len(c.data.Marks)) || (replace && index != len(c.data.Marks)-1) {
		c.fail("mark index", index)
		return
	}
	if !sequentialAccountingIdentical(b.realized, c.realized) {
		c.fail("mark realized continuity", index)
		return
	}
	unrealized := float64(0)
	if b.hasPosition {
		if c.open == nil {
			c.fail("mark position", index)
			return
		}
		// Collector-only arithmetic has explicit binary64 rounding barriers.
		difference := float64(b.series.C[index] - b.position.Entry)
		points := float64(difference * float64(b.position.Side))
		unrealized = float64(points * b.position.Size)
	}
	cash := float64(c.data.StartEquity + b.realized)
	equity := float64(cash + unrealized)
	if !c.finite("mark", index, b.series.T[index], cash, unrealized, equity) {
		return
	}
	mark := SequentialEquityMark{index, b.series.T[index], b.realized, unrealized, equity}
	if replace {
		c.data.Marks[index] = mark
	} else {
		c.data.Marks = append(c.data.Marks, mark)
	}
	c.data.FinalRealized, c.data.EndEquity = b.realized, equity
}

func (c *sequentialAccountingCollector) finish(bars, trades int) (SequentialAccounting, error) {
	if c.err != nil {
		return SequentialAccounting{}, c.err
	}
	if c.open != nil || len(c.data.Marks) != bars || len(c.data.Trades) != trades || len(c.data.Events) != 2*trades {
		return SequentialAccounting{}, sequentialAccountingError("terminal count or position", bars-1)
	}
	end := float64(c.data.StartEquity + c.realized)
	if !isFinite(end) || !sequentialAccountingIdentical(c.realized, c.data.FinalRealized) || !sequentialAccountingIdentical(end, c.data.EndEquity) {
		return SequentialAccounting{}, sequentialAccountingError("terminal equity or realized", bars-1)
	}
	if bars > 0 && c.data.Marks[bars-1].Unrealized != 0 {
		return SequentialAccounting{}, sequentialAccountingError("terminal unrealized", bars-1)
	}
	return c.data, nil
}
