package engine

import (
	"encoding/json"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func bidAskTestRequest(t testing.TB) PaperBidAskRequest {
	t.Helper()
	old := paperORBRequest(t, "close")
	r := PaperBidAskRequest{Schema: PaperBidAskRequestSchema, Source: old.Source, Route: old.Route, Identity: BidAskIdentity{strings.Repeat("a", 64), strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64), strings.Repeat("4", 64)}, Product: BidAskProduct{"oanda-v20", "XAU_USD", strings.Repeat("5", 64), "USD", "USD/troy-ounce", "troy-ounce", "1"}, Economics: BidAskEconomics{"1000", "0", "0", "0", "0", "0.25", "0.25", "10000", "none", "0"}, Policy: BidAskPolicy{PaperBidAskModel, "5000", "290000", "refuse-discontinuous-exposure", "decimal-rational-family-shortest-v1"}, ActivationFromT: bidAskStamp(int64(old.Bars[0][0]) * 1000000), Records: []BidAskRecord{}}
	for _, row := range old.Bars[:4] {
		b := bidAskTestBar(int64(row[0])*1000000, row[1], row[2], row[3], row[4])
		bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
	}
	return r
}
func bidAskTestBar(t int64, o, h, l, c float64) BidAskBar {
	n := func(f float64) BidAskDecimal { return BidAskDecimal(strconv.FormatFloat(f, 'f', -1, 64)) }
	v := BidAskOHLC{n(o), n(h), n(l), n(c)}
	return BidAskBar{OpenT: bidAskStamp(t), CloseT: bidAskStamp(t + 300000000000), ReceivedT: bidAskStamp(t + 300000000001), ProviderID: strings.Repeat("6", 64), ProvenanceDigest: strings.Repeat("7", 64), Mid: v, Bid: v, Ask: v, PriceCount: "1", Complete: true}
}
func bidAskAppend(r *PaperBidAskRequest, v BidAskRecord) {
	v.Sequence = strconv.Itoa(len(r.Records) + 1)
	v.Epoch = r.Identity.Epoch
	r.Records = append(r.Records, v)
}
func bidAskTestQuote(r PaperBidAskRequest, ns int64, bid, ask string, signal string) BidAskQuote {
	return BidAskQuote{ProviderT: bidAskStamp(ns), RequestStartT: bidAskStamp(ns - 1000000), ReceivedT: bidAskStamp(ns + 1000000), QualifiedT: bidAskStamp(ns + 2000000), ProviderID: strings.Repeat("8", 64), ProvenanceDigest: strings.Repeat("9", 64), AccountDigest: r.Identity.AccountDigest, Instrument: r.Product.Instrument, Tradeable: true, Bids: []BidAskTier{{BidAskDecimal(bid), "10000"}}, Asks: []BidAskTier{{BidAskDecimal(ask), "10000"}}, EpisodeSignalSequence: signal}
}
func bidAskAddEntry(r *PaperBidAskRequest, bid, ask string) {
	close, _ := bidAskTime(r.Records[len(r.Records)-1].Bar.CloseT)
	signal := r.Records[len(r.Records)-1].Sequence
	e := BidAskEpisode{signal, bidAskStamp(close + 100000000), bidAskStamp(close + 200000000000)}
	bidAskAppend(r, BidAskRecord{Kind: "episode", Episode: &e, CommitT: e.AdmittedT})
	q := bidAskTestQuote(*r, close+1000000000, bid, ask, signal)
	bidAskAppend(r, BidAskRecord{Kind: "quote", Quote: &q, CommitT: bidAskStamp(close + 1003000000)})
}

