package master

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/engine/regime"
	"math"
)

func finite(v float64) bool      { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func pointer(v float64) *float64 { return &v }

func buildIndicators(r Request) ([]IndicatorRow, []H4Row, int, error) {
	rows, used, err := regime.ReferenceIndicators(r.M5, r.WarmupFromT, r.TradeToT)
	if err != nil {
		return nil, nil, 0, err
	}
	bars := []NativeBar{}
	counts := []int{}
	for _, row := range rows {
		bt := row.BucketT / H4MS * H4MS
		// An unfinished H4 bucket may contribute to M30 context but is never an
		// available H4 observation, even at the end of a shorter synthetic prefix.
		if bt+H4MS > r.TradeToT {
			continue
		}
		if len(bars) == 0 || bars[len(bars)-1].BucketT != bt {
			bars = append(bars, NativeBar{BucketT: bt, FirstObservedT: row.FirstObservedT, CloseT: bt + H4MS, Open: row.Open, High: row.High, Low: row.Low})
			counts = append(counts, 0)
		}
		b := &bars[len(bars)-1]
		b.High = math.Max(b.High, row.High)
		b.Low = math.Min(b.Low, row.Low)
		b.Close = row.Close
		b.Volume += row.Volume
		b.Count += row.Count
		counts[len(counts)-1]++
		b.Complete = b.Count == 48
		if !finite(b.Volume) {
			return nil, nil, 0, fmt.Errorf("master H4 volume overflow")
		}
	}
	hr, err := regime.ReferenceBarIndicators(bars)
	if err != nil {
		return nil, nil, 0, err
	}
	h4 := make([]H4Row, len(hr))
	for i, row := range hr {
		h4[i] = H4Row{NativeBar: row.NativeBar, M30Count: counts[i], ATR10: row.ATR10, Supertrend: row.Supertrend, Regime: row.Regime}
	}
	out := make([]IndicatorRow, len(rows))
	historical, stable := -1, -1
	for i, row := range rows {
		for historical+1 < len(h4) && h4[historical+1].CloseT <= row.CloseT {
			historical++
		}
		for stable+1 < len(h4) && h4[stable+1].CloseT <= row.BucketT {
			stable++
		}
		o := IndicatorRow{IndicatorRow: row}
		if row.ATR14 != nil {
			v := 100 * *row.ATR14 / row.Close
			if !finite(v) {
				return nil, nil, 0, fmt.Errorf("master ATR percent overflow")
			}
			o.ATRPercent = pointer(v)
		}
		snap := func(k int) *H4Snapshot {
			if k < 0 {
				return nil
			}
			h := h4[k]
			return &H4Snapshot{AvailableT: h.CloseT, Regime: h.Regime, Supertrend: h.Supertrend}
		}
		o.HistoricalH4 = snap(historical)
		o.StableH4 = snap(stable)
		o.SourceCandidate = gatedCandidate(row.Candidate, o.ATRPercent, o.HistoricalH4)
		o.ProtectedCandidate = gatedCandidate(row.Candidate, o.ATRPercent, o.StableH4)
		out[i] = o
	}
	return out, h4, used, nil
}
func gatedCandidate(base int, pct *float64, h4 *H4Snapshot) int {
	if base != 0 && pct != nil && *pct >= .08 && h4 != nil && base == -h4.Regime {
		return base
	}
	return 0
}
