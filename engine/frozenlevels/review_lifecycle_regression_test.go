package frozenlevels_test

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	fl "github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

const reviewMaxTime int64 = 9007199254740991

func reviewSource() fl.Source {
	return fl.Source{Provider: "independent-review", Dataset: "invented-ordered-quotes", SHA256: strings.Repeat("a", 64), Side: fl.Bid}
}
func reviewSpec(side fl.Direction) fl.LevelSpec {
	return fl.LevelSpec{Version: fl.LifecycleVersion, ProviderID: "hand-derived", SourceWindowID: "window-1", ContextSource: reviewSource(), QuoteSource: reviewSource(), OrderingProof: "invented-global-order-1", KnownAt: fl.EventKey{AtMillis: 100, Sequence: 1}, CreatedAt: fl.EventKey{AtMillis: 100, Sequence: 2}, ExpiresAtMillis: 1000, Side: side, Price: 100.003, Policy: fl.LevelPolicy{AssumptionID: "review-v1", MaxPredecessorAgeMillis: 10, MaxObservationAgeMillis: 20, SubmissionLatencyMillis: 0, UsePolicy: "one-success-no-retry"}}
}
func reviewQuote(at, seq, available int64, bid float64) fl.LevelQuote {
	return fl.LevelQuote{Source: reviewSource(), Quote: fl.Quote{Event: fl.EventKey{AtMillis: at, Sequence: seq}, AvailableAtMillis: available, Bid: bid, Ask: bid}}
}
func reviewCreate(t *testing.T, s fl.LevelSpec, prev *fl.LevelQuote) *fl.LevelLifecycle {
	t.Helper()
	m, e, err := fl.NewLevelLifecycle(s, prev)
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != "created" || e.At != s.CreatedAt || e.AvailableAtMillis != s.CreatedAt.AtMillis || e.ExpiresAtMillis != s.ExpiresAtMillis || e.Version != s.Version || e.LevelID != m.ID() || e.SignalID != "" {
		t.Fatalf("bad created: %+v", e)
	}
	return m
}
func reviewStep(t *testing.T, m *fl.LevelLifecycle, q fl.LevelQuote, busy bool, want ...string) []fl.LevelEvent {
	t.Helper()
	es, err := m.Observe(q, true, busy)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range es {
		got = append(got, e.Kind+"/"+e.Reason)
		if e.At != q.Quote.Event || e.AvailableAtMillis != q.Quote.AvailableAtMillis || e.LevelID != m.ID() || e.Version != fl.LifecycleVersion {
			t.Fatalf("event attribution: %+v", e)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	return es
}

// Independent oracle: a completed bid crossing is determined only by strict
// predecessor/current inequalities, never by an inferred path or ask price.
func TestIndependentLifecycleCrossingOracle(t *testing.T) {
	count := 0
	for _, side := range []fl.Direction{fl.Long, fl.Short} {
		for _, threshold := range []float64{math.SmallestNonzeroFloat64, 0.0003, 0.007, 0.125, 100.003, math.MaxFloat64} {
			candidates := []float64{math.Nextafter(threshold, 0), threshold, math.Nextafter(threshold, math.Inf(1))}
			for _, prior := range candidates {
				for _, next := range candidates {
					if prior <= 0 || next <= 0 || math.IsInf(prior, 0) || math.IsInf(next, 0) {
						continue
					}
					for _, delta := range []int64{0, 10, 11} {
						for _, busy := range []bool{false, true} {
							s := reviewSpec(side)
							s.Price = threshold
							p := reviewQuote(100, 1, 100, prior)
							m := reviewCreate(t, s, &p)
							q := reviewQuote(100+delta, 3, 100+delta, next)
							crossing := (side == fl.Long && prior < threshold && next >= threshold) || (side == fl.Short && prior > threshold && next <= threshold)
							inside := (side == fl.Long && next < threshold) || (side == fl.Short && next > threshold)
							priorInside := (side == fl.Long && prior < threshold) || (side == fl.Short && prior > threshold)
							switch {
							case delta > 10:
								reviewStep(t, m, q, busy, "reset/stale-predecessor")
							case crossing && busy:
								reviewStep(t, m, q, busy, "crossed/bid-crossing", "skipped/position-busy")
							case crossing:
								reviewStep(t, m, q, busy, "crossed/bid-crossing", "submitted/admission-pending")
							case inside && !priorInside:
								reviewStep(t, m, q, busy, "reset/inside-observation")
							default:
								reviewStep(t, m, q, busy)
							}
							count++
						}
					}
				}
			}
		}
	}
	t.Logf("%d independent crossing scenarios", count)
}

func TestIndependentLifecycleDelayedAdmissionBusyAndOneUse(t *testing.T) {
	for _, side := range []fl.Direction{fl.Long, fl.Short} {
		for _, resolution := range []string{"consumed", "rejected", "busy"} {
			s := reviewSpec(side)
			s.Policy.SubmissionLatencyMillis = 3
			inside, outside := 100.0, 101.0
			if side == fl.Short {
				inside, outside = 101, 100
			}
			m := reviewCreate(t, s, nil)
			reviewStep(t, m, reviewQuote(101, 1, 102, inside), false, "reset/inside-observation")
			signal := reviewStep(t, m, reviewQuote(102, 1, 110, outside), false, "crossed/bid-crossing", "submitted/admission-pending")
			// Receipt time is past 113, but the source time is not. No retrofill.
			reviewStep(t, m, reviewQuote(112, 1, 115, inside), false)
			readyQ := reviewQuote(113, 1, 115, inside)
			ready := reviewStep(t, m, readyQ, false, "admission-ready/strictly-subsequent-source-observation")
			if signal[0].SignalID == "" || signal[0].SignalID != ready[0].SignalID {
				t.Fatal("signal changed")
			}
			q := readyQ
			q.Quote.Event.Sequence++
			if _, err := m.ResolveAdmission(q.Quote.Event, "consumed"); err == nil || m.State() != "ready" {
				t.Fatal("wrong event resolved")
			}
			if _, err := m.ResolveAdmission(readyQ.Quote.Event, "later"); err == nil || m.State() != "ready" {
				t.Fatal("invalid outcome resolved")
			}
			e, err := m.ResolveAdmission(readyQ.Quote.Event, resolution)
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string]string{"consumed": "consumed/external-successful-execution", "rejected": "invalidated/external-execution-rejected-no-retry", "busy": "skipped/position-busy-at-resolution"}[resolution]
			if e.Kind+"/"+e.Reason != expected || e.SignalID != ready[0].SignalID || e.At != readyQ.Quote.Event {
				t.Fatalf("wrong resolution %+v", e)
			}
			if resolution == "busy" {
				again := reviewStep(t, m, reviewQuote(114, 1, 115, outside), false, "crossed/bid-crossing", "submitted/admission-pending")
				if again[0].SignalID == e.SignalID {
					t.Fatal("new crossing reused signal ID")
				}
			} else {
				if !m.Terminal() {
					t.Fatal("not terminal")
				}
				if _, err = m.Observe(reviewQuote(114, 1, 115, inside), true, false); err == nil {
					t.Fatal("terminal reused")
				}
				if _, err = m.ResolveAdmission(readyQ.Quote.Event, resolution); err == nil {
					t.Fatal("resolution reused")
				}
			}
		}
	}
}

func TestIndependentLifecycleBusyAdmissionNeedsRecross(t *testing.T) {
	for _, side := range []fl.Direction{fl.Long, fl.Short} {
		inside, outside := 100.0, 101.0
		if side == fl.Short {
			inside, outside = 101, 100
		}
		m := reviewCreate(t, reviewSpec(side), nil)
		reviewStep(t, m, reviewQuote(101, 1, 101, inside), false, "reset/inside-observation")
		reviewStep(t, m, reviewQuote(102, 1, 102, outside), false, "crossed/bid-crossing", "submitted/admission-pending")
		reviewStep(t, m, reviewQuote(103, 1, 103, outside), true, "skipped/position-busy-at-admission")
		reviewStep(t, m, reviewQuote(104, 1, 104, outside), false)
		reviewStep(t, m, reviewQuote(105, 1, 105, inside), false, "reset/inside-observation")
		reviewStep(t, m, reviewQuote(106, 1, 106, outside), false, "crossed/bid-crossing", "submitted/admission-pending")
	}
}

func TestIndependentLifecycleFailClosedStatesAndReasons(t *testing.T) {
	mutations := map[string]struct {
		mutate func(*fl.LevelQuote)
		reason string
	}{
		"duplicate":             {func(q *fl.LevelQuote) { q.Quote.Event = fl.EventKey{AtMillis: 101, Sequence: 1} }, "non-subsequent-level-observation"},
		"timestamp-order":       {func(q *fl.LevelQuote) { q.Quote.Event = fl.EventKey{AtMillis: 100, Sequence: 99} }, "non-subsequent-level-observation"},
		"sequence-order":        {func(q *fl.LevelQuote) { q.Quote.Event = fl.EventKey{AtMillis: 101, Sequence: 0} }, "non-subsequent-level-observation"},
		"before-creation":       {func(q *fl.LevelQuote) { q.Quote.Event = fl.EventKey{AtMillis: 100, Sequence: 2} }, "observation-before-level-creation"},
		"receipt-order":         {func(q *fl.LevelQuote) { q.Quote.AvailableAtMillis = 102 }, "non-subsequent-level-observation"},
		"unavailable":           {func(q *fl.LevelQuote) { q.Quote.AvailableAtMillis = 101 }, "invalid-quote"},
		"stale":                 {func(q *fl.LevelQuote) { q.Quote.AvailableAtMillis = 123 }, "stale-level-observation"},
		"nan-bid":               {func(q *fl.LevelQuote) { q.Quote.Bid = math.NaN() }, "invalid-quote"},
		"nan-ask":               {func(q *fl.LevelQuote) { q.Quote.Ask = math.NaN() }, "invalid-quote"},
		"negative":              {func(q *fl.LevelQuote) { q.Quote.Bid = -1 }, "invalid-quote"},
		"zero":                  {func(q *fl.LevelQuote) { q.Quote.Bid = math.Copysign(0, -1) }, "invalid-quote"},
		"infinity":              {func(q *fl.LevelQuote) { q.Quote.Ask = math.Inf(1) }, "invalid-quote"},
		"crossed":               {func(q *fl.LevelQuote) { q.Quote.Bid = 102 }, "invalid-quote"},
		"time-overflow":         {func(q *fl.LevelQuote) { q.Quote.Event.AtMillis = math.MaxInt64 }, "invalid-quote"},
		"sequence-overflow":     {func(q *fl.LevelQuote) { q.Quote.Event.Sequence = reviewMaxTime + 1 }, "invalid-quote"},
		"negative-sequence":     {func(q *fl.LevelQuote) { q.Quote.Event.Sequence = -1 }, "invalid-quote"},
		"availability-overflow": {func(q *fl.LevelQuote) { q.Quote.AvailableAtMillis = math.MaxInt64 }, "invalid-quote"},
		"digest":                {func(q *fl.LevelQuote) { q.Source.SHA256 = strings.Repeat("b", 64) }, "level-quote-source-mismatch"},
		"side":                  {func(q *fl.LevelQuote) { q.Source.Side = fl.Ask }, "level-quote-source-mismatch"},
	}
	for name, tc := range mutations {
		for _, state := range []string{"active", "pending", "ready"} {
			t.Run(name+"/"+state, func(t *testing.T) {
				m := reviewCreate(t, reviewSpec(fl.Long), nil)
				reviewStep(t, m, reviewQuote(101, 1, 103, 100), false, "reset/inside-observation")
				if state != "active" {
					reviewStep(t, m, reviewQuote(101, 2, 103, 101), false, "crossed/bid-crossing", "submitted/admission-pending")
				}
				if state == "ready" {
					reviewStep(t, m, reviewQuote(103, 1, 103, 101), false, "admission-ready/strictly-subsequent-source-observation")
				}
				q := reviewQuote(102, 1, 104, 101)
				tc.mutate(&q)
				es, err := m.Observe(q, true, false)
				if err == nil || len(es) != 1 || es[0].Kind != "invalidated" || !m.Terminal() {
					t.Fatalf("did not invalidate: %+v %v", es, err)
				}
				// Ready setup's later source time may make ordering take precedence.
				if state != "ready" && es[0].Reason != tc.reason {
					t.Fatalf("reason %s want %s", es[0].Reason, tc.reason)
				}
				if _, err = m.Observe(reviewQuote(105, 1, 105, 100), true, false); err == nil {
					t.Fatal("repair silently resumed")
				}
			})
		}
	}
	for _, state := range []string{"active", "pending", "ready"} {
		t.Run("gap/"+state, func(t *testing.T) {
			m := reviewCreate(t, reviewSpec(fl.Long), nil)
			reviewStep(t, m, reviewQuote(101, 1, 101, 100), false, "reset/inside-observation")
			if state != "active" {
				reviewStep(t, m, reviewQuote(102, 1, 102, 101), false, "crossed/bid-crossing", "submitted/admission-pending")
			}
			if state == "ready" {
				reviewStep(t, m, reviewQuote(103, 1, 103, 101), false, "admission-ready/strictly-subsequent-source-observation")
			}
			es, err := m.Observe(reviewQuote(104, 1, 104, 101), false, false)
			if err == nil || es[0].Reason != "unverified-level-quote-gap" || !m.Terminal() {
				t.Fatal(es, err)
			}
		})
	}
}

func TestIndependentLifecycleExpiryBoundary(t *testing.T) {
	for _, side := range []fl.Direction{fl.Long, fl.Short} {
		for _, mode := range []string{"source", "availability", "submission"} {
			for _, offset := range []int64{-1, 0, 1} {
				s := reviewSpec(side)
				s.ExpiresAtMillis = 120
				inside, outside := 100.0, 101.0
				if side == fl.Short {
					inside, outside = 101, 100
				}
				at, available := int64(119), int64(119)
				if mode == "source" {
					at = 120 + offset
					available = at
				}
				if mode == "availability" {
					available = 120 + offset
				}
				if mode == "submission" {
					s.Policy.SubmissionLatencyMillis = 1 + offset
				}
				m := reviewCreate(t, s, nil)
				reviewStep(t, m, reviewQuote(118, 1, 118, inside), false, "reset/inside-observation")
				q := reviewQuote(at, 1, available, outside)
				if offset < 0 {
					reviewStep(t, m, q, false, "crossed/bid-crossing", "submitted/admission-pending")
				} else if mode == "submission" {
					reviewStep(t, m, q, false, "crossed/bid-crossing", "expired/submission-not-before-expiry")
				} else {
					reviewStep(t, m, q, false, "expired/expiry-before-admission")
				}
			}
		}
	}
}

func TestIndependentLifecycleIdentityAndContractValidation(t *testing.T) {
	base := reviewSpec(fl.Long)
	id := reviewCreate(t, base, nil).ID()
	mutations := map[string]func(*fl.LevelSpec){
		"provider": func(s *fl.LevelSpec) { s.ProviderID += "2" }, "window": func(s *fl.LevelSpec) { s.SourceWindowID += "2" },
		"context-provider": func(s *fl.LevelSpec) { s.ContextSource.Provider += "2" }, "context-data": func(s *fl.LevelSpec) { s.ContextSource.Dataset += "2" }, "context-digest": func(s *fl.LevelSpec) { s.ContextSource.SHA256 = strings.Repeat("b", 64) },
		"quote-provider": func(s *fl.LevelSpec) { s.QuoteSource.Provider += "2" }, "quote-data": func(s *fl.LevelSpec) { s.QuoteSource.Dataset += "2" }, "quote-digest": func(s *fl.LevelSpec) { s.QuoteSource.SHA256 = strings.Repeat("b", 64) },
		"proof": func(s *fl.LevelSpec) { s.OrderingProof += "2" }, "known-time": func(s *fl.LevelSpec) { s.KnownAt.AtMillis-- }, "known-sequence": func(s *fl.LevelSpec) { s.KnownAt.Sequence-- }, "created-time": func(s *fl.LevelSpec) { s.CreatedAt.AtMillis++ }, "created-sequence": func(s *fl.LevelSpec) { s.CreatedAt.Sequence++ }, "expiry": func(s *fl.LevelSpec) { s.ExpiresAtMillis++ }, "side": func(s *fl.LevelSpec) { s.Side = fl.Short }, "price": func(s *fl.LevelSpec) { s.Price = math.Nextafter(s.Price, math.Inf(1)) }, "assumption": func(s *fl.LevelSpec) { s.Policy.AssumptionID += "2" }, "predecessor-age": func(s *fl.LevelSpec) { s.Policy.MaxPredecessorAgeMillis++ }, "observation-age": func(s *fl.LevelSpec) { s.Policy.MaxObservationAgeMillis++ }, "latency": func(s *fl.LevelSpec) { s.Policy.SubmissionLatencyMillis++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s := base
			mutate(&s)
			if reviewCreate(t, s, nil).ID() == id {
				t.Fatal("identity ignores changed contract")
			}
		})
	}
	invalid := map[string]func(*fl.LevelSpec){
		"missing-version": func(s *fl.LevelSpec) { s.Version = "" }, "unsupported-version": func(s *fl.LevelSpec) { s.Version = "v2" }, "future-known": func(s *fl.LevelSpec) { s.KnownAt.Sequence = 3 }, "invalid-created": func(s *fl.LevelSpec) { s.CreatedAt.Sequence = reviewMaxTime + 1 }, "known-negative": func(s *fl.LevelSpec) { s.KnownAt.AtMillis = -1 }, "empty-window": func(s *fl.LevelSpec) { s.SourceWindowID = "" }, "empty-provider": func(s *fl.LevelSpec) { s.ProviderID = "" }, "empty-proof": func(s *fl.LevelSpec) { s.OrderingProof = "" }, "expiry-at-creation": func(s *fl.LevelSpec) { s.ExpiresAtMillis = 100 }, "expiry-overflow": func(s *fl.LevelSpec) { s.ExpiresAtMillis = math.MaxInt64 }, "zero-price": func(s *fl.LevelSpec) { s.Price = 0 }, "inf-price": func(s *fl.LevelSpec) { s.Price = math.Inf(1) }, "nan-price": func(s *fl.LevelSpec) { s.Price = math.NaN() }, "bad-direction": func(s *fl.LevelSpec) { s.Side = 0 }, "no-assumption": func(s *fl.LevelSpec) { s.Policy.AssumptionID = "" }, "retry-policy": func(s *fl.LevelSpec) { s.Policy.UsePolicy = "retry" }, "negative-latency": func(s *fl.LevelSpec) { s.Policy.SubmissionLatencyMillis = -1 }, "overflow-latency": func(s *fl.LevelSpec) { s.Policy.SubmissionLatencyMillis = math.MaxInt64 }, "zero-freshness": func(s *fl.LevelSpec) { s.Policy.MaxPredecessorAgeMillis = 0 }, "zero-age": func(s *fl.LevelSpec) { s.Policy.MaxObservationAgeMillis = 0 }, "context-side": func(s *fl.LevelSpec) { s.ContextSource.Side = fl.Mid }, "quote-side": func(s *fl.LevelSpec) { s.QuoteSource.Side = fl.Ask }, "bad-digest": func(s *fl.LevelSpec) { s.QuoteSource.SHA256 = "x" },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			s := base
			mutate(&s)
			m, e, err := fl.NewLevelLifecycle(s, nil)
			if err == nil || m != nil || e != (fl.LevelEvent{}) {
				t.Fatalf("accepted bad spec: %+v %+v %v", m, e, err)
			}
		})
	}
}

