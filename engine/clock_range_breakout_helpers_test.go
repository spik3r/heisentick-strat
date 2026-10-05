package engine

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Shared builders for the clock range breakout tests. Every bar and every
// expected value in the tests is invented and hand-derived in the test that
// uses it; nothing is read back from engine output.

const crbFiveMinutes = 5 * 60 * 1000

// crbOptions describes the strategy text. Zero values give the spec example:
// UTC+10, range 11:05 to 14:05, close 03:00, 1 percent stop, XAUUSD 5m.
type crbOptions struct {
	name    string // strategy name
	desc    string // strategy description
	offset  string // "UTC+10"
	rng     string // "11:05 to 14:05"
	expire  string // "" omits the orders expire line
	closeAt string // "03:00"
	buffer  string // "" omits the buffer line
	stop    string // "1"
	side    string // "" omits the side line
	route   string // "slices(XAUUSD 5m)"
	risk    string // "200"
	extra   string // extra lines appended inside the setup block
}

func (o crbOptions) program() string {
	def := func(value, fallback string) string {
		if value == "" {
			return fallback
		}
		return value
	}
	var b strings.Builder
	fmt.Fprintf(&b, "dsl v7\nstrategy %q {\n  description %q\n}\n", def(o.name, "Clock Range Breakout Test"), def(o.desc, "synthetic test strategy"))
	fmt.Fprintf(&b, "market conditions {\n  %s\n  clock %s\n}\n", def(o.route, "slices(XAUUSD 5m)"), def(o.offset, "UTC+10"))
	fmt.Fprintf(&b, "setup {\n  type: clock range breakout\n  range %s\n", def(o.rng, "11:05 to 14:05"))
	if o.expire != "" {
		fmt.Fprintf(&b, "  orders expire %s\n", o.expire)
	}
	if o.buffer != "" {
		fmt.Fprintf(&b, "  buffer %s pips\n", o.buffer)
	}
	if o.extra != "" {
		b.WriteString("  " + o.extra + "\n")
	}
	b.WriteString("}\n")
	if o.side != "" {
		fmt.Fprintf(&b, "filters {\n  side %s\n}\n", o.side)
	}
	fmt.Fprintf(&b, "risk {\n  stop %s percent\n}\n", def(o.stop, "1"))
	fmt.Fprintf(&b, "management {\n  close positions at %s\n}\n", def(o.closeAt, "03:00"))
	fmt.Fprintf(&b, "execution {\n  risk: %s USD\n}\n", def(o.risk, "200"))
	return b.String()
}

func crbUTC(year int, month time.Month, day, hour, minute int) float64 {
	return float64(time.Date(year, month, day, hour, minute, 0, 0, time.UTC).UnixMilli())
}

// crbBaseRangeEnd is 2026-03-10 04:05Z, the end of the UTC+10 11:05-14:05
// range on local date 2026-03-10.
var (
	crbBaseRangeStart = crbUTC(2026, time.March, 10, 1, 5)
	crbBaseRangeEnd   = crbUTC(2026, time.March, 10, 4, 5)
	crbBaseClose      = crbUTC(2026, time.March, 10, 17, 0)
)

// crbRangeBars returns the 36 five-minute bars opening in [start, start+3h)
// for a range spanning [low, high]. Bar 3 carries the high and bar 7 the low;
// the others sit inside. Slots listed in skip are omitted.
func crbRangeBars(start, low, high float64, skip ...int) []marketdata.Bar {
	return crbRangeBarsN(start, 36, low, high, skip...)
}

func crbRangeBarsN(start float64, count int, low, high float64, skip ...int) []marketdata.Bar {
	skipped := map[int]bool{}
	for _, slot := range skip {
		skipped[slot] = true
	}
	mid := (low + high) / 2
	pad := (high - low) / 10
	var bars []marketdata.Bar
	for slot := 0; slot < count; slot++ {
		if skipped[slot] {
			continue
		}
		bar := marketdata.Bar{T: start + float64(slot)*crbFiveMinutes, O: mid, H: mid + pad, L: mid - pad, C: mid, V: 1}
		switch slot {
		case 3:
			bar.H = high
		case 7:
			bar.L = low
		}
		bars = append(bars, bar)
	}
	return bars
}

func crbBar(t, o, h, l, c float64) marketdata.Bar {
	return marketdata.Bar{T: t, O: o, H: h, L: l, C: c, V: 1}
}

// crbRun compiles source and runs it on bars through the public Run path,
// returning the prepared run so tests can read the per-day outcomes.
func crbRun(t *testing.T, source, symbol, timeframe string, bars []marketdata.Bar, costs Costs) (RunResult, *PreparedRun, error) {
	t.Helper()
	parsed, err := dsl.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("unexpected compile errors: %v", parsed.Errors)
	}
	request := RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars), Symbol: symbol, Timeframe: timeframe, StrategyID: "crb-test", Costs: costs}
	prepared, err := PrepareRun(request)
	if err != nil {
		return RunResult{}, nil, err
	}
	result, err := prepared.RunChecked(costs)
	return result, prepared, err
}

