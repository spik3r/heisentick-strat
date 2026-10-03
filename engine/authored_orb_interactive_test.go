package engine

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestAuthoredOrbInteractiveAccounting(t *testing.T) {
	raw, err := os.ReadFile("../authoredorb/testdata/parity-held-through-session-close.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	delete(fixture.Request, "contextVwap")
	fixture.Request["costs"].(map[string]any)["feePerUnit"] = 0.1
	fixture.Request["costs"].(map[string]any)["slippageBps"] = 0.2
	input, err := json.Marshal(fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RunAuthoredOrbInteractive(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Schema != AuthoredOrbInteractiveSchema || got.StrategyVersion != AuthoredOrbInteractiveVersion || len(got.Run.Trades) != 1 || len(got.EquityCurve) != 13 || len(got.ClosedEquity) != 13 {
		t.Fatalf("incomplete authored ORB result: %+v", got)
	}
	trade := got.Run.Trades[0]
	wantNet := trade.PnL - 0.1*trade.Size
	if math.Abs(got.TradeNetPnL[0]-wantNet) > 1e-9 || math.Abs(got.CashEndEquity-(10000+wantNet)) > 1e-9 || got.Stats.EndEquity != got.CashEndEquity || got.Stats.Trades != 1 {
		t.Fatalf("fee-inclusive accounting mismatch: cash=%v net=%v trade=%+v stats=%+v", got.CashEndEquity, got.TradeNetPnL, trade, got.Stats)
	}
	if got.ClosedEquity[trade.EntryIndex] >= 10000 || got.EquityCurve[trade.ExitIndex] == got.CashEndEquity {
		t.Fatalf("entry debit and final-bar liquidation must remain distinct: marked=%v closed=%v", got.EquityCurve, got.ClosedEquity)
	}
	fixture.Request["closeAtEnd"] = false
	bad, _ := json.Marshal(fixture.Request)
	if _, err := RunAuthoredOrbInteractive(bad); err == nil {
		t.Fatal("interactive route accepted an open-ended run")
	}
	delete(fixture.Request, "closeAtEnd")
	for _, key := range []string{"feePerUnit", "slippage", "slippageBps", "fillOn"} {
		costs := fixture.Request["costs"].(map[string]any)
		original := costs[key]
		costs[key] = nil
		bad, _ := json.Marshal(fixture.Request)
		if _, err := RunAuthoredOrbInteractive(bad); err == nil {
			t.Errorf("accepted null costs.%s", key)
		}
		costs[key] = original
	}
	originalBars := fixture.Request["bars"]
	for _, bars := range []any{[]any{map[string]any{}}, []any{nil}} {
		fixture.Request["bars"] = bars
		bad, _ := json.Marshal(fixture.Request)
		if _, err := RunAuthoredOrbInteractive(bad); err == nil {
			t.Errorf("accepted incomplete bars: %s", bad)
		}
	}
	fixture.Request["bars"] = originalBars
	costs := fixture.Request["costs"].(map[string]any)
	costs["FillOn"] = ""
	bad, _ = json.Marshal(fixture.Request)
	if _, err := RunAuthoredOrbInteractive(bad); err == nil {
		t.Fatal("accepted case-aliased costs.FillOn")
	}
	delete(costs, "FillOn")
	bar := originalBars.([]any)[0].(map[string]any)
	bar["C"] = bar["c"]
	bad, _ = json.Marshal(fixture.Request)
	if _, err := RunAuthoredOrbInteractive(bad); err == nil {
		t.Fatal("accepted case-aliased bar close")
	}
}
