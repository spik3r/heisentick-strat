package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Hand-derived invented cases for the v2 market bridge. Bars 0-13 form a nine
// bar buy Setup (bars 5-13, lowest low 88 on bar 7) that is perfected on bar 13:
// ATR14(13) = 10, stop = 88 - 0.1*10 = 87, next open 91 + 0.5 slippage = fill
// 91.5, risk distance 4.5, target 100.5, quantity 90/4.5 = 20. All expected
// numbers below are written out by hand from those facts, not read from the
// engine. Tolerance in nearly(): relative 1e-9.

var marketSetupBars = [][4]float64{
	{100, 105, 95, 100}, {100, 105, 95, 100}, {100, 105, 95, 100}, {100, 105, 95, 100}, {100, 105, 95, 101},
	{101, 108, 98, 99}, {99, 107, 97, 98}, {98, 98, 88, 97}, {97, 105, 95, 96}, {96, 104, 94, 95},
	{95, 103, 93, 94}, {94, 102, 92, 93}, {93, 101, 91, 92}, {92, 100, 90, 91},
}

func marketBars(tail ...[4]float64) [][4]float64 {
	return append(append([][4]float64{}, marketSetupBars...), tail...)
}

var (
	marketEntryAndTarget = marketBars([4]float64{91, 93, 90, 92}, [4]float64{92, 96, 91, 95}, [4]float64{95, 101, 94, 100})
	marketEntryAndStop   = marketBars([4]float64{91, 93, 90, 92}, [4]float64{92, 93, 86.5, 87.5})
	marketEntryOnly      = marketBars([4]float64{91, 93, 90, 92})
)

const marketMonday = 1709510400000

func marketSource(symbol, timeframe, policy string) string {
	return fmt.Sprintf("dsl v7\nstrategy \"Invented\" { description \"Invented valid OHLC\" }\nmarket { sequential symbol %s sequential timeframe %s }\nsetup { type: sequential full sequential profile seq.full.public_approx.v1 sequential policy %s }\nrisk { sequential riskUsd 90 sequential maxNotionalUsd 1000000 }\n", symbol, timeframe, policy)
}

// marketFixture builds the request JSON. offsets adds extra whole bars of time
// before the given bar index (a hole in the timeline).
func marketFixture(symbol, timeframe string, stepMS int64, bars [][4]float64, costs string, holes map[int]int64) string {
	var rows []string
	offset := int64(0)
	for i, b := range bars {
		offset += holes[i] * stepMS
		t := marketMonday + int64(i)*stepMS + offset
		rows = append(rows, fmt.Sprintf("[%d,%s,%s,%s,%s,1]", t, num(b[0]), num(b[1]), num(b[2]), num(b[3])))
	}
	return fmt.Sprintf(`{"schema":"dsl-conformance-run-fixture-v1","case":"invented","strategyId":"invented-e1","symbol":%q,"timeframe":%q,"rangeMethod":"zone","costs":%s,"bars":[%s]}`,
		symbol, timeframe, costs, strings.Join(rows, ","))
}

func num(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

const marketCosts = `{"feePerUnit":0.01,"fillOn":"open","slippage":0.5,"startEquity":10000}`

func decodeMarket(t *testing.T, raw []byte) sequentialMarketEnvelope {
	t.Helper()
	var envelope sequentialMarketEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("envelope: %v\n%s", err, raw)
	}
	return envelope
}

func nearly(t *testing.T, name string, got, want float64) {
	t.Helper()
	if got != want && !(math.Abs(got-want) <= 1e-9*math.Max(math.Abs(got), math.Abs(want))) {
		t.Errorf("%s = %.17g, want %.17g", name, got, want)
	}
}

func mustRun(t *testing.T, symbol, timeframe string, step int64, bars [][4]float64, costs string) (sequentialMarketEnvelope, []byte) {
	t.Helper()
	raw, ok := runSequentialMarket(marketFixture(symbol, timeframe, step, bars, costs, nil), marketSource(symbol, timeframe, "E1"))
	if !ok {
		t.Fatalf("refused: %s", raw)
	}
	return decodeMarket(t, raw), raw
}

