package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
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

// Regression: the one-per-level-per-day dedup must be keyed on the level's
// *price*, not just its name (heisentick-backlog HT-054 Phase C — found live
// via the dslDailySndRetestXauusdFourHour port's trade-by-trade parity run).
// PDL is re-resolved every bar, so its price rolls over at UTC midnight
// (the calendar-day boundary), which does not line up with this dedup's own
// "local day" boundary (localDayKey's fixed UTC+10h offset). Keying on the
// level name alone would let a signal against one calendar day's PDL
// silently suppress an unrelated signal against the *next* day's (different)
// PDL for the rest of the dedup's local day. Mirrors the identical fix in
// heisentick's engine/dsl/setups/namedLevelSweep.js.
func namedLevelSweepPriceKeyDedupFixture() RunFixture {
	const hour = 3_600_000.0
	// Dec 31 2023 (day0, low 90), Jan 1 2024 (day1, low 85 via its own
	// stop-out bar), Jan 2 2024 (day2) — three calendar days of 4h bars.
	t0 := 1703980800000.0 // 2023-12-31T00:00:00Z
	bars := []marketdata.Bar{
		{T: t0 + 0*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 4*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 8*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 12*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 16*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 20*hour, O: 100, H: 101, L: 90, C: 100}, // day0 low = 90
		{T: t0 + 24*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 28*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 32*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 36*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 40*hour, O: 100, H: 101, L: 87, C: 91}, // sweep #1 vs PDL=90 (day0), closes back above
		{T: t0 + 44*hour, O: 91, H: 92, L: 85, C: 90},   // stops out sweep #1; day1 low = 85
		{T: t0 + 48*hour, O: 90, H: 91, L: 83, C: 86},   // sweep #2 vs PDL=85 (day1, a *different* price), closes back above
		{T: t0 + 52*hour, O: 86, H: 100, L: 85, C: 99},  // hits target, flat again
		{T: t0 + 56*hour, O: 99, H: 101, L: 98, C: 100},
		{T: t0 + 60*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 64*hour, O: 100, H: 101, L: 99, C: 100},
		{T: t0 + 68*hour, O: 100, H: 101, L: 99, C: 100},
	}
	return RunFixture{
		Schema:      "dsl-conformance-run-fixture-v1",
		Case:        "named-level-sweep-price-key-dedup-regression",
		Symbol:      "XAUUSD",
		Timeframe:   "4h",
		RangeMethod: "zone",
		Costs:       Costs{FeePerUnit: 0, FillOn: "close", Slippage: 0, StartEquity: 10000},
		Bars:        bars,
	}
}

func TestNamedLevelSweepDedupKeyedOnLevelPriceNotJustName(t *testing.T) {
	const source = `dsl v8
strategy "Named Level Sweep Price-Keyed Dedup Regression" {
  description "A signal against one day's PDL must not suppress a later signal against a different day's PDL in the same dedup-local-day bucket."
}

market conditions {
  slices(XAUUSD 4h)
  day type in (ranging, choppy, trending)
  movement below 1.5
}

levels {
  priority(PDL)
}

filters {
  side long only
  trade window unrestricted
}

trigger {
  no confirmation candle
}

setup {
  type: named level sweep
}

entry {
  when price sweeps PDL and closes back above it then signal long
}

risk {
  stop below the signal candle and the level by 0 ATR
  stop size min 0 max 10
}

target {
  target 0.5R
}

management {
  wait 0 candles after trade
}

execution {
  risk 200 USD
}`
	result, err := RunFixtureCase(namedLevelSweepPriceKeyDedupFixture(), source)
	if err != nil {
		t.Fatalf("run fixture: %v", err)
	}
	if result.TradeCount != 2 {
		t.Fatalf("trades = %d, want exactly 2 (a differently-priced PDL later the same dedup-local-day must still be able to signal)", result.TradeCount)
	}
}

// "closes back above|below it" with no `by at least` margin is a strict
// close past the level: a bar that wicks through and closes exactly on the
// level has not reclaimed it (the hand-written originals use `c > level`).
// With a margin, a close exactly margin*ATR past the level does count.
func TestNamedLevelSweepReclaimCloseOnLevelIsNotAReclaim(t *testing.T) {
	series := marketdata.Series{
		T: []float64{0}, O: []float64{100.5}, H: []float64{101}, L: []float64{99}, C: []float64{100}, V: []float64{0},
	}
	b := broker{series: series}
	long := namedLevelSweepRule{Mode: "sweep", Side: sideLong, ReclaimCandles: 1}
	short := namedLevelSweepRule{Mode: "sweep", Side: sideShort, ReclaimCandles: 1}
	if b.namedLevelSweptOrTested(0, long, 100, 1) {
		t.Fatal("long: close exactly on the level must not count as closing back above it")
	}
	if b.namedLevelSweptOrTested(0, short, 100, 1) {
		t.Fatal("short: close exactly on the level must not count as closing back below it")
	}
	if !b.namedLevelSweptOrTested(0, long, 99.9, 1) {
		t.Fatal("long: a strict close above the swept level must reclaim")
	}
	withMargin := namedLevelSweepRule{Mode: "sweep", Side: sideLong, ReclaimCandles: 1, ReclaimAtr: 0.5}
	if !b.namedLevelSweptOrTested(0, withMargin, 99.5, 1) {
		t.Fatal("long: a close exactly `by at least` the margin past the level must reclaim")
	}
}
