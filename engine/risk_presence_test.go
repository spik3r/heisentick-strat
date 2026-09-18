package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestOpenPositionRiskPresenceSizing(t *testing.T) {
	tests := []struct {
		name     string
		order    order
		slip     float64
		wantSize float64
	}{
		{name: "explicit zero size overrides present risk", order: order{SL: 90, Size: 0, HasSize: true, RiskUSD: 100, HasRisk: true}, wantSize: 0},
		{name: "explicit fractional size overrides present risk", order: order{SL: 90, Size: 0.25, HasSize: true, RiskUSD: 100, HasRisk: true}, wantSize: 0.25},
		{name: "explicit negative size overrides present risk", order: order{SL: 90, Size: -2, HasSize: true, RiskUSD: 100, HasRisk: true}, wantSize: -2},
		{name: "omitted size uses present risk", order: order{SL: 90, RiskUSD: 100, HasRisk: true}, wantSize: 10},
		{name: "omitted size and risk default to one", order: order{SL: 90}, wantSize: 1},
		{name: "explicit zero risk remains zero", order: order{SL: 90, HasRisk: true}, wantSize: 0},
		{name: "zero distance remains zero", order: order{SL: 100, RiskUSD: 100, HasRisk: true}, wantSize: 0},
		{name: "slipped fill sets risk distance", order: order{SL: 90, RiskUSD: 110, HasRisk: true}, slip: 1, wantSize: 10},
		{name: "negative risk remains negative", order: order{SL: 90, RiskUSD: -100, HasRisk: true}, wantSize: -10},
		{name: "legacy nonzero size wins without presence bit", order: order{SL: 90, Size: 2, RiskUSD: 100, HasRisk: true}, wantSize: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := broker{
				series: marketdata.Series{T: []float64{0}},
				costs:  Costs{Slippage: tt.slip},
			}
			b.openPosition(sideLong, 100, tt.order, 0)
			if b.position.Size != tt.wantSize {
				t.Fatalf("size = %v, want %v", b.position.Size, tt.wantSize)
			}
		})
	}
}

func TestOpenPositionSizePresenceFlowsThroughEndOfDataCosts(t *testing.T) {
	tests := []struct {
		name     string
		order    order
		wantSize float64
	}{
		{name: "explicit zero", order: order{SL: 90, TP: 150, HasSize: true, RiskUSD: 105, HasRisk: true}, wantSize: 0},
		{name: "explicit fractional", order: order{SL: 90, TP: 150, Size: 0.25, HasSize: true, RiskUSD: 105, HasRisk: true}, wantSize: 0.25},
		{name: "explicit negative", order: order{SL: 90, TP: 150, Size: -2, HasSize: true, RiskUSD: 105, HasRisk: true}, wantSize: -2},
		{name: "risk sized", order: order{SL: 90, TP: 150, RiskUSD: 105, HasRisk: true}, wantSize: 10},
		{name: "default sized", order: order{SL: 90, TP: 150}, wantSize: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := broker{
				series: marketdata.Series{
					T: []float64{1000}, O: []float64{100}, H: []float64{105},
					L: []float64{99}, C: []float64{105}, V: []float64{1},
				},
				costs: Costs{Slippage: 0.5, FeePerUnit: 0.2},
			}
			b.openPosition(sideLong, 100, tt.order, 0)

			trades := b.run()
			if len(trades) != 1 || b.hasPosition {
				t.Fatalf("end-of-data trades=%+v live=%v, want one closed position", trades, b.hasPosition)
			}
			trade := trades[0]
			wantPnL := 3.8 * tt.wantSize
			wantRealized := 3.6 * tt.wantSize
			if trade.Reason != "eod" || trade.Partial || trade.Size != tt.wantSize ||
				trade.Entry != 100.5 || trade.Exit != 104.5 || trade.Points != 4 ||
				math.Abs(trade.PnL-wantPnL) > 1e-12 {
				t.Fatalf("end-of-data trade = %+v, want size=%v entry=100.5 exit=104.5 points=4 pnl=%v", trade, tt.wantSize, wantPnL)
			}
			if math.Abs(b.realized-wantRealized) > 1e-12 {
				t.Fatalf("realized = %v, want entry and exit cost accounting %v", b.realized, wantRealized)
			}
		})
	}
}

