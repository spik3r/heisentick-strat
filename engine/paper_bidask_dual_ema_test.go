package engine

import (
	"encoding/json"
	"github.com/spik3r/heisentick-strat/dsl"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const dualPaperSource = `dsl v7
strategy "Independent dual EMA paper fixture" { description "Synthetic closed bars; no profitability claim." }
market conditions { slices(XAUUSD 4h) }
setup {
 type: dual ema resumption
 ema fast 20
 ema slow 80
 ema slow rise 12
 ema wilder atr 20
 stop initial 2.5 ATR
 trail close 3 ATR
 fallback below slow ema
}
filters { side long only }
execution { risk: 200 USD }
`

func dualPaperRequest(t testing.TB) PaperBidAskRequest {
	r := bidAskTestRequest(t)
	r.Source = dualPaperSource
	r.Route.Timeframe = "4h"
	r.Route.RangeMethod = "zone"
	r.Route.StrategyID = "dual-paper-fixture"
	r.Records = nil
	r.Policy.MaxSignalAgeMS = "290000"
	start := int64(1735689600000000000)
	duration := int64(14400000000000)
	r.ActivationFromT = bidAskStamp(start + 90*duration)
	for i := 0; i < 90; i++ {
		dualAppendBar(&r, start+int64(i)*duration, 100+float64(i)*.2)
	}
	return r
}
func dualAppendBar(r *PaperBidAskRequest, at int64, price float64) {
	b := bidAskTestBar(at, price, price+.5, price-.5, price)
	b.CloseT = bidAskStamp(at + 14400000000000)
	b.ReceivedT = bidAskStamp(at + 14400000000001)
	bidAskAppend(r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
}
func TestPaperBidAskDualEMAWarmupAndPendingEntry(t *testing.T) {
	r := dualPaperRequest(t)
	v := bidAskTestRun(t, r)
	if v.Position != nil || v.Pending != nil || len(v.Events) != 0 {
		t.Fatalf("warmup executed: %+v", v)
	}
	at, _ := bidAskTime(r.ActivationFromT)
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+14400000000000, 119)
	v = bidAskTestRun(t, r)
	if v.Pending == nil {
		t.Fatalf("missing pending: %+v", v)
	}
	bidAskAddEntry(&r, "118.9", "119.1")
	v = bidAskTestRun(t, r)
	if v.Position == nil || !v.Position.NoTarget || v.Position.Target != "0" || v.Position.Quantity == "0" {
		t.Fatalf("entry: %+v", v)
	}
	if v.Binding.Family != "dualEmaResumption" {
		t.Fatal(v.Binding)
	}
	entryID := v.Position.ID
	stop := v.Position.Stop
	dualAppendBar(&r, at+2*14400000000000, 123)
	v = bidAskTestRun(t, r)
	if v.Position == nil || v.Position.ID != entryID || v.Position.Stop == stop || v.Position.ProtectionVersion < 2 {
		t.Fatalf("trail missing: %+v", v)
	}
	full := v
	for split := 0; split < len(r.Records); split++ {
		prefix := r
		prefix.Records = r.Records[:split]
		old := bidAskTestRun(t, prefix)
		resume := r
		resume.Checkpoint = old.Checkpoint
		got := bidAskTestRun(t, resume)
		if got.StateDigest != full.StateDigest || !reflect.DeepEqual(got.Position, full.Position) {
			t.Fatalf("split %d diverged", split)
		}
		events := append(append([]BidAskEvent{}, old.Events...), got.Events...)
		if !reflect.DeepEqual(events, full.Events) {
			t.Fatalf("split %d events", split)
		}
	}
	bad := r
	bad.Checkpoint = append(json.RawMessage{}, v.Checkpoint...)
	bad.Source += "\n# change binding\n"
	if _, err := RunPaperBidAsk(bad); err == nil {
		t.Fatal("changed source checkpoint accepted")
	}
}
func TestPaperBidAskDualEMASourceRefusals(t *testing.T) {
	for _, extra := range []string{"management { trail 1 ATR after 1R }", "filters { side both }", "filters { side both }\nfilters { side long only }", "setup { type: opening range breakout }", "filters { parabolic sar bullish }"} {
		r := dualPaperRequest(t)
		r.Source += "\n" + extra
		if _, err := RunPaperBidAsk(r); err == nil {
			t.Fatalf("admitted %s", extra)
		}
	}
	for _, value := range []string{"0", "1.5", "2147483648"} {
		r := dualPaperRequest(t)
		r.Source = strings.Replace(r.Source, "ema fast 20", "ema fast "+value+"\n ema fast 20", 1)
		if _, err := RunPaperBidAsk(r); err == nil {
			t.Fatalf("erased invalid EMA count %s admitted", value)
		}
	}
	r := dualPaperRequest(t)
	r.Route.Timeframe = "1h"
	if _, err := RunPaperBidAsk(r); err == nil {
		t.Fatal("wrong route")
	}
	parsed, err := dsl.Parse(dualPaperSource)
	if err != nil || len(parsed.Errors) > 0 {
		t.Fatal(err, parsed.Errors)
	}
}

func TestPaperBidAskDualEMASlowEMANextOpenAndStop(t *testing.T) {
	for _, slowExit := range []bool{false, true} {
		r := dualPaperRequest(t)
		at, _ := bidAskTime(r.ActivationFromT)
		duration := int64(14400000000000)
		dualAppendBar(&r, at, 114)
		dualAppendBar(&r, at+duration, 119)
		bidAskAddEntry(&r, "118.9", "119.1")
		entry := bidAskTestRun(t, r)
		id := entry.Position.ID
		if slowExit {
			// Wide, valid bars keep ATR high while the slow EMA catches up. They
			// retain the frozen 3 ATR trail and do not touch the initial stop.
			for i := 2; i < 62; i++ {
				dualAppendBar(&r, at+int64(i)*duration, 123)
				b := r.Records[len(r.Records)-1].Bar
				for _, leg := range []*BidAskOHLC{&b.Mid, &b.Bid, &b.Ask} {
					leg.High = "140"
					leg.Low = "117"
				}
			}
			dualAppendBar(&r, at+62*duration, 118)
			before := bidAskTestRun(t, r)
			if before.Position == nil {
				t.Fatal("closed before fallback open")
			}
			stale := r
			stale.Records = append([]BidAskRecord{}, r.Records...)
			delayedQuote := bidAskTestQuote(r, at+63*duration+1000000000, "118.4", "118.6", "")
			bidAskAppend(&stale, BidAskRecord{Kind: "quote", Quote: &delayedQuote, CommitT: bidAskStamp(at + 63*duration + 7000000000)})
			staleResult := bidAskTestRun(t, stale)
			if staleResult.Position == nil || staleResult.Coverage != "unresolved-exposure:unqualified-slow-ema-exit-quote" {
				t.Fatal("stale delayed commit executed fallback")
			}
			early := bidAskTestQuote(r, at+63*duration+1000000000, "118.4", "118.6", "")
			early.RequestStartT = bidAskStamp(at + 63*duration - 1000000000)
			bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &early, CommitT: early.QualifiedT})
			delayed := bidAskTestRun(t, r)
			if delayed.Position == nil {
				t.Fatal("predecision quote request executed fallback")
			}
			q := bidAskTestQuote(r, at+63*duration+2000000000, "118.4", "118.6", "")
			bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &q, CommitT: q.QualifiedT})
			immediate := bidAskTestRun(t, r)
			if immediate.Position != nil {
				t.Fatal("slow EMA did not close at next quote")
			}
			dualAppendBar(&r, at+63*duration, 118.5)
		} else {
			dualAppendBar(&r, at+2*duration, 119)
			dualAppendBar(&r, at+3*duration, 110)
		}
		full := bidAskTestRun(t, r)
		if full.Position != nil {
			t.Fatalf("position did not close: %+v", full.Position)
		}
		exits := 0
		for _, e := range full.Events {
			if e.Kind == "exit" {
				exits++
				if e.PositionID != id {
					t.Fatal("position identity changed")
				}
				if slowExit && (e.Outcome != "slow-ema" || e.Basis != "observed-liquidation-side-with-modeled-costs") {
					t.Fatalf("fallback %+v", e)
				}
			}
		}
		if exits != 1 {
			t.Fatalf("expected one close got %d", exits)
		}
		for _, split := range []int{90, 92, 93, len(r.Records) - 1, len(r.Records)} {
			prefix := r
			prefix.Records = r.Records[:split]
			prior := bidAskTestRun(t, prefix)
			resume := r
			resume.Checkpoint = prior.Checkpoint
			got := bidAskTestRun(t, resume)
			if got.StateDigest != full.StateDigest {
				t.Fatalf("recovery split %d", split)
			}
			again := r
			again.Checkpoint = got.Checkpoint
			duplicate := bidAskTestRun(t, again)
			if len(duplicate.Events) != 0 {
				t.Fatal("duplicate close effects")
			}
		}
	}
}

