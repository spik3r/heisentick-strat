package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func forwardSMAFixture(t *testing.T, count int) ([]byte, string) {
	t.Helper()
	path := filepath.Join("..", "conformance", "run", "deployed-dsl-sma-golden-cross-xauusd-one-minute-canary")
	raw, err := os.ReadFile(path + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	for key := range input {
		if key != "schema" && key != "case" && key != "strategyId" && key != "symbol" && key != "timeframe" && key != "rangeMethod" && key != "costs" && key != "bars" {
			delete(input, key)
		}
	}
	input["bars"] = input["bars"].([]any)[:count]
	input["rangeMethod"] = "zone"
	input["costs"] = map[string]any{"fillOn": "close", "feePerUnit": 0, "slippage": 0, "slippageBps": 0, "startEquity": 10000}
	source, err := os.ReadFile(path + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return raw, string(source)
}

func TestForwardPrefixFixturePreservesExposureAndStableIdentity(t *testing.T) {
	raw, source := forwardSMAFixture(t, 313)
	open, err := RunForwardPrefixFixture(raw, source)
	if err != nil {
		t.Fatal(err)
	}
	if open.Schema != ForwardPrefixSchema || len(open.Result.Trades) != 0 || len(open.Result.OpenPositions) != 1 || open.Result.OpenPositions[0].EntryIndex != 312 {
		t.Fatalf("prefix must preserve first SMA exposure: %+v", open)
	}
	raw, _ = forwardSMAFixture(t, 317)
	closed, err := RunForwardPrefixFixture(raw, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(closed.Result.Trades) != 1 || len(closed.Result.OpenPositions) != 0 || closed.Result.Trades[0].Trade.ExitIndex != 316 || closed.Result.Trades[0].PositionID != open.Result.OpenPositions[0].PositionID {
		t.Fatalf("extended prefix must close the same position once: %+v", closed)
	}
	if len(closed.Provenance.FixtureSHA256) != 64 || closed.Provenance.SourceSHA256 != open.Provenance.SourceSHA256 || closed.Provenance.FixtureSHA256 == open.Provenance.FixtureSHA256 {
		t.Fatal("exact byte provenance did not distinguish the two prefixes")
	}
}

func TestForwardPrefixFixtureRejectsIncompleteOrUnqualifiedInput(t *testing.T) {
	raw, source := forwardSMAFixture(t, 313)
	for _, tc := range []struct{ name, from, to string }{
		{"null cost", `"feePerUnit":0`, `"feePerUnit":null`},
		{"cost alias", `"fillOn":"close"`, `"FillOn":"close"`},
		{"missing cost", `"slippageBps":0,`, ``},
		{"null cell", `"bars":[[`, `"bars":[[null,`},
		{"strategy alias", `"strategyId":`, `"StrategyId":`},
		{"null symbol", `"symbol":"XAUUSD"`, `"symbol":null`},
		{"off route", `"timeframe":"1m"`, `"timeframe":"5m"`},
		{"ignored HTF", `"schema":`, `"higherTimeframe":"1h","schema":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutated := strings.Replace(string(raw), tc.from, tc.to, 1)
			if mutated == string(raw) {
				t.Fatal("test mutation did not match input")
			}
			if _, err := RunForwardPrefixFixture([]byte(mutated), source); err == nil {
				t.Fatal("invalid Forward input was accepted")
			}
		})
	}
}

func TestForwardPrefixFixtureRejectsMalformedBarSequences(t *testing.T) {
	raw, source := forwardSMAFixture(t, 313)
	for _, name := range []string{"reverse", "duplicate", "fractional time", "negative time", "high", "low", "negative volume"} {
		t.Run(name, func(t *testing.T) {
			var input map[string]any
			if err := json.Unmarshal(raw, &input); err != nil {
				t.Fatal(err)
			}
			bars := input["bars"].([]any)
			first, second := bars[0].([]any), bars[1].([]any)
			switch name {
			case "reverse":
				first[0], second[0] = second[0], first[0]
			case "duplicate":
				second[0] = first[0]
			case "fractional time":
				first[0] = first[0].(float64) + 0.5
			case "negative time":
				first[0] = -1.0
			case "high":
				first[2] = 0.0
			case "low":
				first[3] = 1e6
			case "negative volume":
				first[5] = -1.0
			}
			mutated, _ := json.Marshal(input)
			if _, err := RunForwardPrefixFixture(mutated, source); err == nil {
				t.Fatal("malformed Forward bars were accepted")
			}
		})
	}
}
