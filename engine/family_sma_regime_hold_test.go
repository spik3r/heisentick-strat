package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSMARegimeHoldSeedsFromFirstValidPriceAndExecutesNextOpen(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{0, 1, 2, 3, 4},
			O: []float64{10, 10, 11, 12, 8},
			H: []float64{10, 10, 11, 12, 8},
			L: []float64{10, 10, 11, 9, 8},
			C: []float64{10, 10, 11, 9, 8},
			V: []float64{1, 1, 1, 1, 1},
		},
		params: flagParams{
			SetupType:     "smaRegimeHold",
			SMARegimeHold: smaRegimeHoldParams{Period: 2},
		},
		costs: Costs{FillOn: "close"},
	}

	trades := b.run()
	if len(trades) != 1 {
		t.Fatalf("trades = %+v, want one seeded long/cash regime trade", trades)
	}
	trade := trades[0]
	if trade.Side != "long" || trade.Size != 1 || trade.EntryIndex != 3 || trade.Entry != 12 {
		t.Fatalf("entry = %+v, want next-open long at index 3 price 12", trade)
	}
	if trade.ExitIndex != 4 || trade.Exit != 8 || trade.Rule != "sma-regime-below-or-equal" {
		t.Fatalf("exit = %+v, want next-open cash transition at index 4 price 8", trade)
	}
	if !trade.NoStop || !trade.NoTarget {
		t.Fatalf("brackets = stop %v target %v, want both absent", trade.NoStop, trade.NoTarget)
	}
	if trade.Meta["setup"] != "smaRegimeHold" || trade.Meta["signalIndex"] != float64(2) || trade.Meta["smaPeriod"] != float64(2) {
		t.Fatalf("trade metadata = %+v", trade.Meta)
	}
}

func TestSMARegimeHoldEqualityStaysInCashAtInitialization(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{0, 1, 2},
			O: []float64{10, 10, 11},
			H: []float64{10, 10, 11},
			L: []float64{10, 10, 11},
			C: []float64{10, 10, 11},
			V: []float64{1, 1, 1},
		},
		params: flagParams{SetupType: "smaRegimeHold", SMARegimeHold: smaRegimeHoldParams{Period: 2}},
		costs:  Costs{FillOn: "close"},
	}
	if trades := b.run(); len(trades) != 0 {
		t.Fatalf("trades = %+v, equality should remain cash until a later close initializes long", trades)
	}
}

func TestSMARegimeHoldEntersAtFirstEligibleCloseWithoutPriorCross(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{0, 1, 2},
			O: []float64{9, 11, 12},
			H: []float64{9, 11, 12},
			L: []float64{9, 11, 12},
			C: []float64{9, 11, 12},
			V: []float64{1, 1, 1},
		},
		params: flagParams{SetupType: "smaRegimeHold", SMARegimeHold: smaRegimeHoldParams{Period: 2}},
		costs:  Costs{FillOn: "close"},
	}
	trades := b.run()
	if len(trades) != 1 || trades[0].EntryIndex != 2 || trades[0].Entry != 12 {
		t.Fatalf("trades = %+v, want entry at first signal's next open (index 2, price 12)", trades)
	}
}
