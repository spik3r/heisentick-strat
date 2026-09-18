package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestTypedFamilyPartialDispatchParity(t *testing.T) {
	partialFamilies := []string{
		"breakRetest",
		"openingRangeBreakout",
		"sessionBreakHold",
		"triplePushExhaustion",
		"fibContinuation",
		"insideDayExpansion",
		"dayOpenReclaim",
		"supplyDemand",
		"doubleTopBottom",
		"trendPullback",
		"priceMomentum",
	}
	for _, setupType := range partialFamilies {
		t.Run(setupType+" dispatches partial", func(t *testing.T) {
			b := partialDispatchBroker(setupType, []float64{105})

			b.onBar(0)

			if len(b.trades) != 1 || b.trades[0].Reason != "partial" || b.trades[0].Size != 5 {
				t.Fatalf("trades = %+v, want one size-5 partial", b.trades)
			}
			if !b.hasPosition || b.position.Size != 5 || !b.position.PartialTaken {
				t.Fatalf("remaining position = %+v, want size-5 partial remainder", b.position)
			}
		})
	}

	nonPartialFamilies := []string{
		"rangeBreakFake",
		"vwapExtensionFade",
		"volumeAnomalyExhaustion",
		"elderTripleScreen",
	}
	for _, setupType := range nonPartialFamilies {
		t.Run(setupType+" keeps partial disabled", func(t *testing.T) {
			b := partialDispatchBroker(setupType, []float64{105})

			b.onBar(0)

			if len(b.trades) != 0 || !b.hasPosition || b.position.Size != 10 || b.position.PartialTaken {
				t.Fatalf("unexpected partial dispatch: trades=%+v position=%+v", b.trades, b.position)
			}
		})
	}
}

func TestTypedFamilyPartialPrecedesBreakevenAndTimeExit(t *testing.T) {
	b := partialDispatchBroker("breakRetest", []float64{100, 105})
	b.position.EntryIndex = 0
	b.params.BreakevenR = 0.25
	b.params.BreakevenOffsetATR = 0.5
	b.params.MaxHoldBars = 1

	b.onBar(1)

	if len(b.trades) != 2 {
		t.Fatalf("trades = %d, want partial then time: %+v", len(b.trades), b.trades)
	}
	partial, timed := b.trades[0], b.trades[1]
	if partial.Reason != "partial" || partial.Size != 5 || partial.ExitIndex != 1 || partial.SL != 90 {
		t.Fatalf("first trade = %+v, want pre-breakeven size-5 partial at index 1", partial)
	}
	if timed.Reason != "time" || timed.Size != 5 || timed.ExitIndex != 1 || timed.SL != 100.5 {
		t.Fatalf("second trade = %+v, want breakeven-adjusted size-5 time exit at index 1", timed)
	}
	if b.hasPosition {
		t.Fatalf("position remains after time exit: %+v", b.position)
	}
}

func partialDispatchBroker(setupType string, closes []float64) broker {
	n := len(closes)
	bars := make([]marketdata.Bar, n)
	atr := make([]float64, n)
	for i, close := range closes {
		bars[i] = marketdata.Bar{T: float64(i + 1), O: close, H: close, L: close, C: close}
		atr[i] = 1
	}
	return broker{
		series: marketdata.SeriesFromBars(bars),
		cols:   contextcols.Columns{ATR: atr},
		params: flagParams{
			SetupType:    setupType,
			BreakevenR:   100,
			Partial:      partialParams{Enabled: true, Fraction: 0.5, TriggerR: 0.25},
			SupplyDemand: supplyDemandParams{ImpulseCandles: 1, MinBaseCandles: 1},
		},
		position: position{
			Side: sideLong, Entry: 100, Size: 10, SL: 90, TP: 110,
			InitialSL: 90, InitialTP: 110, EntryIndex: 0, EntryT: 1, Tag: "DSL-TEST",
		},
		hasPosition: true,
	}
}
