package engine

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

const interactiveSMASource = `dsl v7
strategy "Interactive SMA" {
  description "Small marked-equity fixture."
}
market conditions {
  slices(XAUUSD 4h)
}
setup {
  type: sma golden cross
  sma fast 2
  sma slow 3
}
filters {
  side long only
}`

func interactiveSMAFixture(t *testing.T, bars [][]float64) []byte {
	t.Helper()
	fixture := RunFixture{
		Schema: runFixtureSchema, Case: "interactive-sma", StrategyID: "sma",
		Symbol: "XAUUSD", Timeframe: "4h", RangeMethod: "zone",
		Costs:   Costs{FillOn: "close", StartEquity: 10000, FeePerUnit: 0.1},
		RawBars: bars,
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInteractiveResultUsesCausalMarksAndFeeInclusiveStats(t *testing.T) {
	bars := [][]float64{
		{0, 3, 3, 3, 3, 1}, {1, 2, 2, 2, 2, 1}, {2, 1, 1, 1, 1, 1},
		{3, 4, 4, 4, 4, 1}, {4, 5, 5, 3, 3, 1}, {5, 3, 3, 0, 0, 1},
		{6, 6, 6, 2, 2, 1},
	}
	raw := interactiveSMAFixture(t, bars)
	result, err := RunInteractiveFixture(raw, interactiveSMASource)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := RunFixtureCase(fixture, interactiveSMASource)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Run, legacy) {
		t.Fatalf("interactive trades differ from conformance: %+v / %+v", result.Run, legacy)
	}
	if result.Schema != InteractiveRunSchema || result.Run.TradeCount != 1 ||
		len(result.Provenance.FixtureSHA256) != 64 || len(result.Provenance.SourceSHA256) != 64 ||
		len(result.Skips) != 0 {
		t.Fatalf("interactive envelope = %+v", result)
	}
	for _, check := range []struct {
		name string
		got  float64
		want float64
	}{
		{"entry mark", result.EquityCurve[4], 9997.9},
		{"intratrade mark", result.EquityCurve[5], 9994.9},
		{"closed entry fee", result.ClosedEquity[5], 9999.9},
		{"rule exit mark", result.EquityCurve[6], 10000.8},
		{"cash end", result.CashEndEquity, 10000.8},
		{"fee-inclusive net", result.Stats.Net, 0.8},
		{"legacy trade net", result.Stats.TradeNet, 0.9},
		{"marked drawdown", result.Stats.MaxDD, 5.1},
		{"closed drawdown", result.Stats.MaxClosedDD, 0.1},
		{"marked percentage drawdown", result.Stats.MaxDDpct, 0.051},
	} {
		if math.IsNaN(check.got) || math.IsInf(check.got, 0) || math.Abs(check.got-check.want) > 1e-9 {
			t.Errorf("%s = %.12f, want %.12f", check.name, check.got, check.want)
		}
	}
	if result.Stats.ProfitFactor != nil || result.Stats.ProfitFactorState != "unbounded" {
		t.Fatalf("profit factor = %v/%s", result.Stats.ProfitFactor, result.Stats.ProfitFactorState)
	}
	prefix, err := RunInteractiveFixture(interactiveSMAFixture(t, bars[:6]), interactiveSMASource)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prefix.EquityCurve, result.EquityCurve[:6]) ||
		!reflect.DeepEqual(prefix.ClosedEquity, result.ClosedEquity[:6]) ||
		prefix.CashEndEquity >= prefix.EquityCurve[5] {
		t.Fatalf("prefix changed prior marks or lost final exit cost: %+v", prefix)
	}
}

func TestInteractiveRejectsIncompleteRoutesAndInputs(t *testing.T) {
	raw := interactiveSMAFixture(t, [][]float64{{0, 1, 1, 1, 1, 1}})
	for _, source := range []string{
		"broken DSL",
		`dsl v7
strategy "Down shock" {}
market conditions { slices(XAUUSD 4h) }
setup { type: down shock rebound }`,
	} {
		if _, err := RunInteractiveFixture(raw, source); err == nil {
			t.Fatalf("accepted source %q", source)
		}
	}
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.RawSourceBars = [][]float64{{0, 1, 1, 1, 1, 1}}
	withSource, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunInteractiveFixture(withSource, interactiveSMASource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("source route error = %v", err)
	}
	fixture.RawSourceBars = nil
	fixture.Costs.StartEquity = -1
	withBadCosts, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunInteractiveFixture(withBadCosts, interactiveSMASource); err == nil {
		t.Fatal("negative start equity accepted")
	}
	for _, costs := range []string{
		`{"startEquity":0}`, `{"startEquity":null}`, `{"fillOn":null}`,
		`{"fillOn":""}`, `{"feePerUnit":null}`, `{"commission":1}`,
	} {
		raw := []byte(`{"schema":"dsl-conformance-run-fixture-v1","costs":` + costs +
			`,"symbol":"XAUUSD","timeframe":"4h","bars":[[0,1,1,1,1,1]]}`)
		if _, err := RunInteractiveFixture(raw, interactiveSMASource); err == nil {
			t.Errorf("accepted invalid explicit costs %s", costs)
		}
	}
	for _, invalid := range []string{
		`{"schema":"dsl-conformance-run-fixture-v1","costs":{},"symbol":"XAUUSD","timeframe":"4h","bars":[[0,1,1,1,null,1]]}`,
		`{"schema":"dsl-conformance-run-fixture-v1","costs":{},"symbol":"XAUUSD","timeframe":"4h","bars":[[0,1,1,1,1,1]],"forceRoute":true}`,
		`{"schema":"dsl-conformance-run-fixture-v1","costs":{},"symbol":"XAUUSD","timeframe":"4h","bars":[[0,1,1,1,1,1]],"contextOptions":{"pivotK":1}}`,
	} {
		if _, err := RunInteractiveFixture([]byte(invalid), interactiveSMASource); err == nil {
			t.Errorf("accepted unrepresented interactive input %s", invalid)
		}
	}
}
