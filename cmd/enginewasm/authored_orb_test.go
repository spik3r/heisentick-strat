package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestAuthoredOrbWholeStrategyBridge(t *testing.T) {
	raw, err := os.ReadFile("../../authoredorb/testdata/parity-held-through-session-close.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Request json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	out, err := runAuthoredOrb(string(fixture.Request))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Schema     string `json:"schema"`
		StrategyID string `json:"strategyId"`
		Trades     []struct {
			Reason string `json:"reason"`
		} `json:"trades"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != "authored-orb-run-v1" || result.StrategyID != "daySessionOrbOriginalTvParity" || len(result.Trades) != 1 || result.Trades[0].Reason != "eod" {
		t.Fatalf("unexpected bridge result: %s", out)
	}
	if _, err := runAuthoredOrb(`{"schema":"authored-orb-run-v1","unknown":true}`); err == nil {
		t.Fatal("unknown request field admitted")
	}
}
