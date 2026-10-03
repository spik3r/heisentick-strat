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
