package report

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
)

const retestReportSource = `dsl v7
strategy "Invented report retest" { description "Mechanical supplied-bar test only." }
market conditions {
 slices(XAUUSD 1h)
 sessions(london, ny)
 day type in (trending, ranging, choppy)
 movement below 1.0
}
setup {
 type: supply demand
 source timeframe 4h
 retest on entry timeframe
 patterns(DBR, RBR, RBD, DBD)
 base 1 to 4 candles
 impulse at least 2.5 ATR within 4 candles
 zone width max 1.0 ATR
 retest within 96 candles
 retest first touch only
 retest must reject zone
 wait 0 candles after zone
}
filters {
 higher timeframe off
 side both
}
risk {
 stop: beyond zone + 0.3 ATR min 0.4 ATR max 3 ATR
}
target { target 2R }
management {
 breakeven off
 wait 6 candles after trade
}
entryTf 1h
`

// Invented OHLC recipe: calm, one base, source rally, entry-chart rejection.
// Input aggregation here constructs the supplied test data, never statistics.
func retestReportRequest(t *testing.T) (string, string) {
	t.Helper()
	chart := make([][]float64, 200)
	for i := range chart {
		o, c, h, l := 100.0, 100.0, 100.5, 99.5
		switch {
		case i >= 120 && i < 124:
			h, l = 100.2, 99.8
		case i >= 124 && i < 140:
			o = 100 + 0.5*float64(i-124)
			c = o + 0.5
			h, l = c+0.1, o-0.1
		case i >= 140:
			o, c, h, l = 108, 108, 108.5, 107.5
		}
		if i == 157 {
			o, h, l, c = 108, 108.2, 100.1, 101
		}
		chart[i] = []float64{float64(i) * 3600000, o, h, l, c, 1}
	}
	source := make([][]float64, 50)
	for i := range source {
		high, low := math.Inf(-1), math.Inf(1)
		for j := 4 * i; j < 4*i+4; j++ {
			high = math.Max(high, chart[j][2])
			low = math.Min(low, chart[j][3])
		}
		source[i] = []float64{chart[4*i][0], chart[4*i][1], high, low, chart[4*i+3][4], 4}
	}
	request := map[string]any{"schema": engine.SupplyDemandRetestRequestSchema, "case": "invented-retest-report", "strategyId": "invented-supply-demand", "symbol": "XAUUSD", "timeframe": "1h", "sourceTimeframe": "4h", "rangeMethod": "zone", "higherTimeframe": "", "sourceHtfBars": [][]float64{}, "costs": map[string]any{"feePerUnit": 0.01, "slippage": 0.02, "slippageBps": 0.0, "startEquity": 10000.0, "fillOn": "close"}, "bars": chart, "sourceBars": source}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), retestReportSource
}

func retestReportExecution(t *testing.T) engine.SupplyDemandRetestExecution {
	t.Helper()
	raw, source := retestReportRequest(t)
	execution, err := engine.RunSupplyDemandRetestWithAccounting([]byte(raw), source)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Run.TradeCount != 1 {
		t.Fatalf("expected one invented trade, got %d", execution.Run.TradeCount)
	}
	return execution
}

func retestReportDocument(t *testing.T, e engine.SupplyDemandRetestExecution) SupplyDemandRetestReport {
	t.Helper()
	m, err := ProjectSequentialMetrics(e.Run, e.Accounting)
	if err != nil {
		t.Fatal(err)
	}
	m.Basis = SupplyDemandRetestMetricsBasis
	return SupplyDemandRetestReport{Schema: SupplyDemandRetestResultSchema, ContractVersion: 1, Identity: &e.Identity, Run: &e.Run, Metrics: &m, Terminal: &e.Terminal, Capabilities: &SupplyDemandRetestCapabilities{Headline: true, Trades: true, Equity: true, Limits: SupplyDemandRetestLimits{500000, 64 << 20, 1 << 20, 64 << 20, 500000000}}}
}

func TestSupplyDemandRetestReportNativeProjectionAndIdentity(t *testing.T) {
	raw, source := retestReportRequest(t)
	execution := retestReportExecution(t)
	encoded := RunSupplyDemandRetestReport(raw, source)
	var result SupplyDemandRetestReport
	if err := json.Unmarshal(encoded, &result); err != nil || result.Error != nil {
		t.Fatalf("report refused: %s (%v)", encoded, err)
	}
	expected, err := ProjectSequentialMetrics(execution.Run, execution.Accounting)
	if err != nil {
		t.Fatal(err)
	}
	expected.Basis = SupplyDemandRetestMetricsBasis
	if !reflect.DeepEqual(*result.Metrics, expected) {
		t.Fatal("dedicated projection changed native reducer output")
	}
	if result.Identity.RequestSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) || result.Identity.SourceSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(source))) {
		t.Fatal("exact source/request identity lost")
	}
	legacy, err := json.Marshal(execution.Run)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["run"]) != string(legacy) {
		t.Fatal("retained RunResult bytes changed")
	}
	if result.Capabilities.Live || result.Capabilities.Paper || result.Capabilities.Monthly || result.Capabilities.Groupings || result.Capabilities.Portfolio || result.Capabilities.RDistribution {
		t.Fatal("unqualified capability advertised")
	}
	if result.Terminal.Position != "flat" || result.Metrics.Headline.Net != execution.Accounting.FinalRealized || len(result.Metrics.Equity) != 200 {
		t.Fatal("terminal/cash/marks binding lost")
	}
	for i := 0; i < 3; i++ {
		if string(RunSupplyDemandRetestReport(raw, source)) != string(encoded) {
			t.Fatal("repeated invocation changed result")
		}
	}
}

