package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
)

func paperBridgeRequest(t *testing.T) engine.PaperPrefixRequest {
	t.Helper()
	source, err := os.ReadFile("../../conformance/parse/setup-opening-range-breakout.strat")
	if err != nil {
		t.Fatal(err)
	}
	return engine.PaperPrefixRequest{
		Schema: engine.PaperPrefixRequestSchema, Source: string(source),
		Route:           engine.PaperRoute{StrategyID: "authored-orb", Symbol: "XAUUSD", Timeframe: "5m", RangeMethod: "zone"},
		Units:           engine.PaperUnits{QuoteCurrency: "USD", PriceUnit: "USD/unit", QuantityUnit: "unit", Multiplier: 1},
		Costs:           engine.PaperCosts{StartEquity: 5000, FeePerUnit: .125, FillOn: "close", Slippage: .25},
		ActivationFromT: 1735689600000, ClosedThroughT: 1735690800000,
		Bars: [][]float64{
			{1735689600000, 100, 100.5, 99.5, 100, 1},
			{1735689900000, 100, 100.5, 99.5, 100, 1},
			{1735690200000, 100, 100.5, 99.5, 100, 1},
			{1735690500000, 100, 101.5, 99.75, 101, 1},
		},
	}
}

func TestPaperPrefixBridgeExactProducerDocument(t *testing.T) {
	request := paperBridgeRequest(t)
	for _, fill := range []string{"close", "nextOpen"} {
		request.Costs.FillOn = fill
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		result, err := engine.RunPaperPrefix(request)
		if err != nil {
			t.Fatal(err)
		}
		want, err := engine.EncodePaperPrefixResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if got := runPaperPrefixJSON(string(raw)); got != string(want) {
			t.Fatal("bridge changed producer JSON")
		}
		request.Checkpoint = result.Checkpoint
		raw, _ = json.Marshal(request)
		retry, err := engine.RunPaperPrefix(request)
		if err != nil {
			t.Fatal(err)
		}
		want, _ = engine.EncodePaperPrefixResult(retry)
		if got := runPaperPrefixJSON(string(raw)); got != string(want) || len(retry.Events) != 0 {
			t.Fatal("bridge changed checkpoint retry")
		}
		request.Checkpoint = nil
	}
}

func TestPaperPrefixBridgeStrictAndBoundedInput(t *testing.T) {
	raw, _ := json.Marshal(paperBridgeRequest(t))
	want := runPaperPrefixJSON(string(raw))
	for _, bad := range []string{
		"", "null", "[]", "{}", string(raw) + " {}",
		strings.Replace(string(raw), `"schema":`, `"unknown":0,"schema":`, 1),
		strings.Replace(string(raw), `"schema":`, `"schema":"duplicate","schema":`, 1),
		strings.Replace(string(raw), `"source":"`, `"source":"\ud800`, 1),
		strings.Replace(string(raw), `"feePerUnit":0.125`, `"feePerUnit":null`, 1),
		string([]byte{0xff}),
	} {
		if got := runPaperPrefixJSON(bad); got != paperPrefixFailure("PAPER_PREFIX_INPUT") {
			t.Fatalf("wrong fixed input refusal: %s", got)
		}
		if runPaperPrefixJSON(string(raw)) != want {
			t.Fatal("malformed request changed later invocation")
		}
	}
	exact := string(raw) + strings.Repeat(" ", paperPrefixWASMMaxRequestBytes-len(raw))
	if runPaperPrefixJSON(exact) != want {
		t.Fatal("exact request byte budget refused")
	}
	for _, oversized := range []string{exact + " ", strings.Repeat("Ω", paperPrefixWASMMaxRequestBytes/2+1)} {
		if got := runPaperPrefixJSON(oversized); got != paperPrefixFailure("PAPER_PREFIX_INPUT_LIMIT") {
			t.Fatal("UTF-8 byte budget not enforced")
		}
	}
}

func TestPaperPrefixBridgeSafeTypedErrors(t *testing.T) {
	for _, err := range []error{
		errors.New("private source /private/path secret"),
		&engine.PaperPrefixError{Code: `PAPER_PREFIX_INPUT","secret":"private`},
		(*engine.PaperPrefixError)(nil), nil,
	} {
		if got := paperPrefixError(err); got != paperPrefixFailure("PAPER_PREFIX_INTERNAL") {
			t.Fatalf("unsafe error escaped: %s", got)
		}
	}
	for _, code := range []string{"PAPER_PREFIX_CHECKPOINT", "PAPER_PREFIX_COSTS", "PAPER_PREFIX_NONFINITE", "PAPER_PREFIX_OUTPUT_LIMIT"} {
		if got := paperPrefixError(fmt.Errorf("private wrapper: %w", &engine.PaperPrefixError{Code: code})); got != paperPrefixFailure(code) {
			t.Fatal("lost fixed typed producer code")
		}
	}
}
