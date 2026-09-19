package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestHelpDocumentsOptInTradeExport(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--help"}, &out); err != nil {
		t.Fatalf("run help: %v", err)
	}
	if !strings.Contains(out.String(), "--include-trades=1") {
		t.Fatalf("help does not document trade export: %s", out.String())
	}
}

func TestReportJSONOnlyUsesLoadedBTB1(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	var out bytes.Buffer
	err := run([]string{
		"report",
		"--dsl-file=" + dslFile,
		"--dsl-id=cli-range-fake",
		"--symbol=XAUUSD",
		"--tf=5m",
		"--range=zone",
		"--slippage=0.05",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out)
	if err != nil {
		t.Fatalf("run report: %v", err)
	}
	var payload report.Document
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode report JSON: %v\n%s", err, out.String())
	}
	if payload.Strategy != "cli-range-fake" {
		t.Fatalf("strategy = %q, want cli-range-fake", payload.Strategy)
	}
	if payload.Bars != 80 {
		t.Fatalf("bars = %d, want 80", payload.Bars)
	}
	if !reflect.DeepEqual(payload.ActiveSessions, []string{"london", "ny"}) {
		t.Fatalf("activeSessions = %#v", payload.ActiveSessions)
	}
	if len(payload.Costs) != 1 || payload.Costs[0].Label != "slip 0.05" || payload.Costs[0].Slippage != 0.05 {
		t.Fatalf("costs = %+v, want one slip 0.05 row", payload.Costs)
	}
	if len(payload.Slices) != 1 || len(payload.Slices[0].Costs) != 1 {
		t.Fatalf("slices = %+v, want one slice with one cost row", payload.Slices)
	}
	if payload.Slices[0].HTF != nil {
		t.Fatalf("slice HTF = %q, want nil for DSL without HTF gate", *payload.Slices[0].HTF)
	}
	if payload.TradeSchema != "" || payload.Slices[0].Trades != nil {
		t.Fatalf("default report unexpectedly included trade payload: schema=%q trades=%v", payload.TradeSchema, payload.Slices[0].Trades)
	}
	if len(payload.Warnings) != 0 {
		t.Fatalf("default report warnings = %#v, want []", payload.Warnings)
	}
	var raw map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw report JSON: %v", err)
	}
	if _, exists := raw["tradeSchema"]; exists {
		t.Fatal("default report JSON must omit tradeSchema")
	}
	rawSlice := raw["slices"].([]any)[0].(map[string]any)
	if _, exists := rawSlice["trades"]; exists {
		t.Fatal("default report JSON must omit slices[].trades")
	}
	envelope, ok := raw["evidenceEnvelope"].(map[string]any)
	if !ok {
		t.Fatal("default report JSON must include the single-route evidence envelope core")
	}
	if raw["generatedAt"] != envelope["generatedAt"] {
		t.Fatalf("generatedAt = %#v, envelope = %#v", raw["generatedAt"], envelope["generatedAt"])
	}
	if envelope["schema"] != "xauusd-backtester/evidence-envelope" || envelope["artifact"] != "strategy-report" || envelope["source"] != "cli" {
		t.Fatalf("evidence envelope identity = %#v", envelope)
	}
	diagnostics, ok := envelope["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("evidence envelope diagnostics = %#v", envelope["diagnostics"])
	}
	if warnings, ok := diagnostics["warnings"].([]any); !ok || len(warnings) != 0 {
		t.Fatalf("evidence envelope warnings = %#v, want []", diagnostics["warnings"])
	}
	strategy, ok := envelope["strategy"].(map[string]any)
	if !ok || strategy["id"] != "cli-range-fake" || strategy["name"] != "CLI Range Fake" {
		t.Fatalf("evidence envelope strategy = %#v", envelope["strategy"])
	}
	route, ok := envelope["route"].(map[string]any)
	if !ok || route["symbol"] != "XAUUSD" || route["tf"] != "5m" || route["rangeMethod"] != "zone" {
		t.Fatalf("evidence envelope route = %#v", envelope["route"])
	}
	summary, ok := envelope["summary"].(map[string]any)
	if !ok || summary["trades"] != float64(payload.Costs[payload.PrimaryCost.Index].Trades) {
		t.Fatalf("evidence envelope summary = %#v", envelope["summary"])
	}
	aggregate, ok := envelope["aggregate"].(map[string]any)
	if !ok || aggregate["kind"] != "strategy-report" || aggregate["routeSliceCount"] != float64(1) || aggregate["bars"] != float64(payload.Bars) {
		t.Fatalf("evidence envelope aggregate = %#v", envelope["aggregate"])
	}
	rows, ok := envelope["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("evidence envelope rows = %#v", envelope["rows"])
	}
	row, ok := rows[0].(map[string]any)
	if !ok || row["symbol"] != "XAUUSD" || row["tf"] != "5m" || row["bars"] != float64(payload.Bars) || row["status"] != "ok" {
		t.Fatalf("evidence envelope row = %#v", rows[0])
	}
}

