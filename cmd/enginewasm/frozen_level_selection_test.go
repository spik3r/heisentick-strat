package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFrozenMalformedSelectorCannotExecuteLegacyTrades(t *testing.T) {
	const base = "../../conformance/run/research-dsl-failed-breakout-five-minute-early-breakeven"
	fixture, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	positive, err := runFixture(string(fixture), string(source))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		TradeCount int `json:"tradeCount"`
	}
	if err := json.Unmarshal(positive, &result); err != nil || result.TradeCount != 5 {
		t.Fatalf("legacy five-trade positive control: %v %+v", err, result)
	}
	var parsedFixture map[string]any
	if err := json.Unmarshal(fixture, &parsedFixture); err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{"schema": "enginewasm-columnar-v1"}
	for _, key := range []string{"case", "strategyId", "symbol", "timeframe", "rangeMethod", "costs"} {
		meta[key] = parsedFixture[key]
	}
	metaRaw, _ := json.Marshal(meta)
	var actualColumns [6][]float64
	for _, row := range parsedFixture["bars"].([]any) {
		for j, value := range row.([]any) {
			if j < 6 {
				actualColumns[j] = append(actualColumns[j], value.(float64))
			}
		}
	}
	columnPositive, err := runColumns(string(metaRaw), string(source), actualColumns)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(columnPositive.SummaryJSON), &result); err != nil || result.TradeCount != 5 {
		t.Fatalf("column five-trade positive control: %v %+v", err, result)
	}
	for _, selector := range []string{`"frozen level breakout"`, `'frozen level breakout'`, "`frozen level breakout`", `frozenLevelBreakout`} {
		attempt := "dsl v7\nsetup { type: " + selector + " }\n" + string(source)
		if _, err := runFixture(string(fixture), attempt); err == nil || !strings.Contains(err.Error(), "DSL parse errors") {
			t.Errorf("malformed selector %s executed legacy fixture: %v", selector, err)
		}
		var columns [6][]float64
		if _, err := runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"5m"}`, attempt, columns); err == nil || !strings.Contains(err.Error(), "DSL parse errors") {
			t.Errorf("malformed selector %s passed column bridge: %v", selector, err)
		}
	}
	for _, body := range []string{
		"symbols XAUUSD type: frozen level breakout type: failed breakout",
		`symbols XAUUSD type: "frozen level breakout" type: failed breakout`,
		"description free type: frozen level breakout type: failed breakout",
		"(type: frozen level breakout) type: failed breakout",
	} {
		attempt := "dsl v7\nsetup { " + body + " }\n" + string(source)
		if _, err := runFixture(string(fixture), attempt); err == nil || !strings.Contains(err.Error(), "DSL parse errors") {
			t.Errorf("inline selector ran fixture: %v", err)
		}
		if _, err := runColumns(string(metaRaw), attempt, actualColumns); err == nil || !strings.Contains(err.Error(), "DSL parse errors") {
			t.Errorf("inline selector ran actual columns: %v", err)
		}
	}
}
