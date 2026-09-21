package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

const htfHour = float64(60 * 60 * 1000)
const htfQuarter = float64(15 * 60 * 1000)

// buildHTFBars returns 1h HTF bars whose close is flat at 100 except an upward
// candle opening at 24h and a downward candle opening at 25h. When dropIndex
// is >= 0 that HTF bar is omitted, leaving a source gap at its boundary.
func buildHTFBars(dropIndex int) []marketdata.Bar {
	bars := make([]marketdata.Bar, 0, 26)
	for i := 0; i < 26; i++ {
		if i == dropIndex {
			continue
		}
		o, c := 100.0, 100.0
		if i == 24 {
			o = 100
			c = 200
		}
		if i == 25 {
			o = 200
			c = 0
		}
		bars = append(bars, marketdata.Bar{T: float64(i) * htfHour, O: o, H: max(o, c) + 1, L: min(o, c) - 1, C: c, V: 1})
	}
	return bars
}

// contiguousPrimary builds 15m primary bars with opens in [startHour, endHour).
func contiguousPrimary(startHour, endHour float64) marketdata.Series {
	bars := []marketdata.Bar{}
	for t := startHour * htfHour; t < endHour*htfHour; t += htfQuarter {
		bars = append(bars, marketdata.Bar{T: t, O: 100, H: 101, L: 99, C: 100, V: 1})
	}
	return marketdata.SeriesFromBars(bars)
}

// TestRunnerHTFProjectionIsCausalCompletedBar pins the C2 completed-bar rule:
// an HTF bar is projected onto a primary bar only once the HTF bar has closed
// at or before the primary bar's own close, the still-forming HTF bar is never
// read, and a missing required HTF bar reads as unavailable rather than stale.
func TestRunnerHTFProjectionIsCausalCompletedBar(t *testing.T) {
	primary := contiguousPrimary(23, 27) // opens 23:00 .. 26:45 inclusive
	trend := computeHTFTrend(primary, marketdata.SeriesFromBars(buildHTFBars(-1)))

	// Index helper: primary opens start at 23:00, one every 15 minutes.
	at := func(hour, minute float64) int8 {
		idx := int(((hour-23)*60 + minute) / 15)
		return trend[idx]
	}

	// 24:45 closes at 25:00, exactly when the 24h HTF bar closes -> first read.
	if got := at(24, 45); got != trendUp {
		t.Fatalf("24:45 primary = %d, want trendUp (just-closed 24h HTF bar)", got)
	}
	// 25:00 closes at 25:15: the 25h HTF bar is still forming, keep the 24h bar.
	if got := at(25, 0); got != trendUp {
		t.Fatalf("25:00 primary = %d, want trendUp (forming 25h bar must not be read)", got)
	}
	// 25:45 closes at 26:00, exactly when the 25h HTF bar closes -> next read.
	if got := at(25, 45); got != trendDown {
		t.Fatalf("25:45 primary = %d, want trendDown (just-closed 25h HTF bar)", got)
	}
	// The 23h candle is already complete at this point, so its flat direction
	// is available even though the 24h candle is still forming.
	if got := at(24, 30); got != trendFlat {
		t.Fatalf("24:30 primary = %d, want trendFlat (last completed candle is flat)", got)
	}
}

func TestRunnerHTFDirectionUsesLastCompletedCandle(t *testing.T) {
	htf := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 100, H: 105, L: 99, C: 101, V: 1},
		{T: htfHour, O: 101, H: 102, L: 98, C: 100, V: 1},
		{T: 2 * htfHour, O: 100, H: 101, L: 99, C: 100, V: 1},
	})
	primary := contiguousPrimary(0, 3.25)
	trend := computeHTFTrend(primary, htf)

	if got := trend[3]; got != trendUp { // 00:45 closes at 01:00
		t.Fatalf("first completed HTF candle = %d, want trendUp", got)
	}
	if got := trend[4]; got != trendUp { // 01:00 is inside the down candle
		t.Fatalf("forming HTF candle = %d, want prior trendUp", got)
	}
	if got := trend[7]; got != trendDown { // 01:45 closes at 02:00
		t.Fatalf("second completed HTF candle = %d, want trendDown", got)
	}
	if got := trend[11]; got != trendFlat { // 02:45 closes at 03:00
		t.Fatalf("flat completed HTF candle = %d, want trendFlat", got)
	}
}

func TestRunnerHTFUnclosedBarFixtureKeepsCausalTradeBoundary(t *testing.T) {
	dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", "pm-htf-unclosed-bar-no-lookahead")
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
	trade := result.Trades[0]
	if trade.EntryIndex != 42 || trade.ExitIndex != 45 || trade.Reason != "tp" {
		t.Fatalf("trade boundary = %d -> %d (%s), want 42 -> 45 (tp)", trade.EntryIndex, trade.ExitIndex, trade.Reason)
	}
}

// A missing required HTF bar must read as unavailable, never the stale prior bar.
func TestRunnerHTFProjectionGapIsUnavailable(t *testing.T) {
	primary := contiguousPrimary(25, 27) // opens 25:00 .. 26:45
	trend := computeHTFTrend(primary, marketdata.SeriesFromBars(buildHTFBars(25)))
	// 25:45 closes at 26:00; the 25h HTF bar is absent, so the 24h bar is now a
	// full period stale and must not be reused. 25:45 is the fourth 15m open.
	idx := 3
	if trend[idx] != htfUnavailable {
		t.Fatalf("gapped 25:45 primary = %d, want htfUnavailable", trend[idx])
	}
}

// When no HTF series is supplied the projection is nil (HTF not in use), which
// callers treat as a no-op rather than a fail-closed unavailable state.
func TestRunnerHTFProjectionAbsentSeriesIsNil(t *testing.T) {
	primary := contiguousPrimary(24, 26)
	if trend := computeHTFTrend(primary, marketdata.Series{}); trend != nil {
		t.Fatalf("absent HTF series projection = %v, want nil", trend)
	}
}
