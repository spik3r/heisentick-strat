package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestRunForceClosesOpenPositionAtEndOfData(t *testing.T) {
	const finalBarCount = 679
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "money-risk-sizing.fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	if len(fixture.Bars) < finalBarCount || len(fixture.RawBars) < finalBarCount {
		t.Fatalf("fixture bars = %d/%d, need at least %d", len(fixture.Bars), len(fixture.RawBars), finalBarCount)
	}
	fixture.Bars = fixture.Bars[:finalBarCount]
	fixture.RawBars = fixture.RawBars[:finalBarCount]
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "money-risk-sizing.strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}

	result, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run truncated fixture: %v", err)
	}

	if result.Costs != (Costs{FeePerUnit: 0, FillOn: "close", Slippage: 0.06, StartEquity: 10000}) {
		t.Fatalf("costs = %+v, want reviewed fixture costs", result.Costs)
	}
	want := Trade{
		Entry:      2649.735,
		EntryIndex: 677,
		EntryT:     1736335800000,
		Exit:       2650.535,
		ExitIndex:  678,
		ExitT:      1736336700000,
		InitialSL:  2646.89053571429,
		InitialTP:  2655.01146428571,
		Meta: TradeMeta{
			"approachHi":      2654.995,
			"approachLo":      2647.454,
			"edge":            "low",
			"grade":           "A",
			"gradeRequired":   float64(0),
			"gradeScore":      float64(0),
			"levelKey":        "AL",
			"levelPrice":      2645.095,
			"rangeEnd":        677,
			"rangeHi":         2655.01146428571,
			"rangeLo":         2648.25453571429,
			"rangeStart":      663,
			"setup":           "rangeSweepReclaim",
			"setupAgeCandles": 0,
			"side":            "long",
			"source":          "range",
		},
		PnL:    28.1248038169273,
		Points: 0.799999999999727,
		Reason: "eod",
		Side:   "long",
		Size:   35.1560047711711,
		SL:     2646.89053571429,
		Tag:    "DSL:AL:A",
		TP:     2655.01146428571,
	}
	if result.TradeCount != 1 || len(result.Trades) != 1 {
		t.Fatalf("trade envelope = count %d, trades %+v; want one eod trade", result.TradeCount, result.Trades)
	}
	if got := result.Trades[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("eod trade = %#v\nwant %#v", got, want)
	}
}

func TestRunWithNoBarsDoesNotForceCloseOrConsumeOrders(t *testing.T) {
	b := broker{
		series:        marketdata.NewSeries(0),
		hasPosition:   true,
		position:      position{Side: sideLong, Entry: 100, Size: 1, SL: 90, TP: 110},
		pendingOrders: []order{{Side: sideLong, Index: 0}},
		limitOrders:   []order{{Side: sideLong, Limit: 95}},
	}

	trades := b.run()

	if len(trades) != 0 || !b.hasPosition {
		t.Fatalf("empty run trades=%+v position=%+v, want live position and no forced close", trades, b.position)
	}
	if len(b.pendingOrders) != 1 || len(b.limitOrders) != 1 {
		t.Fatalf("empty run consumed orders: pending=%d limit=%d", len(b.pendingOrders), len(b.limitOrders))
	}
}

func TestRunFinalBarPendingFillAppliesPartialBeforeEndOfDataClose(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{1000},
			O: []float64{100},
			H: []float64{106},
			L: []float64{95},
			C: []float64{105},
		},
		params: flagParams{
			SetupType: "flagContinuation",
			Partial: partialParams{
				Enabled: true, Fraction: 0.5, TriggerR: 0.25,
			},
		},
		pendingOrders: []order{{
			Side: sideLong, SL: 90, TP: 120, RiskUSD: 100,
			Tag: "DSL-FLAG", Meta: TradeMeta{"setup": "flag"}, Index: 0,
		}},
	}

	trades := b.run()

	if len(trades) != 2 || b.hasPosition || len(b.pendingOrders) != 0 {
		t.Fatalf("final-bar trades=%+v live=%v pending=%d, want partial plus eod and no live order", trades, b.hasPosition, len(b.pendingOrders))
	}
	partial, eod := trades[0], trades[1]
	if !partial.Partial || partial.Reason != "partial" || partial.Size != 5 || partial.Exit != 105 || partial.ExitIndex != 0 || partial.ExitT != 1000 {
		t.Fatalf("partial trade = %+v, want size-5 final-bar partial", partial)
	}
	if eod.Partial || eod.Reason != "eod" || eod.Size != 5 || eod.Exit != 105 || eod.ExitIndex != 0 || eod.ExitT != 1000 {
		t.Fatalf("eod trade = %+v, want size-5 final-bar remainder", eod)
	}
	for _, trade := range trades {
		if trade.Entry != 100 || trade.EntryIndex != 0 || trade.EntryT != 1000 || trade.InitialSL != 90 || trade.SL != 90 || trade.InitialTP != 120 || trade.TP != 120 || trade.Points != 5 || trade.PnL != 25 {
			t.Fatalf("final-bar trade fields = %+v, want shared fill and management values", trade)
		}
	}
}

func TestRunDoesNotDuplicatePositionClosedOnFinalBar(t *testing.T) {
	const midnightBrisbane = 14 * 60 * 60 * 1000
	b := broker{
		series: marketdata.Series{
			T: []float64{midnightBrisbane},
			O: []float64{100},
			H: []float64{111},
			L: []float64{95},
			C: []float64{105},
		},
		params: flagParams{SetupType: "elderTripleScreen"},
		position: position{
			Side: sideLong, Entry: 100, Size: 2, SL: 90, TP: 110,
			InitialSL: 90, InitialTP: 110, EntryT: midnightBrisbane,
		},
		hasPosition:   true,
		pendingOrders: []order{{Side: sideLong, Index: 2}},
		limitOrders:   []order{{Side: sideLong, Limit: 95, PlacedAt: 1, ExpireAt: 2}},
	}

	trades := b.run()

	if len(trades) != 1 || trades[0].Reason != "tp" || b.hasPosition {
		t.Fatalf("final-bar close produced %+v with live=%v, want one tp and no eod duplicate", trades, b.hasPosition)
	}
	if len(b.pendingOrders) != 1 || len(b.limitOrders) != 1 {
		t.Fatalf("final close consumed future orders: pending=%d limit=%d", len(b.pendingOrders), len(b.limitOrders))
	}
}
