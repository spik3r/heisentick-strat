package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Optional clock range breakout gates. Every bar and expected value is
// invented and derived by hand. The strategy runs on XAUUSD 1h with clock
// UTC+0, range 00:00-03:00, orders until 06:00, close 09:00 and 100 USD risk.

const crgHour = 60 * 60 * 1000

func crgSource(setupLines []string, stopLine, targetLine string) string {
	var b strings.Builder
	b.WriteString("dsl v7\nstrategy \"gates\" {\n  description \"synthetic\"\n}\n")
	b.WriteString("market conditions {\n  slices(XAUUSD 1h)\n  clock UTC+0\n}\n")
	b.WriteString("setup {\n  type: clock range breakout\n  range 00:00 to 03:00\n  orders expire 06:00\n")
	for _, line := range setupLines {
		b.WriteString("  " + line + "\n")
	}
	b.WriteString("}\nrisk {\n  " + stopLine + "\n}\n")
	if targetLine != "" {
		b.WriteString("target {\n  " + targetLine + "\n}\n")
	}
	b.WriteString("management {\n  close positions at 09:00\n}\nexecution {\n  risk: 100 USD\n}\n")
	return b.String()
}

func crgDate(day int) float64 { return crbUTC(2026, time.March, day, 0, 0) }

// crgFullDay is 24 hourly bars from midnight: the first opens at o, the last
// closes at c, bar 5 makes the high h and bar 6 the low l, and the rest sit at
// the midpoint with a 0.1 wick each way (h - l must exceed 0.2).
func crgFullDay(day int, o, h, l, c float64) []marketdata.Bar {
	mid := (h + l) / 2
	bars := make([]marketdata.Bar, 0, 24)
	for k := 0; k < 24; k++ {
		open, close := mid, mid
		if k == 0 {
			open = o
		}
		if k == 23 {
			close = c
		}
		bar := crbBar(crgDate(day)+float64(k*crgHour), open, max(open, close)+0.1, min(open, close)-0.1, close)
		switch k {
		case 5:
			bar.H = h
		case 6:
			bar.L = l
		}
		bars = append(bars, bar)
	}
	return bars
}

// crgRangeDay is the three range bars 00:00-02:00 of a day spanning [lo, hi],
// then the given bars at 03:00, 04:00, ... and a close bar at 09:00 opening
// at closeOpen.
func crgRangeDay(day int, lo, hi, closeOpen float64, after ...marketdata.Bar) []marketdata.Bar {
	mid := (lo + hi) / 2
	start := crgDate(day)
	bars := []marketdata.Bar{
		crbBar(start, mid, hi, mid, mid),
		crbBar(start+crgHour, mid, mid, lo, mid),
		crbBar(start+2*crgHour, mid, mid, mid, mid),
	}
	for k, bar := range after {
		bar.T = start + float64((3+k)*crgHour)
		bars = append(bars, bar)
	}
	return append(bars, crbBar(start+9*crgHour, closeOpen, closeOpen, closeOpen, closeOpen))
}

// crgHistory: 2 March flat (open = close = 100, high 101, low 99); 3 March
// green or red (open 100, high 102, low 98, close 101 or 99). The true range of
// 3 March is max(102-98, |102-100|, |98-100|) = 4.
func crgHistory(green bool) []marketdata.Bar {
	closeB := 101.0
	if !green {
		closeB = 99
	}
	return append(crgFullDay(2, 100, 101, 99, 100), crgFullDay(3, 100, 102, 98, closeB)...)
}

