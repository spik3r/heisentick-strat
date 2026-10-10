package seqcore_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	sc "github.com/spik3r/heisentick-strat/engine/seqcore"
)

func independentIdentity() sc.Identity {
	return sc.DefaultIdentity(sc.Source{Symbol: "SYN", Timeframe: "5m"}, "cfg-independent", 300000)
}
func independentSlide(n int) []sc.Bar {
	bars := make([]sc.Bar, n)
	for i := range bars {
		close := float64(104 - i)
		if i < 4 {
			close = 100
		}
		if i == 4 {
			close = 101
		}
		bars[i] = sc.Bar{OpenMS: 1699999800000 + int64(i)*300000, O: close, H: close + 1, L: close - 1, C: close}
	}
	return bars
}
func sideSnapshot(s sc.Snapshot, side string) sc.SideSnapshot {
	if side == "buy" {
		return s.Buy
	}
	return s.Sell
}
func episodeID(side string, open int64) string {
	return fmt.Sprintf("seq.full.public_approx.v1:SYN:5m:%s:%d", side, open)
}
func mirrorBars(bars []sc.Bar) []sc.Bar {
	out := make([]sc.Bar, len(bars))
	for i, b := range bars {
		out[i] = sc.Bar{OpenMS: b.OpenMS, O: 200 - b.O, H: 200 - b.L, L: 200 - b.H, C: 200 - b.C}
	}
	return out
}
func TestSetupRunContinuesBeyondTwentyTwo(t *testing.T) {
	for _, side := range []string{"buy", "sell"} {
		t.Run(side, func(t *testing.T) {
			bars := independentSlide(80)
			if side == "sell" {
				bars = mirrorBars(bars)
			}
			trace, err := sc.Run(sc.DefaultConfig(), independentIdentity(), bars)
			if err != nil {
				t.Fatal(err)
			}
			completions, recycles := 0, 0
			for _, ev := range trace.Events {
				if ev.Type == "setup_complete" {
					completions++
					if ev.BarIndex != 13 {
						t.Fatalf("extra Setup at bar %d", ev.BarIndex)
					}
				}
				if ev.Type == "countdown_recycle" {
					recycles++
					if ev.BarIndex != 26 {
						t.Fatalf("recycle at %d, want 26", ev.BarIndex)
					}
				}
				if ev.BarIndex > 26 {
					t.Fatalf("extra event after one-time recycle: %+v", ev)
				}
			}
			if completions != 1 || recycles != 1 {
				t.Fatalf("got %d Setups and %d recycles, want one each", completions, recycles)
			}
			completed := sideSnapshot(trace.Snapshots[13], side).LastCompletedSetup
			for i := 27; i < len(bars); i++ {
				snapshot := sideSnapshot(trace.Snapshots[i], side)
				if snapshot.SetupRun != int64(i-4) || !snapshot.SetupActive {
					t.Fatalf("bar %d: run=%d active=%v", i, snapshot.SetupRun, snapshot.SetupActive)
				}
				if snapshot.Countdown.State != "idle" || snapshot.Countdown.Count != 0 || snapshot.Countdown.SetupEpisodeID != nil || snapshot.Countdown.Bar5Index != nil || snapshot.Countdown.Bar8Index != nil || snapshot.Countdown.Bar13Index != nil {
					t.Fatalf("bar %d: recycle did not clear Countdown: %+v", i, snapshot.Countdown)
				}
				equalJSON(t, "recycle preserves completed Setup", snapshot.LastCompletedSetup, completed)
			}
		})
	}
}

