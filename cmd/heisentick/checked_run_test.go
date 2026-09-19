package main

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestReportAndGridRejectCheckedOverflowBeforeOutput(t *testing.T) {
	maxFloat := strconv.FormatFloat(math.MaxFloat64, 'g', -1, 64)

	t.Run("report", func(t *testing.T) {
		dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "money-risk-sizing")
		var out bytes.Buffer
		err := run([]string{
			"report",
			"--dsl-file=" + dslFile,
			"--symbol=" + expected.Symbol,
			"--tf=" + expected.Timeframe,
			"--range=" + expected.RangeMethod,
			"--slippage=" + maxFloat,
			"--json-only=1",
			"--data-root=" + dataRoot,
		}, &out)
		assertCheckedCostError(t, err, "cost mode ")
		if out.Len() != 0 {
			t.Fatalf("report wrote %d bytes before checked error: %q", out.Len(), out.String())
		}
	})

	t.Run("one-variant grid", func(t *testing.T) {
		dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "money-risk-sizing")
		var out bytes.Buffer
		err := run([]string{
			"grid",
			"--dsl-file=" + dslFile,
			"--symbol=" + expected.Symbol,
			"--tf=" + expected.Timeframe,
			"--range=" + expected.RangeMethod,
			"--set=riskUsd=200",
			"--slippage=" + maxFloat,
			"--json-only=1",
			"--data-root=" + dataRoot,
		}, &out)
		assertCheckedCostError(t, err, "variant 0: cost mode ")
		if out.Len() != 0 {
			t.Fatalf("grid wrote %d bytes before checked error: %q", out.Len(), out.String())
		}
	})
}

func assertCheckedCostError(t *testing.T, err error, context string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected checked run error")
	}
	message := err.Error()
	if !strings.Contains(message, context) || !strings.Contains(message, "contains non-finite value") {
		t.Fatalf("checked error = %q, want context %q and non-finite detail", message, context)
	}
}
