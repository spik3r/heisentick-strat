package engine

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Group 8: numeric tie fixture. Zero buffer and costs, range 99-101, entry-bar
// open 100 / high 102 / low 98. Buy trigger 101, sell trigger 99. Both are
// touched and both are 1.0 from the open, so the tie goes long. The long fills
// at max(101, 100) = 101; the 1% stop is 101 - 1.01 = 99.99. The bar low 98 is
// below the stop, so the whole-entry-bar rule exits at the stop in the same
// bar. One stopped trade; size = risk / 1.01.
func TestClockRangeBreakoutNumericTieFixture(t *testing.T) {
	bars := crbRangeBars(crbBaseRangeStart, 99, 101)
	bars = append(bars,
		crbBar(crbBaseRangeEnd, 100, 102, 98, 100),
		// A later touch of either trigger must not re-arm the used day.
		crbBar(crbBaseRangeEnd+crbFiveMinutes, 100, 103, 97, 100),
	)
	result, prepared := crbMustRun(t, crbOptions{risk: "101"}.program(), bars, Costs{})
	trade := crbOnlyTrade(t, "tie", result)
	crbCheckTrade(t, "tie", trade, crbWant{
		side: "long", entry: 101, exit: 99.99, reason: "sl", sl: 99.99, exitReason: "stop",
		checkIndices: true, entryIndex: 36, exitIdx: 36,
		checkMeta: true, rangeHigh: 101, rangeLow: 99, rangeBars: 36, dayKey: "2026-03-10", orderPlacedAt: crbBaseRangeEnd,
		checkSize: true, size: 100,
		checkTimes: true, entryT: crbBaseRangeEnd, exitT: crbBaseRangeEnd,
	})
	if day := crbDay(t, prepared, 0); day.Outcome != ClockDayEntered || day.Side != "long" || day.DayKey != "2026-03-10" || day.RangeBars != 36 || day.ExpectedBars != 36 {
		t.Errorf("day audit = %+v", day)
	}
}

func crbEntryAt(slot int) float64 { return crbBaseRangeEnd + float64(slot)*crbFiveMinutes }

// crbSession is the base range 99-101 followed by the given bars.
func crbSession(after ...marketdata.Bar) []marketdata.Bar {
	return append(crbRangeBars(crbBaseRangeStart, 99, 101), after...)
}

// Group 5: fills. Range 99-101, zero buffer and costs, 1% stop. Each case is
// derived by hand from the fill rules: a long fills at max(101, open), a short
// at min(99, open), and the stop is 1% of the fill on the losing side.
func TestClockRangeBreakoutFills(t *testing.T) {
	closeBar := crbBar(crbBaseClose, 105, 106, 104, 105)
	tests := []struct {
		name  string
		entry marketdata.Bar
		want  crbWant
	}{
		{"ordinary long", crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101), crbWant{side: "long", entry: 101, sl: 99.99}},
		{"gap-through long", crbBar(crbBaseRangeEnd, 102, 103, 101.5, 102.5), crbWant{side: "long", entry: 102, sl: 100.98}},
		{"ordinary short", crbBar(crbBaseRangeEnd, 99.5, 99.8, 98.5, 99), crbWant{side: "short", entry: 99, sl: 99.99}},
		{"gap-through short", crbBar(crbBaseRangeEnd, 98, 98.5, 97, 97.5), crbWant{side: "short", entry: 98, sl: 98.98}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, _ := crbMustRun(t, crbOptions{}.program(), crbSession(tc.entry, closeBar), Costs{})
			trade := crbOnlyTrade(t, tc.name, result)
			want := tc.want
			want.exit, want.reason, want.rule, want.exitReason = 105, "rule", "clock-range-close", "clock"
			want.checkIndices, want.entryIndex, want.exitIdx = true, 36, 37
			crbCheckTrade(t, tc.name, trade, want)
		})
	}
}

// Both triggers touched with unequal distances: the trigger nearer the open
// wins. Open 100.2: |100.2-101| = 0.8 < |100.2-99| = 1.2, so long. Open 99.7:
// 1.3 > 0.7, so short. Each fills and then stops in its own bar because the
// bar spans both triggers and its 1% stop.
func TestClockRangeBreakoutBothTouchedNearestTriggerWins(t *testing.T) {
	long, _ := crbMustRun(t, crbOptions{}.program(), crbSession(crbBar(crbBaseRangeEnd, 100.2, 102, 98, 100)), Costs{})
	crbCheckTrade(t, "nearer long", crbOnlyTrade(t, "nearer long", long), crbWant{
		side: "long", entry: 101, exit: 99.99, reason: "sl", sl: 99.99, exitReason: "stop"})
	short, _ := crbMustRun(t, crbOptions{}.program(), crbSession(crbBar(crbBaseRangeEnd, 99.7, 102, 98, 100)), Costs{})
	crbCheckTrade(t, "nearer short", crbOnlyTrade(t, "nearer short", short), crbWant{
		side: "short", entry: 99, exit: 99.99, reason: "sl", sl: 99.99, exitReason: "stop"})
}

// The first side to fill cancels the other order, and nothing re-arms the day
// after the stop. Long fills at 101 on bar 36 (no stop yet: low 100.2 > 99.99).
// Bar 37 trades down through 99 and the stop: low 98.5 <= 99.99, open 101 is
// above the stop, so an ordinary touch exits at 99.99. Bar 38 touches both
// triggers again and must not trade.
func TestClockRangeBreakoutFirstSideCancelsOtherAndDayIsUsed(t *testing.T) {
	bars := crbSession(
		crbBar(crbEntryAt(0), 100.5, 101.5, 100.2, 101),
		crbBar(crbEntryAt(1), 101, 101.2, 98.5, 99),
		crbBar(crbEntryAt(2), 100, 105, 95, 100),
	)
	result, prepared := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
	trade := crbOnlyTrade(t, "oco", result)
	crbCheckTrade(t, "oco", trade, crbWant{side: "long", entry: 101, exit: 99.99, reason: "sl", sl: 99.99,
		exitReason: "stop", checkIndices: true, entryIndex: 36, exitIdx: 37})
	if day := crbDay(t, prepared, 0); day.Outcome != ClockDayEntered {
		t.Errorf("day = %+v", day)
	}
}

// Static side allowance: only an allowed side receives an order.
func TestClockRangeBreakoutSideAllowance(t *testing.T) {
	closeBar := crbBar(crbBaseClose, 100, 100.4, 99.6, 100)
	sellOnly := crbBar(crbBaseRangeEnd, 99.5, 99.8, 98.5, 99)
	buyOnly := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101)
	tests := []struct {
		name     string
		side     string
		entry    marketdata.Bar
		wantSide string
	}{
		{"long only ignores a sell touch", "long only", sellOnly, ""},
		{"short only ignores a buy touch", "short only", buyOnly, ""},
		{"long only takes a buy touch", "long only", buyOnly, "long"},
		{"short only takes a sell touch", "short only", sellOnly, "short"},
		{"both takes a buy touch", "both", buyOnly, "long"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, prepared := crbMustRun(t, crbOptions{side: tc.side}.program(), crbSession(tc.entry, closeBar), Costs{})
			if tc.wantSide == "" {
				if result.TradeCount != 0 {
					t.Fatalf("trades = %+v", result.Trades)
				}
				if day := crbDay(t, prepared, 0); day.Outcome != ClockDayExpired {
					t.Errorf("day = %+v, want expired at the close bar", day)
				}
				return
			}
			if trade := crbOnlyTrade(t, tc.name, result); trade.Side != tc.wantSide {
				t.Errorf("side = %s, want %s", trade.Side, tc.wantSide)
			}
		})
	}
}

