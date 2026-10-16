package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"

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

func donchianBracketBroker(s side, atr, stopATR, targetR float64) broker {
	first := 100 - float64(s)*2
	bars := []marketdata.Bar{{T: 0, O: first, H: first + 0.5, L: first - 0.5, C: first, V: 1}, {T: 1, O: 100, H: 100.5, L: 99.5, C: 100, V: 1}, {T: 2, O: 100, H: 100.5, L: 99.5, C: 100, V: 1}}
	return broker{
		series: marketdata.SeriesFromBars(bars), cols: contextcols.Columns{ATR: []float64{atr, atr, atr}, ER: make([]float64, len(bars))}, htfTrend: []int8{trendFlat, trendFlat, trendFlat},
		costs:  Costs{FillOn: "close", StartEquity: 10000},
		params: flagParams{SetupType: string(dsl.FamilyDonchianBreakout), UseHTFBias: true, UseAsiaWindow: true, UseMidWindow: true, UseLondonWindow: true, UseNYWindow: true, AllowLong: true, AllowShort: true, RiskUSD: 200, MaxMovementER: 1.1, TargetR: targetR, DonchianBreakout: donchianBreakoutParams{ChannelBars: 1, StopATR: stopATR}},
	}
}

func TestDonchianBreakoutRejectsUnusableBracketGeometry(t *testing.T) {
	tests := []struct {
		name              string
		atr, stop, target float64
	}{
		{"NaN ATR", math.NaN(), 2, 1}, {"infinite ATR", math.Inf(1), 2, 1}, {"zero ATR", 0, 2, 1}, {"negative ATR", -1, 2, 1},
		{"NaN stop", 1, math.NaN(), 1}, {"infinite stop", 1, math.Inf(1), 1}, {"zero stop", 1, 0, 1}, {"negative stop", 1, -2, 1},
		{"stop product overflow", 10, 1e308, 1}, {"stop rounds to entry", 10, 1e-300, 1}, {"stop product underflow", 1e-300, 1e-300, 1},
		{"target product overflow", 10, 2, 1e308}, {"target rounds to entry", 10, 2, 1e-300}, {"NaN target", 1, 2, math.NaN()}, {"infinite target", 1, 2, math.Inf(1)}, {"negative target", 1, 2, -1},
	}
	for _, s := range []side{sideLong, sideShort} {
		for _, tc := range tests {
			t.Run(s.String()+"/"+tc.name, func(t *testing.T) {
				b := donchianBracketBroker(s, tc.atr, tc.stop, tc.target)
				b.onDonchianBreakoutBar(1)
				if b.hasPosition || len(b.pendingOrders) != 0 || len(b.trades) != 0 {
					t.Fatalf("unusable brackets entered: position=%+v trades=%+v", b.position, b.trades)
				}
			})
		}
	}
	for _, s := range []side{sideLong, sideShort} {
		for _, close := range []float64{math.NaN(), math.Inf(int(s))} {
			b := donchianBracketBroker(s, 1, 2, 1)
			b.series.C[1] = close
			b.onDonchianBreakoutBar(1)
			if b.hasPosition || len(b.pendingOrders) != 0 {
				t.Fatalf("nonfinite close %v entered", close)
			}
		}
	}
}

func TestDonchianBreakoutPreservesFiniteBracketArithmetic(t *testing.T) {
	for _, s := range []side{sideLong, sideShort} {
		for _, target := range []float64{0, 1, 2.5} {
			b := donchianBracketBroker(s, 0.3, 2, target)
			b.costs.Slippage = 0.1
			b.onDonchianBreakoutBar(1)
			if !b.hasPosition {
				t.Fatal("ordinary breakout refused")
			}
			sign := float64(s)
			stopATR, atr := b.params.DonchianBreakout.StopATR, b.cols.ATR[1]
			stop := b.series.C[1] - sign*stopATR*atr
			if b.position.SL != stop || b.position.InitialSL != stop {
				t.Fatalf("stop changed: %.17g != %.17g", b.position.SL, stop)
			}
			entry := 100 + sign*0.1
			if b.position.Entry != entry || b.position.Size != 200/math.Abs(entry-stop) {
				t.Fatalf("fill/risk sizing changed: %+v", b.position)
			}
			if target == 0 {
				if !b.position.NoTarget {
					t.Fatal("zero target must disable target")
				}
			} else {
				want := b.series.C[1] + sign*stopATR*atr*target
				if b.position.NoTarget || b.position.TP != want || b.position.InitialTP != want {
					t.Fatalf("target arithmetic changed: %+v want %.17g", b.position, want)
				}
			}
		}
	}
}

