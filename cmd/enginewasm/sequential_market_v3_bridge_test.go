package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Hand-derived v3 cases. The bars are the v2 Setup (perfected on bar 13: ATR14
// = 10, stop 87, fee 0.01 per unit, slippage 0.5, quantity 20 at a fill of
// 91.5 unless said otherwise) with tails chosen so every number below can be
// written down without running the engine.

// marketCalendar returns a calendar document. holes maps bar index to the
// number of missing bars before it; closures are derived exactly from them.
func marketCalendar(symbol, timeframe string, stepMS int64, id string, holes map[int]int64, mutate func(pairs [][2]int64) [][2]int64) string {
	var keys []int
	for k := range holes {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	var pairs [][2]int64
	offset := int64(0)
	for _, k := range keys {
		start := int64(marketMonday) + (int64(k)+offset)*stepMS
		pairs = append(pairs, [2]int64{start, start + holes[k]*stepMS})
		offset += holes[k]
	}
	if mutate != nil {
		pairs = mutate(pairs)
	}
	canon, items := make([]string, len(pairs)), make([]string, len(pairs))
	for i, p := range pairs {
		canon[i] = fmt.Sprintf("[%d,%d]", p[0], p[1])
		items[i] = fmt.Sprintf(`{"startMs":%d,"endMs":%d}`, p[0], p[1])
	}
	canonical := fmt.Sprintf(`{"schema":"sequential-closure-calendar-v1","calendarId":"%s","provider":"venue-a","symbol":"%s","timeframe":"%s","version":"1","closures":[%s]}`, id, symbol, timeframe, strings.Join(canon, ","))
	sum := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf(`{"schema":"sequential-closure-calendar-v1","calendarId":"%s","provider":"venue-a","symbol":"%s","timeframe":"%s","version":"1","closures":[%s],"sha256":"%s"}`,
		id, symbol, timeframe, strings.Join(items, ","), hex.EncodeToString(sum[:]))
}

func decodeV3(t *testing.T, raw []byte) sequentialMarketV3Envelope {
	t.Helper()
	var e sequentialMarketV3Envelope
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("envelope: %v\n%s", err, raw)
	}
	return e
}

func decodeV3Admission(t *testing.T, raw []byte) sequentialMarketV3Admission {
	t.Helper()
	var a sequentialMarketV3Admission
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("admission: %v\n%s", err, raw)
	}
	if a.Schema != "strat-sequential-market-admission-v3" || a.ContractVersion != 3 || a.Limits == nil {
		t.Fatalf("admission envelope: %s", raw)
	}
	return a
}

func v3Run(t *testing.T, fixture, source, calendar string) (sequentialMarketV3Envelope, []byte) {
	t.Helper()
	raw, ok := runSequentialMarketV3(fixture, source, calendar)
	if !ok {
		t.Fatalf("refused: %s", raw)
	}
	return decodeV3(t, raw), raw
}

