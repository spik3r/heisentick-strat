package frozenlevels

import (
	"math"
	"reflect"
	"testing"
)

func TestReviewExactOneGridPivotEligibilityLong(t *testing.T) {
	s := seed(Long)
	m, err := NewLockTracker(s, policy(PivotLock), nil, levels(s, 110.01))
	if err != nil {
		t.Fatal(err)
	}
	q := quote(s.EntryQuote.Event.AtMillis, 2, 110.02, 110.03)
	events, err := m.Observe(q, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(events), []string{"activated", "submitted"}) {
		t.Fatalf("exact decimal one-grid pivot rejected: 110.02 - 110.01 = %.17g, grid .01; events=%+v", 110.02-float64(110.01), events)
	}
}

func TestReviewExactOneGridPivotEligibilityShort(t *testing.T) {
	s := seed(Short)
	m, err := NewLockTracker(s, policy(PivotLock), nil, levels(s, 89.99))
	if err != nil {
		t.Fatal(err)
	}
	events, err := m.Observe(quote(s.EntryQuote.Event.AtMillis, 2, 89.97, 89.98), true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(events), []string{"activated", "submitted"}) {
		t.Fatalf("exact decimal one-grid short pivot rejected: 89.99 - 89.98 = %.17g, grid .01; events=%+v", 89.99-float64(89.98), events)
	}
}

func TestReviewSeedAtOrPastTargetRejected(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		s := seed(side)
		if side == Long {
			s.EntryQuote.Bid, s.EntryQuote.Ask = 120, 120.1
		} else {
			s.EntryQuote.Bid, s.EntryQuote.Ask = 79.9, 80
		}
		m, err := NewLockTracker(s, policy(NoLock), nil, nil)
		if err == nil {
			t.Errorf("already-triggered target seed accepted: side=%v target=%v terminal=%v", side, m.CommonTarget(), m.Terminal())
		}
	}
}

func TestReviewCamarillaHandDerivedCompleteLadder(t *testing.T) {
	bars := syntheticBars()[:6]
	for i := range bars {
		bars[i].Open = 100
		bars[i].Close = 100
		bars[i].High = 106
		bars[i].Low = 94
	}
	w, err := FreezeWindow(RollingM30, bars[5].EndMillis, bars)
	if err != nil {
		t.Fatal(err)
	}
	l, err := Camarilla(w)
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{93.4, 96.7, 97.8, 98.9, 100, 101.1, 102.2, 103.3, 106.6}
	for i, p := range l.Levels {
		if math.Abs(p.Price-want[i]) > 1e-12 {
			t.Fatalf("level %d=%v want %v", i, p.Price, want[i])
		}
	}
}

func TestReviewExactOneGridInitialGeometry(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		s := seed(side)
		if side == Long {
			s.Entry = 100.03
			s.InitialStop = 100.01
			s.EntryQuote.Bid = 100.02
			s.EntryQuote.Ask = 100.03
		} else {
			s.Entry = 100
			s.InitialStop = 100.02
			s.EntryQuote.Bid = 100
			s.EntryQuote.Ask = 100.01
		}
		_, err := NewLockTracker(s, policy(NoLock), nil, nil)
		if err != nil {
			t.Errorf("side %v: exact one-grid protective stop rejected: %v", side, err)
		}
	}
}

func TestReviewExactOneRActivation(t *testing.T) {
	s := seed(Long)
	s.Entry = 100.01
	s.InitialStop = 100
	s.EntryQuote.Bid = 100.01
	s.EntryQuote.Ask = 100.01
	p := policy(PivotLock)
	p.Grid = .001
	m, err := NewLockTracker(s, p, nil, levels(s, 100.015))
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Observe(quote(s.EntryQuote.Event.AtMillis, 2, 100.02, 100.02), true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(got), []string{"activated", "submitted"}) {
		t.Fatalf("exact +1R quote did not activate: events=%+v", got)
	}
}

