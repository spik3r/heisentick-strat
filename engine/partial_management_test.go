package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestApplyPartialManagementReducesOnceAndMovesRemainderToBreakeven(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{1000}, C: []float64{102.5},
		},
		costs: Costs{Slippage: 0.1},
		params: flagParams{Partial: partialParams{
			Enabled: true, Fraction: 0.5, TriggerR: 0.25, MoveBreakeven: true,
		}},
		position: position{
			Side: sideLong, Entry: 100, Size: 10, SL: 90, TP: 110,
			InitialSL: 90, InitialTP: 110, Meta: TradeMeta{"setup": "flag"},
			EntryIndex: 0, EntryT: 500, Tag: "DSL-FLAG",
		},
		hasPosition: true,
	}

	b.applyPartialManagement(0)

	if got, want := len(b.trades), 1; got != want {
		t.Fatalf("trades = %d, want %d", got, want)
	}
	trade := b.trades[0]
	if trade.Reason != "partial" {
		t.Fatalf("partial trade reason = %q", trade.Reason)
	}
	if !trade.Partial {
		t.Fatal("fractional close must be marked partial")
	}
	encoded, err := json.Marshal(trade)
	if err != nil {
		t.Fatalf("marshal partial trade: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"partial":true`)) {
		t.Fatalf("partial trade JSON = %s, want partial marker", encoded)
	}
	if got, want := trade.Size, 5.0; got != want {
		t.Fatalf("partial size = %v, want %v", got, want)
	}
	if got, want := trade.Exit, 102.4; math.Abs(got-want) > 1e-12 {
		t.Fatalf("partial exit = %v, want %v", got, want)
	}
	if got, want := trade.PnL, 12.0; math.Abs(got-want) > 1e-12 {
		t.Fatalf("partial pnl = %v, want %v", got, want)
	}
	if got, want := b.position.Size, 5.0; got != want {
		t.Fatalf("remaining size = %v, want %v", got, want)
	}
	if got, want := b.position.SL, b.position.Entry; got != want {
		t.Fatalf("remaining stop = %v, want breakeven %v", got, want)
	}

	b.applyPartialManagement(0)
	if got := len(b.trades); got != 1 {
		t.Fatalf("partial management repeated: trades = %d", got)
	}
}

func TestApplyPartialManagementFullFractionUsesUnmarkedFullClose(t *testing.T) {
	b := broker{
		series: marketdata.Series{T: []float64{1000}, C: []float64{102.5}},
		params: flagParams{Partial: partialParams{
			Enabled: true, Fraction: 1, TriggerR: 0.25,
		}},
		position: position{
			Side: sideLong, Entry: 100, Size: 10, SL: 90, TP: 110,
			InitialSL: 90, InitialTP: 110,
		},
		hasPosition: true,
	}

	b.applyPartialManagement(0)

	if got, want := len(b.trades), 1; got != want {
		t.Fatalf("trades = %d, want %d", got, want)
	}
	trade := b.trades[0]
	if trade.Partial {
		t.Fatal("full close at fraction 1 must not be marked partial")
	}
	if trade.Reason != "partial" {
		t.Fatalf("full close reason = %q, want partial", trade.Reason)
	}
	if got, want := trade.Size, 10.0; got != want {
		t.Fatalf("full close size = %v, want %v", got, want)
	}
	if b.hasPosition {
		t.Fatal("full close left a position open")
	}
	encoded, err := json.Marshal(trade)
	if err != nil {
		t.Fatalf("marshal full-close trade: %v", err)
	}
	if bytes.Contains(encoded, []byte(`"partial":`)) {
		t.Fatalf("full-close trade JSON = %s, want partial field omitted", encoded)
	}
}

func TestApplyPartialManagementWaitsForConfiguredRMultiple(t *testing.T) {
	b := broker{
		series: marketdata.Series{T: []float64{1000}, C: []float64{101}},
		params: flagParams{Partial: partialParams{
			Enabled: true, Fraction: 0.5, TriggerR: 0.25,
		}},
		position:    position{Side: sideLong, Entry: 100, Size: 10, SL: 90},
		hasPosition: true,
	}

	b.applyPartialManagement(0)
	if len(b.trades) != 0 || b.position.Size != 10 || b.position.PartialTaken {
		t.Fatalf("partial fired below trigger: trades=%d position=%+v", len(b.trades), b.position)
	}
}

