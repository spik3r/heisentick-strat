package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// fixedATRStopBroker builds a two-signal-bar series whose bar 1 closes at 100
// after a 2% move from bar 0, so a 1-candle lookback with a 1% neutral zone
// signals at bar 1 only: long when bar 0 closes at 98, short when it closes at
// 102. Bar 2 exists for next-open fills; its open is set by the caller.
func fixedATRStopBroker(s side, multiple, minATR, maxATR, atr float64) broker {
	first := 98.0
	if s == sideShort {
		first = 102
	}
	bars := []marketdata.Bar{
		{T: 0, O: first, H: first + 0.5, L: first - 0.5, C: first, V: 1},
		{T: 1, O: 100, H: 100.5, L: 99.5, C: 100, V: 1},
		{T: 2, O: 100, H: 100.5, L: 99.5, C: 100, V: 1},
	}
	return broker{
		series:   marketdata.SeriesFromBars(bars),
		cols:     contextcols.Columns{ATR: []float64{atr, atr, atr}, ER: make([]float64, len(bars))},
		htfTrend: []int8{trendFlat, trendFlat, trendFlat},
		costs:    Costs{FillOn: "close", StartEquity: 10000},
		params: flagParams{
			SetupType:           string(dsl.FamilyPriceMomentum),
			UseHTFBias:          true,
			UseAsiaWindow:       true,
			UseMidWindow:        true,
			UseLondonWindow:     true,
			UseNYWindow:         true,
			AllowLong:           true,
			AllowShort:          true,
			StopLookbackCandles: 1,
			StopBufferATR:       0.25,
			MinStopATR:          minATR,
			MaxStopATR:          maxATR,
			TargetR:             2,
			CooldownBars:        3,
			RiskUSD:             200,
			MaxMovementER:       1.1,
			PriceMomentum: priceMomentumParams{
				LookbackBars: 1,
				ThresholdPct: 1,
				StopATR:      multiple,
			},
		},
	}
}

// Hand-derived from the reviewed arithmetic, close 100 and ATR 0.3, on both
// sides: 100 -/+ 0.3*2 rounds to 99.400000000000006 / 100.59999999999999,
// whose distance from 100 is 0.59999999999999432, below 0.3*2 =
// 0.59999999999999998; 100 -/+ 0.3*3 rounds to 99.099999999999994 /
// 100.90000000000001, distance 0.90000000000000568, above 0.3*3 =
// 0.89999999999999991. Reconstructing the distance after the price
// subtraction therefore rejects multipliers on, and one ulp inside, an
// inclusive `stop size` bound. The operands are runtime values so the Go
// compiler cannot fold them exactly.
var reconstructionEntry, reconstructionATR = 100.0, 0.3

func TestPriceMomentumFixedATRStopReconstructionCrossesInclusiveBounds(t *testing.T) {
	entry, atr := reconstructionEntry, reconstructionATR
	for _, s := range []float64{1, -1} {
		if got, want := math.Abs(entry-(entry-s*atr*2)), 0.59999999999999432; got != want || !(got < atr*2) {
			t.Fatalf("side %v N=2 distance = %.17g, want %.17g below %.17g", s, got, want, atr*2)
		}
		if got, want := math.Abs(entry-(entry-s*atr*3)), 0.90000000000000568; got != want || !(got > atr*3) {
			t.Fatalf("side %v N=3 distance = %.17g, want %.17g above %.17g", s, got, want, atr*3)
		}
	}
}

