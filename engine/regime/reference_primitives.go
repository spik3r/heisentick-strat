package regime

import (
	"fmt"
	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
	"github.com/spik3r/heisentick-strat/marketdata"
	"math"
)

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

// ReferenceIndicatorsPortableV1 is the explicit separated-binary64 opt-in.
func ReferenceIndicatorsPortableV1(s marketdata.Series, warmup, until int64) (rows []IndicatorRow, used int, err error) {
	defer func() {
		if err != nil {
			rows = nil
			used = 0
		}
	}()
	defer fp.Recover(&err)
	bars, used, err := aggregateArithmetic(s, warmup, until, true)
	if err != nil {
		return nil, 0, err
	}
	rows, err = calculateArithmetic(bars, true)
	return rows, used, err
}
func ReferenceBarIndicatorsPortableV1(bars []NativeBar) (rows []IndicatorRow, err error) {
	defer func() {
		if err != nil {
			rows = nil
		}
	}()
	defer fp.Recover(&err)
	return calculateArithmetic(bars, true)
}

// ReferenceBarrierPortableV1 checks only operations reached by the existing
// gap/path chronology. It neither pre-evaluates a skipped branch nor changes
// the legacy barrier used by v9 and the default Master runner.
func ReferenceBarrierPortableV1(b NativeBar, sl, tp float64, d int, spread float64) (price float64, reason string, when int64, err error) {
	defer func() {
		if err != nil {
			price = 0
			reason = ""
			when = 0
		}
	}()
	defer fp.Recover(&err)
	if d != 1 && d != -1 {
		return 0, "", 0, fmt.Errorf("portable barrier direction must be +1 or -1")
	}
	o, h, l := b.Open, b.High, b.Low
	if d == -1 {
		o = fp.Add(o, spread)
		h = fp.Add(h, spread)
		l = fp.Add(l, spread)
	}
	if fp.Mul(float64(d), fp.Sub(o, sl)) <= 0 {
		return o, "stop_gap", b.FirstObservedT, nil
	}
	if fp.Mul(float64(d), fp.Sub(o, tp)) >= 0 {
		return o, "target_gap", b.FirstObservedT, nil
	}
	fp.Check(l, "barrier low comparison")
	fp.Check(h, "barrier high comparison")
	stop, target := l <= sl, h >= tp
	if d == -1 {
		stop, target = h >= sl, l <= tp
	}
	if stop && target {
		firstHigh := math.Abs(fp.Sub(o, h)) < math.Abs(fp.Sub(o, l))
		targetFirst := firstHigh
		if d == -1 {
			targetFirst = !firstHigh
		}
		if targetFirst {
			return tp, "target_ambiguous", b.CloseT, nil
		}
		return sl, "stop_ambiguous", b.CloseT, nil
	}
	if stop {
		return sl, "stop", b.CloseT, nil
	}
	if target {
		return tp, "target", b.CloseT, nil
	}
	return 0, "", 0, nil
}
