package report

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/engine"
)

const (
	ordinaryMoneyUnit      = "native-account-unit"
	ordinaryCompletedBasis = "logical-position-completed-v1"
	ordinaryDrawdownBasis  = "post-execution-original-close-running-peak-v1"
)

// OrdinaryBacktestProjection contains only the financial sections of the v1
// contract. It is not a report success, native result, or admission receipt.
// Native-envelope validation and assembly remain separate work.
type OrdinaryBacktestProjection struct {
	Account            OrdinaryBacktestAccount       `json:"account"`
	CompletedPositions OrdinaryBacktestCompleted     `json:"completedPositions"`
	Drawdown           OrdinaryBacktestDrawdown      `json:"drawdown"`
	Equity             []OrdinaryBacktestEquityPoint `json:"equity"`
	Analyses           OrdinaryBacktestAnalyses      `json:"analyses"`
}

// OrdinaryBacktestMetric omits value for unavailable metrics and reason for
// available metrics. Zero is an available value, never an omitted value.
type OrdinaryBacktestMetric struct {
	Status string   `json:"status"`
	Value  *float64 `json:"value,omitempty"`
	Reason string   `json:"reason,omitempty"`
	Unit   string   `json:"unit"`
	Basis  string   `json:"basis"`
}

type OrdinaryBacktestAccount struct {
	StartEquity        OrdinaryBacktestMetric `json:"startEquity"`
	CompletedEndEquity OrdinaryBacktestMetric `json:"completedEndEquity"`
	CompletedNetPnl    OrdinaryBacktestMetric `json:"completedNetPnl"`
	CompletedReturnPct OrdinaryBacktestMetric `json:"completedReturnPct"`
	RealizedCashPnl    OrdinaryBacktestMetric `json:"realizedCashPnl"`
	MarkedEndEquity    OrdinaryBacktestMetric `json:"markedEndEquity"`
	MarkedNetPnl       OrdinaryBacktestMetric `json:"markedNetPnl"`
	MarkedReturnPct    OrdinaryBacktestMetric `json:"markedReturnPct"`
	OpenUnrealizedPnl  OrdinaryBacktestMetric `json:"openUnrealizedPnl"`
	OpenEntryFeePaid   OrdinaryBacktestMetric `json:"openEntryFeePaid"`
	EntryFeesPaid      OrdinaryBacktestMetric `json:"entryFeesPaid"`
	ExitFeesPaid       OrdinaryBacktestMetric `json:"exitFeesPaid"`
}

type OrdinaryBacktestCompleted struct {
	Count        OrdinaryBacktestMetric `json:"count"`
	Wins         OrdinaryBacktestMetric `json:"wins"`
	Losses       OrdinaryBacktestMetric `json:"losses"`
	Breakeven    OrdinaryBacktestMetric `json:"breakeven"`
	NetPnl       OrdinaryBacktestMetric `json:"netPnl"`
	WinRate      OrdinaryBacktestMetric `json:"winRate"`
	ProfitFactor OrdinaryBacktestMetric `json:"profitFactor"`
	Expectancy   OrdinaryBacktestMetric `json:"expectancy"`
	AverageWin   OrdinaryBacktestMetric `json:"averageWin"`
	AverageLoss  OrdinaryBacktestMetric `json:"averageLoss"`
	WorstLoss    OrdinaryBacktestMetric `json:"worstLoss"`
}

type OrdinaryBacktestLocation struct {
	Index int     `json:"index"`
	T     float64 `json:"t"`
}

type OrdinaryBacktestDrawdown struct {
	Basis     string                    `json:"basis"`
	Amount    OrdinaryBacktestMetric    `json:"amount"`
	Percent   OrdinaryBacktestMetric    `json:"percent"`
	AmountAt  *OrdinaryBacktestLocation `json:"amountAt"`
	PercentAt *OrdinaryBacktestLocation `json:"percentAt"`
}

