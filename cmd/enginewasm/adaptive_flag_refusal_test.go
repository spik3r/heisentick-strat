package main

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"strings"
	"testing"
)

const adaptiveFlagUnsupportedSource = `dsl v7
strategy "Adaptive volume flag INITIAL synthetic control" {
  description "Input identity only; dedicated raw reference runner"
}
market {
  adaptiveflag timeframe M30
}
setup {
  type: adaptive volume flag
  adaptiveflag policy DELAYED_OHLC_REFERENCE_V1
  adaptiveflag bundle INITIAL
  adaptiveflag pivotSensitivity 3
  adaptiveflag minPoleATR 1.8
  adaptiveflag minFlagBars 3
  adaptiveflag maxFlagBars 16
  adaptiveflag maxFlagRetrace 0.5
  adaptiveflag useVolumeFilter true
  adaptiveflag useEMATrend true
  adaptiveflag fastEMALen 50
  adaptiveflag slowEMALen 200
  adaptiveflag targetR 2.5
  adaptiveflag atrStopMult 1.2
  adaptiveflag validBars 12
  adaptiveflag maxHold 60
  adaptiveflag atrLen 14
  adaptiveflag volumeSMALen 20
  adaptiveflag volumeSMAMult 0.9
  adaptiveflag flagWidthPoleMult 0.55
  adaptiveflag entryBufferATR 0.05
}
`

func TestAdaptiveFlagWASMFixtureAndColumnsFailClosed(t *testing.T) {
	_, e := runFixture(`{"schema":"dsl-conformance-run-fixture-v1","bars":[]}`, adaptiveFlagUnsupportedSource)
	if e == nil || !strings.Contains(e.Error(), dsl.AdaptiveFlagDedicatedRunnerRequired) {
		t.Fatalf("fixture:%v", e)
	}
	_, e = runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"30m"}`, adaptiveFlagUnsupportedSource, [6][]float64{})
	if e == nil || !strings.Contains(e.Error(), dsl.AdaptiveFlagDedicatedRunnerRequired) {
		t.Fatalf("columns:%v", e)
	}
}

func TestAdaptiveFlagAuthoredTailWASMRefusal(t *testing.T) {
	for _, clause := range []string{"risk 100 USD adaptiveflag policy DELAYED_OHLC_REFERENCE_V1", "stop 2 ATR adaptiveflag policy DELAYED_OHLC_REFERENCE_V1", "stop 2 ATR adaptiveVolumeFlag null", "unknown adaptiveVolumeFlag null", "type: flag continuation adaptiveflag\npolicy DELAYED_OHLC_REFERENCE_V1"} {
		source := "dsl v7\nstrategy \"Adversarial\"\nsetup { " + clause + " }"
		if _, e := runFixture(`{"schema":"dsl-conformance-run-fixture-v1","symbol":"XAUUSD","timeframe":"30m","bars":[]}`, source); e == nil {
			t.Fatal("WASM fixture accepted tail", clause)
		}
		if _, e := runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"XAUUSD","timeframe":"30m"}`, source, [6][]float64{{1800000}, {100}, {101}, {99}, {100}, {1}}); e == nil {
			t.Fatal("WASM columns accepted tail", clause)
		}
	}
}

func TestAdaptiveFlagPunctuationTailWASMRefusal(t *testing.T) {
	for _, head := range []string{"adaptive:flag", "adaptive.flag", "adaptive/flag", "adaptive | flag"} {
		source := "dsl v7\nstrategy \"Review\"\nsetup { type: flag continuation }\nrisk { stop 2 ATR " + head + " targetR 2.5 }"
		if _, e := runFixture(`{"schema":"dsl-conformance-run-fixture-v1","symbol":"SYNTHETIC","timeframe":"30m","bars":[]}`, source); e == nil {
			t.Fatalf("WASM fixture accepted %q", head)
		}
		if _, e := runColumns(`{"schema":"enginewasm-columnar-v1","symbol":"SYNTHETIC","timeframe":"30m"}`, source, [6][]float64{}); e == nil {
			t.Fatalf("WASM columns accepted %q", head)
		}
	}
}
