package engine

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// OrdinaryBacktestFinancials is a copied, unrounded financial observation from
// one ordinary broker lifecycle. It is preparation for report -> engine use,
// not an execution API, admission receipt, or proof of full accounting coverage.
// In particular, these hooks do not retain the required contemporaneous full
// position/order history. No public runner attaches this capture yet.
//
// Events and Marks contain values only and do not alias the broker or collector.
// Signed zero is retained here; normalization belongs to report serialization.
// A successful ordinary finalization is flat. Pending orders are not filled.
type OrdinaryBacktestFinancials struct {
	StartEquity        float64
	TerminalRealized   float64
	LastFlatRealized   float64
	TerminalUnrealized float64
	OpenEntryFee       float64
	EntryFees          float64
	ExitFees           float64
	Events             []OrdinaryBacktestFinancialEvent
	Marks              []OrdinaryBacktestFinancialMark
}

// OrdinaryBacktestFinancialEvent retains actual broker mutation observations.
// ExitCredit excludes the original entry fee and is zero on entry events.
// TradeIndex refers to the raw native exit row, or -1 for an entry. A partial
// does not complete a logical position. A later final Size is the actual broker
// remainder witness; it must not be reconstructed by subtracting rounded sizes.
type OrdinaryBacktestFinancialEvent struct {
	Kind, PositionID, Side        string
	Index, Ordinal, TradeIndex    int
	T, Price, Size, Fee           float64
	RealizedBefore, RealizedAfter float64
	ExitCredit                    float64
}

// OrdinaryBacktestFinancialMark is one original-close mark per input row. The
// final mark is freshly observed after ordinary liquidation, replacing the
// provisional last mark. No collector peak or drawdown is exported as authority.
type OrdinaryBacktestFinancialMark struct {
	Index                           int
	T, Realized, Unrealized, Equity float64
	EntryFees, ExitFees             float64
}

type ordinaryBacktestFinancialCapture struct {
	broker    *broker
	collector *paperCollector
	costs     Costs
	bars      []marketdata.Bar
	finished  bool
}

func ordinaryFinancialError(field string, index int) error {
	return fmt.Errorf("ordinary financial capture: %s at bar %d", field, index)
}

func ordinaryFinancialFinite(values ...float64) bool {
	for _, v := range values {
		if !isFinite(v) {
			return false
		}
	}
	return true
}

// attachOrdinaryBacktestFinancialCapture is private and must be called after
// reset and any SAR reattachment, before execution. It only attaches the
// existing raw hooks; it neither executes nor alters financial expressions.
// These checks are lifecycle guards, not the shared ordinary admission checks.
func attachOrdinaryBacktestFinancialCapture(b *broker) (*ordinaryBacktestFinancialCapture, error) {
	if b == nil || b.paperAccounting != nil || b.sequentialAccounting != nil || b.hasPosition ||
		!paperSame(b.realized, 0) || len(b.trades) != 0 || len(b.pendingOrders) != 0 ||
		len(b.pendingExits) != 0 || len(b.limitOrders) != 0 || b.windowed || b.captureEntries {
		return nil, ordinaryFinancialError("attachment lifecycle", -1)
	}
	costs := b.costs
	if !ordinaryFinancialFinite(costs.StartEquity, costs.FeePerUnit, costs.Slippage, costs.SlippageBps) ||
		costs.StartEquity <= 0 || costs.FeePerUnit < 0 || costs.Slippage < 0 || costs.SlippageBps < 0 ||
		(costs.FillOn != "close" && costs.FillOn != "open" && costs.FillOn != "nextOpen") {
		return nil, ordinaryFinancialError("costs", -1)
	}
	n := b.series.Len()
	if n < 1 || n > OrdinaryBacktestMaxBars || len(b.series.O) != n || len(b.series.H) != n ||
		len(b.series.L) != n || len(b.series.C) != n || len(b.series.V) != n {
		return nil, ordinaryFinancialError("bars", -1)
	}
	bars := make([]marketdata.Bar, n)
	for i := range bars {
		bar := marketdata.Bar{T: b.series.T[i], O: b.series.O[i], H: b.series.H[i], L: b.series.L[i], C: b.series.C[i], V: b.series.V[i]}
		if !ordinaryFinancialFinite(bar.T, bar.O, bar.H, bar.L, bar.C, bar.V) || bar.T < 0 ||
			bar.T > ordinaryBacktestMaxInteger || bar.T != math.Trunc(bar.T) || (i > 0 && bar.T <= bars[i-1].T) {
			return nil, ordinaryFinancialError("bars", i)
		}
		bars[i] = bar
	}
	c := newPaperCollector("ordinary-financial-v1", costs.StartEquity)
	x := &ordinaryBacktestFinancialCapture{broker: b, collector: c, costs: costs, bars: bars}
	b.paperAccounting = c
	return x, nil
}

