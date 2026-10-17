package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

const emaRibbonT0 = 1767571200000.0 // 2026-01-05T00:00Z
const emaRibbonHour = 3600000.0

type emaRibbonSemanticTrade struct {
	Side       string  `json:"side"`
	EntryIndex int     `json:"entryIndex"`
	EntryT     float64 `json:"entryT"`
	Entry      float64 `json:"entry"`
	InitialSL  float64 `json:"initialSl"`
	ExitIndex  int     `json:"exitIndex"`
	ExitT      float64 `json:"exitT"`
	Exit       float64 `json:"exit"`
	Reason     string  `json:"reason"`
	Rule       string  `json:"rule"`
}

// TestEMARibbonSemanticCases checks the hand-derived er-* cases in
// conformance/semantic. Their expected.json files are written by hand from
// the spec and are never regenerated from an engine.
func TestEMARibbonSemanticCases(t *testing.T) {
	for _, name := range []string{"er-next-open-entry-ribbon-break"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", name)
			raw, err := os.ReadFile(filepath.Join(dir, "expected.json"))
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			var expected struct {
				Trades []emaRibbonSemanticTrade `json:"trades"`
			}
			if err := json.Unmarshal(raw, &expected); err != nil {
				t.Fatalf("decode expected: %v", err)
			}
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
			if len(result.Trades) != len(expected.Trades) {
				t.Fatalf("trades = %d, want %d: %+v", len(result.Trades), len(expected.Trades), result.Trades)
			}
			for k, want := range expected.Trades {
				got := result.Trades[k]
				if got.Side != want.Side || got.EntryIndex != want.EntryIndex || got.EntryT != want.EntryT ||
					got.Entry != want.Entry || got.InitialSL != want.InitialSL || got.ExitIndex != want.ExitIndex ||
					got.ExitT != want.ExitT || got.Exit != want.Exit || got.Reason != want.Reason || got.Rule != want.Rule {
					t.Fatalf("trade %d = %+v, want %+v", k, got, want)
				}
			}
		})
	}
}

// emaRibbonSeries builds hourly bars from closes; each bar opens at the
// prior close and has a 10-point range around its body.
func emaRibbonSeries(closes []float64) marketdata.Series {
	bars := make([]marketdata.Bar, len(closes))
	prev := closes[0]
	for i, c := range closes {
		o := prev
		bars[i] = marketdata.Bar{T: emaRibbonT0 + float64(i)*emaRibbonHour, O: o, H: math.Max(o, c) + 5, L: math.Min(o, c) - 5, C: c}
		prev = c
	}
	return marketdata.SeriesFromBars(bars)
}

func flatThen(n int, level float64, tail ...float64) []float64 {
	out := make([]float64, 0, n+len(tail))
	for i := 0; i < n; i++ {
		out = append(out, level)
	}
	return append(out, tail...)
}

func TestEMARibbonAlignmentIsStrict(t *testing.T) {
	cases := []struct {
		values []float64
		want   int
	}{
		{[]float64{3, 2, 1}, 1},
		{[]float64{1, 2, 3}, -1},
		{[]float64{3, 3, 1}, 0},
		{[]float64{3, 1, 2}, 0},
		{[]float64{3, math.NaN(), 1}, 0},
	}
	for _, c := range cases {
		if got := emaRibbonAlignment(c.values); got != c.want {
			t.Errorf("alignment(%v) = %d, want %d", c.values, got, c.want)
		}
	}
}

func TestEMARibbonEMAWarmup(t *testing.T) {
	out := emaRibbonEMA([]float64{1, 1, 1, 1, 1, 1, 1}, 2)
	for j := 0; j < 6; j++ {
		if !math.IsNaN(out[j]) {
			t.Fatalf("EMA(2)[%d] = %v, want NaN before 3 x length", j, out[j])
		}
	}
	if out[6] != 1 {
		t.Fatalf("EMA(2)[6] = %v, want 1", out[6])
	}
}

