package engine

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestCheckedResultRejectsNonFiniteNormalizedCosts(t *testing.T) {
	values := []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}
	fields := []struct {
		name string
		set  func(*Costs, float64)
	}{
		{name: "feePerUnit", set: func(costs *Costs, value float64) { costs.FeePerUnit = value }},
		{name: "slippage", set: func(costs *Costs, value float64) { costs.Slippage = value }},
		{name: "startEquity", set: func(costs *Costs, value float64) { costs.StartEquity = value }},
	}

	for _, field := range fields {
		for _, invalid := range values {
			t.Run(field.name+"/"+invalid.name, func(t *testing.T) {
				costs := Costs{StartEquity: 10_000}
				field.set(&costs, invalid.value)
				_, err := checkedResultEnvelope(RunFixture{Costs: costs}, nil)
				want := "run result costs." + field.name + " contains non-finite value"
				if err == nil || err.Error() != want {
					t.Fatalf("checkedResultEnvelope() error = %v, want %q", err, want)
				}
			})
		}
	}

	result, err := checkedResultEnvelope(RunFixture{}, nil)
	if err != nil {
		t.Fatalf("normalized defaults: %v", err)
	}
	if result.Costs != (Costs{FillOn: "close", StartEquity: 10_000}) {
		t.Fatalf("normalized costs = %+v, want close/10000 defaults", result.Costs)
	}
	if result.Trades == nil || len(result.Trades) != 0 {
		t.Fatalf("normalized empty trades = %#v, want nonnil empty slice", result.Trades)
	}
	if err := validateDerivedOutput(Costs{FeePerUnit: -1, Slippage: math.Copysign(0, -1), StartEquity: -1}, nil); err != nil {
		t.Fatalf("finite negative and signed-zero costs rejected: %v", err)
	}

	_, err = checkedResultEnvelope(RunFixture{Costs: Costs{
		FeePerUnit:  math.NaN(),
		Slippage:    math.Inf(1),
		StartEquity: math.Inf(-1),
	}}, nil)
	if err == nil || err.Error() != "run result costs.feePerUnit contains non-finite value" {
		t.Fatalf("cost precedence error = %v, want feePerUnit first", err)
	}
}

func TestCheckedResultRejectsEveryNonFiniteTradeFloat(t *testing.T) {
	values := []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}
	fields := []struct {
		name string
		set  func(*Trade, float64)
	}{
		{name: "entry", set: func(trade *Trade, value float64) { trade.Entry = value }},
		{name: "exit", set: func(trade *Trade, value float64) { trade.Exit = value }},
		{name: "entryT", set: func(trade *Trade, value float64) { trade.EntryT = value }},
		{name: "exitT", set: func(trade *Trade, value float64) { trade.ExitT = value }},
		{name: "initialSl", set: func(trade *Trade, value float64) { trade.InitialSL = value }},
		{name: "initialTp", set: func(trade *Trade, value float64) { trade.InitialTP = value }},
		{name: "pnl", set: func(trade *Trade, value float64) { trade.PnL = value }},
		{name: "points", set: func(trade *Trade, value float64) { trade.Points = value }},
		{name: "size", set: func(trade *Trade, value float64) { trade.Size = value }},
		{name: "sl", set: func(trade *Trade, value float64) { trade.SL = value }},
		{name: "tp", set: func(trade *Trade, value float64) { trade.TP = value }},
	}

	for _, field := range fields {
		for _, invalid := range values {
			t.Run(field.name+"/"+invalid.name, func(t *testing.T) {
				trade := finiteDerivedTrade()
				field.set(&trade, invalid.value)
				err := validateDerivedOutput(Costs{StartEquity: 10_000}, []Trade{trade})
				want := "run result trade 0 " + field.name + " contains non-finite value"
				if err == nil || err.Error() != want {
					t.Fatalf("validateDerivedOutput() error = %v, want %q", err, want)
				}
			})
		}
	}

	first := finiteDerivedTrade()
	first.TP = math.NaN()
	first.Entry = math.Inf(1)
	second := finiteDerivedTrade()
	second.Entry = math.Inf(-1)
	err := validateDerivedOutput(Costs{StartEquity: 10_000}, []Trade{first, second})
	if err == nil || err.Error() != "run result trade 0 entry contains non-finite value" {
		t.Fatalf("field/index precedence error = %v, want trade 0 entry first", err)
	}

	first = finiteDerivedTrade()
	second = finiteDerivedTrade()
	second.Entry = math.Inf(-1)
	err = validateDerivedOutput(Costs{StartEquity: 10_000}, []Trade{first, second})
	if err == nil || err.Error() != "run result trade 1 entry contains non-finite value" {
		t.Fatalf("trade-index error = %v, want trade 1 entry", err)
	}
}