func TestPaperBidAskDualEMATrailIsFutureOnlyAndGapFailsClosed(t *testing.T) {
	r := dualPaperRequest(t)
	at, _ := bidAskTime(r.ActivationFromT)
	duration := int64(14400000000000)
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+duration, 119)
	bidAskAddEntry(&r, "118.9", "119.1")
	dualAppendBar(&r, at+2*duration, 123)
	prior := bidAskTestRun(t, r)
	oldStop, err := strconv.ParseFloat(prior.Position.Stop, 64)
	if err != nil {
		t.Fatal(err)
	}
	dualAppendBar(&r, at+3*duration, math.Ceil(oldStop)+10)
	bar := r.Records[len(r.Records)-1].Bar
	low := BidAskDecimal(strconv.FormatFloat(math.Ceil(oldStop), 'f', -1, 64))
	for _, leg := range []*BidAskOHLC{&bar.Mid, &bar.Bid, &bar.Ask} {
		leg.Low = low
	}
	after := bidAskTestRun(t, r)
	if after.Position == nil {
		t.Fatal("close-derived stop applied to its own bar")
	}
	newStop, _ := strconv.ParseFloat(after.Position.Stop, 64)
	if newStop <= math.Ceil(oldStop) {
		t.Fatal("fixture did not put low below the new stop")
	}
	dualAppendBar(&r, at+5*duration, math.Ceil(oldStop)+11)
	gap := bidAskTestRun(t, r)
	if gap.Position == nil || gap.Coverage != "unresolved-exposure:missing-completed-interval" {
		t.Fatalf("gap did not preserve unresolved exposure: %+v", gap)
	}
	for _, event := range gap.Events {
		if event.Kind == "exit" {
			t.Fatal("invented gap exit")
		}
	}
}

