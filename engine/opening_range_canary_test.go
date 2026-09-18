package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestOpeningRangeCanaryMapsUTCSlotsAndFixedPips(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
strategy "canary" {
}
setup {
  type: opening range breakout
  opening range every 3 hours UTC
}
risk {
  stop 10 pips
}
target {
  target 20 pips
}
`)
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse DSL diagnostics: %v", parsed.Errors)
	}

	params := paramsFromConfig(parsed.Config)
	if params.ORBUTCSlotMinutes != 180 {
		t.Fatalf("UTC slot minutes = %d, want 180", params.ORBUTCSlotMinutes)
	}
	if params.ORBFirstMinutes != 15 || params.ORBFirstCandles != 0 {
		t.Fatalf("opening range = %d minutes/%d candles, want 15/0", params.ORBFirstMinutes, params.ORBFirstCandles)
	}
	if params.ORBFixedStopPips != 10 || params.ORBFixedTargetPips != 20 {
		t.Fatalf("fixed pip stop/target = %v/%v, want 10/20", params.ORBFixedStopPips, params.ORBFixedTargetPips)
	}
}

func TestOpeningRangeCanaryUsesReviewedXAUUSDPipSize(t *testing.T) {
	b := broker{
		series:  marketdata.Series{C: []float64{100, 101}},
		cols:    contextcols.Columns{ATR: []float64{2, 2}},
		fixture: RunFixture{Symbol: "XAUUSD"},
		params: flagParams{
			AllowLong:          true,
			ORBHoldCandles:     1,
			ORBUTCSlotMinutes:  180,
			ORBFixedStopPips:   10,
			ORBFixedTargetPips: 20,
		},
	}
	r := openingRange{Session: "utc:0", EndIndex: 0, High: 100, Low: 99, Size: 1}

	setup, ok := b.openingRangeBreakoutSetup(1, r, sideLong)
	if !ok {
		t.Fatal("fixed-pip opening-range setup was rejected")
	}
	if setup.Stop != 100 || setup.Target != 103 {
		t.Fatalf("stop/target = %v/%v, want 100/103", setup.Stop, setup.Target)
	}
	if setup.Meta["forwardBracketAnchor"] != "entry-fill" || setup.Meta["forwardStopDistance"] != 1.0 || setup.Meta["forwardTargetDistance"] != 2.0 {
		t.Fatalf("forward bracket metadata = %+v, want entry-fill/1/2", setup.Meta)
	}
	if setup.Meta["forwardExitAt"] != "1970-01-01T03:00:00.000Z" || setup.Meta["forwardExitReason"] != "window-close" {
		t.Fatalf("forward exit metadata = %+v, want timed window close", setup.Meta)
	}
}

func TestUTCSlotProgressUsesContiguousThreeHourBoundaries(t *testing.T) {
	progress, ok := utcSlotProgress(10_790_000, 180)
	if !ok {
		t.Fatal("UTC slot progress was rejected")
	}
	if progress.Key != "utc:0" || progress.SlotStart != 0 || progress.MinutesFromStart != 179 {
		t.Fatalf("progress = %+v, want final minute of utc:0 slot", progress)
	}

	next, ok := utcSlotProgress(10_800_000, 180)
	if !ok || next.Key != "utc:10800000" || next.SlotStart != 10_800_000 || next.MinutesFromStart != 0 {
		t.Fatalf("next progress = %+v/%v, want first minute of utc:10800000 slot", next, ok)
	}
}

func TestOpeningRangeCanaryRejectsFixedPipsOutsideORB(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
strategy "not orb" {
}
setup {
  type failed breakout
}
risk {
  stop 10 pips
}
target {
  target 20 pips
}
`)
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) < 2 {
		t.Fatalf("expected fixed-pip stop and target diagnostics, got %v", parsed.Errors)
	}
}

func TestOpeningRangeCanaryRejectsInvalidUTCSlotHours(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
strategy "bad slot" {
}
setup {
  type opening range breakout
  opening range every 0 hours UTC
}
`)
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) != 1 {
		t.Fatalf("expected one invalid-hours diagnostic, got %v", parsed.Errors)
	}
}

func TestOpeningRangeCanaryRejectsFractionalUTCSlotHours(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
strategy "fractional slot" {
}
setup {
  type opening range breakout
  opening range every 3.5 hours UTC
}
`)
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) != 1 {
		t.Fatalf("expected one fractional-hours diagnostic, got %v", parsed.Errors)
	}
}

func TestOpeningRangeCanaryBoundaryClosePrecedesBracket(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{0, 180 * 60_000}, O: []float64{100, 100},
			H: []float64{100, 103.5}, L: []float64{100, 99.8}, C: []float64{100, 100.2},
		},
		position: position{
			Side: sideLong, Entry: 101, Size: 1, SL: 100, TP: 103,
			InitialSL: 100, InitialTP: 103, EntryIndex: 0, EntryT: 0,
			Meta: TradeMeta{"slotEnd": int64(180 * 60_000)},
		},
		hasPosition: true,
	}
	b.closeExpiredWindowPosition(1)
	b.resolveIntrabarExit(1)
	if len(b.trades) != 1 {
		t.Fatalf("trades = %+v, want one boundary close", b.trades)
	}
	got := b.trades[0]
	if got.Reason != "window-close" || got.Exit != 100 || got.ExitT != 180*60_000 {
		t.Fatalf("trade = %+v, want boundary/open close before target", got)
	}
}

func TestOpeningRangeCanaryAnchorsFixedBracketsToFilledEntry(t *testing.T) {
	b := broker{
		series: marketdata.Series{T: []float64{0}, O: []float64{101}, H: []float64{101}, L: []float64{101}, C: []float64{101}},
		costs:  Costs{Slippage: 0.06},
	}
	b.openPosition(sideLong, 101, order{
		SL: 100, TP: 103, StopDistance: 1, HasStopDistance: true,
		TargetDistance: 2, HasTargetDistance: true, Size: 1, HasSize: true,
	}, 0)
	if b.position.Entry != 101.06 || b.position.SL != 100.06 || b.position.TP != 103.06 {
		t.Fatalf("filled bracket = entry %v sl %v tp %v, want 101.06/100.06/103.06", b.position.Entry, b.position.SL, b.position.TP)
	}
}
