package engine

import (
	"math"
	"testing"
)

// Absolute and percentage drawdown are independent maxima. In this invented
// curve the first fall is 20%, while the later larger cash fall is only 15%.
func TestInteractiveDrawdownIndependentMaxima(t *testing.T) {
	abs, pct := runningPeakDrawdown([]float64{100, 80, 200, 170}, 100, 170)
	if abs != 30 || pct != 20 {
		t.Fatalf("drawdown = %v/%v; want 30 cash and 20 percent", abs, pct)
	}
}

func TestInteractiveDrawdownTerminalAndNonpositiveMarks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		curve    []float64
		terminal float64
		cash     float64
		percent  float64
	}{
		{"terminal liquidation", []float64{100, 120, 110}, 90, 30, 25},
		{"zero equity", []float64{100, 0}, 0, 100, 100},
		{"negative equity", []float64{100, -50}, -50, 150, 150},
		{"recover after negative", []float64{100, -50, 200, 180}, 180, 150, 150},
		{"empty gain", nil, 125, 0, 0},
		{"empty loss", nil, 75, 25, 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cash, percent := runningPeakDrawdown(tc.curve, 100, tc.terminal)
			if math.Abs(cash-tc.cash) > 1e-12 || math.Abs(percent-tc.percent) > 1e-12 {
				t.Fatalf("drawdown = %v/%v, want %v/%v", cash, percent, tc.cash, tc.percent)
			}
		})
	}
}

func TestInteractiveDrawdownMarkedAndCashCurvesStaySeparate(t *testing.T) {
	markedCash, markedPct := runningPeakDrawdown([]float64{100, 80, 100}, 100, 100)
	closedCash, closedPct := runningPeakDrawdown([]float64{100, 100, 100}, 100, 100)
	if markedCash != 20 || markedPct != 20 || closedCash != 0 || closedPct != 0 {
		t.Fatalf("marked %v/%v and closed %v/%v were conflated", markedCash, markedPct, closedCash, closedPct)
	}
}