// A ribbon that stays aligned only re-enters after a close on the wrong
// side of the pullback EMA, and only when the pullback EMA is set.
func TestEMARibbonPullbackReentry(t *testing.T) {
	// Rising closes keep EMA(2) > EMA(3) throughout; bar 24 dips below EMA(2)
	// but not far enough to break the ribbon, and bar 25 closes back above.
	closes := flatThen(20, 2000, 2010, 2020, 2030, 2040, 2034, 2050)
	series := emaRibbonSeries(closes)
	base := emaRibbonSpec{emas: []int{2, 3}, stopATR: 3}
	entry, _ := computeEMARibbonColumns(series, base)
	if entry[20] <= 0 {
		t.Fatalf("bar 20 should be a fresh long signal, entry = %v", entry[20])
	}
	if entry[25] != 0 {
		t.Fatalf("bar 25 must not trigger without a pullback EMA, entry = %v", entry[25])
	}
	withPullback := base
	withPullback.pullback = 2
	entry, _ = computeEMARibbonColumns(series, withPullback)
	if entry[25] <= 0 {
		t.Fatalf("bar 25 should re-enter after the bar 24 close below EMA(2), entry = %v", entry[25])
	}
	if entry[23] != 0 {
		t.Fatalf("bar 23 follows a close above EMA(2) and must not re-enter, entry = %v", entry[23])
	}
}

// The act window gates entries and exit permission by the bar's close time.
func TestEMARibbonActWindow(t *testing.T) {
	closes := flatThen(20, 2000, 2010, 2020)
	series := emaRibbonSeries(closes)
	// Bar 20 closes at 21:00 UTC.
	inside := emaRibbonSpec{emas: []int{2, 3}, stopATR: 3, hasWindow: true, actFrom: 21, actTo: 3}
	entry, exit := computeEMARibbonColumns(series, inside)
	if entry[20] <= 0 || exit[20] == 0 {
		t.Fatalf("bar 20 closes inside 21-3 UTC: entry %v exit %v", entry[20], exit[20])
	}
	outside := emaRibbonSpec{emas: []int{2, 3}, stopATR: 3, hasWindow: true, actFrom: 22, actTo: 3}
	entry, exit = computeEMARibbonColumns(series, outside)
	if entry[20] != 0 || exit[20] != 0 {
		t.Fatalf("bar 20 closes outside 22-3 UTC: entry %v exit %v", entry[20], exit[20])
	}
	if exit[21] != float64(emaRibbonExitShort) {
		t.Fatalf("bar 21 closes at 22:00 inside the window while long-aligned: exit flags %v, want short-exit only", exit[21])
	}
}

// The higher-timeframe filter reads the last bucket closed by the bar's
// close and must agree with the trade direction.
func TestEMARibbonTrendFilterAndDailyGate(t *testing.T) {
	// 300 hourly bars falling then a sharp hourly rally: the 1h ribbon turns
	// long while the 4h EMA pair is still bearish.
	closes := make([]float64, 0, 310)
	for i := 0; i < 300; i++ {
		closes = append(closes, 2300-float64(i))
	}
	closes = append(closes, 2010, 2030, 2050)
	series := emaRibbonSeries(closes)
	spec := emaRibbonSpec{emas: []int{2, 3}, stopATR: 3}
	entry, _ := computeEMARibbonColumns(series, spec)
	longBar := -1
	for i := 300; i < len(closes); i++ {
		if entry[i] > 0 {
			longBar = i
			break
		}
	}
	if longBar < 0 {
		t.Fatalf("without a filter the rally should give a long signal, entries %v", entry[300:])
	}
	filtered := spec
	filtered.filter = &emaRibbonTrendFilter{hours: 4, pair: true, fast: 2, slow: 5}
	entry, _ = computeEMARibbonColumns(series, filtered)
	if entry[longBar] != 0 {
		t.Fatalf("a bearish 4h pair must block the long at bar %d, entry = %v", longBar, entry[longBar])
	}

	// Daily short gate: a short in a series whose last closed day is above
	// its daily EMA is blocked; longs are never gated.
	up := make([]float64, 0, 24*20)
	for i := 0; i < 24*20; i++ {
		up = append(up, 2000+float64(i))
	}
	up = append(up, 2300, 2200, 2100)
	series = emaRibbonSeries(up)
	shortSpec := emaRibbonSpec{emas: []int{2, 3}, stopATR: 3}
	entry, _ = computeEMARibbonColumns(series, shortSpec)
	shortBar := -1
	for i := len(up) - 3; i < len(up); i++ {
		if entry[i] < 0 {
			shortBar = i
			break
		}
	}
	if shortBar < 0 {
		t.Fatalf("expected an ungated short signal in the final bars, entries %v", entry[len(up)-3:])
	}
	shortSpec.shortDailyEMA = 3
	entry, _ = computeEMARibbonColumns(series, shortSpec)
	if entry[shortBar] != 0 {
		t.Fatalf("daily close above its EMA must block the short, entry = %v", entry[shortBar])
	}
}