func TestSequentialMarketHandDerivedTargetWithFees(t *testing.T) {
	// Fee 0.01 per unit on entry and exit, quantity 20: 0.2 each. Target 100.5
	// is touched on bar 16; the exit fills 0.5 worse at 100. Gross (100-91.5)*20
	// = 170; exit credit 169.8; net after both fees 169.6.
	e, _ := mustRun(t, "XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts)
	if e.Schema != "strat-sequential-market-result-v2" || e.ContractVersion != 2 || e.Error != nil {
		t.Fatalf("envelope: %+v", e)
	}
	id := e.Identity
	if id.Route.Symbol != "XAUUSD" || id.Route.Timeframe != "1h" || id.Route.TimeframeMS != 3_600_000 || id.Route.Policy != "E1" || id.Route.RiskUSD != 90 || id.Route.MaxNotionalUSD != 1_000_000 || id.BarCount != 17 {
		t.Fatalf("identity: %+v", id)
	}
	if e.Terminal.Position != "flat" || e.Terminal.PendingOrderRejected || !e.Limits.ContiguousBarsOnly || e.Limits.ClosureCalendar || e.Limits.TerminalOpen {
		t.Fatalf("terminal/limits: %+v %+v", e.Terminal, e.Limits)
	}
	if len(e.Run.Trades) != 1 {
		t.Fatalf("trades: %d", len(e.Run.Trades))
	}
	tr := e.Run.Trades[0]
	if tr.Side != "long" || tr.EntryIndex != 14 || tr.ExitIndex != 16 || tr.Reason != "tp" {
		t.Fatalf("trade: %+v", tr)
	}
	nearly(t, "entry", tr.Entry, 91.5)
	nearly(t, "exit", tr.Exit, 100)
	nearly(t, "size", tr.Size, 20)
	m := e.Metrics
	if m.Basis != "sequential-broker-close-mtm-v1" || len(m.TradeAccounting) != 1 {
		t.Fatalf("metrics: %+v", m)
	}
	nearly(t, "entryFee", m.TradeAccounting[0].EntryFee, 0.2)
	nearly(t, "exitFee", m.TradeAccounting[0].ExitFee, 0.2)
	nearly(t, "netPnl", m.TradeAccounting[0].NetPnL, 169.6)
	want := map[int]float64{0: 10000, 13: 10000, 14: 10009.8, 15: 10069.8, 16: 10169.6}
	for i, p := range m.Equity {
		w, ok := want[i]
		if !ok {
			continue
		}
		nearly(t, fmt.Sprintf("equity[%d]", i), p.Equity, w)
	}
	if len(m.Equity) != 17 {
		t.Fatalf("one mark per bar: %d", len(m.Equity))
	}
	h := m.Headline
	nearly(t, "net", h.Net, 169.6)
	nearly(t, "endEquity", h.EndEquity, 10169.6)
	nearly(t, "returnPct", h.ReturnPct, 1.696)
	nearly(t, "maxDD", h.MaxDD, 0)
	nearly(t, "maxDDpct", h.MaxDDpct, 0)
	if h.Trades != 1 || h.WinRate == nil || *h.WinRate != 100 || h.AvgHoldBars == nil || *h.AvgHoldBars != 2 {
		t.Fatalf("headline: %+v", h)
	}
	if h.ProfitFactor != nil || h.ProfitFactorReason != "no-losses" || h.AvgLoss != nil || h.AvgLossReason != "no-nonwinning-trades" || h.AvgWin == nil {
		t.Fatalf("reasons: %+v", h)
	}
}

func TestSequentialMarketHandDerivedStopAndDrawdown(t *testing.T) {
	// Bar 14 closes 92: open P&L (92-91.5)*20 = 10 less the 0.2 entry fee. Bar 15
	// stops at 87, filled 0.5 worse at 86.5: gross -100, exit credit -100.2,
	// realized -100.4. Peak 10009.8 on bar 14, trough 9899.6 on bar 15.
	e, _ := mustRun(t, "GBPUSD", "15m", 900_000, marketEntryAndStop, marketCosts)
	nearly(t, "equity[14]", e.Metrics.Equity[14].Equity, 10009.8)
	nearly(t, "equity[15]", e.Metrics.Equity[15].Equity, 9899.6)
	h := e.Metrics.Headline
	nearly(t, "net", h.Net, -100.4)
	nearly(t, "maxDD", h.MaxDD, 110.2)
	nearly(t, "maxDDpct", h.MaxDDpct, 110.2/10009.8*100)
	nearly(t, "worstLoss", h.WorstLoss, -100.4)
	if e.Run.Trades[0].Reason != "sl" || e.Run.Trades[0].ExitIndex != 15 {
		t.Fatalf("trade: %+v", e.Run.Trades[0])
	}
	if h.WinRate == nil || *h.WinRate != 0 || h.ProfitFactor == nil || *h.ProfitFactor != 0 || h.AvgWin != nil || h.AvgWinReason != "no-winning-trades" || h.AvgLoss == nil || h.MaxLossStreak != 1 || h.MaxWinStreak != 0 {
		t.Fatalf("headline: %+v", h)
	}
	nearly(t, "avgLoss", *h.AvgLoss, -100.4)
}

func TestSequentialMarketPendingTerminalOrderIsNotATrade(t *testing.T) {
	// The Setup is perfected on the last bar. There is no next open, so the
	// order is rejected as no_next_bar. The run is valid, flat and flat-equity.
	e, _ := mustRun(t, "EURUSD", "5m", 300_000, marketSetupBars, marketCosts)
	if len(e.Run.Trades) != 0 || !e.Terminal.PendingOrderRejected || e.Terminal.Position != "flat" {
		t.Fatalf("terminal: %+v trades %d", e.Terminal, len(e.Run.Trades))
	}
	if len(e.Run.SequentialFull.Opportunities) != 1 || string(e.Run.SequentialFull.Opportunities[0].Reason) != "no_next_bar" {
		t.Fatalf("audit: %+v", e.Run.SequentialFull.Opportunities)
	}
	for i, p := range e.Metrics.Equity {
		nearly(t, fmt.Sprintf("equity[%d]", i), p.Equity, 10000)
	}
	h := e.Metrics.Headline
	if h.Trades != 0 || h.WinRate != nil || h.WinRateReason != "no-trades" || h.ProfitFactorReason != "no-trades" || h.ExpectancyReason != "no-trades" || h.AvgHoldBarsReason != "no-trades" || h.AvgWinReason != "no-winning-trades" || h.AvgLossReason != "no-nonwinning-trades" {
		t.Fatalf("headline: %+v", h)
	}
	nearly(t, "maxDD", h.MaxDD, 0)
}

func TestSequentialMarketHeldTerminalPositionIsRefusedNotMarked(t *testing.T) {
	fixture := marketFixture("XAUUSD", "1h", 3_600_000, marketEntryOnly, marketCosts, nil)
	source := marketSource("XAUUSD", "1h", "E1")
	raw, ok := runSequentialMarket(fixture, source)
	e := decodeMarket(t, raw)
	if ok || e.Error == nil || e.Error.Code != "unsupported-incomplete-terminal-run" || *e.Error.BarIndex != 14 {
		t.Fatalf("want terminal refusal, got %s", raw)
	}
	if e.Run != nil || e.Metrics != nil || e.Identity != nil || e.Terminal != nil || e.Capabilities != nil || e.Limits != nil {
		t.Fatalf("refusal leaked success fields: %s", raw)
	}
	adm := decodeAdmission(t, mustAdmit(t, fixture, source))
	if adm.Runnable || adm.Disposition != "unsupported" || adm.Error.Code != "unsupported-incomplete-terminal-run" || adm.Route == nil || adm.Route.Symbol != "XAUUSD" || adm.Timeline.GapCount != 0 {
		t.Fatalf("admission: %+v", adm)
	}
}

func TestSequentialMarketWeekendHoleIsRefusedAndReported(t *testing.T) {
	// A 48 hour closure between bars 6 and 7 of a 1h series. No calendar is
	// consulted: any interval longer than one step is a refused gap.
	holes := map[int]int64{7: 48}
	fixture := marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, holes)
	source := marketSource("XAUUSD", "1h", "E1")
	raw, ok := runSequentialMarket(fixture, source)
	e := decodeMarket(t, raw)
	if ok || e.Error == nil || e.Error.Code != "unsupported-sequential-time-gap" || e.Error.BarIndex == nil || *e.Error.BarIndex != 7 || e.Run != nil || e.Metrics != nil {
		t.Fatalf("want gap refusal at bar 7: %s", raw)
	}
	adm := decodeAdmission(t, mustAdmit(t, fixture, source))
	if adm.Runnable || adm.Disposition != "unsupported" || adm.Timeline.GapCount != 1 || len(adm.Timeline.Gaps) != 1 {
		t.Fatalf("admission: %+v", adm)
	}
	g := adm.Timeline.Gaps[0]
	if g.Index != 7 || !g.OnGrid || g.MissingBars == nil || *g.MissingBars != 48 || g.DeltaMS != 49*3_600_000 || g.OpenMS-g.PreviousOpenMS != 49*3_600_000 {
		t.Fatalf("gap: %+v", g)
	}
}