func TestGapDropsDistinctPendingSetupBeforeOldCountdown(t *testing.T) {
	for _, side := range []string{"buy", "sell"} {
		t.Run(side, func(t *testing.T) {
			// At bar 24 the new, imperfect Setup is distinct from the still-active
			// Countdown admitted by bar 13. The next unexpected gap must report both.
			c := caseNamed(t, "second_imperfect_setup.main."+side)
			bars := append([]sc.Bar{}, c.Bars[:25]...)
			gap := c.Bars[25]
			gap.OpenMS = bars[24].OpenMS + 2*c.Series.TimeframeMS
			bars = append(bars, gap)
			engine := newEngine(t, fixtureIdentity(c))
			before, err := engine.Run(bars[:25])
			if err != nil {
				t.Fatal(err)
			}
			old := sideSnapshot(before.Snapshots[24], side)
			if old.LastCompletedSetup == nil || old.LastCompletedSetup.Perfected || old.LastCompletedSetup.EpisodeID != episodeID(side, bars[24].OpenMS) || old.Countdown.State != "active" || old.Countdown.SetupEpisodeID == nil || *old.Countdown.SetupEpisodeID != episodeID(side, bars[13].OpenMS) {
				t.Fatalf("independent scenario precondition failed: %+v", old)
			}
			saved, _ := checkpointJSON(t, engine)
			snapshot, events, err := engine.Step(gap)
			if err != nil {
				t.Fatal(err)
			}
			assertGapOnly(t, events, []string{episodeID(side, bars[24].OpenMS), episodeID(side, bars[13].OpenMS)})
			assertGapClearedSide(t, snapshot.Buy)
			assertGapClearedSide(t, snapshot.Sell)
			restored, err := sc.Restore(saved, fixtureIdentity(c), gap.OpenMS)
			if err != nil {
				t.Fatal(err)
			}
			resumed, resumedEvents, err := restored.Step(gap)
			if err != nil {
				t.Fatal(err)
			}
			equalJSON(t, "gap after actual checkpoint", resumed, snapshot)
			equalJSON(t, "ordered gap identities after checkpoint", resumedEvents, events)
		})
	}
}

func TestGapPendingBuyBeforeSellAndUniqueEpisodeIDs(t *testing.T) {
	for _, firstSide := range []string{"buy", "sell"} {
		t.Run("first_"+firstSide, func(t *testing.T) {
			// Buy Setup at 13 stays pending. The rise gives sell Setup at 23;
			// its bar-6 high of 150 makes that Setup imperfect too. Its Countdown
			// shares its pending Setup's id and therefore must appear only once.
			c := caseNamed(t, "delayed_perfection.after_opposite_setup.buy")
			bars := append([]sc.Bar{}, c.Bars[:24]...)
			bars[20].H = 150
			if firstSide == "sell" {
				bars = mirrorBars(bars)
			}
			engine := newEngine(t, independentIdentity())
			before, err := engine.Run(bars)
			if err != nil {
				t.Fatal(err)
			}
			last := before.Snapshots[len(before.Snapshots)-1]
			if last.Buy.LastCompletedSetup == nil || last.Buy.LastCompletedSetup.Perfected || last.Sell.LastCompletedSetup == nil || last.Sell.LastCompletedSetup.Perfected {
				t.Fatalf("both pending precondition failed: %+v", last)
			}
			buyIndex, sellIndex := 13, 23
			if firstSide == "sell" {
				buyIndex, sellIndex = 23, 13
			}
			expected := []string{episodeID("buy", bars[buyIndex].OpenMS), episodeID("sell", bars[sellIndex].OpenMS)}
			gap := bars[len(bars)-1]
			gap.OpenMS += 600000
			saved, _ := checkpointJSON(t, engine)
			restored, err := sc.Restore(saved, independentIdentity(), gap.OpenMS)
			if err != nil {
				t.Fatal(err)
			}
			s, events, err := restored.Step(gap)
			if err != nil {
				t.Fatal(err)
			}
			assertGapOnly(t, events, expected)
			assertGapClearedSide(t, s.Buy)
			assertGapClearedSide(t, s.Sell)
		})
	}
}
func assertGapOnly(t *testing.T, events []sc.Event, ids []string) {
	t.Helper()
	if len(events) != 1 || events[0].Type != "data_gap" || events[0].Phase != "P0" || events[0].Side != "both" || events[0].Reason != "data_gap" {
		t.Fatalf("gap emitted extra or wrong events: %+v", events)
	}
	equalJSON(t, "ordered unique dropped episodes", events[0].Detail, map[string]any{"dropped_episodes": ids})
}
func assertGapClearedSide(t *testing.T, s sc.SideSnapshot) {
	t.Helper()
	if s.SetupRun != 0 || s.SetupActive || s.LastCompletedSetup != nil {
		t.Fatalf("gap did not clear Setup: %+v", s)
	}
	equalJSON(t, "gap clears unfinished Countdown", s.Countdown, sc.Countdown{State: "idle"})
}

