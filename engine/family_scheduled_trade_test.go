package engine

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const scheduledSource = `dsl v7
strategy "Scheduled synthetic"
market conditions {
 slices(USDJPY 5m)
}
setup {
 type: scheduled trade
 scheduled timezone UTC
 scheduled calendar japan-settlement
 scheduled entry 00:55
 scheduled exit 01:30
 scheduled side short
 scheduled stop 20 pips
 scheduled quantity 1 unit
}`

func scheduledRequest(t *testing.T, bars []marketdata.Bar) RunRequest {
	t.Helper()
	parsed, err := dsl.Parse(scheduledSource)
	if err != nil || len(parsed.Errors) > 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	return RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars), Symbol: "USDJPY", Timeframe: "5m", StrategyID: "scheduled-synthetic", Costs: Costs{FillOn: "nextOpen", FeePerUnit: .0075, StartEquity: 10000}}
}
func scheduledBars(day int, opens []float64, highs, lows []float64) []marketdata.Bar {
	base := time.Date(2021, time.January, day, 0, 50, 0, 0, time.UTC).UnixMilli()
	rows := make([]marketdata.Bar, len(opens))
	for i, o := range opens {
		h, l := o+.05, o-.05
		if highs != nil {
			h = highs[i]
		}
		if lows != nil {
			l = lows[i]
		}
		rows[i] = marketdata.Bar{T: float64(base + int64(i)*5*60*1000), O: o, H: h, L: l, C: o, V: 1}
	}
	return rows
}

func TestScheduledTradeCalendarRollAndSkip(t *testing.T) {
	for _, tc := range []struct {
		date string
		want bool
	}{{"2021-01-01", false}, {"2021-01-05", true}, {"2021-05-05", false}, {"2021-10-08", true}, {"2021-01-29", true}} {
		d, _ := time.Parse("2006-01-02", tc.date)
		if got := isJapanSettlementDate(d); got != tc.want {
			t.Fatalf("%s settlement=%v want %v", tc.date, got, tc.want)
		}
	}
}

func TestScheduledTradeExactEntryTimeExitAndTwoSidedFee(t *testing.T) {
	bars := scheduledBars(5, []float64{110, 110, 109.99, 109.95, 109.9, 109.8, 109.7, 109.8, 109.9}, nil, nil)
	r, err := Run(scheduledRequest(t, bars))
	if err != nil {
		t.Fatal(err)
	}
	if r.TradeCount != 1 || r.ScheduledAudit == nil || r.ScheduledAudit.Rows[0].Status != "time-exit" {
		t.Fatalf("result=%+v", r)
	}
	trade := r.Trades[0]
	if trade.EntryT != float64(bars[1].T) || trade.ExitT != float64(bars[8].T) {
		t.Fatalf("trade times=%v..%v", trade.EntryT, trade.ExitT)
	}
	if trade.Points <= 0 || math.Abs(trade.PnL-(trade.Points-.015)) > 1e-12 {
		t.Fatalf("gross points=%v pnl=%v; want two fixed fees", trade.Points, trade.PnL)
	}
}

func TestScheduledTradeStopGapAndTimeExitOrdering(t *testing.T) {
	// A later opening gap through the short stop uses the worse open.
	bars := scheduledBars(5, []float64{110, 110, 110.25}, nil, nil)
	r, err := Run(scheduledRequest(t, bars))
	if err != nil {
		t.Fatal(err)
	}
	if r.Trades[0].Exit != 110.25 || !r.ScheduledAudit.Rows[0].GapStop {
		t.Fatalf("gap result=%+v audit=%+v", r.Trades[0], r.ScheduledAudit.Rows[0])
	}
	// At/after deadline the scheduled open exit precedes that bar's extreme.
	bars = scheduledBars(5, []float64{110, 110, 110, 110, 110, 110, 110, 110, 109.9}, nil, nil)
	bars[8].H = 111
	r, err = Run(scheduledRequest(t, bars))
	if err != nil {
		t.Fatal(err)
	}
	if r.Trades[0].Reason != ReasonRule || r.Trades[0].Exit != 109.9 || r.Trades[0].Rule != "scheduled-time-exit" {
		t.Fatalf("time precedence=%+v", r.Trades[0])
	}
}