func TestProductionOrdersPreserveRiskPresenceAcrossFills(t *testing.T) {
	t.Run("flag next-open queue", func(t *testing.T) {
		b := riskPresenceBroker(
			[]float64{100, 105},
			[]float64{100, 105},
			[]float64{100, 105},
		)
		b.costs = Costs{FillOn: "nextOpen", Slippage: 1}
		b.params.RiskUSD = 0

		b.enter(0, sideLong, flagSetup{Stop: 90, Target: 120})
		if len(b.pendingOrders) != 1 || !b.pendingOrders[0].HasRisk || b.pendingOrders[0].HasSize {
			t.Fatalf("pending order = %+v, want risk presence without fixed-size presence", b.pendingOrders)
		}
		b.fillPending(1)
		if !b.hasPosition || b.position.Entry != 106 || b.position.Size != 0 {
			t.Fatalf("next-open position = %+v, want entry 106 and zero size", b.position)
		}
	})

	t.Run("typed next-open queue", func(t *testing.T) {
		b := riskPresenceBroker(
			[]float64{100, 105},
			[]float64{100, 105},
			[]float64{100, 105},
		)
		b.costs = Costs{FillOn: "nextOpen"}
		b.params.RiskUSD = 100

		b.enterSetup(0, setupPlan{Side: sideLong, Stop: 90, Target: 120})
		if len(b.pendingOrders) != 1 || !b.pendingOrders[0].HasRisk || b.pendingOrders[0].HasSize {
			t.Fatalf("typed pending order = %+v, want risk presence without fixed-size presence", b.pendingOrders)
		}
	})

	t.Run("typed close fill preserves zero risk", func(t *testing.T) {
		b := riskPresenceBroker(
			[]float64{100},
			[]float64{100},
			[]float64{100},
		)
		b.costs = Costs{Slippage: 1}
		b.params.RiskUSD = 0

		b.enterSetup(0, setupPlan{Side: sideLong, Stop: 90, Target: 120})
		if !b.hasPosition || b.position.Entry != 101 || b.position.Size != 0 {
			t.Fatalf("close-fill position = %+v, want entry 101 and zero size", b.position)
		}
	})

	for _, tt := range []struct {
		name     string
		open     float64
		wantFill float64
		wantSize float64
	}{
		{name: "exact limit", open: 100, wantFill: 103, wantSize: 100.0 / 7.0},
		{name: "better gap limit", open: 105, wantFill: 105, wantSize: 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := riskPresenceBroker(
				[]float64{100, tt.open},
				[]float64{101, 107},
				[]float64{99, 99},
			)
			b.costs = Costs{Slippage: 0.5}
			b.params.RiskUSD = 100

			b.enterLimit(0, 103, setupPlan{Side: sideShort, Stop: 110, Target: 90}, 5)
			if len(b.limitOrders) != 1 || !b.limitOrders[0].HasRisk || b.limitOrders[0].HasSize {
				t.Fatalf("limit order = %+v, want risk presence without fixed-size presence", b.limitOrders)
			}
			b.fillLimits(1)
			if !b.hasPosition || b.position.Entry != tt.wantFill || math.Abs(b.position.Size-tt.wantSize) > 1e-12 {
				t.Fatalf("limit position = %+v, want entry %v size %v", b.position, tt.wantFill, tt.wantSize)
			}
		})
	}
}

func riskPresenceBroker(opens, highs, lows []float64) broker {
	n := len(opens)
	times := make([]float64, n)
	closes := make([]float64, n)
	for i := range times {
		times[i] = float64(i+2) * float64(contextHourMS)
		closes[i] = opens[i]
	}
	return broker{
		series: marketdata.Series{T: times, O: opens, H: highs, L: lows, C: closes, V: make([]float64, n)},
		cols: contextcols.Columns{
			ER:     make([]float64, n),
			Regime: make([]int8, n),
		},
		params: flagParams{
			UseMidWindow:   true,
			AdmitMidWindow: true,
			MaxMovementER:  1,
		},
	}
}