func mustAdmit(t *testing.T, fixture, source string) []byte {
	t.Helper()
	raw, _ := admitSequentialMarket(fixture, source)
	return raw
}

func decodeAdmission(t *testing.T, raw []byte) sequentialMarketAdmission {
	t.Helper()
	var a sequentialMarketAdmission
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatalf("admission: %v\n%s", err, raw)
	}
	if a.Schema != "strat-sequential-market-admission-v2" || a.ContractVersion != 2 || a.Limits == nil {
		t.Fatalf("admission envelope: %s", raw)
	}
	return a
}

func TestSequentialMarketRefusalTable(t *testing.T) {
	good := marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, nil)
	source := marketSource("XAUUSD", "1h", "E1")
	legacy, err := os.ReadFile("../../conformance/parse/setup-legacy-setup9.strat")
	if err != nil {
		t.Fatal(err)
	}
	withCosts := func(costs string) string {
		return marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, costs, nil)
	}
	cases := []struct {
		name, fixture, source, code, field, disposition, message string
	}{
		{"symbol differs from source", marketFixture("EURUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, nil), source, "invalid-sequential-accounting", "route", "invalid", ""},
		{"timeframe differs from source", marketFixture("XAUUSD", "5m", 300_000, marketEntryAndTarget, marketCosts, nil), source, "invalid-sequential-accounting", "route", "invalid", ""},
		{"start equity omitted is not defaulted", withCosts(`{"feePerUnit":0.01,"fillOn":"open","slippage":0.5}`), source, "invalid-sequential-accounting", "costs.startEquity", "invalid", ""},
		{"start equity zero is not defaulted", withCosts(`{"feePerUnit":0.01,"fillOn":"open","slippage":0.5,"startEquity":0}`), source, "invalid-sequential-accounting", "costs.startEquity", "invalid", ""},
		{"start equity negative", withCosts(`{"feePerUnit":0.01,"fillOn":"open","slippage":0.5,"startEquity":-5}`), source, "invalid-sequential-accounting", "costs.startEquity", "invalid", ""},
		{"close fills are not next-open", withCosts(`{"feePerUnit":0.01,"fillOn":"close","slippage":0.5,"startEquity":10000}`), source, "unsupported_config", "costs.fillOn", "unsupported", ""},
		{"negative slippage", withCosts(`{"feePerUnit":0.01,"fillOn":"open","slippage":-0.5,"startEquity":10000}`), source, "unsupported_config", "costs.slippage", "unsupported", ""},
		{"unknown cost field", withCosts(`{"feePerUnit":0.01,"fillOn":"open","slippage":0.5,"startEquity":10000,"spread":1}`), source, "unsupported_config", "costs.spread", "unsupported", ""},
		{"legacy Setup-9 source is not a market E1/E2 source", good, string(legacy), "invalid-sequential-accounting", "source.family", "unsupported", ""},
		{"generic family source", good, "dsl v7\nstrategy \"x\" {}\nmarket { symbols: [XAUUSD] timeframes: [1h] }\nsetup { type: orb }\n", "invalid-sequential-accounting", "source.family", "unsupported", ""},
		{"P1 policy", good, strings.Replace(source, "policy E1", "policy P1", 1), "unsupported_config", "source", "unsupported", "sequentialFull.policy"},
		{"lowercase symbol", good, strings.ReplaceAll(source, "XAUUSD", "xauusd"), "unsupported_config", "source", "unsupported", "sequentialFull.symbol"},
		{"two hour timeframe", good, strings.ReplaceAll(source, "timeframe 1h", "timeframe 2h"), "unsupported_config", "source", "unsupported", "sequentialFull.timeframe"},
		{"calendar declaration", strings.Replace(good, `"bars":`, `"timedCalendar":{},"bars":`, 1), source, "unsupported-sequential-route", "timedCalendar", "unsupported", ""},
		{"unknown top-level field", strings.Replace(good, `"bars":`, `"closures":[],"bars":`, 1), source, "unsupported_config", "closures", "unsupported", ""},
		{"duplicate key", strings.Replace(good, `"case":"invented",`, `"case":"a","case":"b",`, 1), source, "invalid-sequential-input", "$.duplicate-key", "invalid", ""},
		{"fractional timestamp", strings.Replace(good, "[1709510400000,", "[1709510400000.5,", 1), source, "invalid-sequential-input", "bars.timestamp", "invalid", ""},
		{"high below low", strings.Replace(good, ",100,105,95,100,1]", ",100,94,95,100,1]", 1), source, "invalid-sequential-accounting", "bars", "invalid", ""},
		{"null price cell", strings.Replace(good, ",100,105,95,100,1]", ",null,105,95,100,1]", 1), source, "invalid-sequential-input", "$.bars", "invalid", ""},
		{"bars closer than one step", marketFixture("XAUUSD", "1h", 1_800_000, marketEntryAndTarget, marketCosts, nil), source, "invalid-sequential-accounting", "bars.timestamp", "invalid", ""},
		{"empty bars", strings.Replace(good, good[strings.Index(good, `"bars":`):], `"bars":[]}`, 1), source, "SEQUENTIAL_FIXTURE_INVALID", "bars", "invalid", ""},
		{"empty source", good, "", "SEQUENTIAL_DSL_INVALID", "source", "invalid", ""},
		{"not JSON", "{", source, "invalid-sequential-input", "$", "invalid", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, ok := runSequentialMarket(c.fixture, c.source)
			e := decodeMarket(t, raw)
			if ok || e.Error == nil {
				t.Fatalf("want refusal, got %s", raw)
			}
			if e.Error.Code != c.code || (c.field != "" && !strings.Contains(e.Error.Field, strings.TrimPrefix(c.field, "$."))) || !strings.Contains(e.Error.Message, c.message) {
				t.Fatalf("error = %+v, want %s / %s", e.Error, c.code, c.field)
			}
			if e.Run != nil || e.Metrics != nil || e.Identity != nil || e.Terminal != nil || e.Capabilities != nil || e.Limits != nil {
				t.Fatalf("refusal leaked success fields: %s", raw)
			}
			adm := decodeAdmission(t, mustAdmit(t, c.fixture, c.source))
			if adm.Runnable || adm.Disposition != c.disposition || adm.Error == nil || adm.Error.Code != c.code {
				t.Fatalf("admission = %+v (error %+v), want %s %s", adm, adm.Error, c.disposition, c.code)
			}
		})
	}
}

