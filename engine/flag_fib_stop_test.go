package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestParamsFromConfigMapsFlagFibStop(t *testing.T) {
	tests := []struct {
		name      string
		stop      map[string]any
		wantType  string
		wantRatio float64
	}{
		{name: "defaults", stop: map[string]any{}, wantRatio: 0.618},
		{name: "compiled fib stop", stop: map[string]any{"type": "fibRetrace", "fibRatio": 0.786}, wantType: "fibRetrace", wantRatio: 0.786},
		{name: "explicit zero preserved for runtime fallback", stop: map[string]any{"type": "fibRetrace", "fibRatio": 0.0}, wantType: "fibRetrace", wantRatio: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := paramsFromConfig(dsl.Config{"setupType": string(dsl.FamilyFlagContinuation), "stop": tt.stop})
			if params.StopType != tt.wantType || params.StopFibRatio != tt.wantRatio {
				t.Fatalf("flag fib stop mapping = %q/%v, want %q/%v", params.StopType, params.StopFibRatio, tt.wantType, tt.wantRatio)
			}
		})
	}
}

func TestApplyFlagCompiledStopMirrorsJavaScriptFormula(t *testing.T) {
	base := flagSetup{
		Stop:   95,
		Target: 120,
		Meta:   TradeMeta{"poleHi": 110.0, "poleLo": 90.0},
	}
	tests := []struct {
		name       string
		side       side
		params     flagParams
		wantStop   float64
		wantTarget float64
	}{
		{
			name:       "long retraces from pole high with downside padding",
			side:       sideLong,
			params:     flagParams{StopType: "fibRetrace", StopFibRatio: 0.618, StopBufferATR: 0.25},
			wantStop:   97.14,
			wantTarget: 120,
		},
		{
			name:       "short retraces from pole low with upside padding",
			side:       sideShort,
			params:     flagParams{StopType: "fibRetrace", StopFibRatio: 0.618, StopBufferATR: 0.25},
			wantStop:   102.86,
			wantTarget: 120,
		},
		{
			name:       "zero ratio uses JavaScript fallback",
			side:       sideLong,
			params:     flagParams{StopType: "fibRetrace", StopBufferATR: 0.25},
			wantStop:   97.14,
			wantTarget: 120,
		},
		{
			name:       "non fib stop is unchanged",
			side:       sideLong,
			params:     flagParams{StopType: "recentExtreme", StopFibRatio: 0.786, StopBufferATR: 0.25},
			wantStop:   95,
			wantTarget: 120,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyFlagCompiledStop(base, 2, tt.side, tt.params)
			if math.Abs(got.Stop-tt.wantStop) > 1e-12 || got.Target != tt.wantTarget {
				t.Fatalf("compiled flag stop/target = %v/%v, want %v/%v", got.Stop, got.Target, tt.wantStop, tt.wantTarget)
			}
		})
	}
}

func TestPublicRunConsumesFlagFibStopConfig(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "deployed-dsl-flag-continuation-one-four-hour-review")
	stop := mapValue(cfg, "stop")
	stop["type"] = "fibRetrace"
	stop["fibRatio"] = 0.618
	stop["paddingAtr"] = 0.1

	result, err := Run(RunRequest{
		Config:          cfg,
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
	if result.TradeCount != 18 || len(result.Trades) != 18 {
		t.Fatalf("fib-stop trades = %d/%d, want D19 isolated-fixture count 18", result.TradeCount, len(result.Trades))
	}
	first := result.Trades[0]
	if first.EntryIndex != 66 || math.Abs(first.InitialSL-1901.6747757142857) > 1e-9 || math.Abs(first.InitialTP-1924.8543714285713) > 1e-9 {
		t.Fatalf("first long fib-stop trade entry/sl/tp = %d/%v/%v, want 66/1901.6747757142857/1924.8543714285713", first.EntryIndex, first.InitialSL, first.InitialTP)
	}
	var firstShort *Trade
	for i := range result.Trades {
		if result.Trades[i].Side == "short" {
			firstShort = &result.Trades[i]
			break
		}
	}
	if firstShort == nil {
		t.Fatal("compiled flag fib-stop run produced no short trade")
	}
	if firstShort.EntryIndex != 85 || math.Abs(firstShort.InitialSL-1901.4309014285714) > 1e-9 || math.Abs(firstShort.InitialTP-1880.9284228571428) > 1e-9 {
		t.Fatalf("first short fib-stop trade entry/sl/tp = %d/%v/%v, want 85/1901.4309014285714/1880.9284228571428", firstShort.EntryIndex, firstShort.InitialSL, firstShort.InitialTP)
	}
}
