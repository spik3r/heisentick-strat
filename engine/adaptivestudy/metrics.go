package adaptivestudy

import (
	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
	"math"
)

func metrics(r Result) Metrics {
	m := Metrics{TradeCount: len(r.Trades), NetReturn: fp.Sub(fp.Div(r.Terminal.Equity, 10000), 1)}
	peak := 10000.
	profit, loss := 0., 0.
	for _, p := range r.Equity {
		peak = math.Max(peak, p.Equity)
		if peak > 0 {
			m.MaxDrawdown = math.Max(m.MaxDrawdown, fp.Div(fp.Sub(peak, p.Equity), peak))
		}
	}
	for _, t := range r.Trades {
		m.NetR = fp.Add(m.NetR, t.R)
		if t.Net > 0 {
			m.Wins++
			profit = fp.Add(profit, t.Net)
		} else if t.Net < 0 {
			m.Losses++
			loss = fp.Sub(loss, t.Net)
		}
	}
	if loss > 0 {
		x := fp.Div(profit, loss)
		m.ProfitFactor = &x
	}
	return m
}

// Select is frozen expanding-training selection. Caller supplies only earlier-year
// independently run BASIC/none training results. No assessment result is admissible.
func Select(training map[string]Result, nextFoldStart int64) (string, error) {
	if len(training) != 3 || (nextFoldStart != date(2024, 1, 1) && nextFoldStart != date(2025, 1, 1) && nextFoldStart != date(2026, 1, 1)) {
		return "", errSelection("frozen candidate/fold set required")
	}
	base, ok := training["B0"]
	if !ok {
		return "", errSelection("B0 missing")
	}
	best := "B0"
	bestReturn := math.Inf(-1)
	for _, id := range []string{"B0", "B1", "B2"} {
		r, ok := training[id]
		if !ok || r.Config.Candidate != id || r.Config.Cost != "BASIC" || r.Config.Stress != "none" || r.Config.EndT != nextFoldStart || r.Config.StartT != date(2021, 4, 1) || r.DataHash != M30Hash || r.Profile != Profile || r.Schema != "adaptive-study-result-v1" || r.Status != "retrospective-assumed-model-not-Pine-parity" || math.IsNaN(r.Metrics.NetReturn) || math.IsInf(r.Metrics.NetReturn, 0) || math.IsNaN(r.Metrics.MaxDrawdown) || math.IsInf(r.Metrics.MaxDrawdown, 0) || r.Metrics.MaxDrawdown < 0 {
			return "", errSelection("invalid expanding training identity")
		}
		if r.Metrics.TradeCount < 30 || r.Metrics.MaxDrawdown > base.Metrics.MaxDrawdown+1e-12 {
			continue
		}
		if r.Metrics.NetReturn > bestReturn+1e-12 {
			best, bestReturn = id, r.Metrics.NetReturn
		}
	}
	return best, nil
}
