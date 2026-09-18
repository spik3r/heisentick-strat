package engine

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestParamsFromConfigMapsBreakRetestFibStop(t *testing.T) {
	tests := []struct {
		name      string
		stop      map[string]any
		wantType  string
		wantRatio float64
	}{
		{name: "defaults", stop: map[string]any{}, wantRatio: 0.618},
		{name: "compiled fib stop", stop: map[string]any{"type": "fibRetrace", "fibRatio": 0.786}, wantType: "fibRetrace", wantRatio: 0.786},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := paramsFromConfig(dsl.Config{"setupType": string(dsl.FamilyBreakRetest), "stop": tt.stop})
			if params.BRStopType != tt.wantType || params.BRStopFibRatio != tt.wantRatio {
				t.Fatalf("break-retest fib stop mapping = %q/%v, want %q/%v", params.BRStopType, params.BRStopFibRatio, tt.wantType, tt.wantRatio)
			}
		})
	}
}

func TestBreakRetestFibStopMirrorsSideAwareFormula(t *testing.T) {
	tests := []struct {
		name         string
		levelPrice   float64
		breakExtreme float64
		side         side
		ratio        float64
		want         float64
	}{
		{name: "long", levelPrice: 100, breakExtreme: 110, side: sideLong, ratio: 0.618, want: 103.32},
		{name: "short", levelPrice: 100, breakExtreme: 90, side: sideShort, ratio: 0.618, want: 96.68},
		{name: "zero ratio uses JavaScript fallback", levelPrice: 100, breakExtreme: 110, side: sideLong, ratio: 0, want: 103.32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := breakRetestFibStop(tt.levelPrice, tt.breakExtreme, 2, tt.side, tt.ratio, 0.25); got != tt.want {
				t.Fatalf("fib stop = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPublicRunConsumesBreakRetestFibStopConfig(t *testing.T) {
	const caseName = "family-break-retest"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil {
		t.Fatalf("parse DSL: %v", err)
	}
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse DSL diagnostics: %v", parsed.Errors)
	}
	stop := mapValue(parsed.Config, "stop")
	stop["type"] = "fibRetrace"
	stop["fibRatio"] = 0.618
	stop["paddingAtr"] = 0.25

	result, err := Run(RunRequest{
		Config:          parsed.Config,
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	})
	if err != nil {
		t.Fatalf("run compiled config: %v", err)
	}
	if result.TradeCount != 14 || len(result.Trades) != 14 {
		t.Fatalf("fib-stop trades = %d/%d, want JavaScript parity count 14", result.TradeCount, len(result.Trades))
	}
	first := result.Trades[0]
	if first.EntryIndex != 315 || math.Abs(first.InitialSL-1802.1697664285714) > 1e-9 || math.Abs(first.InitialTP-1774.50084285714) > 1e-9 {
		t.Fatalf("first fib-stop trade entry/sl/tp = %d/%v/%v, want 315/1802.1697664285714/1774.50084285714", first.EntryIndex, first.InitialSL, first.InitialTP)
	}
}
