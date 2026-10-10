package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report"
	"github.com/spik3r/heisentick-strat/testsupport"
)

type sequentialBridgeCase struct{ name, strategy, source string }

var sequentialBridgeCases = []sequentialBridgeCase{
	{"family-legacy-setup9", "dslSequentialLegacySetup9", "conformance/parse/setup-legacy-setup9.strat"},
	{"family-legacy-setup9-fall", "dslSequentialLegacySetup9", "conformance/parse/setup-legacy-setup9.strat"},
	{"family-legacy-setup9-perf-seasonal-long", "dslSequentialLegacySetup9PerfSeasonal", "conformance/parse/setup-legacy-setup9-perf-seasonal.strat"},
	{"family-legacy-setup9-perf-seasonal-short", "dslSequentialLegacySetup9PerfSeasonal", "conformance/parse/setup-legacy-setup9-perf-seasonal.strat"},
	{"family-sequential-full-e1-long", "dslSequentialFullE1", "conformance/run/family-sequential-full-e1-long.strat"},
	{"family-sequential-full-e1-short", "dslSequentialFullE1", "conformance/run/family-sequential-full-e1-long.strat"},
	{"family-sequential-full-e2-long", "dslSequentialFullE2", "conformance/run/family-sequential-full-e2-long.strat"},
	{"family-sequential-full-e2-short", "dslSequentialFullE2", "conformance/run/family-sequential-full-e2-long.strat"},
}

func sequentialBridgeInput(t *testing.T, c sequentialBridgeCase) (string, string) {
	t.Helper()
	root := testsupport.MustRepoRoot()
	raw, err := os.ReadFile(filepath.Join(root, "conformance/run", c.name+".fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	// Qualification executes the actual frozen source and explicitly removes
	// unused old HTF/provenance metadata. Existing fixture/golden bytes stay put.
	f["strategyId"], f["higherTimeframe"] = c.strategy, nil
	for _, key := range []string{"htfBars", "source", "contextOptions"} {
		delete(f, key)
	}
	raw, err = json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, c.source))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), string(source)
}

func sequentialBridgeSuccess(t *testing.T, raw, source string) (sequentialEnvelope, []byte) {
	t.Helper()
	out, err := runSequentialFixture(raw, source)
	if err != nil {
		t.Fatal(err)
	}
	var result sequentialEnvelope
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != sequentialResultSchema || result.ContractVersion != 1 || result.Error != nil || result.Identity == nil || result.Run == nil || result.Metrics == nil || result.Capabilities == nil {
		t.Fatalf("incomplete success: %s", out)
	}
	if *result.Capabilities != (sequentialCapabilities{Headline: true, Trades: true, Equity: true}) {
		t.Fatalf("unsupported capability: %+v", result.Capabilities)
	}
	if result.Identity.FixtureSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) || result.Identity.DSLSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(source))) {
		t.Fatal("request identity lost")
	}
	if result.Metrics.Basis != report.SequentialMetricsBasis || len(result.Metrics.Equity) != result.Identity.BarCount || len(result.Metrics.TradeAccounting) != result.Run.TradeCount || result.Metrics.Headline.Trades != result.Run.TradeCount {
		t.Fatal("unaligned native projection")
	}
	if result.Metrics.Equity[len(result.Metrics.Equity)-1].Equity != result.Metrics.Headline.EndEquity {
		t.Fatal("final mark differs from end equity")
	}
	for i, mark := range result.Metrics.Equity {
		if mark.Index != i {
			t.Fatal("mark order lost")
		}
	}
	for i, trade := range result.Metrics.TradeAccounting {
		if trade.Index != i {
			t.Fatal("trade order lost")
		}
	}
	return result, out
}

func TestSequentialBridgeFrozenSourcesCaptureOffUnchanged(t *testing.T) {
	for _, c := range sequentialBridgeCases {
		t.Run(c.name, func(t *testing.T) {
			raw, source := sequentialBridgeInput(t, c)
			before, err := runFixture(raw, source)
			if err != nil {
				t.Fatal(err)
			}
			result, first := sequentialBridgeSuccess(t, raw, source)
			if result.Run.TradeCount == 0 {
				t.Fatal("expected a meaningful frozen-source execution")
			}
			retained, err := json.Marshal(result.Run)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, retained) {
				t.Fatalf("capture changed retained run\n%s\n%s", before, retained)
			}
			after, err := runFixture(raw, source)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("capture leaked into generic run: %v", err)
			}
			_, repeated := sequentialBridgeSuccess(t, raw, source)
			if !bytes.Equal(first, repeated) {
				t.Fatal("repeated run changed")
			}
			var fixture native.RunFixture
			if err := json.Unmarshal([]byte(raw), &fixture); err != nil {
				t.Fatal(err)
			}
			var columns [6][]float64
			for _, row := range fixture.RawBars {
				for j, value := range row {
					columns[j] = append(columns[j], value)
				}
			}
			meta, err := json.Marshal(columnRunMeta{Schema: "enginewasm-columnar-v1", Case: fixture.Case, StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs})
			if err != nil {
				t.Fatal(err)
			}
			packed, err := runColumns(string(meta), source, columns)
			if err != nil {
				t.Fatal(err)
			}
			wantValues, wantText, wantSummary, err := packColumnResult(*result.Run)
			if err != nil || !reflect.DeepEqual(packed.Values, wantValues) || packed.StringsJSON != wantText || packed.SummaryJSON != wantSummary {
				t.Fatalf("column path changed: %v", err)
			}
		})
	}
}