// Expiry boundary. `orders expire 15:00` on UTC+10 is 05:00Z. A bar opening at
// 04:55Z is inside [rangeEnd, expiry) and may fill; a bar opening at 05:00Z is
// not. Neither a later touch nor a retry may trade after expiry.
func TestClockRangeBreakoutExpiryBoundaryExcludesExpiryBar(t *testing.T) {
	program := crbOptions{expire: "15:00"}.program()
	quiet := crbBar(crbBaseRangeEnd, 100, 100.4, 99.6, 100)
	t.Run("last bar before expiry fills", func(t *testing.T) {
		bars := crbSession(quiet, crbBar(crbUTC(2026, time.March, 10, 4, 55), 100, 101.5, 100, 101), crbBar(crbBaseClose, 102, 103, 101, 102))
		result, _ := crbMustRun(t, program, bars, Costs{})
		crbCheckTrade(t, "04:55", crbOnlyTrade(t, "04:55", result), crbWant{side: "long", entry: 101, exit: 102,
			reason: "rule", rule: "clock-range-close", sl: 99.99, checkIndices: true, entryIndex: 37, exitIdx: 38})
	})
	t.Run("expiry bar does not fill and nothing retries", func(t *testing.T) {
		bars := crbSession(quiet,
			crbBar(crbUTC(2026, time.March, 10, 5, 0), 100, 101.5, 100, 101),
			crbBar(crbUTC(2026, time.March, 10, 5, 5), 100, 105, 95, 100),
			crbBar(crbBaseClose, 102, 103, 101, 102))
		result, prepared := crbMustRun(t, program, bars, Costs{})
		if result.TradeCount != 0 {
			t.Fatalf("trades = %+v", result.Trades)
		}
		if day := crbDay(t, prepared, 0); day.Outcome != ClockDayExpired {
			t.Errorf("day = %+v", day)
		}
	})
}

// Close and expiry are processed at a bar open before entry tests: with the
// default expiry equal to the close, a bar at the close instant that spans a
// trigger still cannot enter.
func TestClockRangeBreakoutCloseInstantBarCannotEnter(t *testing.T) {
	bars := crbSession(crbBar(crbBaseRangeEnd, 100, 100.4, 99.6, 100), crbBar(crbBaseClose, 100, 105, 95, 100))
	result, prepared := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
	if result.TradeCount != 0 {
		t.Fatalf("trades = %+v", result.Trades)
	}
	if day := crbDay(t, prepared, 0); day.Outcome != ClockDayExpired {
		t.Errorf("day = %+v", day)
	}
}

// Group 6: exit ordering after an entry on bar 36 (long at 101, stop 99.99).
func TestClockRangeBreakoutLaterBarStopsAndClockPrecedence(t *testing.T) {
	enterLong := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101)
	enterShort := crbBar(crbBaseRangeEnd, 99.5, 99.8, 98.5, 99)
	tests := []struct {
		name string
		bars []marketdata.Bar
		want crbWant
	}{
		{"long gaps below the stop and exits at the open",
			[]marketdata.Bar{enterLong, crbBar(crbEntryAt(1), 99.5, 99.8, 99, 99.2)},
			crbWant{side: "long", entry: 101, exit: 99.5, reason: "sl", sl: 99.99, exitReason: "stop"}},
		{"short gaps above the stop and exits at the open",
			[]marketdata.Bar{enterShort, crbBar(crbEntryAt(1), 100.5, 101, 100, 100.8)},
			crbWant{side: "short", entry: 99, exit: 100.5, reason: "sl", sl: 99.99, exitReason: "stop"}},
		{"long ordinary touch exits at the stop",
			[]marketdata.Bar{enterLong, crbBar(crbEntryAt(1), 101, 101.2, 99.9, 100)},
			crbWant{side: "long", entry: 101, exit: 99.99, reason: "sl", sl: 99.99, exitReason: "stop"}},
		{"long: the clock close beats that bar's low",
			[]marketdata.Bar{enterLong, crbBar(crbBaseClose, 101.5, 120, 50, 100)},
			crbWant{side: "long", entry: 101, exit: 101.5, reason: "rule", rule: "clock-range-close", sl: 99.99, exitReason: "clock"}},
		{"short: the clock close beats that bar's high",
			[]marketdata.Bar{enterShort, crbBar(crbBaseClose, 98.5, 120, 50, 100)},
			crbWant{side: "short", entry: 99, exit: 98.5, reason: "rule", rule: "clock-range-close", sl: 99.99, exitReason: "clock"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, _ := crbMustRun(t, crbOptions{}.program(), crbSession(tc.bars...), Costs{})
			trade := crbOnlyTrade(t, tc.name, result)
			want := tc.want
			want.checkIndices, want.entryIndex, want.exitIdx = true, 36, 37
			crbCheckTrade(t, tc.name, trade, want)
		})
	}
}

// Group 7: a missing clock-close quote delays the exit to the next real open;
// with no later quote, finalization closes at the last close; prefixes keep
// the open state without liquidating.
func TestClockRangeBreakoutMissingCloseQuoteAndFinalization(t *testing.T) {
	enter := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 100.9)
	t.Run("next available open closes", func(t *testing.T) {
		late := crbBar(crbUTC(2026, time.March, 10, 17, 35), 100.8, 130, 50, 100)
		result, _ := crbMustRun(t, crbOptions{}.program(), crbSession(enter, late), Costs{})
		crbCheckTrade(t, "late", crbOnlyTrade(t, "late", result), crbWant{side: "long", entry: 101, exit: 100.8,
			reason: "rule", rule: "clock-range-close", sl: 99.99, exitReason: "clock",
			checkTimes: true, entryT: crbBaseRangeEnd, exitT: late.T})
	})
	t.Run("no later quote gives one terminal trade at the last close", func(t *testing.T) {
		result, _ := crbMustRun(t, crbOptions{}.program(), crbSession(enter), Costs{})
		trade := crbOnlyTrade(t, "terminal", result)
		crbCheckTrade(t, "terminal", trade, crbWant{side: "long", entry: 101, exit: 100.9, reason: "end-of-test", sl: 99.99,
			exitReason: "end-of-data", checkIndices: true, entryIndex: 36, exitIdx: 36})
	})
}