func TestCompletedCountdownReferencesSurviveGapAndWarmup(t *testing.T) {
	for _, side := range []string{"buy", "sell"} {
		t.Run(side, func(t *testing.T) {
			bars := independentSlide(26)
			if side == "sell" {
				bars = mirrorBars(bars)
			}
			e := newEngine(t, independentIdentity())
			before, err := e.Run(bars)
			if err != nil {
				t.Fatal(err)
			}
			completed := sideSnapshot(before.Snapshots[25], side).Countdown
			expectedID := episodeID(side, bars[13].OpenMS)
			b5, b8, b13 := int64(17), int64(20), int64(25)
			equalJSON(t, "completed references", completed, sc.Countdown{State: "completed", Count: 13, SetupEpisodeID: &expectedID, Bar5Index: &b5, Bar8Index: &b8, Bar13Index: &b13})
			gapBars := independentSlide(6)
			if side == "sell" {
				gapBars = mirrorBars(gapBars)
			}
			var newEvents []sc.Event
			for i := range gapBars {
				gapBars[i].OpenMS = bars[25].OpenMS + 600000 + int64(i)*300000
			}
			for i, bar := range gapBars {
				s, events, err := e.Step(bar)
				if err != nil {
					t.Fatal(err)
				}
				newEvents = append(newEvents, events...)
				equalJSON(t, "gap preserves completed references", sideSnapshot(*s, side).Countdown, completed)
				if s.Buy.LastCompletedSetup != nil || s.Sell.LastCompletedSetup != nil {
					t.Fatal("gap retained latest completed Setup")
				}
				if i < 5 && (s.Buy.SetupRun != 0 || s.Sell.SetupRun != 0) {
					t.Fatalf("comparison or flip used pre-gap data at segment %d", i)
				}
				if i == 5 && sideSnapshot(*s, side).SetupRun != 1 {
					t.Fatalf("earliest fresh flip did not start at segment index 5: %+v", s)
				}
			}
			assertGapOnly(t, newEvents, []string{})
		})
	}
}

