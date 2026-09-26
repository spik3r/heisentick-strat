package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase B (heisentick-backlog plan §5): behaviour tests for the named level
// sweep family, built on the same fixture-mutation idiom
// day_theme_gate_test.go already uses (RunFixtureCase against the family's
// own conformance fixture, with a targeted phrase substitution per case) so
// each case exercises the real parser + runtime, not a hand-built broker.
const namedLevelSweepCase = "family-named-level-sweep"

func loadNamedLevelSweepFixtureSource(t *testing.T) (RunFixture, string) {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), namedLevelSweepCase+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), namedLevelSweepCase+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	return fixture, string(source)
}

func TestNamedLevelSweepBaselineFixtureSweepsAndReclaims(t *testing.T) {
	fixture, source := loadNamedLevelSweepFixtureSource(t)
	result, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	if result.TradeCount != 1 || len(result.Trades) != 1 {
		t.Fatalf("baseline trades = %d/%d, want 1", result.TradeCount, len(result.Trades))
	}
	trade := result.Trades[0]
	if trade.Side != "long" || trade.Meta["mode"] != "sweep" || trade.Meta["levelKey"] != "PDL" {
		t.Fatalf("unexpected baseline trade: %+v", trade)
	}
}

func TestNamedLevelSweepStopSizeMaxRejectsWideStop(t *testing.T) {
	fixture, source := loadNamedLevelSweepFixtureSource(t)
	mutated := strings.Replace(source, "stop size min 0 max 10", "stop size min 0 max 0.1", 1)
	if mutated == source {
		t.Fatal("fixture source did not contain the expected stop-size directive")
	}
	result, err := RunFixtureCase(fixture, mutated)
	if err != nil {
		t.Fatalf("run mutated fixture: %v", err)
	}
	if result.TradeCount != 0 || len(result.Trades) != 0 {
		t.Fatalf("trades = %d, want 0 (stop distance should exceed max 0.1 ATR)", result.TradeCount)
	}
}

func TestNamedLevelSweepStopSizeMinRejectsTightStop(t *testing.T) {
	fixture, source := loadNamedLevelSweepFixtureSource(t)
	mutated := strings.Replace(source, "stop size min 0 max 10", "stop size min 5 max 10", 1)
	if mutated == source {
		t.Fatal("fixture source did not contain the expected stop-size directive")
	}
	result, err := RunFixtureCase(fixture, mutated)
	if err != nil {
		t.Fatalf("run mutated fixture: %v", err)
	}
	if result.TradeCount != 0 || len(result.Trades) != 0 {
		t.Fatalf("trades = %d, want 0 (stop distance should be below min 5 ATR)", result.TradeCount)
	}
}

func TestNamedLevelSweepHTFAgreementBlocksWithoutHTFData(t *testing.T) {
	fixture, source := loadNamedLevelSweepFixtureSource(t)
	mutated := strings.Replace(source, "filters {\n  side long only", "filters {\n  higher timeframe must agree\n  side long only", 1)
	if mutated == source {
		t.Fatal("fixture source did not contain the expected filters block")
	}
	result, err := RunFixtureCase(fixture, mutated)
	if err != nil {
		t.Fatalf("run mutated fixture: %v", err)
	}
	// The fixture's HTF bars are deliberately absent/unused (fail-closed per
	// htfAllows: a requested-but-missing HTF bar rejects rather than admits).
	if result.TradeCount != 0 || len(result.Trades) != 0 {
		t.Fatalf("trades = %d, want 0 (HTF agreement requested with no HTF data must fail closed)", result.TradeCount)
	}
}

func TestNamedLevelSweepOnePerLevelPerDayDedup(t *testing.T) {
	fixture, source := loadNamedLevelSweepFixtureSource(t)
	// Widen the tests-mode-equivalent geometry so the same PDL sweep condition
	// stays true across two consecutive bars, then confirm only one signal
	// fires per level per day (the second bar's identical geometry is
	// deduped, not re-signaled).
	mutated := strings.Replace(source,
		"entry {\n  when price sweeps PDL and closes back above it then signal long\n}",
		"entry {\n  when price tests PDL within 5 ATR and closes above it then signal long\n}", 1)
	if mutated == source {
		t.Fatal("fixture source did not contain the expected entry phrase")
	}
	fixture.Bars = append(fixture.Bars, fixture.Bars[len(fixture.Bars)-1])
	result, err := RunFixtureCase(fixture, mutated)
	if err != nil {
		t.Fatalf("run mutated fixture: %v", err)
	}
	if result.TradeCount != 1 {
		t.Fatalf("trades = %d, want exactly 1 (one signal per level per day)", result.TradeCount)
	}
}