func TestScheduledTradeRequiresContiguousPriorAndPreservesTerminalExposure(t *testing.T) {
	bars := scheduledBars(5, []float64{110}, nil, nil)
	r, err := Run(scheduledRequest(t, bars))
	if err != nil {
		t.Fatal(err)
	}
	if r.ScheduledAudit.Rows[0].Status != "missing-entry-bar" || r.ScheduledAudit.Rows[0].EntryIndex != nil || r.ScheduledAudit.Rows[0].ExitIndex != nil {
		t.Fatalf("missing entry indices should be absent: %+v", r.ScheduledAudit.Rows[0])
	}
	bars = scheduledBars(5, []float64{110}, nil, nil)
	bars[0].T = float64(time.Date(2021, time.January, 5, 0, 55, 0, 0, time.UTC).UnixMilli())
	r, err = Run(scheduledRequest(t, bars))
	if err != nil {
		t.Fatal(err)
	}
	if r.TradeCount != 0 || r.ScheduledAudit.Rows[0].Status != "missing-contiguous-prior" {
		t.Fatalf("missing prior: %+v", r.ScheduledAudit)
	}
	bars = scheduledBars(5, []float64{110, 110, 110}, nil, nil)
	r, err = Run(scheduledRequest(t, bars))
	if err != nil {
		t.Fatal(err)
	}
	if r.TradeCount != 0 || r.ScheduledAudit.Rows[0].Status != "open-terminal" || r.ScheduledAudit.Rows[0].EntryIndex == nil || *r.ScheduledAudit.Rows[0].EntryIndex != 1 || r.ScheduledAudit.Rows[0].ExitIndex != nil || r.ScheduledAudit.OpenPosition == nil || r.ScheduledAudit.OpenPosition.EntryIndex != 1 {
		t.Fatalf("terminal exposure: %+v", r.ScheduledAudit)
	}
}

func TestScheduledTradeRejectsDirectConfigOverride(t *testing.T) {
	request := scheduledRequest(t, scheduledBars(5, []float64{110, 110}, nil, nil))
	request.Config["scheduledTrade"].(map[string]any)["quantity"] = float64(2)
	if _, err := Run(request); err == nil || !strings.Contains(err.Error(), "quantity") {
		t.Fatalf("direct config override error=%v", err)
	}
}

func TestScheduledTradeCountsOnlyHeldGapsAndBlocksSameIndexReentry(t *testing.T) {
	bars := scheduledBars(5, []float64{110, 110}, nil, nil)
	bars = append(bars, marketdata.Bar{T: float64(time.Date(2021, time.January, 8, 0, 55, 0, 0, time.UTC).UnixMilli()), O: 109.9, H: 109.95, L: 109.85, C: 109.9, V: 1})
	r, err := Run(scheduledRequest(t, bars))
	if err != nil {
		t.Fatal(err)
	}
	if r.TradeCount != 1 || r.Trades[0].Rule != "scheduled-time-exit" {
		t.Fatalf("trades=%+v", r.Trades)
	}
	if r.ScheduledAudit.GapBars != 863 || r.ScheduledAudit.GapObservations != 1 {
		t.Fatalf("held gap audit=%+v", r.ScheduledAudit)
	}
	if len(r.ScheduledAudit.Rows) < 2 || r.ScheduledAudit.Rows[1].Status != "busy-at-entry" {
		t.Fatalf("same-index reentry audit=%+v", r.ScheduledAudit.Rows)
	}
}

func TestScheduledTradePrefixCausalityAndLongSide(t *testing.T) {
	source := strings.Replace(scheduledSource, "scheduled side short", "scheduled side long", 1)
	parsed, err := dsl.Parse(source)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse long: %v %v", err, parsed.Errors)
	}
	bars := scheduledBars(5, []float64{110, 110, 110.1, 110.2, 110.3, 110.4, 110.3, 110.2, 110.1}, nil, nil)
	request := scheduledRequest(t, bars)
	request.Config = parsed.Config
	full, err := Run(request)
	if err != nil {
		t.Fatal(err)
	}
	for end := 2; end < len(bars); end++ {
		prefix := request
		prefix.Series = marketdata.SeriesFromBars(bars[:end])
		got, e := Run(prefix)
		if e != nil {
			t.Fatal(e)
		}
		if got.TradeCount > full.TradeCount {
			t.Fatalf("prefix trades=%d full=%d", got.TradeCount, full.TradeCount)
		}
	}
	if full.TradeCount != 1 || full.Trades[0].Side != "long" {
		t.Fatalf("long result=%+v", full)
	}
}

func TestScheduledTradeRejectsOutOfCoverageAndUnsupportedPaths(t *testing.T) {
	bars := scheduledBars(5, []float64{110, 110}, nil, nil)
	bars[0].T = float64(time.Date(2020, 12, 31, 23, 50, 0, 0, time.UTC).UnixMilli())
	bars[1].T = float64(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli())
	if _, err := Run(scheduledRequest(t, bars)); err == nil || !strings.Contains(err.Error(), "coverage") {
		t.Fatalf("coverage error=%v", err)
	}
	request := scheduledRequest(t, scheduledBars(5, []float64{110, 110}, nil, nil))
	request.SourceSeries = marketdata.SeriesFromBars(request.Series.Bars())
	if _, err := Run(request); err == nil || !strings.Contains(err.Error(), "rejects") {
		t.Fatalf("source route error=%v", err)
	}
}
