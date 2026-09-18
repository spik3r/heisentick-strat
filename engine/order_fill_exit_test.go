package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestProductionLoopResolvesQueuedOrderExitsOnFillBar(t *testing.T) {
	tests := []struct {
		name       string
		orderType  string
		high       float64
		low        float64
		wantEntry  float64
		wantExit   float64
		wantReason string
	}{
		{name: "next-open stop", orderType: "nextOpen", high: 103, low: 94, wantEntry: 101, wantExit: 94, wantReason: "sl"},
		{name: "next-open target", orderType: "nextOpen", high: 106, low: 99, wantEntry: 101, wantExit: 104, wantReason: "tp"},
		{name: "next-open stop precedes target", orderType: "nextOpen", high: 106, low: 94, wantEntry: 101, wantExit: 94, wantReason: "sl"},
		{name: "limit stop", orderType: "limit", high: 103, low: 94, wantEntry: 100, wantExit: 94, wantReason: "sl"},
		{name: "limit target", orderType: "limit", high: 106, low: 99, wantEntry: 100, wantExit: 104, wantReason: "tp"},
		{name: "limit stop precedes target", orderType: "limit", high: 106, low: 94, wantEntry: 100, wantExit: 94, wantReason: "sl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := fillBarExitBroker(tt.high, tt.low)
			setup := setupPlan{
				Side:   sideLong,
				Stop:   95,
				Target: 105,
				Tag:    "fill-bar-exit",
				Meta:   TradeMeta{"setup": "fillBarExit"},
			}
			switch tt.orderType {
			case "nextOpen":
				b.costs.FillOn = "nextOpen"
				b.enterSetup(0, setup)
				if len(b.pendingOrders) != 1 || len(b.limitOrders) != 0 {
					t.Fatalf("queued next-open state = pending %d limits %d, want 1/0", len(b.pendingOrders), len(b.limitOrders))
				}
			case "limit":
				b.enterLimit(0, 100, setup, 5)
				if len(b.pendingOrders) != 0 || len(b.limitOrders) != 1 {
					t.Fatalf("queued limit state = pending %d limits %d, want 0/1", len(b.pendingOrders), len(b.limitOrders))
				}
			default:
				t.Fatalf("unknown order type %q", tt.orderType)
			}

			trades := b.run()
			if len(trades) != 1 {
				t.Fatalf("trades = %+v, want exactly one fill-bar exit", trades)
			}
			trade := trades[0]
			if trade.Entry != tt.wantEntry || trade.EntryIndex != 1 {
				t.Fatalf("fill = %v at %d, want %v at 1", trade.Entry, trade.EntryIndex, tt.wantEntry)
			}
			if trade.Reason != tt.wantReason || trade.Exit != tt.wantExit || trade.ExitIndex != 1 {
				t.Fatalf("exit = %s at %v/%d, want %s at %v/1", trade.Reason, trade.Exit, trade.ExitIndex, tt.wantReason, tt.wantExit)
			}
			if b.hasPosition || len(b.pendingOrders) != 0 || len(b.limitOrders) != 0 || len(b.trades) != 1 {
				t.Fatalf("post-run state = position %v pending %d limits %d trades %d, want false/0/0/1",
					b.hasPosition, len(b.pendingOrders), len(b.limitOrders), len(b.trades))
			}
		})
	}
}

func fillBarExitBroker(high, low float64) broker {
	const hourMS = 60 * 60 * 1000
	series := marketdata.Series{
		T: []float64{
			2 * float64(hourMS),
			3 * float64(hourMS),
		},
		O: []float64{100, 100},
		H: []float64{101, high},
		L: []float64{99, low},
		C: []float64{100, 100},
		V: []float64{1, 1},
	}
	return broker{
		series: series,
		cols: contextcols.Columns{
			ER:     make([]float64, series.Len()),
			Regime: make([]int8, series.Len()),
		},
		costs: Costs{Slippage: 1},
		params: flagParams{
			SetupType:      "rangeBreakFake",
			UseMidWindow:   true,
			AdmitMidWindow: true,
			MaxMovementER:  1,
			RiskUSD:        100,
		},
	}
}
