package engine

import (
	"math"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestTrendPullbackSessionVWAPFirstTouchIsCausal(t *testing.T) {
	start := float64(time.Date(2024, time.January, 1, 23, 0, 0, 0, time.UTC).UnixMilli()) // local Asia open
	baseBars := []marketdata.Bar{
		{T: start, O: 105, H: 106, L: 99, C: 99, V: 1},                                 // first touch, still below VWAP
		{T: start + float64(contextcols.HourMS), O: 106, H: 108, L: 104, C: 107, V: 1}, // reclaim without another touch
	}
	params := flagParams{
		UseAsiaWindow:          true,
		AllowLong:              true,
		TPBMinER:               0.35,
		TPBMaxER:               0.95,
		TPBPullbackWithin:      3,
		TPBMaxPullbackDepth:    2,
		TPBImpulseLookbackBars: 12,
		TPBTargetR:             2,
		TPBMinStopATR:          0.5,
		TPBMaxStopATR:          math.Inf(1),
		TPBStopBufferATR:       0.25,
		MaxMovementER:          1,
		TPBVWAPTouch:           "wick",
		TPBVWAPFirst:           true,
		TPBVWAPReclaim:         true,
	}
	run := func(bars []marketdata.Bar, runParams flagParams) broker {
		n := len(bars)
		atr := make([]float64, n)
		vwap := make([]float64, n)
		er := make([]float64, n)
		regime := make([]int8, n)
		ema := make([]float64, n)
		emaSlope := make([]float64, n)
		for i := range bars {
			atr[i] = 2
			vwap[i] = 100
			er[i] = 0.6
			regime[i] = regimeTrending
			ema[i] = 103
			emaSlope[i] = 1
		}
		b := broker{
			series:         marketdata.SeriesFromBars(bars),
			cols:           contextcols.Columns{ATR: atr, VWAP: vwap, ER: er, Regime: regime},
			ema:            ema,
			emaSlope:       emaSlope,
			params:         runParams,
			captureEntries: true,
		}
		for i := 0; i < n; i++ {
			b.onTrendPullbackBar(i)
		}
		return b
	}

	firstOnly := run(baseBars, params)
	if len(firstOnly.capturedEntries) != 1 {
		t.Fatalf("first-touch run captured %d entries, want 1", len(firstOnly.capturedEntries))
	}
	if got := firstOnly.capturedEntries[0].Meta["sessionVwapFirstTouchIdx"]; got != 0 {
		t.Fatalf("first touch index = %v, want 0", got)
	}

	secondTouch := append([]marketdata.Bar(nil), baseBars...)
	secondTouch[1].L = 99
	blocked := run(secondTouch, params)
	if len(blocked.capturedEntries) != 0 {
		t.Fatalf("second-touch run captured %d entries, want 0", len(blocked.capturedEntries))
	}

	t.Run("Mid touch resets the configured-window first-touch counter", func(t *testing.T) {
		midBars := []marketdata.Bar{
			baseBars[0], // Asia touch; it must not consume Mid's first touch.
			{T: start + 3*float64(contextcols.HourMS), O: 105, H: 106, L: 99, C: 99, V: 1},
			{T: start + 4*float64(contextcols.HourMS), O: 106, H: 108, L: 104, C: 107, V: 1},
		}
		midParams := params
		midParams.UseMidWindow = true
		mid := run(midBars, midParams)
		if len(mid.capturedEntries) != 1 {
			t.Fatalf("Mid first-touch run captured %d entries, want 1", len(mid.capturedEntries))
		}
		if got := mid.capturedEntries[0].Meta["sessionVwapFirstTouchIdx"]; got != 1 {
			t.Fatalf("Mid first touch index = %v, want 1", got)
		}
		if got := mid.tpbVWAPFirstTouch; got != 1 {
			t.Fatalf("stored Mid first touch index = %d, want 1", got)
		}
	})
}