// Group 4: coverage. 36 expected five-minute slots need ceil(0.9*36) = 33
// distinct slots including the last one before range end. The entry bar at
// range end would fill at 101 whenever an order exists.
func TestClockRangeBreakoutCoverage(t *testing.T) {
	entry := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101)
	closeBar := crbBar(crbBaseClose, 102, 103, 101, 102)
	tests := []struct {
		name       string
		skip       []int
		wantTrade  bool
		wantReason string
		wantBars   int
	}{
		{"33 slots including the final slot", []int{1, 2, 4}, true, "", 33},
		{"32 slots", []int{1, 2, 4, 5}, false, ClockReasonInsufficient, 32},
		{"35 slots without the final slot", []int{35}, false, ClockReasonMissingFinalSlot, 35},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bars := append(crbRangeBars(crbBaseRangeStart, 99, 101, tc.skip...), entry, closeBar)
			result, prepared := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
			day := crbDay(t, prepared, 0)
			if day.ExpectedBars != 36 || day.RangeBars != tc.wantBars {
				t.Errorf("coverage = %d/%d, want %d/36", day.RangeBars, day.ExpectedBars, tc.wantBars)
			}
			if tc.wantTrade {
				crbOnlyTrade(t, tc.name, result)
				return
			}
			if result.TradeCount != 0 || day.Outcome != ClockDayRejected || day.Reason != tc.wantReason {
				t.Errorf("trades=%d day=%+v, want rejected %s", result.TradeCount, day, tc.wantReason)
			}
		})
	}
}

func TestClockRangeBreakoutEmptyAndFlatRangesAreRejected(t *testing.T) {
	entry := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101)
	t.Run("no range bars", func(t *testing.T) {
		before := crbBar(crbUTC(2026, time.March, 9, 12, 0), 100, 100.1, 99.9, 100)
		result, prepared := crbMustRun(t, crbOptions{}.program(), []marketdata.Bar{before, entry}, Costs{})
		if day := crbDay(t, prepared, 0); result.TradeCount != 0 || day.Reason != ClockReasonEmptyRange || day.RangeBars != 0 {
			t.Errorf("trades=%d day=%+v", result.TradeCount, day)
		}
	})
	t.Run("flat range", func(t *testing.T) {
		var bars []marketdata.Bar
		for slot := 0; slot < 36; slot++ {
			bars = append(bars, crbBar(crbBaseRangeStart+float64(slot)*crbFiveMinutes, 100, 100, 100, 100))
		}
		result, prepared := crbMustRun(t, crbOptions{}.program(), append(bars, entry), Costs{})
		if day := crbDay(t, prepared, 0); result.TradeCount != 0 || day.Reason != ClockReasonNonpositiveWidth {
			t.Errorf("trades=%d day=%+v", result.TradeCount, day)
		}
	})
}

// The bar opening at range end is not part of the range: its high 105 must not
// move the buy trigger, which stays 101.
func TestClockRangeBreakoutRangeEndBarIsExcludedFromRangeButCanFill(t *testing.T) {
	bars := crbSession(crbBar(crbBaseRangeEnd, 100.5, 105, 100.2, 104), crbBar(crbBaseClose, 104, 105, 103, 104))
	result, _ := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
	crbCheckTrade(t, "range end bar", crbOnlyTrade(t, "range end bar", result), crbWant{
		side: "long", entry: 101, exit: 104, reason: "rule", rule: "clock-range-close", sl: 99.99,
		checkMeta: true, rangeHigh: 101, rangeLow: 99, rangeBars: 36, dayKey: "2026-03-10", orderPlacedAt: crbBaseRangeEnd,
		checkIndices: true, entryIndex: 36, exitIdx: 37})
}

// Malformed series fail closed with a family error; they cannot be counted as
// coverage or repaired.
func TestClockRangeBreakoutRejectsMalformedSeries(t *testing.T) {
	entry := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101)
	good := append(crbRangeBars(crbBaseRangeStart, 99, 101), entry)
	with := func(mutate func([]marketdata.Bar) []marketdata.Bar) []marketdata.Bar {
		return mutate(append([]marketdata.Bar(nil), good...))
	}
	tests := []struct {
		name string
		bars []marketdata.Bar
		want string
	}{
		{"duplicate timestamp", with(func(b []marketdata.Bar) []marketdata.Bar { b[10] = b[9]; return b }), "strictly increasing"},
		{"six duplicates pad a short range to 36 rows", with(func(b []marketdata.Bar) []marketdata.Bar {
			for i := 10; i < 16; i++ {
				b[i] = b[9]
			}
			return b
		}), "strictly increasing"},
		{"unordered", with(func(b []marketdata.Bar) []marketdata.Bar { b[4], b[5] = b[5], b[4]; return b }), "strictly increasing"},
		{"off-grid timestamp", with(func(b []marketdata.Bar) []marketdata.Bar { b[12].T += 1000; return b }), "not aligned"},
		{"high below open", with(func(b []marketdata.Bar) []marketdata.Bar { b[20].H = b[20].O - 0.5; return b }), "OHLC"},
		{"low above close", with(func(b []marketdata.Bar) []marketdata.Bar { b[21].L = b[21].C + 0.5; return b }), "OHLC"},
		{"negative timestamp", []marketdata.Bar{crbBar(-300000, 1, 2, 0.5, 1)}, "outside the supported range"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := crbRun(t, crbOptions{}.program(), "XAUUSD", "5m", tc.bars, Costs{})
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "malformed series") {
				t.Fatalf("err = %v, want malformed series containing %q", err, tc.want)
			}
		})
	}
}

// Existing families keep their input contract: the same duplicate timestamps
// that this family rejects still run, without a new error, for another family.
func TestOtherFamiliesKeepTheirSeriesContract(t *testing.T) {
	source := "dsl v7\nstrategy \"orb\" {\n}\nmarket conditions {\n  slices(XAUUSD 5m)\n}\nsetup {\n  type: opening range breakout\n}\n"
	bars := crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101))
	bars[10] = bars[9]
	if _, _, err := crbRun(t, source, "XAUUSD", "5m", bars, Costs{}); err != nil {
		t.Fatalf("opening range breakout rejected duplicate timestamps: %v", err)
	}
}

// The runtime route must have a reviewed pip size even when the buffer is
// zero, and a timeframe the schedule cannot sit on is refused.
func TestClockRangeBreakoutFailsClosedOnUnknownRuntimeRoute(t *testing.T) {
	bars := crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101))
	source := crbOptions{route: "symbols XAUUSD\n  timeframes 5m 1m"}.program()
	if _, _, err := crbRun(t, source, "FOOUSD", "5m", bars, Costs{}); err == nil || !strings.Contains(err.Error(), "no reviewed pip size") {
		t.Errorf("unknown symbol: err = %v", err)
	}
	if _, _, err := crbRun(t, source, "", "5m", bars, Costs{}); err == nil || !strings.Contains(err.Error(), "no reviewed pip size") {
		t.Errorf("blank symbol: err = %v", err)
	}
	if _, _, err := crbRun(t, source, "XAUUSD", "7m", bars, Costs{}); err == nil {
		t.Error("unsupported timeframe accepted")
	}
}