func TestSupplyDemandRetestReportNoTradesAndRefusals(t *testing.T) {
	raw, source := retestReportRequest(t)
	var request map[string]any
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}
	bars := request["bars"].([]any)
	bars[157] = []any{float64(157) * 3600000, 108.0, 108.5, 107.5, 108.0, 1.0}
	quiet, _ := json.Marshal(request)
	var result SupplyDemandRetestReport
	if err := json.Unmarshal(RunSupplyDemandRetestReport(string(quiet), source), &result); err != nil || result.Error != nil {
		t.Fatalf("quiet request refused: %+v %v", result.Error, err)
	}
	if result.Run.TradeCount != 0 || result.Metrics.Headline.Net != 0 || result.Metrics.Headline.WinRate != nil || result.Metrics.Headline.WinRateReason != "no-trades" {
		t.Fatal("no-trade conventions changed")
	}
	for _, input := range []struct{ raw, source string }{
		{"{}", source}, {raw + " {}", source}, {strings.Replace(raw, `"case":`, `"extra":true,"case":`, 1), source},
		{strings.Replace(raw, `"case":`, `"case":"duplicate","case":`, 1), source},
		{strings.Replace(raw, `"case":"`, `"case":"\ud800`, 1), source}, {string([]byte{0xff}), source}, {raw, string([]byte{0xff})},
	} {
		encoded := RunSupplyDemandRetestReport(input.raw, input.source)
		var got map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got["schema"] == nil || got["contractVersion"] == nil || got["error"] == nil || len(encoded) > 2048 {
			t.Fatalf("partial or unbounded failure: %s", encoded)
		}
	}
}

func TestSupplyDemandRetestReportAccountingCorruptionRefusesAll(t *testing.T) {
	for _, mutate := range []func(*engine.SupplyDemandRetestExecution){
		func(e *engine.SupplyDemandRetestExecution) { e.Accounting.FinalRealized = math.Inf(1) },
		func(e *engine.SupplyDemandRetestExecution) { e.Accounting.Marks[157].Equity = math.NaN() },
		func(e *engine.SupplyDemandRetestExecution) { e.Accounting.Events[0].Amount += 1 },
		func(e *engine.SupplyDemandRetestExecution) { e.Accounting.Trades[0].NetPnL += 1 },
		func(e *engine.SupplyDemandRetestExecution) {
			e.Run.Trades[0].Meta["unknown"] = map[string]any{"nested": true}
		},
		func(e *engine.SupplyDemandRetestExecution) { e.Run.Trades[0].Meta["chochLevel"] = []float64{1} },
		func(e *engine.SupplyDemandRetestExecution) { e.Run.Trades[0].Meta["zoneHigh"] = math.Inf(1) },
		func(e *engine.SupplyDemandRetestExecution) { e.Identity.WorkScore = 500000001 },
		func(e *engine.SupplyDemandRetestExecution) { e.Terminal.EndOfTestTrades++ },
	} {
		e := retestReportExecution(t)
		mutate(&e)
		if data, err := EncodeSupplyDemandRetestReport(e); err == nil || data != nil {
			t.Fatalf("corruption returned partial/success output: %s %v", data, err)
		}
	}
}

type retestPoisonMarshaler struct{ called *bool }

func (p retestPoisonMarshaler) MarshalJSON() ([]byte, error) {
	*p.called = true
	return []byte(`"poison"`), nil
}

