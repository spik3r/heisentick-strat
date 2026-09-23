package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func writePrefixBars(t *testing.T, dataRoot string, count int) {
	t.Helper()
	fixture, err := engine.LoadRunFixture(filepath.Join(testsupport.StratConformanceRoot(), "run", "money-risk-sizing.fixture.json"))
	if err != nil {
		t.Fatalf("load prefix fixture: %v", err)
	}
	if count > len(fixture.Bars) {
		t.Fatalf("prefix fixture has %d bars, need %d", len(fixture.Bars), count)
	}
	symbolDir := filepath.Join(dataRoot, fixture.Symbol)
	if err := os.MkdirAll(symbolDir, 0o755); err != nil {
		t.Fatalf("mkdir prefix data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(symbolDir, fixture.Timeframe+".bin"), marketdata.EncodeBTB1(marketdata.SeriesFromBars(fixture.Bars[:count])), 0o644); err != nil {
		t.Fatalf("write prefix data: %v", err)
	}
}

func runMoneyPrefixCLI(t *testing.T, dataRoot string) engine.PrefixResult {
	t.Helper()
	dslFile := filepath.Join(testsupport.StratConformanceRoot(), "run", "money-risk-sizing.strat")
	var out bytes.Buffer
	err := run([]string{
		"forward-prefix", "--dsl-file=" + dslFile, "--symbol=XAUUSD", "--tf=15m", "--range=zone",
		"--slippage=0.06", "--data-root=" + dataRoot,
	}, &out)
	if err != nil {
		t.Fatalf("run forward-prefix: %v", err)
	}
	var result engine.PrefixResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("decode forward-prefix JSON: %v\n%s", err, out.String())
	}
	return result
}

func TestForwardPrefixJSONPreservesOpenThenEmitsOneRealClose(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "data")
	writePrefixBars(t, dataRoot, 679)
	open := runMoneyPrefixCLI(t, dataRoot)
	if open.CheckpointDigest == "" || open.Trades == nil || len(open.Trades) != 0 || len(open.OpenPositions) != 1 {
		t.Fatalf("open CLI prefix = %#v, want digest, [] trades and one open position", open)
	}
	positionID := open.OpenPositions[0].PositionID

	writePrefixBars(t, dataRoot, 700)
	closed := runMoneyPrefixCLI(t, dataRoot)
	matches := 0
	for _, row := range closed.Trades {
		if row.PositionID == positionID {
			matches++
			if row.Trade.Reason == engine.ReasonEndOfTest {
				t.Fatalf("forward-prefix emitted synthetic end-of-test close: %#v", row)
			}
		}
	}
	if matches != 1 || len(closed.OpenPositions) != 0 {
		t.Fatalf("extended CLI prefix = %#v, want one real matching close and no open position", closed)
	}
}

func TestForwardPrefixRejectsUnsupportedSpecialFamily(t *testing.T) {
	dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "family-daily-flush-failure")
	var out bytes.Buffer
	err := run([]string{
		"forward-prefix", "--dsl-file=" + dslFile, "--symbol=" + expected.Symbol, "--tf=" + expected.Timeframe, "--data-root=" + dataRoot,
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "preserved-open prefix execution is unsupported for special family") {
		t.Fatalf("unsupported forward-prefix error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("unsupported forward-prefix wrote success output: %s", out.String())
	}
}

func TestHelpDocumentsForwardPrefixReplayBoundary(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"help"}, &out); err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(out.String(), "forward-prefix") || !strings.Contains(out.String(), "loaded data prefix") || !strings.Contains(out.String(), "does not support checkpoint resume") {
		t.Fatalf("help omits forward-prefix boundary: %s", out.String())
	}
}