// Group 1: the schedule is fixed-offset arithmetic on UTC instants. Each case
// describes one range day by its UTC range start; the range runs three hours,
// a long fills at 101 on the bar at range end, and the clock close at the
// given UTC instant exits at that bar's open, 102. dayKey is the date on the
// declared clock at range start.
func TestClockRangeBreakoutClockDatesAndOffsets(t *testing.T) {
	tests := []struct {
		name     string
		opts     crbOptions
		start    float64
		closeAt  float64
		wantDay  string
		wantSize float64
	}{
		{"UTC+10 baseline", crbOptions{}, crbUTC(2026, time.March, 10, 1, 5), crbUTC(2026, time.March, 10, 17, 0), "2026-03-10", 0},
		{"UTC+2 same instants", crbOptions{offset: "UTC+2", rng: "03:05 to 06:05", closeAt: "19:00"}, crbUTC(2026, time.March, 10, 1, 5), crbUTC(2026, time.March, 10, 17, 0), "2026-03-10", 0},
		{"UTC+0 same instants", crbOptions{offset: "UTC+0", rng: "01:05 to 04:05", closeAt: "17:00"}, crbUTC(2026, time.March, 10, 1, 5), crbUTC(2026, time.March, 10, 17, 0), "2026-03-10", 0},
		{"UTC-5 clock date is the previous day", crbOptions{offset: "UTC-5", rng: "20:05 to 23:05", closeAt: "12:00"}, crbUTC(2026, time.March, 10, 1, 5), crbUTC(2026, time.March, 10, 17, 0), "2026-03-09", 0},
		{"negative fractional UTC-3:30", crbOptions{offset: "UTC-3:30", rng: "09:05 to 12:05", closeAt: "20:00"}, crbUTC(2026, time.March, 10, 12, 35), crbUTC(2026, time.March, 10, 23, 30), "2026-03-10", 0},
		{"positive fractional UTC+5:30", crbOptions{offset: "UTC+5:30", rng: "08:05 to 11:05", closeAt: "22:00"}, crbUTC(2026, time.March, 10, 2, 35), crbUTC(2026, time.March, 10, 16, 30), "2026-03-10", 0},
		{"range crosses UTC midnight", crbOptions{rng: "08:00 to 11:00", closeAt: "20:00"}, crbUTC(2026, time.March, 9, 22, 0), crbUTC(2026, time.March, 10, 10, 0), "2026-03-10", 0},
		{"year end", crbOptions{}, crbUTC(2026, time.December, 31, 1, 5), crbUTC(2026, time.December, 31, 17, 0), "2026-12-31", 0},
		{"clock date before UTC year start", crbOptions{offset: "UTC-8", rng: "19:05 to 22:05", closeAt: "10:00"}, crbUTC(2026, time.January, 1, 3, 5), crbUTC(2026, time.January, 1, 18, 0), "2025-12-31", 0},
		{"month end", crbOptions{}, crbUTC(2026, time.February, 28, 1, 5), crbUTC(2026, time.February, 28, 17, 0), "2026-02-28", 0},
		{"leap day", crbOptions{}, crbUTC(2028, time.February, 29, 1, 5), crbUTC(2028, time.February, 29, 17, 0), "2028-02-29", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			end := tc.start + 36*crbFiveMinutes
			bars := append(crbRangeBars(tc.start, 99, 101),
				crbBar(end, 100.5, 101.5, 100.2, 101),
				crbBar(tc.closeAt, 102, 103, 101, 102))
			result, _ := crbMustRun(t, tc.opts.program(), bars, Costs{})
			crbCheckTrade(t, tc.name, crbOnlyTrade(t, tc.name, result), crbWant{
				side: "long", entry: 101, exit: 102, reason: "rule", rule: "clock-range-close", sl: 99.99, exitReason: "clock",
				checkMeta: true, rangeHigh: 101, rangeLow: 99, rangeBars: 36, dayKey: tc.wantDay, orderPlacedAt: end,
				checkTimes: true, entryT: end, exitT: tc.closeAt, checkIndices: true, entryIndex: 36, exitIdx: 37})
		})
	}
}

// UTC+10 and UTC+2 spellings of the same instants give identical trades apart
// from nothing: both resolve to local date 2026-03-10 here, so the whole trade
// record, metadata included, must match.
func TestClockRangeBreakoutEquivalentOffsetsGiveIdenticalTrades(t *testing.T) {
	bars := crbSession(crbBar(crbBaseRangeEnd, 99.7, 102, 98, 100))
	a, _ := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
	b, _ := crbMustRun(t, crbOptions{offset: "UTC+2", rng: "03:05 to 06:05", closeAt: "19:00"}.program(), bars, Costs{})
	if !reflect.DeepEqual(a.Trades, b.Trades) || a.TradeCount != 1 {
		t.Fatalf("UTC+10: %+v\nUTC+2: %+v", a.Trades, b.Trades)
	}
}

// Several range days in one series: each day is evaluated on its own clock
// date. Day 1 (03-10) goes long and closes at the close bar; day 2 (03-11)
// short-only fills are not requested, so the default both-sides pair applies
// and its sell trigger fills; day 3 has only 30 range bars and is rejected.
func TestClockRangeBreakoutMultipleDays(t *testing.T) {
	day := 24 * 60 * 60 * 1000.0
	var bars []marketdata.Bar
	bars = append(bars, crbRangeBars(crbBaseRangeStart, 99, 101)...)
	bars = append(bars, crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose, 102, 103, 101, 102))
	bars = append(bars, crbRangeBars(crbBaseRangeStart+day, 99, 101)...)
	bars = append(bars, crbBar(crbBaseRangeEnd+day, 99.5, 99.8, 98.5, 99), crbBar(crbBaseClose+day, 98, 99, 97, 98))
	bars = append(bars, crbRangeBars(crbBaseRangeStart+2*day, 99, 101, 1, 2, 4, 5, 6, 8)...)
	bars = append(bars, crbBar(crbBaseRangeEnd+2*day, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose+2*day, 102, 103, 101, 102))
	result, prepared := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
	if result.TradeCount != 2 {
		t.Fatalf("trades = %+v", result.Trades)
	}
	crbCheckTrade(t, "day 1", result.Trades[0], crbWant{side: "long", entry: 101, exit: 102, reason: "rule", rule: "clock-range-close", sl: 99.99,
		checkMeta: true, rangeHigh: 101, rangeLow: 99, rangeBars: 36, dayKey: "2026-03-10", orderPlacedAt: crbBaseRangeEnd})
	crbCheckTrade(t, "day 2", result.Trades[1], crbWant{side: "short", entry: 99, exit: 98, reason: "rule", rule: "clock-range-close", sl: 99.99,
		checkMeta: true, rangeHigh: 101, rangeLow: 99, rangeBars: 36, dayKey: "2026-03-11", orderPlacedAt: crbBaseRangeEnd + day})
	days := prepared.ClockRangeDays()
	if len(days) != 3 || days[0].Outcome != ClockDayEntered || days[1].Outcome != ClockDayEntered ||
		days[2].Outcome != ClockDayRejected || days[2].Reason != ClockReasonInsufficient || days[2].RangeBars != 30 || days[2].DayKey != "2026-03-12" {
		t.Errorf("days = %+v", days)
	}
}

