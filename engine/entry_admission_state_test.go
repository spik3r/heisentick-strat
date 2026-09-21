package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestRejectedAdmissionRecordsFamilyCooldownAttempt(t *testing.T) {
	tests := []struct {
		name      string
		caseName  string
		lastState func(*broker) (bool, int)
		wantLast  int
	}{
		{
			name:     "range break fake",
			caseName: "family-range-break-fake",
			lastState: func(b *broker) (bool, int) {
				return b.hasRBFEntry, b.rbfLastEntry
			},
			wantLast: 2577,
		},
		{
			name:     "opening range breakout",
			caseName: "family-opening-range-breakout",
			lastState: func(b *broker) (bool, int) {
				return b.hasORBEntry, b.orbLastEntry
			},
			wantLast: 1753,
		},
		{
			name:     "inside day expansion",
			caseName: "family-inside-day-expansion",
			lastState: func(b *broker) (bool, int) {
				return b.hasIDEEntry, b.ideLastEntry
			},
			wantLast: 750,
		},
		{
			name:     "day open reclaim",
			caseName: "family-day-open-reclaim",
			lastState: func(b *broker) (bool, int) {
				return b.hasDOREntry, b.dorLastEntry
			},
			wantLast: 2536,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture, cfg := loadEntryAttemptCase(t, tt.caseName)
			// ER is non-negative, so this makes every otherwise-valid setup fail
			// the final admission gate without suppressing family detection.
			cfg["maxMovementEr"] = -1.0
			runner := newPreparedRunner(fixture, cfg)
			if trades := runner.RunPrepared(); len(trades) != 0 {
				t.Fatalf("guard-rejected run produced %d trades, want 0", len(trades))
			}
			hasAttempt, last := tt.lastState(&runner.broker)
			if !hasAttempt {
				t.Fatal("guard-rejected setup did not record its cooldown attempt")
			}
			if last != tt.wantLast {
				t.Fatalf("last cooldown attempt = %d, want %d", last, tt.wantLast)
			}
		})
	}
}

func TestFlagRejectedMarketGateRecordsCooldownAttempt(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "deployed-dsl-flag-continuation-one-four-hour-review")
	baseline := newPreparedRunner(fixture, cfg)
	if trades := baseline.RunPrepared(); len(trades) != 18 {
		t.Fatalf("baseline flag trades = %d, want 18", len(trades))
	}

	// Flag setup detection requires ER >= 0.5 in this fixture. A negative
	// global maximum therefore rejects every otherwise-valid candidate only at
	// the final market gate, after the family has consumed its cooldown attempt.
	cfg["maxMovementEr"] = -1.0
	runner := newPreparedRunner(fixture, cfg)
	if trades := runner.RunPrepared(); len(trades) != 0 {
		t.Fatalf("guard-rejected flag run produced %d trades, want 0", len(trades))
	}
	if runner.broker.hasPosition || len(runner.broker.pendingOrders) != 0 || len(runner.broker.limitOrders) != 0 {
		t.Fatalf("guard-rejected flag left live admission state: position=%v pending=%d limits=%d",
			runner.broker.hasPosition, len(runner.broker.pendingOrders), len(runner.broker.limitOrders))
	}
	if !runner.broker.hasFlagEntry {
		t.Fatal("guard-rejected flag setup did not record its cooldown attempt")
	}
	if runner.broker.flagLastEntry != 2884 {
		t.Fatalf("last flag cooldown attempt = %d, want 2884", runner.broker.flagLastEntry)
	}
}

func TestDisabledMidWindowDiscoversThenRejectsFamilySetup(t *testing.T) {
	tests := []struct {
		name      string
		caseName  string
		shiftHour int
		lastState func(*broker) (bool, int)
		wantLast  int
	}{
		{
			name:     "range break fake",
			caseName: "family-range-break-fake",
			lastState: func(b *broker) (bool, int) {
				return b.hasRBFEntry, b.rbfLastEntry
			},
			wantLast: 2577,
		},
		{
			name:      "inside day expansion",
			caseName:  "family-inside-day-expansion",
			shiftHour: -12,
			lastState: func(b *broker) (bool, int) {
				return b.hasIDEEntry, b.ideLastEntry
			},
			wantLast: 1156,
		},
		{
			name:      "day open reclaim",
			caseName:  "family-day-open-reclaim",
			shiftHour: -10,
			lastState: func(b *broker) (bool, int) {
				return b.hasDOREntry, b.dorLastEntry
			},
			wantLast: 2303,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture, cfg := loadEntryAttemptCase(t, tt.caseName)
			fixture = shiftEntryAttemptFixture(fixture, tt.shiftHour)
			runner := newPreparedRunner(fixture, cfg)
			runner.params.UseAsiaWindow = false
			runner.params.UseMidWindow = false
			runner.params.UseLondonWindow = false
			runner.params.UseNYWindow = false

			if trades := runner.RunPrepared(); len(trades) != 0 {
				t.Fatalf("disabled-session run produced %d trades, want 0", len(trades))
			}
			hasAttempt, last := tt.lastState(&runner.broker)
			if !hasAttempt {
				t.Fatal("MID setup was not discovered before final admission rejection")
			}
			if last != tt.wantLast {
				t.Fatalf("last MID cooldown attempt = %d, want %d", last, tt.wantLast)
			}
			at := runner.series.T[last]
			if !inSetupTradeWindow(at, runner.params, 0) {
				t.Fatal("setup-local window rejected the recorded MID attempt")
			}
			if inFlagTradeWindow(at, runner.params, 0) {
				t.Fatal("final admission window accepted disabled MID")
			}
		})
	}
}

func shiftEntryAttemptFixture(fixture RunFixture, hours int) RunFixture {
	delta := float64(hours * 60 * 60 * 1000)
	fixture.Bars = append([]marketdata.Bar(nil), fixture.Bars...)
	for i := range fixture.Bars {
		fixture.Bars[i].T += delta
	}
	fixture.HTFBars = append([]marketdata.Bar(nil), fixture.HTFBars...)
	for i := range fixture.HTFBars {
		fixture.HTFBars[i].T += delta
	}
	return fixture
}

func loadEntryAttemptCase(t *testing.T, caseName string) (RunFixture, dsl.Config) {
	t.Helper()
	runDir := runFixtureDir()
	fixture, err := LoadRunFixture(filepath.Join(runDir, caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	fixture = flatHTFForUnrelatedTest(fixture)
	source, err := os.ReadFile(filepath.Join(runDir, caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) > 0 {
		t.Fatalf("parse DSL diagnostics: %v", parsed.Errors)
	}
	return fixture, parsed.Config
}
