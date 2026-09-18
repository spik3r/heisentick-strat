package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
)

func TestRunFixtureCaseHonorsAllowedSessionPhase(t *testing.T) {
	const caseName = "family-break-retest"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	withOpenPhase := strings.Replace(string(source),
		"sessions(london, ny)",
		"sessions(london, ny)\n  session phase in (open)",
		1,
	)
	if withOpenPhase == string(source) {
		t.Fatal("fixture source did not contain the expected sessions directive")
	}
	baseline, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	if baseline.TradeCount != 13 {
		t.Fatalf("baseline trade count = %d, want reviewed fixture count 13", baseline.TradeCount)
	}

	result, err := RunFixtureCase(fixture, withOpenPhase)
	if err != nil {
		t.Fatalf("run fixture with allowed phase: %v", err)
	}
	if result.TradeCount != 0 || len(result.Trades) != 0 {
		t.Fatalf("open-only phase produced %d trades: %+v; want no middle/close entries", result.TradeCount, result.Trades)
	}
}

func TestMarketNonSessionGateHonorsAllowedPhase(t *testing.T) {
	b := broker{
		params: flagParams{SessionPhases: []string{"open"}},
		cols: contextcols.Columns{
			SessionPhase: []int8{1, 2},
			PriorDayType: []int8{0, 0},
			Regime:       []int8{0, 0},
			ER:           []float64{0, 0},
		},
	}

	if !b.marketNonSessionGatesOK(0) {
		t.Fatal("open phase should pass the open-only allowlist")
	}
	if b.marketNonSessionGatesOK(1) {
		t.Fatal("middle phase should fail the open-only allowlist")
	}
	b.cols.SessionPhase = nil
	if b.marketNonSessionGatesOK(0) {
		t.Fatal("missing phase context should fail a nonempty allowlist without panicking")
	}
}

func TestDisallowedPhasePreservesTypedCandidateConsumptionOrder(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "family-opening-range-breakout")
	runner := newPreparedRunner(fixture, cfg)
	runner.params.SessionPhases = []string{"open"}
	runner.broker.reset(
		runner.series,
		runner.cols,
		runner.htfTrend,
		runner.ema,
		runner.emaSlope,
		runner.params,
		runner.fixture,
		nil,
	)

	const firstCandidate = 35
	if got := sessionPhaseName(runner.broker.cols.SessionPhase[firstCandidate]); got != "close" {
		t.Fatalf("reviewed candidate phase = %q, want close", got)
	}
	runner.broker.onBar(firstCandidate)
	if !runner.broker.hasORBEntry || runner.broker.hasPosition || len(runner.broker.seen.orb.keys) != 1 {
		t.Fatalf("typed phase rejection lost JS candidate consumption: cooldown=%v position=%v seen=%v",
			runner.broker.hasORBEntry, runner.broker.hasPosition, runner.broker.seen.orb.keys)
	}

	// Typed JS handlers consume their seen/cooldown state around guarded enter.
	// Re-presenting the candidate after the phase becomes allowed must therefore
	// remain suppressed, matching the established C6 final-gate contract.
	runner.broker.cols.SessionPhase[firstCandidate] = 1
	runner.broker.onBar(firstCandidate)
	if runner.broker.hasPosition || len(runner.broker.seen.orb.keys) != 1 {
		t.Fatalf("consumed typed candidate re-entered: position=%v seen=%v",
			runner.broker.hasPosition, runner.broker.seen.orb.keys)
	}
}

func TestDisallowedPhaseDoesNotConsumeGenericFailedBreakoutCandidate(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "money-risk-sizing")
	runner := newPreparedRunner(fixture, cfg)
	runner.params.SessionPhases = []string{"open"}
	runner.broker.reset(
		runner.series,
		runner.cols,
		runner.htfTrend,
		runner.ema,
		runner.emaSlope,
		runner.params,
		runner.fixture,
		nil,
	)

	const firstCandidate = 677
	if got := sessionPhaseName(runner.broker.cols.SessionPhase[firstCandidate]); got != "close" {
		t.Fatalf("reviewed candidate phase = %q, want close", got)
	}
	runner.broker.onBar(firstCandidate)
	if runner.broker.hasLSEntry || runner.broker.hasPosition || len(runner.broker.seen.ls.keys) != 0 {
		t.Fatalf("generic phase rejection consumed candidate: cooldown=%v position=%v seen=%v",
			runner.broker.hasLSEntry, runner.broker.hasPosition, runner.broker.seen.ls.keys)
	}

	// Generic failed breakout applies its market gate before cooldown and seen
	// state in both runtimes, so the same candidate remains admissible later.
	runner.broker.cols.SessionPhase[firstCandidate] = 1
	runner.broker.onBar(firstCandidate)
	if !runner.broker.hasLSEntry || !runner.broker.hasPosition || len(runner.broker.seen.ls.keys) != 1 {
		t.Fatalf("allowed generic candidate did not enter: cooldown=%v position=%v seen=%v",
			runner.broker.hasLSEntry, runner.broker.hasPosition, runner.broker.seen.ls.keys)
	}
}

func TestContextOptionsRequestAllowedSessionPhase(t *testing.T) {
	tests := []struct {
		name string
		cfg  dsl.Config
		want bool
	}{
		{
			name: "authored phase",
			cfg: dsl.Config{
				"setupType":     string(dsl.FamilyFlagContinuation),
				"sessionPhases": []any{"open"},
			},
			want: true,
		},
		{
			name: "existing lunch avoidance",
			cfg: dsl.Config{
				"setupType": string(dsl.FamilyFlagContinuation),
				"flag":      map[string]any{"avoidLunchBreakouts": true},
			},
			want: true,
		},
		{
			name: "no phase consumer",
			cfg:  dsl.Config{"setupType": string(dsl.FamilyFlagContinuation)},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := contextOptions(RunFixture{}, tt.cfg)
			if !options.Selective {
				t.Fatal("flag context should remain selective")
			}
			if options.NeedSessionPhase != tt.want {
				t.Fatalf("NeedSessionPhase = %v, want %v", options.NeedSessionPhase, tt.want)
			}
		})
	}
}

func TestSessionPhaseNameMatchesJSContextCodes(t *testing.T) {
	tests := []struct {
		code int8
		want string
	}{
		{code: 1, want: "open"},
		{code: 2, want: "middle"},
		{code: 3, want: "lunch"},
		{code: 4, want: "close"},
		{code: 5, want: "overnight"},
		{code: 6, want: "other"},
		{code: 0, want: ""},
	}
	for _, tt := range tests {
		if got := sessionPhaseName(tt.code); got != tt.want {
			t.Errorf("sessionPhaseName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}
