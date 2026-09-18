package main

import (
	"bytes"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestFirstGridVariantErrorUsesCanonicalIndexOrder(t *testing.T) {
	variantErrors := make([]error, 4)
	for _, completed := range []struct {
		index int
		err   error
	}{
		{index: 3, err: errors.New("variant 3")},
		{index: 1, err: errors.New("variant 1")},
		{index: 2, err: errors.New("variant 2")},
	} {
		variantErrors[completed.index] = completed.err
	}
	if got := firstGridVariantError(variantErrors); got == nil || got.Error() != "variant 1" {
		t.Fatalf("firstGridVariantError() = %v, want variant 1", got)
	}

	onlyLater := []error{nil, nil, errors.New("variant 2"), nil}
	if got := firstGridVariantError(onlyLater); got == nil || got.Error() != "variant 2" {
		t.Fatalf("firstGridVariantError(later) = %v, want variant 2", got)
	}
	if got := firstGridVariantError(make([]error, 4)); got != nil {
		t.Fatalf("firstGridVariantError(success) = %v, want nil", got)
	}
}

func TestGridMultipleFailuresReturnExactLowestVariantWithoutOutput(t *testing.T) {
	dslFile, dataRoot, expected := writeConformanceCLIInputs(t, "money-risk-sizing")
	maxFloat := strconv.FormatFloat(math.MaxFloat64, 'g', -1, 64)
	var wantError string

	for iteration := 0; iteration < 20; iteration++ {
		var out bytes.Buffer
		err := run([]string{
			"grid",
			"--dsl-file=" + dslFile,
			"--symbol=" + expected.Symbol,
			"--tf=" + expected.Timeframe,
			"--range=" + expected.RangeMethod,
			"--set=riskUsd=100,200,300",
			"--slippage=" + maxFloat,
			"--json-only=1",
			"--data-root=" + dataRoot,
		}, &out)
		if err == nil {
			t.Fatal("grid succeeded, want checked overflow error")
		}
		if !strings.HasPrefix(err.Error(), `variant 0: cost mode "slip `) || !strings.Contains(err.Error(), "contains non-finite value") {
			t.Fatalf("grid error = %q, want canonical variant 0 checked error", err)
		}
		if iteration == 0 {
			wantError = err.Error()
		} else if err.Error() != wantError {
			t.Fatalf("iteration %d error = %q, want exact %q", iteration, err, wantError)
		}
		if out.Len() != 0 {
			t.Fatalf("iteration %d wrote %d bytes before error: %q", iteration, out.Len(), out.String())
		}
	}
}
