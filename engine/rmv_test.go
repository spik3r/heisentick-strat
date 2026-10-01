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

func TestRMVGateBoundariesAndUnavailable(t *testing.T) {
	for _, tc := range []struct {
		op    string
		value float64
		want  bool
	}{
		{"above", 50, false}, {"below", 50, false}, {"atLeast", 50, true}, {"atMost", 50, true},
		{"above", 49, true}, {"below", 51, true}, {"invalid", 50, false},
	} {
		b := broker{cols: contextcols.Columns{RMV: []float64{math.NaN(), 50}}, params: flagParams{RMV: rmvFilter{Op: tc.op, Value: tc.value}}}
		if b.rmvGateOK(0) || b.rmvGateOK(-1) || b.rmvGateOK(2) {
			t.Fatal("unavailable RMV passed")
		}
		if got := b.rmvGateOK(1); got != tc.want {
			t.Fatalf("%s %v: %v", tc.op, tc.value, got)
		}
	}
	b := broker{}
	if !b.rmvGateOK(0) {
		t.Fatal("absent comparison should not gate")
	}
}

func TestRMVRunFixtureGatesAndParameterOnly(t *testing.T) {
	for _, name := range []string{"family-break-retest", "family-named-level-sweep", "deployed-dsl-flag-continuation-one-four-hour-review", "family-sma-golden-cross", "family-daily-flush-failure", "deployed-dsl-weekend-extreme-fade-btcusdt-four-hour", "family-intra-hour-run-exhaustion"} {
		t.Run(name, func(t *testing.T) {
			fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), name+".fixture.json"))
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(filepath.Join(runFixtureDir(), name+".strat"))
			if err != nil {
				t.Fatal(err)
			}
			sourceText := enableRMVV7(string(source))
			baseline, err := RunFixtureCase(fixture, sourceText)
			if err != nil {
				t.Fatal(err)
			}
			if baseline.TradeCount == 0 {
				t.Fatal("fixture has no baseline trades")
			}
			for _, directive := range []string{"rmv below 0", "rmv above 100", "rmv atr period 1\nrmv lookback 1\nrmv at least 0"} {
				result, err := RunFixtureCase(fixture, sourceText+"\nfilters {\n"+directive+"\n}\n")
				if err != nil {
					t.Fatal(err)
				}
				if result.TradeCount != 0 {
					t.Fatalf("%s admitted %d trades", directive, result.TradeCount)
				}
			}
			result, err := RunFixtureCase(fixture, sourceText+"\nfilters { rmv lookback 20 }\n")
			if err != nil {
				t.Fatal(err)
			}
			if result.TradeCount != baseline.TradeCount {
				t.Fatal("parameter-only RMV changed admission")
			}
		})
	}
}

func enableRMVV7(source string) string {
	return strings.Replace(source, "dsl v6", "dsl v7", 1)
}

func TestRMVSharedContextSeparatesParameters(t *testing.T) {
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "family-break-retest.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "family-break-retest.strat"))
	if err != nil {
		t.Fatal(err)
	}
	parse := func(lookback string) dsl.Config {
		p, e := dsl.Parse(enableRMVV7(string(source)) + "\nfilters { rmv lookback " + lookback + " }\n")
		if e != nil || len(p.Errors) > 0 {
			t.Fatal(e, p.Errors)
		}
		return p.Config
	}
	one, two := parse("20"), parse("40")
	request := RunRequest{Config: one, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
	key1, err := SharedContextKey(request)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := PrepareSharedRunContext(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Config = two
	key2, err := SharedContextKey(request)
	if err != nil {
		t.Fatal(err)
	}
	if key1 == key2 {
		t.Fatal("different RMV parameters share a key")
	}
	if _, err := shared.PrepareVariant(two); err == nil {
		t.Fatal("different RMV parameters reused context")
	}
	if _, err := shared.PrepareVariant(one); err != nil {
		t.Fatal(err)
	}
}

func TestRMVConfigRejectsMalformedAndUnsupported(t *testing.T) {
	for _, raw := range []any{"bad", map[string]any{"atrPeriod": "bad"}, map[string]any{"lookback": 0}, map[string]any{"value": 50}, map[string]any{"op": "unknown", "value": 50}, map[string]any{"op": map[string]any{}, "value": 50}, map[string]any{"op": "below", "value": math.NaN()}} {
		if validateRMVConfig(dsl.Config{"relativeMeasuredVolatility": raw}) == nil {
			t.Fatalf("accepted %#v", raw)
		}
	}
	if err := validateRMVConfig(dsl.Config{"setupType": string(dsl.FamilyDownShockRebound), "relativeMeasuredVolatility": map[string]any{"lookback": 100}}); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatal("unsupported direct runner was accepted", err)
	}
	for _, entryTf := range []string{"", "current"} {
		cfg := dsl.Config{"sourceTimeframe": "4h", "entryTf": entryTf, "relativeMeasuredVolatility": map[string]any{"op": "above", "value": 50}}
		if err := validateRMVConfig(cfg); err == nil || err.Error() != "RMV with a source timeframe requires a supported source-entry route" {
			t.Fatalf("source-only RMV config error = %v", err)
		}
	}
	for _, tc := range []struct {
		symbol, timeframe string
	}{
		{"EURUSD", "15m"},
		{"XAUUSD", "1h"},
	} {
		cfg := dsl.Config{"sourceTimeframe": "4h", "entryTf": "15m", "relativeMeasuredVolatility": map[string]any{"lookback": 100}}
		if err := validateRMVSourceEntryRoute(tc.symbol, tc.timeframe, cfg); err == nil || err.Error() != "RMV with a source timeframe requires a supported source-entry route" {
			t.Fatalf("route %s %s error = %v", tc.symbol, tc.timeframe, err)
		}
	}
}
