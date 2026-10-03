package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/spik3r/heisentick-strat/engine"
)

func TestForwardSmokeNativeWASMAdapter(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "engine", "testdata", "forward-smoke", "btc-close-cost.json"))
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	input["schema"] = "forward-smoke-request-v1"
	input["mode"] = "whole"
	input["strategyVersion"] = native.ForwardSmokeBTCVersion
	delete(input, "name")
	delete(input, "wholeTrades")
	delete(input, "prefix")
	encoded, _ := json.Marshal(input)
	out, err := runForwardSmokeJSON(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Schema          string `json:"schema"`
		StrategyVersion string `json:"strategyVersion"`
		Result          struct {
			Schema     string `json:"schema"`
			TradeCount int    `json:"tradeCount"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Schema != "forward-smoke-result-v1" || envelope.StrategyVersion != native.ForwardSmokeBTCVersion || envelope.Result.Schema != "forward-smoke-trades-v1" || envelope.Result.TradeCount != 4 {
		t.Fatalf("adapter output = %s", out)
	}
	input["mode"] = "prefix"
	encoded, _ = json.Marshal(input)
	out, err = runForwardSmokeJSON(string(encoded))
	if err != nil || !strings.Contains(string(out), `"openPositions"`) {
		t.Fatalf("prefix output = %s, %v", out, err)
	}
	input["strategyVersion"] = "wrong"
	encoded, _ = json.Marshal(input)
	if _, err := runForwardSmokeJSON(string(encoded)); err == nil {
		t.Fatal("unknown version succeeded")
	}
}

func TestForwardSmokeOpenAdapterPreservesPrefixIdentity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "engine", "testdata", "forward-smoke", "btc-open-cost.json"))
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	input["schema"] = "forward-smoke-request-v1"
	input["mode"] = "whole"
	delete(input, "name")
	delete(input, "wholeTrades")
	delete(input, "prefix")
	encoded, _ := json.Marshal(input)
	out, err := runForwardSmokeJSON(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	var whole struct {
		Result native.RunResult `json:"result"`
	}
	if err := json.Unmarshal(out, &whole); err != nil {
		t.Fatal(err)
	}
	wantEntries := []int{1, 2, 6, 8}
	if len(whole.Result.Trades) != len(wantEntries) {
		t.Fatalf("open-mode trades = %d", len(whole.Result.Trades))
	}
	for i, want := range wantEntries {
		if whole.Result.Trades[i].EntryIndex != want {
			t.Fatalf("open-mode trade %d entry = %d, want %d", i, whole.Result.Trades[i].EntryIndex, want)
		}
	}
	input["mode"] = "prefix"
	allBars := input["bars"].([]any)
	input["bars"] = allBars[:2]
	encoded, _ = json.Marshal(input)
	out, err = runForwardSmokeJSON(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	var first struct {
		Result native.PrefixResult `json:"result"`
	}
	if err := json.Unmarshal(out, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Result.Trades) != 0 || len(first.Result.OpenPositions) != 1 || first.Result.OpenPositions[0].EntryIndex != 1 {
		t.Fatalf("open-mode two-bar exposure = %+v", first.Result)
	}
	input["bars"] = allBars[:3]
	encoded, _ = json.Marshal(input)
	out, err = runForwardSmokeJSON(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	var second struct {
		Result native.PrefixResult `json:"result"`
	}
	if err := json.Unmarshal(out, &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Result.Trades) != 1 || second.Result.Trades[0].PositionID != first.Result.OpenPositions[0].PositionID || len(second.Result.OpenPositions) != 1 || second.Result.OpenPositions[0].EntryIndex != 2 {
		t.Fatalf("open-mode three-bar restart = %+v", second.Result)
	}
}