func mirrorDonchianFixture(f *RunFixture) {
	for i := range f.Bars {
		b := &f.Bars[i]
		b.O, b.H, b.L, b.C = 4000-b.O, 4000-b.L, 4000-b.H, 4000-b.C
	}
}

func TestDonchianBreakoutPartialTargetsAndCashflows(t *testing.T) {
	for _, s := range []side{sideLong, sideShort} {
		for _, target := range []string{"absent", "0R", "1R"} {
			t.Run(s.String()+"/"+target, func(t *testing.T) {
				fixture, source := donchianValidationFixture(t)
				if target == "absent" {
					source = strings.Replace(source, "target {\n  target 1R\n}\n", "", 1)
				} else {
					source = strings.Replace(source, "target 1R", "target "+target, 1)
				}
				source = strings.Replace(source, "execution {", "management {\n  partial 50% at 0.25R\n}\nexecution {", 1)
				if s == sideShort {
					mirrorDonchianFixture(&fixture)
					source = strings.Replace(source, "side long only", "side short only", 1)
				}
				result, err := RunFixtureCase(fixture, source)
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Trades) != 2 {
					t.Fatalf("trades = %+v", result.Trades)
				}
				for i, trade := range result.Trades {
					raw, err := json.Marshal(RunResult{Trades: []Trade{trade}})
					if err != nil {
						t.Fatal(err)
					}
					var envelope struct {
						Trades []map[string]any `json:"trades"`
					}
					if err = json.Unmarshal(raw, &envelope); err != nil {
						t.Fatal(err)
					}
					fields := envelope.Trades[0]
					if target == "1R" {
						want := 2010 + float64(s)*20
						if s == sideShort {
							want = 1970
						}
						if fields["tp"] != want || fields["initialTp"] != want || trade.NoTarget {
							t.Fatalf("explicit target metadata: %s", raw)
						}
					} else {
						if fields["tp"] != nil || fields["initialTp"] != nil || !trade.NoTarget {
							t.Fatalf("no-target metadata: %s", raw)
						}
					}
					if trade.Size != 5 || trade.Partial != (i == 0) {
						t.Fatalf("partial sizing/marker changed: %+v", trade)
					}
				}
				first, last := result.Trades[0], result.Trades[1]
				if first.PnL != 50 || first.Points != 10 || first.Reason != "partial" || first.ExitIndex != 21 {
					t.Fatalf("partial cashflow = %+v", first)
				}
				wantPnL, wantReason := 40.0, ReasonEndOfTest
				if target == "1R" {
					wantPnL, wantReason = 100, "tp"
				}
				if last.PnL != wantPnL || last.Reason != wantReason {
					t.Fatalf("final cashflow = %+v", last)
				}
			})
		}
	}
}

func TestDonchianBreakoutNextOpenAndStopGapRemainShared(t *testing.T) {
	for _, s := range []side{sideLong, sideShort} {
		t.Run(s.String()+"/next-open", func(t *testing.T) {
			fixture, source := donchianValidationFixture(t)
			fixture.Costs.FillOn = "open"
			fixture.Bars[21].O = 2015
			if s == sideShort {
				mirrorDonchianFixture(&fixture)
				source = strings.Replace(source, "side long only", "side short only", 1)
			}
			result, err := RunFixtureCase(fixture, source)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Trades) != 1 {
				t.Fatalf("trades=%+v", result.Trades)
			}
			tr := result.Trades[0]
			entry, stop, target := 2015.0, 1990.0, 2030.0
			if s == sideShort {
				entry, stop, target = 1985, 2010, 1970
			}
			if tr.EntryIndex != 21 || tr.Entry != entry || tr.InitialSL != stop || tr.InitialTP != target || tr.Size != 8 {
				t.Fatalf("next-open semantics changed: %+v", tr)
			}
		})
		t.Run(s.String()+"/stop-gap", func(t *testing.T) {
			fixture, source := donchianValidationFixture(t)
			fixture.Bars = fixture.Bars[:22]
			fixture.Bars[21].O = 1980
			fixture.Bars[21].H = 1985
			fixture.Bars[21].L = 1975
			fixture.Bars[21].C = 1980
			if s == sideShort {
				mirrorDonchianFixture(&fixture)
				source = strings.Replace(source, "side long only", "side short only", 1)
			}
			result, err := RunFixtureCase(fixture, source)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Trades) != 1 {
				t.Fatalf("trades=%+v", result.Trades)
			}
			tr := result.Trades[0]
			want := 1980.0
			if s == sideShort {
				want = 2020
			}
			if tr.ExitIndex != 21 || tr.Exit != want || tr.Reason != "sl" {
				t.Fatalf("stop-gap semantics changed: %+v", tr)
			}
		})
	}
}