func TestReportRouteWarningMatchesEvidenceEnvelope(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	sourcePath := filepath.Join(dataRoot, "XAUUSD", "5m.bin")
	series, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read CLI market series: %v", err)
	}
	excludedDir := filepath.Join(dataRoot, "EURUSD")
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		t.Fatalf("mkdir excluded route: %v", err)
	}
	if err := os.WriteFile(filepath.Join(excludedDir, "5m.bin"), series, 0o644); err != nil {
		t.Fatalf("write excluded-route market series: %v", err)
	}

	var out bytes.Buffer
	if err := run([]string{
		"report",
		"--dsl-file=" + dslFile,
		"--dsl-id=cli-excluded-route",
		"--symbol=EURUSD",
		"--tf=5m",
		"--range=zone",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out); err != nil {
		t.Fatalf("run excluded-route report: %v", err)
	}

	var payload report.Document
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode excluded-route report JSON: %v\n%s", err, out.String())
	}
	if len(payload.Warnings) != 1 || !strings.Contains(payload.Warnings[0], "route EURUSD 5m is excluded") {
		t.Fatalf("report warnings = %#v, want one excluded-route warning", payload.Warnings)
	}
	if payload.EvidenceEnvelope == nil {
		t.Fatal("excluded-route report omitted evidence envelope")
	}
	if !reflect.DeepEqual(payload.EvidenceEnvelope.Diagnostics.Warnings, payload.Warnings) {
		t.Fatalf("evidence warnings = %#v, want top-level %#v", payload.EvidenceEnvelope.Diagnostics.Warnings, payload.Warnings)
	}
}

func TestReportIncludeTradesSerializesPrimaryCostExecutions(t *testing.T) {
	dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "family-range-break-fake")
	var out bytes.Buffer
	err := run([]string{
		"report",
		"--dsl-file=" + dslFile,
		"--symbol=" + expected.Symbol,
		"--tf=" + expected.Timeframe,
		"--range=" + expected.RangeMethod,
		"--include-trades=1",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out)
	if err != nil {
		t.Fatalf("run report with trades: %v", err)
	}
	var payload report.Document
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode report JSON: %v\n%s", err, out.String())
	}
	if payload.TradeSchema != report.TradesSchema {
		t.Fatalf("trade schema = %q, want %q", payload.TradeSchema, report.TradesSchema)
	}
	if len(payload.Slices) != 1 || payload.Slices[0].Trades == nil {
		t.Fatalf("slices = %+v, want one slice with a trades array", payload.Slices)
	}
	primary := payload.PrimaryCost.Index
	if got, want := len(*payload.Slices[0].Trades), payload.Costs[primary].Trades; got != want {
		t.Fatalf("serialized trades = %d, want primary cost count %d", got, want)
	}
	for i, got := range *payload.Slices[0].Trades {
		if !reflect.DeepEqual(got.Trade, expected.Trades[i]) {
			t.Fatalf("serialized primary-cost trade %d differs from conformance golden\ngot:  %+v\nwant: %+v", i, got.Trade, expected.Trades[i])
		}
		if want := "other"; got.ReportSessionPhase != want && got.ReportSessionPhase != "overnight" && got.ReportSessionPhase != "open" && got.ReportSessionPhase != "lunch" && got.ReportSessionPhase != "middle" && got.ReportSessionPhase != "close" {
			t.Fatalf("trade %d reportSessionPhase = %q, want a supported phase", i, got.ReportSessionPhase)
		}
		if got.ReportOpenLocation == "" || got.ReportPriorDayType == "" {
			t.Fatalf("trade %d missing report context labels: %#v", i, got)
		}
		if got.ReportPreviousOutcome != "first trade" && got.ReportPreviousOutcome != "after win" && got.ReportPreviousOutcome != "after loss" {
			t.Fatalf("trade %d reportPreviousOutcome = %q, want a supported outcome", i, got.ReportPreviousOutcome)
		}
		if got.ReportSymbol != payload.Slices[0].Symbol || got.ReportTF != payload.Slices[0].TF || got.ReportRange != payload.Slices[0].Range || got.ReportSlice != payload.Slices[0].Symbol+" "+payload.Slices[0].TF {
			t.Fatalf("trade %d route identity = %#v, want %s %s %s", i, got, payload.Slices[0].Symbol, payload.Slices[0].TF, payload.Slices[0].Range)
		}
	}
	var raw map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw report JSON: %v", err)
	}
	rawSlice := raw["slices"].([]any)[0].(map[string]any)
	if _, exists := rawSlice["trades"]; !exists {
		t.Fatal("--include-trades must emit slices[].trades even when empty")
	}
}