// Equity fields are plain native-account-unit numbers, not tagged metrics.
type OrdinaryBacktestEquityPoint struct {
	Index           int     `json:"index"`
	T               float64 `json:"t"`
	RealizedCashPnl float64 `json:"realizedCashPnl"`
	UnrealizedPnl   float64 `json:"unrealizedPnl"`
	EntryFeesPaid   float64 `json:"entryFeesPaid"`
	ExitFeesPaid    float64 `json:"exitFeesPaid"`
	Equity          float64 `json:"equity"`
}

type OrdinaryBacktestUnavailableAnalysis struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type OrdinaryBacktestAnalyses struct {
	Groupings        OrdinaryBacktestUnavailableAnalysis `json:"groupings"`
	RDistribution    OrdinaryBacktestUnavailableAnalysis `json:"rDistribution"`
	MonthlyReturns   OrdinaryBacktestUnavailableAnalysis `json:"monthlyReturns"`
	CostSweep        OrdinaryBacktestUnavailableAnalysis `json:"costSweep"`
	PortfolioFit     OrdinaryBacktestUnavailableAnalysis `json:"portfolioFit"`
	ReplayMetrics    OrdinaryBacktestUnavailableAnalysis `json:"replayMetrics"`
	PersistedSummary OrdinaryBacktestUnavailableAnalysis `json:"persistedSummary"`
}

