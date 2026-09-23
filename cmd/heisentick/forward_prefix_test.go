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
	if !strings.Contains(out.String(), "forward-prefix") || !strings.Contains(out.String(), "complete extended bar prefix") || !strings.Contains(out.String(), "cannot be atomic together") {
		t.Fatalf("help omits forward-prefix boundary: %s", out.String())
	}
}

func writeORBPrefixBars(t *testing.T, dataRoot string, count int) string {
	t.Helper()
	fixture, err := engine.LoadRunFixture(filepath.Join(testsupport.StratConformanceRoot(), "run", "family-opening-range-breakout.fixture.json"))
	if err != nil {
		t.Fatalf("load ORB fixture: %v", err)
	}
	if count > len(fixture.Bars) {
		t.Fatalf("ORB fixture has %d bars, need %d", len(fixture.Bars), count)
	}
	symbolDir := filepath.Join(dataRoot, fixture.Symbol)
	if err := os.MkdirAll(symbolDir, 0o755); err != nil {
		t.Fatalf("mkdir ORB data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(symbolDir, fixture.Timeframe+".bin"), marketdata.EncodeBTB1(marketdata.SeriesFromBars(fixture.Bars[:count])), 0o644); err != nil {
		t.Fatalf("write ORB prefix: %v", err)
	}
	lastT := fixture.Bars[count-1].T
	htfCount := 0
	for htfCount < len(fixture.HTFBars) && fixture.HTFBars[htfCount].T <= lastT {
		htfCount++
	}
	if err := os.WriteFile(filepath.Join(symbolDir, fixture.HigherTimeframe+".bin"), marketdata.EncodeBTB1(marketdata.SeriesFromBars(fixture.HTFBars[:htfCount])), 0o644); err != nil {
		t.Fatalf("write ORB HTF prefix: %v", err)
	}
	return filepath.Join(testsupport.StratConformanceRoot(), "run", "family-opening-range-breakout.strat")
}

func runORBPrefixCLI(t *testing.T, dslFile, dataRoot string, checkpointArgs ...string) (engine.PrefixResult, error, string) {
	t.Helper()
	args := []string{
		"forward-prefix", "--dsl-file=" + dslFile, "--symbol=XAUUSD", "--tf=15m", "--range=zone",
		"--slippage=0.06", "--data-root=" + dataRoot,
	}
	args = append(args, checkpointArgs...)
	var out bytes.Buffer
	err := run(args, &out)
	var result engine.PrefixResult
	if err == nil {
		if decodeErr := json.Unmarshal(out.Bytes(), &result); decodeErr != nil {
			t.Fatalf("decode ORB prefix JSON: %v\n%s", decodeErr, out.String())
		}
	}
	return result, err, out.String()
}

func TestForwardPrefixCheckpointCLIResumesWithExactDeltaParity(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "data")
	dslFile := writeORBPrefixBars(t, dataRoot, 36)
	checkpoint1 := filepath.Join(t.TempDir(), "checkpoint-1.json")
	first, err, stdout := runORBPrefixCLI(t, dslFile, dataRoot, "--checkpoint-out="+checkpoint1)
	if err != nil {
		t.Fatalf("initial checkpoint CLI: %v", err)
	}
	if stdout == "" || len(first.Trades) != 0 || len(first.OpenPositions) != 1 {
		t.Fatalf("initial checkpoint result = %#v stdout=%q", first, stdout)
	}
	info, err := os.Stat(checkpoint1)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("checkpoint mode = %v / %v, want 0600", info, err)
	}

	dslFile = writeORBPrefixBars(t, dataRoot, 1800)
	checkpoint2 := filepath.Join(t.TempDir(), "checkpoint-2.json")
	resumed, err, _ := runORBPrefixCLI(t, dslFile, dataRoot, "--checkpoint-in="+checkpoint1, "--checkpoint-out="+checkpoint2)
	if err != nil {
		t.Fatalf("resume checkpoint CLI: %v", err)
	}
	full, err, _ := runORBPrefixCLI(t, dslFile, dataRoot)
	if err != nil {
		t.Fatalf("full replay CLI: %v", err)
	}
	combined := append(append([]engine.PrefixClosedTrade(nil), first.Trades...), resumed.Trades...)
	combinedJSON, _ := json.Marshal(combined)
	fullJSON, _ := json.Marshal(full.Trades)
	if string(combinedJSON) != string(fullJSON) {
		t.Fatalf("checkpoint deltas differ from full replay")
	}
	openJSON, _ := json.Marshal(resumed.OpenPositions)
	fullOpenJSON, _ := json.Marshal(full.OpenPositions)
	if string(openJSON) != string(fullOpenJSON) {
		t.Fatalf("checkpoint open state differs from full replay")
	}
}

func TestForwardPrefixCheckpointCLIRejectsUnsafePathsAndExistingCandidate(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "data")
	dslFile := writeORBPrefixBars(t, dataRoot, 36)
	existing := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(existing, []byte("prior"), 0o600); err != nil {
		t.Fatalf("write existing candidate: %v", err)
	}

	_, err, stdout := runORBPrefixCLI(t, dslFile, dataRoot, "--checkpoint-in="+existing, "--checkpoint-out="+existing)
	if err == nil || !strings.Contains(err.Error(), "must name different files") || stdout != "" {
		t.Fatalf("same-path result = %v stdout=%q", err, stdout)
	}
	_, err, stdout = runORBPrefixCLI(t, dslFile, dataRoot, "--checkpoint-out="+existing)
	if err == nil || !strings.Contains(err.Error(), "already exists") || stdout != "" {
		t.Fatalf("existing-output result = %v stdout=%q", err, stdout)
	}
	if got, _ := os.ReadFile(existing); string(got) != "prior" {
		t.Fatalf("existing checkpoint changed to %q", got)
	}
}

func TestForwardPrefixCheckpointCLIFailureDoesNotPublishCandidate(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "data")
	dslFile := writeORBPrefixBars(t, dataRoot, 36)
	malformed := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(malformed, []byte("{"), 0o600); err != nil {
		t.Fatalf("write malformed checkpoint: %v", err)
	}
	candidate := filepath.Join(t.TempDir(), "candidate.json")
	_, err, stdout := runORBPrefixCLI(t, dslFile, dataRoot, "--checkpoint-in="+malformed, "--checkpoint-out="+candidate)
	if err == nil || !strings.Contains(err.Error(), "malformed envelope") || stdout != "" {
		t.Fatalf("malformed checkpoint result = %v stdout=%q", err, stdout)
	}
	if _, statErr := os.Stat(candidate); !os.IsNotExist(statErr) {
		t.Fatalf("malformed checkpoint published candidate: %v", statErr)
	}

	invalidDSL := filepath.Join(t.TempDir(), "invalid.strat")
	if err := os.WriteFile(invalidDSL, []byte("not dsl"), 0o600); err != nil {
		t.Fatalf("write invalid DSL: %v", err)
	}
	var out bytes.Buffer
	err = run([]string{
		"forward-prefix", "--dsl-file=" + invalidDSL, "--symbol=XAUUSD", "--tf=15m", "--data-root=" + dataRoot,
		"--checkpoint-out=" + candidate,
	}, &out)
	if err == nil || out.Len() != 0 {
		t.Fatalf("invalid DSL result = %v stdout=%q", err, out.String())
	}
	if _, statErr := os.Stat(candidate); !os.IsNotExist(statErr) {
		t.Fatalf("parse failure published candidate: %v", statErr)
	}
}
