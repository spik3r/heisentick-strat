package engine

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func dualBoundExperiment(r *PaperBidAskRequest) {
	activation, _ := bidAskTime(r.ActivationFromT)
	day := int64(86400000000000)
	r.Experiment = &BidAskExperiment{Profile: "oanda-au-v20-gold-dated-v1", AttestationDigest: strings.Repeat("c", 64),
		EntryUntilT: bidAskStamp(activation + 32*day), ObservationUntilT: bidAskStamp(activation + 46*day), CalendarThroughT: bidAskStamp(activation + 46*day), SampleCadenceMS: "300000", Closures: []BidAskClosureWindow{}}
}
func TestPaperBidAskDualEMADatedClosurePreservesPositionAndFamily(t *testing.T) {
	r := dualPaperRequest(t)
	dualBoundExperiment(&r)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+d, 119)
	bidAskAddEntry(&r, "118.9", "119.1")
	dualAppendBar(&r, at+2*d, 123)
	from, through := at+3*d, at+12*d
	r.Experiment.Closures = []BidAskClosureWindow{{FromT: bidAskStamp(from - 1), ThroughT: bidAskStamp(through + 1)}}
	before := bidAskTestRun(t, r)
	bidAskAppend(&r, BidAskRecord{Kind: "market-closure", CommitT: bidAskStamp(through), Closure: &BidAskClosure{
		WindowIndex: "1", FromT: bidAskStamp(from), ThroughT: bidAskStamp(through)}})
	closure := bidAskTestRun(t, r)
	if closure.Position == nil || closure.Position.ID != before.Position.ID || closure.Position.Stop != before.Position.Stop || closure.ProcessedBars != before.ProcessedBars {
		t.Fatalf("closure mutated financial/family state: %+v", closure)
	}
	if closure.Coverage != "sampled-observations-only" || closure.Account.MarkStatus != "scheduled-market-closure" {
		t.Fatal(closure)
	}
	dualAppendBar(&r, through, 124)
	full := bidAskTestRun(t, r)
	if full.Position == nil || full.Coverage != "sampled-observations-only" {
		t.Fatal(full)
	}
	// The same real bars without closed-time intervals have the same indicator
	// and stop progression; the calendar only advances expected wall-clock gaps.
	contiguous := r
	contiguous.Experiment = nil
	contiguous.Records = append([]BidAskRecord{}, r.Records[:len(r.Records)-2]...)
	dualAppendBar(&contiguous, from, 124)
	control := bidAskTestRun(t, contiguous)
	if control.Position.Stop != full.Position.Stop {
		t.Fatalf("indicator advanced through closure: %s %s", control.Position.Stop, full.Position.Stop)
	}
	for split := 0; split < len(r.Records); split++ {
		prefix := r
		prefix.Records = r.Records[:split]
		old := bidAskTestRun(t, prefix)
		resume := r
		resume.Checkpoint = old.Checkpoint
		got := bidAskTestRun(t, resume)
		if got.StateDigest != full.StateDigest || !reflect.DeepEqual(got.Position, full.Position) {
			t.Fatalf("closure replay split%d", split)
		}
	}
	bad := r
	bad.Experiment = new(BidAskExperiment)
	*bad.Experiment = *r.Experiment
	bad.Experiment.AttestationDigest = strings.Repeat("d", 64)
	bad.Checkpoint = closure.Checkpoint
	if _, err := RunPaperBidAsk(bad); err == nil {
		t.Fatal("calendar binding drift accepted")
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePaperBidAskRequest(raw); err != nil {
		t.Fatal(err)
	}
}
func TestPaperBidAskDualEMAUnprovedClosureRefused(t *testing.T) {
	for _, name := range []string{"no-profile", "wrong-window", "not-grid", "past-commit", "overlap", "wrong-state"} {
		t.Run(name, func(t *testing.T) {
			r := dualPaperRequest(t)
			dualBoundExperiment(&r)
			at, _ := bidAskTime(r.ActivationFromT)
			d := int64(14400000000000)
			dualAppendBar(&r, at, 114)
			from, through := at+d, at+3*d
			r.Experiment.Closures = []BidAskClosureWindow{{FromT: bidAskStamp(from), ThroughT: bidAskStamp(through)}}
			c := &BidAskClosure{WindowIndex: "1", FromT: bidAskStamp(from), ThroughT: bidAskStamp(through)}
			rec := BidAskRecord{Kind: "market-closure", CommitT: bidAskStamp(through), Closure: c}
			switch name {
			case "no-profile":
				r.Experiment = nil
			case "wrong-window":
				c.WindowIndex = "2"
			case "not-grid":
				c.FromT = bidAskStamp(from + 1)
			case "past-commit":
				rec.CommitT = bidAskStamp(through - 1)
			case "overlap":
				r.Experiment.Closures = append(r.Experiment.Closures, r.Experiment.Closures[0])
			case "wrong-state":
				c.FromT = bidAskStamp(from + d)
			}
			bidAskAppend(&r, rec)
			if _, err := RunPaperBidAsk(r); err == nil {
				t.Fatal("invalid closure accepted")
			}
		})
	}
}
func TestPaperBidAskDualEMAEntryHorizonDoesNotLiquidateHeldPosition(t *testing.T) {
	r := dualPaperRequest(t)
	dualBoundExperiment(&r)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	// Place the entry horizon immediately after the synthetic decision by moving
	// activation back exactly32days while retaining the real warmup bars.
	r.ActivationFromT = bidAskStamp(at - 32*86400000000000 + 2*d)
	dualBoundExperiment(&r)
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+d, 119)
	got := bidAskTestRun(t, r)
	if got.Pending != nil {
		t.Fatal("decision created at entry horizon")
	}
	// Unbounded control still captures that exact signal.
	r.Experiment = nil
	if bidAskTestRun(t, r).Pending == nil {
		t.Fatal("control signal absent")
	}
}

