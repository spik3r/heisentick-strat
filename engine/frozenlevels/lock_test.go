package frozenlevels

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func quote(at int64, seq int64, bid, ask float64) Quote {
	return Quote{Event: EventKey{AtMillis: at, Sequence: seq}, AvailableAtMillis: at, Bid: bid, Ask: ask}
}
func seed(side Direction) PositionSeed {
	at := ms("2026-03-09T00:00:00Z")
	stop := 90.0
	bid, ask := 99.9, 100.0
	if side == Short {
		stop = 110
		bid, ask = 100, 100.1
	}
	return PositionSeed{Side: side, Entry: 100, InitialStop: stop, EntryQuote: quote(at, 1, bid, ask)}
}
func policy(mode LockMode) LockPolicy {
	return LockPolicy{Mode: mode, ActivationR: 1, Grid: .01, LatencyMillis: 0, AssumptionID: "synthetic-reference-control-unverified-activation"}
}
func levels(s PositionSeed, prices ...float64) *LockLevels {
	return &LockLevels{ID: "invented-levels", SourceWindowID: "invented-window", KnownAtMillis: s.EntryQuote.Event.AtMillis - 2, FrozenAtMillis: s.EntryQuote.Event.AtMillis - 1, Prices: prices}
}
func kinds(events []LockEvent) []string {
	var r []string
	for _, e := range events {
		r = append(r, e.Kind)
	}
	return r
}
func observe(t *testing.T, m *LockTracker, q Quote, want ...string) []LockEvent {
	t.Helper()
	got, e := m.Observe(q, true)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(kinds(got), want) {
		t.Fatalf("event kinds: got %v want %v (%+v)", kinds(got), want, got)
	}
	return got
}
func TestPivotLockAcceptedBeforeCommonTarget(t *testing.T) {
	s := seed(Long)
	m, e := NewLockTracker(s, policy(PivotLock), nil, levels(s, 106, 112, 127))
	if e != nil {
		t.Fatal(e)
	}
	if m.CommonTarget() != 120 {
		t.Fatal("target is not common 2R")
	}
	at := s.EntryQuote.Event.AtMillis
	events := observe(t, m, quote(at, 2, 110, 110.1), "activated", "submitted")
	if m.ActiveStop() != 90 || events[1].ProposedStop != 106 {
		t.Fatal("lock became retrospective or wrong pivot")
	}
	// Same timestamp, increasing source sequence is strictly subsequent.
	observe(t, m, quote(at, 3, 109, 109.1), "accepted")
	if m.ActiveStop() != 106 || m.Counters() != (LockCounters{Activations: 1, Eligible: 1, Submitted: 1, Accepted: 1}) {
		t.Fatalf("lock not accepted: %+v", m.Counters())
	}
	observe(t, m, quote(at, 4, 105, 105.1), "exit-due")
	if !m.Terminal() {
		t.Fatal("stop not due")
	}
}
func TestShortAskAndLongBidActivation(t *testing.T) {
	s := seed(Short)
	m, e := NewLockTracker(s, policy(PivotLock), nil, levels(s, 94, 88, 73))
	if e != nil {
		t.Fatal(e)
	}
	at := s.EntryQuote.Event.AtMillis
	// Bid reaches +1R but ask does not; must not activate the short lock yet.
	observe(t, m, quote(at, 2, 89.9, 90.1))
	observe(t, m, quote(at, 3, 89.9, 90), "activated", "submitted")
	observe(t, m, quote(at, 4, 90, 90.1), "accepted")
	if m.ActiveStop() != 94 || m.CommonTarget() != 80 {
		t.Fatal("short geometry")
	}
	out := observe(t, m, quote(at, 5, 93.99, 94), "exit-due")
	if out[0].Reason != "stop" {
		t.Fatal("short stop ignored ask")
	}
	s = seed(Long)
	m, e = NewLockTracker(s, policy(PivotLock), nil, levels(s, 106))
	if e != nil {
		t.Fatal(e)
	}
	observe(t, m, quote(at, 2, 109.9, 110.1))
	if m.Counters().Activations != 0 {
		t.Fatal("long activated from ask")
	}
}
func TestOldOrdersPrecedeAmendmentAndTargetPreemptsRequest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bid    float64
		reason string
	}{{"stop", 89, "stop"}, {"target", 121, "target"}} {
		t.Run(tc.name, func(t *testing.T) {
			s := seed(Long)
			m, e := NewLockTracker(s, policy(PivotLock), nil, levels(s, 106))
			if e != nil {
				t.Fatal(e)
			}
			at := s.EntryQuote.Event.AtMillis
			observe(t, m, quote(at, 2, 110, 110.1), "activated", "submitted")
			got := observe(t, m, quote(at, 3, tc.bid, tc.bid+.1), "canceled", "exit-due")
			if got[1].Reason != tc.reason || m.ActiveStop() != 90 || m.Counters().Accepted != 0 || m.Counters().Canceled != 1 {
				t.Fatal("old order lost precedence")
			}
		})
	}
	s := seed(Long)
	m, e := NewLockTracker(s, policy(PivotLock), nil, levels(s, 106))
	if e != nil {
		t.Fatal(e)
	}
	observe(t, m, quote(s.EntryQuote.Event.AtMillis, 2, 121, 121.1), "exit-due")
	if m.Counters().Activations != 0 {
		t.Fatal("activated on target exit quote")
	}
}
func TestLockRejectsAtRequestAndAcceptanceWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		prices       []float64
		acceptBid    float64
		requestKinds []string
	}{
		{"no eligible", []float64{112, 127}, 111, []string{"activated", "rejected"}},
		{"empty ladder", nil, 111, []string{"activated", "rejected"}},
		{"rejected acceptance", []float64{106}, 106, []string{"activated", "submitted"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := seed(Long)
			m, e := NewLockTracker(s, policy(PivotLock), nil, levels(s, tc.prices...))
			if e != nil {
				t.Fatal(e)
			}
			at := s.EntryQuote.Event.AtMillis
			observe(t, m, quote(at, 2, 110, 110.1), tc.requestKinds...)
			if m.Counters().Submitted == 1 {
				observe(t, m, quote(at, 3, tc.acceptBid, tc.acceptBid+.1), "rejected")
			}
			observe(t, m, quote(at, 4, 115, 115.1))
			if m.ActiveStop() != 90 || m.Counters().Activations != 1 || m.Counters().Rejected != 1 {
				t.Fatalf("retry or widened stop: %+v", m.Counters())
			}
		})
	}
}
func TestScaleAndNoLockCommonTarget(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		s := seed(side)
		f := frozenOpening(t, 3800)
		m, e := NewLockTracker(s, policy(ScaleLock), &f, nil)
		if e != nil {
			t.Fatal(e)
		}
		at := s.EntryQuote.Event.AtMillis
		bid, ask := 110.0, 110.1
		if side == Short {
			bid, ask = 89.9, 90
		}
		observe(t, m, quote(at, 2, bid, ask), "activated", "submitted")
		observe(t, m, quote(at, 3, bid, ask), "accepted")
		if m.ActiveStop() != 100+float64(side) {
			t.Fatal("wrong scale lock")
		}
		no, e := NewLockTracker(s, policy(NoLock), nil, nil)
		if e != nil {
			t.Fatal(e)
		}
		observe(t, no, quote(at, 2, bid, ask))
		if no.CommonTarget() != m.CommonTarget() || no.ActiveStop() != s.InitialStop {
			t.Fatal("control target or stop changed")
		}
	}
}
func TestLatencySourceSequenceAndUnresolvedGap(t *testing.T) {
	s := seed(Long)
	p := policy(PivotLock)
	p.LatencyMillis = 100
	m, e := NewLockTracker(s, p, nil, levels(s, 106))
	if e != nil {
		t.Fatal(e)
	}
	at := s.EntryQuote.Event.AtMillis
	observe(t, m, quote(at, 2, 110, 110.1), "activated", "submitted")
	observe(t, m, quote(at+99, 1, 110, 110.1))
	if m.ActiveStop() != 90 {
		t.Fatal("accepted before latency")
	}
	observe(t, m, quote(at+100, 1, 110, 110.1), "accepted")
	before := m.Counters()
	if _, e = m.Observe(quote(at+100, 1, 110, 110.1), true); e == nil || m.Counters() != before {
		t.Fatal("duplicate consumed")
	}
	got, e := m.Observe(quote(at+101, 1, 110, 110.1), false)
	if e != nil || got[0].Kind != "unresolved" || !m.Terminal() {
		t.Fatal("gap not unresolved")
	}
	if _, e = m.Observe(quote(at+102, 1, 110, 110.1), true); e == nil {
		t.Fatal("continued across unknown path")
	}
}
func TestContextFrozenAndNoImplicitModes(t *testing.T) {
	s := seed(Long)
	l := levels(s, 106, 112, 127)
	m, e := NewLockTracker(s, policy(PivotLock), nil, l)
	if e != nil {
		t.Fatal(e)
	}
	l.Prices[0] = 108
	observe(t, m, quote(s.EntryQuote.Event.AtMillis, 2, 110, 110.1), "activated", "submitted")
	if m.proposed.project() != 106 {
		t.Fatal("mutable ladder retained")
	}
	for _, bad := range []LockPolicy{{Mode: PivotLock}, {Mode: "unknown", ActivationR: 1, Grid: .01, AssumptionID: "x"}, {Mode: PivotLock, ActivationR: math.NaN(), Grid: .01, AssumptionID: "x"}} {
		if _, e := NewLockTracker(s, bad, nil, l); e == nil {
			t.Fatal("invalid policy accepted")
		}
	}
	l.FrozenAtMillis++
	if _, e := NewLockTracker(s, policy(PivotLock), nil, l); e == nil {
		t.Fatal("future ladder accepted")
	}
}
func TestPrimitiveParityTrace(t *testing.T) {
	bars := syntheticBars()
	var windows []Window
	var ladders []Ladder
	for _, kind := range []WindowKind{RollingM30, ClockM30, ClockM15} {
		w, e := FreezeWindow(kind, ms("2026-02-05T00:15:00Z"), bars)
		if e != nil {
			t.Fatal(e)
		}
		l, e := Camarilla(w)
		if e != nil {
			t.Fatal(e)
		}
		windows = append(windows, w)
		ladders = append(ladders, l)
	}
	f := frozenOpening(t, 3857)
	scale, e := PriorOpenScaleLocation(f, 100, Long, .01)
	if e != nil {
		t.Fatal(e)
	}
	s := seed(Long)
	m, e := NewLockTracker(s, policy(PivotLock), nil, levels(s, 106, 112, 127))
	if e != nil {
		t.Fatal(e)
	}
	var events []LockEvent
	at := s.EntryQuote.Event.AtMillis
	events = append(events, observe(t, m, quote(at, 2, 110, 110.1), "activated", "submitted")...)
	events = append(events, observe(t, m, quote(at, 3, 109, 109.1), "accepted")...)
	events = append(events, observe(t, m, quote(at, 4, 105, 105.1), "exit-due")...)
	result := struct {
		Version  string
		Windows  []Window
		Ladders  []Ladder
		Opening  FrozenOpening
		Scale    ScaleLocation
		Events   []LockEvent
		Counters LockCounters
	}{ContractVersion, windows, ladders, f, scale, events, m.Counters()}
	b, e := json.Marshal(result)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("PRIMITIVE_TRACE=" + string(b))
}

