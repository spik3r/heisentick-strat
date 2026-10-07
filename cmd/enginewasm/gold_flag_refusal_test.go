package main

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"strings"
	"testing"
)

const goldFlagUnsupportedSource = `dsl v7
strategy "Native only" { description "Synthetic" }
market { goldflag timeframe M30 from M15 }
setup { type: gold flag reference
goldflag policy PR388_CAUSAL_STRESS_V1
}`

func TestGoldFlagWASMFixtureAndColumnsFailClosed(t *testing.T) {
	_, e := runFixture(`{"schema":"dsl-conformance-run-fixture-v1","bars":[]}`, goldFlagUnsupportedSource)
	if e == nil || !strings.Contains(e.Error(), dsl.GoldFlagReferenceDedicatedRunnerRequired) {
		t.Fatalf("fixture:%v", e)
	}
	_, e = runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"30m"}`, goldFlagUnsupportedSource, [6][]float64{})
	if e == nil || !strings.Contains(e.Error(), dsl.GoldFlagReferenceDedicatedRunnerRequired) {
		t.Fatalf("columns:%v", e)
	}
}

func TestGoldFlagAuthoredTailWASMRefusal(t *testing.T) {
	for _, clause := range []string{"risk 100 USD goldflag policy PR388_CAUSAL_STRESS_V1", "stop 2 ATR goldflag policy PR388_CAUSAL_STRESS_V1", "stop 2 ATR goldFlagReference null", "unknown goldFlagReference null", "type: flag continuation goldflag\npolicy PR388_CAUSAL_STRESS_V1"} {
		source := "dsl v7\nstrategy \"Adversarial\"\nsetup { " + clause + " }"
		if _, e := runFixture(`{"schema":"dsl-conformance-run-fixture-v1","symbol":"XAUUSD","timeframe":"30m","bars":[]}`, source); e == nil {
			t.Fatal("WASM fixture accepted tail", clause)
		}
		if _, e := runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"30m"}`, source, [6][]float64{{1800000}, {100}, {101}, {99}, {100}, {1}}); e == nil {
			t.Fatal("WASM columns accepted tail", clause)
		}
	}
}