func TestSequentialMarketAdmissionNeverDisagreesWithRun(t *testing.T) {
	source := marketSource("XAUUSD", "1h", "E1")
	for name, fixture := range map[string]string{
		"target":      marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, nil),
		"stop":        marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndStop, marketCosts, nil),
		"no order":    marketFixture("XAUUSD", "1h", 3_600_000, marketSetupBars, marketCosts, nil),
		"open end":    marketFixture("XAUUSD", "1h", 3_600_000, marketEntryOnly, marketCosts, nil),
		"hole":        marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, map[int]int64{3: 2}),
		"tiny equity": marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, `{"feePerUnit":0.01,"fillOn":"open","slippage":0.5,"startEquity":5e-324}`, nil),
		"half step":   marketFixture("XAUUSD", "1h", 1_800_000, marketEntryAndTarget, marketCosts, nil),
		"bad route":   marketFixture("EURUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, nil),
	} {
		t.Run(name, func(t *testing.T) {
			run, runOK := runSequentialMarket(fixture, source)
			adm, admOK := admitSequentialMarket(fixture, source)
			a := decodeAdmission(t, adm)
			if runOK != a.Runnable || runOK != admOK {
				t.Fatalf("run ok=%v, admission runnable=%v", runOK, a.Runnable)
			}
			if e := decodeMarket(t, run); !runOK {
				if e.Error == nil || a.Error == nil || !reflect.DeepEqual(e.Error, a.Error) {
					t.Fatalf("different refusal: run %+v admission %+v", e.Error, a.Error)
				}
			}
		})
	}
}