func TestReportLoadsResolvedHigherTimeframe(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	raw, err := os.ReadFile(dslFile)
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	withHTF := strings.Replace(string(raw), "filters {\n", "filters {\n  higher timeframe must agree\n", 1)
	if err := os.WriteFile(dslFile, []byte(withHTF), 0o644); err != nil {
		t.Fatalf("write HTF DSL: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "XAUUSD", "1h.bin"), marketdata.EncodeBTB1(testHTFSeries()), 0o644); err != nil {
		t.Fatalf("write HTF BTB1: %v", err)
	}

	var out bytes.Buffer
	err = run([]string{
		"report",
		"--dsl-file=" + dslFile,
		"--symbol=XAUUSD",
		"--tf=5m",
		"--range=zone",
		"--slippage=0.05",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out)
	if err != nil {
		t.Fatalf("run report with HTF: %v", err)
	}
	var payload report.Document
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode report JSON: %v\n%s", err, out.String())
	}
	if len(payload.Slices) != 1 {
		t.Fatalf("slices = %+v, want one slice", payload.Slices)
	}
	if payload.Slices[0].HTF == nil || *payload.Slices[0].HTF != "1h" {
		t.Fatalf("slice HTF = %v, want 1h", payload.Slices[0].HTF)
	}
}

func TestLoadRouteKeepsChartSeriesSeparateFromResolvedSourceContext(t *testing.T) {
	_, dataRoot := writeCLIInputs(t)
	symbolDir := filepath.Join(dataRoot, "XAUUSD")
	if err := os.WriteFile(filepath.Join(symbolDir, "1h.bin"), marketdata.EncodeBTB1(testHTFSeries()), 0o644); err != nil {
		t.Fatalf("write source BTB1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(symbolDir, "4h.bin"), marketdata.EncodeBTB1(testHTFSeries()), 0o644); err != nil {
		t.Fatalf("write source HTF BTB1: %v", err)
	}
	route, err := loadRoute(flagSet{
		"symbol": []string{"XAUUSD"}, "tf": []string{"5m"}, "range": []string{"zone"}, "data-root": []string{dataRoot},
	}, dsl.Config{
		"sourceTimeframe": "1h",
		"htf":             map[string]any{"mode": "notAgainst", "timeframe": "auto"},
	})
	if err != nil {
		t.Fatalf("loadRoute: %v", err)
	}
	if route.TF != "5m" || route.SourceTimeframe != "1h" || route.HigherTimeframe != "4h" {
		t.Fatalf("route timeframe identity = chart %q source %q htf %q", route.TF, route.SourceTimeframe, route.HigherTimeframe)
	}
	if route.Series.Len() == route.SourceSeries.Len() {
		t.Fatal("source series must be loaded separately from chart series")
	}
	if route.SourceHTFSeries.Len() == 0 {
		t.Fatal("resolved source HTF series was not loaded")
	}
}

func TestReportJSONDisclosesCanonicalSourceEntryRouteProvenance(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	if err := os.WriteFile(dslFile, []byte(`dsl v7
strategy "C5 report provenance" { description "canonical source route" }
market conditions { slices(XAUUSD 15m) }
setup { type: flag continuation source timeframe 4h }
filters { higher timeframe must agree }
entryTf 15m`), 0o644); err != nil {
		t.Fatalf("write C5 DSL: %v", err)
	}
	symbolDir := filepath.Join(dataRoot, "XAUUSD")
	if err := os.WriteFile(filepath.Join(symbolDir, "15m.bin"), marketdata.EncodeBTB1(testSeries()), 0o644); err != nil {
		t.Fatalf("write entry BTB1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(symbolDir, "4h.bin"), marketdata.EncodeBTB1(testHTFSeries()), 0o644); err != nil {
		t.Fatalf("write source BTB1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(symbolDir, "1d.bin"), marketdata.EncodeBTB1(testHTFSeries()), 0o644); err != nil {
		t.Fatalf("write source HTF BTB1: %v", err)
	}

	var out bytes.Buffer
	if err := run([]string{
		"report", "--dsl-file=" + dslFile, "--symbol=XAUUSD", "--tf=15m", "--range=zone", "--json-only=1", "--data-root=" + dataRoot,
	}, &out); err != nil {
		t.Fatalf("run source-entry report: %v", err)
	}
	var payload report.Document
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode source-entry report: %v\n%s", err, out.String())
	}
	if len(payload.Slices) != 1 {
		t.Fatalf("slices = %+v, want one source-entry slice", payload.Slices)
	}
	slice := payload.Slices[0]
	if slice.SourceTimeframe == nil || *slice.SourceTimeframe != "4h" || slice.EntryTimeframe == nil || *slice.EntryTimeframe != "15m" || slice.SourceBars == nil || *slice.SourceBars != testHTFSeries().Len() {
		t.Fatalf("source-entry slice provenance = %+v", slice)
	}
	if slice.SourceHTF == nil || *slice.SourceHTF != "1d" || slice.SourceHTFBars == nil || *slice.SourceHTFBars != testHTFSeries().Len() {
		t.Fatalf("source-entry slice source HTF provenance = %+v", slice)
	}
	row := payload.EvidenceEnvelope.Rows[0]
	if row.SourceTimeframe == nil || *row.SourceTimeframe != "4h" || row.EntryTimeframe == nil || *row.EntryTimeframe != "15m" || row.SourceBars == nil || *row.SourceBars != testHTFSeries().Len() {
		t.Fatalf("source-entry evidence row provenance = %+v", row)
	}
	if row.SourceHTF == nil || *row.SourceHTF != "1d" || row.SourceHTFBars == nil || *row.SourceHTFBars != testHTFSeries().Len() {
		t.Fatalf("source-entry evidence row source HTF provenance = %+v", row)
	}
}

func TestReportDefaultCostsUseRouteSymbol(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	var out bytes.Buffer
	err := run([]string{
		"report",
		"--dsl-file=" + dslFile,
		"--symbol=XAUUSD",
		"--tf=5m",
		"--range=zone",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out)
	if err != nil {
		t.Fatalf("run report: %v", err)
	}
	var payload report.Document
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode report JSON: %v\n%s", err, out.String())
	}
	if payload.PrimaryCost.Index != 1 || payload.PrimaryCost.Label != "realistic" {
		t.Fatalf("primary cost = %+v, want realistic index 1", payload.PrimaryCost)
	}
	if len(payload.Costs) != 3 {
		t.Fatalf("costs = %+v, want 3 default rows", payload.Costs)
	}
	assertCostRows(t, payload.Costs, []report.CostRow{
		{Label: "raw", Slippage: 0},
		{Label: "realistic", Slippage: 0.06},
		{Label: "harsh", Slippage: 0.12},
	})
}

func TestGridExpandsSetsInStableOrder(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	var out bytes.Buffer
	err := run([]string{
		"grid",
		"--dsl-file", dslFile,
		"--symbol=XAUUSD",
		"--tf=5m",
		"--range=pivot",
		"--set", "maxHoldCandles=4,8",
		"--set=riskUsd=100,200",
		"--slippage=0",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out)
	if err != nil {
		t.Fatalf("run grid: %v", err)
	}
	var payload gridPayload
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode grid JSON: %v\n%s", err, out.String())
	}
	if payload.Range != "pivot" {
		t.Fatalf("range = %q, want pivot", payload.Range)
	}
	if len(payload.Warnings) != 0 {
		t.Fatalf("admitted-route grid warnings = %#v, want []", payload.Warnings)
	}
	if got := len(payload.Variants); got != 4 {
		t.Fatalf("variants = %d, want 4", got)
	}
	want := []map[string]float64{
		{"maxHoldCandles": 4, "riskUsd": 100},
		{"maxHoldCandles": 4, "riskUsd": 200},
		{"maxHoldCandles": 8, "riskUsd": 100},
		{"maxHoldCandles": 8, "riskUsd": 200},
	}
	for i, variant := range payload.Variants {
		if variant.Index != i {
			t.Fatalf("variant[%d].Index = %d, want %d", i, variant.Index, i)
		}
		for key, value := range want[i] {
			if variant.Params[key] != value {
				t.Fatalf("variant[%d].Params[%s] = %v, want %v", i, key, variant.Params[key], value)
			}
		}
		if len(variant.Costs) != 1 || variant.Costs[0].Label != "slip 0" {
			t.Fatalf("variant[%d].Costs = %+v, want one slip 0 row", i, variant.Costs)
		}
	}
}

func TestGridExcludedRouteWarnsAndKeepsZeroVariantsInStableOrder(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	sourcePath := filepath.Join(dataRoot, "XAUUSD", "5m.bin")
	series, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read CLI market series: %v", err)
	}
	excludedDir := filepath.Join(dataRoot, "EURUSD")
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		t.Fatalf("mkdir excluded route: %v", err)
	}
	if err := os.WriteFile(filepath.Join(excludedDir, "5m.bin"), series, 0o644); err != nil {
		t.Fatalf("write excluded-route market series: %v", err)
	}

	var out bytes.Buffer
	if err := run([]string{
		"grid",
		"--dsl-file=" + dslFile,
		"--symbol=EURUSD",
		"--tf=5m",
		"--range=pivot",
		"--set=maxHoldCandles=4,8",
		"--set=riskUsd=100,200",
		"--slippage=0",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out); err != nil {
		t.Fatalf("run excluded-route grid: %v", err)
	}

	var payload gridPayload
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode excluded-route grid JSON: %v\n%s", err, out.String())
	}
	wantWarning := "route EURUSD 5m is excluded by the strategy's slices()/symbols()/timeframes() market conditions; zero trades (matches JS gating)"
	if !reflect.DeepEqual(payload.Warnings, []string{wantWarning}) {
		t.Fatalf("grid warnings = %#v, want one excluded-route warning", payload.Warnings)
	}
	wantParams := []map[string]float64{
		{"maxHoldCandles": 4, "riskUsd": 100},
		{"maxHoldCandles": 4, "riskUsd": 200},
		{"maxHoldCandles": 8, "riskUsd": 100},
		{"maxHoldCandles": 8, "riskUsd": 200},
	}
	if len(payload.Variants) != len(wantParams) {
		t.Fatalf("variants = %d, want %d", len(payload.Variants), len(wantParams))
	}
	for i, variant := range payload.Variants {
		if variant.Index != i || !reflect.DeepEqual(variant.Params, wantParams[i]) {
			t.Fatalf("variant[%d] = index %d params %#v, want index %d params %#v", i, variant.Index, variant.Params, i, wantParams[i])
		}
		if len(variant.Costs) != 1 || variant.Costs[0].Trades != 0 {
			t.Fatalf("variant[%d].Costs = %+v, want one zero-trade row", i, variant.Costs)
		}
	}
}

func TestGridDefaultCostsUseRouteSymbol(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	var out bytes.Buffer
	err := run([]string{
		"grid",
		"--dsl-file", dslFile,
		"--symbol=XAUUSD",
		"--tf=5m",
		"--range=pivot",
		"--set", "riskUsd=100",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out)
	if err != nil {
		t.Fatalf("run grid: %v", err)
	}
	var payload gridPayload
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode grid JSON: %v\n%s", err, out.String())
	}
	if len(payload.Variants) != 1 {
		t.Fatalf("variants = %+v, want one variant", payload.Variants)
	}
	assertCostRows(t, payload.Variants[0].Costs, []report.CostRow{
		{Label: "raw", Slippage: 0},
		{Label: "realistic", Slippage: 0.06},
		{Label: "harsh", Slippage: 0.12},
	})
}

func assertCostRows(t *testing.T, got []report.CostRow, want []report.CostRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cost rows = %+v, want %+v", got, want)
	}
	for i := range got {
		if got[i].Label != want[i].Label || got[i].Slippage != want[i].Slippage {
			t.Fatalf("cost row %d = %s/%v, want %s/%v", i, got[i].Label, got[i].Slippage, want[i].Label, want[i].Slippage)
		}
	}
}