func TestApplyPartialManagementSkipsZeroSizedPositionBeforeEndOfDataClose(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{1000}, O: []float64{100}, H: []float64{106},
			L: []float64{95}, C: []float64{105},
		},
		params: flagParams{
			SetupType: "flagContinuation",
			Partial: partialParams{
				Enabled: true, Fraction: 0.5, TriggerR: 0.25,
			},
		},
		position: position{
			Side: sideLong, Entry: 100, Size: 0, SL: 90, TP: 120,
			InitialSL: 90, InitialTP: 120, EntryIndex: 0, EntryT: 1000,
		},
		hasPosition: true,
	}

	trades := b.run()

	if len(trades) != 1 {
		t.Fatalf("zero-sized final-bar trades = %+v, want one ordinary end-of-data close", trades)
	}
	trade := trades[0]
	if trade.Partial || trade.Reason != ReasonEndOfTest || trade.Size != 0 || trade.PnL != 0 {
		t.Fatalf("zero-sized final trade = %+v, want unmarked zero-sized eod close", trade)
	}
	if b.hasPosition || b.position.Size != 0 || b.position.PartialTaken {
		t.Fatalf("zero-sized final position = %+v live=%v, want closed without partial state", b.position, b.hasPosition)
	}
}

func TestApplyPartialManagementMarksNegativeSizedPositionWithoutReducingIt(t *testing.T) {
	for _, fraction := range []float64{0.5, 1} {
		for _, moveBreakeven := range []bool{false, true} {
			name := fmt.Sprintf("fraction_%g_move_breakeven_%t", fraction, moveBreakeven)
			t.Run(name, func(t *testing.T) {
				b := broker{
					series: marketdata.Series{
						T: []float64{1000, 2000},
						O: []float64{102, 103},
						H: []float64{103, 105},
						L: []float64{101, 102},
						C: []float64{103, 105},
					},
					costs: Costs{Slippage: 0.1, FeePerUnit: 0.2},
					params: flagParams{
						SetupType: "flagContinuation",
						Partial: partialParams{
							Enabled: true, Fraction: fraction, TriggerR: 0.25,
							MoveBreakeven: moveBreakeven,
						},
					},
				}
				b.openPosition(sideLong, 100, order{
					SL: 90, TP: 120, Size: -10, Tag: "NEGATIVE-PARTIAL",
					Meta: TradeMeta{"setup": "flag"},
				}, 0)

				b.applyPartialManagement(0)

				if len(b.trades) != 0 {
					t.Fatalf("management trades = %+v, want none", b.trades)
				}
				if !b.hasPosition || b.position.Size != -10 || !b.position.PartialTaken {
					t.Fatalf("managed position = %+v live=%v, want unchanged negative size marked partial", b.position, b.hasPosition)
				}
				wantStop := 90.0
				if moveBreakeven {
					wantStop = 100.1
				}
				if got := b.position.SL; math.Abs(got-wantStop) > 1e-12 {
					t.Fatalf("managed stop = %v, want %v", got, wantStop)
				}
				if got, want := b.realized, 2.0; math.Abs(got-want) > 1e-12 {
					t.Fatalf("realized after management = %v, want entry-fee credit %v", got, want)
				}

				trades := b.run()
				if len(trades) != 1 {
					t.Fatalf("final trades = %+v, want one ordinary end-of-data close", trades)
				}
				trade := trades[0]
				wantPoints := (104.9 - 100.1)
				wantPnL := wantPoints*-10 - 0.2*-10
				if trade.Partial || trade.Reason != ReasonEndOfTest || trade.Side != "long" || trade.Size != -10 ||
					math.Abs(trade.Entry-100.1) > 1e-12 || math.Abs(trade.Exit-104.9) > 1e-12 ||
					math.Abs(trade.Points-wantPoints) > 1e-12 || math.Abs(trade.PnL-wantPnL) > 1e-12 ||
					trade.EntryIndex != 0 || trade.ExitIndex != 1 || trade.EntryT != 1000 || trade.ExitT != 2000 ||
					trade.SL != wantStop || trade.InitialSL != 90 || trade.TP != 120 || trade.InitialTP != 120 ||
					trade.Tag != "NEGATIVE-PARTIAL" {
					t.Fatalf("final trade = %+v, want exact unmarked negative-sized eod close (points=%v pnl=%v)", trade, wantPoints, wantPnL)
				}
				if got, want := b.realized, 2.0+wantPnL; math.Abs(got-want) > 1e-12 {
					t.Fatalf("final realized = %v, want entry and exit cost accounting %v", got, want)
				}
			})
		}
	}
}
