package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report"
)

func ordinaryBridgeTestStamp(t *testing.T) {
	t.Helper()
	tag, commit, tree, schema := ordinaryBacktestBuildTag, ordinaryBacktestBuildSourceCommit, ordinaryBacktestBuildSourceTree, ordinaryBacktestBuildSchemaSetSHA256
	t.Cleanup(func() {
		ordinaryBacktestBuildTag, ordinaryBacktestBuildSourceCommit, ordinaryBacktestBuildSourceTree, ordinaryBacktestBuildSchemaSetSHA256 = tag, commit, tree, schema
	})
	// Invented test-only identities, never a release/artifact attestation.
	ordinaryBacktestBuildTag = "ordinary-public-api-test-only"
	ordinaryBacktestBuildSourceCommit = strings.Repeat("a", 40)
	ordinaryBacktestBuildSourceTree = strings.Repeat("b", 40)
	ordinaryBacktestBuildSchemaSetSHA256 = "ed1817b86c68eb4f12c6a249c93c1d87b9fb0fc48a939bc5fe70e110d33ed6d8"
}

func ordinaryBridgeFixture(t *testing.T, name, operation string) (string, string, engine.RunFixture) {
	t.Helper()
	base := filepath.Join("..", "..", "conformance", "run", name)
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	var fixture engine.RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Case, fixture.StrategyID = "ordinary.public.1", "ordinary:public-fixture"
	if fixture.RangeMethod == "" {
		fixture.RangeMethod = "zone"
	}
	if fixture.Costs.StartEquity == 0 {
		fixture.Costs.StartEquity = 10000
	}
	if fixture.Costs.FillOn == "" {
		fixture.Costs.FillOn = "close"
	}
	fixture.Bars = rowsToBars(fixture.RawBars)
	duration, ok := dsl.SequentialFullTimeframeMS(fixture.Timeframe)
	if !ok || len(fixture.RawBars) == 0 {
		t.Fatal("fixture must have supported original bars")
	}
	bars := make([][6]float64, len(fixture.RawBars))
	for i, row := range fixture.RawBars {
		if len(row) != 6 {
			t.Fatal("fixture row shape")
		}
		copy(bars[i][:], row)
	}
	c := fixture.Costs
	request := engine.OrdinaryBacktestRequest{
		Schema: engine.OrdinaryBacktestRequestSchema, RequestID: fixture.Case, Operation: operation, StrategyID: fixture.StrategyID,
		Route: engine.OrdinaryBacktestRoute{Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}, ReportingBasis: engine.OrdinaryBacktestReportingBasis,
		Execution: engine.OrdinaryBacktestExecution{RangeMethod: fixture.RangeMethod, ChartType: "candles", Costs: engine.OrdinaryBacktestCosts{
			FeePerUnit: c.FeePerUnit, FillOn: c.FillOn, Slippage: c.Slippage, SlippageBps: c.SlippageBps, StartEquity: c.StartEquity}},
		Data: &engine.OrdinaryBacktestData{Schema: "strat-original-bars-v1", Profile: "original-chart-bars-v1", Bars: bars,
			FirstT: bars[0][0], LastT: bars[len(bars)-1][0], ClosedThroughT: bars[len(bars)-1][0] + float64(duration),
			Origin: engine.OrdinaryBacktestOrigin{DatasetID: "invented-public-bridge-fixture", Provenance: "caller-declared"}},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded), string(source), fixture
}

func ordinaryBridgeMutate(t *testing.T, raw string, mutate func(map[string]any)) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	mutate(m)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func ordinaryBridgeCheckError(t *testing.T, raw, operation, code, stage string, capture *engine.OrdinaryBacktestCapture, config *string) engine.OrdinaryBacktestErrorEnvelope {
	t.Helper()
	var out engine.OrdinaryBacktestErrorEnvelope
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if out.Error.Code != "BACKTEST_"+code || out.Error.Stage != stage {
		t.Fatalf("got %s", raw)
	}
	if err := engine.ValidateOrdinaryBacktestErrorEnvelope(out, operation, capture, ordinaryBacktestProducer(), config); err != nil {
		t.Fatalf("invalid error envelope: %v: %s", err, raw)
	}
	return out
}