func TestCheckedResultValidatesOnlyFlatExactFloat64Metadata(t *testing.T) {
	values := []struct {
		name  string
		value float64
	}{
		{name: "NaN", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "negative infinity", value: math.Inf(-1)},
	}
	for _, invalid := range values {
		t.Run(invalid.name, func(t *testing.T) {
			trade := finiteDerivedTrade()
			trade.Meta["metric"] = invalid.value
			err := validateDerivedOutput(Costs{StartEquity: 10_000}, []Trade{trade})
			want := `run result trade 0 meta "metric" contains non-finite float64 value`
			if err == nil || err.Error() != want {
				t.Fatalf("validateDerivedOutput() error = %v, want %q", err, want)
			}
		})
	}

	trade := finiteDerivedTrade()
	trade.Meta = TradeMeta{"zMetric": math.NaN(), "aMetric": math.Inf(1)}
	err := validateDerivedOutput(Costs{StartEquity: 10_000}, []Trade{trade})
	if err == nil || err.Error() != `run result trade 0 meta "aMetric" contains non-finite float64 value` {
		t.Fatalf("metadata key precedence error = %v, want aMetric first", err)
	}

	type otherFloat float64
	trade = finiteDerivedTrade()
	trade.Meta = TradeMeta{
		"float32": float32(math.Inf(1)),
		"named":   otherFloat(math.Inf(-1)),
		"nested":  map[string]any{"metric": math.NaN()},
		"slice":   []any{math.Inf(1)},
	}
	if err := validateDerivedOutput(Costs{StartEquity: 10_000}, []Trade{trade}); err != nil {
		t.Fatalf("excluded metadata shapes were recursively validated: %v", err)
	}

	trade.Entry = math.NaN()
	trade.Meta["aMetric"] = math.NaN()
	err = validateDerivedOutput(Costs{StartEquity: 10_000}, []Trade{trade})
	if err == nil || err.Error() != "run result trade 0 entry contains non-finite value" {
		t.Fatalf("trade/meta precedence error = %v, want entry first", err)
	}
}

func TestCheckedRunPathsRejectOverflowAndPreserveLegacyMasking(t *testing.T) {
	fixture, cfg, source := loadCheckedOutputCase(t)
	request := checkedOutputRequest(fixture, cfg)
	prepared, err := PrepareRun(request)
	if err != nil {
		t.Fatalf("PrepareRun: %v", err)
	}

	validLegacy := prepared.Run(fixture.Costs)
	validChecked, err := prepared.RunChecked(fixture.Costs)
	if err != nil {
		t.Fatalf("valid RunChecked: %v", err)
	}
	validDirect, err := Run(request)
	if err != nil {
		t.Fatalf("valid Run: %v", err)
	}
	validFixture, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatalf("valid RunFixtureCase: %v", err)
	}
	if validLegacy.TradeCount != 13 || len(validLegacy.Trades) != 13 {
		t.Fatalf("reviewed trades = %d/%d, want 13/13", validLegacy.TradeCount, len(validLegacy.Trades))
	}
	if !reflect.DeepEqual(validChecked, validLegacy) || !reflect.DeepEqual(validDirect, validLegacy) {
		t.Fatalf("valid legacy/checked/direct results differ\nlegacy: %s\nchecked: %s\ndirect: %s", canonicalJSON(validLegacy), canonicalJSON(validChecked), canonicalJSON(validDirect))
	}
	validFixture.Case = ""
	if !reflect.DeepEqual(validFixture, validLegacy) {
		t.Fatalf("valid fixture result differs after removing fixture-only case label\n got: %s\nwant: %s", canonicalJSON(validFixture), canonicalJSON(validLegacy))
	}

	overflowCosts := fixture.Costs
	overflowCosts.FeePerUnit = math.MaxFloat64
	legacyOverflow := prepared.Run(overflowCosts)
	if legacyOverflow.TradeCount != 13 || legacyOverflow.Trades[0].PnL != 0 {
		t.Fatalf("legacy overflow result = %d trades, first pnl %v; want 13 and masked zero", legacyOverflow.TradeCount, legacyOverflow.Trades[0].PnL)
	}
	wantOverflow := "run result trade 0 pnl contains non-finite value"
	if _, err := prepared.RunChecked(overflowCosts); err == nil || err.Error() != wantOverflow {
		t.Fatalf("RunChecked overflow error = %v, want %q", err, wantOverflow)
	}
	overflowRequest := request
	overflowRequest.Costs = overflowCosts
	if _, err := Run(overflowRequest); err == nil || err.Error() != wantOverflow {
		t.Fatalf("Run overflow error = %v, want %q", err, wantOverflow)
	}
	overflowFixture := fixture
	overflowFixture.Costs = overflowCosts
	if _, err := RunFixtureCase(overflowFixture, source); err == nil || err.Error() != wantOverflow {
		t.Fatalf("RunFixtureCase overflow error = %v, want %q", err, wantOverflow)
	}

	recovered, err := prepared.RunChecked(fixture.Costs)
	if err != nil {
		t.Fatalf("RunChecked recovery: %v", err)
	}
	if !reflect.DeepEqual(recovered, validLegacy) {
		t.Fatalf("checked recovery differs from valid baseline\n got: %s\nwant: %s", canonicalJSON(recovered), canonicalJSON(validLegacy))
	}
}