func TestSequentialMarketV3OpenPositionHandDerived(t *testing.T) {
	// Entry bar 14 closes 92: open P&L (92-91.5)*20 = 10; entry fee 0.2 already paid.
	source := marketSource("XAUUSD", "1h", "E1")
	fixture := marketFixture("XAUUSD", "1h", 3_600_000, marketEntryOnly, marketCosts, nil)
	e, _ := v3Run(t, fixture, source, "")
	if e.Schema != "strat-sequential-market-result-v3" || e.ContractVersion != 3 || e.Terminal.Position != "open" || e.Terminal.OpenTrade == nil {
		t.Fatalf("terminal: %+v", e.Terminal)
	}
	o := e.Terminal.OpenTrade
	if o.Side != "long" || o.EntryIndex != 14 || o.BarsHeld != 1 {
		t.Fatalf("open trade: %+v", o)
	}
	nearly(t, "entry", o.Entry, 91.5)
	nearly(t, "size", o.Size, 20)
	nearly(t, "stop", o.Stop, 87)
	nearly(t, "target", o.Target, 100.5)
	nearly(t, "markPrice", o.MarkPrice, 92)
	nearly(t, "unrealized", o.UnrealizedPnL, 10)
	nearly(t, "entryFeePaid", o.EntryFeePaid, 0.2)
	if len(e.Run.Trades) != 0 || e.Metrics.Headline.Trades != 0 || e.Metrics.Headline.WinRateReason != "no-trades" || e.Metrics.Headline.ProfitFactorReason != "no-trades" {
		t.Fatalf("completed statistics must ignore the open trade: %+v", e.Metrics.Headline)
	}
	m := e.Metrics
	nearly(t, "equity[13]", m.Equity[13].Equity, 10000)
	nearly(t, "equity[14]", m.Equity[14].Equity, 10009.8)
	nearly(t, "headline.net (completed)", m.Headline.Net, 0)
	nearly(t, "headline.endEquity (settled)", m.Headline.EndEquity, 10000)
	nearly(t, "marked.endEquity", m.Marked.EndEquity, 10009.8)
	nearly(t, "marked.net", m.Marked.Net, 9.8)
	nearly(t, "marked.returnPct", m.Marked.ReturnPct, 0.098)
	nearly(t, "maxDD", m.Headline.MaxDD, 0)
	if e.Limits.UndeclaredGaps != "refused" || !e.Limits.TerminalOpenPosition || !e.Limits.ClosureCalendar {
		t.Fatalf("limits: %+v", e.Limits)
	}
	// v2 keeps refusing the same input: the extension is a separate version.
	v2, ok := runSequentialMarket(fixture, source)
	if ok || decodeMarket(t, v2).Error.Code != "unsupported-incomplete-terminal-run" {
		t.Fatalf("v2 changed: %s", v2)
	}
}

func TestSequentialMarketV3OpenPositionDrawdownAndFees(t *testing.T) {
	// Bar 15 closes 89 with low 88.5 above the stop 87: open P&L (89-91.5)*20 = -50.
	bars := marketBars([4]float64{91, 93, 90, 92}, [4]float64{92, 93, 88.5, 89})
	e, _ := v3Run(t, marketFixture("GBPUSD", "15m", 900_000, bars, marketCosts, nil), marketSource("GBPUSD", "15m", "E1"), "")
	m := e.Metrics
	nearly(t, "equity[14]", m.Equity[14].Equity, 10009.8)
	nearly(t, "equity[15]", m.Equity[15].Equity, 9949.8)
	nearly(t, "maxDD", m.Headline.MaxDD, 60)
	nearly(t, "maxDDpct", m.Headline.MaxDDpct, 60/10009.8*100)
	nearly(t, "marked.endEquity", m.Marked.EndEquity, 9949.8)
	nearly(t, "marked.net", m.Marked.Net, -50.2)
	nearly(t, "marked.returnPct", m.Marked.ReturnPct, -0.502)
	nearly(t, "marked.openUnrealizedPnl", m.Marked.OpenUnrealizedPnL, -50)
	nearly(t, "headline.net (completed only)", m.Headline.Net, 0)
	if e.Terminal.OpenTrade.BarsHeld != 2 {
		t.Fatalf("barsHeld %d", e.Terminal.OpenTrade.BarsHeld)
	}
}