func TestOrdinaryBacktestBridgeInspectionDoesNotExecute(t *testing.T) {
	ordinaryBridgeTestStamp(t)
	raw, source, _ := ordinaryBridgeFixture(t, "sar-named-level-allow", "inspect")
	for _, textOnly := range []bool{false, true} {
		raw := raw
		if textOnly {
			raw = ordinaryBridgeMutate(t, raw, func(m map[string]any) { m["data"] = nil })
		}
		for _, source := range []string{source, strings.Replace(source, "Named Level Sweep Synthetic", "A new user source without allowlisting", 1)} {
			deps := ordinaryBacktestDefaultDependencies()
			deps.run = func(*engine.OrdinaryBacktestCapture) (*engine.OrdinaryBacktestRun, *engine.OrdinaryBacktestPlan, error) {
				t.Fatal("inspection executed")
				return nil, nil, nil
			}
			var out ordinaryBacktestInspection
			result := ordinaryBacktestJSON("inspect", raw, source, deps)
			if err := json.Unmarshal([]byte(result), &out); err != nil {
				t.Fatal(err)
			}
			if !out.OK || out.Schema != engine.OrdinaryBacktestInspectionSchema || out.Operation != "inspect" || out.Readiness.Execution != "not-run" || out.Readiness.Parse != "accepted" || out.Binding.ConfigSHA256 == nil || out.SourceFacts == nil || out.SourceFacts.SetupType == "" || out.Capabilities.Accounting != "supported" {
				t.Fatalf("invalid inspection: %s", result)
			}
			if textOnly {
				if out.Binding.Data != nil || out.Readiness.Data != "absent" || out.Readiness.Preflight != "not-evaluated" {
					t.Fatalf("text-only admission: %s", result)
				}
			} else if out.Binding.Data == nil || out.Readiness.Data != "validated" || out.Readiness.Preflight != "admitted" {
				t.Fatalf("data admission: %s", result)
			}
			for text, identity := range map[string]engine.OrdinaryBacktestIdentity{raw: out.Binding.Request, source: out.Binding.Source} {
				sum := sha256.Sum256([]byte(text))
				if identity.SHA256 != hex.EncodeToString(sum[:]) || identity.UTF8Bytes != len(text) {
					t.Fatal("capture bytes changed")
				}
			}
		}
	}
}

func TestOrdinaryBacktestBridgeReportsPreserveNativeAndProjection(t *testing.T) {
	ordinaryBridgeTestStamp(t)
	for _, name := range []string{"sar-named-level-allow", "sar-named-level-block", "money-partial-exit"} {
		t.Run(name, func(t *testing.T) {
			raw, source, fixture := ordinaryBridgeFixture(t, name, "run")
			if name == "money-partial-exit" {
				// A NEW source and single-series request for partial-management
				// coverage. The original HTF-bearing conformance fixture is not
				// changed or claimed to execute under this bounded profile.
				source = strings.Replace(source, "  higher timeframe must agree\n", "", 1)
			}
			deps := ordinaryBacktestDefaultDependencies()
			deps.prepare = func(*engine.OrdinaryBacktestCapture) (*engine.OrdinaryBacktestPlan, error) {
				t.Fatal("run trusted an inspection")
				return nil, nil
			}
			runs := 0
			deps.run = func(c *engine.OrdinaryBacktestCapture) (*engine.OrdinaryBacktestRun, *engine.OrdinaryBacktestPlan, error) {
				runs++
				return engine.RunOrdinaryBacktest(c)
			}
			result := ordinaryBacktestJSON("run", raw, source, deps)
			var out ordinaryBacktestReport
			if err := json.Unmarshal([]byte(result), &out); err != nil {
				t.Fatal(err)
			}
			if !out.OK || out.Schema != engine.OrdinaryBacktestReportSchema || out.Operation != "run" || runs != 1 || out.Execution.BarCount != len(fixture.RawBars) || len(out.Equity) != len(fixture.RawBars) {
				t.Fatalf("invalid report: %s", result)
			}
			parsed, err := dsl.Parse(source)
			if err != nil || len(parsed.Errors) != 0 {
				t.Fatalf("baseline parse: %v, %v", err, parsed.Errors)
			}
			// Independently execute the exact public original-series input.
			// Do not inherit an unrelated fixture's declared HTF or companion bars.
			want, err := engine.Run(engine.RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(fixture.Bars),
				StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs})
			if err != nil {
				t.Fatal(err)
			}
			want.Case = fixture.Case
			wantRaw, err := json.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal([]byte(result), &object); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(object["nativeRun"], wantRaw) {
				t.Fatalf("native envelope changed\ngot %s\nwant %s", object["nativeRun"], wantRaw)
			}
			capture, err := engine.DecodeOrdinaryBacktestRequest("run", raw, source)
			if err != nil {
				t.Fatal(err)
			}
			run, plan, err := engine.RunOrdinaryBacktest(capture)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := report.ProjectOrdinaryBacktestFinancials(run.Financials())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(out.Account, projection.Account) || !reflect.DeepEqual(out.Completed, projection.CompletedPositions) || !reflect.DeepEqual(out.Drawdown, projection.Drawdown) || !reflect.DeepEqual(out.Equity, projection.Equity) || !reflect.DeepEqual(out.Analyses, projection.Analyses) || *out.Binding.ConfigSHA256 != plan.ConfigIdentity() {
				t.Fatal("projection or config identity changed")
			}
			if !strings.Contains(string(object["binding"]), `"slippageBps":0`) || strings.Contains(string(object["nativeRun"]), `"slippageBps":0`) {
				t.Fatal("explicit binding versus native omitempty changed")
			}
			if out.Analyses.MonthlyReturns.Status != "unavailable" || out.Completed.Expectancy.Value != nil || out.Completed.Expectancy.Reason != "not-in-contract" {
				t.Fatal("unsupported statistics synthesized")
			}
		})
	}
}

