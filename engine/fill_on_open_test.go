package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestFillOnOpenMarketOrderUsesNextBarOpenAndPreservesNextOpen(t *testing.T) {
	for _, fillOn := range []string{"open", "nextOpen"} {
		t.Run(fillOn, func(t *testing.T) {
			b := fillBarExitBroker(106, 99)
			b.costs.FillOn = fillOn
			setup := setupPlan{
				Side: sideLong, Stop: 95, Target: 105,
				Tag: "fill-on-open", Meta: TradeMeta{"setup": "fill-on-open"},
			}
			if !b.enterSetup(0, setup) {
				t.Fatal("market order was not admitted")
			}
			if len(b.pendingOrders) != 1 || b.pendingOrders[0].Index != 1 || !b.pendingOrders[0].GapAwareStop {
				t.Fatalf("pending order = %+v, want one protected order at bar 1", b.pendingOrders)
			}

			trades := b.run()
			if len(trades) != 1 || trades[0].EntryIndex != 1 || trades[0].Entry != 101 {
				t.Fatalf("trades = %+v, want one next-open entry at 101 on bar 1", trades)
			}
		})
	}
}

func TestFillOnOpenKeepsSignalProtectionAndSupportsFillRelativeDistances(t *testing.T) {
	series := marketdata.Series{
		T: []float64{0, 1}, O: []float64{100, 110}, H: []float64{101, 111},
		L: []float64{99, 109}, C: []float64{100, 110}, V: []float64{1, 1},
	}
	b := broker{
		series: series,
		costs:  Costs{FillOn: "open"},
		cols:   contextcols.Columns{ER: []float64{0, 0}, Regime: []int8{0, 0}},
		pendingOrders: []order{{
			Side: sideLong, Index: 1, SL: 95, TP: 105,
			Size: 1, HasSize: true,
		}},
	}
	b.fillPending(1)
	if !b.hasPosition || b.position.Entry != 110 || b.position.SL != 95 || b.position.TP != 105 {
		t.Fatalf("signal-time protection = %+v, want fill 110 with SL 95 and TP 105", b.position)
	}

	b = broker{series: series, costs: Costs{FillOn: "open"}}
	b.openPosition(sideLong, 110, order{
		Side: sideLong, StopDistance: 5, HasStopDistance: true,
		TargetDistance: 10, HasTargetDistance: true, Size: 1, HasSize: true,
	}, 1)
	if b.position.SL != 105 || b.position.TP != 120 {
		t.Fatalf("fill-relative protection = %+v, want SL 105 and TP 120", b.position)
	}
}

func TestFillOnOpenGapStopUsesOpenAndFinalBarDoesNotPhantomFill(t *testing.T) {
	series := marketdata.Series{
		T: []float64{0, 1}, O: []float64{100, 90}, H: []float64{101, 95},
		L: []float64{99, 80}, C: []float64{100, 90}, V: []float64{1, 1},
	}
	b := broker{
		series: series,
		costs:  Costs{FillOn: "open"},
		pendingOrders: []order{{
			Side: sideLong, Index: 1, SL: 95, TP: 120, GapAwareStop: true,
			Size: 1, HasSize: true,
		}},
	}
	trades := b.run()
	if len(trades) != 1 || trades[0].EntryIndex != 1 || trades[0].Entry != 90 ||
		trades[0].ExitIndex != 1 || trades[0].Exit != 90 || trades[0].Reason != "sl" {
		t.Fatalf("gap trades = %+v, want entry and worse-open stop at 90 on bar 1", trades)
	}

	finalBar := broker{
		series: marketdata.Series{
			T: []float64{0}, O: []float64{100}, H: []float64{101},
			L: []float64{99}, C: []float64{100}, V: []float64{1},
		},
		costs:         Costs{FillOn: "open"},
		pendingOrders: []order{{Side: sideLong, Index: 1, SL: 95, TP: 105, Size: 1, HasSize: true}},
	}
	if trades := finalBar.run(); len(trades) != 0 || finalBar.hasPosition {
		t.Fatalf("final-bar pending order produced trades=%+v position=%v", trades, finalBar.hasPosition)
	}
}