// A placement while a position is open skips the day. With a valid next-day
// range the due clock close always lands before the final range slot, so the
// guard cannot be reached through data alone; exercise it directly.
func TestClockRangeBreakoutPlacementIsBlockedByAnOpenPosition(t *testing.T) {
	bars := crbSession(crbBar(crbBaseRangeEnd, 100, 100.4, 99.6, 100))
	parsed, err := dsl.Parse(crbOptions{}.program())
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	spec, err := dsl.DecodeClockRangeBreakout(parsed.Config)
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := newClockSchedule(spec, "5m")
	if err != nil {
		t.Fatal(err)
	}
	var b broker
	b.reset(marketdata.SeriesFromBars(bars), contextcols.Columns{}, nil, nil, nil, paramsFromConfig(parsed.Config), RunFixture{Symbol: "XAUUSD", Timeframe: "5m"}, nil)
	b.hasPosition = true
	b.placeClockRangeOrders(schedule, schedule.clockDayOf(crbBaseRangeStart), 36, 0.1)
	if len(b.clock.days) != 1 || b.clock.days[0].Reason != ClockReasonBrokerBlocked || b.clock.pending != nil {
		t.Fatalf("days = %+v pending = %v", b.clock.days, b.clock.pending)
	}
}

// An allowed-side trigger that is not positive rejects the whole day instead of
// placing only the other side. Range 0.5-3 with a 10-pip buffer on XAUUSD
// (1.0 price unit) puts the sell trigger at -0.5.
func TestClockRangeBreakoutNonpositiveTriggerRejectsTheDay(t *testing.T) {
	bars := append(crbRangeBars(crbBaseRangeStart, 0.5, 3), crbBar(crbBaseRangeEnd, 2, 5, 1.5, 4), crbBar(crbBaseClose, 4, 5, 3, 4))
	result, prepared := crbMustRun(t, crbOptions{buffer: "10"}.program(), bars, Costs{})
	if day := crbDay(t, prepared, 0); result.TradeCount != 0 || day.Outcome != ClockDayRejected || day.Reason != ClockReasonInvalidPrice {
		t.Errorf("trades=%d day=%+v", result.TradeCount, day)
	}
	// A long-only run has no sell order, so the same range is valid.
	result, _ = crbMustRun(t, crbOptions{buffer: "10", side: "long only"}.program(), bars, Costs{})
	if result.TradeCount != 1 || result.Trades[0].Side != "long" || !crbClose(result.Trades[0].Entry, 4) {
		t.Errorf("long-only trades = %+v", result.Trades)
	}
}

// Group 9: money. Pips convert through the reviewed registry; slippage moves
// the fill, the percent stop and the size; zero risk stays zero; no inherited
// ATR, target, breakeven or grade setting reaches the trade.
func TestClockRangeBreakoutPipBufferConversion(t *testing.T) {
	// XAUUSD: 10 pips = 1.0, so with range 99-101 the buy trigger is 102.
	touch101 := crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose, 100.5, 101, 100, 100.5))
	if result, _ := crbMustRun(t, crbOptions{buffer: "10", side: "long only"}.program(), touch101, Costs{}); result.TradeCount != 0 {
		t.Fatalf("high 101.5 must not reach a 102 trigger: %+v", result.Trades)
	}
	touch102 := crbSession(crbBar(crbBaseRangeEnd, 101.5, 102.2, 101.2, 102), crbBar(crbBaseClose, 103, 104, 102, 103))
	result, _ := crbMustRun(t, crbOptions{buffer: "10", side: "long only"}.program(), touch102, Costs{})
	crbCheckTrade(t, "xau buffer", crbOnlyTrade(t, "xau buffer", result), crbWant{side: "long", entry: 102, exit: 103, reason: "rule", rule: "clock-range-close", sl: 100.98})

	// EURUSD: 15 pips = 0.0015. Range 1.0990-1.1010, sell trigger 1.0975.
	eur := append(crbRangeBars(crbBaseRangeStart, 1.0990, 1.1010), crbBar(crbBaseRangeEnd, 1.0985, 1.0988, 1.0970, 1.0972), crbBar(crbBaseClose, 1.0960, 1.0970, 1.0950, 1.0960))
	result, _, err := crbRun(t, crbOptions{buffer: "15", side: "short only", route: "slices(EURUSD 5m)"}.program(), "EURUSD", "5m", eur, Costs{})
	if err != nil {
		t.Fatal(err)
	}
	crbCheckTrade(t, "eur buffer", crbOnlyTrade(t, "eur buffer", result), crbWant{side: "short", entry: 1.0975, exit: 1.0960, reason: "rule", rule: "clock-range-close", sl: 1.0975 * 1.01})
}

// Entry slippage 0.5 applies once. A long that triggers at 101 fills at 101.5.
// The 1% stop is 101.5 * 0.01 = 1.015 below it, 100.485; risk 101.5 gives size
// 101.5 / 1.015 = 100. The bar low 100.6 stays above that stop. The clock close
// at open 103 exits through the same slippage: 103 - 0.5 = 102.5. Points 1.0;
// pnl = 1.0 * 100 - 0.01 * 100 = 99.
func TestClockRangeBreakoutSlippageMovesFillStopAndSize(t *testing.T) {
	bars := crbSession(crbBar(crbBaseRangeEnd, 100.8, 101.4, 100.6, 101), crbBar(crbBaseClose, 103, 104, 102, 103))
	result, _ := crbMustRun(t, crbOptions{risk: "101.5"}.program(), bars, Costs{Slippage: 0.5, FeePerUnit: 0.01})
	trade := crbOnlyTrade(t, "slippage", result)
	crbCheckTrade(t, "slippage", trade, crbWant{side: "long", entry: 101.5, exit: 102.5, reason: "rule", rule: "clock-range-close",
		sl: 100.485, checkSize: true, size: 100})
	if !crbClose(trade.Points, 1) || !crbClose(trade.PnL, 99) {
		t.Errorf("points/pnl = %v/%v, want 1/99", trade.Points, trade.PnL)
	}
}

// Risk presence follows the shared broker: an explicit zero stays zero, an
// authored amount sizes the stop distance, and an omitted amount uses the
// default 200 USD of every family.
func TestClockRangeBreakoutRiskPresence(t *testing.T) {
	bars := crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose, 102, 103, 101, 102))
	zero, _ := crbMustRun(t, crbOptions{risk: "0"}.program(), bars, Costs{})
	if trade := crbOnlyTrade(t, "zero risk", zero); trade.Size != 0 {
		t.Errorf("explicit zero risk size = %v, want 0", trade.Size)
	}
	omitted := strings.Replace(crbOptions{}.program(), "execution {\n  risk: 200 USD\n}\n", "", 1)
	result, _ := crbMustRun(t, omitted, bars, Costs{})
	crbCheckTrade(t, "default risk", crbOnlyTrade(t, "default risk", result), crbWant{
		side: "long", entry: 101, exit: 102, reason: "rule", rule: "clock-range-close", sl: 99.99, checkSize: true, size: 200 / 1.01})
}

