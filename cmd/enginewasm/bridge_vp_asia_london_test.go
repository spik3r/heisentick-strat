package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	native "github.com/spik3r/heisentick-strat/engine"
)

func TestAuthoredVPWideBridgeUsesActiveIDAndCompleteResult(t *testing.T) {
	raw, err := os.ReadFile("../../engine/testdata/asia-london-wide-interactive.fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := native.RunAuthoredVPAsiaLondonWideInteractive(raw)
	if err != nil {
		t.Fatal(err)
	}
	var got native.AuthoredVPAsiaLondonWideInteractiveResult
	bridgeRaw := runInteractive(string(raw), "")
	if err := json.Unmarshal(bridgeRaw, &got); err != nil {
		t.Fatal(err)
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bridgeRaw, wantRaw) || got.Run.TradeCount != 3 || len(got.EquityCurve) == 0 || len(got.ClosedEquity) == 0 {
		t.Fatalf("bridge result differs from native authored result: %+v", got)
	}
	var bad interactiveFailure
	if err := json.Unmarshal(runInteractive(string(raw), "dsl v7"), &bad); err != nil {
		t.Fatal(err)
	}
	if bad.Error.Code != "invalid-request" || bad.Error.Message == "" || bad.Schema != native.AuthoredVPAsiaLondonWideInteractiveSchema {
		t.Fatalf("authored ID silently ignored DSL source: %+v", bad)
	}
}
