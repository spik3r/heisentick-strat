package main

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"
)

// The M2 statistics are opt-in on the CLI so the established payload (and
// its goldens) stay byte-identical.
func TestReportEvidenceFlagGatesStatistics(t *testing.T) {
	dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "money-risk-sizing")
	base := []string{
		"report",
		"--dsl-file=" + dslFile,
		"--symbol=" + expected.Symbol,
		"--tf=" + expected.Timeframe,
		"--range=" + expected.RangeMethod,
		"--json-only=1",
		"--data-root=" + dataRoot,
	}
	holdoutFrom := int64(expected.Trades[0].EntryT)

	t.Run("default omits the statistics", func(t *testing.T) {
		var out bytes.Buffer
		if err := run(append(append([]string{}, base...), "--holdout-from="+strconv.FormatInt(holdoutFrom, 10)), &out); err != nil {
			t.Fatalf("run report: %v", err)
		}
		var raw map[string]any
		if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
			t.Fatalf("decode report JSON: %v", err)
		}
		for _, key := range []string{"headline", "groupings", "dateBounds", "holdout"} {
			if _, exists := raw[key]; exists {
				t.Fatalf("default report JSON must omit %s", key)
			}
		}
	})

	t.Run("evidence emits them", func(t *testing.T) {
		var out bytes.Buffer
		if err := run(append(append([]string{}, base...), "--evidence=1", "--holdout-from="+strconv.FormatInt(holdoutFrom, 10)), &out); err != nil {
			t.Fatalf("run report: %v", err)
		}
		var raw map[string]any
		if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
			t.Fatalf("decode report JSON: %v", err)
		}
		for _, key := range []string{"headline", "groupings", "dateBounds", "holdout"} {
			if _, exists := raw[key]; !exists {
				t.Fatalf("--evidence=1 report JSON lacks %s", key)
			}
		}
		headline := raw["headline"].(map[string]any)
		if headline["trades"] != float64(expected.TradeCount) {
			t.Fatalf("headline trades = %v, want %d", headline["trades"], expected.TradeCount)
		}
		holdout := raw["holdout"].(map[string]any)
		if holdout["fromT"] != float64(holdoutFrom) || holdout["inSample"].(map[string]any)["trades"] != float64(0) {
			t.Fatalf("holdout = %v, want every trade on the holdout side of the first entry", holdout)
		}
	})

	t.Run("bad holdout flag", func(t *testing.T) {
		var out bytes.Buffer
		if err := run(append(append([]string{}, base...), "--holdout-from=2025-01-01"), &out); err == nil {
			t.Fatal("expected an error for a non-integer --holdout-from")
		}
	})
}
