package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestPriceMomentumDirectionBoundariesWarmupAndInvalidDenominator(t *testing.T) {
	closes := []float64{100, 100, 101, 99, 100}
	if _, _, valid := priceMomentumDirection(closes, 1, 2, 1); valid {
		t.Fatal("signal became valid before N + 1 closes")
	}
	if got, roc, valid := priceMomentumDirection(closes, 2, 2, 1); !valid || got != sideLong || math.Abs(roc-1) > 1e-12 {
		t.Fatalf("long boundary = (%v, %v, %v)", got, roc, valid)
	}
	if got, roc, valid := priceMomentumDirection(closes, 3, 2, 1); !valid || got != sideShort || math.Abs(roc+1) > 1e-12 {
		t.Fatalf("short boundary = (%v, %v, %v)", got, roc, valid)
	}
	if got, _, valid := priceMomentumDirection(closes, 4, 2, 1); !valid || got != 0 {
		t.Fatalf("neutral = (%v, %v)", got, valid)
	}
	if _, _, valid := priceMomentumDirection([]float64{0, 1}, 1, 1, 1); valid {
		t.Fatal("non-positive denominator must fail closed")
	}
}

func priceMomentumBroker(htf []int8) broker {
	closes := []float64{100, 102, 104, 106, 108}
	bars := make([]marketdata.Bar, len(closes))
	atr := make([]float64, len(closes))
	er := make([]float64, len(closes))
	for i, close := range closes {
		bars[i] = marketdata.Bar{T: float64(i), O: close, H: close + 0.5, L: close - 0.5, C: close, V: 1}
		atr[i] = 1
	}
	return broker{
		series:   marketdata.SeriesFromBars(bars),
		cols:     contextcols.Columns{ATR: atr, ER: er},
		htfTrend: htf,
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
			StopBufferATR:       0,
			MinStopATR:          0,
			MaxStopATR:          10,
			TargetR:             1,
			CooldownBars:        3,
			RiskUSD:             200,
			MaxMovementER:       1.1,
			PriceMomentum: priceMomentumParams{
				LookbackBars: 1,
				ThresholdPct: 1,
			},
		},
	}
}

func TestPriceMomentumHTFGuardAndEntryRelativeCooldown(t *testing.T) {
	for name, htf := range map[string][]int8{
		"missing":  nil,
		"opposite": {trendFlat, trendDown, trendDown, trendDown, trendDown},
	} {
		t.Run(name, func(t *testing.T) {
			b := priceMomentumBroker(htf)
			b.onPriceMomentumBar(1)
			if b.hasPosition {
				t.Fatal("guarded entry was admitted")
			}
		})
	}

	b := priceMomentumBroker([]int8{trendFlat, trendFlat, trendFlat, trendFlat, trendFlat})
	b.onPriceMomentumBar(1)
	if !b.hasPosition || b.priceMomentumLastEntry != 1 {
		t.Fatalf("flat HTF did not permit entry: position=%v last=%d", b.hasPosition, b.priceMomentumLastEntry)
	}
	b.hasPosition = false
	b.position = position{}
	b.onPriceMomentumBar(3)
	if b.hasPosition {
		t.Fatal("cooldown admitted entry before equality boundary")
	}
	b.onPriceMomentumBar(4)
	if !b.hasPosition || b.priceMomentumLastEntry != 4 {
		t.Fatalf("cooldown rejected equality boundary: position=%v last=%d", b.hasPosition, b.priceMomentumLastEntry)
	}
}

func TestPriceMomentumCapacityRejectionDoesNotSeedCooldown(t *testing.T) {
	b := priceMomentumBroker([]int8{trendFlat, trendFlat, trendFlat, trendFlat, trendFlat})
	b.pendingOrders = append(b.pendingOrders, order{Side: sideLong, Index: 2})

	b.onPriceMomentumBar(1)

	if b.hasPriceMomentumEntry || b.priceMomentumLastEntry != 0 {
		t.Fatalf("capacity rejection seeded cooldown: has=%v last=%d", b.hasPriceMomentumEntry, b.priceMomentumLastEntry)
	}
}
