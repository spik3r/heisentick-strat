package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const entryRetestSource = `dsl v7
strategy "Retest on entry timeframe" { description "Synthetic zone retested on the entry candles." }
market conditions {
  slices(XAUUSD 1h)
  sessions(london, ny)
  day type in (trending, ranging, choppy)
  movement below 1.0
}
setup {
  type: supply demand
  source timeframe 4h
  retest on entry timeframe
  patterns(DBR, RBR, RBD, DBD)
  base 1 to 4 candles
  impulse at least 2.5 ATR within 4 candles
  zone width max 1.0 ATR
  retest within 96 candles
  retest first touch only
  retest must reject zone
  wait 0 candles after zone
}
filters {
  higher timeframe off
  side both
}
risk {
  stop: beyond zone + 0.3 ATR min 0.4 ATR max 3 ATR
}
target {
  target 2R
}
management {
  breakeven off
  wait 6 candles after trade
}
entryTf 1h
`

const (
	hourMS     = 3600e3
	retestBar  = 157 // 13:00 UTC, inside the second source candle after the zone
	zoneSource = 34  // source index at which the departure completes
)

// entryRetestBars builds a 1h chart and its 4h source: a calm stretch, a one-bar
// base at source 30, a rally through source 34, a plateau, then one 1h bar at
// retestBar that dips into the zone and closes back above it.
func entryRetestBars(mutateAfterSource int) (chart []marketdata.Bar, source []marketdata.Bar) {
	n := 4 * 50
	chart = make([]marketdata.Bar, n)
	for i := 0; i < n; i++ {
		o, c := 100.0, 100.0
		h, l := 100.5, 99.5
		switch {
		case i >= 120 && i < 124: // base candle(s)
			h, l = 100.2, 99.8
		case i >= 124 && i < 140: // rally, +0.5 a bar
			o = 100 + 0.5*float64(i-124)
			c = o + 0.5
			h, l = c+0.1, o-0.1
		case i >= 140:
			o, c = 108, 108
			h, l = 108.5, 107.5
		}
		if i == retestBar {
			o, h, l, c = 108, 108.2, 100.1, 101
		}
		chart[i] = marketdata.Bar{T: float64(i) * hourMS, O: o, H: h, L: l, C: c, V: 1}
	}
	if mutateAfterSource >= 0 {
		for i := 4 * mutateAfterSource; i < n; i++ {
			chart[i].H += 40
			chart[i].L -= 40
			chart[i].C += 20
		}
	}
	source = make([]marketdata.Bar, 0, n/4)
	for s := 0; s < n/4; s++ {
		bar := marketdata.Bar{T: chart[4*s].T, O: chart[4*s].O, H: math.Inf(-1), L: math.Inf(1), C: chart[4*s+3].C, V: 4}
		for j := 4 * s; j < 4*s+4; j++ {
			bar.H = math.Max(bar.H, chart[j].H)
			bar.L = math.Min(bar.L, chart[j].L)
		}
		source = append(source, bar)
	}
	return chart, source
}

func runEntryRetest(t *testing.T, chart, source []marketdata.Bar) RunResult {
	t.Helper()
	fixture := RunFixture{
		Schema: runFixtureSchema, Case: "entry-retest", StrategyID: "entry-retest", Symbol: "XAUUSD", Timeframe: "1h",
		SourceTimeframe: "4h", RangeMethod: "zone", Bars: chart, SourceBars: source,
	}
	result, err := RunFixtureCase(fixture, entryRetestSource)
	if err != nil {
		t.Fatalf("RunFixtureCase: %v", err)
	}
	return result
}

func TestRetestOnEntryTimeframeEntersOnTheEntryCandle(t *testing.T) {
	chart, source := entryRetestBars(-1)
	result := runEntryRetest(t, chart, source)
	if len(result.Trades) != 1 {
		t.Fatalf("trades = %d, want 1: %+v", len(result.Trades), result.Trades)
	}
	trade := result.Trades[0]
	if trade.EntryIndex != retestBar || trade.Side != "long" {
		t.Fatalf("entry index %d side %s, want %d long (the 1h rejection candle, not a 4h close)", trade.EntryIndex, trade.Side, retestBar)
	}
	if retestBar%4 == 3 {
		t.Fatal("test bar must not sit on a 4h boundary")
	}
	meta := trade.Meta
	if meta["zoneType"] != "demand" || int(meta["baseEnd"].(int)) != 30 {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestRetestOnEntryTimeframeIgnoresSourceCandlesThatHaveNotClosed(t *testing.T) {
	chart, source := entryRetestBars(-1)
	base := runEntryRetest(t, chart, source)
	// Wreck every source candle from index 40 on: the entry at chart bar 157 closes
	// inside source candle 39, so nothing decided up to there may change.
	mutatedChart, mutatedSource := entryRetestBars(40)
	changed := runEntryRetest(t, mutatedChart, mutatedSource)
	if len(base.Trades) != 1 || len(changed.Trades) == 0 {
		t.Fatalf("base %d, changed %d trades", len(base.Trades), len(changed.Trades))
	}
	got, want := changed.Trades[0], base.Trades[0]
	if got.EntryIndex != want.EntryIndex || got.Entry != want.Entry || got.InitialSL != want.InitialSL || got.InitialTP != want.InitialTP {
		t.Fatalf("future source candles changed the entry: got %+v want %+v", got, want)
	}
}

func TestRetestOnEntryTimeframeNeedsAFullyClosedZoneCandle(t *testing.T) {
	chart, source := entryRetestBars(-1)
	// A touch inside the very source candle that completes the departure must not trade.
	chart[4*zoneSource+1] = marketdata.Bar{T: chart[4*zoneSource+1].T, O: 107, H: 107.2, L: 100.1, C: 101, V: 1}
	for s := range source {
		if s == zoneSource {
			source[s].L = math.Min(source[s].L, 100.1)
		}
	}
	result := runEntryRetest(t, chart, source)
	for _, trade := range result.Trades {
		if trade.EntryIndex < 4*(zoneSource+1) {
			t.Fatalf("entered on bar %d before source candle %d closed", trade.EntryIndex, zoneSource)
		}
	}
}