func TestPaperBidAskDualEMACalendarCannotEraseObservedTrading(t *testing.T) {
	for _, kind := range []string{"bar", "quote"} {
		t.Run(kind, func(t *testing.T) {
			r := dualPaperRequest(t)
			dualBoundExperiment(&r)
			at, _ := bidAskTime(r.ActivationFromT)
			d := int64(14400000000000)
			r.Experiment.Closures = []BidAskClosureWindow{{FromT: bidAskStamp(at), ThroughT: bidAskStamp(at + 2*d)}}
			if kind == "bar" {
				dualAppendBar(&r, at, 114)
			} else {
				q := bidAskTestQuote(r, at+1000000000, "119", "119.2", "")
				bidAskAppend(&r, BidAskRecord{Kind: "quote", CommitT: q.QualifiedT, Quote: &q})
			}
			bidAskExpectError(t, r, "PAPER_BIDASK_CALENDAR_CONTRADICTION")
		})
	}
}
func TestPaperBidAskDualEMAExitOnlyReserveKeepsManagement(t *testing.T) {
	r := dualPaperRequest(t)
	dualBoundExperiment(&r)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+d, 119)
	bidAskAddEntry(&r, "118.9", "119.1")
	// Move the declared start32days before the next realbarclose. No market
	// history is fabricated; only this synthetic experiment's fixed horizon moves.
	r.ActivationFromT = bidAskStamp(at + 3*d - 32*86400000000000)
	dualBoundExperiment(&r)
	before := bidAskTestRun(t, r)
	dualAppendBar(&r, at+2*d, 123)
	managed := bidAskTestRun(t, r)
	if managed.Position == nil || managed.Position.Stop == before.Position.Stop {
		t.Fatal("horizon stopped position management")
	}
	dualAppendBar(&r, at+3*d, 100)
	closed := bidAskTestRun(t, r)
	if closed.Position != nil || closed.CompletedTrades.Completed != 1 {
		t.Fatal("exit-only reserve failed to close")
	}
	for _, event := range closed.Events {
		if event.Kind == "decision" {
			close, _ := bidAskTime(event.Clock.SignalCloseT)
			until, _ := bidAskTime(r.Experiment.EntryUntilT)
			if close >= until {
				t.Fatal("new entry in exit-only reserve")
			}
		}
	}
}