// Nothing inherited from the shared defaults reaches the trade. After the long
// fills at 101 (stop distance 1.01), the following bar trades up to 103: a
// default 1R target (102.01) or an ATR-derived target would exit there, and a
// 0.75R breakeven would have moved the stop to the entry, so the next bar's dip
// to 100.5 would stop it. The trade must stay open to the clock close.
func TestClockRangeBreakoutInheritedManagementDoesNotLeak(t *testing.T) {
	bars := crbSession(
		crbBar(crbEntryAt(0), 100.5, 101.5, 100.2, 101),
		crbBar(crbEntryAt(1), 101, 103, 100.9, 102.9),
		crbBar(crbEntryAt(2), 102.9, 103, 100.5, 101),
		crbBar(crbBaseClose, 101.2, 104, 100, 101),
	)
	result, _ := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
	trade := crbOnlyTrade(t, "management", result)
	crbCheckTrade(t, "management", trade, crbWant{side: "long", entry: 101, exit: 101.2, reason: "rule", rule: "clock-range-close", sl: 99.99,
		checkIndices: true, entryIndex: 36, exitIdx: 39})
	if trade.SL != trade.InitialSL {
		t.Errorf("stop moved from %v to %v", trade.InitialSL, trade.SL)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"tp":null`, `"initialTp":null`, `"reason":"rule"`, `"rule":"clock-range-close"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("serialized result lacks %s: %s", want, raw)
		}
	}
}

// Group 10: causality. The fill is at 14:05 on the UTC+10 clock, in the shared
// default's unadmitted mid-session hours, and still happens. The fill bar's
// close, volume and candle shape cannot change admission or any result field,
// and market-order fill timing does not move a stop-entry.
func TestClockRangeBreakoutBareFamilyIgnoresInheritedWindows(t *testing.T) {
	failed, err := dsl.Parse("dsl v7\nstrategy \"baseline\" {\n}\nsetup {\n  type: failed breakout\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if inAdmittedTradeWindow(crbBaseRangeEnd, paramsFromConfig(failed.Config), 0) {
		t.Fatal("test premise: 14:05 on UTC+10 should be outside the default admitted windows")
	}
	bars := crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose, 102, 103, 101, 102))
	result, _ := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
	crbOnlyTrade(t, "outside windows", result)
}

func TestClockRangeBreakoutFillBarShapeAndFillOnDoNotChangeTheTrade(t *testing.T) {
	variant := func(closePrice, volume float64) []marketdata.Bar {
		entry := crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, closePrice)
		entry.V = volume
		return crbSession(entry, crbBar(crbBaseClose, 102, 103, 101, 102))
	}
	want, _ := crbMustRun(t, crbOptions{}.program(), variant(101, 1), Costs{})
	for _, bars := range [][]marketdata.Bar{variant(100.3, 1), variant(101.45, 5e9), variant(100.2, 0)} {
		got, _ := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
		if !reflect.DeepEqual(got.Trades, want.Trades) {
			t.Fatalf("fill-bar shape changed the trade:\n got %+v\nwant %+v", got.Trades, want.Trades)
		}
	}
	for _, fillOn := range []string{"close", "open", "nextOpen"} {
		got, _ := crbMustRun(t, crbOptions{}.program(), variant(101, 1), Costs{FillOn: fillOn})
		if !reflect.DeepEqual(got.Trades, want.Trades) {
			t.Fatalf("fillOn %s changed the stop-entry:\n got %+v\nwant %+v", fillOn, got.Trades, want.Trades)
		}
	}
}

// Resolution: the same rules run at 1m. A 1m series of the same prices gives
// the same economic trade as 5m when each 5m bar is a block of five identical
// 1m bars; the 1m run does not claim to resolve intraminute order.
func TestClockRangeBreakoutOneMinuteRoute(t *testing.T) {
	oneMinute := func(bar marketdata.Bar) []marketdata.Bar {
		var out []marketdata.Bar
		for k := 0; k < 5; k++ {
			out = append(out, marketdata.Bar{T: bar.T + float64(k)*60000, O: bar.O, H: bar.H, L: bar.L, C: bar.C, V: 1})
		}
		return out
	}
	var bars []marketdata.Bar
	for _, bar := range crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101)) {
		bars = append(bars, oneMinute(bar)...)
	}
	bars = append(bars, crbBar(crbBaseClose, 102, 103, 101, 102))
	source := crbOptions{route: "slices(XAUUSD 1m)"}.program()
	result, _, err := crbRun(t, source, "XAUUSD", "1m", bars, Costs{})
	if err != nil {
		t.Fatal(err)
	}
	trade := crbOnlyTrade(t, "1m", result)
	crbCheckTrade(t, "1m", trade, crbWant{side: "long", entry: 101, exit: 102, reason: "rule", rule: "clock-range-close", sl: 99.99,
		checkMeta: true, rangeHigh: 101, rangeLow: 99, rangeBars: 180, dayKey: "2026-03-10", orderPlacedAt: crbBaseRangeEnd,
		checkIndices: true, entryIndex: 180, exitIdx: 185})
}

// Group 7, prefix and resume: closing a prefix is not the end of the test. A
// prefix cut while the long is open keeps it open and omits no trade; the
// extension that reaches the clock close closes that same position, and every
// prefix agrees with the full run.
func TestClockRangeBreakoutPrefixRetainsOpenStateUntilFinalization(t *testing.T) {
	bars := crbSession(
		crbBar(crbEntryAt(0), 100.5, 101.5, 100.2, 101),
		crbBar(crbEntryAt(1), 101, 101.3, 100.8, 101.1),
		crbBar(crbUTC(2026, time.March, 10, 17, 35), 100.8, 130, 50, 100),
	)
	parsed, err := dsl.Parse(crbOptions{}.program())
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	prefixAt := func(n int) PrefixResult {
		t.Helper()
		result, err := RunPrefix(RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars[:n]), Symbol: "XAUUSD", Timeframe: "5m", StrategyID: "crb-prefix"})
		if err != nil {
			t.Fatalf("prefix %d: %v", n, err)
		}
		return result
	}
	full, _ := crbMustRun(t, crbOptions{}.program(), bars, Costs{})
	if full.TradeCount != 1 {
		t.Fatalf("full run: %+v", full.Trades)
	}
	final := full.Trades[0]
	if final.Reason != "rule" || final.ExitIndex != len(bars)-1 {
		t.Fatalf("full run trade = %+v", final)
	}
	for n := 30; n <= len(bars); n++ {
		got := prefixAt(n)
		switch {
		case n <= 36: // before the entry bar exists
			if len(got.Trades) != 0 || len(got.OpenPositions) != 0 {
				t.Errorf("prefix %d: unexpected state %+v", n, got)
			}
		case n < len(bars): // entry bar present, clock close not yet quoted
			if len(got.Trades) != 0 || len(got.OpenPositions) != 1 {
				t.Fatalf("prefix %d: trades=%d open=%d, want 0 closed and 1 open (no premature liquidation)", n, len(got.Trades), len(got.OpenPositions))
			}
			open := got.OpenPositions[0]
			if open.Side != "long" || !crbClose(open.Entry, 101) || open.EntryIndex != 36 {
				t.Errorf("prefix %d: open position = %+v", n, open)
			}
			if _, has := crbOpenMeta(t, open)["exitReason"]; has {
				t.Errorf("prefix %d: an open position must not carry an exit reason", n)
			}
		default:
			if len(got.Trades) != 1 || len(got.OpenPositions) != 0 {
				t.Fatalf("prefix %d: trades=%d open=%d", n, len(got.Trades), len(got.OpenPositions))
			}
			closed := serializeTrade(got.Trades[0].Trade)
			if !reflect.DeepEqual(closed, final) {
				t.Errorf("final prefix trade differs from the full run:\n got %+v\nwant %+v", closed, final)
			}
		}
	}
}