// finish must follow the existing ordinary loop's final liquidation. It does
// not liquidate, replay prefixes, fill pending orders, or manufacture missing
// observations. A failure returns no financials. It is deliberately one-shot.
func (x *ordinaryBacktestFinancialCapture) finish() (OrdinaryBacktestFinancials, error) {
	if x == nil || x.finished {
		return OrdinaryBacktestFinancials{}, ordinaryFinancialError("finalization lifecycle", -1)
	}
	x.finished = true
	out, err := x.finalize()
	if err != nil {
		return OrdinaryBacktestFinancials{}, err
	}
	return out, nil
}

func (x *ordinaryBacktestFinancialCapture) finalize() (OrdinaryBacktestFinancials, error) {
	b, c := x.broker, x.collector
	n := len(x.bars)
	if b == nil || c == nil || b.paperAccounting != c || c.err != nil || n == 0 ||
		b.hasPosition || c.open != nil || b.windowed || b.captureEntries || len(c.events) < n || len(c.events) > PaperPrefixMaxEvents ||
		!ordinaryFinancialSameCosts(x.costs, b.costs) || b.series.Len() != n ||
		len(b.series.O) != n || len(b.series.H) != n || len(b.series.L) != n || len(b.series.C) != n || len(b.series.V) != n {
		return OrdinaryBacktestFinancials{}, ordinaryFinancialError("incomplete capture", -1)
	}
	for i, bar := range x.bars {
		if !paperSame(bar.T, b.series.T[i]) || !paperSame(bar.O, b.series.O[i]) || !paperSame(bar.H, b.series.H[i]) ||
			!paperSame(bar.L, b.series.L[i]) || !paperSame(bar.C, b.series.C[i]) || !paperSame(bar.V, b.series.V[i]) {
			return OrdinaryBacktestFinancials{}, ordinaryFinancialError("changed bars", i)
		}
	}
	if !ordinaryFinancialFinite(b.realized) || !ordinaryFinancialAccountFinite(c.account) ||
		!ordinaryFinancialFinite(c.statistics.NetPnL, c.statistics.GrossWins, c.statistics.GrossLosses) ||
		!ordinaryFinancialOptionalFinite(c.statistics.WinRate) || !ordinaryFinancialOptionalFinite(c.statistics.ProfitFactor) ||
		!paperSame(c.account.StartEquity, x.costs.StartEquity) || !paperSame(c.account.RealizedNet, b.realized) {
		return OrdinaryBacktestFinancials{}, ordinaryFinancialError("terminal state", n-1)
	}

	// Collect genuine later final-exit sizes, already checked by the raw hook
	// against its observed remainder. Also bind every exit to its raw native row.
	// This witnesses post-partial financial sizes retrospectively, not a complete
	// contemporaneous position/order trace. Never derive entrySize - partialSize.
	finalSizes := make(map[string]float64)
	tradeIndex := 0
	for _, e := range c.events {
		if e.Kind != "partial" && e.Kind != "exit" {
			continue
		}
		if tradeIndex >= len(b.trades) || !ordinaryFinancialExitMatches(e, b.trades[tradeIndex]) {
			return OrdinaryBacktestFinancials{}, ordinaryFinancialError("raw exit linkage", e.Index)
		}
		if e.Kind == "exit" {
			if _, exists := finalSizes[e.PositionID]; exists {
				return OrdinaryBacktestFinancials{}, ordinaryFinancialError("duplicate completion", e.Index)
			}
			finalSizes[e.PositionID] = e.Effect.Size
		}
		tradeIndex++
	}
	if tradeIndex != len(b.trades) {
		return OrdinaryBacktestFinancials{}, ordinaryFinancialError("missing exit", n-1)
	}

	out := OrdinaryBacktestFinancials{StartEquity: x.costs.StartEquity,
		Events: make([]OrdinaryBacktestFinancialEvent, 0, len(c.events)-n),
		Marks:  make([]OrdinaryBacktestFinancialMark, 0, n)}
	var open *PaperEvent
	var size, credits, realized float64
	partial := false
	lastIndex, ordinal, nextMark, tradeIndex := -1, 0, 0, 0
	seenEvents, seenPositions := map[string]bool{}, map[string]bool{}
	for _, e := range c.events {
		if e.Index < 0 || e.Index >= n || e.Index < lastIndex || !paperSame(e.T, x.bars[e.Index].T) ||
			!ordinaryFinancialFinite(e.RealizedBefore, e.RealizedAfter) || !paperSame(e.RealizedBefore, realized) ||
			e.EventID == "" || seenEvents[e.EventID] {
			return OrdinaryBacktestFinancials{}, ordinaryFinancialError("event continuity", e.Index)
		}
		seenEvents[e.EventID] = true
		if e.Index != lastIndex {
			ordinal = 0
			lastIndex = e.Index
		}
		if e.Ordinal != ordinal {
			return OrdinaryBacktestFinancials{}, ordinaryFinancialError("event ordinal", e.Index)
		}
		ordinal++
		// The only mutation after a retained bar mark is native final liquidation
		// after the provisional last mark. Earlier marks cannot hide later fills.
		afterLastMark := nextMark == n && e.Index == n-1 && e.Kind == "exit" && e.Effect != nil && e.Effect.Reason == ReasonEndOfTest
		if e.Index != nextMark && !afterLastMark {
			return OrdinaryBacktestFinancials{}, ordinaryFinancialError("mark ordering", e.Index)
		}
		if e.Kind == "mark" {
			if e.Account == nil || e.Effect != nil || e.PositionID != "" || !paperSame(e.RealizedAfter, realized) ||
				!ordinaryFinancialAccountFinite(*e.Account) {
				return OrdinaryBacktestFinancials{}, ordinaryFinancialError("mark", e.Index)
			}
			u := float64(0)
			if open != nil {
				sign := float64(1)
				if open.Effect.Side == "short" {
					sign = -1
				}
				difference := float64(x.bars[e.Index].C - open.Effect.Price)
				points := float64(difference * sign)
				u = float64(points * size)
			}
			a := e.Account
			cash := float64(out.StartEquity + realized)
			equity := float64(cash + u)
			if !ordinaryFinancialFinite(u, cash, equity) || !paperSame(a.StartEquity, out.StartEquity) ||
				!paperSame(a.RealizedNet, realized) || !paperSame(a.Unrealized, u) || !paperSame(a.Cash, cash) || !paperSame(a.Equity, equity) ||
				!paperSame(a.EntryFees, out.EntryFees) || !paperSame(a.ExitFees, out.ExitFees) || !paperSame(a.Fees, float64(out.EntryFees+out.ExitFees)) {
				return OrdinaryBacktestFinancials{}, ordinaryFinancialError("mark financials", e.Index)
			}
			out.Marks = append(out.Marks, OrdinaryBacktestFinancialMark{e.Index, e.T, realized, u, equity, out.EntryFees, out.ExitFees})
			nextMark++
			continue
		}
		f := e.Effect
		if f == nil || e.Account != nil || e.PositionID == "" || (f.Side != "long" && f.Side != "short") ||
			!ordinaryFinancialFinite(f.Price, f.Size, f.Fee) || f.Size <= 0 ||
			!ordinaryFinancialOptionalFinite(f.Stop) || !ordinaryFinancialOptionalFinite(f.Target) ||
			!paperSame(f.Fee, float64(x.costs.FeePerUnit*f.Size)) {
			return OrdinaryBacktestFinancials{}, ordinaryFinancialError("effect", e.Index)
		}
		value := OrdinaryBacktestFinancialEvent{Kind: e.Kind, PositionID: e.PositionID, Side: f.Side,
			Index: e.Index, Ordinal: e.Ordinal, TradeIndex: -1, T: e.T, Price: f.Price, Size: f.Size, Fee: f.Fee,
			RealizedBefore: e.RealizedBefore, RealizedAfter: e.RealizedAfter}
		switch e.Kind {
		case "entry":
			if open != nil || seenPositions[e.PositionID] || f.ExitCredit != nil || f.CompletedNetPnL != nil || f.Reason != "" || f.Rule != "" {
				return OrdinaryBacktestFinancials{}, ordinaryFinancialError("entry lifecycle", e.Index)
			}
			entry := e
			open, size, credits, partial = &entry, f.Size, 0, false
			seenPositions[e.PositionID] = true
			out.EntryFees = float64(out.EntryFees + f.Fee)
		case "partial", "exit":
			if open == nil || open.PositionID != e.PositionID || open.Effect.Side != f.Side || f.ExitCredit == nil || !isFinite(*f.ExitCredit) {
				return OrdinaryBacktestFinancials{}, ordinaryFinancialError("exit lifecycle", e.Index)
			}
			trade := b.trades[tradeIndex]
			if trade.EntryIndex != open.Index || !paperSame(trade.EntryT, open.T) || !paperSame(trade.Entry, open.Effect.Price) {
				return OrdinaryBacktestFinancials{}, ordinaryFinancialError("entry linkage", e.Index)
			}
			credits = float64(credits + *f.ExitCredit)
			out.ExitFees = float64(out.ExitFees + f.Fee)
			value.TradeIndex, value.ExitCredit = tradeIndex, *f.ExitCredit
			tradeIndex++
			if e.Kind == "partial" {
				remainder, ok := finalSizes[e.PositionID]
				if partial || f.CompletedNetPnL != nil || f.Size > size || !ok || remainder <= 0 || remainder > size {
					return OrdinaryBacktestFinancials{}, ordinaryFinancialError("partial remainder witness", e.Index)
				}
				size, partial = remainder, true
			} else {
				net := float64(credits - open.Effect.Fee)
				if !paperSame(f.Size, size) || f.CompletedNetPnL == nil || !ordinaryFinancialFinite(net, *f.CompletedNetPnL) || !paperSame(*f.CompletedNetPnL, net) {
					return OrdinaryBacktestFinancials{}, ordinaryFinancialError("completion", e.Index)
				}
				out.LastFlatRealized = e.RealizedAfter
				open = nil
			}
		default:
			return OrdinaryBacktestFinancials{}, ordinaryFinancialError("event kind", e.Index)
		}
		if !ordinaryFinancialFinite(out.EntryFees, out.ExitFees, credits) {
			return OrdinaryBacktestFinancials{}, ordinaryFinancialError("financial reduction", e.Index)
		}
		realized = e.RealizedAfter
		out.Events = append(out.Events, value)
	}
	if nextMark != n || open != nil || !paperSame(realized, b.realized) || !paperSame(out.LastFlatRealized, b.realized) ||
		!paperSame(c.account.EntryFees, out.EntryFees) || !paperSame(c.account.ExitFees, out.ExitFees) ||
		!paperSame(c.account.Fees, float64(out.EntryFees+out.ExitFees)) {
		return OrdinaryBacktestFinancials{}, ordinaryFinancialError("incomplete lifecycle", n-1)
	}

	// This is a fresh terminal snapshot, not c.account: ordinary liquidation
	// updates realized and fees after its final provisional collector mark. Its
	// discarded peak must not contaminate the finalized curve's later reduction.
	out.TerminalRealized = b.realized
	cash := float64(out.StartEquity + b.realized)
	equity := float64(cash + out.TerminalUnrealized)
	if !ordinaryFinancialFinite(cash, equity) {
		return OrdinaryBacktestFinancials{}, ordinaryFinancialError("terminal mark", n-1)
	}
	out.Marks[n-1] = OrdinaryBacktestFinancialMark{n - 1, x.bars[n-1].T, b.realized, 0, equity, out.EntryFees, out.ExitFees}
	return out, nil
}