func TestDonchianBreakoutWarmupAndCooldownBoundaries(t *testing.T) {
	for _, length := range []int{20, 21, 27} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			fixture, source := donchianValidationFixture(t)
			source = strings.Replace(source, "channel 3 candles", "channel 3 candles\n  trend ema "+strconv.Itoa(length), 1)
			result, err := RunFixtureCase(fixture, source)
			if err != nil {
				t.Fatal(err)
			}
			if length == 27 {
				if len(result.Trades) != 0 {
					t.Fatalf("warmup disabled: %+v", result.Trades)
				}
			} else if len(result.Trades) != 1 || result.Trades[0].EntryIndex != length {
				t.Fatalf("warmup boundary = %+v", result.Trades)
			}
		})
	}
	for _, s := range []side{sideLong, sideShort} {
		b := donchianBracketBroker(s, 1, 2, 0)
		b.params.CooldownBars = 2
		b.trades = []Trade{{Meta: TradeMeta{"signalIndex": float64(0)}}}
		b.onDonchianBreakoutBar(1)
		if b.hasPosition {
			t.Fatal("cooldown admitted early")
		}
		b.series.H[1], b.series.L[1] = 98.5, 97.5
		if s == sideShort {
			b.series.H[1], b.series.L[1] = 102.5, 101.5
		}
		b.onDonchianBreakoutBar(2)
		if !b.hasPosition {
			t.Fatal("exact cooldown boundary refused")
		}
	}
}

func TestDonchianBreakoutRejectsPriceAndReconstructedDistanceOverflow(t *testing.T) {
	// Each operand is finite. Subtracting the rounded finite stop (or target)
	// can itself overflow even when the original product is MaxFloat64.
	for _, s := range []side{sideLong, sideShort} {
		sign := float64(s)
		cases := []struct {
			name                string
			entry, stop, target float64
			finiteBracket       bool
		}{
			{"stop price overflow", -sign * 1e308, 1e308, 0, false},
			{"target price overflow", sign * 1e308, 1e307, 10, false},
			{"stop distance overflow", sign * 5.308678458123087e307, math.MaxFloat64, 0, true},
			{"target distance overflow", -sign * 5.308678458123087e307, math.Ldexp(1, 1022), math.Nextafter(4, 0), true},
		}
		for _, tc := range cases {
			t.Run(s.String()+"/"+tc.name, func(t *testing.T) {
				b := donchianBracketBroker(s, 1, tc.stop, tc.target)
				b.series.C[1] = tc.entry
				b.series.H[0], b.series.L[0] = tc.entry-sign*1e307, tc.entry-sign*1e307
				stop := tc.entry - sign*tc.stop
				bracket, distance := stop, sign*(tc.entry-stop)
				if tc.target > 0 {
					bracket = tc.entry + sign*tc.stop*tc.target
					distance = sign * (bracket - tc.entry)
				}
				if isFinite(bracket) != tc.finiteBracket || isFinite(distance) {
					t.Fatalf("overflow test precondition failed: bracket=%v distance=%v", bracket, distance)
				}
				b.onDonchianBreakoutBar(1)
				if b.hasPosition || len(b.pendingOrders) != 0 || len(b.trades) != 0 {
					t.Fatalf("overflow admitted: %+v", b.position)
				}
			})
		}
	}
}