func TestReviewExactTwoRTarget(t *testing.T) {
	s := seed(Long)
	s.Entry = 100.01
	s.InitialStop = 100
	s.EntryQuote.Bid = 100.01
	s.EntryQuote.Ask = 100.01
	p := policy(NoLock)
	p.Grid = .001
	m, err := NewLockTracker(s, p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Observe(quote(s.EntryQuote.Event.AtMillis, 2, 100.03, 100.03), true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(got), []string{"exit-due"}) {
		t.Fatalf("exact 2R target 100.03 missed: internal target=%.17g events=%+v", m.CommonTarget(), got)
	}
}

func TestReviewExactShortTwoRTarget(t *testing.T) {
	s := seed(Short)
	s.Entry = 100.02
	s.InitialStop = 100.03
	s.EntryQuote.Bid = 100.02
	s.EntryQuote.Ask = 100.02
	p := policy(NoLock)
	p.Grid = .001
	m, err := NewLockTracker(s, p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Observe(quote(s.EntryQuote.Event.AtMillis, 2, 100, 100), true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kinds(got), []string{"exit-due"}) {
		t.Fatalf("exact short 2R target 100 missed: internal target=%.17g events=%+v", m.CommonTarget(), got)
	}
}

func TestReviewContextAfterActualEntryRejected(t *testing.T) {
	s := seed(Long)
	actualEntry := s.EntryQuote.Event.AtMillis
	s.EntryQuote.AvailableAtMillis += 1000
	l := levels(s, 106)
	l.KnownAtMillis = actualEntry + 500
	l.FrozenAtMillis = actualEntry + 500
	if _, err := NewLockTracker(s, policy(PivotLock), nil, l); err == nil {
		t.Error("pivot context learned and frozen 500ms after actual entry admitted due to delayed entry availability")
	}
	c := syntheticCalendar()
	o := opening(c, "2026-03-06", 3800)
	o.KnownAtMillis = actualEntry + 500
	f, err := FreezePriorOpening(c, actualEntry+500, []Opening{o})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewLockTracker(s, policy(ScaleLock), &f, nil); err == nil {
		t.Error("prior opening first known 500ms after actual entry admitted due to delayed entry availability")
	}
}

func TestReviewIndependentScaleOtherGrid(t *testing.T) {
	f := frozenOpening(t, 3857)
	for _, tc := range []struct {
		side Direction
		want float64
	}{{Long, 101.01}, {Short, 99}} {
		got, err := PriorOpenScaleLocation(f, 100.009, tc.side, .03)
		if err != nil || got.Stop != tc.want || got.CentDistance != 1.02 {
			t.Fatalf(".03-grid %v: %+v %v", tc.side, got, err)
		}
	}
}

func TestReviewClockM30Boundary(t *testing.T) {
	bars := syntheticBars()
	for _, tc := range []struct{ at, end string }{{"2026-02-05T00:29:59.999Z", "2026-02-05T00:00:00Z"}, {"2026-02-05T00:30:00Z", "2026-02-05T00:30:00Z"}} {
		w, err := FreezeWindow(ClockM30, ms(tc.at), bars)
		if err != nil || w.EndMillis != ms(tc.end) {
			t.Fatalf("boundary %s: %+v %v", tc.at, w, err)
		}
	}
}

func TestReviewPendingLatencyOldStopStillLive(t *testing.T) {
	s := seed(Long)
	p := policy(PivotLock)
	p.LatencyMillis = 100
	m, err := NewLockTracker(s, p, nil, levels(s, 106))
	if err != nil {
		t.Fatal(err)
	}
	at := s.EntryQuote.Event.AtMillis
	observe(t, m, quote(at, 2, 110, 110.1), "activated", "submitted")
	got := observe(t, m, quote(at+50, 1, 89, 89.1), "canceled", "exit-due")
	if got[1].Reason != "stop" || m.ActiveStop() != 90 || m.Counters().Accepted != 0 {
		t.Fatalf("old stop lost priority %+v", got)
	}
}