func TestScaleAdmissibilityAndOneAttempt(t *testing.T) {
	s := seed(Long)
	s.InitialStop = 99.5
	f := frozenOpening(t, 3800)
	m, e := NewLockTracker(s, policy(ScaleLock), &f, nil)
	if e != nil {
		t.Fatal(e)
	}
	at := s.EntryQuote.Event.AtMillis
	// +1R is 100.5, but the evidence-inspired location is 101: reject once.
	observe(t, m, quote(at, 2, 100.5, 100.6), "activated", "rejected")
	observe(t, m, quote(at, 3, 100.8, 100.9))
	if m.Counters().Activations != 1 || m.ActiveStop() != 99.5 {
		t.Fatal("retried rejected scale lock")
	}
	s = seed(Long)
	m, e = NewLockTracker(s, policy(ScaleLock), &f, nil)
	if e != nil {
		t.Fatal(e)
	}
	observe(t, m, quote(at, 2, 110, 110.1), "activated", "submitted")
	observe(t, m, quote(at, 3, 100.99, 101.09), "rejected")
	observe(t, m, quote(at, 4, 112, 112.1))
	if m.Counters().Accepted != 0 || m.Counters().Rejected != 1 || m.ActiveStop() != 90 {
		t.Fatal("invalid scale accepted or retried")
	}
}
func TestStrictlySubsequentAndAvailability(t *testing.T) {
	s := seed(Long)
	m, e := NewLockTracker(s, policy(PivotLock), nil, levels(s, 106))
	if e != nil {
		t.Fatal(e)
	}
	at := s.EntryQuote.Event.AtMillis
	for _, q := range []Quote{quote(at, 1, 110, 110.1), quote(at-1, 100, 110, 110.1), quote(at, 2, 110, 109.9), quote(at, 2, math.NaN(), 111)} {
		if _, e = m.Observe(q, true); e == nil {
			t.Fatal("invalid observation admitted")
		}
	}
	delayed := quote(at, 2, 110, 110.1)
	delayed.AvailableAtMillis = at + 100
	observe(t, m, delayed, "activated", "submitted")
	stale := quote(at+50, 3, 109, 109.1)
	if _, e = m.Observe(stale, true); e == nil {
		t.Fatal("availability regressed")
	}
	later := quote(at, 3, 109, 109.1)
	later.AvailableAtMillis = at + 100
	observe(t, m, later, "accepted")
	if _, e = m.Observe(later, true); e == nil {
		t.Fatal("same event amended twice")
	}
}
func TestRejectedInvalidSeedsAndContexts(t *testing.T) {
	s := seed(Long)
	p := policy(PivotLock)
	for name, mutate := range map[string]func(*PositionSeed){
		"zero side": func(s *PositionSeed) { s.Side = 0 }, "wrong stop": func(s *PositionSeed) { s.InitialStop = 101 }, "nonfinite entry": func(s *PositionSeed) { s.Entry = math.Inf(1) }, "invalid liquidation": func(s *PositionSeed) { s.EntryQuote.Bid = 89 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := s
			mutate(&bad)
			if _, e := NewLockTracker(bad, p, nil, levels(s, 106)); e == nil {
				t.Fatal("invalid seed admitted")
			}
		})
	}
	f := frozenOpening(t, 3800)
	if _, e := NewLockTracker(s, policy(NoLock), &f, nil); e == nil {
		t.Fatal("unused context accepted")
	}
	if _, e := NewLockTracker(s, p, nil, levels(s, math.Inf(1))); e == nil {
		t.Fatal("infinite pivot accepted")
	}
	if _, e := NewLockTracker(s, policy(ScaleLock), nil, nil); e == nil {
		t.Fatal("missing scale accepted")
	}
}
func TestArchivedTargetPreemptionAndSubgridQualification(t *testing.T) {
	// Hand-derived geometry only; this package intentionally implements common
	// 2R, not the archived nearest-pivot target. A quote permitting a 106 lock
	// from 100 must pass a nearest target of 106 first.
	entry, grid, target, lock, liq := 100.0, .01, 106.0, 106.0, 106.01
	if !(lock-entry >= grid && liq-lock >= grid-1e-12 && liq >= target) {
		t.Fatal("ordinary preemption geometry")
	}
	// Off-grid fill/raw level is a narrow exception, not a useful net lock.
	entry = 100.009
	raw := 100.011
	snapped := gridSnap(decimal(raw), grid, false)
	if raw-entry >= grid || snapped <= entry || snapped-entry-.06 >= 0 {
		t.Fatal("subgrid cost qualification")
	}
}
