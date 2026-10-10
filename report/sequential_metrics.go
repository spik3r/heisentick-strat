package report

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/engine"
)

// SequentialMetricsBasis is deliberately separate from the existing report
// reducer: marks are post-execution original-bar closes, anchored at start equity.
const SequentialMetricsBasis = "sequential-broker-close-mtm-v1"

// SequentialMetrics is a pure projection of one captured Sequential execution.
// Its numbers use encoding/json's shortest round-trip binary64 representation;
// the companion RunResult retains its existing 15-significant-digit projection.
type SequentialMetrics struct {
	Basis           string                      `json:"basis"`
	Headline        SequentialHeadline          `json:"headline"`
	TradeAccounting []SequentialTradeAccounting `json:"tradeAccounting"`
	Equity          []SequentialEquityPoint     `json:"equity"`
}

type SequentialHeadline struct {
	StartEquity        float64  `json:"startEquity"`
	EndEquity          float64  `json:"endEquity"`
	Net                float64  `json:"net"`
	ReturnPct          float64  `json:"returnPct"`
	Trades             int      `json:"trades"`
	WinRate            *float64 `json:"winRate"`
	WinRateReason      string   `json:"winRateReason,omitempty"`
	ProfitFactor       *float64 `json:"profitFactor"`
	ProfitFactorReason string   `json:"profitFactorReason,omitempty"`
	Expectancy         *float64 `json:"expectancy"`
	ExpectancyReason   string   `json:"expectancyReason,omitempty"`
	AvgWin             *float64 `json:"avgWin"`
	AvgWinReason       string   `json:"avgWinReason,omitempty"`
	AvgLoss            *float64 `json:"avgLoss"`
	AvgLossReason      string   `json:"avgLossReason,omitempty"`
	WorstLoss          float64  `json:"worstLoss"`
	MaxDD              float64  `json:"maxDD"`
	MaxDDpct           float64  `json:"maxDDpct"`
	MaxWinStreak       int      `json:"maxWinStreak"`
	MaxLossStreak      int      `json:"maxLossStreak"`
	AvgHoldBars        *float64 `json:"avgHoldBars"`
	AvgHoldBarsReason  string   `json:"avgHoldBarsReason,omitempty"`
}

type SequentialTradeAccounting struct {
	Index    int     `json:"index"`
	EntryFee float64 `json:"entryFee"`
	ExitFee  float64 `json:"exitFee"`
	NetPnL   float64 `json:"netPnl"`
}

type SequentialEquityPoint struct {
	Index  int     `json:"index"`
	T      float64 `json:"t"`
	Equity float64 `json:"equity"`
}

// SequentialMetricsError refuses the complete projection, never a partial
// success or a serialization-time replacement of nonfinite values by zero.
type SequentialMetricsError struct {
	Kind  string `json:"kind"`
	Field string `json:"field"`
	Index int    `json:"index"`
}

func (e *SequentialMetricsError) Error() string {
	return fmt.Sprintf("%s: %s (index %d)", e.Kind, e.Field, e.Index)
}

func sequentialMetricsError(field string, index int) error {
	return &SequentialMetricsError{Kind: "invalid-sequential-metrics", Field: field, Index: index}
}

func sequentialMetricFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func sequentialMetricIdentical(a, b float64) bool { return math.Float64bits(a) == math.Float64bits(b) }