func TestSequentialMarketV3ClosureInsideSetupKeepsObservedBars(t *testing.T) {
	// The v2 target case with a 48 bar weekend between bars 6 and 7, inside the
	// nine bar Setup. Observed bars are counted across it, so every index, price
	// and number equals the contiguous case; only timestamps differ.
	holes := map[int]int64{7: 48}
	fixture := marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, holes)
	source := marketSource("XAUUSD", "1h", "E1")
	calendar := marketCalendar("XAUUSD", "1h", 3_600_000, "week-1", holes, nil)
	e, _ := v3Run(t, fixture, source, calendar)
	tr := e.Run.Trades[0]
	if tr.EntryIndex != 14 || tr.ExitIndex != 16 || tr.Reason != "tp" {
		t.Fatalf("trade: %+v", tr)
	}
	nearly(t, "entry", tr.Entry, 91.5)
	nearly(t, "exit", tr.Exit, 100)
	nearly(t, "entryT", tr.EntryT, float64(marketMonday)+float64(14+48)*3_600_000)
	if len(e.Metrics.Equity) != 17 {
		t.Fatalf("one mark per observed bar, got %d", len(e.Metrics.Equity))
	}
	for i, p := range e.Metrics.Equity {
		if i > 0 && p.T-e.Metrics.Equity[i-1].T < 3_600_000 {
			t.Fatalf("mark %d is not after the previous one", i)
		}
	}
	nearly(t, "equity[14]", e.Metrics.Equity[14].Equity, 10009.8)
	nearly(t, "equity[16]", e.Metrics.Equity[16].Equity, 10169.6)
	nearly(t, "net", e.Metrics.Headline.Net, 169.6)
	cal := e.Identity.Calendar
	if cal == nil || cal.CalendarID != "week-1" || cal.Provider != "venue-a" || cal.DeclaredClosures != 1 || cal.AppliedCount != 1 || len(cal.Applied) != 1 {
		t.Fatalf("calendar identity: %+v", cal)
	}
	if a := cal.Applied[0]; a.BeforeIndex != 7 || a.MissingBars != 48 || a.ClosureIndex != 0 || a.EndMS-a.StartMS != 48*3_600_000 {
		t.Fatalf("applied: %+v", a)
	}
	if len(cal.SHA256) != 64 || e.Terminal.Position != "flat" {
		t.Fatalf("identity/terminal: %+v %+v", cal, e.Terminal)
	}
	// The same closure with no calendar is an undeclared gap.
	raw, ok := runSequentialMarketV3(fixture, source, "")
	if ok || decodeV3(t, raw).Error.Code != "unsupported-sequential-time-gap" || *decodeV3(t, raw).Error.BarIndex != 7 {
		t.Fatalf("undeclared: %s", raw)
	}
}

func TestSequentialMarketV3FillAtTheReopenedOpen(t *testing.T) {
	// A 60 bar closure between the decision bar 13 and the entry bar 14.
	holes := map[int]int64{14: 60}
	calendar := marketCalendar("XAUUSD", "1h", 3_600_000, "gap-1", holes, nil)
	source := marketSource("XAUUSD", "1h", "E1")
	run := func(tail ...[4]float64) sequentialMarketV3Envelope {
		e, _ := v3Run(t, marketFixture("XAUUSD", "1h", 3_600_000, marketBars(tail...), marketCosts, holes), source, calendar)
		return e
	}
	// Reopens where it closed: the usual fill 91 + 0.5.
	e := run([4]float64{91, 93, 90, 92}, [4]float64{92, 96, 91, 95}, [4]float64{95, 101, 94, 100})
	nearly(t, "ordinary reopen fill", e.Run.Trades[0].Entry, 91.5)
	nearly(t, "net", e.Metrics.Headline.Net, 169.6)
	// Reopens 4 higher: fill 95.5, risk 8.5, quantity 90/8.5, target 112.5. A 114 high
	// touches it on bar 15 and fills 0.5 worse at 112: gross 16.5 * 90/8.5.
	e = run([4]float64{95, 96, 94, 95.5}, [4]float64{95.5, 114, 95, 113})
	tr := e.Run.Trades[0]
	nearly(t, "gap-up fill", tr.Entry, 95.5)
	nearly(t, "gap-up size", tr.Size, 90.0/8.5)
	nearly(t, "gap-up target", tr.TP, 112.5)
	nearly(t, "gap-up exit", tr.Exit, 112)
	nearly(t, "gap-up gross less exit fee", tr.PnL, 16.5*90/8.5-0.01*90/8.5)
	// Reopens at 86: fill 86.5 is below the stop 87, so the order is rejected and no trade exists.
	e = run([4]float64{86, 90, 85, 88}, [4]float64{88, 91, 87, 90})
	if len(e.Run.Trades) != 0 || e.Terminal.Position != "flat" {
		t.Fatalf("wrong-side reopen traded: %+v", e.Run.Trades)
	}
	o := e.Run.SequentialFull.Opportunities[0]
	if string(o.Status) != "rejected" || string(o.Reason) != "stop_breached_at_entry" || o.FillIndex != nil {
		t.Fatalf("audit: %+v", o)
	}
	nearly(t, "equity unchanged", e.Metrics.Headline.EndEquity, 10000)
}