func TestSequentialBridgeNoTradesNormalizationAndRecovery(t *testing.T) {
	for _, c := range []sequentialBridgeCase{sequentialBridgeCases[0], sequentialBridgeCases[2], sequentialBridgeCases[4], sequentialBridgeCases[6]} {
		t.Run(c.strategy, func(t *testing.T) {
			raw, source := sequentialBridgeInput(t, c)
			var f map[string]any
			if err := json.Unmarshal([]byte(raw), &f); err != nil {
				t.Fatal(err)
			}
			f["bars"] = f["bars"].([]any)[:4]
			for _, omit := range []bool{false, true} {
				costs := f["costs"].(map[string]any)
				if omit {
					delete(costs, "startEquity")
				} else {
					costs["startEquity"] = 0
				}
				b, _ := json.Marshal(f)
				result, expected := sequentialBridgeSuccess(t, string(b), source)
				h := result.Metrics.Headline
				if h.Trades != 0 || h.Net != 0 || h.ReturnPct != 0 || h.MaxDD != 0 || h.MaxDDpct != 0 || h.StartEquity != 10000 || h.EndEquity != 10000 || h.WinRate != nil || h.ProfitFactor != nil || h.Expectancy != nil || h.AvgHoldBars != nil || h.WinRateReason != "no-trades" || h.ProfitFactorReason != "no-trades" || h.ExpectancyReason != "no-trades" || h.AvgHoldBarsReason != "no-trades" {
					t.Fatalf("incorrect no-trade projection: %+v", h)
				}
				for _, p := range result.Metrics.Equity {
					if p.Equity != 10000 {
						t.Fatal("no-trade curve moved")
					}
				}
				if _, err := runSequentialFixture(string(b), source+"\n"); err == nil {
					t.Fatal("mutated source accepted")
				}
				_, recovered := sequentialBridgeSuccess(t, string(b), source)
				if !bytes.Equal(expected, recovered) {
					t.Fatal("failure retained partial state")
				}
			}
		})
	}
}

