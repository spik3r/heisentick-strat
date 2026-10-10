package seqcore_test

import (
	"fmt"
	"testing"

	sc "github.com/spik3r/heisentick-strat/engine/seqcore"
)

type fixtureAge struct {
	Age      int64 `json:"age"`
	Eligible bool  `json:"eligible"`
}
type fixtureAgeTable struct {
	Policy    sc.ResponseWindowPolicy `json:"policy"`
	Setup     []fixtureAge            `json:"setup"`
	Countdown []fixtureAge            `json:"countdown"`
}
type fixtureWindow struct {
	Anchor struct {
		Type     string `json:"type"`
		Side     string `json:"side"`
		BarIndex int64  `json:"bar_index"`
	} `json:"anchor"`
	DecisionType      *string `json:"decision_type"`
	DecisionBarIndex  int64   `json:"decision_bar_index"`
	Age               int64   `json:"age"`
	Eligible          bool    `json:"eligible"`
	FillBarIndex      int64   `json:"fill_bar_index"`
	FillAge           int64   `json:"fill_age"`
	FillOutsideWindow bool    `json:"fill_outside_window"`
}

func testAgeTable(t *testing.T, table fixtureAgeTable) {
	t.Helper()
	if table.Policy != sc.DefaultResponseWindowPolicy() {
		t.Fatalf("default policy mismatch: %+v", table.Policy)
	}
	for kind, rows := range map[string][]fixtureAge{"setup": table.Setup, "countdown": table.Countdown} {
		for _, row := range rows {
			if got := table.Policy.Eligible(kind, row.Age); got != row.Eligible {
				t.Fatalf("Eligible(%s,%d)=%v want %v", kind, row.Age, got, row.Eligible)
			}
		}
	}
}
func testWindowProbes(t *testing.T, c fixtureCase, trace sc.Trace) {
	t.Helper()
	policy := sc.DefaultResponseWindowPolicy()
	hasEvent := func(kind, side string, index int64) bool {
		for _, e := range trace.Events {
			if e.Type == kind && e.Side == side && e.BarIndex == index {
				return true
			}
		}
		return false
	}
	for i, probe := range c.Expected.Windows {
		label := fmt.Sprintf("window probe %d", i)
		if !hasEvent(probe.Anchor.Type, probe.Anchor.Side, probe.Anchor.BarIndex) {
			t.Fatalf("%s: missing anchor event", label)
		}
		if probe.DecisionType != nil && !hasEvent(*probe.DecisionType, probe.Anchor.Side, probe.DecisionBarIndex) {
			t.Fatalf("%s: missing decision event", label)
		}
		age := probe.DecisionBarIndex - probe.Anchor.BarIndex
		if age != probe.Age || policy.Eligible(probe.Anchor.Type, age) != probe.Eligible {
			t.Fatalf("%s: age/eligibility differs from fixture", label)
		}
		// These are metadata consistency checks only. The core has no fills or
		// execution rules; a window never changes the decision + 1 fill convention.
		if probe.FillBarIndex != probe.DecisionBarIndex+1 || probe.FillAge != probe.FillBarIndex-probe.Anchor.BarIndex || probe.FillOutsideWindow == policy.Eligible(probe.Anchor.Type, probe.FillAge) {
			t.Fatalf("%s: inconsistent decision/fill metadata", label)
		}
	}
}
func TestResponseWindowBoundariesAndNegativeAges(t *testing.T) {
	policy := sc.DefaultResponseWindowPolicy()
	for _, kind := range []string{"setup", "setup_complete", "countdown", "countdown_complete"} {
		limit := int64(4)
		if kind == "countdown" || kind == "countdown_complete" {
			limit = 12
		}
		for _, age := range []int64{-1 << 63, -100, -1, 0, 1, limit - 1, limit, limit + 1, 1<<63 - 1} {
			want := age >= 0 && age <= limit
			if got := policy.Eligible(kind, age); got != want {
				t.Errorf("Eligible(%q,%d)=%v, want %v", kind, age, got, want)
			}
		}
	}
	for _, kind := range []string{"", "perfection", "delayed_perfection", "active_countdown", "other"} {
		if policy.Eligible(kind, 0) {
			t.Errorf("unknown/unanchored kind %q eligible", kind)
		}
	}
	custom := sc.ResponseWindowPolicy{SetupBars: 0, CountdownBars: 2}
	if !custom.Eligible("setup", 0) || custom.Eligible("setup", 1) || !custom.Eligible("countdown", 2) || custom.Eligible("countdown", 3) {
		t.Fatal("helper ignored caller's typed window policy")
	}
}
func TestWindowQueriesLeaveLifecycleAndCheckpointUntouched(t *testing.T) {
	for _, id := range []string{"window_policy.setup_age10.buy", "window_policy.active_countdown_past_window.buy", "window_policy.countdown_age.buy"} {
		t.Run(id, func(t *testing.T) {
			c := caseNamed(t, id)
			e := newEngine(t, fixtureIdentity(c))
			var got sc.Trace
			for _, bar := range c.Bars {
				s, ev, err := e.Step(bar)
				if err != nil {
					t.Fatal(err)
				}
				got.Events = append(got.Events, ev...)
				got.Snapshots = append(got.Snapshots, *s)
				before, _ := checkpointJSON(t, e)
				for _, kind := range []string{"setup", "countdown"} {
					for _, age := range []int64{-1, 0, 4, 5, 12, 13, 1000} {
						_ = sc.DefaultResponseWindowPolicy().Eligible(kind, age)
					}
				}
				after, _ := checkpointJSON(t, e)
				if string(before) != string(after) {
					t.Fatal("read-only window helper changed lifecycle checkpoint")
				}
			}
			baseline, err := sc.Run(sc.DefaultConfig(), fixtureIdentity(c), c.Bars)
			if err != nil {
				t.Fatal(err)
			}
			compareTrace(t, "window queries", got, baseline)
		})
	}
}