func TestOrdinaryBacktestBridgeTransportAndBuildFailClosed(t *testing.T) {
	ordinaryBridgeTestStamp(t)
	raw, source, _ := ordinaryBridgeFixture(t, "sar-named-level-allow", "run")
	cases := []struct{ raw, source, code string }{
		{raw, string([]byte{0xff}), "SOURCE_ENCODING"},
		{raw + " {}", source, "JSON_ENCODING"},
		{strings.Replace(raw, `"requestId":`, `"requestId":"first","\u0072equestId":`, 1), source, "DUPLICATE_KEY"},
		{strings.Replace(raw, `"schema":`, `"extra":true,"schema":`, 1), source, "UNKNOWN_FIELD"},
		{ordinaryBridgeMutate(t, raw, func(m map[string]any) { m["operation"] = "inspect" }), source, "SCHEMA"},
		{ordinaryBridgeMutate(t, raw, func(m map[string]any) { m["data"] = nil }), source, "DATA_REQUIRED"},
		{strings.Replace(raw, "invented-public-bridge-fixture", `\ud800`, 1), source, "JSON_ENCODING"},
		{raw, strings.Repeat("s", engine.OrdinaryBacktestMaxSourceBytes+1), "INPUT_LIMIT"},
	}
	for _, tc := range cases {
		ordinaryBridgeCheckError(t, runOrdinaryBacktestReportJSON(tc.raw, tc.source), "run", tc.code, "transport", nil, nil)
	}
	ordinaryBacktestBuildSourceTree = ""
	ordinaryBridgeCheckError(t, runOrdinaryBacktestReportJSON(raw, source), "run", "INTERNAL", "transport", nil, nil)
	ordinaryBridgeCheckError(t, runOrdinaryBacktestReportJSON("{", source), "run", "JSON_ENCODING", "transport", nil, nil)
}