func TestPriorSnapshotsAndEventsRemainUnchanged(t *testing.T) {
	for _, name := range []string{"second_imperfect_setup.main.buy", "countdown_overlap.fresh_after_completed.buy", "setup_22_recycle.completed.buy", "data_gap_reset.deferred_reset.buy"} {
		t.Run(name, func(t *testing.T) {
			c := caseNamed(t, name)
			e := newEngine(t, fixtureIdentity(c))
			type retained struct {
				snapshot                       *sc.Snapshot
				events                         []sc.Event
				encodedSnapshot, encodedEvents []byte
			}
			var all []retained
			for _, bar := range c.Bars {
				s, events, err := e.Step(bar)
				if err != nil {
					t.Fatal(err)
				}
				all = append(all, retained{s, events, jsonBytes(t, s), jsonBytes(t, events)})
				for i, r := range all {
					if string(jsonBytes(t, r.snapshot)) != string(r.encodedSnapshot) || string(jsonBytes(t, r.events)) != string(r.encodedEvents) {
						t.Fatalf("later transition mutated retained output from bar %d", i)
					}
				}
			}
		})
	}
}
func mutateIndex(p *int64) {
	if p != nil {
		*p = -999
	}
}
func mutateString(p *string) {
	if p != nil {
		*p = "caller changed this"
	}
}
func mutateSide(s *sc.SideSnapshot) {
	s.SetupRun = -999
	s.SetupActive = false
	c := &s.Countdown
	c.State = "caller changed this"
	c.Count = -999
	mutateString(c.SetupEpisodeID)
	mutateIndex(c.Bar5Index)
	mutateIndex(c.Bar8Index)
	mutateIndex(c.Bar13Index)
	if s.LastCompletedSetup != nil {
		v := s.LastCompletedSetup
		v.EpisodeID = "caller changed this"
		v.Bar6Extreme = -999
		v.Bar7Extreme = -999
		v.Perfected = !v.Perfected
		mutateIndex(v.PerfectedAtBarIndex)
		mutateIndex(v.PerfectedAtBarOpenMS)
		mutateIndex(v.PerfectedAtDecisionMS)
	}
}
func mutateEvent(e *sc.Event) {
	e.EventSeq = -999
	e.EpisodeID = "caller changed this"
	e.Type = "caller changed this"
	switch d := e.Detail.(type) {
	case sc.ReferenceDetail:
		mutateIndex(d.Bar5Index)
		mutateIndex(d.Bar8Index)
		mutateIndex(d.Bar13Index)
	case *sc.ReferenceDetail:
		mutateIndex(d.Bar5Index)
		mutateIndex(d.Bar8Index)
		mutateIndex(d.Bar13Index)
		d.Count = -999
	case sc.DroppedEpisodesDetail:
		for i := range d.DroppedEpisodes {
			d.DroppedEpisodes[i] = "caller changed this"
		}
	case *sc.DroppedEpisodesDetail:
		for i := range d.DroppedEpisodes {
			d.DroppedEpisodes[i] = "caller changed this"
		}
	case *sc.CountDetail:
		d.Count = -999
	case *sc.ReplacementDetail:
		d.ReplacedEpisodeID = "caller changed this"
	}
}
func TestCallerCannotMutateEngineThroughReturnedOutputs(t *testing.T) {
	for _, name := range []string{"countdown_overlap.fresh_after_completed.buy", "second_imperfect_setup.main.buy", "setup_22_recycle.completed.buy", "data_gap_reset.deferred_reset.buy", "opposite_setup_cancels_before_count.main.buy"} {
		t.Run(name, func(t *testing.T) {
			c := caseNamed(t, name)
			e := newEngine(t, fixtureIdentity(c))
			control := newEngine(t, fixtureIdentity(c))
			for i, bar := range c.Bars {
				s, events, err := e.Step(bar)
				if err != nil {
					t.Fatal(err)
				}
				want, wantEvents, err := control.Step(bar)
				if err != nil {
					t.Fatal(err)
				}
				equalJSON(t, fmt.Sprintf("bar %d detached snapshot", i), s, want)
				equalJSON(t, fmt.Sprintf("bar %d detached events", i), events, wantEvents)
				before, _ := checkpointJSON(t, e)
				mutateSide(&s.Buy)
				mutateSide(&s.Sell)
				s.BarIndex = -999
				for j := range events {
					mutateEvent(&events[j])
				}
				after, _ := checkpointJSON(t, e)
				if string(before) != string(after) {
					t.Fatalf("caller mutation changed engine state at bar %d", i)
				}
			}
		})
	}
}

func TestDisabledTerminalDeferralIsTypedUnsupported(t *testing.T) {
	// Disabling the required 13-vs-8 rule is outside the accepted default
	// profile too; this is additional to the corpus's 20 mirrored rejections.
	c := caseNamed(t, "config_unsupported.eight_vs_five_enabled.buy")
	c.Expected.Config = []byte(`{"deferral_13_vs_8":false}`)
	c.Expected.Error = map[string]any{"type": "UnsupportedConfigError", "field": "deferral_13_vs_8", "value": false, "reason": "unsupported_config"}
	testUnsupportedFixture(t, c)
}

