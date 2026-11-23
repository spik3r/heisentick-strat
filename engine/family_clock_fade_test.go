package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

func runClockFadeSemantic(t *testing.T, edit func(string) string, adjust func(*RunFixture)) (RunResult, error) {
	t.Helper()
	dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", "cf-fade-ny-close")
	fixture, err := LoadRunFixture(filepath.Join(dir, "fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	if adjust != nil {
		adjust(&fixture)
	}
	source, err := os.ReadFile(filepath.Join(dir, "strategy.strat"))
	if err != nil {
		t.Fatalf("read strategy: %v", err)
	}
	text := string(source)
	if edit != nil {
		text = edit(text)
	}
	return RunFixtureCase(fixture, text)
}

// The hand-derived case: bars 32 to 35 close 2000, 2004, 2008, 2012, so the
// 3-candle move into the 15:55 New York bar is +12. The family shorts at the
// signal close (2012) with a 3 ATR stop (ATR 10 gives 2042) and exits at the
// close of the next bar (2006) on the time stop.
func TestClockFadeSemanticCase(t *testing.T) {
	result, err := runClockFadeSemantic(t, nil, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Trades) != 1 {
		t.Fatalf("trades = %d, want 1: %+v", len(result.Trades), result.Trades)
	}
	got := result.Trades[0]
	if got.Side != "short" || got.EntryIndex != 35 || got.Entry != 2012 || got.InitialSL != 2042 ||
		got.ExitIndex != 36 || got.Exit != 2006 || got.Reason != "time" {
		t.Fatalf("trade = %+v", got)
	}
}

// A clock minute with no move in its window (the flat 15:30 bar) has no signal.
func TestClockFadeOtherMinuteNoTrade(t *testing.T) {
	result, err := runClockFadeSemantic(t, func(s string) string {
		return strings.Replace(s, "signal at new york 15:55", "signal at new york 15:30", 1)
	}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Trades) != 0 {
		t.Fatalf("trades = %+v, want none", result.Trades)
	}
}

// A gap inside the lookback window is not a trailing move of N bars.
func TestClockFadeGapInWindowNoTrade(t *testing.T) {
	result, err := runClockFadeSemantic(t, nil, func(f *RunFixture) {
		for i := 34; i < len(f.Bars); i++ {
			f.Bars[i].T += 300000
		}
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(result.Trades) != 0 {
		t.Fatalf("trades = %+v, want none", result.Trades)
	}
}

func TestClockFadeDirection(t *testing.T) {
	ts := []float64{0, 1, 2, 3, 4}
	closes := []float64{10, 11, 12, 13, 12}
	if s, _, ok := clockFadeDirection(ts, closes, 3, 3); !ok || s != sideShort {
		t.Fatalf("up move: side %v ok %v, want short", s, ok)
	}
	if s, _, ok := clockFadeDirection(ts, []float64{13, 12, 11, 10, 9}, 4, 3); !ok || s != sideLong {
		t.Fatalf("down move: side %v ok %v, want long", s, ok)
	}
	if _, _, ok := clockFadeDirection(ts, []float64{10, 10, 10, 10, 10}, 3, 3); ok {
		t.Fatal("zero move must not signal")
	}
	if _, _, ok := clockFadeDirection(ts, closes, 2, 3); ok {
		t.Fatal("window before the first bar must not signal")
	}
	closes[1] = math.NaN()
	if _, _, ok := clockFadeDirection(ts, closes, 4, 3); ok {
		t.Fatal("non-finite prior close must fail closed")
	}
}
