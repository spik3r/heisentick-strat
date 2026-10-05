package frozenlevels

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func lifecycleSpec(side Direction) LevelSpec {
	return LevelSpec{Version: LifecycleVersion, ProviderID: "synthetic-stored-m30-control", SourceWindowID: "synthetic-completed-window-v1", ContextSource: source(), QuoteSource: source(), OrderingProof: "synthetic-total-order-v1", KnownAt: EventKey{AtMillis: 1000, Sequence: 9}, CreatedAt: EventKey{AtMillis: 1000, Sequence: 10}, ExpiresAtMillis: 10000, Side: side, Price: 100, Policy: LevelPolicy{AssumptionID: "reference-one-use-bid-cross-v1", MaxPredecessorAgeMillis: 5000, MaxObservationAgeMillis: 500, SubmissionLatencyMillis: 0, UsePolicy: "one-success-no-retry"}}
}
func levelQuote(at, seq int64, bid float64) LevelQuote {
	return LevelQuote{Quote: quote(at, seq, bid, bid+.1), Source: source()}
}
func levelKinds(events []LevelEvent) []string {
	var k []string
	for _, e := range events {
		k = append(k, e.Kind)
	}
	return k
}
func lifecycle(t *testing.T, spec LevelSpec, previous *LevelQuote) *LevelLifecycle {
	t.Helper()
	m, created, err := NewLevelLifecycle(spec, previous)
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "created" || created.LevelID == "" || m.ID() != created.LevelID || m.State() != "active" {
		t.Fatal("invalid creation", created)
	}
	return m
}
func stepLevel(t *testing.T, m *LevelLifecycle, q LevelQuote, busy bool, want ...string) []LevelEvent {
	t.Helper()
	events, err := m.Observe(q, true, busy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(levelKinds(events), want) {
		t.Fatalf("got %v want %v events=%+v", levelKinds(events), want, events)
	}
	return events
}
func TestLevelAlreadyOutsideResetAndOneUse(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		s := lifecycleSpec(side)
		m := lifecycle(t, s, nil)
		inside, outside := 99.0, 101.0
		if side == Short {
			inside, outside = 101, 99
		}
		out := stepLevel(t, m, levelQuote(1001, 1, outside), false, "reset")
		if out[0].Reason != "already-outside" {
			t.Fatal("outside level generated a signal")
		}
		stepLevel(t, m, levelQuote(1002, 1, outside), false)
		stepLevel(t, m, levelQuote(1003, 1, inside), false, "reset")
		events := stepLevel(t, m, levelQuote(1004, 1, 100), false, "crossed", "submitted")
		if events[0].SignalID == "" || events[0].SignalID != events[1].SignalID || m.State() != "pending" {
			t.Fatal("missing signal identity")
		}
		// A strictly later source sequence at the same timestamp can be admitted.
		ready := levelQuote(1004, 2, outside)
		stepLevel(t, m, ready, false, "admission-ready")
		consumed, err := m.ResolveAdmission(ready.Quote.Event, "consumed")
		if err != nil || consumed.Kind != "consumed" || !m.Terminal() {
			t.Fatal("not consumed", err)
		}
		if _, err = m.Observe(levelQuote(1005, 1, inside), true, false); err == nil {
			t.Fatal("reused consumed level")
		}
		if _, err = m.ResolveAdmission(ready.Quote.Event, "consumed"); err == nil {
			t.Fatal("consumed twice")
		}
	}
}
func TestLevelFreshPredecessorAtCreationAndBoundaryEquality(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		s := lifecycleSpec(side)
		inside := 99.0
		if side == Short {
			inside = 101
		}
		previous := levelQuote(1000, 8, inside)
		m := lifecycle(t, s, &previous)
		stepLevel(t, m, levelQuote(1000, 11, 100), false, "crossed", "submitted")
		ready := levelQuote(1000, 12, 100)
		stepLevel(t, m, ready, false, "admission-ready")
		if _, err := m.ResolveAdmission(ready.Quote.Event, "busy"); err != nil {
			t.Fatal(err)
		}
		stepLevel(t, m, levelQuote(1000, 13, 100), false) // equal repeated boundary cannot retrigger
		stepLevel(t, m, levelQuote(1000, 14, inside), false, "reset")
		stepLevel(t, m, levelQuote(1000, 15, 100), false, "crossed", "submitted")
	}
}
func TestLevelBusyCrossingDoesNotConsumeOrAutoRetrigger(t *testing.T) {
	m := lifecycle(t, lifecycleSpec(Long), nil)
	stepLevel(t, m, levelQuote(1001, 1, 99), false, "reset")
	first := stepLevel(t, m, levelQuote(1002, 1, 100), true, "crossed", "skipped")
	stepLevel(t, m, levelQuote(1003, 1, 101), false)
	stepLevel(t, m, levelQuote(1004, 1, 99), false, "reset")
	second := stepLevel(t, m, levelQuote(1005, 1, 100), false, "crossed", "submitted")
	if first[0].SignalID == second[0].SignalID || first[0].LevelID != second[0].LevelID {
		t.Fatal("crossing vs level identity")
	}
	stepLevel(t, m, levelQuote(1006, 1, 101), true, "skipped")
	if m.State() != "active" {
		t.Fatal("busy admission consumed level")
	}
	stepLevel(t, m, levelQuote(1007, 1, 101), false)
	stepLevel(t, m, levelQuote(1008, 1, 99), false, "reset")
	stepLevel(t, m, levelQuote(1009, 1, 100), false, "crossed", "submitted")
}
func TestLevelFreshnessAndStalePredecessorReset(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		s := lifecycleSpec(side)
		s.Policy.MaxPredecessorAgeMillis = 5
		inside, outside := 99.0, 101.0
		if side == Short {
			inside, outside = 101, 99
		}
		for _, gap := range []int64{5, 6} {
			m := lifecycle(t, s, nil)
			stepLevel(t, m, levelQuote(1001, 1, inside), false, "reset")
			if gap == 5 {
				stepLevel(t, m, levelQuote(1001+gap, 1, outside), false, "crossed", "submitted")
			} else {
				events := stepLevel(t, m, levelQuote(1001+gap, 1, outside), false, "reset")
				if events[0].Reason != "stale-predecessor" {
					t.Fatal(events)
				}
				stepLevel(t, m, levelQuote(1008, 1, outside), false)
				stepLevel(t, m, levelQuote(1009, 1, inside), false, "reset")
				stepLevel(t, m, levelQuote(1010, 1, outside), false, "crossed", "submitted")
			}
		}
	}
}
func TestLevelExpiryAtSignalAndPendingAdmission(t *testing.T) {
	for _, at := range []int64{9999, 10000} {
		s := lifecycleSpec(Long)
		m := lifecycle(t, s, nil)
		stepLevel(t, m, levelQuote(at-1, 1, 99), false, "reset")
		if at < 10000 {
			stepLevel(t, m, levelQuote(at, 1, 100), false, "crossed", "submitted")
			stepLevel(t, m, levelQuote(10000, 1, 101), false, "expired")
		} else if at == 10000 {
			stepLevel(t, m, levelQuote(at, 1, 100), false, "expired")
		}
	}
}
func TestLevelDelayedAvailabilityCannotBeatExpiryOrSubmission(t *testing.T) {
	s := lifecycleSpec(Long)
	s.Policy.SubmissionLatencyMillis = 100
	m := lifecycle(t, s, nil)
	stepLevel(t, m, levelQuote(1001, 1, 99), false, "reset")
	signal := levelQuote(1002, 1, 100)
	signal.Quote.AvailableAtMillis = 1100
	stepLevel(t, m, signal, false, "crossed", "submitted")
	// The next source event arrived later but predates submission+latency=1200.
	before := levelQuote(1199, 1, 101)
	before.Quote.AvailableAtMillis = 1250
	stepLevel(t, m, before, false)
	exact := levelQuote(1200, 1, 101)
	exact.Quote.AvailableAtMillis = 1250
	stepLevel(t, m, exact, false, "admission-ready")
	s = lifecycleSpec(Long)
	m = lifecycle(t, s, nil)
	stepLevel(t, m, levelQuote(9998, 1, 99), false, "reset")
	late := levelQuote(9999, 1, 100)
	late.Quote.AvailableAtMillis = 10000
	stepLevel(t, m, late, false, "expired")
	s = lifecycleSpec(Long)
	s.Policy.SubmissionLatencyMillis = 100
	m = lifecycle(t, s, nil)
	stepLevel(t, m, levelQuote(9899, 1, 99), false, "reset")
	out := stepLevel(t, m, levelQuote(9900, 1, 100), false, "crossed", "expired")
	if out[1].Reason != "submission-not-before-expiry" {
		t.Fatal(out)
	}
}
func TestLevelMalformedDuplicateStaleAndUnknownInputInvalidates(t *testing.T) {
	mutations := map[string]func(*LevelQuote){
		"duplicate":       func(q *LevelQuote) { q.Quote.Event = EventKey{AtMillis: 1001, Sequence: 1} },
		"reordered":       func(q *LevelQuote) { q.Quote.Event = EventKey{AtMillis: 1000, Sequence: 11} },
		"before creation": func(q *LevelQuote) { q.Quote.Event = EventKey{AtMillis: 1000, Sequence: 10} },
		"availability regression": func(q *LevelQuote) {
			q.Quote.Event.AtMillis = 1001
			q.Quote.Event.Sequence = 2
			q.Quote.AvailableAtMillis = 1001
		},
		"nan":           func(q *LevelQuote) { q.Quote.Bid = math.NaN() },
		"infinite":      func(q *LevelQuote) { q.Quote.Ask = math.Inf(1) },
		"crossed quote": func(q *LevelQuote) { q.Quote.Ask = 99 },
		"wrong source":  func(q *LevelQuote) { q.Source.Dataset = "wrong" },
		"stale current": func(q *LevelQuote) { q.Quote.AvailableAtMillis = q.Quote.Event.AtMillis + 501 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			m := lifecycle(t, lifecycleSpec(Long), nil)
			initial := levelQuote(1001, 1, 99)
			initial.Quote.AvailableAtMillis = 1002
			stepLevel(t, m, initial, false, "reset")
			bad := levelQuote(1003, 1, 100)
			mutate(&bad)
			events, err := m.Observe(bad, true, false)
			if err == nil || len(events) != 1 || events[0].Kind != "invalidated" || !m.Terminal() {
				t.Fatal("did not fail closed", events, err)
			}
			if _, err = m.Observe(levelQuote(2000, 1, 99), true, false); err == nil {
				t.Fatal("silently repaired invalid input")
			}
		})
	}
	m := lifecycle(t, lifecycleSpec(Long), nil)
	if _, err := m.Observe(levelQuote(1001, 1, 99), false, false); err == nil || !m.Terminal() {
		t.Fatal("unknown gap was accepted")
	}
}
func TestLevelContractsAndIdentity(t *testing.T) {
	s := lifecycleSpec(Long)
	a := lifecycle(t, s, nil)
	b := lifecycle(t, s, nil)
	if a.ID() != b.ID() {
		t.Fatal("unstable ID")
	}
	s.Side = Short
	b = lifecycle(t, s, nil)
	if a.ID() == b.ID() {
		t.Fatal("directions share a level ID/OCO")
	}
	s = lifecycleSpec(Long)
	s.Policy.AssumptionID = "another-version"
	b = lifecycle(t, s, nil)
	if a.ID() == b.ID() {
		t.Fatal("assumption omitted from identity")
	}
	for name, mutate := range map[string]func(*LevelSpec){
		"version": func(s *LevelSpec) { s.Version = "" }, "source": func(s *LevelSpec) { s.QuoteSource.SHA256 = "" }, "ordering proof": func(s *LevelSpec) { s.OrderingProof = "" }, "future knowledge": func(s *LevelSpec) { s.KnownAt.Sequence = 11 }, "expiry": func(s *LevelSpec) { s.ExpiresAtMillis = 1000 }, "side": func(s *LevelSpec) { s.Side = 0 }, "price": func(s *LevelSpec) { s.Price = math.NaN() }, "policy": func(s *LevelSpec) { s.Policy.UsePolicy = "retry" }, "freshness": func(s *LevelSpec) { s.Policy.MaxPredecessorAgeMillis = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			s := lifecycleSpec(Long)
			mutate(&s)
			if _, _, err := NewLevelLifecycle(s, nil); err == nil {
				t.Fatal("invalid contract accepted")
			}
		})
	}
	previous := levelQuote(1000, 11, 99)
	if _, _, err := NewLevelLifecycle(lifecycleSpec(Long), &previous); err == nil {
		t.Fatal("future predecessor")
	}
	previous = levelQuote(999, 1, 99)
	previous.Quote.AvailableAtMillis = 1001
	if _, _, err := NewLevelLifecycle(lifecycleSpec(Long), &previous); err == nil {
		t.Fatal("not yet available predecessor")
	}
	s = lifecycleSpec(Long)
	s.Policy.MaxPredecessorAgeMillis = 1
	previous = levelQuote(998, 1, 99)
	if _, _, err := NewLevelLifecycle(s, &previous); err == nil {
		t.Fatal("stale predecessor")
	}
}
func TestLevelAdmissionMustResolveBeforeFurtherObservation(t *testing.T) {
	s := lifecycleSpec(Long)
	previous := levelQuote(1000, 8, 99)
	m := lifecycle(t, s, &previous)
	stepLevel(t, m, levelQuote(1000, 11, 100), false, "crossed", "submitted")
	ready := levelQuote(1000, 12, 101)
	stepLevel(t, m, ready, false, "admission-ready")
	if _, err := m.ResolveAdmission(EventKey{AtMillis: 1000, Sequence: 13}, "consumed"); err == nil || m.State() != "ready" {
		t.Fatal("wrong resolution key accepted")
	}
	if _, err := m.ResolveAdmission(ready.Quote.Event, "guess"); err == nil || m.State() != "ready" {
		t.Fatal("unsupported resolution accepted")
	}
	if _, err := m.Observe(levelQuote(1000, 13, 102), true, false); err == nil || !m.Terminal() {
		t.Fatal("skipped ready result to choose later price")
	}
	m = lifecycle(t, s, &previous)
	stepLevel(t, m, levelQuote(1000, 11, 100), false, "crossed", "submitted")
	stepLevel(t, m, ready, false, "admission-ready")
	e, err := m.ResolveAdmission(ready.Quote.Event, "rejected")
	if err != nil || e.Kind != "invalidated" || !m.Terminal() {
		t.Fatal("rejection retried", e, err)
	}
}
func TestLifecycleParityTrace(t *testing.T) {
	var trace []LevelEvent
	for _, side := range []Direction{Long, Short} {
		spec := lifecycleSpec(side)
		m, created, err := NewLevelLifecycle(spec, nil)
		if err != nil {
			t.Fatal(err)
		}
		trace = append(trace, created)
		inside, outside := 99.0, 101.0
		if side == Short {
			inside, outside = 101, 99
		}
		trace = append(trace, stepLevel(t, m, levelQuote(1001, 1, outside), false, "reset")...)
		trace = append(trace, stepLevel(t, m, levelQuote(1002, 1, inside), false, "reset")...)
		trace = append(trace, stepLevel(t, m, levelQuote(1003, 1, 100), false, "crossed", "submitted")...)
		ready := levelQuote(1003, 2, outside)
		trace = append(trace, stepLevel(t, m, ready, false, "admission-ready")...)
		consumed, err := m.ResolveAdmission(ready.Quote.Event, "consumed")
		if err != nil {
			t.Fatal(err)
		}
		trace = append(trace, consumed)
	}
	b, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("LIFECYCLE_TRACE=" + string(b))
}

