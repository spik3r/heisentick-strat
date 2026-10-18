package engine

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Invented transport data; these tests do not execute or qualify a strategy.
const ordinarySyntheticRequest = `{"schema":"strat-ordinary-backtest-request-v1","requestId":"run.1","operation":"run","strategyId":"ordinary:SYNTH-1","route":{"symbol":"SYNTH","timeframe":"1h"},"execution":{"rangeMethod":"zone","costs":{"feePerUnit":0,"fillOn":"close","slippage":0,"slippageBps":0,"startEquity":10000},"params":{},"forceRoute":false,"chartType":"candles"},"reportingBasis":"ordinary-logical-completed-close-mtm-v1","data":{"schema":"strat-original-bars-v1","profile":"original-chart-bars-v1","bars":[[0,10,12,9,11,0],[3600000,11,13,10,12,1]],"firstT":0,"lastT":3600000,"closedThroughT":7200000,"origin":{"datasetId":"invented","historyGeneration":0,"provenance":"caller-declared"}}}`

func ordinaryTestRequest(t *testing.T, operation string, mutate func(map[string]any)) string {
	t.Helper()
	var m map[string]any
	if e := json.Unmarshal([]byte(ordinarySyntheticRequest), &m); e != nil {
		t.Fatal(e)
	}
	m["operation"] = operation
	if mutate != nil {
		mutate(m)
	}
	raw, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	return string(raw)
}
func ordinaryTestFault(t *testing.T, e error, code string) {
	t.Helper()
	var f *OrdinaryBacktestFault
	if !errors.As(e, &f) || f.Code != "BACKTEST_"+code {
		t.Fatalf("error = %v; want %s", e, code)
	}
}
func ordinaryTestCapture(t *testing.T, operation string) *OrdinaryBacktestCapture {
	t.Helper()
	c, e := DecodeOrdinaryBacktestRequest(operation, ordinaryTestRequest(t, operation, nil), "invented source\n")
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestOrdinaryBacktestCaptureExactBytesAndIsolation(t *testing.T) {
	raw := strings.Replace(ordinarySyntheticRequest, `"data":{`, `"data":{  `, 1)
	source := "synthetic \U0001f600\n"
	c, e := DecodeOrdinaryBacktestRequest("run", raw, source)
	if e != nil {
		t.Fatal(e)
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &object)
	if c.Source() != source || c.RequestJSON() != raw || c.SourceIdentity() != ordinaryIdentity([]byte(source)) || c.RequestIdentity() != ordinaryIdentity([]byte(raw)) || *c.DataIdentity() != ordinaryIdentity(object["data"]) {
		t.Fatal("capture did not bind exact immutable bytes")
	}
	r := c.Request()
	r.Data.Bars[0][1] = 999
	r.Data.Origin.DatasetID = "mutated"
	r.Route.Symbol = "changed"
	id := c.DataIdentity()
	id.SHA256 = "changed"
	if c.Request().Data.Bars[0][1] != 10 || c.Request().Data.Origin.DatasetID != "invented" || c.Request().Route.Symbol != "SYNTH" || c.DataIdentity().SHA256 == "changed" {
		t.Fatal("mutable snapshot leaked into capture")
	}
	encoded, e := json.Marshal(c.Request())
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(encoded), `"slippageBps":0`) || !strings.Contains(string(encoded), `"params":{}`) {
		t.Fatal("explicit zero/empty object dropped")
	}
}