// Exercise the public source gate and actual family for every replay fixture.
// No synthetic order/fill is injected into this runner.
func bidAskTestRun(t *testing.T, r PaperBidAskRequest) PaperBidAskResult {
	t.Helper()
	v, e := RunPaperBidAsk(r)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func bidAskTestDirect(t *testing.T, s side, risk, stop, target float64) (*bidAskRunner, BidAskRecord) {
	t.Helper()
	r := bidAskTestRequest(t)
	r.Economics = BidAskEconomics{"1000", "0.1", "0.1", "0.1", "10", "0.25", "0.25", "10000", "none", "0"}
	p, b, e := prepareBidAsk(r, true)
	if e != nil {
		t.Fatal(e)
	}
	x := newBidAskRunner(r, p, b)
	x.barIndex = 3
	x.lastBarOpen, _ = bidAskTime(r.Records[3].Bar.OpenT)
	x.lastBarClose, _ = bidAskTime(r.Records[3].Bar.CloseT)
	x.pending = &bidAskDecision{view: BidAskPending{DecisionID: strings.Repeat("a", 64), SignalSequence: "4"}, order: order{Side: s, SL: stop, TP: target, RiskUSD: risk, HasRisk: true, Index: 3}, signal: *r.Records[3].Bar, index: 3}
	q := bidAskTestQuote(r, x.lastBarClose+1000000000, "100", "102", "4")
	rec := BidAskRecord{Sequence: "6", Epoch: r.Identity.Epoch, Kind: "quote", Quote: &q, CommitT: bidAskStamp(x.lastBarClose + 1003000000)}
	x.fill(rec, &q)
	if x.err != nil {
		t.Fatal(x.err)
	}
	return x, rec
}
func TestPaperBidAskIndependentExactEconomics(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		side                             side
		risk, stop, target               float64
		entry, equity, stopFill, balance string
	}{{"long", sideLong, 46, 98, 110, "102.202", "976.98", "97.802", "954"}, {"short", sideShort, 46.04, 104, 90, "99.8", "977", "104.204", "953.96"}} {
		t.Run(tc.name, func(t *testing.T) {
			x, rec := bidAskTestDirect(t, tc.side, tc.risk, tc.stop, tc.target)
			if x.position == nil {
				t.Fatal(x.events)
			}
			a, _ := x.collector.snapshot()
			if x.position.view.Entry != tc.entry || x.position.view.Quantity != "10" || a.Balance != "999" || a.Equity != tc.equity {
				t.Fatalf("entry: %+v %+v", x.position.view, a)
			}
			x.close(rec, bidAskRat(BidAskDecimal(strconv.FormatFloat(tc.stop, 'f', -1, 64))), "stop", "modeled-side-bar-stop-first", BidAskClock{})
			a, s := x.collector.snapshot()
			if a.Balance != tc.balance || s.NetPnL != "-"+strconv.FormatFloat(tc.risk, 'f', -1, 64) || s.Losers != 1 || x.events[len(x.events)-2].Price != tc.stopFill {
				t.Fatalf("stop: %+v %+v %+v", a, s, x.events)
			}
		})
	}
	for _, tc := range []struct{ price, balance, profit string }{{"110", "1073.88", "73.88"}, {"96", "934.02", "-65.98"}} {
		x, rec := bidAskTestDirect(t, sideLong, 46, 98, 120)
		x.close(rec, bidAskRat(BidAskDecimal(tc.price)), "test", "modeled-side-close", BidAskClock{})
		a, s := x.collector.snapshot()
		if a.Balance != tc.balance || s.NetPnL != tc.profit {
			t.Fatalf("independent timed/gap: %+v %+v", a, s)
		}
	}
}
func TestPaperBidAskQuantityBoundaryAndLimits(t *testing.T) {
	for _, tc := range []struct{ budget, loss, quantum, want string }{{"46", "4.6", "0.25", "10"}, {"45.999999999999", "4.6", "0.25", "9.75"}, {"46.000000000001", "4.6", "0.25", "10"}, {"0.1", "0.3", "0.1", "0.3"}} {
		got := bidAskCash(bidAskFloorQuantity(bidAskRat(BidAskDecimal(tc.budget)), bidAskRat(BidAskDecimal(tc.loss)), bidAskRat(BidAskDecimal(tc.quantum))))
		if got != tc.want {
			t.Fatalf("quantity %v: %s", tc, got)
		}
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*PaperBidAskRequest)
		outcome string
	}{{"liquidity", func(r *PaperBidAskRequest) { r.Records[5].Quote.Asks[0].Liquidity = "199.99" }, "insufficient-entry-liquidity"}, {"minimum", func(r *PaperBidAskRequest) { r.Economics.QuantityMinimum = "200.25" }, "below-minimum-quantity"}, {"maximum", func(r *PaperBidAskRequest) { r.Economics.QuantityMaximum = "199.75" }, "above-maximum-quantity"}, {"notional", func(r *PaperBidAskRequest) { r.Economics.NotionalPolicy = "cap"; r.Economics.NotionalMaximum = "20239" }, "notional-cap"}, {"breached", func(r *PaperBidAskRequest) { r.Records[5].Quote.Bids[0].Price = "99" }, "invalid-or-breached-protection"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := bidAskTestRequest(t)
			bidAskAddEntry(&r, "101", "101.2")
			tc.mutate(&r)
			got := bidAskTestRun(t, r)
			if got.Position != nil || got.Account.Balance != "1000" || got.Events[len(got.Events)-1].Outcome != tc.outcome {
				t.Fatalf("%+v", got)
			}
		})
	}
}
func bidAskPhaseRequest(t *testing.T) PaperBidAskRequest {
	r := bidAskTestRequest(t)
	bidAskAddEntry(&r, "101", "101.2")
	entry, _ := bidAskTime(r.Records[5].Quote.ProviderT)
	q := bidAskTestQuote(r, entry+1000000000, "101.3", "101.5", "")
	bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &q, CommitT: bidAskStamp(entry + 1003000000)})
	start, _ := bidAskTime(r.Records[3].Bar.CloseT)
	straddle := bidAskTestBar(start, 101, 106, 90, 101.4)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &straddle, CommitT: straddle.ReceivedT})
	boundary := bidAskTestQuote(r, start+300000000000, "99", "99.2", "")
	bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &boundary, CommitT: bidAskStamp(start + 300003000000)})
	full := bidAskTestBar(start+300000000000, 101.3, 104, 99, 102)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &full, CommitT: full.ReceivedT})
	return r
}
func TestPaperBidAskExclusivePhaseAndPreEntryExtrema(t *testing.T) {
	r := bidAskPhaseRequest(t)
	short := r
	short.Records = r.Records[:8]
	got := bidAskTestRun(t, short)
	if got.Position == nil || got.Account.Balance != "1000" || got.CompletedTrades.Completed != 0 || got.Account.Equity != "1020" {
		t.Fatalf("straddle extrema applied or mark changed: %+v", got)
	}
	full := bidAskTestRun(t, r)
	if full.Position != nil || full.Account.Balance != "800" || full.CompletedTrades.NetPnL != "-200" {
		t.Fatalf("full stop-first: %+v", full)
	}
	for _, e := range full.Events {
		if e.Kind == "exit" && (e.Price != "100.2" || e.Basis != "modeled-side-bar-stop-first" || e.Clock.ProviderT != "" || e.Clock.IntervalOpenT != r.Records[9].Bar.OpenT) {
			t.Fatalf("fabricated tick or competing boundary quote: %+v", e)
		}
	}
}
func TestPaperBidAskSampledExitUsesObservedSide(t *testing.T) {
	r := bidAskTestRequest(t)
	bidAskAddEntry(&r, "101", "101.2")
	at, _ := bidAskTime(r.Records[5].Quote.ProviderT)
	q := bidAskTestQuote(r, at+1000000000, "100", "100.2", "")
	bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &q, CommitT: bidAskStamp(at + 1003000000)})
	got := bidAskTestRun(t, r)
	if got.Account.Balance != "760" || got.CompletedTrades.NetPnL != "-240" {
		t.Fatalf("sampled stop must fill observed100: %+v", got)
	}
	r.Records[6].Quote.Bids[0].Liquidity = "199.99"
	got = bidAskTestRun(t, r)
	if got.Position == nil || got.Account.Balance != "1000" || !strings.Contains(got.Coverage, "insufficient-sampled-exit-liquidity") || got.Account.MarkStatus != "stale-unresolved" {
		t.Fatalf("unfillable exit: %+v", got)
	}
}
func TestPaperBidAskManagementPredicatesMatchOriginalExhaustively(t *testing.T) {
	series := marketdata.NewSeries(9)
	for i := range series.T {
		series.T[i] = float64(i * 300000)
		series.O[i] = 100
		series.C[i] = 101
	}
	for _, s := range []side{sideLong, sideShort} {
		for _, hold := range []float64{-10, -0.5, 0, 0.1, 1, 1.5, 2, 999, math.SmallestNonzeroFloat64} {
			for entry := 0; entry < 9; entry++ {
				for index := 0; index < 9; index++ {
					for _, held := range []bool{false, true} {
						b := broker{series: series, params: flagParams{MaxHoldBars: hold}, hasPosition: held, position: position{Side: s, Entry: 100, EntryIndex: entry, Size: 1}}
						want := bidAskDueHold(&b, index)
						b.exitAfterBars(index)
						if (held && !b.hasPosition) != want {
							t.Fatalf("hold mismatch side%d hold%g entry%d index%d held%v", s, hold, entry, index, held)
						}
					}
				}
			}
		}
	}
	for _, end := range []any{nil, "300000", int(300000), int64(300000), float32(300000), float64(300000), float64(300000.5), -1.0, 0.0} {
		for index := 0; index < 9; index++ {
			for _, held := range []bool{false, true} {
				b := broker{series: series, hasPosition: held, position: position{Side: sideLong, Entry: 100, Size: 1, Meta: TradeMeta{"slotEnd": end}}}
				want := bidAskDueSlot(&b, index)
				b.closeExpiredWindowPosition(index)
				if (held && !b.hasPosition) != want {
					t.Fatalf("slot mismatch %T %v index%d", end, end, index)
				}
			}
		}
	}
}
func TestPaperBidAskBreakevenUsesUnchangedCurrentStopArithmetic(t *testing.T) {
	for _, s := range []side{sideLong, sideShort} {
		b := broker{series: marketdata.Series{C: []float64{100 + float64(s)*2, 100 + float64(s)*0.1}}, cols: contextcols.Columns{ATR: []float64{2, 2}}, params: flagParams{BreakevenR: .5, BreakevenOffsetATR: .05}, hasPosition: true, position: position{Side: s, Entry: 100, SL: 100 - float64(s)*4, Size: 1}}
		b.moveStopToBreakeven(0)
		want := 100 + float64(s)*.1
		if b.position.SL != want {
			t.Fatalf("breakeven %.17g !=%.17g", b.position.SL, want)
		}
		b.moveStopToBreakeven(1)
		if b.position.SL != want {
			t.Fatal("stop loosened")
		}
	}
}
func TestPaperBidAskStrictInputAndReviewedAdmission(t *testing.T) {
	r := bidAskTestRequest(t)
	raw, _ := json.Marshal(r)
	if _, e := DecodePaperBidAskRequest(raw); e != nil {
		t.Fatal(e)
	}
	mutations := [][]byte{[]byte(`{"schema":"x","schema":"y"}`), []byte(strings.Replace(string(raw), `"startEquity":"1000"`, `"startEquity":1000`, 1)), []byte(strings.Replace(string(raw), `"entryFee":"0",`, "", 1)), []byte(strings.Replace(string(raw), `"complete":true`, `"complete":null`, 1)), []byte(strings.Replace(string(raw), `"source":`, `"Source":`, 1)), append(raw, []byte(" {}")...)}
	for i, v := range mutations {
		if _, e := DecodePaperBidAskRequest(v); e == nil {
			t.Fatalf("accepted malformed %d", i)
		}
	}
	if _, e := RunPaperBidAsk(r); e != nil {
		t.Fatalf("reviewed public admission: %v", e)
	}
}
func TestPaperBidAskExactDecimalNoBinaryQuantityFuzz(t *testing.T) {
	v := newRatQuo(big.NewRat(1, 1), big.NewRat(10, 1))
	if bidAskCash(v) != "0.1" {
		t.Fatal(v)
	}
	for _, s := range []BidAskDecimal{"-1", "+1", "01", ".1", "1.", "1e3", "NaN", "0.0000000000001", "1000000000000"} {
		if _, e := bidAskNumber(s); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	for _, s := range []string{"2025-01-01T00:00:00.123456789Z", "2025-01-01T00:00:00Z"} {
		n, e := bidAskTime(s)
		if e != nil || bidAskStamp(n) != s {
			t.Fatalf("nanosecond transport %s", s)
		}
	}
}
func TestPaperBidAskSyntheticArtifactRequests(t *testing.T) {
	dir := os.Getenv("BIDASK_FIXTURES_DIR")
	if dir == "" {
		return
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	r := bidAskPhaseRequest(t)
	raw, _ := json.Marshal(r)
	if e := os.WriteFile(filepath.Join(dir, "long-phase-request.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	expected := bidAskTestRun(t, r)
	raw, _ = json.Marshal(expected)
	if e := os.WriteFile(filepath.Join(dir, "long-phase-expected.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	// Reflection enforces that the end-of-prefix view is not aliased to a mutable
	// checkpoint position loaded from outside the native process.
	again := bidAskTestRun(t, r)
	if !reflect.DeepEqual(expected, again) {
		t.Fatal("non-deterministic synthetic request")
	}
}
func TestPaperBidAskShortNativeFamilyAndOpeningGap(t *testing.T) {
	r := bidAskTestRequest(t)
	start, _ := bidAskTime(r.Records[3].Bar.OpenT)
	bar := bidAskTestBar(start, 100, 100.25, 98.5, 99)
	r.Records[3].Bar = &bar
	r.Records[3].CommitT = bar.ReceivedT
	bidAskAddEntry(&r, "98.8", "99")
	got := bidAskTestRun(t, r)
	if got.Position == nil || got.Position.Side != "short" || got.Position.Entry != "98.8" || got.Position.Stop != "99.8" || got.Position.Target != "96.8" || got.Account.Equity != "960" {
		t.Fatalf("short Go family: %+v", got)
	}
	straddle := bidAskTestBar(start+300000000000, 99, 101, 90, 98.8)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &straddle, CommitT: straddle.ReceivedT})
	gap := bidAskTestBar(start+600000000000, 101, 102, 96, 99)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &gap, CommitT: gap.ReceivedT})
	closed := bidAskTestRun(t, r)
	if closed.Account.Balance != "560" || closed.CompletedTrades.NetPnL != "-440" {
		t.Fatalf("short opening-gap: %+v", closed)
	}
}
func TestPaperBidAskUTCSlotOpenBeforeBracketExtrema(t *testing.T) {
	r := bidAskTestRequest(t)
	bidAskAddEntry(&r, "101", "101.2")
	start, _ := bidAskTime(r.Records[3].Bar.CloseT)
	slotEnd := start - start%(3*3600000000000) + 3*3600000000000
	for at := start; at < slotEnd; at += 300000000000 {
		b := bidAskTestBar(at, 101.2, 101.5, 100.8, 101.2)
		bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
	}
	boundary := bidAskTestBar(slotEnd, 100, 105, 90, 104)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &boundary, CommitT: boundary.ReceivedT})
	got := bidAskTestRun(t, r)
	if got.Account.Balance != "760" {
		t.Fatalf("slot must close at100 before bracket: %+v", got)
	}
	found := false
	for _, e := range got.Events {
		if e.Kind == "exit" {
			found = true
			if e.Outcome != "window-close" || e.Basis != "modeled-side-open" || e.Clock.ModelEffectiveT != boundary.OpenT || e.Clock.ReceivedT != boundary.ReceivedT || e.Clock.ProviderT != "" {
				t.Fatalf("slot clocks %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("no slot close")
	}
}
func TestPaperBidAskBreakevenProtectionFutureOnly(t *testing.T) {
	r := bidAskTestRequest(t)
	r.Source = strings.Replace(r.Source, "after 999R plus 0 ATR", "after 0.5R plus 0 ATR", 1)
	bidAskAddEntry(&r, "101", "101.2")
	start, _ := bidAskTime(r.Records[3].Bar.CloseT)
	straddle := bidAskTestBar(start, 101.2, 101.5, 100.3, 101.4)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &straddle, CommitT: straddle.ReceivedT})
	trigger := bidAskTestBar(start+300000000000, 101.4, 102, 100.3, 101.8)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &trigger, CommitT: trigger.ReceivedT})
	open := bidAskTestRun(t, r)
	if open.Position == nil || open.Position.Stop != "101.2" || open.Position.ProtectionVersion != 2 || open.CompletedTrades.Completed != 0 {
		t.Fatalf("new stop applied to elapsed extrema: %+v", open)
	}
	found := false
	for _, e := range open.Events {
		if e.Kind == "protection" {
			found = true
			if e.PriorProtectionVersion != 1 || e.ProtectionVersion != 2 || e.Clock.ModelEffectiveT != trigger.CloseT {
				t.Fatal(e)
			}
		}
	}
	if !found {
		t.Fatal("no authoritative protection event")
	}
	next := bidAskTestBar(start+600000000000, 101.3, 101.4, 101.1, 101.3)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &next, CommitT: next.ReceivedT})
	closed := bidAskTestRun(t, r)
	if closed.Position != nil || closed.Account.Balance != "1000" || closed.CompletedTrades.Flat != 1 {
		t.Fatalf("future stop: %+v", closed)
	}
}
func TestPaperBidAskTimedCloseUsesSideCloseAndSignalIndex(t *testing.T) {
	r := bidAskTestRequest(t)
	r.Source += "\nmanagement { maxHoldCandles 1 }\n"
	bidAskAddEntry(&r, "101", "101.2")
	start, _ := bidAskTime(r.Records[3].Bar.CloseT)
	b := bidAskTestBar(start, 101.2, 101.6, 100.8, 101.5)
	b.Bid.Close = "101.4"
	b.Ask.Close = "101.6"
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
	got := bidAskTestRun(t, r)
	if got.Account.Balance != "1040" || got.CompletedTrades.NetPnL != "40" {
		t.Fatalf("time uses source index and side close: %+v", got)
	}
}

