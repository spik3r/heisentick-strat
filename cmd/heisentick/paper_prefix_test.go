package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func paperCLIRequest(t *testing.T) engine.PaperPrefixRequest {
	source, err := os.ReadFile(filepath.Join(testsupport.StratConformanceRoot(), "parse/setup-opening-range-breakout.strat"))
	if err != nil {
		t.Fatal(err)
	}
	return engine.PaperPrefixRequest{Schema: engine.PaperPrefixRequestSchema, Source: string(source), Route: engine.PaperRoute{StrategyID: "test", Symbol: "XAUUSD", Timeframe: "5m", RangeMethod: "zone"}, Units: engine.PaperUnits{QuoteCurrency: "USD", PriceUnit: "USD/unit", QuantityUnit: "unit", Multiplier: 1}, Costs: engine.PaperCosts{StartEquity: 12000, FeePerUnit: .1, FillOn: "close", Slippage: .2, SlippageBps: 1}, ActivationFromT: 1735689600000, ClosedThroughT: 1735689900000, Bars: [][]float64{{1735689600000, 100, 101, 99, 100, 1}}}
}
func TestPaperCLIExactNativeEnvelope(t *testing.T) {
	request := paperCLIRequest(t)
	raw, _ := json.Marshal(request)
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"paper-prefix", "--request-file=" + path}, &out); err != nil {
		t.Fatal(err)
	}
	expected, err := engine.RunPaperPrefix(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := engine.EncodePaperPrefixResult(expected)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != string(encoded)+"\n" {
		t.Fatal("CLI output changed Go result")
	}
	request.Checkpoint = expected.Checkpoint
	raw, _ = json.Marshal(request)
	os.WriteFile(path, raw, 0600)
	out.Reset()
	if err := runPaperPrefix([]string{"--request-file=" + path}, &out); err != nil {
		t.Fatal(err)
	}
	var repeated engine.PaperPrefixResult
	if json.Unmarshal(out.Bytes(), &repeated) != nil || len(repeated.Events) != 0 {
		t.Fatal("checkpoint retry")
	}
}
func TestPaperCLIRefusesWithoutOutputOrPathLeaks(t *testing.T) {
	for _, args := range [][]string{nil, {"--request-file"}, {"--request-file="}, {"--request-file=a", "extra"}, {"--request-file=/private/no-such-secret-path"}} {
		var out bytes.Buffer
		err := runPaperPrefix(args, &out)
		if err == nil || out.Len() != 0 || strings.Contains(err.Error(), "secret") {
			t.Fatalf("bad args %v: %v", args, err)
		}
	}
	path := filepath.Join(t.TempDir(), "private.json")
	for _, raw := range []string{`{}`, `{"schema":null}`, `{"source":"\ud800"}`} {
		os.WriteFile(path, []byte(raw), 0600)
		var out bytes.Buffer
		err := runPaperPrefix([]string{"--request-file=" + path}, &out)
		if err == nil || out.Len() != 0 || strings.Contains(err.Error(), path) {
			t.Fatal("bad input success/leak")
		}
	}
}
