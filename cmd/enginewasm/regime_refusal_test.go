package main

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"strings"
	"testing"
)

const regimeUnsupportedSource = `dsl v7
strategy "Native only" { description "Synthetic" }
market { regime timeframe M30 from M5 }
setup { type: regime engine
regime profile v9-floor-half-v1
regime mode source-like-v1
}`

func TestRegimeWASMFixtureAndColumnsFailClosed(t *testing.T) {
	_, e := runFixture(`{"schema":"dsl-conformance-run-fixture-v1","bars":[]}`, regimeUnsupportedSource)
	if e == nil || !strings.Contains(e.Error(), dsl.RegimeEngineDedicatedRunnerRequired) {
		t.Fatalf("fixture:%v", e)
	}
	_, e = runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"30m"}`, regimeUnsupportedSource, [6][]float64{})
	if e == nil || !strings.Contains(e.Error(), dsl.RegimeEngineDedicatedRunnerRequired) {
		t.Fatalf("columns:%v", e)
	}
}