func TestPaperBidAskDualEMAPartialReopenUsesAssumedTradableClock(t *testing.T) {
	for _, row := range []struct {
		name       string
		offsetDays int64
		reopenHour int64
	}{{"winter", 0, 23}, {"summer", 182, 22}} {
		t.Run(row.name, func(t *testing.T) {
			r := dualPaperRequest(t)
			if row.offsetDays != 0 {
				raw, _ := json.Marshal(r)
				var value any
				_ = json.Unmarshal(raw, &value)
				var shift func(any) any
				shift = func(value any) any {
					switch v := value.(type) {
					case string:
						if at, err := bidAskTime(v); err == nil {
							return bidAskStamp(at + row.offsetDays*86400000000000)
						}
					case []any:
						for i, item := range v {
							v[i] = shift(item)
						}
					case map[string]any:
						for k, item := range v {
							v[k] = shift(item)
						}
					}
					return value
				}
				raw, _ = json.Marshal(shift(value))
				_ = json.Unmarshal(raw, &r)
			}
			dualBoundExperiment(&r)
			at, _ := bidAskTime(r.ActivationFromT)
			d := int64(14400000000000)
			dualAppendBar(&r, at, 114)
			dualAppendBar(&r, at+d, 119)
			bidAskAddEntry(&r, "118.9", "119.1")
			// Actual partial Sunday20:00–24:00 bar begins trading at the reviewed
			// NewYork18:05 reopen (23:05winter/22:05summer). No first-tick claim.
			sunday := at + 3*86400000000000 + 20*3600000000000
			fridayClosed := at + 86400000000000 + (row.reopenHour-2)*3600000000000 + 59*60000000000
			reopen := at + 3*86400000000000 + row.reopenHour*3600000000000 + 5*60000000000
			from := at + 2*86400000000000
			r.Experiment.Closures = []BidAskClosureWindow{{FromT: bidAskStamp(fridayClosed), ThroughT: bidAskStamp(reopen)}}
			for next := at + 2*d; next < from; next += d {
				dualAppendBar(&r, next, 124)
			}
			bidAskAppend(&r, BidAskRecord{Kind: "market-closure", CommitT: bidAskStamp(sunday), Closure: &BidAskClosure{WindowIndex: "1", FromT: bidAskStamp(from), ThroughT: bidAskStamp(sunday)}})
			dualAppendBar(&r, sunday, 100)
			result := bidAskTestRun(t, r)
			var exit *BidAskEvent
			for i := range result.Events {
				if result.Events[i].Kind == "exit" {
					exit = &result.Events[i]
				}
			}
			if exit == nil || exit.Basis != "modeled-side-open-after-scheduled-reopen" || exit.Clock.ModelEffectiveT != bidAskStamp(reopen) || exit.Clock.IntervalOpenT != bidAskStamp(sunday) {
				t.Fatal("partial reopen clock incorrect", exit)
			}
		})
	}
}