func ordinaryFinancialSameCosts(a, b Costs) bool {
	return a.FillOn == b.FillOn && paperSame(a.StartEquity, b.StartEquity) && paperSame(a.FeePerUnit, b.FeePerUnit) &&
		paperSame(a.Slippage, b.Slippage) && paperSame(a.SlippageBps, b.SlippageBps)
}

func ordinaryFinancialOptionalFinite(v *float64) bool { return v == nil || isFinite(*v) }

func ordinaryFinancialAccountFinite(a PaperAccount) bool {
	return ordinaryFinancialFinite(a.StartEquity, a.RealizedNet, a.Unrealized, a.Cash, a.Equity, a.EntryFees, a.ExitFees,
		a.Fees, a.PeakEquity, a.Drawdown, a.DrawdownFraction, a.MaxDrawdown, a.MaxDrawdownFraction)
}

func ordinaryFinancialExitMatches(e PaperEvent, t Trade) bool {
	f := e.Effect
	if f == nil || f.ExitCredit == nil || e.Index != t.ExitIndex || !paperSame(e.T, t.ExitT) || f.Side != t.Side ||
		!paperSame(f.Price, t.Exit) || !paperSame(f.Size, t.Size) || !paperSame(*f.ExitCredit, t.PnL) ||
		(e.Kind == "partial") != t.Partial || f.Reason != t.Reason || f.Rule != t.Rule ||
		!ordinaryFinancialFinite(t.Entry, t.Exit, t.Size, t.Points, t.PnL, t.EntryT, t.ExitT) {
		return false
	}
	return (f.Stop == nil) == t.NoStop && (f.Target == nil) == t.NoTarget &&
		(f.Stop == nil || (isFinite(*f.Stop) && paperSame(*f.Stop, t.SL) && isFinite(t.InitialSL))) &&
		(f.Target == nil || (isFinite(*f.Target) && paperSame(*f.Target, t.TP) && isFinite(t.InitialTP)))
}
