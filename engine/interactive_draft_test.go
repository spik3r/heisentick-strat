package engine

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const draftSMASource = `dsl v7
strategy "Mutable SMA" { description "A mutable source, with no catalog identity." }
market conditions { slices(XAUUSD 5m) }
setup {
 type: sma golden cross
 sma fast 2
 sma slow 4
}
filters { side long only }`

func draftFixture(t *testing.T) []byte {
	t.Helper()
	prices := []float64{10, 10, 10, 9, 8, 8, 9, 10, 11, 12, 11, 10, 9, 8}
	rows := make([][]float64, len(prices))
	for i, price := range prices {
		rows[i] = []float64{float64(i) * 300_000, price, price + 0.1, price - 0.1, price, 1}
	}
	raw, err := json.Marshal(map[string]any{"schema": "dsl-conformance-run-fixture-v1", "case": "mutable-draft", "strategyId": InteractiveDraftStrategyID,
		"symbol": "XAUUSD", "timeframe": "5m", "rangeMethod": "pivot", "costs": map[string]any{"startEquity": 10000, "fillOn": "close", "feePerUnit": 0.1, "slippage": 0.06, "slippageBps": 0}, "bars": rows})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInteractiveDraftMutableSourceAndAccounting(t *testing.T) {
	raw := draftFixture(t)
	profile, err := InspectInteractiveSource([]byte(`{"schema":"dsl-interactive-source-profile-v1","symbol":"XAUUSD","timeframe":"5m"}`), draftSMASource)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Profile.Family != "smaGoldenCross" || profile.Profile.MaxBars != 30000 || profile.Profile.RangeMethod != "pivot" {
		t.Fatalf("profile = %#v", profile)
	}
	result, err := RunInteractiveDraftFixture(raw, draftSMASource)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Run.Trades) == 0 || result.Schema != InteractiveDraftSchema {
		t.Fatalf("draft = %#v", result)
	}
	if result.Stats.Net != result.CashEndEquity-10000 || len(result.TradeNetPnL) != len(result.Run.Trades) {
		t.Fatal("draft discarded Go accounting")
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["strategyId"] = "unregisteredSource"
	baselineRaw, _ := json.Marshal(fields)
	baseline, err := RunInteractiveFixture(baselineRaw, draftSMASource)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Run.Trades, baseline.Run.Trades) || !reflect.DeepEqual(result.Stats, baseline.Stats) {
		t.Fatal("draft changed shared execution semantics")
	}
	mutated := strings.Replace(draftSMASource, "sma slow 4", "sma slow 6", 1)
	changed, err := RunInteractiveDraftFixture(raw, mutated)
	if err != nil {
		t.Fatal(err)
	}
	if result.Provenance.SourceSHA256 == changed.Provenance.SourceSHA256 || reflect.DeepEqual(result.Run.Trades, changed.Run.Trades) {
		t.Fatal("mutated source did not affect identity and execution")
	}
}

func TestInteractiveDraftRefusesUnqualifiedSource(t *testing.T) {
	for name, source := range map[string]string{
		"unknown setup":    strings.Replace(draftSMASource, "sma golden cross", "not a setup", 1),
		"side no-op":       strings.Replace(draftSMASource, "side long only", "side bananas", 1),
		"side ambiguous":   strings.Replace(draftSMASource, "side long only", "side long and short", 1),
		"trailing slices":  strings.Replace(draftSMASource, "XAUUSD 5m", "XAUUSD 5m ignored", 1),
		"empty slices":     strings.Replace(draftSMASource, "XAUUSD 5m", "", 1),
		"two types":        strings.Replace(draftSMASource, "type: sma golden cross", "type: opening range breakout\n type: sma golden cross", 1),
		"period suffix":    strings.Replace(draftSMASource, "sma fast 2", "sma fast 2R", 1),
		"version trailing": strings.Replace(draftSMASource, "dsl v7", "dsl v7 ignored", 1),
		"other family":     strings.Replace(draftSMASource, "sma golden cross", "failed breakout", 1),
		"micro":            strings.Replace(draftSMASource, "side long only", "side long only\n micro spread < 2", 1),
		"morphology":       strings.Replace(draftSMASource, "side long only", "side long only\n candle body fraction >= 0.5", 1),
		"htf":              strings.Replace(draftSMASource, "side long only", "side long only\n higher timeframe must not be against", 1),
		"off route":        strings.Replace(draftSMASource, "XAUUSD 5m", "XAUUSD 1h", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := RunInteractiveDraftFixture(draftFixture(t), source); err == nil {
				t.Fatal("unqualified source was admitted")
			}
		})
	}
	unknown := strings.Replace(draftSMASource, "sma golden cross", "unknown type", 1)
	legacy, _ := dsl.Parse(unknown)
	strict, _ := dsl.ParseStrict(unknown)
	if !strings.Contains(strings.Join(strict.Errors, " "), "unknown setup type") || strings.Contains(strings.Join(legacy.Errors, " "), "unknown setup type") {
		t.Fatal("strict mode changed historical Parse or omitted its diagnostic")
	}
}

