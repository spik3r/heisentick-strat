package engine

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestInteractiveDraftHeikinAshiChangesDecisionAndRetainsRawFills(t *testing.T) {
	// Same causality witness as the ordinary interactive HA contract: HA
	// crosses at decision 3; raw closes cross later. All fills use raw prices.
	bars := [][]float64{
		{0, 97.4337173887123, 98.46782213880319, 88.22426462960587, 91.06750421981548, 1},
		{14400000, 89.39901707172075, 91.88724639028254, 87.19833420751091, 88.87218184923867, 1},
		{28800000, 93.2288105408643, 96.93891087894082, 91.06814226564106, 95.93838905659194, 1},
		{43200000, 91.88464648478794, 92.67506510922438, 82.90019574732415, 85.46735762364025, 1},
		{57600000, 85.42731292212541, 86.22482144948039, 84.50580757129003, 84.51574751423136, 1},
		{72000000, 80.0080394367005, 86.90536849001896, 78.5360852839294, 84.87224646011977, 1},
		{86400000, 87.92616788662865, 91.88972964337306, 86.94150465202323, 91.35604667299349, 1},
		{100800000, 91.68421156457433, 93.63455911926134, 83.3611838521956, 84.63033062749777, 1},
	}
	fixture := RunFixture{Schema: runFixtureSchema, Case: "draft-ha", StrategyID: InteractiveDraftStrategyID,
		Symbol: "XAUUSD", Timeframe: "4h", RangeMethod: "pivot",
		Costs: Costs{FillOn: "nextOpen", Slippage: 0.2, FeePerUnit: 0.1, StartEquity: 10000}, RawBars: bars}
	run := func(f RunFixture, draft bool) InteractiveRunResult {
		t.Helper()
		request := map[string]any{"schema": f.Schema, "case": f.Case, "strategyId": f.StrategyID,
			"symbol": f.Symbol, "timeframe": f.Timeframe, "rangeMethod": f.RangeMethod, "bars": f.RawBars,
			"costs": map[string]any{"fillOn": f.Costs.FillOn, "startEquity": f.Costs.StartEquity,
				"feePerUnit": f.Costs.FeePerUnit, "slippage": f.Costs.Slippage, "slippageBps": f.Costs.SlippageBps}}
		if f.CalculationSource != "" {
			request["calculationSource"] = f.CalculationSource
		}
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		var result InteractiveRunResult
		if draft {
			result, err = RunInteractiveDraftFixture(encoded, interactiveSMASource)
		} else {
			result, err = RunInteractiveFixture(encoded, interactiveSMASource)
		}
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	standard := run(fixture, true)
	fixture.CalculationSource = "raw"
	explicitRaw := run(fixture, true)
	if standard.CalculationSource != "" || !reflect.DeepEqual(standard.Run, explicitRaw.Run) ||
		!reflect.DeepEqual(standard.EquityCurve, explicitRaw.EquityCurve) {
		t.Fatal("optional calculation source changed the raw default")
	}
	fixture.CalculationSource = "heikinAshi"
	ha := run(fixture, true)
	if standard.Run.Trades[0].EntryIndex != 7 || ha.Run.Trades[0].EntryIndex != 4 ||
		math.Abs(ha.Run.Trades[0].Entry-(bars[4][1]+0.2)) > 1e-9 ||
		ha.CalculationSource != "heikinAshi" || ha.CalculationSourceSchema != InteractiveCalculationSourceSchema {
		t.Fatalf("draft calculation/raw fill = %+v", ha)
	}
	wantMark := 10000 - 0.1 + (bars[4][4]-ha.Run.Trades[0].Entry)*ha.Run.Trades[0].Size
	if math.Abs(ha.EquityCurve[4]-wantMark) > 1e-9 {
		t.Fatal("draft used a synthetic HA equity mark")
	}
	fixture.StrategyID = "ordinary-sma"
	ordinary := run(fixture, false)
	if !reflect.DeepEqual(ordinary.Run.Trades, ha.Run.Trades) || !reflect.DeepEqual(ordinary.Stats, ha.Stats) ||
		!reflect.DeepEqual(ordinary.EquityCurve, ha.EquityCurve) {
		t.Fatal("draft changed existing Go HA execution semantics")
	}
	fixture.StrategyID, fixture.RawBars = InteractiveDraftStrategyID, bars[:6]
	prefix := run(fixture, true)
	if prefix.Run.Trades[0].EntryIndex != 4 || !reflect.DeepEqual(prefix.EquityCurve, ha.EquityCurve[:6]) ||
		!reflect.DeepEqual(prefix.ClosedEquity, ha.ClosedEquity[:6]) {
		t.Fatal("future append changed prior draft HA decisions/marks")
	}
}

func TestInteractiveDraftCalculationSourceProfileAndStrictRefusals(t *testing.T) {
	base := `{"schema":"dsl-interactive-source-profile-v1","symbol":"XAUUSD","timeframe":"5m"`
	for _, source := range []string{"raw", "heikinAshi"} {
		profile, err := InspectInteractiveSource([]byte(base+`,"calculationSource":"`+source+`"}`), draftSMASource)
		if err != nil || profile.Profile.CalculationSource != source {
			t.Fatalf("profile %s = %+v, %v", source, profile, err)
		}
	}
	for _, value := range []string{`null`, `""`, `"renko"`, `0`, `true`, `[]`, `{}`} {
		if _, err := InspectInteractiveSource([]byte(base+`,"calculationSource":`+value+`}`), draftSMASource); err == nil {
			t.Fatalf("profile accepted %s", value)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(draftFixture(t), &fields); err != nil {
			t.Fatal(err)
		}
		fields["calculationSource"] = json.RawMessage(value)
		encoded, _ := json.Marshal(fields)
		if _, err := RunInteractiveDraftFixture(encoded, draftSMASource); err == nil {
			t.Fatalf("run accepted %s", value)
		}
	}
	for _, invalid := range []string{base + `,"CalculationSource":"raw"}`, base + `,"calculationSource":"raw","calculationSource":"heikinAshi"}`} {
		if _, err := InspectInteractiveSource([]byte(invalid), draftSMASource); err == nil {
			t.Fatal("profile accepted calculation alias/duplicate")
		}
	}
	runBase := strings.TrimSuffix(string(draftFixture(t)), "}")
	for _, suffix := range []string{`,"CalculationSource":"raw"}`, `,"calculationSource":"raw","calculationSource":"heikinAshi"}`, `,"calculationSource":"heikinAshi","htfBars":[]}`} {
		if _, err := RunInteractiveDraftFixture([]byte(runBase+suffix), draftSMASource); err == nil {
			t.Fatal("run accepted calculation alias/duplicate or auxiliary HA input")
		}
	}
}

func TestInteractiveDraftHeikinAshiDualEMAUsesSharedExecution(t *testing.T) {
	source := `dsl v7
strategy "Mutable Dual HA"
market conditions { slices(XAUUSD 5m) }
setup {
 type: dual ema resumption
 ema fast 3
 ema slow 8
 ema slow rise 1
 ema wilder atr 3
 stop initial 2.5 ATR
 trail close 3 ATR
 fallback below slow ema
}
filters { side long only }
execution { risk: 200 USD }`
	rows := make([][]float64, 1600)
	for i := range rows {
		close := 1900 + float64(i)*0.2
		if i%12 == 9 {
			close -= 2.4
		}
		rows[i] = []float64{float64(i) * 300000, close - 0.1, close + 0.3, close - 0.3, close, 1}
	}
	var fields map[string]any
	if err := json.Unmarshal(draftFixture(t), &fields); err != nil {
		t.Fatal(err)
	}
	fields["bars"], fields["calculationSource"] = rows, "heikinAshi"
	raw, _ := json.Marshal(fields)
	draft, err := RunInteractiveDraftFixture(raw, source)
	if err != nil || len(draft.Run.Trades) == 0 {
		t.Fatalf("Dual HA = %+v, %v", draft, err)
	}
	fields["strategyId"] = "ordinary-dual"
	raw, _ = json.Marshal(fields)
	ordinary, err := RunInteractiveFixture(raw, source)
	if err != nil || !reflect.DeepEqual(draft.Run.Trades, ordinary.Run.Trades) ||
		!reflect.DeepEqual(draft.Stats, ordinary.Stats) || !reflect.DeepEqual(draft.EquityCurve, ordinary.EquityCurve) {
		t.Fatalf("Dual HA changed existing execution: %v", err)
	}
}