func TestSequentialMarketV3HoldingPeriodCountsObservedBars(t *testing.T) {
	// Entry 14; no stop (87) or target (100.5) is touched. E1 holds 4 observed bars,
	// so the exit is at the open of bar 18: 95 less 0.5 slippage, gross (94.5-91.5)*20 = 60.
	tail := [][4]float64{{91, 93, 90, 92}, {92, 94, 91, 93}, {93, 95, 92, 94}, {94, 96, 93, 95}, {95, 96, 94, 95.5}}
	bars := marketBars(tail...)
	holes := map[int]int64{16: 100, 18: 5} // a closure during the position
	e, _ := v3Run(t, marketFixture("XAUUSD", "1h", 3_600_000, bars, marketCosts, holes), marketSource("XAUUSD", "1h", "E1"),
		marketCalendar("XAUUSD", "1h", 3_600_000, "mid-trade", holes, nil))
	tr := e.Run.Trades[0]
	if tr.ExitIndex != 18 || tr.Reason != "time" {
		t.Fatalf("trade: %+v", tr)
	}
	nearly(t, "exit", tr.Exit, 94.5)
	nearly(t, "credit", tr.PnL, 60-0.2)
	nearly(t, "net", e.Metrics.Headline.Net, 59.6)
	nearly(t, "exitT", tr.ExitT, float64(marketMonday)+float64(18+105)*3_600_000)
}

func TestSequentialMarketV3CalendarRefusals(t *testing.T) {
	holes := map[int]int64{7: 48}
	fixture := marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, holes)
	source := marketSource("XAUUSD", "1h", "E1")
	exact := func(mutate func([][2]int64) [][2]int64) string {
		return marketCalendar("XAUUSD", "1h", 3_600_000, "week-1", holes, mutate)
	}
	good := exact(nil)
	step := int64(3_600_000)
	cases := []struct {
		name, calendar, code, field string
		disposition                 string
	}{
		{"starts one step late", exact(func(p [][2]int64) [][2]int64 { p[0][0] += step; return p }), "unsupported-sequential-time-gap", "series.timestamp", "unsupported"},
		{"ends one step early", exact(func(p [][2]int64) [][2]int64 { p[0][1] -= step; return p }), "unsupported-sequential-time-gap", "series.timestamp", "unsupported"},
		{"covers a longer span", exact(func(p [][2]int64) [][2]int64 { p[0][1] += step; return p }), "unsupported-sequential-time-gap", "series.timestamp", "unsupported"},
		{"also covers observed bars", exact(func(p [][2]int64) [][2]int64 {
			return append([][2]int64{{int64(marketMonday) + 2*step, int64(marketMonday) + 4*step}}, p...)
		}), "invalid-sequential-calendar", "closures.contains-bar", "invalid"},
		{"declares another hole", exact(func(p [][2]int64) [][2]int64 {
			return [][2]int64{{p[0][0] + 100*step, p[0][1] + 100*step}}
		}), "unsupported-sequential-time-gap", "series.timestamp", "unsupported"},
		{"empty calendar", exact(func(p [][2]int64) [][2]int64 { return nil }), "unsupported-sequential-time-gap", "series.timestamp", "unsupported"},
		{"other symbol", strings.ReplaceAll(good, `"symbol":"XAUUSD"`, `"symbol":"EURUSD"`), "invalid-sequential-calendar", "calendar.sha256", "invalid"},
		{"edited after hashing", strings.Replace(good, `"version":"1"`, `"version":"2"`, 1), "invalid-sequential-calendar", "calendar.sha256", "invalid"},
		{"not JSON", "{", "invalid-sequential-calendar", "calendar.json", "invalid"},
		{"duplicate key", strings.Replace(good, `"provider":"venue-a"`, `"provider":"venue-a","provider":"venue-b"`, 1), "invalid-sequential-calendar", "calendar.duplicate-key", "invalid"},
		{"other timeframe", marketCalendar("XAUUSD", "4h", 14_400_000, "week-1", map[int]int64{}, nil), "invalid-sequential-calendar", "calendar.timeframe", "invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, ok := runSequentialMarketV3(fixture, source, c.calendar)
			e := decodeV3(t, raw)
			if ok || e.Error == nil || e.Error.Code != c.code || (c.field != "" && e.Error.Field != c.field) {
				t.Fatalf("run: %s", raw)
			}
			if e.Run != nil || e.Metrics != nil || e.Identity != nil || e.Terminal != nil || e.Capabilities != nil || e.Limits != nil {
				t.Fatalf("refusal leaked success fields: %s", raw)
			}
			adm, admOK := admitSequentialMarketV3(fixture, source, c.calendar)
			a := decodeV3Admission(t, adm)
			if admOK || a.Runnable || a.Disposition != c.disposition || a.Error == nil || !reflect.DeepEqual(a.Error, e.Error) {
				t.Fatalf("admission %+v vs run %+v", a, e.Error)
			}
		})
	}
	// A calendar for a different route is refused before any gap is considered.
	other := marketCalendar("EURUSD", "1h", 3_600_000, "week-1", holes, nil)
	raw, _ := runSequentialMarketV3(fixture, source, other)
	if e := decodeV3(t, raw); e.Error == nil || e.Error.Code != "invalid-sequential-calendar" || e.Error.Field != "calendar.symbol" {
		t.Fatalf("route binding: %s", raw)
	}
}

