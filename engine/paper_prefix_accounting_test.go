package engine

import (
	"math"
	"runtime"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestPaperPartialFeesAreLogicalTradeAccounting(t *testing.T) {
	for _, direction := range []side{sideLong, sideShort} {
		t.Run(direction.String(), func(t *testing.T) {
			closes := []float64{100, 105, 110}
			if direction == sideShort {
				closes = []float64{100, 95, 90}
			}
			bars := make([]marketdata.Bar, 3)
			for i, v := range closes {
				bars[i] = marketdata.Bar{T: float64(1000 + i), O: v, H: v + 1, L: v - 1, C: v, V: 1}
			}
			series := marketdata.SeriesFromBars(bars)
			var b broker
			b.reset(series, contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: Costs{StartEquity: 1000, FeePerUnit: .5, Slippage: .25, FillOn: "close"}}, nil)
			c := newPaperCollector("binding", 1000)
			b.paperAccounting = c
			stop, target := 90.0, 120.0
			if direction == sideShort {
				stop, target = 110, 80
			}
			b.openPosition(direction, 100, order{Size: 10, HasSize: true, SL: stop, TP: target}, 0)
			c.mark(&b, 0)
			b.params.Partial.Enabled = true
			b.params.Partial.Fraction = .5
			b.params.Partial.TriggerR = .25
			b.applyPartialManagement(1)
			c.mark(&b, 1)
			if c.err != nil {
				t.Fatal(c.err)
			}
			paperNear(t, b.realized, 15)
			paperNear(t, c.account.Equity, 1038.75)
			paperNear(t, c.account.Fees, 7.5)
			if c.statistics.Count != 0 || c.open == nil || c.open.RemainingSize != 5 {
				t.Fatal("partial completed a logical trade")
			}
			b.closePosition(closes[2], 2, "target", "")
			c.mark(&b, 2)
			if c.err != nil {
				t.Fatal(c.err)
			}
			paperNear(t, c.account.Equity, 1060)
			paperNear(t, c.account.RealizedNet, 60)
			paperNear(t, c.account.Fees, 10)
			paperNear(t, c.statistics.NetPnL, 60)
			paperNear(t, c.account.MaxDrawdown, 7.5)
			if c.statistics.Count != 1 || c.statistics.Wins != 1 || c.open != nil {
				t.Fatal("completion")
			}
			if len(c.events) != 6 || c.events[0].Kind != "entry" || c.events[2].Kind != "partial" || c.events[4].Kind != "exit" {
				t.Fatalf("events: %+v", c.events)
			}
			if c.events[2].Effect.CompletedNetPnL != nil || *c.events[4].Effect.CompletedNetPnL != 60 {
				t.Fatal("partial fee allocation")
			}
			seen := map[string]bool{}
			for _, e := range c.events {
				if seen[e.EventID] {
					t.Fatal("duplicate effect ID")
				}
				seen[e.EventID] = true
			}
		})
	}
}

func TestPaperLoserOnlyAndBreakevenDenominators(t *testing.T) {
	for _, price := range []float64{99, 100} {
		series := marketdata.SeriesFromBars([]marketdata.Bar{{T: 1, O: 100, H: 100, L: 100, C: 100}, {T: 2, O: price, H: price, L: price, C: price}})
		var b broker
		b.reset(series, contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: Costs{StartEquity: 100}}, nil)
		c := newPaperCollector("binding", 100)
		b.paperAccounting = c
		if c.statistics.WinRate != nil || c.statistics.ProfitFactor != nil || c.statistics.ProfitFactorStatus != "no-trades" {
			t.Fatal("no-trade denominator")
		}
		b.openPosition(sideLong, 100, order{Size: 1, HasSize: true, SL: 90, TP: 110}, 0)
		b.closePosition(price, 1, "rule", "test")
		c.mark(&b, 1)
		if c.err != nil {
			t.Fatal(c.err)
		}
		if *c.statistics.WinRate != 0 {
			t.Fatal("win rate")
		}
		if price == 99 {
			if c.statistics.Losses != 1 || c.statistics.ProfitFactor == nil || *c.statistics.ProfitFactor != 0 || c.statistics.ProfitFactorStatus != "defined" {
				t.Fatal("loser-only")
			}
		} else if c.statistics.Breakeven != 1 || c.statistics.ProfitFactor != nil || c.statistics.ProfitFactorStatus != "no-losses" {
			t.Fatal("breakeven")
		}
	}
}

