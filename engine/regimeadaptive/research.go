package regimeadaptive

import (
	"fmt"
	"math"

	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
)

// ResearchRows is an explicit Go-native flat-risk research bridge. Inspect remains
// synthetic-only; these rows have no Pine/execution qualification or inferred fills.
// Observed gaps are retained; indicators advance on observed bars, without padding.
func ResearchRows(bars []Bar, disableWick bool) (rows []Row, err error) {
	defer fp.Recover(&err)
	if len(bars) == 0 || len(bars) > 1000000 {
		return nil, fmt.Errorf("research bars outside bounds")
	}
	for n, b := range bars {
		if b.T < 0 || b.T > 9007199254740991 || b.CloseT > 9007199254740991 || b.CloseT-b.T != 1800000 || n > 0 && b.T < bars[n-1].CloseT {
			return nil, fmt.Errorf("invalid M30 time at%d", n)
		}
		if b.Low <= 0 || b.High < b.Low || b.Open < b.Low || b.Open > b.High || b.Close < b.Low || b.Close > b.High {
			return nil, fmt.Errorf("invalid OHLC at%d", n)
		}
		for _, v := range []float64{b.Open, b.High, b.Low, b.Close} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("nonfinite OHLC")
			}
			fp.Check(v, "research OHLC")
		}
	}
	rows = calculate(bars)
	for n := range rows {
		rows[n].Regime = classify(rows[n].CHOP, rows[n].RSI, rows[n].PSAR, bars[n].Close, true)
		if n > 0 {
			rows[n].Signals = signals(rows[n], &rows[n-1], bars[n], &bars[n-1], !disableWick, FlatProfile)
		}
	}
	return rows, nil
}