func TestSequentialMarketV3AdmissionListsUndeclaredFirst(t *testing.T) {
	holes := map[int]int64{3: 2, 7: 48, 12: 5}
	calendar := marketCalendar("XAUUSD", "1h", 3_600_000, "partial", holes, func(p [][2]int64) [][2]int64 { return p[1:2] })
	fixture := marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, holes)
	raw, ok := admitSequentialMarketV3(fixture, marketSource("XAUUSD", "1h", "E1"), calendar)
	a := decodeV3Admission(t, raw)
	if ok || a.Runnable || a.Disposition != "unsupported" || a.Calendar == nil || a.Calendar.DeclaredClosures != 1 {
		t.Fatalf("admission: %s", raw)
	}
	tl := a.Timeline
	if tl.GapCount != 3 || tl.UndeclaredGapCount != 2 || len(tl.Gaps) != 3 {
		t.Fatalf("timeline: %+v", tl)
	}
	// Undeclared gaps first (bars 3 and 12), then the declared one.
	if tl.Gaps[0].Declared || tl.Gaps[1].Declared || !tl.Gaps[2].Declared || tl.Gaps[0].Index != 3 || tl.Gaps[1].Index != 12 || tl.Gaps[2].Index != 7 {
		t.Fatalf("order: %+v", tl.Gaps)
	}
	if tl.Gaps[2].ClosureIndex == nil || *tl.Gaps[2].ClosureIndex != 0 || tl.Gaps[0].ClosureIndex != nil {
		t.Fatalf("closure index: %+v", tl.Gaps)
	}
	// No calendar: every gap is undeclared.
	raw, _ = admitSequentialMarketV3(fixture, marketSource("XAUUSD", "1h", "E1"), "")
	if a := decodeV3Admission(t, raw); a.Timeline.UndeclaredGapCount != 3 || a.Calendar != nil {
		t.Fatalf("no calendar: %s", raw)
	}
}