func TestOrdinaryBacktestStrictRequestMutants(t *testing.T) {
	cases := []struct{ name, raw, source, operation, code string }{
		{"source encoding", ordinarySyntheticRequest, string([]byte{0xed, 0xa0, 0x80}), "run", "SOURCE_ENCODING"},
		{"JSON encoding", strings.Replace(ordinarySyntheticRequest, "invented", string([]byte{0xff}), 1), "x", "run", "JSON_ENCODING"},
		{"lone high", strings.Replace(ordinarySyntheticRequest, "invented", `\ud800`, 1), "x", "run", "JSON_ENCODING"},
		{"lone low", strings.Replace(ordinarySyntheticRequest, "invented", `\udfff`, 1), "x", "run", "JSON_ENCODING"},
		{"bad pair", strings.Replace(ordinarySyntheticRequest, "invented", `\ud800\u0041`, 1), "x", "run", "JSON_ENCODING"},
		{"duplicate", strings.Replace(ordinarySyntheticRequest, `"requestId":"run.1"`, `"requestId":"run.1","requestId":"run.2"`, 1), "x", "run", "DUPLICATE_KEY"},
		{"decoded duplicate", strings.Replace(ordinarySyntheticRequest, `"requestId":"run.1"`, `"requestId":"run.1","\u0072equestId":"run.2"`, 1), "x", "run", "DUPLICATE_KEY"},
		{"nested duplicate", strings.Replace(ordinarySyntheticRequest, `"feePerUnit":0`, `"feePerUnit":0,"feePerUnit":1`, 1), "x", "run", "DUPLICATE_KEY"},
		{"trailing document", ordinarySyntheticRequest + ` {}`, "x", "run", "JSON_ENCODING"},
		{"trailing comma", strings.Replace(ordinarySyntheticRequest, `"params":{}`, `"params":{"x":1,}`, 1), "x", "run", "JSON_ENCODING"},
		{"unknown method", ordinarySyntheticRequest, "x", "other", "ARGUMENTS"},
		{"wrong method", ordinarySyntheticRequest, "x", "inspect", "SCHEMA"},
		{"unknown root", strings.Replace(ordinarySyntheticRequest, `"schema":`, `"extra":1,"schema":`, 1), "x", "run", "UNKNOWN_FIELD"},
		{"wrong key case", strings.Replace(ordinarySyntheticRequest, `"requestId":`, `"RequestId":`, 1), "x", "run", "UNKNOWN_FIELD"},
		{"params not empty", strings.Replace(ordinarySyntheticRequest, `"params":{}`, `"params":{"risk":1}`, 1), "x", "run", "UNKNOWN_FIELD"},
		{"params null", strings.Replace(ordinarySyntheticRequest, `"params":{}`, `"params":null`, 1), "x", "run", "SCHEMA"},
		{"force route", strings.Replace(ordinarySyntheticRequest, `"forceRoute":false`, `"forceRoute":true`, 1), "x", "run", "SCHEMA"},
		{"chart transform", strings.Replace(ordinarySyntheticRequest, `"candles"`, `"heikin-ashi"`, 1), "x", "run", "SCHEMA"},
		{"null cost", strings.Replace(ordinarySyntheticRequest, `"feePerUnit":0`, `"feePerUnit":null`, 1), "x", "run", "SCHEMA"},
		{"string cost", strings.Replace(ordinarySyntheticRequest, `"feePerUnit":0`, `"feePerUnit":"0"`, 1), "x", "run", "SCHEMA"},
		{"negative cost", strings.Replace(ordinarySyntheticRequest, `"feePerUnit":0`, `"feePerUnit":-1`, 1), "x", "run", "SCHEMA"},
		{"zero capital", strings.Replace(ordinarySyntheticRequest, `"startEquity":10000`, `"startEquity":0`, 1), "x", "run", "SCHEMA"},
		{"overflow", strings.Replace(ordinarySyntheticRequest, `"startEquity":10000`, `"startEquity":1e999`, 1), "x", "run", "SCHEMA"},
		{"missing Bps", strings.Replace(ordinarySyntheticRequest, `"slippageBps":0,`, "", 1), "x", "run", "SCHEMA"},
		{"null row scalar", strings.Replace(ordinarySyntheticRequest, `[0,10,12,9,11,0]`, `[0,null,12,9,11,0]`, 1), "x", "run", "SCHEMA"},
		{"short row", strings.Replace(ordinarySyntheticRequest, `[0,10,12,9,11,0]`, `[0,10,12,9,11]`, 1), "x", "run", "SCHEMA"},
		{"long row", strings.Replace(ordinarySyntheticRequest, `[0,10,12,9,11,0]`, `[0,10,12,9,11,0,2]`, 1), "x", "run", "SCHEMA"},
		{"fractional t", strings.Replace(ordinarySyntheticRequest, `[0,10,12,9,11,0]`, `[0.5,10,12,9,11,0]`, 1), "x", "run", "SCHEMA"},
		{"unsafe t", strings.Replace(ordinarySyntheticRequest, `[0,10,12,9,11,0]`, `[9007199254740992,10,12,9,11,0]`, 1), "x", "run", "SCHEMA"},
		{"negative volume", strings.Replace(ordinarySyntheticRequest, `[0,10,12,9,11,0]`, `[0,10,12,9,11,-1]`, 1), "x", "run", "SCHEMA"},
	}
	for _, v := range cases {
		t.Run(v.name, func(t *testing.T) {
			c, e := DecodeOrdinaryBacktestRequest(v.operation, v.raw, v.source)
			if c != nil {
				t.Fatal("invalid request created capture")
			}
			ordinaryTestFault(t, e, v.code)
		})
	}
	for _, field := range []string{"requestId", "strategyId"} {
		for _, id := range []string{"", "-a", " x", "é", "a\n", "a/b", strings.Repeat("a", 129)} {
			t.Run(field+"/"+id, func(t *testing.T) {
				raw := ordinaryTestRequest(t, "run", func(m map[string]any) { m[field] = id })
				_, e := DecodeOrdinaryBacktestRequest("run", raw, "x")
				ordinaryTestFault(t, e, "SCHEMA")
			})
		}
	}
	for _, token := range []string{"9007199254740991.1", "1.0000000000000000000000001", "1e-999999999", "90071992547409910e-1"} {
		raw := strings.Replace(ordinarySyntheticRequest, "[0,10,12,9,11,0]", "["+token+",10,12,9,11,0]", 1)
		_, err := DecodeOrdinaryBacktestRequest("run", raw, "x")
		if token == "90071992547409910e-1" {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			ordinaryTestFault(t, err, "SCHEMA")
		}
	}
	for _, token := range []string{"0e999999999", "-0.0", "1.000", "10e-1", "9007199254740991"} {
		if !ordinaryExactSafeInteger(token) {
			t.Fatal("valid decimal integer refused", token)
		}
	}
	for _, id := range []string{"0", "A._:-", strings.Repeat("Z", 128)} {
		raw := ordinaryTestRequest(t, "run", func(m map[string]any) { m["requestId"] = id; m["strategyId"] = id })
		if _, e := DecodeOrdinaryBacktestRequest("run", raw, "x"); e != nil {
			t.Fatal(e)
		}
	}
	for _, replacement := range []string{`\ud83d\ude00`, `\\ud800`} {
		if _, e := DecodeOrdinaryBacktestRequest("run", strings.Replace(ordinarySyntheticRequest, "invented", replacement, 1), "x"); e != nil {
			t.Fatalf("valid Unicode escape refused: %v", e)
		}
	}
}