func TestPaperBidAskPublicAuthoredSourceMatrix(t *testing.T) {
	for _, extra := range []string{
		"", "# partial half at 1R is only a comment\nmanagement { trail off }",
		"strategy \"Independent synthetic ORB\" { description \"partial and trailing metadata\" }",
		"management {\n breakeven off\n trail off\n wait 3.5 candles after trade\n maxHoldCandles 1.4\n}",
		"filters {\n higher timeframe off\n entryTf current\n}",
		"market conditions {\n session phase in (open middle lunch close overnight other)\n local weekday in (Monday Tue)\n weekday not in (Fri)\n local hour in (9 10)\n hour not in (23)\n open location in (nearPDH near DO other)\n prior day type in (range trend wide narrow outside)\n prior day range at least 0.5 ATR\n day type in (trending ranging choppy) strictly\n day theme in (join_momentum buy_lows sell_highs)\n movement below 999\n}",
		"filters {\n rmv ATR period 3\n rmv lookback 5\n rmv at least 0\n}",
		"market conditions {\n seasonality intraday all in (bullish extreme_bullish) min samples 1\n seasonality day of week 30d supports entry\n seasonality week of year lookback all in (neutral)\n seasonality monthly history aligns entry\n}",
	} {
		t.Run("accept/"+extra, func(t *testing.T) {
			r := bidAskTestRequest(t)
			r.Records = nil // Admission cannot depend on seeing a favorable outcome.
			r.Route.StrategyID = "independent-authored-identity"
			r.Source += "\n" + extra + "\n"
			cfg := orbParseAdmission(t, r.Source)
			before, _ := json.Marshal(cfg)
			if err := bidAskAdmitSource(r.Source, cfg, r.Route); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(cfg)
			if string(before) != string(after) {
				t.Fatal("admission mutated ordinary Parse config")
			}
			v := bidAskTestRun(t, r)
			if v.Binding.Source.SHA256 != paperSourceDigest(r.Source) || v.Binding.ConfigIdentity.SHA256 != bidAskHash(cfg) || v.ProcessedRecords != 0 || len(v.Events) != 0 {
				t.Fatal("public admission changed source/config binding")
			}
		})
	}
	for _, tc := range []struct{ extra, code string }{
		{"management { partial half at 1R }", "PARTIAL_UNQUALIFIED"},
		{"management { partial half at 1R }\nsetup { type: opening range breakout }", "PARTIAL_UNQUALIFIED"},
		{"management { trail 1 ATR after 1R }", "TRAILING_UNQUALIFIED"},
		{"management {\n trail 1 ATR after 1R\n trail off\n}", "TRAILING_UNQUALIFIED"},
		{"management { trail 1 ATR after 1R }\nsetup { type: opening range breakout }", "TRAILING_UNQUALIFIED"},
		{"filters {\n higher timeframe must agree\n higher timeframe off\n}", "ADMISSION"},
		{"filters {\n source timeframe 1h\n entryTf current\n}", "ADMISSION"},
		{"setup { retest off }", "ADMISSION"},
		{"target {\n target oppositeRangeEdge\n target 20 pips\n}", "SOURCE"},
		{"market conditions { new york hour in (1) }", "ADMISSION"},
		{"execution {\n enter limit at swept edge\n enter market\n}", "ADMISSION"},
		{"management { breakeven off } partial 0.5 at 1R", "ADMISSION"},
		{"filters { approach 0 ATR side both }", "ADMISSION"},
		{"filters { side both } unexpected discarded text", "ADMISSION"},
		{"management { breakeven after 1R trail off }", "ADMISSION"},
		{"setup {\n type: vwap extension fade\n type: opening range breakout\n}", "ADMISSION"},
	} {
		t.Run("refuse/"+tc.extra, func(t *testing.T) {
			r := bidAskTestRequest(t)
			r.Records = nil
			r.Source += "\n" + tc.extra + "\n"
			v, err := RunPaperBidAsk(r)
			if err == nil || err.Error() != "PAPER_BIDASK_"+tc.code || !reflect.DeepEqual(v, PaperBidAskResult{}) {
				t.Fatalf("source refusal: %v %+v", err, v)
			}
		})
	}
	r := bidAskTestRequest(t)
	r.Records = nil
	r.Route.Timeframe = "15m"
	if _, err := RunPaperBidAsk(r); err == nil || err.Error() != "PAPER_BIDASK_ROUTE_UNSUPPORTED" {
		t.Fatalf("source route was ignored: %v", err)
	}
}