func crbMustRun(t *testing.T, source string, bars []marketdata.Bar, costs Costs) (RunResult, *PreparedRun) {
	t.Helper()
	result, prepared, err := crbRun(t, source, "XAUUSD", "5m", bars, costs)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return result, prepared
}

func crbMeta(t *testing.T, trade Trade) map[string]any {
	t.Helper()
	meta, ok := trade.Meta[clockRangeMetaKey].(map[string]any)
	if !ok {
		t.Fatalf("trade meta lacks %s: %#v", clockRangeMetaKey, trade.Meta)
	}
	return meta
}

// crbWant is the hand-derived expectation for one trade. Unset fields are not
// checked.
type crbWant struct {
	side                 string
	entry, exit          float64
	entryIndex, exitIdx  int
	reason, rule         string
	sl                   float64
	exitReason           string
	checkIndices         bool
	rangeHigh, rangeLow  float64
	rangeBars            int
	dayKey               string
	orderPlacedAt        float64
	checkMeta            bool
	size                 float64
	checkSize            bool
	entryT, exitT        float64
	checkTimes           bool
	skipSL, skipExitCost bool
}

func crbClose(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }

func crbCheckTrade(t *testing.T, label string, got Trade, want crbWant) {
	t.Helper()
	if got.Side != want.side {
		t.Errorf("%s: side = %s, want %s", label, got.Side, want.side)
	}
	if !crbClose(got.Entry, want.entry) {
		t.Errorf("%s: entry = %v, want %v", label, got.Entry, want.entry)
	}
	if !crbClose(got.Exit, want.exit) {
		t.Errorf("%s: exit = %v, want %v", label, got.Exit, want.exit)
	}
	if got.Reason != want.reason || got.Rule != want.rule {
		t.Errorf("%s: reason/rule = %q/%q, want %q/%q", label, got.Reason, got.Rule, want.reason, want.rule)
	}
	if !want.skipSL && !crbClose(got.SL, want.sl) {
		t.Errorf("%s: sl = %v, want %v", label, got.SL, want.sl)
	}
	if !got.NoTarget || got.TP != 0 || got.InitialTP != 0 {
		t.Errorf("%s: target must be absent (NoTarget=%v tp=%v initialTp=%v)", label, got.NoTarget, got.TP, got.InitialTP)
	}
	if got.Tag != clockRangeTag {
		t.Errorf("%s: tag = %q", label, got.Tag)
	}
	if want.checkIndices && (got.EntryIndex != want.entryIndex || got.ExitIndex != want.exitIdx) {
		t.Errorf("%s: indices = %d/%d, want %d/%d", label, got.EntryIndex, got.ExitIndex, want.entryIndex, want.exitIdx)
	}
	if want.checkTimes && (got.EntryT != want.entryT || got.ExitT != want.exitT) {
		t.Errorf("%s: times = %v/%v, want %v/%v", label, got.EntryT, got.ExitT, want.entryT, want.exitT)
	}
	if want.checkSize && !crbClose(got.Size, want.size) {
		t.Errorf("%s: size = %v, want %v", label, got.Size, want.size)
	}
	if want.exitReason != "" || want.checkMeta {
		meta := crbMeta(t, got)
		if want.exitReason != "" && meta["exitReason"] != want.exitReason {
			t.Errorf("%s: exitReason = %v, want %s", label, meta["exitReason"], want.exitReason)
		}
		if want.checkMeta {
			if meta["side"] != want.side || meta["dayKey"] != want.dayKey || meta["rangeBars"] != want.rangeBars {
				t.Errorf("%s: meta = %#v", label, meta)
			}
			if !crbClose(meta["rangeHigh"].(float64), want.rangeHigh) || !crbClose(meta["rangeLow"].(float64), want.rangeLow) || meta["orderPlacedAt"] != want.orderPlacedAt {
				t.Errorf("%s: meta = %#v", label, meta)
			}
		}
	}
}

func crbOnlyTrade(t *testing.T, label string, result RunResult) Trade {
	t.Helper()
	if result.TradeCount != 1 || len(result.Trades) != 1 {
		t.Fatalf("%s: trade count = %d, want exactly one: %+v", label, result.TradeCount, result.Trades)
	}
	return result.Trades[0]
}

func crbDay(t *testing.T, prepared *PreparedRun, index int) ClockRangeDay {
	t.Helper()
	days := prepared.ClockRangeDays()
	if index >= len(days) {
		t.Fatalf("day %d missing; days = %+v", index, days)
	}
	return days[index]
}
