package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

type donchianSemanticTrade struct {
	Side       string  `json:"side"`
	EntryIndex int     `json:"entryIndex"`
	Entry      float64 `json:"entry"`
	InitialSL  float64 `json:"initialSl"`
	InitialTP  float64 `json:"initialTp"`
	ExitIndex  int     `json:"exitIndex"`
	Exit       float64 `json:"exit"`
	Reason     string  `json:"reason"`
}

// TestDonchianBreakoutSemanticCases checks the hand-derived dcb-* cases in
// conformance/semantic. Their expected.json files are written by hand from
// the spec and are never regenerated from an engine.
func TestDonchianBreakoutSemanticCases(t *testing.T) {
	for _, name := range []string{"dcb-signal-close-entry", "dcb-close-equal-rail-no-entry", "dcb-trend-ema-gate"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", name)
			raw, err := os.ReadFile(filepath.Join(dir, "expected.json"))
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			var expected struct {
				Trades []donchianSemanticTrade `json:"trades"`
			}
			if err := json.Unmarshal(raw, &expected); err != nil {
				t.Fatalf("decode expected: %v", err)
			}
			result := runDonchianSemantic(t, dir, nil)
			if len(result.Trades) != len(expected.Trades) {
				t.Fatalf("trades = %d, want %d: %+v", len(result.Trades), len(expected.Trades), result.Trades)
			}
			for k, want := range expected.Trades {
				got := result.Trades[k]
				if got.Side != want.Side || got.EntryIndex != want.EntryIndex || got.Entry != want.Entry ||
					got.InitialSL != want.InitialSL || got.InitialTP != want.InitialTP ||
					got.ExitIndex != want.ExitIndex || got.Exit != want.Exit || got.Reason != want.Reason {
					t.Fatalf("trade %d = %+v, want %+v", k, got, want)
				}
			}
		})
	}
}

// The stop and target are measured from the signal close, not from the
// slipped fill.
func TestDonchianBreakoutStopFromSignalClose(t *testing.T) {
	dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", "dcb-signal-close-entry")
	result := runDonchianSemantic(t, dir, func(f *RunFixture) { f.Costs.Slippage = 1 })
	if len(result.Trades) != 1 {
		t.Fatalf("trades = %+v", result.Trades)
	}
	trade := result.Trades[0]
	if trade.Entry == 2010 {
		t.Fatalf("entry = %v, want a slipped fill", trade.Entry)
	}
	if trade.InitialSL != 1990 || trade.InitialTP != 2030 {
		t.Fatalf("brackets = %v / %v, want 1990 / 2030 from the 2010 signal close", trade.InitialSL, trade.InitialTP)
	}
}

func runDonchianSemantic(t *testing.T, dir string, adjust func(*RunFixture)) RunResult {
	t.Helper()
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
	result, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run fixture: %v", err)
	}
	return result
}

func TestDonchianChannelExcludesSignalBar(t *testing.T) {
	high := []float64{101, 103, 102, 110}
	low := []float64{99, 97, 98, 100}
	if _, _, ok := donchianChannel(high, low, 2, 3); ok {
		t.Fatal("index < n must have no channel")
	}
	upper, lower, ok := donchianChannel(high, low, 3, 3)
	if !ok || upper != 103 || lower != 97 {
		t.Fatalf("channel = %v %v %v, want 103 97", upper, lower, ok)
	}
	high[1] = math.NaN()
	if _, _, ok := donchianChannel(high, low, 3, 3); ok {
		t.Fatal("non-finite high must fail closed")
	}
}

// The gate case expects no trade, which a missing EMA column would also give.
// The same flat-then-breakout bars with `trend ema 10` must still enter:
// EMA(10) of closes at 2000 then 2010 is about 2001.8, below the close.
func TestDonchianBreakoutTrendEMAAdmitsCloseAboveEMA(t *testing.T) {
	dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", "dcb-signal-close-entry")
	fixture, err := LoadRunFixture(filepath.Join(dir, "fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(dir, "strategy.strat"))
	if err != nil {
		t.Fatalf("read strategy: %v", err)
	}
	gated := strings.Replace(string(source), "  channel 3 candles\n", "  channel 3 candles\n  trend ema 10\n", 1)
	if gated == string(source) {
		t.Fatal("strategy did not contain the channel line")
	}
	result, err := RunFixtureCase(fixture, gated)
	if err != nil {
		t.Fatalf("run fixture: %v", err)
	}
	if len(result.Trades) != 1 || result.Trades[0].EntryIndex != 20 {
		t.Fatalf("trades = %+v, want the bar 20 entry", result.Trades)
	}
	if ema, ok := numericMetaValue(result.Trades[0].Meta["trendEma"]); !ok || !(ema > 2000 && ema < 2010) {
		t.Fatalf("trendEma meta = %v", result.Trades[0].Meta["trendEma"])
	}
}