func TestSequentialMarketRepeatableAndV1Untouched(t *testing.T) {
	fixture := marketFixture("XAUUSD", "1h", 3_600_000, marketEntryAndTarget, marketCosts, nil)
	source := marketSource("XAUUSD", "1h", "E1")
	a, _ := runSequentialMarket(fixture, source)
	b, _ := runSequentialMarket(fixture, source)
	if string(a) != string(b) {
		t.Fatal("repeat run changed bytes")
	}
	// The v1 surface still refuses a strategy id outside its four pinned
	// controls, then a source whose hash is not the pinned one.
	for field, input := range map[string]string{
		"strategyId":    fixture,
		"source.sha256": strings.Replace(fixture, `"invented-e1"`, `"dslSequentialFullE1"`, 1),
	} {
		_, err := runSequentialFixture(input, source)
		if err == nil {
			t.Fatalf("v1 admitted an unpinned input (%s)", field)
		}
		var v1 sequentialEnvelope
		if json.Unmarshal(sequentialFailure(err), &v1) != nil || v1.Schema != "strat-sequential-backtest-result-v1" || v1.ContractVersion != 1 || v1.Error == nil || v1.Error.Field != field {
			t.Fatalf("v1 refusal changed: %+v / %+v", v1, v1.Error)
		}
	}
}

