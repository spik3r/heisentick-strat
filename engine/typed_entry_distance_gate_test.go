package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestParamsFromConfigSeparatesTypedAndRangeBreakFakeEntryDistance(t *testing.T) {
	tests := []struct {
		name        string
		cfg         dsl.Config
		wantGeneric float64
		wantRBF     float64
	}{
		{
			name:        "missing",
			cfg:         dsl.Config{},
			wantGeneric: 0,
			wantRBF:     0.6,
		},
		{
			name: "explicit generic zero",
			cfg: dsl.Config{
				"trigger": map[string]any{"maxEntryDistanceAtr": 0.0},
			},
			wantGeneric: 0,
			wantRBF:     0,
		},
		{
			name: "generic directive value",
			cfg: dsl.Config{
				"trigger": map[string]any{"maxEntryDistanceAtr": 0.5},
			},
			wantGeneric: 0.5,
			wantRBF:     0.5,
		},
		{
			name: "range break fake override remains local",
			cfg: dsl.Config{
				"trigger":        map[string]any{"maxEntryDistanceAtr": 0.5},
				"rangeBreakFake": map[string]any{"maxEntryDistanceAtr": 0.4},
			},
			wantGeneric: 0.5,
			wantRBF:     0.4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := paramsFromConfig(tt.cfg)
			if params.MaxEntryDistanceATR != tt.wantGeneric || params.RBFMaxEntryDistanceATR != tt.wantRBF {
				t.Fatalf("entry distances = generic %g, RBF %g; want %g, %g",
					params.MaxEntryDistanceATR, params.RBFMaxEntryDistanceATR, tt.wantGeneric, tt.wantRBF)
			}
		})
	}

	_, parsedRBF := loadEntryAttemptCase(t, "family-range-break-fake")
	params := paramsFromConfig(parsedRBF)
	if params.MaxEntryDistanceATR != 0.5 || params.RBFMaxEntryDistanceATR != 0.5 {
		t.Fatalf("authored RBF directive mapped to generic %g, RBF %g; want 0.5, 0.5",
			params.MaxEntryDistanceATR, params.RBFMaxEntryDistanceATR)
	}
}

func TestTypedEntryDistanceBoundaryMatchesJS(t *testing.T) {
	newBroker := func(close, atr, maxDistance float64, setupType string) broker {
		return broker{
			series: marketdata.Series{C: []float64{close}},
			cols:   contextcols.Columns{ATR: []float64{atr}},
			params: flagParams{SetupType: setupType, MaxEntryDistanceATR: maxDistance},
		}
	}

	tests := []struct {
		name      string
		broker    broker
		meta      TradeMeta
		wantAllow bool
	}{
		{name: "exact boundary", broker: newBroker(105, 10, 0.5, "breakRetest"), meta: TradeMeta{"levelPrice": 100.0}, wantAllow: true},
		{name: "beyond boundary", broker: newBroker(105.0001, 10, 0.5, "breakRetest"), meta: TradeMeta{"levelPrice": 100.0}, wantAllow: false},
		{name: "missing level", broker: newBroker(106, 10, 0.5, "breakRetest"), meta: TradeMeta{}, wantAllow: true},
		{name: "NaN level", broker: newBroker(106, 10, 0.5, "breakRetest"), meta: TradeMeta{"levelPrice": math.NaN()}, wantAllow: true},
		{name: "zero ATR", broker: newBroker(106, 0, 0.5, "breakRetest"), meta: TradeMeta{"levelPrice": 100.0}, wantAllow: true},
		{name: "NaN ATR", broker: newBroker(106, math.NaN(), 0.5, "breakRetest"), meta: TradeMeta{"levelPrice": 100.0}, wantAllow: true},
		{name: "missing ATR", broker: broker{series: marketdata.Series{C: []float64{106}}, params: flagParams{SetupType: "breakRetest", MaxEntryDistanceATR: 0.5}}, meta: TradeMeta{"levelPrice": 100.0}, wantAllow: true},
		{name: "disabled zero", broker: newBroker(106, 10, 0, "breakRetest"), meta: TradeMeta{"levelPrice": 100.0}, wantAllow: true},
		{name: "failed breakout excluded", broker: newBroker(106, 10, 0.5, string(dsl.FamilyFailedBreakout)), meta: TradeMeta{"levelPrice": 100.0}, wantAllow: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.broker.typedEntryDistanceOK(0, tt.meta); got != tt.wantAllow {
				t.Fatalf("typedEntryDistanceOK = %v, want %v", got, tt.wantAllow)
			}
		})
	}
}

func TestFlagEntryAppliesTypedEntryDistance(t *testing.T) {
	newBroker := func(close float64) broker {
		return broker{
			series: marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: close, H: close + 1, L: close - 1, C: close}}),
			cols: contextcols.Columns{
				ATR:          []float64{10},
				Regime:       []int8{0},
				ER:           []float64{0},
				SessionPhase: []int8{0},
				PriorDayType: []int8{0},
			},
			params: flagParams{
				SetupType:           string(dsl.FamilyFlagContinuation),
				UseAsiaWindow:       true,
				AdmitAsiaWindow:     true,
				MaxMovementER:       1,
				MaxEntryDistanceATR: 0.5,
				RiskUSD:             1,
			},
			costs: Costs{StartEquity: 10000},
		}
	}

	setup := flagSetup{Stop: 90, Target: 120, Meta: TradeMeta{"levelPrice": 100.0}}
	b := newBroker(105)
	b.enter(0, sideLong, setup)
	if !b.hasPosition {
		t.Fatal("flag at exact entry-distance boundary did not enter")
	}

	b = newBroker(105.0001)
	b.enter(0, sideLong, setup)
	if b.hasPosition || len(b.pendingOrders) != 0 {
		t.Fatal("flag beyond entry-distance boundary entered")
	}
}

func TestBreakRetestFixtureHonorsTypedEntryDistanceAndPreservesCooldown(t *testing.T) {
	const caseName = "family-break-retest"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	baseline, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	if baseline.TradeCount != 14 || len(baseline.Trades) != 14 {
		t.Fatalf("baseline trades = %d/%d, want reviewed fixture count 14", baseline.TradeCount, len(baseline.Trades))
	}

	mutated := strings.Replace(string(source), "\nrisk {", "\nfilters {\n  entry distance max 0.5 ATR from level\n}\n\nrisk {", 1)
	if mutated == string(source) {
		t.Fatal("fixture source did not contain the expected risk block")
	}
	result, err := RunFixtureCase(fixture, mutated)
	if err != nil {
		t.Fatalf("run fixture with entry-distance directive: %v", err)
	}
	if result.TradeCount != 0 || len(result.Trades) != 0 {
		t.Fatalf("entry-distance directive produced %d trades: %+v; want none", result.TradeCount, result.Trades)
	}

	parsed, err := dsl.Parse(mutated)
	if err != nil {
		t.Fatalf("parse mutated DSL: %v", err)
	}
	if len(parsed.Errors) > 0 {
		t.Fatalf("parse mutated DSL diagnostics: %v", parsed.Errors)
	}
	runner := newPreparedRunner(fixture, parsed.Config)
	if trades := runner.RunPrepared(); len(trades) != 0 {
		t.Fatalf("prepared distance-rejected run produced %d trades, want 0", len(trades))
	}
	if !runner.broker.hasBREntry || runner.broker.brLastEntry <= 0 {
		t.Fatalf("final distance rejection did not preserve typed candidate consumption: cooldown=%v last=%d",
			runner.broker.hasBREntry, runner.broker.brLastEntry)
	}
}