func TestSequentialBridgeStrictInputAndNoPartialFailure(t *testing.T) {
	raw, source := sequentialBridgeInput(t, sequentialBridgeCases[4])
	mutate := func(fn func(map[string]any)) string {
		var f map[string]any
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			t.Fatal(err)
		}
		fn(f)
		b, _ := json.Marshal(f)
		return string(b)
	}
	cases := map[string]string{
		"duplicate":      strings.Replace(raw, `"symbol":"SYNTH"`, `"symbol":"SYNTH","symbol":"SYNTH"`, 1),
		"cost-duplicate": strings.Replace(raw, `"feePerUnit":0`, `"feePerUnit":0,"feePerUnit":0`, 1),
		"trailing":       raw + ` {}`, "null": `null`, "array": `[]`,
		"fractional-timestamp": strings.Replace(raw, `[300000,`, `[300000.00000000001,`, 1),
		"unknown":              mutate(func(f map[string]any) { f["capture"] = true }),
		"calendar":             mutate(func(f map[string]any) { f["timedCalendar"] = nil }),
		"context":              mutate(func(f map[string]any) { f["contextOptions"] = map[string]any{"atrLen": 14} }),
		"htf":                  mutate(func(f map[string]any) { f["higherTimeframe"] = "1d" }),
		"aux-bars":             mutate(func(f map[string]any) { f["sourceBars"] = []any{[]any{0, 100, 101, 99, 100, 1}} }),
		"null-cost":            mutate(func(f map[string]any) { f["costs"].(map[string]any)["slippage"] = nil }),
		"missing-cost":         mutate(func(f map[string]any) { delete(f["costs"].(map[string]any), "feePerUnit") }),
		"null-number":          mutate(func(f map[string]any) { f["bars"].([]any)[0].([]any)[1] = nil }),
		"five-columns":         mutate(func(f map[string]any) { f["bars"].([]any)[0] = []any{0, 100, 101, 99, 100} }),
		"seven-columns":        mutate(func(f map[string]any) { f["bars"].([]any)[0] = []any{0, 100, 101, 99, 100, 1, 2} }),
		"empty-bars":           mutate(func(f map[string]any) { f["bars"] = []any{} }),
		"missing-symbol":       mutate(func(f map[string]any) { delete(f, "symbol") }),
		"null-symbol":          mutate(func(f map[string]any) { f["symbol"] = nil }),
		"wrong-route":          mutate(func(f map[string]any) { f["symbol"] = "XAUUSD" }),
		"wrong-id":             mutate(func(f map[string]any) { f["strategyId"] = "dslSequentialFullE2" }),
		"bad-schema":           mutate(func(f map[string]any) { f["schema"] = "other" }),
		"negative-start":       mutate(func(f map[string]any) { f["costs"].(map[string]any)["startEquity"] = -1 }),
		"unknown-cost":         mutate(func(f map[string]any) { f["costs"].(map[string]any)["risk"] = 1 }),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := runSequentialFixture(input, source)
			if err == nil || out != nil {
				t.Fatalf("invalid fixture accepted: %s", out)
			}
			var failure map[string]any
			if err := json.Unmarshal(sequentialFailure(err), &failure); err != nil {
				t.Fatal(err)
			}
			if len(failure) != 3 || failure["schema"] != sequentialResultSchema || failure["contractVersion"] != float64(1) || failure["error"] == nil {
				t.Fatalf("partial failure leaked: %+v", failure)
			}
		})
	}
	for name, tc := range map[string]struct {
		raw, code string
		index     int
	}{
		"gap": {mutate(func(f map[string]any) {
			for _, row := range f["bars"].([]any)[1:] {
				r := row.([]any)
				r[0] = r[0].(float64) + 300000
			}
		}), "unsupported-sequential-time-gap", 1},
		"terminal": {mutate(func(f map[string]any) { f["bars"] = f["bars"].([]any)[:15] }), "unsupported-incomplete-terminal-run", 14},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runSequentialFixture(tc.raw, source)
			if err == nil {
				t.Fatal("refusal missing")
			}
			var out sequentialEnvelope
			_ = json.Unmarshal(sequentialFailure(err), &out)
			if out.Error.Code != tc.code || out.Error.BarIndex == nil || *out.Error.BarIndex != tc.index {
				t.Fatalf("typed identity lost: %+v", out.Error)
			}
		})
	}
}

func TestSequentialBridgeBounds(t *testing.T) {
	raw, source := sequentialBridgeInput(t, sequentialBridgeCases[4])
	if _, err := runSequentialFixture(strings.Repeat(" ", sequentialMaxFixture+1), source); err == nil {
		t.Fatal("fixture byte bound missing")
	}
	if _, err := runSequentialFixture(raw, strings.Repeat(" ", sequentialMaxSource+1)); err == nil {
		t.Fatal("source byte bound missing")
	}
	// Empty rows are enough to exceed the count cap without a huge price tape.
	large := strings.Replace(raw, `"bars":[`, `"bars":[`+strings.Repeat(`[],`, sequentialMaxBars+1), 1)
	if _, err := runSequentialFixture(large, source); err == nil {
		t.Fatal("bar count bound missing")
	}
}

func TestSequentialBridgeTypedErrors(t *testing.T) {
	for _, tc := range []struct {
		err         error
		code, field string
		index       *int
	}{
		{&dsl.SequentialFullConfigError{Code: "unsupported_config", Field: "policy"}, "unsupported_config", "policy", nil},
		{&native.SequentialFullExecutionError{Kind: "unsupported-sequential-time-gap", Field: "series.timestamp", BarIndex: 2, OpportunityID: "episode:perf"}, "unsupported-sequential-time-gap", "series.timestamp", func() *int { i := 2; return &i }()},
		{&native.SequentialAccountingError{Kind: "invalid-sequential-accounting", Field: "marks", BarIndex: 0}, "invalid-sequential-accounting", "marks", func() *int { i := 0; return &i }()},
		{&report.SequentialMetricsError{Kind: "invalid-sequential-metrics", Field: "net", Index: -1}, "invalid-sequential-metrics", "net", nil},
	} {
		var result sequentialEnvelope
		if err := json.Unmarshal(sequentialFailure(tc.err), &result); err != nil {
			t.Fatal(err)
		}
		if result.Error.Code != tc.code || result.Error.Field != tc.field || !reflect.DeepEqual(result.Error.BarIndex, tc.index) {
			t.Fatalf("typed error lost: %+v", result.Error)
		}
	}
}