func TestPriceMomentumFixedATRStopHonoursInclusiveBounds(t *testing.T) {
	below2 := math.Nextafter(2, 0)
	above2 := math.Nextafter(2, 3)
	below3 := math.Nextafter(3, 0)
	above3 := math.Nextafter(3, 4)
	cases := []struct {
		name      string
		multiple  float64
		min, max  float64
		wantEntry bool
	}{
		{"exact minimum", 2, 2, 10, true},
		{"just inside minimum", above2, 2, 10, true},
		{"just outside minimum", below2, 2, 10, false},
		{"exact maximum", 3, 0, 3, true},
		{"just inside maximum", below3, 0, 3, true},
		{"just outside maximum", above3, 0, 3, false},
		{"single-point range", 2, 2, 2, true},
	}
	for _, s := range []side{sideLong, sideShort} {
		for _, tc := range cases {
			t.Run(s.String()+"/"+tc.name, func(t *testing.T) {
				b := fixedATRStopBroker(s, tc.multiple, tc.min, tc.max, 0.3)
				b.onPriceMomentumBar(1)
				if b.hasPosition != tc.wantEntry {
					t.Fatalf("entry = %v, want %v", b.hasPosition, tc.wantEntry)
				}
				if !tc.wantEntry {
					if b.hasPriceMomentumEntry {
						t.Fatal("rejected entry seeded the cooldown")
					}
					return
				}
				// Anchored to the signal close, as documented: close -/+ ATR*N.
				wantSL := 100 - float64(s)*0.3*tc.multiple
				if b.position.SL != wantSL || b.position.InitialSL != wantSL {
					t.Fatalf("stop = %.17g, want %.17g", b.position.SL, wantSL)
				}
				if b.position.Side != s || b.position.Entry != 100 {
					t.Fatalf("position = %+v", b.position)
				}
				risk := math.Abs(100 - wantSL)
				if wantTP := 100 + float64(s)*risk*2; b.position.TP != wantTP {
					t.Fatalf("target = %.17g, want %.17g", b.position.TP, wantTP)
				}
				if wantSize := 200 / risk; b.position.Size != wantSize {
					t.Fatalf("size = %.17g, want %.17g", b.position.Size, wantSize)
				}
			})
		}
	}
}

func TestPriceMomentumFixedATRStopRequiresUsableATR(t *testing.T) {
	for name, atr := range map[string]float64{
		"warm-up NaN": math.NaN(),
		"zero":        0,
		"negative":    -0.3,
		"+Inf":        math.Inf(1),
	} {
		for _, s := range []side{sideLong, sideShort} {
			t.Run(name+"/"+s.String(), func(t *testing.T) {
				b := fixedATRStopBroker(s, 2, 0, math.Inf(1), atr)
				b.onPriceMomentumBar(1)
				if b.hasPosition || b.hasPriceMomentumEntry {
					t.Fatalf("entry admitted with ATR %v", atr)
				}
			})
		}
	}
}

func TestPriceMomentumFixedATRStopRejectsNonFiniteStopPrice(t *testing.T) {
	// ATR*N overflows to +Inf, so the stop price is not finite even though the
	// multiplier itself sits inside unbounded `stop size` limits.
	for _, s := range []side{sideLong, sideShort} {
		b := fixedATRStopBroker(s, 4, 0, math.Inf(1), math.MaxFloat64/2)
		b.onPriceMomentumBar(1)
		if b.hasPosition {
			t.Fatalf("%s entry admitted with non-finite stop %v", s, b.position.SL)
		}
	}
}

func TestPriceMomentumWithoutFixedATRStopKeepsRecentExtremeStop(t *testing.T) {
	for _, s := range []side{sideLong, sideShort} {
		b := fixedATRStopBroker(s, 0, 0.5, 10, 1)
		b.onPriceMomentumBar(1)
		if !b.hasPosition {
			t.Fatalf("%s default-stop entry rejected", s)
		}
		// Recent 1-candle extreme of bar 1 (low 99.5 / high 100.5) plus the
		// 0.25 ATR buffer: 99.25 for longs, 100.75 for shorts.
		want := 99.25
		if s == sideShort {
			want = 100.75
		}
		if b.position.SL != want {
			t.Fatalf("%s default stop = %v, want %v", s, b.position.SL, want)
		}
	}
}