func TestInteractiveDraftStrictInputAndReservedIdentity(t *testing.T) {
	raw := draftFixture(t)
	if _, err := RunInteractiveFixture(raw, draftSMASource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("ordinary admitted draft ID: %v", err)
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunFixtureCase(fixture, draftSMASource); err == nil {
		t.Fatal("conformance entrypoint admitted draft ID")
	}
	parsed, _ := dsl.Parse(draftSMASource)
	series := marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 1, H: 1, L: 1, C: 1, V: 1}})
	if _, err := PrepareRun(RunRequest{Config: parsed.Config, Series: series, StrategyID: InteractiveDraftStrategyID}); err == nil {
		t.Fatal("direct PrepareRun admitted draft ID")
	}
	for name, mutate := range map[string]func(map[string]any){
		"htf":                  func(f map[string]any) { f["htfBars"] = []any{} },
		"source":               func(f map[string]any) { f["sourceTimeframe"] = "1h" },
		"ha":                   func(f map[string]any) { f["calculationSource"] = "heikinAshi" },
		"params":               func(f map[string]any) { f["params"] = map[string]any{} },
		"window":               func(f map[string]any) { f["executionWindow"] = map[string]any{} },
		"identity":             func(f map[string]any) { f["strategyId"] = "dslEditorStrategy" },
		"empty symbol":         func(f map[string]any) { f["symbol"] = "" },
		"range":                func(f map[string]any) { f["rangeMethod"] = "zone" },
		"missing costs":        func(f map[string]any) { delete(f["costs"].(map[string]any), "slippageBps") },
		"timestamp grid":       func(f map[string]any) { f["bars"].([]any)[1].([]any)[0] = float64(300001) },
		"mislabeled timeframe": func(f map[string]any) { f["timeframe"] = "1h" },
		"duplicate time":       func(f map[string]any) { f["bars"].([]any)[1].([]any)[0] = float64(0) },
		"high":                 func(f map[string]any) { f["bars"].([]any)[0].([]any)[2] = float64(0) },
		"too many":             func(f map[string]any) { f["bars"] = make([][]float64, 30001) },
	} {
		t.Run(name, func(t *testing.T) {
			var fields map[string]any
			_ = json.Unmarshal(raw, &fields)
			mutate(fields)
			bad, _ := json.Marshal(fields)
			if _, err := RunInteractiveDraftFixture(bad, draftSMASource); err == nil {
				t.Fatal("invalid draft admitted")
			}
		})
	}
	for _, bad := range []string{
		`{"schema":"dsl-interactive-source-profile-v1","symbol":null,"timeframe":"5m"}`,
		`{"schema":"dsl-interactive-source-profile-v1","Symbol":"XAUUSD","timeframe":"5m"}`,
		`{"schema":"dsl-interactive-source-profile-v1","symbol":"XAUUSD","timeframe":"5m","timeframe":"1h"}`,
		`{"schema":"dsl-interactive-source-profile-v1","symbol":"XAUUSD","timeframe":"5m"} {}`,
	} {
		if _, err := InspectInteractiveSource([]byte(bad), draftSMASource); err == nil {
			t.Fatal("malformed profile admitted")
		}
	}
}