func writeCLIInputs(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	dslPath := filepath.Join(root, "probe.strat")
	if err := os.WriteFile(dslPath, []byte(testDSL), 0o644); err != nil {
		t.Fatalf("write DSL: %v", err)
	}
	dataRoot := filepath.Join(root, "data")
	dir := filepath.Join(dataRoot, "XAUUSD")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "5m.bin"), marketdata.EncodeBTB1(testSeries()), 0o644); err != nil {
		t.Fatalf("write BTB1: %v", err)
	}
	return dslPath, dataRoot
}

func writeConformanceCLIInputs(t *testing.T, name string) (string, string, engine.RunResult) {
	t.Helper()
	runDir := filepath.Join(testsupport.StratConformanceRoot(), "run")
	fixturePath := filepath.Join(runDir, name+".fixture.json")
	fixture, err := engine.LoadRunFixture(fixturePath)
	if err != nil {
		t.Fatalf("load conformance fixture: %v", err)
	}
	goldenRaw, err := os.ReadFile(filepath.Join(runDir, name+".trades.json"))
	if err != nil {
		t.Fatalf("read conformance trades: %v", err)
	}
	var expected engine.RunResult
	if err := json.Unmarshal(goldenRaw, &expected); err != nil {
		t.Fatalf("decode conformance trades: %v", err)
	}
	dataRoot := filepath.Join(t.TempDir(), "data")
	symbolDir := filepath.Join(dataRoot, fixture.Symbol)
	if err := os.MkdirAll(symbolDir, 0o755); err != nil {
		t.Fatalf("mkdir conformance data: %v", err)
	}
	series := marketdata.SeriesFromBars(fixture.Bars)
	if err := os.WriteFile(filepath.Join(symbolDir, fixture.Timeframe+".bin"), marketdata.EncodeBTB1(series), 0o644); err != nil {
		t.Fatalf("write conformance bars: %v", err)
	}
	if fixture.HigherTimeframe != "" {
		htfSeries := marketdata.SeriesFromBars(fixture.HTFBars)
		if err := os.WriteFile(filepath.Join(symbolDir, fixture.HigherTimeframe+".bin"), marketdata.EncodeBTB1(htfSeries), 0o644); err != nil {
			t.Fatalf("write conformance HTF bars: %v", err)
		}
	}
	dslFile, err := filepath.Abs(filepath.Join(runDir, name+".strat"))
	if err != nil {
		t.Fatalf("resolve conformance DSL: %v", err)
	}
	return dslFile, dataRoot, expected
}

