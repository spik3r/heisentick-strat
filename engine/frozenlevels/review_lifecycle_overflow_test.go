package frozenlevels_test

import (
	fl "github.com/spik3r/heisentick-strat/engine/frozenlevels"
	"testing"
)

// The documented expiry contract says that a crossing is logged and the
// level expires if its submission cannot precede expiry. No addition required.
func TestIndependentOverflowCrossingAuditContract(t *testing.T) {
	s := reviewSpec(fl.Long)
	s.Policy.SubmissionLatencyMillis = reviewMaxTime
	m := reviewCreate(t, s, nil)
	reviewStep(t, m, reviewQuote(101, 1, 101, 100), false, "reset/inside-observation")
	events, err := m.Observe(reviewQuote(102, 1, 102, 101), true, false)
	t.Logf("state=%s events=%+v error=%v", m.State(), events, err)
	if len(events) != 2 || events[0].Kind != "crossed" || events[1].Kind != "expired" || m.State() != "expired" {
		t.Fatal("documented crossing-plus-expiry audit sequence lost when explicit latency exceeds representable timestamp")
	}
}