// The documented adoption examples must keep behaving as documented.
func TestSequentialMarketAdoptionExamples(t *testing.T) {
	dir := "../../engine/testdata/sequential_market_v1"
	var index struct {
		Examples []struct {
			ID, Fixture, Source string
			Expect              struct {
				OK          bool
				Trades      int
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
			fixture, err := os.ReadFile(dir + "/" + ex.Fixture)
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(dir + "/" + ex.Source)
			if err != nil {
				t.Fatal(err)
			}
			run, ok := runSequentialMarket(string(fixture), string(source))
			e := decodeMarket(t, run)
			a := decodeAdmission(t, mustAdmit(t, string(fixture), string(source)))
			if ok != ex.Expect.OK || a.Runnable != ex.Expect.OK {
				t.Fatalf("ok=%v runnable=%v, want %v: %s", ok, a.Runnable, ex.Expect.OK, run)
			}
			if ok {
				if len(e.Run.Trades) != ex.Expect.Trades || len(e.Metrics.Equity) != e.Identity.BarCount {
					t.Fatalf("trades %d equity %d", len(e.Run.Trades), len(e.Metrics.Equity))
				}
				return
			}
			if e.Error.Code != ex.Expect.Code || a.Disposition != ex.Expect.Disposition || a.Error.Code != ex.Expect.Code {
				t.Fatalf("refusal %+v / %s", e.Error, a.Disposition)
			}
		})
	}
}