func testSeries() marketdata.Series {
	const n = 80
	series := marketdata.NewSeries(n)
	for i := 0; i < n; i++ {
		price := 1900 + float64(i%10)*0.2
		series.T[i] = float64(1_704_067_200_000 + i*5*60*1000)
		series.O[i] = price
		series.H[i] = price + 0.5
		series.L[i] = price - 0.5
		series.C[i] = price + 0.1
		series.V[i] = 100 + float64(i)
	}
	return series
}

func testHTFSeries() marketdata.Series {
	const n = 40
	series := marketdata.NewSeries(n)
	for i := 0; i < n; i++ {
		price := 1900 + float64(i)*0.4
		series.T[i] = float64(1_704_067_200_000 + i*60*60*1000)
		series.O[i] = price
		series.H[i] = price + 1
		series.L[i] = price - 1
		series.C[i] = price + 0.2
		series.V[i] = 1000 + float64(i)
	}
	return series
}

const testDSL = `dsl v7
strategy "CLI Range Fake" {
  description "Small CLI test strategy."
}

market conditions {
  symbols(XAUUSD)
  timeframes(5m)
  sessions(london, ny)
  day type in (ranging, choppy)
}

levels {
  priority(AH, AL, LH, LL, PDH, PDL)
  range edge must be within 1 ATR of level
}

setup {
  type: range break fake
  pierce at least 0.1 ATR
  reclaim within 2 candles
  target range midpoint
}

filters {
  range method: zone
  range active within 8 candles
  tail rejection at least 0.3
  entry distance max 0.5 ATR from level
}

risk {
  stop beyond last 3 candle extreme by 0.25 ATR
  stop size <= 1.4 ATR
}

target {
  fallback: 1R
  minimum reward: 0.6R
}

management {
  move stop to breakeven after 0.75R plus 0.02 ATR
  wait 3 candles after trade
}

execution {
  risk: 200 USD
}
`
