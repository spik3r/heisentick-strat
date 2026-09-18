package main

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
)

func TestSummarizeEdgeRows(t *testing.T) {
	tests := []struct {
		name       string
		trades     []engine.Trade
		wantTrades int
		wantWin    float64
		wantPF     float64
		wantPFNil  bool
		wantNet    float64
		wantExp    float64
		wantDD     float64
	}{
		{name: "empty", wantPF: 0},
		{
			name: "zero pnl is a non-win",
			trades: []engine.Trade{
				{EntryT: 100, PnL: 0},
			},
			wantTrades: 1,
			wantPF:     0,
		},
		{
			name: "all wins use the infinite PF boundary",
			trades: []engine.Trade{
				{EntryT: 100, PnL: 25},
				{EntryT: 200, PnL: 75},
			},
			wantTrades: 2,
			wantWin:    100,
			wantPFNil:  true,
			wantNet:    100,
			wantExp:    50,
		},
		{
			name: "all losses keep numeric zero PF",
			trades: []engine.Trade{
				{EntryT: 100, PnL: -25},
				{EntryT: 200, PnL: -75},
			},
			wantTrades: 2,
			wantPF:     0,
			wantNet:    -100,
			wantExp:    -50,
			wantDD:     10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := mustSummarize(t, "edge", 0.25, engine.RunResult{
				Costs:  engine.Costs{StartEquity: 1000},
				Trades: tt.trades,
			})
			if row.Label != "edge" || row.Slippage != 0.25 || row.Trades != tt.wantTrades {
				t.Fatalf("row identity = label %q slip %v trades %d, want edge/0.25/%d", row.Label, row.Slippage, row.Trades, tt.wantTrades)
			}
			assertStatClose(t, "win rate", row.WinRate, tt.wantWin)
			assertStatClose(t, "net", row.Net, tt.wantNet)
			assertStatClose(t, "expectancy", row.Expectancy, tt.wantExp)
			assertStatClose(t, "drawdown", row.DD, tt.wantDD)
			if tt.wantPFNil {
				if row.PF != nil {
					t.Fatalf("PF = %v, want nil for internal Infinity", *row.PF)
				}
				return
			}
			if row.PF == nil {
				t.Fatalf("PF = nil, want %v", tt.wantPF)
			}
			assertStatClose(t, "profit factor", *row.PF, tt.wantPF)
		})
	}
}

