package engine

import (
	"encoding/json"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestDualEMAPendingExitPrecedesIntrabarStop(t *testing.T) {
	b := broker{
		series: marketdata.SeriesFromBars([]marketdata.Bar{
			{T: 1, O: 383, H: 384, L: 380, C: 382, V: 1},
			{T: 2, O: 381.117, H: 382, L: 366.867, C: 370, V: 1},
		}),
		costs: Costs{Slippage: 0.06},
	}
	b.openPosition(sideLong, 383, order{SL: 378.077, Size: 1, HasSize: true, NoTarget: true}, 0)
	b.pendingExits = append(b.pendingExits, pendingExit{PositionEntryIndex: 0, Index: 1, Rule: "slow-ema"})
	b.fillPendingExits(1)
	b.resolveIntrabarExit(1)
	if len(b.trades) != 1 || b.trades[0].Reason != ReasonRule || b.trades[0].Rule != "slow-ema" || b.trades[0].Exit != 381.057 {
		t.Fatalf("trade = %+v, want rule/slow-ema at slipped open 381.057", b.trades)
	}
}

func TestDualEMANextOpenStopIsActiveOnFillBar(t *testing.T) {
	b := broker{
		series: marketdata.SeriesFromBars([]marketdata.Bar{
			{T: 1, O: 100, H: 101, L: 99, C: 100, V: 1},
			{T: 2, O: 104, H: 105, L: 100, C: 102, V: 1},
		}),
		costs:         Costs{Slippage: 0.5},
		pendingOrders: []order{{Side: sideLong, Index: 1, StopDistance: 2.5, HasStopDistance: true, GapAwareStop: true, NoTarget: true, Size: 1, HasSize: true}},
	}
	b.fillPending(1)
	b.resolveIntrabarExit(1)
	if len(b.trades) != 1 || b.trades[0].Entry != 104.5 || b.trades[0].Exit != 101.5 || b.trades[0].ExitIndex != 1 {
		t.Fatalf("trade = %+v, want entry-bar stop from slipped fill", b.trades)
	}
}

func TestDualEMANoTargetSerializesAsNullWithoutChangingLegacyZero(t *testing.T) {
	result := RunResult{Trades: []Trade{{NoTarget: true}, {TP: 0, InitialTP: 0}}}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	trades := decoded["trades"].([]any)
	noTarget := trades[0].(map[string]any)
	legacy := trades[1].(map[string]any)
	if noTarget["tp"] != nil || noTarget["initialTp"] != nil {
		t.Fatalf("no-target = %#v", noTarget)
	}
	if legacy["tp"] != float64(0) || legacy["initialTp"] != float64(0) {
		t.Fatalf("legacy = %#v", legacy)
	}
}