func TestSequentialMarketV3AdmissionNeverDisagreesWithRun(t *testing.T) {
	source := marketSource("XAUUSD", "1h", "E1")
	holes := map[int]int64{7: 48}
	calendar := marketCalendar("XAUUSD", "1h", 3_600_000, "week-1", holes, nil)
	type input struct{ fixture, source, calendar string }
	inputs := map[string]input{
		"target":           {marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, nil), source, ""},
		"open at end":      {marketFixture("XAUUSD", "1h", 3_600_000, marketEntryOnly, marketCosts, nil), source, ""},
		"no order":         {marketFixture("XAUUSD", "1h", 3_600_000, marketSetupBars, marketCosts, nil), source, ""},
		"declared hole":    {marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, holes), source, calendar},
		"undeclared hole":  {marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, holes), source, ""},
		"tiny equity":      {marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, `{"feePerUnit":0.01,"fillOn":"open","slippage":0.5,"startEquity":5e-324}`, nil), source, ""},
		"tiny equity open": {marketFixture("XAUUSD", "1h", 3_600_000, marketEntryOnly, `{"feePerUnit":0.01,"fillOn":"open","slippage":0.5,"startEquity":5e-324}`, nil), source, ""},
		"half step":        {marketFixture("XAUUSD", "1h", 1_800_000, marketEntryAndTarget, marketCosts, nil), source, ""},
		"bad route":        {marketFixture("EURUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, nil), source, ""},
		"bad calendar":     {marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, holes), source, "{"},
		"legacy source":    {marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, nil), "dsl v7\nstrategy \"x\" {}\nmarket { symbols: [XAUUSD] timeframes: [1h] }\nsetup { type: orb }\n", ""},
	}
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			run, runOK := runSequentialMarketV3(in.fixture, in.source, in.calendar)
			adm, admOK := admitSequentialMarketV3(in.fixture, in.source, in.calendar)
			a := decodeV3Admission(t, adm)
			if runOK != admOK || runOK != a.Runnable {
				t.Fatalf("run ok=%v admission %+v", runOK, a)
			}
			if !runOK && !reflect.DeepEqual(decodeV3(t, run).Error, a.Error) {
				t.Fatalf("different refusal: %s vs %+v", run, a.Error)
			}
		})
	}
}

func TestSequentialMarketV3FailureEnvelopesFollowTheOperation(t *testing.T) {
	for name, err := range map[string]error{
		"arguments": sequentialInvalid("arguments", "expected three strings"),
		"calendar":  sequentialInvalid("calendar", "calendar JSON exceeds 4 MiB"),
	} {
		t.Run(name, func(t *testing.T) {
			a := decodeV3Admission(t, sequentialMarketV3AdmissionFailure(err))
			if a.Runnable || a.Disposition != "invalid" || a.Error == nil || a.Error.Field != name {
				t.Fatalf("admission: %+v", a)
			}
			if e := decodeV3(t, sequentialMarketV3Failure(err)); e.Schema != "strat-sequential-market-result-v3" || e.Error == nil {
				t.Fatalf("run: %+v", e)
			}
		})
	}
}

