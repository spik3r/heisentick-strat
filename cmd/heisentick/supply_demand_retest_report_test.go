package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/report"
)

const retestCLIRequest = `{"schema":"strat-supply-demand-retest-request-v1","case":"invented-cli","strategyId":"invented-retest","symbol":"XAUUSD","timeframe":"1h","sourceTimeframe":"4h","rangeMethod":"zone","higherTimeframe":"","sourceHtfBars":[],"costs":{"feePerUnit":0,"slippage":0,"slippageBps":0,"startEquity":10000,"fillOn":"close"},"bars":[[0,100,101,99,100,1],[3600000,100,101,99,100,1]],"sourceBars":[[0,100,101,99,100,1],[14400000,100,101,99,100,1]]}`
const retestCLISource = `dsl v7
strategy "Invented CLI" { description "Mechanical transport only." }
market conditions { slices(XAUUSD 1h) }
setup {
 type: supply demand
 source timeframe 4h
 retest on entry timeframe
}
filters { higher timeframe off }
entryTf 1h
`

func retestCLIFiles(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	request, source := filepath.Join(dir, "request.json"), filepath.Join(dir, "source.strat")
	if err := os.WriteFile(request, []byte(retestCLIRequest), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(retestCLISource), 0600); err != nil {
		t.Fatal(err)
	}
	return request, source
}
func TestSupplyDemandRetestCLICompleteParityAndDispatch(t *testing.T) {
	request, source := retestCLIFiles(t)
	var out bytes.Buffer
	err := run([]string{"supply-demand-retest-report", "--dsl-file=" + source, "--request-file=" + request}, &out)
	if err != nil {
		t.Fatal(err)
	}
	want := string(report.RunSupplyDemandRetestReport(retestCLIRequest, retestCLISource)) + "\n"
	if out.String() != want || strings.Contains(out.String(), `"error"`) {
		t.Fatalf("CLI did not emit exact shared result: %s", out.String())
	}
	if !strings.Contains(usageText(), "supply-demand-retest-report --request-file=") {
		t.Fatal("new command absent from help")
	}
}
func TestSupplyDemandRetestCLIRefusals(t *testing.T) {
	request, source := retestCLIFiles(t)
	for _, args := range [][]string{nil, {"--request-file=" + request}, {"--request-file=" + request, "--request-file=" + request}, {"--request-file=", "--dsl-file=" + source}, {"--request-file=" + request, "--unknown=" + source}, {"--request-file=" + t.TempDir(), "--dsl-file=" + source}, {"--request-file=" + request, "--dsl-file=" + source + "-missing"}} {
		var out bytes.Buffer
		if err := runSupplyDemandRetestReport(args, &out); err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 3 || fields["error"] == nil || !strings.HasSuffix(out.String(), "\n") {
			t.Fatalf("partial failure: %s", out.String())
		}
	}
	if err := os.WriteFile(source, bytes.Repeat([]byte("x"), (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runSupplyDemandRetestReport([]string{"--request-file=" + request, "--dsl-file=" + source}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"input-unavailable"`) {
		t.Fatal("oversized file admitted")
	}
}

type retestShortWriter struct{}

func (retestShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestSupplyDemandRetestCLIShortWrite(t *testing.T) {
	if err := runSupplyDemandRetestReport(nil, retestShortWriter{}); err != io.ErrShortWrite {
		t.Fatalf("short write = %v", err)
	}
}