func TestLevelExactCrossingSidesAndFreshnessBoundaries(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		for _, offset := range []int{-1, 0, 1} {
			s := lifecycleSpec(side)
			s.Price = 100.01
			inside := 99.0
			if side == Short {
				inside = 101
			}
			previous := levelQuote(1000, 8, inside)
			m := lifecycle(t, s, &previous)
			value := 100.01
			if offset != 0 {
				value = adjacent(value, side, offset > 0)
			}
			if offset < 0 {
				stepLevel(t, m, levelQuote(1000, 11, value), false)
			} else {
				stepLevel(t, m, levelQuote(1000, 11, value), false, "crossed", "submitted")
			}
		}
	}
	for _, age := range []int64{500, 501} {
		m := lifecycle(t, lifecycleSpec(Long), nil)
		q := levelQuote(1001, 1, 99)
		q.Quote.AvailableAtMillis += age
		events, err := m.Observe(q, true, false)
		if age == 500 {
			if err != nil || events[0].Kind != "reset" {
				t.Fatal("exact freshness rejected", events, err)
			}
		} else if err == nil || !m.Terminal() {
			t.Fatal("stale observation admitted")
		}
	}
}
func TestLevelPendingDoesNotReevaluateSignalFromLaterShape(t *testing.T) {
	m := lifecycle(t, lifecycleSpec(Long), nil)
	stepLevel(t, m, levelQuote(1001, 1, 99), false, "reset")
	signal := stepLevel(t, m, levelQuote(1002, 1, 100), false, "crossed", "submitted")
	// Subsequent quote may be back inside; this primitive admits the observation,
	// not a guaranteed fill or structural stop. External validation still decides.
	ready := stepLevel(t, m, levelQuote(1003, 1, 98), false, "admission-ready")
	if ready[0].SignalID != signal[0].SignalID {
		t.Fatal("hindsight changed signal identity")
	}
}
func TestLevelPendingExpiryPastBoundaryAndTimestampOverflow(t *testing.T) {
	m := lifecycle(t, lifecycleSpec(Long), nil)
	stepLevel(t, m, levelQuote(9998, 1, 99), false, "reset")
	stepLevel(t, m, levelQuote(9999, 1, 100), false, "crossed", "submitted")
	stepLevel(t, m, levelQuote(10001, 1, 100), false, "expired")
	if _, err := m.ResolveAdmission(EventKey{AtMillis: 10001, Sequence: 1}, "consumed"); err == nil {
		t.Fatal("expired entry consumed")
	}
	s := lifecycleSpec(Long)
	s.CreatedAt.AtMillis = maxExactMillis - 5
	s.KnownAt.AtMillis = maxExactMillis - 6
	s.ExpiresAtMillis = maxExactMillis
	s.Policy.SubmissionLatencyMillis = 4
	m = lifecycle(t, s, nil)
	stepLevel(t, m, levelQuote(maxExactMillis-4, 1, 99), false, "reset")
	events, err := m.Observe(levelQuote(maxExactMillis-2, 1, 100), true, false)
	if err != nil || len(events) != 2 || events[0].Kind != "crossed" || events[1].Kind != "expired" || events[1].Reason != "submission-not-before-expiry" || m.State() != "expired" || events[0].SignalID != events[1].SignalID {
		t.Fatal("overflowing submission lost crossing-plus-expiry audit", events, err)
	}
}
func TestLevelSourceInputsAreCopiedAndResolvedOnlyProspectively(t *testing.T) {
	s := lifecycleSpec(Long)
	previous := levelQuote(1000, 8, 99)
	m := lifecycle(t, s, &previous)
	previous.Quote.Bid = 101
	s.Price = 1000
	s.Policy.UsePolicy = "reuse"
	stepLevel(t, m, levelQuote(1000, 11, 100), false, "crossed", "submitted")
	if _, err := m.ResolveAdmission(EventKey{AtMillis: 1000, Sequence: 11}, "consumed"); err == nil {
		t.Fatal("filled on signal observation")
	}
}