func TestCostRowProfitFactorJSONBoundary(t *testing.T) {
	for _, tt := range []struct {
		name     string
		trades   []engine.Trade
		wantNull bool
	}{
		{name: "all-win infinite PF is JSON null", trades: []engine.Trade{{PnL: 1}}, wantNull: true},
		{name: "empty zero PF remains numeric"},
		{name: "all-loss zero PF remains numeric", trades: []engine.Trade{{PnL: -1}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			row := mustSummarize(t, "json", 0, engine.RunResult{Trades: tt.trades})
			raw, err := json.Marshal(row)
			if err != nil {
				t.Fatalf("marshal PF boundary: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("decode PF boundary: %v", err)
			}
			if tt.wantNull {
				if decoded["pf"] != nil {
					t.Fatalf("PF JSON = %#v, want null", decoded["pf"])
				}
				return
			}
			if decoded["pf"] != float64(0) {
				t.Fatalf("PF JSON = %#v, want numeric 0", decoded["pf"])
			}
		})
	}
}

func TestSummarizeStablePartialRemainderOrdering(t *testing.T) {
	trades := []engine.Trade{
		{EntryT: 200, PnL: 80, Partial: true, Reason: "partial"},
		{EntryT: 200, PnL: -50, Reason: "eod"},
		{EntryT: 100, PnL: -100, Reason: "sl"},
	}
	wantInput := append([]engine.Trade(nil), trades...)
	row := mustSummarize(t, "stable", 0, engine.RunResult{
		Costs:  engine.Costs{StartEquity: 1000},
		Trades: trades,
	})

	if !reflect.DeepEqual(trades, wantInput) {
		t.Fatalf("summarize mutated input trades\n got: %#v\nwant: %#v", trades, wantInput)
	}
	if row.Trades != 3 {
		t.Fatalf("trades = %d, want 3", row.Trades)
	}
	assertStatClose(t, "win rate", row.WinRate, 100.0/3.0)
	if row.PF == nil {
		t.Fatal("PF = nil, want 80/150")
	}
	assertStatClose(t, "profit factor", *row.PF, 80.0/150.0)
	assertStatClose(t, "net", row.Net, -70)
	assertStatClose(t, "expectancy", row.Expectancy, -70.0/3.0)
	// Chronological sorting puts the -100 trade first; stable equal-time order
	// then applies the +80 partial before the -50 remainder. An unstable tie
	// reorder or raw input order would each produce a larger drawdown instead.
	assertStatClose(t, "drawdown", row.DD, 10)
}

func TestSummarizeUsesLaterFinalPeakAsDrawdownDenominator(t *testing.T) {
	row := mustSummarize(t, "later peak", 0, engine.RunResult{
		Costs: engine.Costs{StartEquity: 1000},
		Trades: []engine.Trade{
			{EntryT: 100, PnL: -100},
			{EntryT: 200, PnL: 1100},
			{EntryT: 300, PnL: -50},
		},
	})

	if row.Trades != 3 || row.PF == nil {
		t.Fatalf("row = %+v, want three trades and finite PF", row)
	}
	assertStatClose(t, "win rate", row.WinRate, 100.0/3.0)
	assertStatClose(t, "profit factor", *row.PF, 1100.0/150.0)
	assertStatClose(t, "net", row.Net, 950)
	assertStatClose(t, "expectancy", row.Expectancy, 950.0/3.0)
	// The maximum absolute drawdown is 100, and the later final peak is 2000.
	assertStatClose(t, "drawdown", row.DD, 5)
}

func TestSummarizeRejectsNonFiniteDerivedFieldsInStableOrder(t *testing.T) {
	tests := []struct {
		name   string
		result engine.RunResult
		want   string
	}{
		{
			name: "gross win accumulation",
			result: engine.RunResult{Trades: []engine.Trade{
				{PnL: math.MaxFloat64},
				{PnL: math.MaxFloat64},
			}},
			want: "summary grossWin contains non-finite value",
		},
		{
			name: "gross loss accumulation",
			result: engine.RunResult{Trades: []engine.Trade{
				{PnL: -math.MaxFloat64},
				{PnL: -math.MaxFloat64},
			}},
			want: "summary grossLoss contains non-finite value",
		},
		{
			name: "equity accumulation",
			result: engine.RunResult{
				Costs:  engine.Costs{StartEquity: math.MaxFloat64},
				Trades: []engine.Trade{{PnL: math.MaxFloat64}},
			},
			want: "summary equity contains non-finite value",
		},
		{
			name: "numeric profit factor",
			result: engine.RunResult{Trades: []engine.Trade{
				{PnL: math.MaxFloat64},
				{PnL: -math.SmallestNonzeroFloat64},
			}},
			want: "summary profitFactor contains non-finite value",
		},
		{
			name: "percentage drawdown",
			result: engine.RunResult{
				Costs:  engine.Costs{StartEquity: math.SmallestNonzeroFloat64},
				Trades: []engine.Trade{{PnL: -math.MaxFloat64}},
			},
			want: "summary drawdown contains non-finite value",
		},
		{
			name: "gross win precedes combined failures",
			result: engine.RunResult{Trades: []engine.Trade{
				{PnL: math.MaxFloat64},
				{PnL: math.MaxFloat64},
				{PnL: -math.MaxFloat64},
				{PnL: -math.MaxFloat64},
			}},
			want: "summary grossWin contains non-finite value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, err := summarize("overflow", 0, tt.result)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("summarize error = %v, want %q", err, tt.want)
			}
			if row != (costRow{}) {
				t.Fatalf("summarize returned partial row on error: %+v", row)
			}
		})
	}
}

func mustSummarize(t *testing.T, label string, slippage float64, result engine.RunResult) costRow {
	t.Helper()
	row, err := summarize(label, slippage, result)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	return row
}

func assertStatClose(t *testing.T, label string, got, want float64) {
	t.Helper()
	tolerance := 1e-12 * math.Max(1, math.Abs(want))
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.16g, want %.16g (tolerance %.3g)", label, got, want, tolerance)
	}
}