func crgRun(t *testing.T, source string, bars []marketdata.Bar) (RunResult, *PreparedRun) {
	t.Helper()
	result, prepared, err := crbRun(t, source, "XAUUSD", "1h", bars, Costs{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return result, prepared
}

func crgLastDay(t *testing.T, prepared *PreparedRun) ClockRangeDay {
	t.Helper()
	days := prepared.ClockRangeDays()
	if len(days) == 0 {
		t.Fatal("no range days")
	}
	return days[len(days)-1]
}

func crgCheck(t *testing.T, label string, got Trade, side string, entry, exit, sl float64, reason, exitReason string) {
	t.Helper()
	if got.Side != side || !crbClose(got.Entry, entry) || !crbClose(got.Exit, exit) || !crbClose(got.SL, sl) || got.Reason != reason {
		t.Errorf("%s: trade = %s %v -> %v sl %v reason %q; want %s %v -> %v sl %v reason %q", label, got.Side, got.Entry, got.Exit, got.SL, got.Reason, side, entry, exit, sl, reason)
	}
	if meta := crbMeta(t, got); meta["exitReason"] != exitReason {
		t.Errorf("%s: exitReason = %v, want %s", label, meta["exitReason"], exitReason)
	}
	if want := 100 / abs(entry-sl); !crbClose(got.Size, want) {
		t.Errorf("%s: size = %v, want %v", label, got.Size, want)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// ATR 1 on 4 March is the true range of 3 March, 4, which uses 2 March's
// close. 2 March and 3 March have no ATR of their own (no-daily-atr). With
// multiple 0.5 the range must be at least 2 wide.
func TestClockRangeWidthGate(t *testing.T) {
	source := crgSource([]string{"range width at least 0.5 daily ATR 1"}, "stop beyond opposite range edge", "")
	// Long fills at max(trigger, open) = 103 at 03:00 and closes at the 09:00 open.
	entry := crbBar(0, 102, 104, 101.5, 103.5)
	tests := []struct {
		name    string
		lo, hi  float64
		trades  int
		outcome string
		reason  string
	}{
		{"too narrow", 101.5, 103, 0, ClockDayRejected, ClockReasonRangeTooNarrow},
		{"exactly at the bar", 101, 103, 1, ClockDayEntered, ""},
		{"wide", 100, 103, 1, ClockDayEntered, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bars := append(crgHistory(true), crgRangeDay(4, test.lo, test.hi, 105, entry)...)
			result, prepared := crgRun(t, source, bars)
			if result.TradeCount != test.trades {
				t.Fatalf("trades = %d, want %d: %+v", result.TradeCount, test.trades, result.Trades)
			}
			day := crgLastDay(t, prepared)
			if day.DayKey != "2026-03-04" || day.Outcome != test.outcome || day.Reason != test.reason {
				t.Errorf("day = %+v", day)
			}
			if test.trades == 1 {
				crgCheck(t, test.name, result.Trades[0], "long", 103, 105, test.lo, "rule", "clock")
			}
			days := prepared.ClockRangeDays()
			for _, early := range days[:2] {
				if early.Reason != ClockReasonNoDailyATR {
					t.Errorf("early day = %+v, want %s", early, ClockReasonNoDailyATR)
				}
			}
		})
	}
}

// 3 March green: only the buy stop is placed, so a sell-trigger touch at 03:00
// does nothing and the 04:00 buy touch fills long at 103. 3 March red: only
// the sell stop; the 03:00 bar fills short at min(100, 101) = 100 with the
// stop at the range high 103, which the 04:00 high reaches.
func TestClockRangePriorDayCandle(t *testing.T) {
	source := crgSource([]string{"trade with prior day candle"}, "stop beyond opposite range edge", "")
	sellTouch := crbBar(0, 101, 101.5, 99, 99.5)
	buyTouch := crbBar(0, 102, 103.5, 101.8, 103.2)

	result, prepared := crgRun(t, source, append(crgHistory(true), crgRangeDay(4, 100, 103, 104, sellTouch, buyTouch)...))
	if result.TradeCount != 1 {
		t.Fatalf("green: trades = %+v", result.Trades)
	}
	crgCheck(t, "green", result.Trades[0], "long", 103, 104, 100, "rule", "clock")
	days := prepared.ClockRangeDays()
	if days[0].Reason != ClockReasonNoPriorDay || days[1].Reason != ClockReasonNoDirection {
		t.Errorf("2 March has no prior day and 3 March's prior (2 March) is flat: %+v", days[:2])
	}

	result, _ = crgRun(t, source, append(crgHistory(false), crgRangeDay(4, 100, 103, 104, sellTouch, buyTouch)...))
	if result.TradeCount != 1 {
		t.Fatalf("red: trades = %+v", result.Trades)
	}
	crgCheck(t, "red", result.Trades[0], "short", 100, 103, 103, "sl", "stop")
}

// 3 March green with the cancel rule: the sell edge is cancel-only.
func TestClockRangeCancelWhenOppositeSideTriggersFirst(t *testing.T) {
	source := crgSource([]string{"trade with prior day candle", "cancel when opposite side triggers first"}, "stop beyond opposite range edge", "")
	tests := []struct {
		name  string
		after []marketdata.Bar
		// want is nil for no trade.
		want *[2]float64 // entry, exit
	}{
		// The sell edge is touched first; the later buy touch cannot fill.
		{"opposite first", []marketdata.Bar{crbBar(0, 101, 101.5, 99, 99.5), crbBar(0, 102, 103.5, 101.8, 103.2)}, nil},
		// Both touched, open 100.5 is 0.5 from the sell trigger and 2.5 from
		// the buy trigger: the cancel side wins.
		{"both touched nearer cancel", []marketdata.Bar{crbBar(0, 100.5, 103.5, 99.5, 101), crbBar(0, 102, 103.5, 101.8, 103.2)}, nil},
		// Both touched, open 102.5 is nearer the buy trigger: long at 103, and
		// the entry-bar low 99.5 reaches the stop 100 in the same bar.
		{"both touched nearer order", []marketdata.Bar{crbBar(0, 102.5, 103.5, 99.5, 101)}, &[2]float64{103, 100}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, prepared := crgRun(t, source, append(crgHistory(true), crgRangeDay(4, 100, 103, 104, test.after...)...))
			day := crgLastDay(t, prepared)
			if test.want == nil {
				if result.TradeCount != 0 || day.Outcome != ClockDayRejected || day.Reason != ClockReasonOppositeTrigger {
					t.Fatalf("trades = %+v day = %+v", result.Trades, day)
				}
				return
			}
			if result.TradeCount != 1 {
				t.Fatalf("trades = %+v", result.Trades)
			}
			crgCheck(t, test.name, result.Trades[0], "long", test.want[0], test.want[1], 100, "sl", "stop")
		})
	}
}

// Long at 103 with the stop at the range low 100: risk 3, so 2R is 109.
func TestClockRangeTargetR(t *testing.T) {
	source := crgSource([]string{"trade with prior day candle"}, "stop beyond opposite range edge", "target 2R")
	entry := crbBar(0, 102, 103.5, 101.8, 103.2)
	tests := []struct {
		name       string
		after      []marketdata.Bar
		exit       float64
		reason     string
		exitReason string
	}{
		{"touch", []marketdata.Bar{entry, crbBar(0, 105, 110, 104.5, 108)}, 109, "tp", "target"},
		// Opening beyond the target still fills at the target, never better.
		{"gap through", []marketdata.Bar{entry, crbBar(0, 111, 112, 110.5, 111)}, 109, "tp", "target"},
		// A bar reaching both stop and target exits at the stop.
		{"stop first", []marketdata.Bar{entry, crbBar(0, 104, 110, 99.5, 104)}, 100, "sl", "stop"},
		// The entry bar's high 110 is not tested against the target; the trade
		// runs to the 09:00 close at 106.
		{"entry bar untested", []marketdata.Bar{crbBar(0, 102, 110, 101.8, 104)}, 106, "rule", "clock"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _ := crgRun(t, source, append(crgHistory(true), crgRangeDay(4, 100, 103, 106, test.after...)...))
			if result.TradeCount != 1 {
				t.Fatalf("trades = %+v", result.Trades)
			}
			trade := result.Trades[0]
			crgCheck(t, test.name, trade, "long", 103, test.exit, 100, test.reason, test.exitReason)
			if trade.NoTarget || !crbClose(trade.TP, 109) || !crbClose(trade.InitialTP, 109) {
				t.Errorf("target = %v / %v (NoTarget %v), want 109", trade.TP, trade.InitialTP, trade.NoTarget)
			}
		})
	}
}

// Friday 6 March is a full day closing at 100. Sunday 8 March has two bars
// (22:00 and 23:00, a 2-hour span) and merges into Monday 9 March, which keeps
// the Sunday open 200 and the wider Sunday extremes. Monday's prior day is
// Friday. The true range of the merged Monday is max(210-195, |210-100|,
// |195-100|) = 110.
func TestClockTradingDaysMergeShortDates(t *testing.T) {
	bars := crgFullDay(6, 100, 101, 99, 100)
	sunday := crbUTC(2026, time.March, 8, 22, 0)
	bars = append(bars, crbBar(sunday, 200, 210, 199, 205), crbBar(sunday+crgHour, 205, 206, 195, 204))
	bars = append(bars, crgFullDay(9, 204, 208, 201, 207)...)
	series := marketdata.SeriesFromBars(bars)
	schedule, err := newClockSchedule(dsl.ClockRangeSpec{}, "1h")
	if err != nil {
		t.Fatal(err)
	}
	days := buildClockTradingDays(schedule, series.T, series.O, series.H, series.L, series.C)
	friday, monday := schedule.clockDayOf(crgDate(6)), schedule.clockDayOf(crgDate(9))
	if len(days.key) != 2 || days.key[0] != friday || days.key[1] != monday {
		t.Fatalf("keys = %v, want [%d %d]", days.key, friday, monday)
	}
	if days.o[1] != 200 || days.h[1] != 210 || days.l[1] != 195 || days.c[1] != 207 {
		t.Errorf("merged Monday = %v %v %v %v", days.o[1], days.h[1], days.l[1], days.c[1])
	}
	if got := days.priorIndex(monday); got != 0 {
		t.Errorf("Monday's prior index = %d, want Friday (0)", got)
	}
	if got := days.priorIndex(monday + 1); got != 1 {
		t.Errorf("Tuesday's prior index = %d, want Monday (1)", got)
	}
	if atr, ok := days.atr(1, 1); !ok || atr != 110 {
		t.Errorf("atr = %v %v, want 110", atr, ok)
	}
	if _, ok := days.atr(0, 1); ok {
		t.Error("the first trading day has no predecessor and no true range")
	}
}

// A v1 percent-stop strategy has every gate off and carries no target.
func TestClockRangeV1RunBuildsNoTradingDays(t *testing.T) {
	source := crgSource(nil, "stop 1 percent", "")
	result, prepared := crgRun(t, source, append(crgHistory(true), crgRangeDay(4, 100, 103, 104, crbBar(0, 102, 103.5, 101.8, 103.2))...))
	if result.TradeCount == 0 {
		t.Fatal("no trades")
	}
	for _, trade := range result.Trades {
		if !trade.NoTarget || trade.TP != 0 {
			t.Errorf("v1 trade carries a target: %+v", trade)
		}
	}
	if g := prepared.clockGates; g == nil || g.UsesDailyInputs() || g.RangeEdgeStop() || g.TargetR != 0 {
		t.Errorf("v1 gates = %+v", g)
	}
}