func TestPaperObserverDoesNotAlterBrokerArithmetic(t *testing.T) {
	// Cancellation-sensitive values deliberately exercise the platform's genuine
	// existing fee and credit schedule, not a recomputed JS/host expectation.
	for _, fee := range []float64{0, .1, .3333333333333333, 1e12, math.SmallestNonzeroFloat64} {
		series := marketdata.SeriesFromBars([]marketdata.Bar{{T: 1, O: 1e8, H: 1e8 + 1, L: 1e8 - 1, C: 1e8}, {T: 2, O: 1e8 + .3, H: 1e8 + 1, L: 1e8 - 1, C: 1e8 + .3}})
		var bare, observed broker
		costs := Costs{StartEquity: 1e16, FeePerUnit: fee, Slippage: .1, SlippageBps: .001, FillOn: "close"}
		bare.reset(series, contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: costs}, nil)
		observed.reset(series, contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: costs}, nil)
		c := newPaperCollector("binding", costs.StartEquity)
		observed.paperAccounting = c
		for _, b := range []*broker{&bare, &observed} {
			b.openPosition(sideLong, 1e8, order{Size: .7, HasSize: true, SL: 1e8 - 5, TP: 1e8 + 5}, 0)
		}
		if math.Float64bits(bare.realized) != math.Float64bits(observed.realized) || math.Float64bits(bare.position.Entry) != math.Float64bits(observed.position.Entry) {
			t.Fatal("entry changed")
		}
		for _, b := range []*broker{&bare, &observed} {
			b.closePosition(1e8+.3, 1, "rule", "test")
		}
		if c.err != nil {
			t.Fatal(c.err)
		}
		if math.Float64bits(bare.realized) != math.Float64bits(observed.realized) || math.Float64bits(bare.trades[0].PnL) != math.Float64bits(observed.trades[0].PnL) {
			t.Fatal("exit changed")
		}
		if !paperSame(c.events[0].RealizedAfter, c.events[1].RealizedBefore) || !paperSame(c.events[1].RealizedAfter, observed.realized) {
			t.Fatal("mutation continuity")
		}
	}
}

func TestPaperPartialObservedPlatformRemainder(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{{T: 1, O: 100, H: 101, L: 99, C: 100}, {T: 2, O: 105, H: 106, L: 104, C: 105}, {T: 3, O: 110, H: 111, L: 109, C: 110}})
	var b broker
	b.reset(series, contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: Costs{StartEquity: 1000, FeePerUnit: .1}}, nil)
	c := newPaperCollector("binding", 1000)
	b.paperAccounting = c
	b.openPosition(sideLong, 100, order{Size: 10, HasSize: true, SL: 90, TP: 120}, 0)
	b.params.Partial.Enabled = true
	b.params.Partial.Fraction = .7
	b.params.Partial.TriggerR = .25
	b.applyPartialManagement(1)
	if c.err != nil {
		t.Fatal(c.err)
	}
	bits := math.Float64bits(b.position.Size)
	if runtime.GOARCH == "arm64" {
		if bits != 0x4008000000000001 {
			t.Fatalf("ARM64 preserved fused remainder bits %x", bits)
		}
	} else if runtime.GOARCH == "amd64" || runtime.GOARCH == "wasm" {
		if bits != 0x4008000000000000 {
			t.Fatalf("separate remainder bits %x", bits)
		}
	}
	if !paperSame(c.open.RemainingSize, b.position.Size) {
		t.Fatal("collector reconstructed the remainder")
	}
	t.Logf("actual target %s/%s remainder bits %016x, closed size bits %016x", runtime.GOOS, runtime.GOARCH, bits, math.Float64bits(b.trades[0].Size))
	c.mark(&b, 1)
	b.closePosition(110, 2, "rule", "test")
	c.mark(&b, 2)
	if c.err != nil || c.statistics.Count != 1 || !paperSame(c.account.RealizedNet, b.realized) {
		t.Fatalf("remaining lifecycle: %v", c.err)
	}
}
