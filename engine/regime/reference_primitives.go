package regime

import "github.com/spik3r/heisentick-strat/marketdata"

// ReferenceIndicators exposes the already-qualified v9 aggregation and indicator
// arithmetic to separately named offline studies. It does not run a v9 policy,
// alter v9 configuration, or admit a caller to generic/live execution.
func ReferenceIndicators(s marketdata.Series, warmup, until int64) ([]IndicatorRow, int, error) {
	bars, used, err := aggregate(s, warmup, until)
	if err != nil {
		return nil, 0, err
	}
	rows, err := calculate(bars)
	return rows, used, err
}

// ReferenceBarIndicators applies the fixed HMA/Wilder/Supertrend recurrence to
// caller-aggregated reference bars, including H4 bars for the v10 study. Only
// ATR10, Supertrend and Regime are relevant for H4; this is not a readiness
// certificate for a broker calendar. It neither reads nor mutates v9 state.
func ReferenceBarIndicators(bars []NativeBar) ([]IndicatorRow, error) { return calculate(bars) }

// ReferenceBarrier resolves an already-live bracket with the qualified native
// OHLC chronology. The caller owns lifecycle, pending market exits and sizing.
func ReferenceBarrier(b NativeBar, sl, tp float64, d int, spread float64) (float64, string, int64) {
	return barrier(b, sl, tp, d, spread)
}
