package main

import (
	"encoding/json"
	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The string bridge must preserve the existing file-loader semantics.
func TestBridgeMatchesFixtureLoader(t *testing.T) {
	paths, err := filepath.Glob("../../conformance/run/deployed-*.fixture.json")
	if err != nil || len(paths) != 6 {
		t.Fatalf("fixtures: %v (%d)", err, len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			fixture, err := native.LoadRunFixture(path)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(strings.TrimSuffix(path, ".fixture.json") + ".strat")
			if err != nil {
				t.Fatal(err)
			}
			result, err := native.RunFixtureCase(fixture, string(source))
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			got, err := runFixture(string(raw), string(source))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatal("string bridge differs from existing file loader")
			}
		})
	}
}

func TestColumnResultUsesFixedNumericRecordWidth(t *testing.T) {
	fixturePath := "../../conformance/run/deployed-dsl-session-expansion-ny.fixture.json"
	fixture, err := native.LoadRunFixture(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(strings.TrimSuffix(fixturePath, ".fixture.json") + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	columns := barsToColumns(fixture.Bars)
	meta, err := json.Marshal(columnRunMeta{Schema: "enginewasm-columnar-v1", Case: fixture.Case, StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs})
	if err != nil {
		t.Fatal(err)
	}
	got, err := runColumns(string(meta), string(source), columns)
	if err != nil {
		t.Fatal(err)
	}
	var summary struct {
		TradeCount int `json:"tradeCount"`
	}
	if err := json.Unmarshal([]byte(got.SummaryJSON), &summary); err != nil {
		t.Fatal(err)
	}
	if len(got.Values) != summary.TradeCount*columnTradeWidth {
		t.Fatalf("trade values=%d, want %d", len(got.Values), summary.TradeCount*columnTradeWidth)
	}
	var text []columnTradeText
	if err := json.Unmarshal([]byte(got.StringsJSON), &text); err != nil {
		t.Fatal(err)
	}
	if len(text) != summary.TradeCount {
		t.Fatalf("trade text=%d, want %d", len(text), summary.TradeCount)
	}
	var identity struct {
		Case string `json:"case"`
	}
	if err := json.Unmarshal([]byte(got.SummaryJSON), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Case != fixture.Case {
		t.Fatalf("case=%q, want %q", identity.Case, fixture.Case)
	}
}

func TestColumnRunRejectsUnsupportedInputs(t *testing.T) {
	columns := [6][]float64{{1}, {2}, {3}, {1}, {2}, {0}}
	meta := columnRunMeta{Schema: "enginewasm-columnar-v1", Symbol: "XAUUSD", Timeframe: "5m"}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runColumns(string(raw), "dsl v7", [6][]float64{{1}, {2, 3}, {3}, {1}, {2}, {0}}); err == nil {
		t.Fatal("mismatched columns were accepted")
	}
	meta.HigherTimeframe = "1h"
	raw, _ = json.Marshal(meta)
	if _, err := runColumns(string(raw), "dsl v7", columns); err == nil {
		t.Fatal("higher-timeframe metadata was accepted")
	}
	meta.HigherTimeframe = ""
	raw, _ = json.Marshal(meta)
	if _, err := runColumns(string(raw), "dsl v7\nstrategy \"source\" {}\nsetup { type: flag continuation source timeframe 1h }\nentryTf 5m", columns); err == nil {
		t.Fatal("DSL source-timeframe dependency was accepted")
	}
}

func TestColumnTradeTransportRetainsNonemptyTradeFields(t *testing.T) {
	want := native.Trade{Entry: 1, EntryIndex: 2, EntryT: 3, Exit: 4, ExitIndex: 5, ExitT: 6, InitialSL: 7, InitialTP: 8, PnL: 9, Points: 10, Size: 11, SL: 12, TP: 13, Side: "long", Reason: native.ReasonRule, Rule: "sma-bearish-cross", Tag: "A", Meta: native.TradeMeta{"window": "london"}, Partial: true, NoStop: true}
	result := native.RunResult{Trades: []native.Trade{want}}
	values, stringsJSON, _, err := packColumnResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != columnTradeWidth {
		t.Fatalf("numeric width=%d, want %d", len(values), columnTradeWidth)
	}
	var text []columnTradeText
	if err := json.Unmarshal([]byte(stringsJSON), &text); err != nil {
		t.Fatal(err)
	}
	if len(text) != 1 || text[0].Side != "long" || text[0].Reason != native.ReasonRule || text[0].Rule != "sma-bearish-cross" || text[0].Tag != "A" || text[0].Meta["window"] != "london" || !text[0].Partial || !text[0].NoStop {
		t.Fatalf("typed trade text lost fields: %#v", text)
	}
	got := reconstructColumnTrade(values, text[0])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("typed transport reconstruction differs\n got: %#v\nwant: %#v", got, want)
	}
}

func reconstructColumnTrade(values []float64, text columnTradeText) native.Trade {
	return native.Trade{Entry: values[0], EntryIndex: int(values[1]), EntryT: values[2], Exit: values[3], ExitIndex: int(values[4]), ExitT: values[5], InitialSL: values[6], InitialTP: values[7], PnL: values[8], Points: values[9], Size: values[10], SL: values[11], TP: values[12], Side: text.Side, Reason: text.Reason, Rule: text.Rule, Tag: text.Tag, Meta: text.Meta, Partial: text.Partial, NoStop: text.NoStop, NoTarget: text.NoTarget}
}

func barsToColumns(bars []marketdata.Bar) [6][]float64 {
	var columns [6][]float64
	for index := range columns {
		columns[index] = make([]float64, len(bars))
	}
	for index, bar := range bars {
		columns[0][index], columns[1][index], columns[2][index], columns[3][index], columns[4][index], columns[5][index] = bar.T, bar.O, bar.H, bar.L, bar.C, bar.V
	}
	return columns
}