// Spacing at, below and above exactly one step, on and off the grid. Run and
// admit must agree; off-grid gaps are reported without a fabricated bar count.
func TestSequentialMarketStepBoundaries(t *testing.T) {
	const step = 3_600_000
	source := marketSource("XAUUSD", "1h", "E1")
	cases := []struct {
		name        string
		delta       int64
		code        string
		disposition string
		gapOnGrid   *bool
	}{
		{"one ms below a step", step - 1, "invalid-sequential-accounting", "invalid", nil},
		{"exactly one step", step, "", "runnable", nil},
		{"one ms above a step", step + 1, "unsupported-sequential-time-gap", "unsupported", boolPtr(false)},
		{"one and a half steps", step + step/2, "unsupported-sequential-time-gap", "unsupported", boolPtr(false)},
		{"just below two steps", 2*step - 1, "unsupported-sequential-time-gap", "unsupported", boolPtr(false)},
		{"exactly two steps", 2 * step, "unsupported-sequential-time-gap", "unsupported", boolPtr(true)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Change only the spacing between bars 6 and 7; later bars keep their
			// spacing relative to bar 7 so exactly one interval is off.
			fixture := marketFixtureWithDelta("XAUUSD", "1h", step, marketEntryAndTarget, marketCosts, 7, c.delta)
			run, runOK := runSequentialMarket(fixture, source)
			adm, admOK := admitSequentialMarket(fixture, source)
			a := decodeAdmission(t, adm)
			if runOK != admOK || runOK != a.Runnable || a.Disposition != c.disposition {
				t.Fatalf("run ok=%v admit=%+v", runOK, a)
			}
			if c.code != "" {
				e := decodeMarket(t, run)
				if e.Error == nil || e.Error.Code != c.code || a.Error == nil || a.Error.Code != c.code {
					t.Fatalf("code: run %s admit %+v", run, a.Error)
				}
			}
			if c.gapOnGrid != nil {
				if a.Timeline == nil || len(a.Timeline.Gaps) != 1 {
					t.Fatalf("timeline: %+v", a.Timeline)
				}
				g := a.Timeline.Gaps[0]
				if g.OnGrid != *c.gapOnGrid || g.DeltaMS != float64(c.delta) || (g.MissingBars != nil) != *c.gapOnGrid {
					t.Fatalf("gap: %+v", g)
				}
			}
			validateMarketSchemaShapes(t, run, adm)
		})
	}
}

func boolPtr(v bool) *bool { return &v }

// marketFixtureWithDelta sets the interval before bar `at` to exactly delta ms.
func marketFixtureWithDelta(symbol, timeframe string, stepMS int64, bars [][4]float64, costs string, at int, delta int64) string {
	var rows []string
	t := int64(marketMonday)
	for i, b := range bars {
		if i > 0 {
			if i == at {
				t += delta
			} else {
				t += stepMS
			}
		}
		rows = append(rows, fmt.Sprintf("[%d,%s,%s,%s,%s,1]", t, num(b[0]), num(b[1]), num(b[2]), num(b[3])))
	}
	return fmt.Sprintf(`{"schema":"dsl-conformance-run-fixture-v1","case":"invented","strategyId":"invented-e1","symbol":%q,"timeframe":%q,"rangeMethod":"zone","costs":%s,"bars":[%s]}`,
		symbol, timeframe, costs, strings.Join(rows, ","))
}

// validateMarketSchemaShapes is a light structural guard: the full draft 2020-12
// validation of the same envelopes runs in scripts/checks/sequential-market-parity.mjs.
func validateMarketSchemaShapes(t *testing.T, run, adm []byte) {
	t.Helper()
	var r, a map[string]any
	if json.Unmarshal(run, &r) != nil || json.Unmarshal(adm, &a) != nil {
		t.Fatal("not JSON")
	}
	if r["schema"] != "strat-sequential-market-result-v2" || a["schema"] != "strat-sequential-market-admission-v2" {
		t.Fatalf("schemas: %v %v", r["schema"], a["schema"])
	}
	for _, key := range []string{"runnable", "disposition", "limits"} {
		if _, ok := a[key]; !ok {
			t.Fatalf("admission lacks %s: %s", key, adm)
		}
	}
}

// Every early admit failure keeps the admission envelope (review of 5903a1a).
func TestSequentialMarketAdmissionFailureEnvelope(t *testing.T) {
	for name, err := range map[string]error{
		"arguments": sequentialInvalid("arguments", "expected fixture JSON and source strings"),
		"fixture":   sequentialInvalid("fixture", "fixture JSON exceeds 64 MiB"),
		"source":    &sequentialInputError{"SEQUENTIAL_DSL_INVALID", "source", sequentialSourceLimitMessage},
	} {
		t.Run(name, func(t *testing.T) {
			a := decodeAdmission(t, sequentialMarketAdmissionFailure(err))
			if a.Runnable || a.Disposition != "invalid" || a.Error == nil || a.Error.Field != name {
				t.Fatalf("admission: %+v", a)
			}
		})
	}
	// The native dispatch uses the selected operation's envelope for argument errors.
	if got := string(sequentialMarketFailure(sequentialInvalid("arguments", "x"))); !strings.Contains(got, "market-result-v2") {
		t.Fatalf("run failure: %s", got)
	}
}
