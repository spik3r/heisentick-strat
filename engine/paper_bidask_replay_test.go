package engine

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func bidAskExpectError(t *testing.T, r PaperBidAskRequest, code string) {
	t.Helper()
	p, b, e := prepareBidAsk(r, true)
	if e == nil {
		_, e = replayBidAsk(r, p, b)
	}
	if e == nil || e.Error() != code {
		t.Fatalf("want %s got %v", code, e)
	}
}
func TestPaperBidAskEverySplitAndRepeatedZeroEventReplay(t *testing.T) {
	r := bidAskPhaseRequest(t)
	full := bidAskTestRun(t, r)
	for split := 0; split <= len(r.Records); split++ {
		t.Run(strconv.Itoa(split), func(t *testing.T) {
			prefix := r
			prefix.Records = r.Records[:split]
			first := bidAskTestRun(t, prefix)
			same := prefix
			same.Checkpoint = first.Checkpoint
			again := bidAskTestRun(t, same)
			if len(again.Events) != 0 || again.EventRange.From != first.EventRange.To || string(again.Checkpoint) != string(first.Checkpoint) {
				t.Fatal("same-prefix replay changed")
			}
			extended := r
			extended.Checkpoint = first.Checkpoint
			last := bidAskTestRun(t, extended)
			events := append(append([]BidAskEvent{}, first.Events...), last.Events...)
			if !reflect.DeepEqual(events, full.Events) || !reflect.DeepEqual(last.Account, full.Account) || !reflect.DeepEqual(last.Position, full.Position) || !reflect.DeepEqual(last.CompletedTrades, full.CompletedTrades) || last.StateDigest != full.StateDigest || string(last.Checkpoint) != string(full.Checkpoint) {
				t.Fatalf("split%d drift", split)
			}
		})
	}
}
func TestPaperBidAskRecoveryOriginalQuoteAndDurableSeal(t *testing.T) {
	r := bidAskTestRequest(t)
	bidAskAddEntry(&r, "101", "101.2")
	committed := bidAskTestRun(t, r)
	if committed.Position == nil {
		t.Fatal("quote not committed")
	}
	again := r
	again.Checkpoint = committed.Checkpoint
	v := bidAskTestRun(t, again)
	if len(v.Events) != 0 || v.Position.ID != committed.Position.ID {
		t.Fatal("committed quote duplicated")
	}
	// Crash after episode creation, before original quote transaction commit.
	lost := r
	lost.Records = append([]BidAskRecord{}, r.Records[:5]...)
	created := bidAskTestRun(t, lost)
	if created.Pending == nil {
		t.Fatal("missing episode")
	}
	at, _ := bidAskTime(r.Records[5].Quote.ProviderT)
	seal := BidAskSeal{"4", "recovery", bidAskStamp(at + 1000000000)}
	bidAskAppend(&lost, BidAskRecord{Kind: "seal", Seal: &seal, CommitT: seal.EffectiveT})
	lost.Checkpoint = created.Checkpoint
	expired := bidAskTestRun(t, lost)
	if expired.Position != nil || expired.Pending != nil || expired.Events[len(expired.Events)-1].Outcome != "expired-recovery" {
		t.Fatalf("expiry: %+v", expired)
	}
	q := bidAskTestQuote(r, at+2000000000, "101", "101.2", "4")
	bidAskAppend(&lost, BidAskRecord{Kind: "quote", Quote: &q, CommitT: bidAskStamp(at + 2003000000)})
	lost.Checkpoint = nil
	bidAskExpectError(t, lost, "PAPER_BIDASK_EPISODE_STATE")
	// A new unbound quote cannot create an entry, and the actual attempt/seen
	// state is retained, so the still-beyond-range next bar does not retry it.
	lost.Records[len(lost.Records)-1].Quote.EpisodeSignalSequence = ""
	start, _ := bidAskTime(r.Records[3].Bar.CloseT)
	bar := bidAskTestBar(start, 101, 102.5, 100.75, 102)
	bidAskAppend(&lost, BidAskRecord{Kind: "bar", Bar: &bar, CommitT: bar.ReceivedT})
	final := bidAskTestRun(t, lost)
	if final.Position != nil || final.Pending != nil || final.Account.PaidFees != "0" {
		t.Fatal("recovery quote rescued original attempt")
	}
}
func TestPaperBidAskPendingEpisodeCannotAdvanceActionableFamily(t *testing.T) {
	r := bidAskTestRequest(t)
	start, _ := bidAskTime(r.Records[3].Bar.CloseT)
	b := bidAskTestBar(start, 101, 102, 100, 101)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
	bidAskExpectError(t, r, "PAPER_BIDASK_UNRESOLVED_EPISODE")
}
func TestPaperBidAskReceiptAndInputMutationAtomicRefusal(t *testing.T) {
	r := bidAskPhaseRequest(t)
	prefix := r
	prefix.Records = r.Records[:6]
	cp := bidAskTestRun(t, prefix).Checkpoint
	for _, key := range []string{"schema", "bindingDigest", "inputDigest", "recordCount", "eventCount", "eventDigest", "stateDigest"} {
		mutated := r
		var v map[string]any
		if e := json.Unmarshal(cp, &v); e != nil {
			t.Fatal(e)
		}
		if _, ok := v[key].(float64); ok {
			v[key] = v[key].(float64) + 1
		} else {
			v[key] = "corrupted"
		}
		mutated.Checkpoint, _ = json.Marshal(v)
		bidAskExpectError(t, mutated, "PAPER_BIDASK_CHECKPOINT")
	}
	mutations := []func(*PaperBidAskRequest){func(r *PaperBidAskRequest) { r.Source += "\n// drift\n" }, func(r *PaperBidAskRequest) { r.Identity.PrincipalDigest = strings.Repeat("a", 64) }, func(r *PaperBidAskRequest) { r.Economics.EntryFee = "0.1" }, func(r *PaperBidAskRequest) { r.Records[0].Bar.Mid.Close = "100.1" }, func(r *PaperBidAskRequest) { r.Records[5].Quote.Asks[0].Price = "101.3" }, func(r *PaperBidAskRequest) { r.Policy.MaxQuoteAgeMS = "4000" }}
	for i, mutate := range mutations {
		raw, _ := json.Marshal(r)
		var changed PaperBidAskRequest
		_ = json.Unmarshal(raw, &changed)
		changed.Checkpoint = cp
		mutate(&changed)
		p, b, e := prepareBidAsk(changed, true)
		if e == nil {
			var result PaperBidAskResult
			result, e = replayBidAsk(changed, p, b)
			if result.Schema != "" {
				t.Fatalf("mutation%d leaked partial result", i)
			}
		}
		if e == nil {
			t.Fatalf("mutation%d accepted", i)
		}
	}
}
func TestPaperBidAskDuplicateConflictAndLateObservations(t *testing.T) {
	r := bidAskTestRequest(t)
	bidAskAddEntry(&r, "101", "101.2")
	q := *r.Records[5].Quote
	q.EpisodeSignalSequence = ""
	q.ProviderID = strings.Repeat("b", 64)
	q.Bids = append([]BidAskTier{}, q.Bids...)
	q.Asks = append([]BidAskTier{}, q.Asks...)
	q.Bids[0].Price = "101.0"
	commit, _ := bidAskTime(r.Records[5].CommitT)
	bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &q, CommitT: bidAskStamp(commit + 1)})
	got := bidAskTestRun(t, r)
	if got.Events[len(got.Events)-1].Outcome != "duplicate-provider-observation" || got.Account.MarkCount != 1 {
		t.Fatal("HTTP cursor converted duplicate to new tick")
	}
	r.Records[6].Quote.Bids[0].Liquidity = "9999"
	bidAskExpectError(t, r, "PAPER_BIDASK_QUOTE_CONFLICT")
	r = bidAskPhaseRequest(t)
	r.Records = r.Records[:8]
	entry, _ := bidAskTime(r.Records[5].Quote.ProviderT)
	late := bidAskTestQuote(r, entry+2000000000, "99", "99.2", "")
	end, _ := bidAskTime(r.Records[7].CommitT)
	late.ReceivedT = bidAskStamp(end + 1000000)
	late.QualifiedT = bidAskStamp(end + 2000000)
	bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &late, CommitT: bidAskStamp(end + 3000000)})
	got = bidAskTestRun(t, r)
	if got.Position == nil || !strings.Contains(got.Coverage, "late-straddle-observation") || got.CompletedTrades.Completed != 0 {
		t.Fatalf("late observation fabricated exit: %+v", got)
	}
}
func TestPaperBidAskMissingCoverageCannotResumeExposure(t *testing.T) {
	for _, kind := range []string{"missing-bar", "gap"} {
		t.Run(kind, func(t *testing.T) {
			r := bidAskPhaseRequest(t)
			r.Records = r.Records[:6]
			start, _ := bidAskTime(r.Records[3].Bar.CloseT)
			if kind == "gap" {
				g := BidAskGap{bidAskStamp(start + 2000000000), bidAskStamp(start + 3000000000), "disconnect"}
				bidAskAppend(&r, BidAskRecord{Kind: "coverage-gap", Gap: &g, CommitT: g.ThroughT})
			}
			b := bidAskTestBar(start+300000000000, 101, 110, 90, 101)
			bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
			later := bidAskTestBar(start+600000000000, 101, 110, 90, 101)
			bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &later, CommitT: later.ReceivedT})
			got := bidAskTestRun(t, r)
			if got.Position == nil || got.CompletedTrades.Completed != 0 || got.Account.MarkStatus != "stale-unresolved" {
				t.Fatalf("discontinuity recovered silently: %+v", got)
			}
		})
	}
}
func TestPaperBidAskExactBoundaryEntryAndDelayedBar(t *testing.T) {
	r := bidAskTestRequest(t)
	close, _ := bidAskTime(r.Records[3].Bar.CloseT)
	e := BidAskEpisode{"4", bidAskStamp(close + 1), bidAskStamp(close + 200000000000)}
	bidAskAppend(&r, BidAskRecord{Kind: "episode", Episode: &e, CommitT: bidAskStamp(close + 2)})
	q := bidAskTestQuote(r, close, "101", "101.2", "4")
	q.RequestStartT = bidAskStamp(close + 2)
	bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &q, CommitT: bidAskStamp(close + 3000000)})
	entry := bidAskTestRun(t, r)
	if entry.Position == nil || entry.Position.FullPhaseFromT != q.ProviderT {
		t.Fatal("exact boundary entry phase")
	}
	q2 := bidAskTestQuote(r, close+1000000000, "99", "99.2", "")
	bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &q2, CommitT: bidAskStamp(close + 1003000000)})
	observed := bidAskTestRun(t, r)
	if observed.Position == nil || observed.Account.MarkCount != 1 {
		t.Fatal("full phase quote financial authority")
	}
	bar := bidAskTestBar(close, 101, 104, 99, 102)
	bar.ReceivedT = bidAskStamp(close + 900000000000)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &bar, CommitT: bar.ReceivedT})
	got := bidAskTestRun(t, r)
	if got.Account.Balance != "800" {
		t.Fatalf("late completed bar %s", got.Account.Balance)
	}
}
func TestPaperBidAskWarmupDoesNotConsumeAttempt(t *testing.T) {
	r := bidAskTestRequest(t)
	r.ActivationFromT = r.Records[3].Bar.CloseT
	warm := bidAskTestRun(t, r)
	if len(warm.Events) != 0 || warm.Pending != nil {
		t.Fatal("warmup consumed signal")
	}
	start, _ := bidAskTime(r.ActivationFromT)
	inside := bidAskTestBar(start, 100, 100.5, 99.5, 100)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &inside, CommitT: inside.ReceivedT})
	breakout := bidAskTestBar(start+300000000000, 100, 101.5, 99.75, 101)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &breakout, CommitT: breakout.ReceivedT})
	r.Checkpoint = warm.Checkpoint
	fresh := bidAskTestRun(t, r)
	if fresh.Pending == nil || fresh.Pending.SignalSequence != "6" {
		t.Fatalf("fresh opportunity unavailable: %+v", fresh)
	}
}
func TestPaperBidAskNativeResourceLimits(t *testing.T) {
	r := bidAskTestRequest(t)
	r.Source = strings.Repeat(" ", PaperBidAskMaxSourceBytes+1)
	bidAskExpectError(t, r, "PAPER_BIDASK_INPUT")
	r = bidAskTestRequest(t)
	r.Records = make([]BidAskRecord, PaperBidAskMaxRecords+1)
	bidAskExpectError(t, r, "PAPER_BIDASK_INPUT")
	if _, e := DecodePaperBidAskRequest(make([]byte, PaperBidAskMaxBytes+1)); e == nil {
		t.Fatal("unbounded request")
	}
	if _, e := DecodePaperBidAskRequest([]byte(strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66))); e == nil {
		t.Fatal("unbounded depth")
	}
	r = bidAskTestRequest(t)
	r.ActivationFromT = bidAskStamp(time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC).UnixNano())
	r.Records = nil
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	for i := 0; i < PaperBidAskMaxBars; i++ {
		b := bidAskTestBar(start+int64(i)*300000000000, 100, 101, 99, 100)
		bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
	}
	before := time.Now()
	v := bidAskTestRun(t, r)
	t.Logf("full-prefix %d bars, %d records, elapsed %s", v.ProcessedBars, v.ProcessedRecords, time.Since(before))
	if v.ProcessedBars != PaperBidAskMaxBars || len(v.Events) != 0 {
		t.Fatal("resource prefix truncated")
	}
	b := bidAskTestBar(start+int64(PaperBidAskMaxBars)*300000000000, 100, 101, 99, 100)
	bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
	bidAskExpectError(t, r, "PAPER_BIDASK_BAR_LIMIT")
}
func TestPaperBidAskAdversarialRecordClocksAndShapes(t *testing.T) {
	base := bidAskTestRequest(t)
	bidAskAddEntry(&base, "101", "101.2")
	cases := []struct {
		name, code string
		mutate     func(*PaperBidAskRequest)
	}{
		{"old-epoch", "SEQUENCE", func(r *PaperBidAskRequest) { r.Records[5].Epoch = strings.Repeat("f", 64) }},
		{"sequence-hole", "SEQUENCE", func(r *PaperBidAskRequest) { r.Records[5].Sequence = "7" }},
		{"commit-regression", "COMMIT_ORDER", func(r *PaperBidAskRequest) { r.Records[5].CommitT = r.Records[0].CommitT }},
		{"incomplete-bar", "BAR", func(r *PaperBidAskRequest) { r.Records[0].Bar.Complete = false }},
		{"crossed-quote", "QUOTE_SPREAD", func(r *PaperBidAskRequest) { r.Records[5].Quote.Bids[0].Price = "102" }},
		{"account-mismatch", "QUOTE", func(r *PaperBidAskRequest) { r.Records[5].Quote.AccountDigest = strings.Repeat("f", 64) }},
		{"depth-limit", "QUOTE", func(r *PaperBidAskRequest) { r.Records[5].Quote.Bids = make([]BidAskTier, 9) }},
		{"two-payloads", "RECORD", func(r *PaperBidAskRequest) { r.Records[5].Seal = &BidAskSeal{} }},
		{"no-fractional-volume", "VOLUME", func(r *PaperBidAskRequest) { r.Records[0].Bar.PriceCount = "1.1" }},
		{"provider-future", "QUOTE_CLOCK", func(r *PaperBidAskRequest) {
			at, _ := bidAskTime(r.Records[5].Quote.ReceivedT)
			r.Records[5].Quote.ProviderT = bidAskStamp(at + 1)
		}},
		{"episode-before-candle-commit", "EPISODE_DEADLINE", func(r *PaperBidAskRequest) { r.Records[4].Episode.AdmittedT = r.Records[3].Bar.CloseT }},
		{"oversized-deadline", "EPISODE_DEADLINE", func(r *PaperBidAskRequest) {
			at, _ := bidAskTime(r.Records[3].Bar.CloseT)
			r.Records[4].Episode.DeadlineT = bidAskStamp(at + 300000000001)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var r PaperBidAskRequest
			_ = json.Unmarshal(raw, &r)
			tc.mutate(&r)
			bidAskExpectError(t, r, "PAPER_BIDASK_"+tc.code)
		})
	}
}
func TestPaperBidAskOriginalExpiryCannotBeRefreshed(t *testing.T) {
	for _, tc := range []string{"stale", "late-commit", "pre-episode-request", "not-tradeable"} {
		t.Run(tc, func(t *testing.T) {
			r := bidAskTestRequest(t)
			bidAskAddEntry(&r, "101", "101.2")
			q := r.Records[5].Quote
			at, _ := bidAskTime(q.ProviderT)
			switch tc {
			case "stale":
				q.ReceivedT = bidAskStamp(at + 6000000000)
				q.QualifiedT = q.ReceivedT
				r.Records[5].CommitT = q.ReceivedT
			case "late-commit":
				d, _ := bidAskTime(r.Records[4].Episode.DeadlineT)
				r.Records[5].CommitT = bidAskStamp(d + 1)
			case "pre-episode-request":
				q.RequestStartT = r.Records[3].Bar.CloseT
			case "not-tradeable":
				q.Tradeable = false
			}
			got := bidAskTestRun(t, r)
			if got.Position != nil || got.Pending != nil || got.Account.Balance != "1000" {
				t.Fatalf("original expiry admitted: %+v", got)
			}
		})
	}
}