func TestPreparedRunResultEnvelopesUseAlternatingSuppliedCosts(t *testing.T) {
	fixture, cfg, _ := loadCheckedOutputCase(t)
	prepared, err := PrepareRun(checkedOutputRequest(fixture, cfg))
	if err != nil {
		t.Fatalf("PrepareRun: %v", err)
	}

	legacyCosts := Costs{FeePerUnit: 0.1, Slippage: 0.2, FillOn: "open", StartEquity: 12_000}
	legacy := prepared.Run(legacyCosts)
	if legacy.Costs != legacyCosts.normalized() {
		t.Fatalf("legacy costs = %+v, want %+v", legacy.Costs, legacyCosts.normalized())
	}

	checkedCosts := Costs{FeePerUnit: 0.3, Slippage: 0.4, StartEquity: 0}
	checked, err := prepared.RunChecked(checkedCosts)
	if err != nil {
		t.Fatalf("RunChecked: %v", err)
	}
	if checked.Costs != checkedCosts.normalized() {
		t.Fatalf("checked costs = %+v, want %+v", checked.Costs, checkedCosts.normalized())
	}

	legacyAgain := prepared.Run(legacyCosts)
	if legacyAgain.Costs != legacyCosts.normalized() {
		t.Fatalf("second legacy costs = %+v, want %+v", legacyAgain.Costs, legacyCosts.normalized())
	}
}

func TestCheckedOffRouteResultsNormalizeAndValidateCosts(t *testing.T) {
	fixture, cfg, source := loadCheckedOutputCase(t)
	offRouteRequest := checkedOutputRequest(fixture, cfg)
	offRouteRequest.Timeframe = "4h"
	prepared, err := PrepareRun(offRouteRequest)
	if err != nil {
		t.Fatalf("PrepareRun off route: %v", err)
	}
	result, err := prepared.RunChecked(Costs{})
	if err != nil {
		t.Fatalf("RunChecked valid off route: %v", err)
	}
	if result.Costs != (Costs{FillOn: "close", StartEquity: 10_000}) {
		t.Fatalf("off-route normalized costs = %+v, want close/10000", result.Costs)
	}
	if result.TradeCount != 0 || result.Trades == nil || len(result.Trades) != 0 {
		t.Fatalf("off-route trades = %d/%#v, want nonnil empty", result.TradeCount, result.Trades)
	}

	invalidCosts := Costs{FeePerUnit: math.NaN()}
	want := "run result costs.feePerUnit contains non-finite value"
	if _, err := prepared.RunChecked(invalidCosts); err == nil || err.Error() != want {
		t.Fatalf("RunChecked off-route cost error = %v, want %q", err, want)
	}

	offRouteFixture := fixture
	offRouteFixture.Timeframe = "4h"
	offRouteFixture.Costs = invalidCosts
	if _, err := RunFixtureCase(offRouteFixture, source); err == nil || err.Error() != want {
		t.Fatalf("RunFixtureCase off-route cost error = %v, want %q", err, want)
	}
}

func finiteDerivedTrade() Trade {
	return Trade{
		Entry: 1, EntryT: 2, Exit: 3, ExitT: 4,
		InitialSL: 5, InitialTP: 6, PnL: 7, Points: 8,
		Size: 9, SL: 10, TP: 11, Meta: TradeMeta{"metric": float64(12)},
	}
}

func loadCheckedOutputCase(t *testing.T) (RunFixture, dsl.Config, string) {
	t.Helper()
	const caseName = "family-break-retest"
	fixture, cfg := loadEntryAttemptCase(t, caseName)
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	return fixture, cfg, string(source)
}

func checkedOutputRequest(fixture RunFixture, cfg dsl.Config) RunRequest {
	return RunRequest{
		Config:          cfg,
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	}
}