func crbOpenMeta(t *testing.T, position OpenPositionSnapshot) map[string]any {
	t.Helper()
	meta, ok := position.Meta[clockRangeMetaKey].(map[string]any)
	if !ok {
		t.Fatalf("open position meta = %#v", position.Meta)
	}
	return meta
}

// The execution window keeps range bars outside the window available but only
// lets fills, stops and liquidation happen inside it. A fill before the window
// start consumes the day without a trade; the window end finalizes the open
// position at that bar's close.
func TestClockRangeBreakoutExecutionWindow(t *testing.T) {
	bars := crbSession(
		crbBar(crbEntryAt(0), 100.5, 101.5, 100.2, 101),
		crbBar(crbEntryAt(1), 101, 101.3, 100.8, 101.1),
		crbBar(crbEntryAt(2), 101.1, 101.4, 100.9, 101.2),
	)
	parsed, err := dsl.Parse(crbOptions{}.program())
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	run := func(from, to float64) RunResult {
		t.Helper()
		fromMs, toMs := int64(from), int64(to)
		request := RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars), Symbol: "XAUUSD", Timeframe: "5m",
			StrategyID: "crb-window", ExecutionWindow: &ExecutionWindow{TradeFromT: &fromMs, TradeToT: &toMs}}
		result, err := Run(request)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	inside := run(crbBaseRangeEnd, crbEntryAt(2))
	trade := crbOnlyTrade(t, "window", inside)
	if trade.EntryIndex != 36 || trade.Reason != "end-of-test" || trade.ExitIndex != 37 || !crbClose(trade.Exit, 101.1) {
		t.Errorf("window trade = %+v, want entry 36 and end-of-test at bar 37's close", trade)
	}
	if after := run(crbEntryAt(1), crbEntryAt(3)); after.TradeCount != 0 {
		t.Errorf("a fill before the window start must not trade later: %+v", after.Trades)
	}
}

// The range closes when its final bar does. A series that ends there already
// shows the day: orders pending with all 36 bars counted, or the rejection.
func TestClockRangeBreakoutPlacesWhenTheFinalRangeBarCloses(t *testing.T) {
	_, prepared := crbMustRun(t, crbOptions{}.program(), crbRangeBars(crbBaseRangeStart, 99, 101), Costs{})
	if day := crbDay(t, prepared, 0); day.Outcome != ClockDayPending || day.RangeBars != 36 || day.DayKey != "2026-03-10" {
		t.Errorf("complete range, no later quote: %+v", day)
	}
	_, prepared = crbMustRun(t, crbOptions{}.program(), crbRangeBars(crbBaseRangeStart, 99, 101, 1, 2, 4, 5), Costs{})
	if day := crbDay(t, prepared, 0); day.Reason != ClockReasonInsufficient {
		t.Errorf("insufficient coverage, no later quote: %+v", day)
	}
}

// First quote after the range arrives past the expiry: the orders were live
// and expired unfilled, and nothing trades even though that bar spans a trigger.
func TestClockRangeBreakoutFirstPostRangeQuoteAfterExpiry(t *testing.T) {
	late := crbBar(crbUTC(2026, time.March, 10, 5, 5), 100, 105, 95, 100)
	result, prepared := crbMustRun(t, crbOptions{expire: "15:00"}.program(), crbSession(late), Costs{})
	if day := crbDay(t, prepared, 0); result.TradeCount != 0 || day.Outcome != ClockDayExpired {
		t.Errorf("trades=%d day=%+v", result.TradeCount, day)
	}
}

// Prefix replay is not checkpoint resume. The resumable entry point stays
// unsupported for this family and says so.
func TestClockRangeBreakoutCheckpointResumeIsUnsupported(t *testing.T) {
	parsed, err := dsl.Parse(crbOptions{}.program())
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	request := RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101))), Symbol: "XAUUSD", Timeframe: "5m", StrategyID: "crb-resume"}
	_, _, err = RunPrefixResumable(request, nil)
	var unsupported *PrefixCheckpointUnsupportedError
	if !errors.As(err, &unsupported) || !strings.Contains(err.Error(), "clockRangeBreakout") {
		t.Fatalf("err = %v, want an unsupported-checkpoint error naming the family", err)
	}
}

// Original fixture rows are checked before the lossy adapters: a short row is
// not dropped (the coverage rule would hide it) and a long one is not cut.
func TestClockRangeBreakoutRejectsMalformedFixtureRows(t *testing.T) {
	source := crbOptions{}.program()
	base := func() RunFixture {
		bars := crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose, 102, 103, 101, 102))
		fixture := RunFixture{Case: "rows", Symbol: "XAUUSD", Timeframe: "5m", Bars: bars}
		for _, bar := range bars {
			fixture.RawBars = append(fixture.RawBars, []float64{bar.T, bar.O, bar.H, bar.L, bar.C, bar.V})
		}
		return fixture
	}
	if _, err := RunFixtureCase(base(), source); err != nil {
		t.Fatalf("well-formed fixture: %v", err)
	}
	dropped := base() // what rowsToBars does to a four-value row
	dropped.RawBars[10] = dropped.RawBars[10][:4]
	dropped.Bars = append(append([]marketdata.Bar(nil), dropped.Bars[:10]...), dropped.Bars[11:]...)
	extra := base()
	extra.RawBars[10] = append(extra.RawBars[10], 7)
	nonfinite := base()
	nonfinite.Bars[36].H = math.Inf(1)
	nonfinite.RawBars = nil // typed caller
	for name, fixture := range map[string]RunFixture{"short row": dropped, "seven-value row": extra, "typed +Inf high": nonfinite} {
		if _, err := RunFixtureCase(fixture, source); err == nil || !strings.Contains(err.Error(), "malformed series") {
			t.Errorf("%s: err = %v, want malformed series", name, err)
		}
	}
}