func TestIndependentLifecycleCreationProofAndCopies(t *testing.T) {
	for _, side := range []fl.Direction{fl.Long, fl.Short} {
		s := reviewSpec(side)
		inside := 100.0
		if side == fl.Short {
			inside = 101
		}
		for _, age := range []int64{10, 11} {
			p := reviewQuote(100-age, 1, 100, inside)
			m, _, err := fl.NewLevelLifecycle(s, &p)
			if (age == 10) != (err == nil) {
				t.Fatal("creation freshness boundary", age, err)
			}
			if age == 10 && m == nil {
				t.Fatal("nil lifecycle")
			}
		}
		for _, mode := range []string{"same-key", "future-key", "future-receipt", "stale-receipt", "source"} {
			p := reviewQuote(99, 1, 99, inside)
			switch mode {
			case "same-key":
				p.Quote.Event = s.CreatedAt
				p.Quote.AvailableAtMillis = 100
			case "future-key":
				p.Quote.Event = fl.EventKey{AtMillis: 100, Sequence: 3}
				p.Quote.AvailableAtMillis = 100
			case "future-receipt":
				p.Quote.AvailableAtMillis = 101
			case "stale-receipt":
				p.Quote.Event.AtMillis = 1
				p.Quote.AvailableAtMillis = 99
			case "source":
				p.Source.Dataset = "wrong"
			}
			if _, _, err := fl.NewLevelLifecycle(s, &p); err == nil {
				t.Fatal("unproved predecessor", mode)
			}
		}
		p := reviewQuote(100, 1, 100, inside)
		m := reviewCreate(t, s, &p)
		p.Quote.Bid = s.Price
		s.Price = 1
		s.Policy.MaxPredecessorAgeMillis = 1
		q := reviewQuote(100, 3, 100, 100.003)
		reviewStep(t, m, q, false, "crossed/bid-crossing", "submitted/admission-pending")
		q.Quote.Event.Sequence = 99
		q.Quote.Bid = 200
		readyQ := reviewQuote(100, 4, 100, 100.003)
		reviewStep(t, m, readyQ, false, "admission-ready/strictly-subsequent-source-observation")
		readyQ.Quote.Event.Sequence = 88
		if _, err := m.ResolveAdmission(fl.EventKey{AtMillis: 100, Sequence: 4}, "consumed"); err != nil {
			t.Fatal("retained mutable caller quote", err)
		}
	}
}