func TestSupplyDemandRetestReportBoundCoversCustomProjectionAndEscapes(t *testing.T) {
	for _, brackets := range []int{0, 1, 2, 3} {
		e := retestReportExecution(t)
		doc := retestReportDocument(t, e)
		doc.Run.Case = strings.Repeat("<>&\u2028\u2029", 10)
		doc.Identity.Case = doc.Run.Case
		tr := &doc.Run.Trades[0]
		tr.NoStop = brackets&1 != 0
		tr.NoTarget = brackets&2 != 0
		tr.Entry = math.MaxFloat64
		tr.Exit = -math.MaxFloat64
		tr.PnL = math.SmallestNonzeroFloat64
		tr.Meta["chochLevel"] = -math.MaxFloat64
		upper, err := supplyDemandRetestOutputBound(doc, SupplyDemandRetestMaxOutputBytes)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if uint64(len(encoded)) > upper {
			t.Fatalf("encoded %d > pre-marshal bound %d", len(encoded), upper)
		}
		if _, err := supplyDemandRetestOutputBound(doc, upper); err != nil {
			t.Fatal("exact bound refused", err)
		}
		if _, err := supplyDemandRetestOutputBound(doc, upper-1); err == nil {
			t.Fatal("one-byte-over bound admitted")
		}
	}
	e := retestReportExecution(t)
	called := false
	e.Run.Trades[0].Meta["zoneHigh"] = retestPoisonMarshaler{&called}
	if raw, err := EncodeSupplyDemandRetestReport(e); err == nil || raw != nil || called {
		t.Fatal("metadata marshaler ran before strict shape refusal")
	}
}

func TestSupplyDemandRetestReportBoundCountsWidthsAndUnknownTypes(t *testing.T) {
	for _, value := range []any{math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64, int64(math.MinInt64), uint64(math.MaxUint64), strings.Repeat("\x00<>&\"\\\u2028", 100)} {
		b := retestJSONBound{limit: SupplyDemandRetestMaxOutputBytes}
		if err := b.value(reflect.ValueOf(value)); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(value)
		if err != nil || uint64(len(raw)) > b.size {
			t.Fatalf("unbounded encoding %s %d %v", raw, b.size, err)
		}
	}
	for _, value := range []any{math.NaN(), math.Inf(1), complex(1, 2), map[string]any{"unknown": true}, make([]SequentialEquityPoint, 500001)} {
		b := retestJSONBound{limit: SupplyDemandRetestMaxOutputBytes}
		if err := b.value(reflect.ValueOf(value)); err == nil {
			t.Fatalf("unexpected shape admitted: %T", value)
		}
	}
	// This maximum profile-sized output array exceeds the conservative byte
	// bound. No JSON serializer or corresponding encoded allocation is invoked.
	b := retestJSONBound{limit: SupplyDemandRetestMaxOutputBytes}
	if err := b.value(reflect.ValueOf(make([]SequentialEquityPoint, 500000))); err == nil {
		t.Fatal("large output escaped pre-serialization byte limit")
	}
	for _, limit := range []uint64{0, 1} {
		b := retestJSONBound{limit: limit}
		if err := b.text(""); err == nil {
			t.Fatal("small-limit arithmetic underflow")
		}
	}
}

func TestSupplyDemandRetestReportSafeFailure(t *testing.T) {
	for _, err := range []error{nil, fmt.Errorf("private parser diagnostic"), (*engine.SupplyDemandRetestError)(nil), &engine.SupplyDemandRetestError{Code: "unknown-secret", Field: "private", BarIndex: -1}} {
		raw := supplyDemandRetestFailure(err)
		if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "unknown-secret") || !strings.Contains(string(raw), `"internal-error"`) {
			t.Fatalf("unsafe failure %s", raw)
		}
	}
}

func TestSupplyDemandRetestCostOutputZeroOmission(t *testing.T) {
	raw, source := retestReportRequest(t)
	var request map[string]any
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		t.Fatal(err)
	}
	for _, bps := range []float64{0, 1.25} {
		request["costs"].(map[string]any)["slippageBps"] = bps
		input, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		encoded := RunSupplyDemandRetestReport(string(input), source)
		var result map[string]any
		if err = json.Unmarshal(encoded, &result); err != nil {
			t.Fatal(err)
		}
		if result["error"] != nil {
			t.Fatalf("unexpected refusal %s", encoded)
		}
		for _, field := range []string{"identity", "run"} {
			costs := result[field].(map[string]any)["costs"].(map[string]any)
			for _, key := range []string{"feePerUnit", "fillOn", "slippage", "startEquity"} {
				if _, ok := costs[key]; !ok {
					t.Fatalf("missing required %s.costs.%s", field, key)
				}
			}
			value, present := costs["slippageBps"]
			if present != (bps != 0) || (present && value != bps) {
				t.Fatalf("%s cost zero encoding: %#v", field, costs)
			}
			expected := 4
			if bps != 0 {
				expected = 5
			}
			if len(costs) != expected {
				t.Fatalf("unexpected cost shape: %#v", costs)
			}
		}
		sum := sha256.Sum256(input)
		if result["identity"].(map[string]any)["requestSha256"] != fmt.Sprintf("%x", sum) {
			t.Fatal("explicit original cost request not bound")
		}
	}
	delete(request["costs"].(map[string]any), "slippageBps")
	input, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var refused SupplyDemandRetestReport
	if err = json.Unmarshal(RunSupplyDemandRetestReport(string(input), source), &refused); err != nil || refused.Error == nil || refused.Error.Code != "invalid-input" || refused.Identity != nil || refused.Run != nil {
		t.Fatal("output omission must never become request fallback", err, refused)
	}
}