func TestOrdinaryBacktestBridgeBoundErrorsRetainStageAndConfig(t *testing.T) {
	ordinaryBridgeTestStamp(t)
	for _, operation := range []string{"inspect", "run"} {
		raw, source, _ := ordinaryBridgeFixture(t, "sar-named-level-allow", operation)
		parseSource := "dsl v999\n"
		capture, err := engine.DecodeOrdinaryBacktestRequest(operation, raw, parseSource)
		if err != nil {
			t.Fatal(err)
		}
		ordinaryBridgeCheckError(t, ordinaryBacktestJSON(operation, raw, parseSource, ordinaryBacktestDefaultDependencies()), operation, "PARSE_REJECTED", "parse", capture, nil)
		for _, mutate := range []func(map[string]any){
			func(m map[string]any) { m["route"].(map[string]any)["symbol"] = "OFF-ROUTE" },
			func(m map[string]any) { m["data"].(map[string]any)["closedThroughT"] = float64(0) },
		} {
			bad := ordinaryBridgeMutate(t, raw, mutate)
			capture, err := engine.DecodeOrdinaryBacktestRequest(operation, bad, source)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := engine.PrepareOrdinaryBacktest(capture)
			var fault *engine.OrdinaryBacktestFault
			if plan == nil || !errors.As(err, &fault) {
				t.Fatalf("missing admission refusal: %v", err)
			}
			digest := plan.ConfigIdentity()
			ordinaryBridgeCheckError(t, ordinaryBacktestJSON(operation, bad, source, ordinaryBacktestDefaultDependencies()), operation, strings.TrimPrefix(fault.Code, "BACKTEST_"), "admission", capture, &digest)
		}
	}
	raw, source, _ := ordinaryBridgeFixture(t, "sar-named-level-allow", "run")
	capture, err := engine.DecodeOrdinaryBacktestRequest("run", raw, source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := engine.PrepareOrdinaryBacktest(capture)
	if err != nil {
		t.Fatal(err)
	}
	digest := plan.ConfigIdentity()
	deps := ordinaryBacktestDefaultDependencies()
	deps.project = func(engine.OrdinaryBacktestFinancials) (report.OrdinaryBacktestProjection, error) {
		panic("private source and bars must not escape")
	}
	result := ordinaryBacktestJSON("run", raw, source, deps)
	ordinaryBridgeCheckError(t, result, "run", "INTERNAL", "report", capture, &digest)
	if strings.Contains(result, "private source") {
		t.Fatal("panic text leaked")
	}
	deps = ordinaryBacktestDefaultDependencies()
	deps.project = func(a engine.OrdinaryBacktestFinancials) (report.OrdinaryBacktestProjection, error) {
		p, err := report.ProjectOrdinaryBacktestFinancials(a)
		v := math.Inf(1)
		p.Account.MarkedEndEquity.Value = &v
		return p, err
	}
	ordinaryBridgeCheckError(t, ordinaryBacktestJSON("run", raw, source, deps), "run", "NONFINITE_OUTPUT", "report", capture, &digest)
}

func TestOrdinaryBacktestBridgeOutputBudgetAndNativeIdentity(t *testing.T) {
	for _, value := range []any{strings.Repeat("x", engine.OrdinaryBacktestMaxOutputBytes), math.NaN(), math.Inf(-1)} {
		out, err := ordinaryBacktestEncode(value)
		if out != nil || err == nil {
			t.Fatal("unbounded or nonfinite output accepted")
		}
	}
	var nested any = 1
	for i := 0; i < engine.OrdinaryBacktestMaxDepth+2; i++ {
		nested = []any{nested}
	}
	if _, err := ordinaryBacktestEncode(nested); err == nil {
		t.Fatal("output depth ignored")
	}
	cycle := map[string]any{}
	cycle["cycle"] = cycle
	if _, err := ordinaryBacktestEncode(cycle); err == nil {
		t.Fatal("cyclic metadata was unbounded")
	}
	raw, source, _ := ordinaryBridgeFixture(t, "sar-named-level-allow", "run")
	capture, err := engine.DecodeOrdinaryBacktestRequest("run", raw, source)
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := engine.RunOrdinaryBacktest(capture)
	if err != nil {
		t.Fatal(err)
	}
	native, financials := run.NativeResult(), run.Financials()
	for _, mutate := range []func(*engine.RunResult){
		func(n *engine.RunResult) { n.Case = "another-request" },
		func(n *engine.RunResult) { n.StrategyID = "another-strategy" },
		func(n *engine.RunResult) { n.Costs.StartEquity++ },
		func(n *engine.RunResult) { n.TradeCount++ },
	} {
		bad := native
		mutate(&bad)
		if ordinaryBacktestValidateNative(capture.Request(), bad, financials) == nil {
			t.Fatal("native binding mutant accepted")
		}
	}
}

func TestOrdinaryBacktestBridgeExactOutputBoundaries(t *testing.T) {
	for _, value := range []string{
		strings.Repeat("x", engine.OrdinaryBacktestMaxOutputBytes-2),
		strings.Repeat(`"`, (engine.OrdinaryBacktestMaxOutputBytes-2)/2),
	} {
		raw, err := ordinaryBacktestEncode(value)
		if err != nil || len(raw) != engine.OrdinaryBacktestMaxOutputBytes {
			t.Fatalf("exact byte ceiling refused: %v, bytes %d", err, len(raw))
		}
		if raw, err := ordinaryBacktestEncode(value + "x"); err == nil || raw != nil {
			t.Fatal("byte ceiling plus one accepted")
		}
	}
	for _, depth := range []int{engine.OrdinaryBacktestMaxDepth, engine.OrdinaryBacktestMaxDepth + 1} {
		var nested any = nil
		for i := 0; i < depth; i++ {
			nested = []any{nested}
		}
		_, err := ordinaryBacktestEncode(nested)
		if (err == nil) != (depth == engine.OrdinaryBacktestMaxDepth) {
			t.Fatalf("exact container depth %d: %v", depth, err)
		}
	}
	for _, count := range []int{engine.OrdinaryBacktestMaxNodes - 1, engine.OrdinaryBacktestMaxNodes} {
		// One array node plus one node per primitive; omitted native fields
		// cannot consume the final JSON node allowance.
		_, err := ordinaryBacktestEncode(make([]int, count))
		if (err == nil) != (count+1 == engine.OrdinaryBacktestMaxNodes) {
			t.Fatalf("exact node count %d: %v", count+1, err)
		}
	}
	for _, native := range []engine.RunResult{
		{},
		{Trades: []engine.Trade{{NoStop: true, NoTarget: true, Meta: engine.TradeMeta{}}}},
		{Trades: []engine.Trade{{NoStop: true, Partial: true, Rule: "rule", Meta: engine.TradeMeta{"escaped": "<>&\u2028\n"}}}},
	} {
		got, err := ordinaryBacktestEncode(native)
		want, marshalErr := json.Marshal(native)
		if err != nil || marshalErr != nil || !bytes.Equal(got, want) {
			t.Fatalf("existing native JSON changed: %v/%v", err, marshalErr)
		}
	}
	if _, err := ordinaryBacktestEncode(json.RawMessage(`{"hidden":"custom marshaler"}`)); err == nil {
		t.Fatal("unbounded custom marshaler accepted")
	}
}

// Invoked explicitly by the bounded parity script. Values below are synthetic
// transport/runner evidence and do not constitute any release receipt.
func TestOrdinaryBacktestParityCorpus(t *testing.T) {
	input, output := os.Getenv("ORDINARY_BACKTEST_PARITY_INPUT"), os.Getenv("ORDINARY_BACKTEST_PARITY_OUTPUT")
	if input == "" && output == "" {
		t.Skip("explicit native/WASM parity corpus only")
	}
	if input == "" || output == "" {
		t.Fatal("both parity paths are required")
	}
	ordinaryBridgeTestStamp(t)
	file, err := os.Open(input)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 16<<20 {
		t.Fatal("parity corpus byte limit")
	}
	var cases []struct {
		Name         string          `json:"name"`
		Operation    string          `json:"operation"`
		RequestJSON  string          `json:"requestJSON"`
		Source       string          `json:"source"`
		Response     json.RawMessage `json:"response,omitempty"`
		ExpectedCode *string         `json:"expectedCode"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) > 128 {
		t.Fatal("parity corpus case limit")
	}
	for i := range cases {
		c := &cases[i]
		if c.Operation != "inspect" && c.Operation != "run" {
			t.Fatal("invalid parity operation")
		}
		c.Response = json.RawMessage(ordinaryBacktestJSON(c.Operation, c.RequestJSON, c.Source, ordinaryBacktestDefaultDependencies()))
	}
	raw, err = json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 32<<20 {
		t.Fatal("parity output byte limit")
	}
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