func bidAskCorrectionSideRequest(t *testing.T, short bool) PaperBidAskRequest {
	t.Helper()
	r := bidAskTestRequest(t)
	if short {
		start, _ := bidAskTime(r.Records[3].Bar.OpenT)
		b := bidAskTestBar(start, 100, 100.25, 98.5, 99)
		r.Records[3].Bar = &b
		r.Records[3].CommitT = b.ReceivedT
		bidAskAddEntry(&r, "98.8", "99")
	} else {
		bidAskAddEntry(&r, "101", "101.2")
	}
	return r
}
func bidAskCorrectionAssertEverySplit(t *testing.T, r PaperBidAskRequest) {
	t.Helper()
	full := bidAskTestRun(t, r)
	for split := 0; split <= len(r.Records); split++ {
		prefix := r
		prefix.Records = r.Records[:split]
		prior := bidAskTestRun(t, prefix)
		prefix.Checkpoint = prior.Checkpoint
		same := bidAskTestRun(t, prefix)
		if len(same.Events) != 0 || same.StateDigest != prior.StateDigest || string(same.Checkpoint) != string(prior.Checkpoint) {
			t.Fatalf("split%d repeated prefix changed", split)
		}
		extended := r
		extended.Checkpoint = prior.Checkpoint
		last := bidAskTestRun(t, extended)
		events := append(append([]BidAskEvent{}, prior.Events...), last.Events...)
		if !reflect.DeepEqual(events, full.Events) || last.StateDigest != full.StateDigest || string(last.Checkpoint) != string(full.Checkpoint) || !reflect.DeepEqual(last.Account, full.Account) || !reflect.DeepEqual(last.Position, full.Position) {
			t.Fatalf("split%d causal output changed", split)
		}
	}
}
func TestPaperBidAskHistoricalQuoteIdentityEveryStateAndSplit(t *testing.T) {
	for _, short := range []bool{false, true} {
		for _, state := range []string{"open", "closed", "unresolved"} {
			t.Run(strconv.FormatBool(short)+"-"+state, func(t *testing.T) {
				r := bidAskCorrectionSideRequest(t, short)
				at, _ := bidAskTime(r.Records[5].Quote.ProviderT)
				bid, ask := "101.3", "101.5"
				if short {
					bid, ask = "98.7", "98.9"
				}
				if state != "open" {
					bid, ask = "100", "100.2"
					if short {
						bid, ask = "99.8", "100"
					}
				}
				newer := bidAskTestQuote(r, at+1000000000, bid, ask, "")
				if state == "unresolved" {
					if short {
						newer.Asks[0].Liquidity = "199.99"
					} else {
						newer.Bids[0].Liquidity = "199.99"
					}
				}
				bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &newer, CommitT: newer.QualifiedT})
				before := bidAskTestRun(t, r)
				old := *r.Records[5].Quote
				old.EpisodeSignalSequence = ""
				old.Bids = append([]BidAskTier{}, old.Bids...)
				old.Asks = append([]BidAskTier{}, old.Asks...)
				old.ProviderID = strings.Repeat("b", 64)
				old.ProvenanceDigest = strings.Repeat("c", 64)
				old.RequestStartT = bidAskStamp(at + 1500000000)
				old.ReceivedT = bidAskStamp(at + 1501000000)
				old.QualifiedT = bidAskStamp(at + 1502000000)
				if short {
					old.Bids[0].Price = "98.800"
				} else {
					old.Bids[0].Price = "101.0"
				}
				bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &old, CommitT: old.QualifiedT})
				got := bidAskTestRun(t, r)
				if got.Coverage != before.Coverage || !reflect.DeepEqual(got.Account, before.Account) || !reflect.DeepEqual(got.Position, before.Position) || !reflect.DeepEqual(got.CompletedTrades, before.CompletedTrades) || got.StateDigest != before.StateDigest || got.Events[len(got.Events)-1].Outcome != "duplicate-provider-observation" {
					t.Fatalf("known historical duplicate changed authoritative state: before%+v after%+v", before, got)
				}
				bidAskCorrectionAssertEverySplit(t, r)
				// Changed canonical content conflicts atomically even after the position has
				// closed or become unresolved, at every valid replay checkpoint boundary.
				r.Records[len(r.Records)-1].Quote.Bids[0].Liquidity = "9999"
				for split := 0; split < len(r.Records); split++ {
					prefix := r
					prefix.Records = r.Records[:split]
					prior := bidAskTestRun(t, prefix)
					extended := r
					extended.Checkpoint = prior.Checkpoint
					bidAskExpectError(t, extended, "PAPER_BIDASK_QUOTE_CONFLICT")
				}
			})
		}
	}
}
func TestPaperBidAskUnseenLateObservationRemainsConservative(t *testing.T) {
	for _, short := range []bool{false, true} {
		r := bidAskCorrectionSideRequest(t, short)
		at, _ := bidAskTime(r.Records[5].Quote.ProviderT)
		bid, ask := "101.3", "101.5"
		if short {
			bid, ask = "98.7", "98.9"
		}
		newer := bidAskTestQuote(r, at+1000000000, bid, ask, "")
		bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &newer, CommitT: newer.QualifiedT})
		unseen := bidAskTestQuote(r, at+500000000, bid, ask, "")
		unseen.ReceivedT = bidAskStamp(at + 1501000000)
		unseen.QualifiedT = bidAskStamp(at + 1502000000)
		bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &unseen, CommitT: unseen.QualifiedT})
		got := bidAskTestRun(t, r)
		if got.Coverage != "unresolved-exposure:late-straddle-observation" || got.Position == nil {
			t.Fatal("unseen late observation treated as duplicate")
		}
		bidAskCorrectionAssertEverySplit(t, r)
		// Once admitted as a late observation, its immutable identity is retained too.
		duplicate := unseen
		duplicate.ProviderID = strings.Repeat("d", 64)
		bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &duplicate, CommitT: bidAskStamp(at + 1503000000)})
		again := bidAskTestRun(t, r)
		if again.StateDigest != got.StateDigest || again.Events[len(again.Events)-1].Outcome != "duplicate-provider-observation" {
			t.Fatal("late identity not retained")
		}
	}
}
func TestPaperBidAskResolvedFullQuotesCannotRegressAccountEverySplit(t *testing.T) {
	for _, short := range []bool{false, true} {
		r := bidAskCorrectionSideRequest(t, short)
		start, _ := bidAskTime(r.Records[3].Bar.CloseT)
		for i := int64(0); i < 2; i++ {
			b := bidAskTestBar(start+i*300000000000, 101.3, 101.5, 100.8, 101.4)
			if short {
				b = bidAskTestBar(start+i*300000000000, 98.8, 99.3, 98.5, 98.7)
			}
			bidAskAppend(&r, BidAskRecord{Kind: "bar", Bar: &b, CommitT: b.ReceivedT})
		}
		prior := bidAskTestRun(t, r)
		bid, ask := "101.3", "101.5"
		if short {
			bid, ask = "98.7", "98.9"
		}
		q := bidAskTestQuote(r, start+599000000000, bid, ask, "")
		q.ReceivedT = bidAskStamp(start + 600001000000)
		q.QualifiedT = bidAskStamp(start + 600002000000)
		bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &q, CommitT: q.QualifiedT})
		got := bidAskTestRun(t, r)
		if !reflect.DeepEqual(prior.Account, got.Account) || !reflect.DeepEqual(prior.Position, got.Position) || prior.Coverage != got.Coverage || !reflect.DeepEqual(prior.Events, got.Events) {
			t.Fatalf("resolved interval regressed: %+v -> %+v", prior.Account, got.Account)
		}
		bidAskCorrectionAssertEverySplit(t, r)
		// An observation exactly at the completed close starts the genuinely pending
		// next full interval; it is still never a financial mark or bracket authority.
		pending := bidAskTestQuote(r, start+600000000000, bid, ask, "")
		pending.ReceivedT = bidAskStamp(start + 600003000000)
		pending.QualifiedT = bidAskStamp(start + 600004000000)
		bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &pending, CommitT: pending.QualifiedT})
		next := bidAskTestRun(t, r)
		if next.Account.MarkStatus != "awaiting-completed-interval" || next.Account.MarkClock != got.Account.MarkClock || next.Account.Equity != got.Account.Equity || next.Account.MarkCount != got.Account.MarkCount {
			t.Fatal("genuinely pending interval lost its status")
		}
		bidAskCorrectionAssertEverySplit(t, r)
	}
}
func TestPaperBidAskTimestampGrammarAndBoundaryPrecision(t *testing.T) {
	invalid := []string{"2025-01-01T00:00:00,123456789Z", "2025-01-01T00:00:00.1234567890Z", "2025-01-01T0:00:00.1234567891Z", "2025-1-01T00:00:00Z", "2025-01-1T00:00:00Z", "2025-01-01T00:0:00Z", "2025-01-01T00:00:0Z", "2025-01-01T00:00:00.Z", "2025-01-01T00:00:00+00:00", "2025-01-01T00:00:00z", "2025-01-01t00:00:00Z", "2025-02-30T00:00:00Z"}
	for _, s := range invalid {
		if _, e := bidAskTime(s); e == nil {
			t.Fatalf("accepted inexact timestamp %q", s)
		}
	}
	for _, short := range []bool{false, true} {
		r := bidAskCorrectionSideRequest(t, short)
		close, _ := bidAskTime(r.Records[3].Bar.CloseT)
		r.Records[4].Episode.AdmittedT = bidAskStamp(close + 1)
		r.Records[4].CommitT = bidAskStamp(close + 2)
		r.Records[5].Quote.ProviderT = "2025-01-01T0:20:00.0000000001Z"
		r.Records[5].Quote.RequestStartT = bidAskStamp(close + 2)
		r.Records[5].Quote.ReceivedT = bidAskStamp(close + 1000000)
		r.Records[5].Quote.QualifiedT = bidAskStamp(close + 2000000)
		r.Records[5].CommitT = bidAskStamp(close + 3000000)
		raw, _ := json.Marshal(r)
		decoded, e := DecodePaperBidAskRequest(raw)
		if e != nil {
			t.Fatal(e)
		}
		bidAskExpectError(t, decoded, "PAPER_BIDASK_TIMESTAMP")
		// Valid exact nanosecond instants retain their value and phase; trailing zeros
		// in a supported RFC3339 string do not silently create different instants.
		for _, ns := range []int64{0, 1, 999999999} {
			r.Records[5].Quote.ProviderT = bidAskStamp(close + ns)
			r.Records[5].Quote.ReceivedT = bidAskStamp(close + ns + 1000000)
			r.Records[5].Quote.QualifiedT = bidAskStamp(close + ns + 2000000)
			r.Records[5].CommitT = bidAskStamp(close + ns + 3000000)
			v := bidAskTestRun(t, r)
			if v.Position == nil || v.Position.EntryProviderT != r.Records[5].Quote.ProviderT {
				t.Fatal("exact provider time not retained")
			}
			want := "sampled-straddle"
			if ns == 0 {
				want = "full-bar"
			}
			if v.Position.Phase != want {
				t.Fatalf("nano%d phase %s", ns, v.Position.Phase)
			}
			bidAskCorrectionAssertEverySplit(t, r)
		}
	}
}
func TestPaperBidAskQuoteIdentityIndexBound(t *testing.T) {
	r := bidAskTestRequest(t)
	p, b, e := prepareBidAsk(r, true)
	if e != nil {
		t.Fatal(e)
	}
	x := newBidAskRunner(r, p, b)
	start, _ := bidAskTime(r.Records[0].Bar.OpenT)
	for i := 0; i < PaperBidAskMaxQuoteIdentities; i++ {
		q := bidAskTestQuote(r, start+int64(i), "100", "100.2", "")
		x.onQuote(BidAskRecord{Sequence: strconv.Itoa(i + 1), Kind: "quote", Epoch: r.Identity.Epoch, Quote: &q, CommitT: q.QualifiedT})
		if x.err != nil {
			t.Fatal(x.err)
		}
	}
	if len(x.quoteIdentities) != PaperBidAskMaxQuoteIdentities {
		t.Fatal("identity history truncated")
	}
	duplicate := bidAskTestQuote(r, start, "100.0", "100.20", "")
	x.onQuote(BidAskRecord{Sequence: "1", Kind: "quote", Quote: &duplicate, CommitT: duplicate.QualifiedT})
	if x.err != nil || len(x.quoteIdentities) != PaperBidAskMaxQuoteIdentities || x.events[len(x.events)-1].Outcome != "duplicate-provider-observation" {
		t.Fatal("full index lost known identity")
	}
	q := bidAskTestQuote(r, start+int64(PaperBidAskMaxQuoteIdentities), "100", "100.2", "")
	x.onQuote(BidAskRecord{Sequence: "1", Kind: "quote", Quote: &q, CommitT: q.QualifiedT})
	if x.err == nil || x.err.Error() != "PAPER_BIDASK_QUOTE_IDENTITY_LIMIT" || len(x.quoteIdentities) != PaperBidAskMaxQuoteIdentities {
		t.Fatal("unbounded identity index")
	}
}
func TestPaperBidAskFullPrefixQuoteIdentityCapacity(t *testing.T) {
	r := bidAskTestRequest(t)
	start, _ := bidAskTime(r.Records[0].Bar.OpenT)
	r.Records = nil
	for i := 0; i < PaperBidAskMaxRecords; i++ {
		q := bidAskTestQuote(r, start+int64(i), "100", "100.2", "")
		bidAskAppend(&r, BidAskRecord{Kind: "quote", Quote: &q, CommitT: q.QualifiedT})
	}
	before := time.Now()
	v := bidAskTestRun(t, r)
	t.Logf("full-prefix %d distinct quote identities, elapsed %s", len(r.Records), time.Since(before))
	if v.ProcessedRecords != PaperBidAskMaxRecords || v.ProcessedBars != 0 || len(v.Events) != 0 || v.Position != nil || v.Account.MarkCount != 0 {
		t.Fatal("quote-only identity prefix was truncated or traded")
	}
	r.Checkpoint = v.Checkpoint
	again := bidAskTestRun(t, r)
	if len(again.Events) != 0 || again.StateDigest != v.StateDigest || string(again.Checkpoint) != string(v.Checkpoint) {
		t.Fatal("maximum quote identity receipt drift")
	}
	// Mutation of the earliest identity remains detectable at the maximum prefix,
	// even though thousands of newer timestamps intervene before the retry.
	old := *r.Records[0].Quote
	old.Bids = append([]BidAskTier{}, old.Bids...)
	old.Bids[0].Liquidity = "9999"
	last := len(r.Records) - 1
	old.ReceivedT = r.Records[last].Quote.ReceivedT
	old.QualifiedT = r.Records[last].Quote.QualifiedT
	r.Records[last].Quote = &old
	r.Checkpoint = nil
	bidAskExpectError(t, r, "PAPER_BIDASK_QUOTE_CONFLICT")
}