func TestInvalidInputDoesNotConsumeBarOrPoisonEngine(t *testing.T) {
	tests := []struct {
		name   string
		change func(*sc.Bar)
	}{
		{"open_nan", func(b *sc.Bar) { b.O = math.NaN() }},
		{"high_nan", func(b *sc.Bar) { b.H = math.NaN() }},
		{"low_nan", func(b *sc.Bar) { b.L = math.NaN() }},
		{"close_nan", func(b *sc.Bar) { b.C = math.NaN() }},
		{"open_positive_infinity", func(b *sc.Bar) { b.O = math.Inf(1) }},
		{"high_positive_infinity", func(b *sc.Bar) { b.H = math.Inf(1) }},
		{"low_negative_infinity", func(b *sc.Bar) { b.L = math.Inf(-1) }},
		{"close_negative_infinity", func(b *sc.Bar) { b.C = math.Inf(-1) }},
		{"low_above_open", func(b *sc.Bar) { b.O = b.L - 1 }},
		{"low_above_close", func(b *sc.Bar) { b.C = b.L - 1 }},
		{"high_below_open", func(b *sc.Bar) { b.O = b.H + 1 }},
		{"high_below_close", func(b *sc.Bar) { b.C = b.H + 1 }},
		{"inverted_range", func(b *sc.Bar) { b.L = b.H + 1 }},
		{"decision_time_overflow", func(b *sc.Bar) { b.OpenMS = math.MaxInt64 }},
	}
	for _, prefix := range []int{0, 16} {
		for _, test := range tests {
			t.Run(fmt.Sprintf("after_%d/%s", prefix, test.name), func(t *testing.T) {
				bars := independentSlide(prefix + 1)
				id := independentIdentity()
				e := newEngine(t, id)
				control := newEngine(t, id)
				if _, err := e.Run(bars[:prefix]); err != nil {
					t.Fatal(err)
				}
				if _, err := control.Run(bars[:prefix]); err != nil {
					t.Fatal(err)
				}
				before, _ := checkpointJSON(t, e)
				bad := bars[prefix]
				test.change(&bad)
				snapshot, events, err := e.Step(bad)
				var typed *sc.InvalidInputError
				if !errors.As(err, &typed) || snapshot != nil || len(events) != 0 {
					t.Fatalf("invalid input produced state: snapshot=%+v events=%+v err=%T %v", snapshot, events, err, err)
				}
				after, _ := checkpointJSON(t, e)
				if string(before) != string(after) {
					t.Fatal("invalid input changed checkpoint")
				}
				// The same intended next time is still available, and diagnostics and
				// sequence have not been consumed by the invalid OHLC/overflow attempt.
				got, gotEvents, err := e.Step(bars[prefix])
				if err != nil {
					t.Fatalf("valid later input refused: %v", err)
				}
				want, wantEvents, err := control.Step(bars[prefix])
				if err != nil {
					t.Fatal(err)
				}
				equalJSON(t, "snapshot after invalid input", got, want)
				equalJSON(t, "events after invalid input", gotEvents, wantEvents)
			})
		}
	}
}

func TestReturnedBarOrderErrorDoesNotAliasTerminalState(t *testing.T) {
	bars := independentSlide(17)
	e := newEngine(t, independentIdentity())
	if _, err := e.Run(bars[:15]); err != nil {
		t.Fatal(err)
	}
	bad := bars[15]
	bad.OpenMS = bars[14].OpenMS
	snapshot, events, err := e.Step(bad)
	if snapshot != nil || len(events) != 0 {
		t.Fatal("bar-order failure emitted output")
	}
	assertBarOrderError(t, err, "duplicate_bar", 15, bars[14].OpenMS, bars[14].OpenMS)
	var typed *sc.BarOrderError
	if !errors.As(err, &typed) {
		t.Fatal(err)
	}
	typed.Kind = "caller mutation"
	typed.BarIndex = -999
	typed.PreviousOpenMS = -999
	typed.OpenMS = -999
	_, _, again := e.Step(bars[16])
	assertBarOrderError(t, again, "duplicate_bar", 15, bars[14].OpenMS, bars[14].OpenMS)
	if !errors.As(again, &typed) {
		t.Fatal(again)
	}
	typed.Kind = "second caller mutation"
	_, checkpointErr := e.Checkpoint()
	assertBarOrderError(t, checkpointErr, "duplicate_bar", 15, bars[14].OpenMS, bars[14].OpenMS)
}