func TestOrdinaryBacktestFiniteTransportBoundaries(t *testing.T) {
	for _, op := range []string{"inspect", "run"} {
		raw := ordinaryTestRequest(t, op, func(m map[string]any) { m["data"] = nil })
		c, e := DecodeOrdinaryBacktestRequest(op, raw, "x")
		if op == "run" {
			ordinaryTestFault(t, e, "DATA_REQUIRED")
		} else if e != nil || c.DataIdentity() != nil {
			t.Fatal("text inspect refused")
		}
	}
	for _, n := range []int{OrdinaryBacktestMaxBars, OrdinaryBacktestMaxBars + 1} {
		raw := ordinaryTestRequest(t, "run", func(m map[string]any) {
			rows := make([]any, n)
			for i := range rows {
				rows[i] = []any{i, 1, 1, 1, 1, 0}
			}
			m["data"].(map[string]any)["bars"] = rows
		})
		_, e := DecodeOrdinaryBacktestRequest("run", raw, "x")
		if n == OrdinaryBacktestMaxBars && e != nil {
			t.Fatal(e)
		}
		if n > OrdinaryBacktestMaxBars {
			ordinaryTestFault(t, e, "DATA_LIMIT")
		}
	}
	source := strings.Repeat("s", OrdinaryBacktestMaxSourceBytes)
	if _, e := DecodeOrdinaryBacktestRequest("run", ordinarySyntheticRequest, source); e != nil {
		t.Fatal(e)
	}
	_, e := DecodeOrdinaryBacktestRequest("run", ordinarySyntheticRequest, source+"s")
	ordinaryTestFault(t, e, "INPUT_LIMIT")
	raw := ordinarySyntheticRequest + strings.Repeat(" ", OrdinaryBacktestMaxInputBytes-len(ordinarySyntheticRequest)-len(source))
	if _, e := DecodeOrdinaryBacktestRequest("run", raw, source); e != nil {
		t.Fatal(e)
	}
	_, e = DecodeOrdinaryBacktestRequest("run", raw+" ", source)
	ordinaryTestFault(t, e, "INPUT_LIMIT")
	raw = ordinaryTestRequest(t, "inspect", func(m map[string]any) { m["data"] = nil })
	raw += strings.Repeat(" ", OrdinaryBacktestMaxTextRequestBytes-len(raw))
	if _, e := DecodeOrdinaryBacktestRequest("inspect", raw, ""); e != nil {
		t.Fatal(e)
	}
	_, e = DecodeOrdinaryBacktestRequest("inspect", raw+" ", "")
	ordinaryTestFault(t, e, "INPUT_LIMIT")
	for _, depth := range []int{32, 33} {
		raw := []byte(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
		e := ordinaryJSONScan(raw, OrdinaryBacktestMaxInputBytes, 32, 500000)
		if depth == 32 && e != nil {
			t.Fatal(e)
		}
		if depth == 33 {
			ordinaryTestFault(t, e, "INPUT_LIMIT")
		}
	}
	// One root array plus one node per scalar, with no node for an array index.
	for _, count := range []int{499999, 500000} {
		raw := []byte("[" + strings.Repeat("0,", count-1) + "0]")
		e := ordinaryJSONScan(raw, OrdinaryBacktestMaxInputBytes, 32, 500000)
		if count == 499999 && e != nil {
			t.Fatal(e)
		}
		if count == 500000 {
			ordinaryTestFault(t, e, "INPUT_LIMIT")
		}
	}
	// Object root + member name + array + scalar is exactly four nodes.
	if e := ordinaryJSONScan([]byte(`{"x":[0]}`), 100, 2, 4); e != nil {
		t.Fatal(e)
	}
	ordinaryTestFault(t, ordinaryJSONScan([]byte(`{"x":[0]}`), 100, 2, 3), "INPUT_LIMIT")
}

func ordinaryTestProducer() OrdinaryBacktestProducer {
	return OrdinaryBacktestProducer{"spik3r/heisentick-strat", "synthetic-only", strings.Repeat("1", 40), strings.Repeat("2", 40), "js/wasm", "go1.22.12", strings.Repeat("3", 64)}
}
func TestOrdinaryBacktestOperationSpecificErrors(t *testing.T) {
	producer := ordinaryTestProducer()
	digest := strings.Repeat("4", 64)
	for _, op := range []string{"inspect", "run"} {
		c := ordinaryTestCapture(t, op)
		for _, stage := range []string{"transport", "parse", "admission", "execution", "report"} {
			t.Run(op+"/"+stage, func(t *testing.T) {
				var config *string
				if stage != "transport" && stage != "parse" {
					config = &digest
				}
				f := ordinaryFault("INTERNAL", stage, "")
				r, e := c.ErrorEnvelope(op, producer, config, *f)
				if op == "inspect" && stage == "execution" {
					if e == nil {
						t.Fatal("inspect accepted execution error")
					}
					return
				}
				if e != nil {
					t.Fatal(e)
				}
				raw, e := json.Marshal(r)
				if e != nil || !json.Valid(raw) {
					t.Fatal(e)
				}
				if e := ValidateOrdinaryBacktestErrorEnvelope(r, op, c, producer, config); e != nil {
					t.Fatal(e)
				}
				mutate := func(fn func(*OrdinaryBacktestErrorEnvelope)) {
					t.Helper()
					var x OrdinaryBacktestErrorEnvelope
					if e := json.Unmarshal(raw, &x); e != nil {
						t.Fatal(e)
					}
					fn(&x)
					if ValidateOrdinaryBacktestErrorEnvelope(x, op, c, producer, config) == nil {
						t.Fatal("invalid envelope accepted")
					}
				}
				mutate(func(x *OrdinaryBacktestErrorEnvelope) {
					x.Operation = map[string]string{"inspect": "run", "run": "inspect"}[op]
				})
				mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.OK = true })
				mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.Error.Code = "BACKTEST_UNKNOWN" })
				mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.Schema = OrdinaryBacktestReportSchema })
				mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.Diagnostics = nil })
				mutate(func(x *OrdinaryBacktestErrorEnvelope) {
					x.Diagnostics = []OrdinaryBacktestDiagnostic{{Code: "x", Severity: "unknown", Message: "x"}}
				})
				if stage == "transport" {
					mutate(func(x *OrdinaryBacktestErrorEnvelope) { id := "invented"; x.RequestID = &id })
				} else {
					mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.Binding = nil; x.RequestID = nil })
					mutate(func(x *OrdinaryBacktestErrorEnvelope) {
						id := "other-consistent-id"
						x.RequestID = &id
						x.Binding.StrategyID = id
					})
					mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.Binding.StrategyID = "other" })
					mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.Binding.Source.SHA256 = strings.Repeat("5", 64) })
					mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.Binding.Execution.Costs.FeePerUnit = 1 })
					mutate(func(x *OrdinaryBacktestErrorEnvelope) { x.Binding.Producer.Tag = "other" })
					mutate(func(x *OrdinaryBacktestErrorEnvelope) {
						x.Binding.ConfigSHA256 = nil
						if config == nil {
							x.Binding.ConfigSHA256 = &digest
						}
					})
				}
			})
		}
	}
	inspect := ordinaryTestCapture(t, "inspect")
	f := ordinaryFault("OUTPUT_LIMIT", "report", "")
	if _, e := inspect.ErrorEnvelope("inspect", producer, nil, *f); e != nil {
		t.Fatal("parse-rejected inspection serialization must preserve truthful null config", e)
	}
	run := ordinaryTestCapture(t, "run")
	if _, e := run.ErrorEnvelope("run", producer, nil, *f); e == nil {
		t.Fatal("run report error allowed missing config")
	}
	if _, e := run.ErrorEnvelope("inspect", producer, &digest, *f); e == nil {
		t.Fatal("cross-operation capture accepted")
	}
	// Native zero-Bps omission remains separate from the explicit binding field.
	if math.Signbit(run.Request().Execution.Costs.SlippageBps) {
		t.Fatal("unexpected negative zero")
	}
}

