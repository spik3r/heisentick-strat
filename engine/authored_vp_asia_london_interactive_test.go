package engine

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func readAuthoredVPInteractiveInput(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/asia-london-wide-interactive.fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAuthoredVPAsiaLondonWideInteractiveMatchesFeeAndEquityOracle(t *testing.T) {
	fixture := readAuthoredVPWholeFixture(t)
	raw := readAuthoredVPInteractiveInput(t)
	result, err := RunAuthoredVPAsiaLondonWideInteractive(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != AuthoredVPAsiaLondonWideInteractiveSchema || result.StrategyVersion != AuthoredVPAsiaLondonWideVersion || result.Run.StrategyID != authoredVPAsiaLondonWideID || result.Run.TradeCount != 3 || result.Provenance.StrategySHA256 != fixture.Provenance.StrategySHA256 || result.Provenance.InheritedBaseSHA256 != fixture.Provenance.InheritedBaseSHA256 || len(result.Provenance.FixtureSHA256) != 64 {
		t.Fatalf("interactive identity/provenance = %+v", result)
	}
	for _, curve := range []struct {
		name      string
		got, want []float64
	}{
		{"marked", result.EquityCurve, fixture.ExpectedWithFees.EquityCurve},
		{"closed", result.ClosedEquity, fixture.ExpectedWithFees.ClosedEquityCurve},
	} {
		if len(curve.got) != len(curve.want) || len(curve.got) != len(fixture.Bars) {
			t.Fatalf("%s curve length = %d, want %d", curve.name, len(curve.got), len(curve.want))
		}
		for i := range curve.got {
			if math.Abs(curve.got[i]-curve.want[i]) > 1e-8 {
				t.Fatalf("%s equity[%d] = %.12g, JS want %.12g", curve.name, i, curve.got[i], curve.want[i])
			}
		}
	}
	if math.Abs(result.CashEndEquity-fixture.ExpectedWithFees.CashEndEquity) > 1e-8 || math.Abs(result.Stats.EndEquity-result.CashEndEquity) > 1e-8 || len(result.TradeNetPnL) != 3 {
		t.Fatalf("fee-inclusive cash/stats = %.12g / %+v", result.CashEndEquity, result.Stats)
	}
	if !(result.TradeNetPnL[0] < result.Run.Trades[0].PnL) || !(result.Stats.MaxDD > 0) {
		t.Fatalf("entry fee or drawdown missing: %+v", result)
	}
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	// The strategy has an open long at this prefix. The marked and closed
	// equity at the last bar precede final-bar liquidation in both runs.
	input["bars"] = input["bars"].([]any)[:124]
	prefixRaw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := RunAuthoredVPAsiaLondonWideInteractive(prefixRaw)
	if err != nil {
		t.Fatal(err)
	}
	for i := range prefix.EquityCurve {
		if math.Abs(prefix.EquityCurve[i]-result.EquityCurve[i]) > 1e-8 || math.Abs(prefix.ClosedEquity[i]-result.ClosedEquity[i]) > 1e-8 {
			t.Fatalf("future suffix changed equity mark at %d", i)
		}
	}
}

func TestAuthoredVPAsiaLondonWideInteractiveRejectsUnsupportedAndAmbiguousInput(t *testing.T) {
	base := readAuthoredVPInteractiveInput(t)
	var original map[string]any
	if err := json.Unmarshal(base, &original); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"zero start equity", func(m map[string]any) { m["costs"].(map[string]any)["startEquity"] = 0 }, "startEquity"},
		{"missing fee", func(m map[string]any) { delete(m["costs"].(map[string]any), "feePerUnit") }, "costs.feePerUnit"},
		{"next open", func(m map[string]any) { m["costs"].(map[string]any)["fillOn"] = "nextOpen" }, "requires close fills"},
		{"wrong symbol", func(m map[string]any) { m["symbol"] = "XAGUSD" }, "active authored VP identity"},
		{"wrong ID", func(m map[string]any) { m["strategyId"] = "vpAsiaLondonSweepContinuation" }, "active authored VP identity"},
		{"source bars", func(m map[string]any) { m["sourceBars"] = []any{} }, "authored VP field sourceBars"},
		{"unknown field", func(m map[string]any) { m["foo"] = 1 }, "fixture field foo"},
		{"no trade", func(m map[string]any) { m["bars"] = m["bars"].([]any)[:4] }, "produced no trades"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input map[string]any
			if err := json.Unmarshal(base, &input); err != nil {
				t.Fatal(err)
			}
			tc.mutate(input)
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			result, err := RunAuthoredVPAsiaLondonWideInteractive(raw)
			if err == nil || !strings.Contains(err.Error(), tc.want) || result.Schema != "" {
				t.Fatalf("result = %+v, error = %v, want %q", result, err, tc.want)
			}
		})
	}
}