// A parsed config changed after parsing keeps the parser's promise: a negative
// risk cannot trade through a direct API, and explicit zero still can.
func TestClockRangeBreakoutDirectConfigRiskBoundary(t *testing.T) {
	bars := crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose, 102, 103, 101, 102))
	for _, tc := range []struct {
		risk    float64
		wantErr bool
	}{{-200, true}, {math.NaN(), true}, {math.Inf(1), true}, {0, false}, {50, false}} {
		parsed, err := dsl.Parse(crbOptions{}.program())
		if err != nil || len(parsed.Errors) != 0 {
			t.Fatalf("parse: %v %v", err, parsed.Errors)
		}
		parsed.Config["riskUsd"] = tc.risk
		_, err = Run(RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars), Symbol: "XAUUSD", Timeframe: "5m", StrategyID: "crb-risk"})
		if (err != nil) != tc.wantErr {
			t.Errorf("riskUsd %v: err = %v, wantErr %v", tc.risk, err, tc.wantErr)
		}
	}
}

// JSON null decodes to zero in [][]float64, so a missing value would pass for
// a real quote. The original primitives are checked on every fixture entry.
func TestClockRangeBreakoutRejectsNullAndNonNumberFixtureValues(t *testing.T) {
	dir := crbCorpusRunDir(t)
	baseJSON, err := os.ReadFile(filepath.Join(dir, "family-clock-range-breakout-ordinary-long.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(dir, "family-clock-range-breakout-ordinary-long.strat"))
	if err != nil {
		t.Fatal(err)
	}
	longOnly := strings.Replace(string(source), "risk {", "filters {\n  side long only\n}\nrisk {", 1)
	mutated := func(mutate func(bars [][]any)) []byte {
		var fixture map[string]any
		if err := json.Unmarshal(baseJSON, &fixture); err != nil {
			t.Fatal(err)
		}
		bars := fixture["bars"].([]any)
		rows := make([][]any, len(bars))
		for i, row := range bars {
			rows[i] = row.([]any)
		}
		mutate(rows)
		for i := range rows {
			bars[i] = rows[i]
		}
		out, err := json.Marshal(fixture)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	run := func(raw []byte, program string) error {
		path := filepath.Join(t.TempDir(), "fixture.json")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		fixture, err := LoadRunFixture(path)
		if err != nil {
			return err // a non-number primitive may already fail to decode
		}
		_, err = RunFixtureCase(fixture, program)
		return err
	}
	if err := run(mutated(func([][]any) {}), string(source)); err != nil {
		t.Fatalf("unmodified fixture: %v", err)
	}
	columns := []string{"timestamp", "open", "high", "low", "close", "volume"}
	for column, name := range columns {
		raw := mutated(func(bars [][]any) { bars[10][column] = nil })
		if err := run(raw, string(source)); err == nil {
			t.Errorf("null %s was accepted", name)
		}
	}
	// Trade-producing cases: the intended range loses a bar, or a missing open
	// is read as a zero quote.
	if err := run(mutated(func(bars [][]any) { bars[0][0] = nil }), string(source)); err == nil {
		t.Error("null first timestamp was accepted")
	}
	if err := run(mutated(func(bars [][]any) { bars[36][1] = nil; bars[36][3] = 0.0 }), longOnly); err == nil {
		t.Error("null open with a zero low was accepted")
	}
	for name, value := range map[string]any{"string": "101", "boolean": true, "object": map[string]any{}} {
		if err := run(mutated(func(bars [][]any) { bars[10][2] = value }), string(source)); err == nil {
			t.Errorf("%s value was accepted", name)
		}
	}
	// Legitimate zeros stay valid: zero volume, and explicit zero risk.
	if err := run(mutated(func(bars [][]any) { bars[10][5] = 0.0 }), string(source)); err != nil {
		t.Errorf("zero volume rejected: %v", err)
	}
	zeroRisk := strings.Replace(string(source), "risk: 200 USD", "risk: 0 USD", 1)
	if err := run(mutated(func([][]any) {}), zeroRisk); err != nil {
		t.Errorf("explicit zero risk rejected: %v", err)
	}
}

// Another family keeps decoding null as before.
func TestOtherFamiliesStillDecodeNullFixtureValuesAsBefore(t *testing.T) {
	var fixture RunFixture
	raw := []byte(`{"bars":[[1,2,3,null,5,6]]}`)
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.ScanRawBarRows(raw)
	if fixture.RawBars[0][3] != 0 {
		t.Fatalf("decoded row = %v", fixture.RawBars[0])
	}
	if err := validateClockRangeFixtureRows(dsl.Config{"setupType": "openingRangeBreakout"}, fixture); err != nil {
		t.Fatalf("other family checked the rows: %v", err)
	}
}

// `bars` is required by the fixture schema. Missing or null is a malformed
// container, distinct from an explicit empty array, which means no quotes.
func TestClockRangeBreakoutRejectsMissingOrNullBarsContainer(t *testing.T) {
	dir := crbCorpusRunDir(t)
	baseJSON, err := os.ReadFile(filepath.Join(dir, "family-clock-range-breakout-ordinary-long.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(dir, "family-clock-range-breakout-ordinary-long.strat"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(mutate func(map[string]any)) (RunResult, error) {
		var fixture map[string]any
		if err := json.Unmarshal(baseJSON, &fixture); err != nil {
			t.Fatal(err)
		}
		mutate(fixture)
		raw, _ := json.Marshal(fixture)
		path := filepath.Join(t.TempDir(), "fixture.json")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadRunFixture(path)
		if err != nil {
			return RunResult{}, err
		}
		return RunFixtureCase(loaded, string(source))
	}
	for name, mutate := range map[string]func(map[string]any){
		"omitted bars": func(f map[string]any) { delete(f, "bars") },
		"null bars":    func(f map[string]any) { f["bars"] = nil },
		"string bars":  func(f map[string]any) { f["bars"] = "none" },
		"object bars":  func(f map[string]any) { f["bars"] = map[string]any{} },
	} {
		if _, err := run(mutate); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	result, err := run(func(f map[string]any) { f["bars"] = []any{} })
	if err != nil || result.TradeCount != 0 {
		t.Errorf("explicit empty bars: err=%v trades=%d, want no error and no trades", err, result.TradeCount)
	}
	// A typed caller supplies Bars directly and has no JSON container.
	typed := RunFixture{Case: "typed", Symbol: "XAUUSD", Timeframe: "5m",
		Bars: crbSession(crbBar(crbBaseRangeEnd, 100.5, 101.5, 100.2, 101), crbBar(crbBaseClose, 102, 103, 101, 102))}
	if result, err := RunFixtureCase(typed, string(source)); err != nil || result.TradeCount != 1 {
		t.Errorf("typed-only bars: err=%v trades=%d", err, result.TradeCount)
	}
}