// ProjectSequentialMetrics does not run a strategy, call report.Build, or infer
// accounting from rounded trade.pnl. The raw account net is FinalRealized; the
// separately reduced trade nets are used only to validate the captured ledger.
func ProjectSequentialMetrics(run engine.RunResult, accounting engine.SequentialAccounting) (SequentialMetrics, error) {
	if err := validateSequentialAccounting(run, accounting); err != nil {
		return SequentialMetrics{}, err
	}
	metrics := SequentialMetrics{
		Basis: SequentialMetricsBasis,
		Headline: SequentialHeadline{
			StartEquity: accounting.StartEquity, EndEquity: accounting.EndEquity,
			Net: accounting.FinalRealized, Trades: len(accounting.Trades),
		},
		TradeAccounting: make([]SequentialTradeAccounting, 0, len(accounting.Trades)),
		Equity:          make([]SequentialEquityPoint, 0, len(accounting.Marks)),
	}
	h := &metrics.Headline
	h.ReturnPct = (h.Net / h.StartEquity) * 100
	peak := h.StartEquity
	for i, mark := range accounting.Marks {
		metrics.Equity = append(metrics.Equity, SequentialEquityPoint{Index: mark.Index, T: mark.T, Equity: mark.Equity})
		if mark.Equity > peak {
			peak = mark.Equity
		}
		dd := peak - mark.Equity
		ddPct := (dd / peak) * 100
		if !sequentialMetricFinite(dd) || !sequentialMetricFinite(ddPct) {
			return SequentialMetrics{}, sequentialMetricsError("drawdown", i)
		}
		// Each percentage uses its own contemporaneous peak. Its maximum need
		// not occur at the maximum currency drawdown.
		if dd > h.MaxDD {
			h.MaxDD = dd
		}
		if ddPct > h.MaxDDpct {
			h.MaxDDpct = ddPct
		}
	}
	var grossWin, grossLoss, holdBars float64
	wins, winStreak, lossStreak := 0, 0, 0
	for i, trade := range accounting.Trades {
		metrics.TradeAccounting = append(metrics.TradeAccounting, SequentialTradeAccounting{
			Index: i, EntryFee: trade.EntryFee, ExitFee: trade.ExitFee, NetPnL: trade.NetPnL,
		})
		holdBars += float64(trade.ExitIndex - trade.EntryIndex)
		if trade.NetPnL > 0 {
			grossWin += trade.NetPnL
			wins++
			winStreak++
			lossStreak = 0
			if winStreak > h.MaxWinStreak {
				h.MaxWinStreak = winStreak
			}
		} else {
			grossLoss -= trade.NetPnL
			lossStreak++
			winStreak = 0
			if lossStreak > h.MaxLossStreak {
				h.MaxLossStreak = lossStreak
			}
			if trade.NetPnL < h.WorstLoss {
				h.WorstLoss = trade.NetPnL
			}
		}
		if !sequentialMetricFinite(grossWin) || !sequentialMetricFinite(grossLoss) || !sequentialMetricFinite(holdBars) {
			return SequentialMetrics{}, sequentialMetricsError("trade reduction", i)
		}
	}
	if h.Trades == 0 {
		h.WinRateReason, h.ProfitFactorReason = "no-trades", "no-trades"
		h.ExpectancyReason, h.AvgHoldBarsReason = "no-trades", "no-trades"
		h.AvgWinReason, h.AvgLossReason = "no-winning-trades", "no-nonwinning-trades"
	} else {
		count := float64(h.Trades)
		h.WinRate = sequentialMetricPtr((float64(wins) / count) * 100)
		h.Expectancy = sequentialMetricPtr(h.Net / count)
		h.AvgHoldBars = sequentialMetricPtr(holdBars / count)
		if wins > 0 {
			h.AvgWin = sequentialMetricPtr(grossWin / float64(wins))
		} else {
			h.AvgWinReason = "no-winning-trades"
		}
		if nonWinners := h.Trades - wins; nonWinners > 0 {
			h.AvgLoss = sequentialMetricPtr(-grossLoss / float64(nonWinners))
		} else {
			h.AvgLossReason = "no-nonwinning-trades"
		}
		switch {
		case grossLoss > 0:
			h.ProfitFactor = sequentialMetricPtr(grossWin / grossLoss)
		case grossWin > 0:
			h.ProfitFactorReason = "no-losses"
		default:
			h.ProfitFactor = sequentialMetricPtr(0)
		}
	}
	if err := validateSequentialHeadline(*h); err != nil {
		return SequentialMetrics{}, err
	}
	normalizeSequentialOutputZeros(&metrics)
	return metrics, nil
}

func sequentialMetricPtr(x float64) *float64 { return &x }

// Normalize signed zero only on the companion output, after raw bitwise
// reconciliation and finite validation. Neither broker observations nor the
// retained RunResult are changed by this JSON-boundary convention.
func normalizeSequentialOutputZeros(m *SequentialMetrics) {
	h := &m.Headline
	values := []*float64{
		&h.StartEquity, &h.EndEquity, &h.Net, &h.ReturnPct, &h.WorstLoss, &h.MaxDD, &h.MaxDDpct,
		h.WinRate, h.ProfitFactor, h.Expectancy, h.AvgWin, h.AvgLoss, h.AvgHoldBars,
	}
	for _, value := range values {
		if value != nil {
			*value = sequentialOutputNumber(*value)
		}
	}
	for i := range m.TradeAccounting {
		t := &m.TradeAccounting[i]
		t.EntryFee = sequentialOutputNumber(t.EntryFee)
		t.ExitFee = sequentialOutputNumber(t.ExitFee)
		t.NetPnL = sequentialOutputNumber(t.NetPnL)
	}
	for i := range m.Equity {
		m.Equity[i].T = sequentialOutputNumber(m.Equity[i].T)
		m.Equity[i].Equity = sequentialOutputNumber(m.Equity[i].Equity)
	}
}

