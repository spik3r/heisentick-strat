package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Key-level break/retest is a legacy pair of independent recent windows.
// These sequences make its causal but unordered behavior explicit until an
// opt-in ordered event is specified. All bars are closed when evaluated.
func TestBreakRetestLegacyWindowsDoNotRequireTouchAfterBreak(t *testing.T) {
	preBreakTouch := []marketdata.Bar{
		{T: 0, O: 99, H: 99.5, L: 98.5, C: 99},
		{T: 1, O: 100.2, H: 100.3, L: 99.5, C: 99.5}, // tag PDH=100, no break
		{T: 2, O: 99.4, H: 102, L: 99.2, C: 101},     // close-through break, no qualifying tag
	}
	postBreakTouch := []marketdata.Bar{
		{T: 0, O: 99, H: 99.5, L: 98.5, C: 99},
		{T: 1, O: 99.4, H: 102, L: 99.1, C: 101},    // break
		{T: 2, O: 101, H: 101.5, L: 99.5, C: 100.4}, // later retest
		{T: 3, O: 99.4, H: 102, L: 99.2, C: 101},    // bullish hold, no qualifying tag
	}
	sameBarTouch := []marketdata.Bar{
		{T: 0, O: 100.2, H: 100.3, L: 99.5, C: 99.5},
		{T: 1, O: 99.4, H: 102, L: 99.4, C: 101}, // break and tag in one OHLC bar
	}
	for _, tc := range []struct {
		name           string
		bars           []marketdata.Bar
		freshBreakBars int
		want           bool
	}{
		{"prior tag qualifies", preBreakTouch, 2, true},
		{"later retest qualifies", postBreakTouch, 3, true},
		{"same bar is unordered", sameBarTouch, 2, true},
		{"break expires despite later retest", postBreakTouch, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := len(tc.bars)
			atr, pdh := make([]float64, n), make([]float64, n)
			for j := range atr {
				atr[j], pdh[j] = 1, 100
			}
			b := broker{
				series: marketdata.SeriesFromBars(tc.bars),
				cols:   contextcols.Columns{ATR: atr, PriorDayH: pdh},
				params: flagParams{
					BRFreshBreakBars: tc.freshBreakBars, BRRetestBars: 2,
					BRLevelTolerance: 0.35, BRMinBreakATR: 1, BRTargetR: 1,
					StopBufferATR: 0.25, MinStopATR: 0.4, MaxStopATR: 3,
				},
			}
			setup, got := b.breakRetestSetup(n-1, sideLong)
			if got != tc.want {
				t.Fatalf("legacy setup admitted = %v, want %v", got, tc.want)
			}
			if got && (setup.Meta["levelKey"] != "PDH" || setup.Meta["levelPrice"] != float64(100)) {
				t.Fatalf("setup level = %v/%v, want PDH/100", setup.Meta["levelKey"], setup.Meta["levelPrice"])
			}
		})
	}
}
