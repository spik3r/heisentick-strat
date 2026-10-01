package contextcols

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestComputeRelativeMeasuredVolatilityUsesInclusiveTrailingRange(t *testing.T) {
	series := marketdata.NewSeries(4)
	for i, width := range []float64{1, 2, 3, 2} {
		series.C[i] = 10
		series.H[i] = 10 + width/2
		series.L[i] = 10 - width/2
	}

	values := ComputeRelativeMeasuredVolatility(series, 1, 3)
	if !math.IsNaN(values[0]) || !math.IsNaN(values[1]) {
		t.Fatalf("warmup values = %v, want unavailable", values[:2])
	}
	if values[2] != 100 || values[3] != 0 {
		t.Fatalf("RMV values = %v, want [100 0] after warmup", values[2:])
	}
}

func TestComputeRelativeMeasuredVolatilityRequiresFullATRSeedAndLookback(t *testing.T) {
	series := marketdata.NewSeries(5)
	for i, width := range []float64{1, 2, 3, 4, 5} {
		series.C[i] = 10
		series.H[i] = 10 + width/2
		series.L[i] = 10 - width/2
	}

	values := ComputeRelativeMeasuredVolatility(series, 2, 3)
	if !math.IsNaN(values[0]) || !math.IsNaN(values[1]) || !math.IsNaN(values[2]) {
		t.Fatalf("RMV should be unavailable before index 3: %v", values)
	}
	if values[3] != 100 || values[4] != 100 {
		t.Fatalf("RMV[3:] = %v, want [100 100]", values[3:])
	}
}

func TestComputeRelativeMeasuredVolatilityFlatWindowIsUnavailable(t *testing.T) {
	series := marketdata.NewSeries(4)
	for i := range series.C {
		series.C[i] = 10
		series.H[i] = 10.5
		series.L[i] = 9.5
	}

	values := ComputeRelativeMeasuredVolatility(series, 1, 3)
	if !math.IsNaN(values[2]) || !math.IsNaN(values[3]) {
		t.Fatalf("flat RMV window = %v, want unavailable", values[2:])
	}
}

func TestRMVOversizedWindowsRemainUnavailable(t *testing.T) {
	series := marketdata.Series{T: []float64{1}, H: []float64{2}, L: []float64{1}, C: []float64{1.5}}
	for _, value := range ComputeRelativeMeasuredVolatility(series, math.MaxInt-1, math.MaxInt-1) {
		if !math.IsNaN(value) {
			t.Fatal("oversized windows must be unavailable")
		}
	}
}