func sequentialOutputNumber(x float64) float64 {
	if x == 0 {
		return 0
	}
	return x
}

func validateSequentialHeadline(h SequentialHeadline) error {
	fields := []struct {
		name  string
		value float64
	}{
		{"startEquity", h.StartEquity}, {"endEquity", h.EndEquity}, {"net", h.Net},
		{"returnPct", h.ReturnPct}, {"worstLoss", h.WorstLoss}, {"maxDD", h.MaxDD}, {"maxDDpct", h.MaxDDpct},
	}
	for _, field := range fields {
		if !sequentialMetricFinite(field.value) {
			return sequentialMetricsError(field.name, -1)
		}
	}
	for _, field := range []struct {
		name  string
		value *float64
	}{
		{"winRate", h.WinRate}, {"profitFactor", h.ProfitFactor}, {"expectancy", h.Expectancy},
		{"avgWin", h.AvgWin}, {"avgLoss", h.AvgLoss}, {"avgHoldBars", h.AvgHoldBars},
	} {
		if field.value != nil && !sequentialMetricFinite(*field.value) {
			return sequentialMetricsError(field.name, -1)
		}
	}
	return nil
}

func validateSequentialAccounting(run engine.RunResult, a engine.SequentialAccounting) error {
	if !sequentialMetricFinite(a.StartEquity) || a.StartEquity <= 0 {
		return sequentialMetricsError("startEquity", -1)
	}
	if !sequentialMetricFinite(a.FinalRealized) || !sequentialMetricFinite(a.EndEquity) {
		return sequentialMetricsError("terminal accounting", -1)
	}
	end := a.StartEquity + a.FinalRealized
	if !sequentialMetricFinite(end) || !sequentialMetricIdentical(end, a.EndEquity) {
		return sequentialMetricsError("startEquity + finalRealized", -1)
	}
	if len(a.Marks) == 0 || run.TradeCount != len(run.Trades) || len(a.Trades) != len(run.Trades) || len(a.Events)/2 != len(a.Trades) || len(a.Events)%2 != 0 {
		return sequentialMetricsError("accounting counts", -1)
	}
	var realized, tradeSum, budget float64
	for i, trade := range a.Trades {
		if trade.TradeIndex != i || trade.EntryIndex < 0 || trade.ExitIndex < trade.EntryIndex || trade.ExitIndex >= len(a.Marks) ||
			run.Trades[i].EntryIndex != trade.EntryIndex || run.Trades[i].ExitIndex != trade.ExitIndex || run.Trades[i].Side != trade.Side ||
			(trade.Side != "long" && trade.Side != "short") {
			return sequentialMetricsError("trade identity", i)
		}
		for _, v := range []float64{trade.EntryT, trade.ExitT, trade.Entry, trade.Exit, trade.Size, trade.Points, trade.EntryFee, trade.ExitFee, trade.ExitCredit, trade.NetPnL} {
			if !sequentialMetricFinite(v) {
				return sequentialMetricsError("raw trade", i)
			}
		}
		if trade.Size < 0 || trade.EntryFee < 0 || trade.ExitFee < 0 || trade.EntryT != a.Marks[trade.EntryIndex].T || trade.ExitT != a.Marks[trade.ExitIndex].T {
			return sequentialMetricsError("raw trade bounds", i)
		}
		net := trade.ExitCredit - trade.EntryFee
		if !sequentialMetricFinite(net) || !sequentialMetricIdentical(net, trade.NetPnL) {
			return sequentialMetricsError("exitCredit - entryFee", i)
		}
		for j, kind := range []string{"entry", "exit"} {
			e := a.Events[2*i+j]
			index, time, amount := trade.EntryIndex, trade.EntryT, trade.EntryFee
			if j == 1 {
				index, time, amount = trade.ExitIndex, trade.ExitT, trade.ExitCredit
			}
			if e.Kind != kind || e.TradeIndex != i || e.Index != index || e.T != time || !sequentialMetricIdentical(e.Amount, amount) || !sequentialMetricIdentical(e.RealizedBefore, realized) ||
				!sequentialMetricFinite(e.RealizedBefore) || !sequentialMetricFinite(e.RealizedAfter) ||
				(2*i+j > 0 && e.Index < a.Events[2*i+j-1].Index) {
				return sequentialMetricsError("observed event chain", 2*i+j)
			}
			// An entry can fuse before - feePerUnit*size. Do not demand an
			// unfused replay. Bound nominal product rounding and the observed
			// mutation rounding, then check the alternate ledger reduction.
			if j == 0 {
				if err := addSequentialRoundingBound(&budget, trade.EntryFee); err != nil {
					return err
				}
			}
			if err := addSequentialRoundingBound(&budget, e.RealizedAfter); err != nil {
				return err
			}
			realized = e.RealizedAfter
		}
		tradeSum += trade.NetPnL
		if !sequentialMetricFinite(tradeSum) {
			return sequentialMetricsError("trade net sum", i)
		}
		for _, result := range []float64{net, tradeSum} {
			if err := addSequentialRoundingBound(&budget, result); err != nil {
				return err
			}
		}
	}
	if !sequentialMetricIdentical(realized, a.FinalRealized) {
		return sequentialMetricsError("terminal observed state", -1)
	}
	difference := math.Abs(tradeSum - a.FinalRealized)
	if !sequentialMetricFinite(difference) || difference > budget {
		return sequentialMetricsError("trade/account reconciliation", -1)
	}
	markRealized, eventIndex, open := 0.0, 0, false
	for i, mark := range a.Marks {
		if mark.Index != i || !sequentialMetricFinite(mark.T) || (i > 0 && mark.T <= a.Marks[i-1].T) ||
			!sequentialMetricFinite(mark.Realized) || !sequentialMetricFinite(mark.Unrealized) || !sequentialMetricFinite(mark.Equity) {
			return sequentialMetricsError("raw equity mark", i)
		}
		for eventIndex < len(a.Events) && a.Events[eventIndex].Index <= i {
			markRealized = a.Events[eventIndex].RealizedAfter
			open = a.Events[eventIndex].Kind == "entry"
			eventIndex++
		}
		cash := float64(a.StartEquity + mark.Realized)
		equity := cash + mark.Unrealized
		if !sequentialMetricIdentical(mark.Realized, markRealized) || !sequentialMetricFinite(cash) || !sequentialMetricFinite(equity) || !sequentialMetricIdentical(mark.Equity, equity) || (!open && mark.Unrealized != 0) {
			return sequentialMetricsError("equity mark reconciliation", i)
		}
	}
	last := a.Marks[len(a.Marks)-1]
	if last.Unrealized != 0 || !sequentialMetricIdentical(last.Realized, a.FinalRealized) || !sequentialMetricIdentical(last.Equity, a.EndEquity) {
		return sequentialMetricsError("terminal equity mark", len(a.Marks)-1)
	}
	return nil
}