func TestSequentialMarketV3AdoptionExamples(t *testing.T) {
	dir := "../../engine/testdata/sequential_market_v3"
	var index struct {
		Examples []struct {
			ID, Fixture, Source, Calendar string
			Expect                        struct {
				OK          bool
				Trades      int
				Open        bool
				Code        string
				Disposition string
			}
		}
	}
	raw, err := os.ReadFile(dir + "/EXAMPLES.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &index); err != nil || len(index.Examples) == 0 {
		t.Fatalf("index: %v", err)
	}
	for _, ex := range index.Examples {
		t.Run(ex.ID, func(t *testing.T) {
			read := func(name string) string {
				if name == "" {
					return ""
				}
				b, err := os.ReadFile(dir + "/" + name)
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			fixture, source, calendar := read(ex.Fixture), read(ex.Source), read(ex.Calendar)
			run, ok := runSequentialMarketV3(fixture, source, calendar)
			adm, admOK := admitSequentialMarketV3(fixture, source, calendar)
			if ok != ex.Expect.OK || admOK != ex.Expect.OK {
				t.Fatalf("ok=%v admit=%v want %v: %s", ok, admOK, ex.Expect.OK, run)
			}
			e := decodeV3(t, run)
			if ok {
				if len(e.Run.Trades) != ex.Expect.Trades || (e.Terminal.Position == "open") != ex.Expect.Open || len(e.Metrics.Equity) != e.Identity.BarCount {
					t.Fatalf("trades %d open %v marks %d", len(e.Run.Trades), e.Terminal.Position, len(e.Metrics.Equity))
				}
				return
			}
			a := decodeV3Admission(t, adm)
			if e.Error.Code != ex.Expect.Code || a.Disposition != ex.Expect.Disposition {
				t.Fatalf("refusal %+v / %s", e.Error, a.Disposition)
			}
		})
	}
}

// Astra's review of df1e8cd: case-variant keys must not override or stand in for
// the schema-defined calendar fields. Each mutation edits only the calendar text
// of a committed example and keeps its original digest, so a permissive decoder
// would still see a matching hash and a runnable request.
func TestSequentialMarketV3CalendarAliasKeysAreRefused(t *testing.T) {
	dir := "../../engine/testdata/sequential_market_v3/"
	read := func(name string) string {
		b, err := os.ReadFile(dir + name)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(b))
	}
	fixture, source := read("xauusd-1h-e1-open-position-after-closure.fixture.json"), read("xauusd-1h-e1-open-position-after-closure.strat")+"\n"
	valid := read("xauusd-1h-e1-open-position-after-closure.calendar.json")
	closure := `{"startMs":1709560800000,"endMs":1709776800000}`
	if !strings.Contains(valid, closure) {
		t.Fatal("example calendar changed; update the mutations")
	}
	if raw, ok := runSequentialMarketV3(fixture, source, valid); !ok || decodeV3(t, raw).Terminal.Position != "open" {
		t.Fatalf("the unmodified example must run: %s", raw)
	}
	mutations := map[string]string{
		"Closures overrides empty closures": strings.Replace(valid, `"closures":[`+closure+`]`, `"closures":[],"Closures":[`+closure+`]`, 1),
		"Closures replaces closures":        strings.Replace(valid, `"closures":`, `"Closures":`, 1),
		"Provider overrides provider":       strings.Replace(valid, `"provider":"invented-venue"`, `"provider":"other","Provider":"invented-venue"`, 1),
		"Symbol overrides symbol":           strings.Replace(valid, `"symbol":"XAUUSD"`, `"symbol":"EURUSD","Symbol":"XAUUSD"`, 1),
		"StartMs replaces startMs":          strings.Replace(valid, `"startMs"`, `"StartMs"`, 1),
		"StartMs overrides startMs":         strings.Replace(valid, `{"startMs":1709560800000,`, `{"startMs":0,"StartMs":1709560800000,`, 1),
		"endms replaces endMs":              strings.Replace(valid, `"endMs"`, `"endms"`, 1),
		"startMs omitted":                   strings.Replace(valid, `"startMs":1709560800000,`, ``, 1),
		"endMs omitted":                     strings.Replace(valid, `,"endMs":1709776800000`, ``, 1),
		"escaped alias of closures":         strings.Replace(valid, `"closures":[`+closure+`]`, `"closures":[`+closure+`],"clo\u0073ures":[]`, 1),
		"long s alias of closures":          strings.Replace(valid, `"closures":[`+closure+`]`, `"closures":[],"clo\u017fures":[`+closure+`]`, 1),
		"unknown nested key":                strings.Replace(valid, closure, `{"startMs":1709560800000,"endMs":1709776800000,"note":"x"}`, 1),
	}
	for name, calendar := range mutations {
		t.Run(name, func(t *testing.T) {
			if calendar == valid {
				t.Fatal("mutation did not change the calendar")
			}
			run, runOK := runSequentialMarketV3(fixture, source, calendar)
			adm, admOK := admitSequentialMarketV3(fixture, source, calendar)
			e, a := decodeV3(t, run), decodeV3Admission(t, adm)
			if runOK || admOK || e.Error == nil || e.Error.Code != "invalid-sequential-calendar" || a.Runnable || a.Disposition != "invalid" || !reflect.DeepEqual(a.Error, e.Error) {
				t.Fatalf("accepted or disagreed: run %s admit %s", run, adm)
			}
			if e.Run != nil || e.Metrics != nil || e.Identity != nil || e.Terminal != nil || a.Calendar != nil {
				t.Fatalf("refusal leaked success fields: %s", run)
			}
		})
	}
}
