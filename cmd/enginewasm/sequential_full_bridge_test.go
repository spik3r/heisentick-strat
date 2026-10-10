package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	native "github.com/spik3r/heisentick-strat/engine"
)

func sequentialFullBridgeInput(t *testing.T) (string, string, [6][]float64) {
	t.Helper()
	source := `dsl v7
strategy "Synthetic Sequential" {}
market { sequential symbol SYNTH sequential timeframe 5m }
setup { type: sequential full sequential profile seq.full.public_approx.v1 sequential policy E1 }
risk { sequential riskUsd 100 sequential maxNotionalUsd 100000 }`
	rows := make([][]float64, 19)
	var columns [6][]float64
	for i := range rows {
		c := 100.0
		if i == 4 {
			c = 101
		} else if i >= 5 && i <= 13 {
			c = float64(104 - i)
		}
		rows[i] = []float64{float64(i * 300000), c, c + 1, c - 1, c, 1}
		if i >= 14 {
			rows[i][2], rows[i][3] = 100.5, 99.5
		}
		for j := range columns {
			columns[j] = append(columns[j], rows[i][j])
		}
	}
	raw, err := json.Marshal(native.RunFixture{Schema: "dsl-conformance-run-fixture-v1", Case: "synthetic-e1", StrategyID: "synthetic-e1", Symbol: "SYNTH", Timeframe: "5m", RangeMethod: "zone", Costs: native.Costs{FillOn: "open"}, RawBars: rows})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), source, columns
}

func TestSequentialFullBridgeMeaningfulTradeAndAudit(t *testing.T) {
	raw, source, columns := sequentialFullBridgeInput(t)
	resultRaw, err := runFixture(raw, source)
	if err != nil {
		t.Fatal(err)
	}
	var result native.RunResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatal(err)
	}
	if result.TradeCount != 1 || result.SequentialFull == nil || len(result.SequentialFull.Opportunities) != 1 {
		t.Fatalf("missing trade/audit: %s", resultRaw)
	}
	meta, _ := json.Marshal(columnRunMeta{Schema: "enginewasm-columnar-v1", Case: result.Case, StrategyID: result.StrategyID, Symbol: result.Symbol, Timeframe: result.Timeframe, RangeMethod: result.RangeMethod, Costs: result.Costs})
	packed, err := runColumns(string(meta), source, columns)
	if err != nil {
		t.Fatal(err)
	}
	var summary struct {
		TradeCount int                         `json:"tradeCount"`
		Audit      *native.SequentialFullAudit `json:"sequentialFull"`
	}
	if err := json.Unmarshal([]byte(packed.SummaryJSON), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.TradeCount != 1 || !reflect.DeepEqual(summary.Audit, result.SequentialFull) {
		t.Fatalf("column audit lost: %s", packed.SummaryJSON)
	}
	var text []columnTradeText
	if err := json.Unmarshal([]byte(packed.StringsJSON), &text); err != nil {
		t.Fatal(err)
	}
	if len(packed.Values) != columnTradeWidth || len(text) != 1 || !reflect.DeepEqual(reconstructColumnTrade(packed.Values, text[0]), result.Trades[0]) {
		t.Fatal("column trade differs")
	}
}

func TestSequentialFullBridgeTypedRefusalIdentity(t *testing.T) {
	raw, source, columns := sequentialFullBridgeInput(t)
	badSource := strings.Replace(source, "sequential policy E1", "sequential policy P1", 1)
	_, err := runFixture(raw, badSource)
	var cfgErr *dsl.SequentialFullConfigError
	if !errors.As(err, &cfgErr) || bridgeErrorEnvelope(err)["code"] != "unsupported_config" {
		t.Fatalf("parse type lost: %T %v", err, err)
	}
	var f native.RunFixture
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	f.RawBars = f.RawBars[:15]
	incomplete, _ := json.Marshal(f)
	_, err = runFixture(string(incomplete), source)
	var execErr *native.SequentialFullExecutionError
	if !errors.As(err, &execErr) || bridgeErrorEnvelope(err)["code"] != "unsupported-incomplete-terminal-run" {
		t.Fatalf("terminal type lost: %T %v", err, err)
	}
	for i := range columns {
		columns[i] = columns[i][:15]
	}
	meta, _ := json.Marshal(columnRunMeta{Schema: "enginewasm-columnar-v1", Case: "x", StrategyID: "x", Symbol: "SYNTH", Timeframe: "5m", Costs: native.Costs{FillOn: "open"}})
	_, err = runColumns(string(meta), source, columns)
	if !errors.As(err, &execErr) {
		t.Fatalf("column terminal type lost: %T %v", err, err)
	}
	failure := columnFailure(err).(map[string]any)
	if failure["ok"] != false || failure["code"] != "unsupported-incomplete-terminal-run" || failure["opportunityId"] == nil {
		t.Fatalf("column failure: %#v", failure)
	}
	ordinary := bridgeErrorEnvelope(errors.New("ordinary"))
	if !reflect.DeepEqual(ordinary, map[string]any{"error": "ordinary"}) {
		t.Fatalf("legacy error altered: %#v", ordinary)
	}
}

func TestSequentialFullBridgeRejectsLossyRawInputs(t *testing.T) {
	raw, source, columns := sequentialFullBridgeInput(t)
	for _, bad := range []string{
		`{"calendar":{},` + raw[1:],
		`{"source":123,` + raw[1:],
		`{"source":[],` + raw[1:],
		`{"source":{},` + raw[1:],
		`{"source":{"bars":{"kind":{"calendar":{}}},"htfBars":null},` + raw[1:],
		`{"source":{"bars":{"kind":"synthetic","barCount":1.00000000000000001},"htfBars":null},` + raw[1:],
		`{"timedCalendar":null,` + raw[1:],
		`{"contextOptions":{"atrLen":15},` + raw[1:],
		`{"contextOptions":null,` + raw[1:],
		`{"symbol":"OTHER",` + raw[1:],
		strings.Replace(raw, `"feePerUnit":0`, `"feePerUnit":null`, 1),
		strings.Replace(raw, `"feePerUnit":0`, `"feePerUnit":0,"spread":1`, 1),
		strings.Replace(raw, `"feePerUnit":0`, `"feePerUnit":0,"feePerUnit":1`, 1),
		strings.Replace(raw, `"bars":[[0,100,101,99,100,1]`, `"bars":[[0,null,101,99,100,1]`, 1),
		raw + ` {}`,
	} {
		_, err := runFixture(bad, source)
		var typed *native.SequentialFullExecutionError
		if !errors.As(err, &typed) {
			t.Fatalf("raw input not typed-rejected: %T %v: %.100s", err, err, bad)
		}
	}
	meta := `{"schema":"enginewasm-columnar-v1","symbol":"SYNTH","timeframe":"5m","costs":{"fillOn":"open"},"symbol":"SYNTH"}`
	if _, err := runColumns(meta, source, columns); err == nil {
		t.Fatal("duplicate metadata accepted")
	}
}
