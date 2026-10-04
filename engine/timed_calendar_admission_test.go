package engine

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"strings"
	"testing"
)

const nonTimedSyntheticSource = `dsl v7
strategy "Synthetic generic admission"
market conditions {
 slices(SYNTHUSD 1m)
}
setup {
 type: sma golden cross
}
`

func TestTimedCalendarCannotBeIgnoredByOtherFamilies(t *testing.T) {
	for _, offRoute := range []bool{false, true} {
		for _, emptyCalendar := range []bool{false, true} {
			f, _, r := timedFixture(t, false)
			parsed, err := dsl.Parse(nonTimedSyntheticSource)
			if err != nil || len(parsed.Errors) > 0 {
				t.Fatalf("%v %v", err, parsed.Errors)
			}
			r.Config = parsed.Config
			if offRoute {
				r.Symbol = "OTHERUSD"
				f.Symbol = r.Symbol
			}
			if emptyCalendar {
				r.TimedCalendar = &TimedReturnCalendar{}
				f.TimedCalendar = r.TimedCalendar
			}
			checks := []struct {
				name string
				run  func() error
			}{
				{"fixture", func() error {
					got, e := RunFixtureCase(f, nonTimedSyntheticSource)
					if e != nil && (len(got.Trades) != 0 || got.TimedAudit != nil) {
						t.Fatal("partial fixture output")
					}
					return e
				}},
				{"run", func() error {
					got, e := Run(r)
					if e != nil && (len(got.Trades) != 0 || got.TimedAudit != nil) {
						t.Fatal("partial native output")
					}
					return e
				}},
				{"prepared", func() error { _, e := PrepareRun(r); return e }},
				{"shared-key", func() error { _, e := SharedContextKey(r); return e }},
				{"shared-context", func() error { _, e := PrepareSharedRunContext(r); return e }},
				{"prefix", func() error { _, e := RunPrefix(r); return e }},
				{"resumable-prefix", func() error { _, _, e := RunPrefixResumable(r, nil); return e }},
			}
			for _, check := range checks {
				if e := check.run(); e == nil || !strings.Contains(e.Error(), "timed calendar requires the timed-return family") {
					t.Fatalf("%s offRoute=%v empty=%v: %v", check.name, offRoute, emptyCalendar, e)
				}
			}
			// No calendar means the existing non-timed zero-trade behavior is preserved.
			r.TimedCalendar = nil
			f.TimedCalendar = nil
			got, e := Run(r)
			if e != nil || got.TradeCount != 0 || got.TimedAudit != nil {
				t.Fatalf("generic baseline: %v %+v", e, got)
			}
			got, e = RunFixtureCase(f, nonTimedSyntheticSource)
			if e != nil || got.TradeCount != 0 || got.TimedAudit != nil {
				t.Fatalf("fixture baseline: %v %+v", e, got)
			}
		}
	}
}
