package report

import "github.com/spik3r/heisentick-strat/engine"

// SequentialMarked is the marked view of a run: cash plus unrealized P&L of a
// position still open after the last bar, with its entry fee already paid.
// Headline net, returnPct and endEquity stay completed-trade (settled) values;
// these are the marked counterparts. For a flat run they are equal.
type SequentialMarked struct {
	EndEquity         float64 `json:"endEquity"`
	Net               float64 `json:"net"`
	ReturnPct         float64 `json:"returnPct"`
	OpenUnrealizedPnL float64 `json:"openUnrealizedPnl"`
	OpenEntryFeePaid  float64 `json:"openEntryFeePaid"`
}

// SequentialMarketMetrics is SequentialMetrics plus the marked block. The
// equity points and the two drawdown fields are always the close-bar marked
// curve (sequential-broker-close-mtm-v1); every other headline statistic counts
// completed trades only.
type SequentialMarketMetrics struct {
	SequentialMetrics
	Marked SequentialMarked `json:"marked"`
}

// ProjectSequentialMarketMetrics projects a run that may end with an open
// position. A flat run is projected by ProjectSequentialMetrics unchanged. For
// an open run the completed-trade statistics come from that same reducer, applied
// to the accounting with the open trade's entry event and mark effects removed;
// the marked curve, drawdown and marked block are then taken from the real marks.
func ProjectSequentialMarketMetrics(run engine.RunResult, a engine.SequentialAccounting) (SequentialMarketMetrics, error) {
	trades := len(a.Trades)
	switch {
	case len(a.Events) == 2*trades:
		m, err := ProjectSequentialMetrics(run, a)
		if err != nil {
			return SequentialMarketMetrics{}, err
		}
		return SequentialMarketMetrics{m, SequentialMarked{EndEquity: m.Headline.EndEquity, Net: m.Headline.Net, ReturnPct: m.Headline.ReturnPct}}, nil
	case len(a.Events) != 2*trades+1 || len(a.Marks) == 0:
		return SequentialMarketMetrics{}, sequentialMetricsError("accounting counts", -1)
	}
	entry := a.Events[2*trades]
	last := a.Marks[len(a.Marks)-1]
	if entry.Kind != "entry" || entry.TradeIndex != trades || entry.Index < 0 || entry.Index >= len(a.Marks) ||
		(trades > 0 && entry.Index < a.Trades[trades-1].ExitIndex) {
		return SequentialMarketMetrics{}, sequentialMetricsError("open entry event", 2*trades)
	}
	realizedCompleted := 0.0
	if trades > 0 {
		realizedCompleted = a.Events[2*trades-1].RealizedAfter
	}
	// The real marks must agree with the observed event chain before any view
	// of them is built.
	completed := a
	completed.Events = a.Events[:2*trades]
	completed.Marks = make([]engine.SequentialEquityMark, len(a.Marks))
	copy(completed.Marks, a.Marks)
	equities := make([]float64, len(a.Marks))
	for i, mark := range a.Marks {
		cash := float64(a.StartEquity + mark.Realized)
		equity := float64(cash + mark.Unrealized)
		if !sequentialMetricFinite(mark.Realized) || !sequentialMetricFinite(mark.Unrealized) || !sequentialMetricFinite(mark.Equity) || !sequentialMetricIdentical(mark.Equity, equity) {
			return SequentialMarketMetrics{}, sequentialMetricsError("raw equity mark", i)
		}
		equities[i] = mark.Equity
		if i >= entry.Index {
			if !sequentialMetricIdentical(mark.Realized, entry.RealizedAfter) {
				return SequentialMarketMetrics{}, sequentialMetricsError("open mark realized", i)
			}
			settled := float64(a.StartEquity + realizedCompleted)
			completed.Marks[i].Realized, completed.Marks[i].Unrealized, completed.Marks[i].Equity = realizedCompleted, 0, float64(settled+0)
		}
	}
	completed.FinalRealized = realizedCompleted
	completed.EndEquity = float64(a.StartEquity + realizedCompleted)
	m, err := ProjectSequentialMetrics(run, completed)
	if err != nil {
		return SequentialMarketMetrics{}, err
	}
	dd, ddPct, err := sequentialMarkedDrawdown(a.StartEquity, equities)
	if err != nil {
		return SequentialMarketMetrics{}, err
	}
	m.Headline.MaxDD, m.Headline.MaxDDpct = dd, ddPct
	m.Equity = m.Equity[:0]
	for _, mark := range a.Marks {
		m.Equity = append(m.Equity, SequentialEquityPoint{Index: mark.Index, T: mark.T, Equity: mark.Equity})
	}
	markedNet := float64(last.Realized + last.Unrealized)
	marked := SequentialMarked{EndEquity: last.Equity, Net: markedNet, ReturnPct: (markedNet / a.StartEquity) * 100, OpenUnrealizedPnL: last.Unrealized, OpenEntryFeePaid: entry.Amount}
	for _, v := range []float64{marked.EndEquity, marked.Net, marked.ReturnPct, marked.OpenUnrealizedPnL, marked.OpenEntryFeePaid} {
		if !sequentialMetricFinite(v) {
			return SequentialMarketMetrics{}, sequentialMetricsError("marked", -1)
		}
	}
	marked.EndEquity, marked.Net, marked.ReturnPct = sequentialOutputNumber(marked.EndEquity), sequentialOutputNumber(marked.Net), sequentialOutputNumber(marked.ReturnPct)
	marked.OpenUnrealizedPnL, marked.OpenEntryFeePaid = sequentialOutputNumber(marked.OpenUnrealizedPnL), sequentialOutputNumber(marked.OpenEntryFeePaid)
	normalizeSequentialOutputZeros(&m)
	return SequentialMarketMetrics{m, marked}, nil
}

// sequentialMarkedDrawdown is the same definition ProjectSequentialMetrics uses:
// the peak starts at starting equity, each mark updates it before drawdown is
// read, and the currency and percentage maxima are independent. It exists only
// so the marked curve of an open run can be reduced after the completed view.
// A test pins it bit for bit to ProjectSequentialMetrics on flat runs.
func sequentialMarkedDrawdown(start float64, equities []float64) (float64, float64, error) {
	peak, maxDD, maxPct := start, 0.0, 0.0
	for i, equity := range equities {
		if equity > peak {
			peak = equity
		}
		dd := peak - equity
		pct := (dd / peak) * 100
		if !sequentialMetricFinite(dd) || !sequentialMetricFinite(pct) {
			return 0, 0, sequentialMetricsError("drawdown", i)
		}
		if dd > maxDD {
			maxDD = dd
		}
		if pct > maxPct {
			maxPct = pct
		}
	}
	return maxDD, maxPct, nil
}