// sequentialRoundingUnit is a conservative one-ULP absolute rounding bound at
// the actual operation result. Frexp avoids Nextafter(MaxFloat64,+Inf), whose
// infinite distance would silently disable reconciliation. Subnormal spacing
// is the only minimum; there is no relative-to-one or arbitrary epsilon floor.
func sequentialRoundingUnit(result float64) (float64, error) {
	if !sequentialMetricFinite(result) {
		return 0, sequentialMetricsError("rounding bound operand", -1)
	}
	_, exponent := math.Frexp(math.Abs(result))
	unit := math.Ldexp(1, exponent-53)
	if result == 0 || unit < math.SmallestNonzeroFloat64 {
		unit = math.SmallestNonzeroFloat64
	}
	return unit, nil
}

// One charge is made per nominal entry multiplication, observed account
// mutation, trade subtraction and trade-sum addition. Outward rounding at each
// addition keeps the accumulated budget an upper bound even on long histories.
// An unrepresentable budget is a refusal, never permission to accept anything.
func addSequentialRoundingBound(budget *float64, result float64) error {
	unit, err := sequentialRoundingUnit(result)
	if err != nil {
		return err
	}
	next := *budget + unit
	if !sequentialMetricFinite(next) || *budget < 0 || !sequentialMetricFinite(*budget) {
		return sequentialMetricsError("rounding budget", -1)
	}
	next = math.Nextafter(next, math.Inf(1))
	if !sequentialMetricFinite(next) {
		return sequentialMetricsError("rounding budget", -1)
	}
	*budget = next
	return nil
}
