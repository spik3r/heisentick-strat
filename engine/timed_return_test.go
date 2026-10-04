package engine

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func timedFixture(t *testing.T, previous bool) (RunFixture, string, RunRequest) {
	t.Helper()
	name := "family-timed-return"
	if previous {
		name += "-predecessor"
	}
	f, err := LoadRunFixture("../conformance/run/" + name + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../conformance/run/" + name + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	p, err := dsl.Parse(string(b))
	if err != nil || len(p.Errors) > 0 {
		t.Fatalf("parse %v %v", err, p.Errors)
	}
	return f, string(b), RunRequest{Config: p.Config, Series: marketdata.SeriesFromBars(f.Bars), Symbol: f.Symbol, Timeframe: f.Timeframe, StrategyID: f.StrategyID, Costs: f.Costs, TimedCalendar: f.TimedCalendar}
}

func TestTimedReturnNativeAPIAndFixtureParity(t *testing.T) {
	for _, previous := range []bool{false, true} {
		f, source, request := timedFixture(t, previous)
		r, err := Run(request)
		if err != nil {
			t.Fatal(err)
		}
		if r.TradeCount != 1 || len(r.TimedAudit.Rows) != 1 {
			t.Fatalf("%+v", r)
		}
		tr := r.Trades[0]
		a := r.TimedAudit.Rows[0]
		if tr.Size != 1 || tr.Side != "long" || !tr.NoStop || !tr.NoTarget || tr.Rule != "timed-scheduled-exit" || math.Abs(tr.PnL+0.2) > 1e-10 {
			t.Fatalf("%+v", tr)
		}
		if tr.EntryT != float64(a.DecisionT+60000) || tr.ExitT != float64(a.ExitT) || a.Status != "executed" {
			t.Fatalf("audit %+v trade %+v", a, tr)
		}
		fr, err := RunFixtureCase(f, source)
		if err != nil {
			t.Fatal(err)
		}
		fr.Case = r.Case
		x, _ := json.Marshal(r)
		y, _ := json.Marshal(fr)
		if string(x) != string(y) {
			t.Fatal("fixture/API disagreement")
		}
		if previous && a.StartT >= a.EndT {
			t.Fatal("predecessor anchor order")
		}
	}
}

func TestTimedReturnOpenIsNotPreviousClose(t *testing.T) {
	_, source, r := timedFixture(t, false)
	// The default fixture has no bar ending at 08:00: O at 08:00 is present.
	good, err := Run(r)
	if err != nil || good.TradeCount != 1 {
		t.Fatalf("%v %+v", err, good)
	}
	p, _ := dsl.Parse(strings.Replace(source, "current session open", "current clock 08:00 completed-close", 1))
	r.Config = p.Config
	missing, err := Run(r)
	if err != nil || missing.TradeCount != 0 || missing.TimedAudit.Rows[0].Status != "unavailable_endpoint" {
		t.Fatalf("%v %+v", err, missing)
	}
}

func TestTimedReturnMissingAndZeroAreDifferent(t *testing.T) {
	_, _, r := timedFixture(t, false)
	end := 119
	r.Series.C[end] = 100
	zero, err := Run(r)
	if err != nil || zero.TimedAudit.Rows[0].Status != "zero_return" {
		t.Fatalf("%v %+v", err, zero)
	}
	r.Series.T[end] += 60000
	if _, err := Run(r); err == nil {
		t.Fatal("duplicate endpoint was silently skipped")
	}
	_, _, r = timedFixture(t, false)
	r.Series = removeTimedBar(r.Series, end)
	missing, err := Run(r)
	if err != nil || missing.TimedAudit.Rows[0].Status != "unavailable_endpoint" {
		t.Fatalf("%v %+v", err, missing)
	}
}

func removeTimedBar(s marketdata.Series, i int) marketdata.Series {
	for _, col := range []*[]float64{&s.T, &s.O, &s.H, &s.L, &s.C, &s.V} {
		*col = append((*col)[:i:i], (*col)[i+1:]...)
	}
	return s
}

func TestTimedReturnRejectsWholeBatchCoverageEvenZeroSignals(t *testing.T) {
	for _, index := range []int{121, 181} {
		_, _, r := timedFixture(t, false)
		r.Series.C[119] = 100
		r.Series = removeTimedBar(r.Series, index)
		got, err := Run(r)
		if err == nil || !strings.Contains(err.Error(), "dataset inadmissible") || len(got.Trades) != 0 {
			t.Fatalf("%v %+v", err, got)
		}
	}
	_, _, r := timedFixture(t, false)
	r.TimedCalendar.QuotedIntervals[0].ToT = int64(r.Series.T[181])
	if _, err := Run(r); err == nil || !strings.Contains(err.Error(), "outside quoted") {
		t.Fatalf("%v", err)
	}
}

func TestTimedReturnFutureHLCDoesNotChangeEarlierDecision(t *testing.T) {
	_, _, r := timedFixture(t, false)
	base, err := Run(r)
	if err != nil {
		t.Fatal(err)
	}
	for i := 120; i < r.Series.Len(); i++ {
		r.Series.H[i] = 1e6
		r.Series.L[i] = 1
		r.Series.C[i] = 500
	}
	got, err := Run(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Trades[0].Side != base.Trades[0].Side || got.Trades[0].EntryT != base.Trades[0].EntryT || got.Trades[0].PnL != base.Trades[0].PnL {
		t.Fatal("future HLC affected signal or execution")
	}
	r.Series.O[181] = 101
	got, err = Run(r)
	if err != nil || got.Trades[0].PnL == base.Trades[0].PnL {
		t.Fatal("future execution O must change its own fill")
	}
}

func TestTimedReturnCalendarAndCostRefusals(t *testing.T) {
	changes := []func(*RunRequest){
		func(r *RunRequest) { r.TimedCalendar = nil }, func(r *RunRequest) { r.TimedCalendar.TimezoneDataSHA256 = strings.Repeat("0", 64) },
		func(r *RunRequest) { r.TimedCalendar.Sessions[0].PreviousSessionDate = "2026-05-01" },
		func(r *RunRequest) {
			r.TimedCalendar.Sessions = append(r.TimedCalendar.Sessions, r.TimedCalendar.Sessions[0])
		},
		func(r *RunRequest) { r.TimedCalendar.TradeToDateExclusive = "2026-06-03" },
		func(r *RunRequest) {
			r.TimedCalendar.QuotedIntervals = append(r.TimedCalendar.QuotedIntervals, r.TimedCalendar.QuotedIntervals[0])
		},
		func(r *RunRequest) { r.Costs.FillOn = "close" }, func(r *RunRequest) { r.Costs.FeePerUnit = 0.1 },
		func(r *RunRequest) { r.Costs.Slippage = -1 }, func(r *RunRequest) { r.Costs.Slippage = 100 },
		func(r *RunRequest) { r.Costs.SlippageBps = math.Inf(1) }, func(r *RunRequest) { r.Costs.StartEquity = math.NaN() },
		func(r *RunRequest) { r.SourceSeries = r.Series }, func(r *RunRequest) { r.HigherTimeframe = "4h" },
		func(r *RunRequest) { r.ForceRoute = true }, func(r *RunRequest) { r.ReportTradeContext = true },
		func(r *RunRequest) { r.ExecutionWindow = &ExecutionWindow{} }, func(r *RunRequest) { r.Symbol = "OTHERUSD" },
	}
	for i, change := range changes {
		_, _, r := timedFixture(t, false)
		change(&r)
		if _, err := Run(r); err == nil {
			t.Errorf("accepted malformed case %d", i)
		}
	}
}

func TestTimedReturnEarlyPredecessorCannotBeSkipped(t *testing.T) {
	_, _, r := timedFixture(t, true)
	r.TimedCalendar.Sessions[0].Kind = "early"
	got, err := Run(r)
	if err != nil || got.TradeCount != 0 || got.TimedAudit.Rows[0].Status != "excluded_predecessor_session" {
		t.Fatalf("%v %+v", err, got)
	}
	r.TimedCalendar.Sessions[3].PreviousSessionDate = "2026-03-05"
	if _, err := Run(r); err == nil {
		t.Fatal("skipped predecessor admitted")
	}
}

func TestTimedReturnStrictBarValidation(t *testing.T) {
	changes := []func(*marketdata.Series){func(s *marketdata.Series) { s.T[1] = s.T[0] }, func(s *marketdata.Series) { s.T[0], s.T[1] = s.T[1], s.T[0] }, func(s *marketdata.Series) { s.T[0] += 0.5 }, func(s *marketdata.Series) { s.T[0] += 1000 }, func(s *marketdata.Series) { s.H[1] = s.L[1] - 1 }, func(s *marketdata.Series) { s.C[1] = 0 }, func(s *marketdata.Series) { s.O[1] = math.NaN() }, func(s *marketdata.Series) { s.V[1] = -1 }, func(s *marketdata.Series) { s.C = s.C[:1] }}
	for i, change := range changes {
		_, _, r := timedFixture(t, false)
		change(&r.Series)
		if _, err := Run(r); err == nil {
			t.Errorf("accepted invalid bars %d", i)
		}
	}
}

func TestTimedReturnRefusesLegacyPreparedGridAndPrefix(t *testing.T) {
	_, _, r := timedFixture(t, false)
	if _, err := PrepareRun(r); err == nil {
		t.Fatal("legacy preparation admitted")
	}
	if _, err := PrepareSharedRunContext(r); err == nil {
		t.Fatal("shared preparation admitted")
	}
	if _, err := SharedContextKey(r); err == nil {
		t.Fatal("shared key admitted")
	}
	if _, err := RunPrefix(r); err == nil {
		t.Fatal("prefix admitted")
	}
	if _, _, err := RunPrefixResumable(r, nil); err == nil {
		t.Fatal("checkpoint admitted")
	}
}

func TestTimedReturnPinnedClockDSTGapFold(t *testing.T) {
	_, _, r := timedFixture(t, true)
	loc, _, err := validateTimedCalendar(r.TimedCalendar, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct{ date, clock string }{{"2026-03-08", "02:30"}, {"2026-11-01", "01:30"}} {
		if _, err := timedResolve(x.date, x.clock, loc); err == nil {
			t.Fatalf("admitted ambiguous/nonexistent %+v", x)
		}
	}
	winter, _ := timedResolve("2026-01-05", "10:00", loc)
	summer, _ := timedResolve("2026-07-06", "10:00", loc)
	if time.UnixMilli(winter).UTC().Hour() != 15 || time.UnixMilli(summer).UTC().Hour() != 14 {
		t.Fatal("wrong pinned DST")
	}
}

func TestTimedReturnDirectionSideAndAdjacentPrice(t *testing.T) {
	for _, tc := range []struct {
		end                                float64
		direction, side, status, tradeSide string
	}{
		{99, "with", "both", "executed", "short"}, {99, "against", "both", "executed", "long"},
		{101, "against", "both", "executed", "short"}, {101, "with", "short only", "side_blocked", ""},
		{math.Nextafter(100, math.Inf(1)), "with", "both", "executed", "long"},
	} {
		_, source, r := timedFixture(t, false)
		r.Series.C[119] = tc.end
		r.Series.L[119] = math.Min(99, tc.end)
		r.Series.H[119] = math.Max(103, tc.end)
		source = strings.Replace(source, "with return", tc.direction+" return", 1) + "filters {\n side " + tc.side + "\n}\n"
		p, _ := dsl.Parse(source)
		r.Config = p.Config
		got, err := Run(r)
		if err != nil || got.TimedAudit.Rows[0].Status != tc.status {
			t.Fatalf("%+v: %v %+v", tc, err, got)
		}
		if tc.status == "executed" && got.Trades[0].Side != tc.tradeSide {
			t.Fatalf("%+v %+v", tc, got.Trades)
		}
	}
}

func TestTimedReturnReferenceCloseAndQuotedAvailabilityAreSeparate(t *testing.T) {
	_, source, r := timedFixture(t, false)
	// The reference session closes at 11:00 but the quote session includes 11:01.
	r.TimedCalendar.Sessions[0].Close = "11:00"
	got, err := Run(r)
	if err != nil || got.TradeCount != 1 || got.Trades[0].ExitT != r.Series.T[181] {
		t.Fatalf("%v %+v", err, got)
	}
	r.TimedCalendar.QuotedIntervals[0].ToT = int64(r.Series.T[180])
	if _, err := Run(r); err == nil {
		t.Fatal("post-close fill outside quoted hours accepted")
	}
	_, _, r = timedFixture(t, false)
	p, _ := dsl.Parse(strings.Replace(source, "current clock 10:00 completed-close", "current clock 10:00 open", 1))
	r.Config = p.Config
	r.Series.O[120] = 102
	r.Series.H[120] = 103
	got, err = Run(r)
	if err != nil || got.TradeCount != 1 || got.Trades[0].EntryT != r.Series.T[121] {
		t.Fatalf("%v %+v", err, got)
	}
	// Signal knowledge after the scheduled decision must not be borrowed.
	p, _ = dsl.Parse(strings.Replace(source, "current clock 10:00 completed-close", "current clock 10:01 completed-close", 1))
	r.Config = p.Config
	if _, err := Run(r); err == nil {
		t.Fatal("future signal endpoint accepted")
	}
}

func TestTimedReturnRawFixtureMalformedBarsFail(t *testing.T) {
	f, source, _ := timedFixture(t, false)
	f.RawBars[0] = f.RawBars[0][:5]
	if _, err := RunFixtureCase(f, source); err == nil {
		t.Fatal("malformed raw row admitted")
	}
	f, source, _ = timedFixture(t, false)
	f.RawSourceBars = [][]float64{{1}}
	if _, err := RunFixtureCase(f, source); err == nil {
		t.Fatal("malformed extra source rows ignored")
	}
}

func TestTimedReturnWholeBatchRefusalDoesNotLeakEarlierTrade(t *testing.T) {
	_, _, r := timedFixture(t, false)
	c := r.TimedCalendar
	c.TradeToDateExclusive = "2026-06-03"
	c.Sessions = append(c.Sessions, TimedReferenceSession{Date: "2026-06-02", Kind: "full", Open: "08:00", Close: "12:00", PreviousSessionDate: "2026-06-01"})
	c.QuotedIntervals[0].ToT += 86400000
	got, err := Run(r)
	if err == nil || !strings.Contains(err.Error(), "2026-06-02") || len(got.Trades) != 0 || got.TimedAudit != nil {
		t.Fatalf("partial success leaked: %v %+v", err, got)
	}
	c.Sessions[1].Kind = "early"
	c.TradeToDateExclusive = "2026-06-04"
	c.Sessions = append(c.Sessions, TimedReferenceSession{Date: "2026-06-03", Kind: "closed"})
	got, err = Run(r)
	if err != nil || got.TradeCount != 1 || len(got.TimedAudit.Rows) != 3 || got.TimedAudit.Rows[1].Status != "excluded_reference_session" || got.TimedAudit.Rows[2].Status != "excluded_reference_session" {
		t.Fatalf("dated exclusions: %v %+v", err, got)
	}
}