func TestPaperBidAskDualEMAFlatLiveGapRefusesSignals(t *testing.T) {
	r := dualPaperRequest(t)
	dualBoundExperiment(&r)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+2*d, 119)
	result := bidAskTestRun(t, r)
	if result.Coverage != "unhealthy-unaccounted-interval" || result.Pending != nil || result.Position != nil {
		t.Fatal("unaccounted flat gap admitted", result.Coverage)
	}
}
func TestPaperBidAskDualEMAEntryCommitAtHorizonCannotOpen(t *testing.T) {
	r := dualPaperRequest(t)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	r.ActivationFromT = bidAskStamp(at + 3*d - 32*86400000000000)
	dualBoundExperiment(&r)
	r.Policy.MaxSignalAgeMS = "14400000"
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+d, 119)
	signal := r.Records[len(r.Records)-1].Sequence
	horizon, _ := bidAskTime(r.Experiment.EntryUntilT)
	admit := at + 2*d + 100000000
	bidAskAppend(&r, BidAskRecord{Kind: "episode", CommitT: bidAskStamp(admit), Episode: &BidAskEpisode{SignalSequence: signal, AdmittedT: bidAskStamp(admit), DeadlineT: bidAskStamp(horizon)}})
	q := bidAskTestQuote(r, horizon-1, "118.9", "119.1", signal)
	q.RequestStartT = q.ProviderT
	q.ReceivedT = q.ProviderT
	q.QualifiedT = q.ProviderT
	bidAskAppend(&r, BidAskRecord{Kind: "quote", CommitT: bidAskStamp(horizon), Quote: &q})
	result := bidAskTestRun(t, r)
	if result.Position != nil || result.Pending != nil || result.Events[len(result.Events)-1].Outcome != "experiment-entry-horizon" {
		t.Fatal("late opening accepted")
	}
}
func TestPaperBidAskDualEMATerminalClockRetainsUnresolvedExposure(t *testing.T) {
	r := dualPaperRequest(t)
	dualBoundExperiment(&r)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+d, 119)
	bidAskAddEntry(&r, "118.9", "119.1")
	until, _ := bidAskTime(r.Experiment.ObservationUntilT)
	bidAskAppend(&r, BidAskRecord{Kind: "experiment-clock", CommitT: bidAskStamp(until + 1), ExperimentClock: &BidAskExperimentClock{EffectiveT: bidAskStamp(until), Reason: "experiment-observation-horizon"}})
	result := bidAskTestRun(t, r)
	if result.Position == nil || result.Coverage != "unresolved-exposure:experiment-observation-horizon" || result.CompletedTrades.Completed != 0 {
		t.Fatal("reserve expiry fabricated disposition")
	}
	event := result.Events[len(result.Events)-1]
	if event.Clock.ModelEffectiveT != bidAskStamp(until) || event.Clock.CommitT != bidAskStamp(until+1) {
		t.Fatal("terminal clock lost authority")
	}
	before := r
	r.Records = append([]BidAskRecord{}, r.Records...)
	q := bidAskTestQuote(r, until+2, "119", "119.2", "")
	bidAskAppend(&r, BidAskRecord{Kind: "quote", CommitT: q.QualifiedT, Quote: &q})
	if _, err := RunPaperBidAsk(r); err == nil {
		t.Fatal("observations appended after terminal clock")
	}
	before.Checkpoint = result.Checkpoint
	replay := bidAskTestRun(t, before)
	if len(replay.Events) != 0 || replay.StateDigest != result.StateDigest {
		t.Fatal("terminal replay duplicated effects")
	}
}

func TestPaperBidAskDualEMADelayedSignalCanBeRejectedWithoutBackdatingEpisode(t *testing.T) {
	r := dualPaperRequest(t)
	dualBoundExperiment(&r)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	dualAppendBar(&r, at, 114)
	dualAppendBar(&r, at+d, 119)
	signal := r.Records[len(r.Records)-1].Sequence
	after := at + 2*d + 290000000000
	bidAskAppend(&r, BidAskRecord{Kind: "seal", CommitT: bidAskStamp(after), Seal: &BidAskSeal{SignalSequence: signal, Reason: "signal-expired", EffectiveT: bidAskStamp(after)}})
	result := bidAskTestRun(t, r)
	if result.Pending != nil || result.Position != nil || result.Events[len(result.Events)-1].Outcome != "expired-signal" {
		t.Fatal("delayed signal not rejected")
	}
	early := r
	early.Records = append([]BidAskRecord{}, r.Records...)
	c := *early.Records[len(early.Records)-1].Seal
	c.EffectiveT = bidAskStamp(after - 1)
	early.Records[len(early.Records)-1].Seal = &c
	if _, err := RunPaperBidAsk(early); err == nil {
		t.Fatal("unexpired signal suppressed")
	}
	dualAppendBar(&r, at+2*d, 123)
	if bidAskTestRun(t, r).Coverage != "sampled-observations-only" {
		t.Fatal("rejected signal poisoned next bar")
	}
}

