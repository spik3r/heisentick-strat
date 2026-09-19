package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

// TestGapThroughStopFillsAtOpen pins conformance/semantic/pm-gap-through-stop:
// a carried stop that the next bar opens beyond fills at the worse open, not
// at the stop level.
func TestGapThroughStopFillsAtOpen(t *testing.T) {
	trade := runSemanticCase(t, "pm-gap-through-stop")
	if trade.EntryIndex != 31 || trade.Entry != 2020 {
		t.Fatalf("entry = %v @ %d, want 2020 @ 31", trade.Entry, trade.EntryIndex)
	}
	if trade.ExitIndex != 32 || trade.Exit != 1980 || trade.Reason != "sl" {
		t.Fatalf("exit = %v @ %d (%s), want 1980 @ 32 (sl)", trade.Exit, trade.ExitIndex, trade.Reason)
	}
}

// TestGapThroughTargetFillsAtTarget pins conformance/semantic/pm-gap-through-target:
// a favourable gap through the target is credited at the target level.
func TestGapThroughTargetFillsAtTarget(t *testing.T) {
	trade := runSemanticCase(t, "pm-gap-through-target")
	if trade.EntryIndex != 31 || trade.Entry != 2020 {
		t.Fatalf("entry = %v @ %d, want 2020 @ 31", trade.Entry, trade.EntryIndex)
	}
	if trade.ExitIndex != 32 || trade.Exit != 2047.5 || trade.Reason != "tp" {
		t.Fatalf("exit = %v @ %d (%s), want 2047.5 @ 32 (tp)", trade.Exit, trade.ExitIndex, trade.Reason)
	}
}

func runSemanticCase(t *testing.T, name string) Trade {
	t.Helper()
	dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", name)
	fixture, err := LoadRunFixture(filepath.Join(dir, "fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(dir, "strategy.strat"))
	if err != nil {
		t.Fatalf("read strategy: %v", err)
	}
	result, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run fixture: %v", err)
	}
	if len(result.Trades) != 1 {
		t.Fatalf("trades = %d, want 1: %+v", len(result.Trades), result.Trades)
	}
	return result.Trades[0]
}