func TestPaperBidAskDualEMAEntryCommitFreshnessKeepsORBContract(t *testing.T) {
	gold := dualPaperRequest(t)
	at, _ := bidAskTime(gold.ActivationFromT)
	d := int64(14400000000000)
	dualAppendBar(&gold, at, 114)
	dualAppendBar(&gold, at+d, 119)
	bidAskAddEntry(&gold, "118.9", "119.1")
	original := bidAskTestRun(t, gold)
	if original.Position == nil {
		t.Fatal("fresh gold control did not fill")
	}
	quote := &gold.Records[len(gold.Records)-1]
	provider, _ := bidAskTime(quote.Quote.ProviderT)
	quote.CommitT = bidAskStamp(provider + 6000000000)
	expired := bidAskTestRun(t, gold)
	if expired.Position != nil {
		t.Fatal("delayed gold entry commit filled")
	}
	found := false
	for _, e := range expired.Events {
		if e.Kind == "decision-outcome" && e.Outcome == "original-quote-expired" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing explicit expired outcome")
	}
	orb := bidAskTestRequest(t)
	bidAskAddEntry(&orb, "101", "101.2")
	quote = &orb.Records[len(orb.Records)-1]
	provider, _ = bidAskTime(quote.Quote.ProviderT)
	quote.CommitT = bidAskStamp(provider + 6000000000)
	if got := bidAskTestRun(t, orb); got.Position == nil {
		t.Fatal("existing ORB episode contract changed")
	}
}
