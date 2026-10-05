package engine

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// These prices are invented: 160 repeating range bars, a long lower-wick
// rejection and a staged rise. Mirroring around 100 supplies the short case.
// The first favorable bar cannot hit even 1R, the second hits 1R, and the
// third hits 2R. This catches correct entries paired with incorrect exits.
func TestFailedBreakoutNumericTargetTradeOutcomes(t *testing.T) {
	for _, short := range []bool{false, true} {
		sideName := "long"
		if short {
			sideName = "short"
		}
		for _, tc := range []struct {
			name, target string
			reward       float64
			exitIndex    int
		}{
			{"numeric 1R", "target 1R", 1, 165},
			{"numeric 2R", "target 2R", 2, 166},
			{"explicit fallback 2R", "fallback 2R", 2, 166},
			{"default fallback", "take profit at opposite range edge", 1, 165},
		} {
			t.Run(sideName+"/"+tc.name, func(t *testing.T) {
				fixture, source := failedBreakoutTargetFixture(t, short)
				source = strings.Replace(source, "target 2R", tc.target, 1)
				result, err := RunFixtureCase(fixture, source)
				if err != nil {
					t.Fatal(err)
				}
				if result.TradeCount != 1 || len(result.Trades) != 1 {
					t.Fatalf("want one trade, got %+v", result)
				}
				tr := result.Trades[0]
				sign := 1.0
				if short {
					sign = -1
				}
				// Signal ATR is 19/14; the extreme is 95 (or mirrored 105).
				// Risk is 5 + (19/14)/4 = 299/56, independent of the result.
				risk := 299.0 / 56
				wantTarget := tr.Entry + sign*risk*tc.reward
				if tr.Side != sideName || tr.EntryIndex != 163 || tr.Entry != 100 || tr.ExitIndex != tc.exitIndex || tr.Reason != "tp" {
					t.Fatalf("entry/exit identity changed: %+v", tr)
				}
				for key, pair := range map[string][2]float64{"initial stop": {tr.InitialSL, tr.Entry - sign*risk}, "initial target": {tr.InitialTP, wantTarget}, "target": {tr.TP, wantTarget}, "exit": {tr.Exit, wantTarget}, "size": {tr.Size, 200 / risk}, "pnl": {tr.PnL, 200 * tc.reward}} {
					if math.Abs(pair[0]-pair[1]) > 1e-9 {
						t.Errorf("%s = %.15g, want %.15g", key, pair[0], pair[1])
					}
				}
				// Exercise public, reusable prepared, and shared-context requests too.
				parsed := parseEngineTestDSL(t, source)
				request := failedBreakoutPriorityRequest(fixture, parsed.Config)
				shared, err := PrepareSharedRunContext(request)
				if err != nil {
					t.Fatal(err)
				}
				checked := runFailedBreakoutPriorityPaths(t, shared, request, parsed.Config)
				if !reflect.DeepEqual(checked.Trades, result.Trades) {
					t.Fatal("checked paths differ from fixture outcome")
				}
			})
		}
	}
}

func TestFailedBreakoutNumericTargetPreservesOppositeEdgePriority(t *testing.T) {
	for _, short := range []bool{false, true} {
		fixture, source := failedBreakoutTargetFixture(t, short)
		// A shallower wick makes the opposite edge exceed the 0.6R minimum.
		if short {
			fixture.Bars[163].H = 101.5
		} else {
			fixture.Bars[163].L = 98.5
		}
		fixture.RawBars = nil
		result, err := RunFixtureCase(fixture, source)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Trades) != 1 {
			t.Fatalf("want one edge trade, got %v", result.Trades)
		}
		tr := result.Trades[0]
		key := "rangeHi"
		if short {
			key = "rangeLo"
		}
		edge := tr.Meta[key].(float64)
		if tr.Reason != "tp" || tr.ExitIndex != 164 || math.Abs(tr.Exit-edge) > 1e-9 || math.Abs(tr.InitialTP-edge) > 1e-9 {
			t.Fatalf("opposite edge must win over fallback: %+v", tr)
		}
		reward := math.Abs(tr.Exit-tr.Entry) / math.Abs(tr.Entry-tr.InitialSL)
		if reward < .6 || reward >= 2 {
			t.Fatalf("fixture must exercise an eligible edge below 2R, got %vR", reward)
		}
	}
}

func failedBreakoutTargetFixture(t *testing.T, short bool) (RunFixture, string) {
	t.Helper()
	f, err := LoadRunFixture(filepath.Join("testdata", "failed-breakout-target.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := os.ReadFile(filepath.Join("testdata", "failed-breakout-target.strat"))
	if err != nil {
		t.Fatal(err)
	}
	if short {
		for i, b := range f.Bars {
			f.Bars[i] = marketdata.Bar{T: b.T, O: 200 - b.O, H: 200 - b.L, L: 200 - b.H, C: 200 - b.C, V: b.V}
		}
		f.RawBars = nil
	}
	return f, string(s)
}
