package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFrozenFixtureAndColumnBridgeRefuse(t *testing.T) {
	source, err := os.ReadFile("../../dsl/testdata/frozen_level/example.strat")
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range []string{"XAUUSD", "WRONG"} {
		for _, bars := range []any{nil, []any{}, []any{[]any{0, 100, 101, 99, 100, 1}}, []any{[]any{nil, 100, 101, 99, 100, 1}}} {
			raw, _ := json.Marshal(map[string]any{"schema": "dsl-conformance-run-fixture-v1", "case": "frozen-refusal", "symbol": symbol, "timeframe": "1m", "bars": bars})
			if _, err := runFixture(string(raw), string(source)); err == nil || !strings.Contains(err.Error(), "not implemented") {
				t.Fatalf("fixture accepted %s: %v", raw, err)
			}
		}
	}
	for _, n := range []int{0, 1} {
		var cols [6][]float64
		for i := range cols {
			cols[i] = make([]float64, n)
		}
		if _, err := runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"1m"}`, string(source), cols); err == nil || !strings.Contains(err.Error(), "frozen-level-quote-execution-unimplemented") {
			t.Fatalf("column bridge admitted family: %v", err)
		}
	}
	if _, err := runFixture(`{"schema":"dsl-conformance-run-fixture-v1"}`, strings.Replace(string(source), "frozen lock scale", "frozen lock scale frozen lock pivot", 1)); err == nil || !strings.Contains(err.Error(), "DSL parse errors") {
		t.Fatalf("malformed source admitted: %v", err)
	}
}