func TestEMARibbonBucketsCloseAtBarClose(t *testing.T) {
	series := emaRibbonSeries(flatThen(9, 2000))
	closes, done := emaRibbonBuckets(series, 4*emaRibbonHour, emaRibbonHour)
	if len(closes) != 3 {
		t.Fatalf("buckets = %d, want 3", len(closes))
	}
	// Bar 3 closes at 04:00, which completes the 00:00-04:00 bucket.
	want := []int{-1, -1, -1, 0, 0, 0, 0, 1, 1}
	for i, w := range want {
		if done[i] != w {
			t.Fatalf("done[%d] = %d, want %d (%v)", i, done[i], w, done)
		}
	}
}

func TestEMARibbonTargetDistance(t *testing.T) {
	dir := filepath.Join(testsupport.StratConformanceRoot(), "semantic", "er-next-open-entry-ribbon-break")
	fixture, err := LoadRunFixture(filepath.Join(dir, "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(dir, "strategy.strat"))
	if err != nil {
		t.Fatal(err)
	}
	withTarget := string(source) + "\ntarget {\n  target 2R\n}\n"
	result, err := RunFixtureCase(fixture, withTarget)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 1 || result.Trades[0].InitialTP != 2070 {
		t.Fatalf("target 2R from fill 2010 with a 30-point stop should be 2070: %+v", result.Trades)
	}
}

func TestValidateEMARibbonConfig(t *testing.T) {
	good := func() dsl.Config {
		return dsl.Config{
			"setupType": "emaRibbon",
			"emaRibbon": map[string]any{"emas": []any{8.0, 21.0}},
			"stop":      map[string]any{"type": "atr", "atrMult": 3.0},
			"target":    map[string]any{},
		}
	}
	if err := validateEMARibbonConfig(good()); err != nil {
		t.Fatalf("valid config refused: %v", err)
	}
	bad := map[string]func(dsl.Config){
		"non-increasing emas": func(c dsl.Config) { c["emaRibbon"] = map[string]any{"emas": []any{21.0, 8.0}} },
		"one ema":             func(c dsl.Config) { c["emaRibbon"] = map[string]any{"emas": []any{8.0}} },
		"bad filter tf": func(c dsl.Config) {
			c["emaRibbon"] = map[string]any{"emas": []any{8.0, 21.0}, "trendFilter": map[string]any{"timeframe": "3h", "mode": "ribbon"}}
		},
		"pair fast >= slow": func(c dsl.Config) {
			c["emaRibbon"] = map[string]any{"emas": []any{8.0, 21.0}, "trendFilter": map[string]any{"timeframe": "4h", "mode": "pair", "fast": 21.0, "slow": 8.0}}
		},
		"equal act hours": func(c dsl.Config) {
			c["emaRibbon"] = map[string]any{"emas": []any{8.0, 21.0}, "actFromUtc": 5.0, "actToUtc": 5.0}
		},
		"one act hour":     func(c dsl.Config) { c["emaRibbon"] = map[string]any{"emas": []any{8.0, 21.0}, "actFromUtc": 5.0} },
		"non-atr stop":     func(c dsl.Config) { c["stop"] = map[string]any{"type": "extreme", "atrMult": 3.0} },
		"negative target":  func(c dsl.Config) { c["target"] = map[string]any{"erR": -1.0} },
		"source timeframe": func(c dsl.Config) { c["sourceTimeframe"] = "5m" },
		"family not a map": func(c dsl.Config) { c["emaRibbon"] = "8 21" },
		"target not a map": func(c dsl.Config) { c["target"] = "x" },
	}
	for name, mutate := range bad {
		cfg := good()
		mutate(cfg)
		if err := validateEMARibbonConfig(cfg); err == nil {
			t.Errorf("%s: config accepted", name)
		}
	}
	typed := good()
	typed["setupType"] = dsl.FamilyEMARibbon
	typed["emaRibbon"] = map[string]any{"emas": []any{21.0, 8.0}}
	if err := validateEMARibbonConfig(typed); err == nil {
		t.Fatal("a dsl.FamilyID setup type must be validated too")
	}
	if err := validateEMARibbonConfig(dsl.Config{"setupType": "donchianBreakout"}); err != nil {
		t.Fatalf("other families must pass through: %v", err)
	}
}
