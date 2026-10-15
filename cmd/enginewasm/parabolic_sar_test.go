package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParabolicSARBridgeRejectsMalformedRows(t *testing.T) {
	raw, err := os.ReadFile("../../conformance/run/family-named-level-sweep.fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../conformance/run/family-named-level-sweep.strat")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source) + "\nfilters { sar trend bearish }\n"
	if _, err := runFixture(string(raw), text); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(f map[string]any) { f["bars"].([]any)[2].([]any)[3] = nil },
		func(f map[string]any) { f["bars"].([]any)[2] = nil },
		func(f map[string]any) { f["bars"].([]any)[2] = []any{1., 2., 3.} },
		func(f map[string]any) { r := f["bars"].([]any)[2].([]any); f["bars"].([]any)[2] = append(r, 0.) },
		func(f map[string]any) { f["bars"].([]any)[2].([]any)[1] = 102. },
		func(f map[string]any) { bars := f["bars"].([]any); bars[2].([]any)[0] = bars[1].([]any)[0] },
	} {
		var fixture map[string]any
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatal(err)
		}
		mutate(fixture)
		bad, err := json.Marshal(fixture)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := runFixture(string(bad), text); err == nil {
			t.Fatalf("malformed SAR fixture accepted: %s", got)
		}
	}
}