func TestPaperBidAskDualEMAWeekendDecisionWaitsForDeclaredExecutableOpen(t *testing.T) {
	r := dualPaperRequest(t)
	dualBoundExperiment(&r)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	from := at + 2*86400000000000
	through := at + 3*86400000000000 + 23*3600000000000 + 5*60000000000
	r.Experiment.Closures = []BidAskClosureWindow{{FromT: bidAskStamp(from - 2*3600000000000 - 60000000000), ThroughT: bidAskStamp(through)}}
	for open := at; open < from-d; open += d {
		price := 118.0
		if open == from-2*d {
			price = 114
		}
		dualAppendBar(&r, open, price)
	}
	dualAppendBar(&r, from-d, 119)
	before := bidAskTestRun(t, r)
	if before.Pending == nil || before.Pending.ExecutableFromT != bidAskStamp(through) {
		t.Fatal("weekend execution clock absent")
	}
	bidAskAppend(&r, BidAskRecord{Kind: "market-closure", CommitT: bidAskStamp(through - 3*3600000000000 - 5*60000000000), Closure: &BidAskClosure{WindowIndex: "1", FromT: bidAskStamp(from), ThroughT: bidAskStamp(through - 3*3600000000000 - 5*60000000000)}})
	signal := before.Pending.SignalSequence
	admit := through + 100000000
	bidAskAppend(&r, BidAskRecord{Kind: "episode", CommitT: bidAskStamp(admit), Episode: &BidAskEpisode{SignalSequence: signal, AdmittedT: bidAskStamp(admit), DeadlineT: bidAskStamp(through + 290000000000)}})
	q := bidAskTestQuote(r, through+1000000000, "118.9", "119.1", signal)
	bidAskAppend(&r, BidAskRecord{Kind: "quote", CommitT: q.QualifiedT, Quote: &q})
	result := bidAskTestRun(t, r)
	if result.Position == nil || result.Position.EntryProviderT != q.ProviderT {
		t.Fatal("first causal reopening quote did not fill")
	}
	late := r
	late.Records = append([]BidAskRecord{}, r.Records[:len(r.Records)-1]...)
	q = bidAskTestQuote(late, through+290000000001, "118.9", "119.1", signal)
	bidAskAppend(&late, BidAskRecord{Kind: "quote", CommitT: q.QualifiedT, Quote: &q})
	rejected := bidAskTestRun(t, late)
	if rejected.Position != nil || rejected.Events[len(rejected.Events)-1].Outcome != "original-quote-expired" {
		t.Fatal("reopening deadline refreshed")
	}
}
func TestPaperBidAskDualEMADailyBreakDoesNotRefreshEarlierSignal(t *testing.T) {
	r := dualPaperRequest(t)
	dualBoundExperiment(&r)
	at, _ := bidAskTime(r.ActivationFromT)
	d := int64(14400000000000)
	// Signal20:00 is before the21:59–23:05 NYwinter dailybreak. Its original
	// deadline must not move merely because a later closure exists.
	for open := at; open < at+3*d; open += d {
		dualAppendBar(&r, open, 118)
	}
	dualAppendBar(&r, at+3*d, 114)
	dualAppendBar(&r, at+4*d, 119)
	close := at + 5*d
	r.Experiment.Closures = []BidAskClosureWindow{{FromT: bidAskStamp(at + 21*3600000000000 + 59*60000000000), ThroughT: bidAskStamp(at + 23*3600000000000 + 5*60000000000)}}
	signal := bidAskTestRun(t, r).Pending
	if signal == nil || signal.ExecutableFromT != bidAskStamp(close) {
		t.Fatal("dailybreak changed an earlier signal")
	}
	admit := close + 100000000
	bidAskAppend(&r, BidAskRecord{Kind: "episode", CommitT: bidAskStamp(admit), Episode: &BidAskEpisode{SignalSequence: signal.SignalSequence, AdmittedT: bidAskStamp(admit), DeadlineT: bidAskStamp(close + 290000000000)}})
	q := bidAskTestQuote(r, at+23*3600000000000+5*60000000000+1000000000, "118.9", "119.1", signal.SignalSequence)
	bidAskAppend(&r, BidAskRecord{Kind: "quote", CommitT: q.QualifiedT, Quote: &q})
	result := bidAskTestRun(t, r)
	if result.Position != nil || result.Events[len(result.Events)-1].Outcome != "original-quote-expired" {
		t.Fatal("daily reopen refreshed expired original")
	}
}