func TestPriceMomentumFixedATRStopNextOpenFillKeepsSignalStopAndSizesFromFill(t *testing.T) {
	for _, s := range []side{sideLong, sideShort} {
		t.Run(s.String(), func(t *testing.T) {
			b := fixedATRStopBroker(s, 2, 0, 3, 0.3)
			b.costs = Costs{FillOn: "nextOpen", StartEquity: 10000, Slippage: 0.05}
			// Gap of 1.0 in the trade's favour direction at the next open.
			b.series.O[2] = 100 + float64(s)*1
			b.onPriceMomentumBar(1)
			if b.hasPosition || len(b.pendingOrders) != 1 {
				t.Fatalf("next-open order not queued: position=%v pending=%d", b.hasPosition, len(b.pendingOrders))
			}
			b.fillPending(2)
			wantSL := 100 - float64(s)*0.3*2
			wantFill := b.series.O[2] + float64(s)*0.05
			if !b.hasPosition || b.position.Entry != wantFill || b.position.EntryIndex != 2 {
				t.Fatalf("fill = %+v, want entry %.17g at bar 2", b.position, wantFill)
			}
			if b.position.SL != wantSL {
				t.Fatalf("stop moved with the fill: %.17g, want signal-close stop %.17g", b.position.SL, wantSL)
			}
			if wantSize := 200 / math.Abs(wantFill-wantSL); b.position.Size != wantSize {
				t.Fatalf("size = %.17g, want fill-adjusted %.17g", b.position.Size, wantSize)
			}
			if !b.position.GapAwareStop {
				t.Fatal("next-open fill must carry the gap-aware stop")
			}
		})
	}
}

func TestPriceMomentumFixedATRStopNextOpenGapThroughStopExitsAtOpen(t *testing.T) {
	for _, s := range []side{sideLong, sideShort} {
		t.Run(s.String(), func(t *testing.T) {
			b := fixedATRStopBroker(s, 2, 0, 3, 0.3)
			b.costs = Costs{FillOn: "nextOpen", StartEquity: 10000}
			// Stop is 99.4 (long) / 100.6 (short); the next bar opens 1.0 beyond it.
			b.series.O[2] = 100 - float64(s)*1.6
			b.onPriceMomentumBar(1)
			b.fillPending(2)
			b.resolveIntrabarExit(2)
			if b.hasPosition || len(b.trades) != 1 {
				t.Fatalf("gap through the stop did not exit: position=%v trades=%d", b.hasPosition, len(b.trades))
			}
			trade := b.trades[0]
			if trade.Reason != "sl" || trade.Exit != b.series.O[2] || trade.ExitIndex != 2 {
				t.Fatalf("gap exit = %+v, want sl at the open %v", trade, b.series.O[2])
			}
		})
	}
}

func TestPriceMomentumFixedATRStopCloseFillWithSlippageSizesFromFill(t *testing.T) {
	for _, s := range []side{sideLong, sideShort} {
		b := fixedATRStopBroker(s, 2, 0, 3, 0.3)
		b.costs = Costs{FillOn: "close", StartEquity: 10000, Slippage: 0.05}
		b.onPriceMomentumBar(1)
		wantSL := 100 - float64(s)*0.3*2
		wantFill := 100 + float64(s)*0.05
		if !b.hasPosition || b.position.Entry != wantFill || b.position.SL != wantSL {
			t.Fatalf("%s close fill = %+v, want entry %.17g stop %.17g", s, b.position, wantFill, wantSL)
		}
		if wantSize := 200 / math.Abs(wantFill-wantSL); b.position.Size != wantSize {
			t.Fatalf("%s size = %.17g, want fill-adjusted %.17g", s, b.position.Size, wantSize)
		}
		// The target stays measured from the signal close and its risk.
		if wantTP := 100 + float64(s)*math.Abs(100-wantSL)*2; b.position.TP != wantTP {
			t.Fatalf("%s target = %.17g, want %.17g", s, b.position.TP, wantTP)
		}
	}
}
