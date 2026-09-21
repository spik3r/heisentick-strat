package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestMaxHoldCandlesPreservesFractionalJSThreshold(t *testing.T) {
	tests := []struct {
		name          string
		maxHold       float64
		wantExitIndex int
		wantReason    string
	}{
		{name: "zero remains disabled", maxHold: 0, wantExitIndex: 2, wantReason: ReasonEndOfTest},
		{name: "fraction below one exits after one elapsed bar", maxHold: 0.4, wantExitIndex: 1, wantReason: "time"},
		{name: "one keeps integer behavior", maxHold: 1, wantExitIndex: 1, wantReason: "time"},
		{name: "fraction above one exits after two elapsed bars", maxHold: 1.4, wantExitIndex: 2, wantReason: "time"},
		{name: "two keeps integer behavior", maxHold: 2, wantExitIndex: 2, wantReason: "time"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := paramsFromConfig(dsl.Config{
				"setupType":      string(dsl.FamilyRangeBreakFake),
				"maxHoldCandles": tt.maxHold,
			})
			if params.MaxHoldBars != tt.maxHold {
				t.Fatalf("mapped max hold = %v, want authored threshold %v", params.MaxHoldBars, tt.maxHold)
			}

			b := maxHoldTestBroker(params, tt.wantExitIndex+1)
			trades := b.run()
			if len(trades) != 1 {
				t.Fatalf("trades = %+v, want one closed position", trades)
			}
			if got := trades[0]; got.ExitIndex != tt.wantExitIndex || got.Reason != tt.wantReason {
				t.Fatalf("trade = %+v, want exit index %d with reason %q", got, tt.wantExitIndex, tt.wantReason)
			}
		})
	}
}

func maxHoldTestBroker(params flagParams, barCount int) broker {
	bars := make([]marketdata.Bar, barCount)
	atr := make([]float64, barCount)
	for i := range bars {
		bars[i] = marketdata.Bar{T: float64(i + 1), O: 100, H: 100, L: 100, C: 100}
		atr[i] = 1
	}
	return broker{
		series: marketdata.SeriesFromBars(bars),
		cols:   contextcols.Columns{ATR: atr},
		params: params,
		position: position{
			Side: sideLong, Entry: 100, Size: 1, SL: 90, TP: 110,
			InitialSL: 90, InitialTP: 110, EntryIndex: 0, EntryT: 1,
		},
		hasPosition: true,
	}
}
