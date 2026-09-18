package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportGoldenOutput(t *testing.T) {
	dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "money-risk-sizing")
	var out bytes.Buffer
	if err := run([]string{
		"report",
		"--dsl-file=" + dslFile,
		"--dsl-id=" + expected.StrategyID,
		"--dsl-name=Golden Risk Sizing",
		"--symbol=" + expected.Symbol,
		"--tf=" + expected.Timeframe,
		"--range=" + expected.RangeMethod,
		"--include-trades=1",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out); err != nil {
		t.Fatalf("run report: %v", err)
	}
	assertNoTemporaryInputPaths(t, out.String(), dslFile, dataRoot)
	assertGoldenFile(t, filepath.Join("goldens", "report.json"), normalizeReportGeneratedAt(t, out.Bytes()))
}

func TestGridGoldenOutput(t *testing.T) {
	dslFile, dataRoot := writeCLIInputs(t)
	var out bytes.Buffer
	if err := run([]string{
		"grid",
		"--dsl-file=" + dslFile,
		"--symbol=XAUUSD",
		"--tf=5m",
		"--range=pivot",
		"--set=riskUsd=100,200",
		"--json-only=1",
		"--data-root=" + dataRoot,
	}, &out); err != nil {
		t.Fatalf("run grid: %v", err)
	}
	assertNoTemporaryInputPaths(t, out.String(), dslFile, dataRoot)
	assertGoldenFile(t, filepath.Join("goldens", "grid.json"), out.Bytes())
}

func normalizeReportGeneratedAt(t *testing.T, raw []byte) []byte {
	t.Helper()
	var payload struct {
		GeneratedAt      string `json:"generatedAt"`
		EvidenceEnvelope struct {
			GeneratedAt string `json:"generatedAt"`
		} `json:"evidenceEnvelope"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode report JSON: %v", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.GeneratedAt); err != nil {
		t.Fatalf("report generatedAt %q is not RFC3339Nano: %v", payload.GeneratedAt, err)
	}
	if payload.EvidenceEnvelope.GeneratedAt != payload.GeneratedAt {
		t.Fatalf("report generatedAt %q differs from evidence envelope %q", payload.GeneratedAt, payload.EvidenceEnvelope.GeneratedAt)
	}
	needle := []byte(payload.GeneratedAt)
	if count := bytes.Count(raw, needle); count != 2 {
		t.Fatalf("report generatedAt occurs %d times, want exactly 2", count)
	}
	return bytes.ReplaceAll(raw, needle, []byte("<generatedAt>"))
}

func assertNoTemporaryInputPaths(t *testing.T, output, dslFile, dataRoot string) {
	t.Helper()
	for _, path := range []string{dslFile, dataRoot, filepath.Dir(dslFile)} {
		if strings.Contains(output, path) {
			t.Fatalf("CLI JSON leaked temporary input path %q", path)
		}
	}
}

func assertGoldenFile(t *testing.T, path string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\n\ngot:\n%s\nwant:\n%s", path, got, want)
	}
}
