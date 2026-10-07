package main

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"strings"
	"testing"
)

const masterUnsupportedSource = `dsl v7
strategy "Native only" { description "Synthetic" }
market { master timeframe M30 from M5 }
setup { type: master structural
master profile v10-phase0-floor-half-reference-v1
master mode SOURCE_HISTORICAL_REFERENCE
}`

func TestMasterWASMFixtureAndColumnsFailClosed(t *testing.T) {
	_, e := runFixture(`{"schema":"dsl-conformance-run-fixture-v1","bars":[]}`, masterUnsupportedSource)
	if e == nil || !strings.Contains(e.Error(), dsl.MasterStructuralDedicatedRunnerRequired) {
		t.Fatalf("fixture:%v", e)
	}
	_, e = runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"30m"}`, masterUnsupportedSource, [6][]float64{})
	if e == nil || !strings.Contains(e.Error(), dsl.MasterStructuralDedicatedRunnerRequired) {
		t.Fatalf("columns:%v", e)
	}
}

func TestMasterAuthoredTailWASMRefusal(t *testing.T) {
	for _, clause := range []string{"risk 100 USD master mode SOURCE_HISTORICAL_REFERENCE", "stop 2 ATR master profile v10-phase0-floor-half-reference-v1", "stop 2 ATR masterStructural null", "unknown masterStructural null", "type: flag continuation master\nmode SOURCE_HISTORICAL_REFERENCE"} {
		source := "dsl v7\nstrategy \"Adversarial\"\nsetup { " + clause + " }"
		if _, e := runFixture(`{"schema":"dsl-conformance-run-fixture-v1","symbol":"XAUUSD","timeframe":"30m","bars":[]}`, source); e == nil {
			t.Fatal("WASM fixture accepted tail", clause)
		}
		if _, e := runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"30m"}`, source, [6][]float64{{1800000}, {100}, {101}, {99}, {100}, {1}}); e == nil {
			t.Fatal("WASM columns accepted tail", clause)
		}
	}
}