func TestOrdinaryBacktestSchemaReferenceClosure(t *testing.T) {
	roots := map[string]string{"request": "#/$defs/request", "inspection": "#/$defs/inspectResponse", "report": "#/$defs/runResponse"}
	for name, want := range roots {
		raw, e := os.ReadFile(filepath.Join("..", "spec", "schemas", "ordinary-backtest-"+name+"-v1.schema.json"))
		if os.IsNotExist(e) { // Explicit-file transport tests can run from the repository root.
			raw, e = os.ReadFile(filepath.Join("spec", "schemas", "ordinary-backtest-"+name+"-v1.schema.json"))
		}
		if e != nil {
			t.Fatal(e)
		}
		var schema map[string]any
		if e = json.Unmarshal(raw, &schema); e != nil {
			t.Fatal(e)
		}
		if schema["$ref"] != want {
			t.Fatalf("%s selected wrong operation root", name)
		}
		defs := schema["$defs"].(map[string]any)
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				if ref, ok := x["$ref"].(string); ok {
					if !strings.HasPrefix(ref, "#/$defs/") || defs[strings.TrimPrefix(ref, "#/$defs/")] == nil {
						t.Fatal("unresolved or external schema reference", ref)
					}
				}
				for _, child := range x {
					walk(child)
				}
			case []any:
				for _, child := range x {
					walk(child)
				}
			}
		}
		walk(schema)
		if reflect.DeepEqual(schema["oneOf"], []any{}) {
			t.Fatal("empty root union")
		}
	}
}
