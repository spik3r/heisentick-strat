package engine

import (
	"strings"
	"testing"
)

func TestTimedReturnCannotUseInteractiveReservedIDs(t *testing.T) {
	for _, tc := range []struct{ id, reason string }{
		{InteractiveDraftStrategyID, "draft strategy requires"},
		{vpNYHandoffStrategyID, "VP NY handoff strategy requires"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			fixture, source, request := timedFixture(t, false)
			fixture.StrategyID = tc.id
			request.StrategyID = tc.id
			for _, run := range []func() (RunResult, error){func() (RunResult, error) { return Run(request) }, func() (RunResult, error) { return RunFixtureCase(fixture, source) }} {
				out, err := run()
				if err == nil || !strings.Contains(err.Error(), tc.reason) || len(out.Trades) != 0 || out.TimedAudit != nil {
					t.Fatalf("reserved identity bypass: %v %+v", err, out)
				}
			}
		})
	}
}

func TestInteractiveMigrationPathsRejectTimedCalendar(t *testing.T) {
	raw := []byte(`{"timedCalendar":{}}`)
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"ordinary", func() error { _, e := RunInteractiveFixture(raw, ""); return e }},
		{"draft", func() error { _, e := RunInteractiveDraftFixture(raw, ""); return e }},
		{"draft-prefix", func() error { _, e := RunInteractiveDraftPrefixFixture(raw, ""); return e }},
		{"composition", func() error { _, e := RunInteractiveCompositionFixture(raw, nil); return e }},
		{"VP veto", func() error { _, e := RunInteractiveVPNYHandoffVetoFixture(raw, ""); return e }},
		{"authored VP", func() error { _, e := RunAuthoredVPAsiaLondonWideInteractive(raw); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), "timedCalendar") {
				t.Fatalf("calendar must fail at admission: %v", err)
			}
		})
	}
}