// ProjectOrdinaryBacktestFinancials validates and projects a copied financial
// snapshot without executing a strategy or invoking the legacy report reducer.
// It accepts the capture's finalized-flat ordinary lifecycle only. It cannot
// authenticate caller-created values, original bars, fees against native costs,
// native trades, or the still-unresolved contemporaneous full position/order
// witness. Those are capture and later assembly/qualification responsibilities.
// It never reconstructs broker realized mutations from credits or equity.
// Every error returns the zero projection; input slices and raw bits are read-only.
func ProjectOrdinaryBacktestFinancials(a engine.OrdinaryBacktestFinancials) (OrdinaryBacktestProjection, error) {
	stats, err := validateOrdinaryFinancials(a)
	if err != nil {
		return OrdinaryBacktestProjection{}, err
	}
	completedEnd := float64(a.StartEquity + a.LastFlatRealized)
	completedReturn := float64(float64(a.LastFlatRealized/a.StartEquity) * 100)
	markedEnd := float64(float64(a.StartEquity+a.TerminalRealized) + a.TerminalUnrealized)
	markedNet := float64(a.TerminalRealized + a.TerminalUnrealized)
	markedReturn := float64(float64(markedNet/a.StartEquity) * 100)
	if !ordinaryProjectionFinite(completedEnd, completedReturn, markedEnd, markedNet, markedReturn) {
		return OrdinaryBacktestProjection{}, ordinaryProjectionError("account reduction", -1)
	}
	money := func(v float64, basis string) OrdinaryBacktestMetric {
		return ordinaryAvailable(v, ordinaryMoneyUnit, basis)
	}
	count := func(v int) OrdinaryBacktestMetric {
		return ordinaryAvailable(float64(v), "count", ordinaryCompletedBasis)
	}
	unavailableStatistic := ordinaryUnavailable("not-in-contract", ordinaryMoneyUnit, "ordinary-statistic-not-in-contract-v1")
	unavailableAnalysis := OrdinaryBacktestUnavailableAnalysis{"unavailable", "not-in-contract"}
	out := OrdinaryBacktestProjection{
		Account: OrdinaryBacktestAccount{
			StartEquity:        money(a.StartEquity, "explicit-start-equity-v1"),
			CompletedEndEquity: money(completedEnd, "broker-last-flat-completed-v1"),
			CompletedNetPnl:    money(a.LastFlatRealized, "broker-last-flat-completed-v1"),
			CompletedReturnPct: ordinaryAvailable(completedReturn, "percent", "broker-last-flat-completed-v1"),
			RealizedCashPnl:    money(a.TerminalRealized, "broker-realized-terminal-v1"),
			MarkedEndEquity:    money(markedEnd, "broker-close-mtm-terminal-v1"),
			MarkedNetPnl:       money(markedNet, "broker-close-mtm-terminal-v1"),
			MarkedReturnPct:    ordinaryAvailable(markedReturn, "percent", "broker-close-mtm-terminal-v1"),
			OpenUnrealizedPnl:  money(a.TerminalUnrealized, "original-close-open-position-v1"),
			OpenEntryFeePaid:   money(a.OpenEntryFee, "actual-open-entry-fee-v1"),
			EntryFeesPaid:      money(a.EntryFees, "nominal-entry-fees-event-order-v1"),
			ExitFeesPaid:       money(a.ExitFees, "nominal-exit-fees-event-order-v1"),
		},
		CompletedPositions: OrdinaryBacktestCompleted{
			Count: count(stats.count), Wins: count(stats.wins), Losses: count(stats.losses), Breakeven: count(stats.breakeven),
			NetPnl:       money(stats.net, "logical-position-net-reduction-v1"),
			WinRate:      ordinaryUnavailable("no-trades", "ratio", ordinaryCompletedBasis),
			ProfitFactor: ordinaryUnavailable("no-trades", "ratio", ordinaryCompletedBasis),
			Expectancy:   unavailableStatistic, AverageWin: unavailableStatistic, AverageLoss: unavailableStatistic, WorstLoss: unavailableStatistic,
		},
		Drawdown: OrdinaryBacktestDrawdown{Basis: ordinaryDrawdownBasis},
		Equity:   make([]OrdinaryBacktestEquityPoint, len(a.Marks)),
		Analyses: OrdinaryBacktestAnalyses{
			Groupings: unavailableAnalysis, RDistribution: unavailableAnalysis, MonthlyReturns: unavailableAnalysis,
			CostSweep: unavailableAnalysis, PortfolioFit: unavailableAnalysis, ReplayMetrics: unavailableAnalysis, PersistedSummary: unavailableAnalysis,
		},
	}
	if stats.count > 0 {
		out.CompletedPositions.WinRate = ordinaryAvailable(float64(float64(stats.wins)/float64(stats.count)), "ratio", ordinaryCompletedBasis)
		out.CompletedPositions.ProfitFactor = ordinaryUnavailable("no-losses", "ratio", ordinaryCompletedBasis)
		if stats.grossLoss > 0 {
			pf := float64(stats.grossWin / stats.grossLoss)
			if !ordinaryProjectionFinite(pf) {
				return OrdinaryBacktestProjection{}, ordinaryProjectionError("profit factor", -1)
			}
			out.CompletedPositions.ProfitFactor = ordinaryAvailable(pf, "ratio", ordinaryCompletedBasis)
		}
	}
	peak, amount, percent := a.StartEquity, float64(0), float64(0)
	for i, mark := range a.Marks {
		if mark.Equity > peak {
			peak = mark.Equity
		}
		dd := float64(peak - mark.Equity)
		pct := float64(float64(dd/peak) * 100)
		if !ordinaryProjectionFinite(dd, pct) {
			return OrdinaryBacktestProjection{}, ordinaryProjectionError("drawdown reduction", i)
		}
		// Independent strict maxima retain their own earliest positive locator.
		if dd > amount {
			amount = dd
			out.Drawdown.AmountAt = &OrdinaryBacktestLocation{i, ordinaryOutputNumber(mark.T)}
		}
		if pct > percent {
			percent = pct
			out.Drawdown.PercentAt = &OrdinaryBacktestLocation{i, ordinaryOutputNumber(mark.T)}
		}
		out.Equity[i] = OrdinaryBacktestEquityPoint{
			Index: i, T: ordinaryOutputNumber(mark.T), RealizedCashPnl: ordinaryOutputNumber(mark.Realized),
			UnrealizedPnl: ordinaryOutputNumber(mark.Unrealized), EntryFeesPaid: ordinaryOutputNumber(mark.EntryFees),
			ExitFeesPaid: ordinaryOutputNumber(mark.ExitFees), Equity: ordinaryOutputNumber(mark.Equity),
		}
	}
	out.Drawdown.Amount = money(amount, ordinaryDrawdownBasis)
	out.Drawdown.Percent = ordinaryAvailable(percent, "percent", ordinaryDrawdownBasis)
	return out, nil
}

