package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestParamsFromConfigMapsOpeningRangeBreakoutFixedATRStop(t *testing.T) {
	parsed, err := dsl.Parse(`dsl v7
strategy "ORB fixed ATR stop" {
}
setup {
  type: opening range breakout
}
risk {
  stop 1.5 ATR
}
`)
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse DSL diagnostics: %v", parsed.Errors)
	}

	params := paramsFromConfig(parsed.Config)
	if params.ORBStopATRMult != 1.5 {
		t.Fatalf("ORB fixed ATR stop = %v, want 1.5", params.ORBStopATRMult)
	}

	stop := mapValue(parsed.Config, "stop")
	stop["atrMult"] = 2.25
	params = paramsFromConfig(parsed.Config)
	if params.ORBStopATRMult != 2.25 {
		t.Fatalf("explicit atrMult stop = %v, want precedence over atr alias", params.ORBStopATRMult)
	}

	if got := paramsFromConfig(dsl.Config{}).ORBStopATRMult; got != 0 {
		t.Fatalf("default ORB fixed ATR stop = %v, want 0", got)
	}
}

func TestOpeningRangeBreakoutSetupUsesFixedATRStop(t *testing.T) {
	tests := []struct {
		name      string
		side      side
		closes    []float64
		rangeHigh float64
		rangeLow  float64
		wantStop  float64
	}{
		{name: "long", side: sideLong, closes: []float64{100, 102}, rangeHigh: 100, rangeLow: 90, wantStop: 99},
		{name: "short", side: sideShort, closes: []float64{100, 88}, rangeHigh: 110, rangeLow: 90, wantStop: 91},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := broker{
				series: marketdata.Series{C: tt.closes},
				cols:   contextcols.Columns{ATR: []float64{2, 2}},
				params: flagParams{
					AllowLong:      true,
					AllowShort:     true,
					ORBHoldCandles: 1,
					ORBStopATRMult: 1.5,
					ORBTargetR:     1,
					MinStopATR:     0,
					MaxStopATR:     math.Inf(1),
					StopBufferATR:  0.25,
				},
			}
			r := openingRange{
				Session:  "mid",
				EndIndex: 0,
				High:     tt.rangeHigh,
				Low:      tt.rangeLow,
				Size:     tt.rangeHigh - tt.rangeLow,
			}

			setup, ok := b.openingRangeBreakoutSetup(1, r, tt.side)
			if !ok {
				t.Fatal("fixed-ATR opening-range setup was rejected")
			}
			if setup.Stop != tt.wantStop {
				t.Fatalf("stop = %v, want %v", setup.Stop, tt.wantStop)
			}
		})
	}
}
