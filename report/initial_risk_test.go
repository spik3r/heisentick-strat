package report

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestReportExcursionTreatsZeroDistanceInitialStopAsAuthoritative(t *testing.T) {
	bars := []marketdata.Bar{{H: 102, L: 99}, {H: 105, L: 98}, {H: 106, L: 97}}
	series := marketdata.SeriesFromBars(bars)
	trade := engine.Trade{
		Entry: 100, Exit: 101, InitialSL: 100, SL: 98,
		EntryIndex: 0, ExitIndex: 1, Side: "long",
	}
	tradeBefore := trade
	seriesBefore := make([]marketdata.Bar, series.Len())
	for i := range seriesBefore {
		seriesBefore[i] = series.Bar(i)
	}

	if got := reportTradeExcursion(trade, series); got != nil {
		t.Fatalf("zero-distance initial-risk excursion = %#v, want nil", got)
	}
	annotated, err := annotateReportTrades([]engine.Trade{trade}, series, nil, reportTradeRoute{})
	if err != nil {
		t.Fatalf("annotate zero-distance initial risk: %v", err)
	}
	if len(annotated) != 1 {
		t.Fatalf("annotated trades = %d, want 1", len(annotated))
	}
	got := annotated[0]
	if got.ReportMfeR != nil || got.ReportMaeR != nil || got.ReportTimeToMfeBars != nil || got.ReportPostExitMfeR != nil || got.ReportPostExitMaeR != nil {
		t.Fatalf("zero-distance initial risk has excursion annotations: %#v", got)
	}
	if !reflect.DeepEqual(got.Trade, trade) {
		t.Fatalf("annotated trade changed input fields: got %#v want %#v", got.Trade, trade)
	}

	payload := Document{TradeSchema: TradesSchema, Slices: []Slice{{Trades: &annotated}}}
	var out bytes.Buffer
	if err := WriteJSON(&out, payload); err != nil {
		t.Fatalf("write annotated report JSON: %v", err)
	}
	var raw struct {
		Slices []struct {
			Trades []map[string]any `json:"trades"`
		} `json:"slices"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("decode annotated report JSON: %v", err)
	}
	serialized := raw.Slices[0].Trades[0]
	for _, field := range []string{"reportMfeR", "reportMaeR", "reportTimeToMfeBars", "reportPostExitMfeR", "reportPostExitMaeR"} {
		if _, exists := serialized[field]; exists {
			t.Fatalf("zero-distance initial risk serialized %s: %#v", field, serialized[field])
		}
	}

	if !reflect.DeepEqual(trade, tradeBefore) {
		t.Fatalf("input trade mutated: got %#v want %#v", trade, tradeBefore)
	}
	for i, want := range seriesBefore {
		if got := series.Bar(i); !reflect.DeepEqual(got, want) {
			t.Fatalf("series bar %d mutated: got %#v want %#v", i, got, want)
		}
	}
}

func TestReportExcursionKeepsPositiveInitialRiskAfterStopMovement(t *testing.T) {
	tests := []struct {
		name  string
		trade engine.Trade
		bars  []marketdata.Bar
	}{
		{
			name: "long moved to breakeven",
			trade: engine.Trade{
				Entry: 100, Exit: 101, InitialSL: 98, SL: 100,
				EntryIndex: 0, ExitIndex: 1, Side: "long",
			},
			bars: []marketdata.Bar{{H: 102, L: 99}, {H: 105, L: 98}, {H: 106, L: 97}},
		},
		{
			name: "short moved to breakeven",
			trade: engine.Trade{
				Entry: 100, Exit: 99, InitialSL: 102, SL: 100,
				EntryIndex: 0, ExitIndex: 1, Side: "short",
			},
			bars: []marketdata.Bar{{H: 101, L: 98}, {H: 102, L: 95}, {H: 103, L: 94}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := reportTradeExcursion(tt.trade, marketdata.SeriesFromBars(tt.bars))
			if summary == nil {
				t.Fatal("positive initial risk produced no excursion")
			}
			if summary.mfeR != 2.5 || summary.maeR != 1 || summary.timeToMfeBars != 1 {
				t.Fatalf("primary excursion = %#v, want MFE=2.5 MAE=1 time=1", summary)
			}
			if summary.postExitMfeR == nil || summary.postExitMaeR == nil || *summary.postExitMfeR != 2.5 || *summary.postExitMaeR != 2 {
				t.Fatalf("post-exit excursion = %#v, want MFE=2.5 MAE=2", summary)
			}
		})
	}
}