func TestIndependentLifecycleMaximumTimeAndTrace(t *testing.T) {
	var trace []fl.LevelEvent
	for _, side := range []fl.Direction{fl.Long, fl.Short} {
		s := reviewSpec(side)
		s.KnownAt = fl.EventKey{AtMillis: reviewMaxTime - 4, Sequence: reviewMaxTime - 2}
		s.CreatedAt = fl.EventKey{AtMillis: reviewMaxTime - 4, Sequence: reviewMaxTime - 1}
		s.ExpiresAtMillis = reviewMaxTime
		s.Policy.SubmissionLatencyMillis = 1
		inside := 100.0
		if side == fl.Short {
			inside = 101
		}
		p := reviewQuote(reviewMaxTime-4, reviewMaxTime-3, reviewMaxTime-4, inside)
		m, e, err := fl.NewLevelLifecycle(s, &p)
		if err != nil {
			t.Fatal(err)
		}
		trace = append(trace, e)
		es := reviewStep(t, m, reviewQuote(reviewMaxTime-3, reviewMaxTime, reviewMaxTime-3, s.Price), false, "crossed/bid-crossing", "submitted/admission-pending")
		trace = append(trace, es...)
		ready := reviewQuote(reviewMaxTime-2, 0, reviewMaxTime-2, s.Price)
		es = reviewStep(t, m, ready, false, "admission-ready/strictly-subsequent-source-observation")
		trace = append(trace, es...)
		e, err = m.ResolveAdmission(ready.Quote.Event, "consumed")
		if err != nil {
			t.Fatal(err)
		}
		trace = append(trace, e)
	}
	b, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("INDEPENDENT_LIFECYCLE_TRACE=" + string(b))
}

func TestIndependentLifecycleCannotSkipReadyForBetterPrice(t *testing.T) {
	m := reviewCreate(t, reviewSpec(fl.Long), nil)
	reviewStep(t, m, reviewQuote(101, 1, 101, 100), false, "reset/inside-observation")
	reviewStep(t, m, reviewQuote(102, 1, 102, 101), false, "crossed/bid-crossing", "submitted/admission-pending")
	ready := reviewQuote(103, 1, 103, 101)
	reviewStep(t, m, ready, false, "admission-ready/strictly-subsequent-source-observation")
	es, err := m.Observe(reviewQuote(104, 1, 104, 99), true, false)
	if err == nil || es[0].Reason != "unresolved-admission-result" || !m.Terminal() {
		t.Fatal(es, err)
	}
	if _, err = m.ResolveAdmission(ready.Quote.Event, "consumed"); err == nil {
		t.Fatal("resolved after observing better price")
	}
}
