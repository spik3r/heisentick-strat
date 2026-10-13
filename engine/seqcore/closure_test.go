package seqcore

import (
	"math/rand"
	"reflect"
	"testing"
)

// StepAfterClosure must make a declared hole invisible to the state machine:
// the same closes fed with holes between them produce the same events and the
// same index-based state as the same closes fed contiguously. Times differ, so
// only index and price fields are compared. Plain Step across the same holes
// must still emit data_gap and reset, which is the unchanged default.

func closureWalk(seed int64, n int) []Bar {
	rng := rand.New(rand.NewSource(seed))
	price := 100.0
	bars := make([]Bar, n)
	for i := range bars {
		open := price
		closePrice := open + (rng.Float64()-0.5)*4
		bars[i] = Bar{O: open, H: max(open, closePrice) + rng.Float64(), L: min(open, closePrice) - rng.Float64(), C: closePrice}
		price = closePrice
	}
	return bars
}

type closureView struct {
	Events []closureEvent
	States []closureState
}
type closureEvent struct {
	Phase, Type, Side, Reason string
	BarIndex                  int64
	Detail                    string
}
type closureState struct {
	Buy, Sell closureSide
}
type closureSide struct {
	Run            int64
	Active         bool
	CountdownState string
	Count          int
	B5, B8, B13    *int64
	Completed      *closureSetup
}
type closureSetup struct {
	Index          int64
	B6, B7         float64
	Perfected      bool
	PerfectedAtIdx *int64
}

func closureSideOf(s SideSnapshot) closureSide {
	out := closureSide{Run: s.SetupRun, Active: s.SetupActive, CountdownState: s.Countdown.State, Count: s.Countdown.Count, B5: s.Countdown.Bar5Index, B8: s.Countdown.Bar8Index, B13: s.Countdown.Bar13Index}
	if c := s.LastCompletedSetup; c != nil {
		out.Completed = &closureSetup{c.CompletionBarIndex, c.Bar6Extreme, c.Bar7Extreme, c.Perfected, c.PerfectedAtBarIndex}
	}
	return out
}

func closureRun(t *testing.T, bars []Bar, holeBefore map[int]int64, declared bool) closureView {
	t.Helper()
	const tf = int64(60000)
	e, err := New(DefaultConfig(), DefaultIdentity(Source{Symbol: "X", Timeframe: "1m"}, "closure-test", tf))
	if err != nil {
		t.Fatal(err)
	}
	var view closureView
	open := int64(1_700_000_000_000)
	for i, b := range bars {
		if i > 0 {
			open += tf
		}
		hole := holeBefore[i]
		open += hole * tf
		b.OpenMS = open
		var (
			snap   *Snapshot
			events []Event
		)
		if hole > 0 && declared {
			snap, events, err = e.StepAfterClosure(b)
		} else {
			snap, events, err = e.Step(b)
		}
		if err != nil {
			t.Fatalf("bar %d: %v", i, err)
		}
		for _, ev := range events {
			detail := ""
			switch d := ev.Detail.(type) {
			case CountDetail:
				detail = "count"
			case ReferenceDetail:
				detail = "reference"
			case ReplacementDetail:
				detail = "replacement"
			case DroppedEpisodesDetail:
				detail = "dropped"
				_ = d
			}
			view.Events = append(view.Events, closureEvent{ev.Phase, ev.Type, ev.Side, ev.Reason, ev.BarIndex, detail})
		}
		view.States = append(view.States, closureState{closureSideOf(snap.Buy), closureSideOf(snap.Sell)})
	}
	return view
}

func TestStepAfterClosureEqualsContiguousObservedBars(t *testing.T) {
	setups := 0
	for seed := int64(1); seed <= 12; seed++ {
		bars := closureWalk(seed, 500)
		rng := rand.New(rand.NewSource(seed * 7919))
		holes := map[int]int64{}
		for i := 1; i < len(bars); i++ {
			if rng.Intn(23) == 0 {
				holes[i] = 1 + int64(rng.Intn(40))
			}
		}
		contiguous := closureRun(t, bars, nil, false)
		declared := closureRun(t, bars, holes, true)
		if !reflect.DeepEqual(contiguous, declared) {
			t.Fatalf("seed %d: declared closures changed events or state", seed)
		}
		for _, ev := range contiguous.Events {
			if ev.Type == "setup_complete" {
				setups++
			}
			if ev.Type == "data_gap" {
				t.Fatalf("seed %d: contiguous run emitted data_gap", seed)
			}
		}
		undeclared := closureRun(t, bars, holes, false)
		gaps := 0
		for _, ev := range undeclared.Events {
			if ev.Type == "data_gap" {
				gaps++
			}
		}
		if gaps == 0 {
			t.Fatalf("seed %d: plain Step across holes no longer emits data_gap", seed)
		}
		if reflect.DeepEqual(contiguous, undeclared) {
			t.Fatalf("seed %d: the undeclared run is indistinguishable, so this test proves nothing", seed)
		}
	}
	if setups < 10 {
		t.Fatalf("only %d completed setups across seeds; the walks do not exercise the machine", setups)
	}
}

func TestStepAfterClosureRefusesWithoutAHole(t *testing.T) {
	e, err := New(DefaultConfig(), DefaultIdentity(Source{Symbol: "X", Timeframe: "1m"}, "closure-test", 60000))
	if err != nil {
		t.Fatal(err)
	}
	bar := Bar{OpenMS: 1_700_000_000_000, O: 1, H: 2, L: 0.5, C: 1.5}
	if _, _, err := e.StepAfterClosure(bar); err == nil {
		t.Fatal("first bar cannot follow a closure")
	}
	if _, _, err := e.Step(bar); err != nil {
		t.Fatalf("a refused call must not stop the engine: %v", err)
	}
	next := bar
	next.OpenMS += 60000
	if _, _, err := e.StepAfterClosure(next); err == nil {
		t.Fatal("adjacent bar is not a closure")
	}
	next.OpenMS += 60000
	if _, _, err := e.StepAfterClosure(next); err != nil {
		t.Fatalf("two-step spacing is a hole: %v", err)
	}
}
