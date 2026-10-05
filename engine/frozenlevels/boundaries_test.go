package frozenlevels

import (
	"math"
	"reflect"
	"testing"
)

func narrowSeed(side Direction) PositionSeed {
	s := seed(side)
	if side == Long {
		s.Entry = 100.01
		s.InitialStop = 100
	} else {
		s.Entry = 100.02
		s.InitialStop = 100.03
	}
	s.EntryQuote.Bid = s.Entry
	s.EntryQuote.Ask = s.Entry
	return s
}
func liquidQuote(s PositionSeed, seq int64, value float64) Quote {
	return quote(s.EntryQuote.Event.AtMillis, seq, value, value)
}

// An adjacent representable float is a distinct, shortest-decimal input price.
// No epsilon is allowed to consume it as equality at a threshold.
func adjacent(value float64, side Direction, favorable bool) float64 {
	toward := math.Inf(int(side))
	if !favorable {
		toward = math.Inf(-int(side))
	}
	return math.Nextafter(value, toward)
}
func TestExactDecimalInitialGridBoundaries(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		for _, offset := range []int{-1, 0, 1} {
			s := seed(side)
			value := 100.02
			if side == Long {
				s.Entry = 100.03
				s.InitialStop = 100.01
			} else {
				s.Entry = 100
				s.InitialStop = 100.02
				value = 100.01
			}
			if offset != 0 {
				value = adjacent(value, side, offset > 0)
			}
			s.EntryQuote.Bid = value
			s.EntryQuote.Ask = value
			_, err := NewLockTracker(s, policy(NoLock), nil, nil)
			if (err == nil) != (offset >= 0) {
				t.Fatalf("side %d offset %d initial grid admitted=%v", side, offset, err == nil)
			}
		}
	}
}
func TestExactDecimalPivotGridBoundaries(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		for _, offset := range []int{-1, 0, 1} {
			s := seed(side)
			raw, value := 110.01, 110.02
			if side == Short {
				raw, value = 89.99, 89.98
			}
			if offset != 0 {
				value = adjacent(value, side, offset > 0)
			}
			m, err := NewLockTracker(s, policy(PivotLock), nil, levels(s, raw))
			if err != nil {
				t.Fatal(err)
			}
			got, err := m.Observe(liquidQuote(s, 2, value), true)
			if err != nil {
				t.Fatal(err)
			}
			last := "submitted"
			if offset < 0 {
				last = "rejected"
			}
			if !reflect.DeepEqual(kinds(got), []string{"activated", last}) {
				t.Fatalf("side %d offset %d grid events %+v", side, offset, got)
			}
		}
	}
}
func TestExactDecimalActivationBoundaries(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		for _, offset := range []int{-1, 0, 1} {
			s := narrowSeed(side)
			p := policy(PivotLock)
			p.Grid = .001
			activation := 100.02
			if side == Short {
				activation = 100.01
			}
			if offset != 0 {
				activation = adjacent(activation, side, offset > 0)
			}
			m, err := NewLockTracker(s, p, nil, levels(s, 100.015))
			if err != nil {
				t.Fatal(err)
			}
			got, err := m.Observe(liquidQuote(s, 2, activation), true)
			if err != nil {
				t.Fatal(err)
			}
			if offset < 0 {
				if len(got) != 0 {
					t.Fatalf("side %d premature activation %+v", side, got)
				}
			} else if !reflect.DeepEqual(kinds(got), []string{"activated", "submitted"}) {
				t.Fatalf("side %d offset %d missing activation %+v", side, offset, got)
			}
		}
	}
}
func TestExactDecimalTargetBoundaries(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		for _, offset := range []int{-1, 0, 1} {
			s := narrowSeed(side)
			p := policy(NoLock)
			p.Grid = .001
			target := 100.03
			if side == Short {
				target = 100
			}
			value := target
			if offset != 0 {
				value = adjacent(value, side, offset > 0)
			}
			m, err := NewLockTracker(s, p, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if m.CommonTarget() != target {
				t.Fatalf("side %d projected target %.17g want %.17g", side, m.CommonTarget(), target)
			}
			got, err := m.Observe(liquidQuote(s, 2, value), true)
			if err != nil {
				t.Fatal(err)
			}
			if offset < 0 {
				if len(got) != 0 {
					t.Fatalf("side %d premature target %+v", side, got)
				}
			} else if len(got) != 1 || got[0].Kind != "exit-due" || got[0].Reason != "target" {
				t.Fatalf("side %d offset %d target missed %+v", side, offset, got)
			}
		}
	}
}
func TestExactDecimalAcceptanceBoundaries(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		for _, offset := range []int{-1, 0, 1} {
			s := seed(side)
			pivot, activation, boundary := 106.0, 110.0, 106.01
			if side == Short {
				pivot, activation, boundary = 94, 90, 93.99
			}
			if offset != 0 {
				boundary = adjacent(boundary, side, offset > 0)
			}
			m, err := NewLockTracker(s, policy(PivotLock), nil, levels(s, pivot))
			if err != nil {
				t.Fatal(err)
			}
			observe(t, m, liquidQuote(s, 2, activation), "activated", "submitted")
			want := "accepted"
			if offset < 0 {
				want = "rejected"
			}
			observe(t, m, liquidQuote(s, 3, boundary), want)
			if offset >= 0 && m.ActiveStop() != pivot {
				t.Fatal("wrong accepted stop")
			}
		}
	}
}
func TestActualEntryCausalCutoffBothModesAndDirections(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		for _, mode := range []LockMode{PivotLock, ScaleLock} {
			for _, offset := range []int64{-1, 0, 1, 500} {
				s := seed(side)
				s.EntryQuote.AvailableAtMillis += 1000
				cutoff := s.EntryQuote.Event.AtMillis + offset
				var ladder *LockLevels
				var f *FrozenOpening
				if mode == PivotLock {
					ladder = levels(s, 106)
					ladder.KnownAtMillis = cutoff
					ladder.FrozenAtMillis = cutoff
				} else {
					c := syntheticCalendar()
					o := opening(c, "2026-03-06", 3800)
					o.KnownAtMillis = cutoff
					frozen, err := FreezePriorOpening(c, cutoff, []Opening{o})
					if err != nil {
						t.Fatal(err)
					}
					f = &frozen
				}
				_, err := NewLockTracker(s, policy(mode), f, ladder)
				if (err == nil) != (offset < 0) {
					t.Fatalf("side %d mode %s cutoff %+d admitted=%v", side, mode, offset, err == nil)
				}
			}
		}
	}
}
func TestSeedAtAndBeyondTargetFailsClosed(t *testing.T) {
	for _, side := range []Direction{Long, Short} {
		for _, offset := range []int{-1, 0, 1} {
			s := seed(side)
			value := 120.0
			if side == Short {
				value = 80
			}
			if offset != 0 {
				value = adjacent(value, side, offset > 0)
			}
			s.EntryQuote.Bid = value
			s.EntryQuote.Ask = value
			_, err := NewLockTracker(s, policy(NoLock), nil, nil)
			if (err == nil) != (offset < 0) {
				t.Fatalf("side %d offset %d target seed admitted=%v", side, offset, err == nil)
			}
		}
	}
}
func TestOffGridInputsAndExactInternalGrid(t *testing.T) {
	s := seed(Long)
	s.Entry = 100.009
	s.InitialStop = 90.008
	s.EntryQuote.Bid = 100.009
	s.EntryQuote.Ask = 100.009
	p := policy(PivotLock)
	p.ActivationR = 1
	p.Grid = .003
	m, err := NewLockTracker(s, p, nil, levels(s, 106.007))
	if err != nil {
		t.Fatal(err)
	}
	if m.CommonTarget() != 120.011 {
		t.Fatalf("off-grid risk was rounded: %.17g", m.CommonTarget())
	}
	observe(t, m, liquidQuote(s, 2, 110.01), "activated", "submitted")
	observe(t, m, liquidQuote(s, 3, 106.008), "accepted")
	if m.ActiveStop() != 106.005 {
		t.Fatalf("non-power-ten grid snap %g", m.ActiveStop())
	}
}