type ordinaryFinancialStatistics struct {
	count, wins, losses, breakeven int
	net, grossWin, grossLoss       float64
}

// Validate internal financial continuity without rerunning target-dependent
// broker arithmetic. Nominal fees are observed values already multiplied at the
// capture boundary; summing them preserves event order, not realized deltas.
func validateOrdinaryFinancials(a engine.OrdinaryBacktestFinancials) (ordinaryFinancialStatistics, error) {
	var stats ordinaryFinancialStatistics
	fail := func(field string, index int) (ordinaryFinancialStatistics, error) {
		return ordinaryFinancialStatistics{}, ordinaryProjectionError(field, index)
	}
	if !ordinaryProjectionFinite(a.StartEquity, a.TerminalRealized, a.LastFlatRealized, a.TerminalUnrealized, a.OpenEntryFee, a.EntryFees, a.ExitFees) ||
		a.StartEquity <= 0 || a.EntryFees < 0 || a.ExitFees < 0 || a.TerminalUnrealized != 0 || a.OpenEntryFee != 0 ||
		len(a.Marks) == 0 || len(a.Marks) > engine.OrdinaryBacktestMaxBars || len(a.Events) > engine.PaperPrefixMaxEvents-len(a.Marks) {
		return fail("snapshot", -1)
	}
	lastIndex := -1
	for _, e := range a.Events {
		if e.Index < 0 || e.Index >= len(a.Marks) || e.Index < lastIndex || e.Ordinal < 0 ||
			!ordinaryProjectionFinite(e.T, e.Price, e.Size, e.Fee, e.RealizedBefore, e.RealizedAfter, e.ExitCredit) ||
			e.Size <= 0 || e.Fee < 0 || e.PositionID == "" || (e.Side != "long" && e.Side != "short") {
			return fail("event shape", e.Index)
		}
		lastIndex = e.Index
	}
	var open *engine.OrdinaryBacktestFinancialEvent
	var realized, lastFlat, entryFees, exitFees, credits float64
	seen := make(map[string]bool)
	partial := false
	nextEvent, nextTrade := 0, 0
	for i, mark := range a.Marks {
		if mark.Index != i || !ordinaryProjectionFinite(mark.T, mark.Realized, mark.Unrealized, mark.Equity, mark.EntryFees, mark.ExitFees) ||
			mark.T < 0 || mark.T > 9007199254740991 || mark.T != math.Trunc(mark.T) || (i > 0 && mark.T <= a.Marks[i-1].T) {
			return fail("mark shape", i)
		}
		ordinal := 0
		for nextEvent < len(a.Events) && a.Events[nextEvent].Index == i {
			e := &a.Events[nextEvent]
			// Marks consumed one collector ordinal. Only final liquidation may
			// follow the discarded last provisional mark, leaving exactly one gap.
			finalAfterMark := i == len(a.Marks)-1 && nextEvent == len(a.Events)-1 && e.Kind == "exit" && e.Ordinal == ordinal+1
			if (e.Ordinal != ordinal && !finalAfterMark) || !ordinaryProjectionSame(e.T, mark.T) || !ordinaryProjectionSame(e.RealizedBefore, realized) {
				return fail("event continuity", i)
			}
			ordinal = e.Ordinal + 1
			switch e.Kind {
			case "entry":
				if open != nil || seen[e.PositionID] || e.TradeIndex != -1 || !ordinaryProjectionSame(e.ExitCredit, 0) {
					return fail("entry lifecycle", i)
				}
				open, credits, partial = e, 0, false
				seen[e.PositionID] = true
				entryFees = float64(entryFees + e.Fee)
			case "partial", "exit":
				if open == nil || e.PositionID != open.PositionID || e.Side != open.Side || e.TradeIndex != nextTrade || e.Size > open.Size {
					return fail("exit lifecycle", i)
				}
				nextTrade++
				credits = float64(credits + e.ExitCredit)
				exitFees = float64(exitFees + e.Fee)
				if e.Kind == "partial" {
					if partial {
						return fail("duplicate partial", i)
					}
					partial = true
				} else {
					if !partial && !ordinaryProjectionSame(e.Size, open.Size) {
						return fail("final size", i)
					}
					// The final size is an actual remainder witness. Do not derive
					// it by subtracting partial size from entry size.
					net := float64(credits - open.Fee)
					stats.net = float64(stats.net + net)
					stats.count++
					switch {
					case net > 0:
						stats.wins++
						stats.grossWin = float64(stats.grossWin + net)
					case net < 0:
						stats.losses++
						stats.grossLoss = float64(stats.grossLoss - net)
					default:
						stats.breakeven++
					}
					if !ordinaryProjectionFinite(net, stats.net, stats.grossWin, stats.grossLoss) {
						return fail("completion reduction", i)
					}
					lastFlat, open = e.RealizedAfter, nil
				}
			default:
				return fail("event kind", i)
			}
			if !ordinaryProjectionFinite(entryFees, exitFees, credits) {
				return fail("event reduction", i)
			}
			realized = e.RealizedAfter
			nextEvent++
		}
		cash := float64(a.StartEquity + mark.Realized)
		equity := float64(cash + mark.Unrealized)
		if !ordinaryProjectionFinite(cash, equity) || !ordinaryProjectionSame(mark.Realized, realized) ||
			!ordinaryProjectionSame(mark.EntryFees, entryFees) || !ordinaryProjectionSame(mark.ExitFees, exitFees) ||
			!ordinaryProjectionSame(mark.Equity, equity) || (open == nil && mark.Unrealized != 0) {
			return fail("mark financials", i)
		}
	}
	terminal := a.Marks[len(a.Marks)-1]
	if open != nil || !ordinaryProjectionSame(realized, a.TerminalRealized) || !ordinaryProjectionSame(lastFlat, a.LastFlatRealized) ||
		!ordinaryProjectionSame(a.TerminalRealized, a.LastFlatRealized) || !ordinaryProjectionSame(terminal.Unrealized, a.TerminalUnrealized) ||
		!ordinaryProjectionSame(entryFees, a.EntryFees) || !ordinaryProjectionSame(exitFees, a.ExitFees) {
		return fail("terminal identity", len(a.Marks)-1)
	}
	return stats, nil
}

func ordinaryAvailable(value float64, unit, basis string) OrdinaryBacktestMetric {
	value = ordinaryOutputNumber(value)
	return OrdinaryBacktestMetric{Status: "available", Value: &value, Unit: unit, Basis: basis}
}

func ordinaryUnavailable(reason, unit, basis string) OrdinaryBacktestMetric {
	return OrdinaryBacktestMetric{Status: "unavailable", Reason: reason, Unit: unit, Basis: basis}
}

func ordinaryOutputNumber(value float64) float64 {
	if value == 0 {
		return 0
	}
	return value
}

func ordinaryProjectionSame(a, b float64) bool { return math.Float64bits(a) == math.Float64bits(b) }

func ordinaryProjectionFinite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func ordinaryProjectionError(field string, index int) error {
	return fmt.Errorf("ordinary financial projection: %s at bar %d", field, index)
}
