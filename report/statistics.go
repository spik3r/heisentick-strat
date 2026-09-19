package report

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Profit-factor reasons for a nil ProfitFactor.
const (
	ProfitFactorNoTrades = "no-trades"
	ProfitFactorNoLosses = "no-losses"
)

// MaxDrawdownUnit names the unit of Headline.MaxDrawdown: the largest
// peak-to-trough equity drop as a percentage of the final equity peak, the
// same figure the cost row reports as dd. It is not an amount of account
// currency.
const MaxDrawdownUnit = "percent-of-peak-equity"

// Headline is the summary of one trade list, shaped as the app's
// headlineFromReport builds it from the primary cost row.
//
// Units: Net and Expectancy are in the account currency of the engine's pnl;
// WinRate is a fraction in [0, 1]; MaxDrawdown is a percentage of the final
// equity peak (MaxDrawdownUnit) over an equity curve that starts at 10000 and
// applies the trades in entry-time order. FirstTradeT is the earliest entryT
// and LastTradeT the latest exitT, both ms since the Unix epoch.
type Headline struct {
	Trades int `json:"trades"`
	// WinRate is nil when there are no trades.
	WinRate *float64 `json:"winRate"`
	// ProfitFactor is nil when there are no trades or no losing trades;
	// ProfitFactorReason then says which.
	ProfitFactor       *float64 `json:"profitFactor"`
	ProfitFactorReason string   `json:"profitFactorReason,omitempty"`
	Net                float64  `json:"net"`
	// Expectancy is nil when there are no trades.
	Expectancy      *float64 `json:"expectancy"`
	MaxDrawdown     float64  `json:"maxDrawdown"`
	MaxDrawdownUnit string   `json:"maxDrawdownUnit"`
	FirstTradeT     *int64   `json:"firstTradeT"`
	LastTradeT      *int64   `json:"lastTradeT"`
}

// Group is the summary of the trades sharing one grouping key. Every group
// holds at least one trade, so WinRate (a fraction) and Expectancy are always
// present; ProfitFactor is nil with reason no-losses when no trade lost.
type Group struct {
	Key                string   `json:"key"`
	Trades             int      `json:"trades"`
	WinRate            float64  `json:"winRate"`
	ProfitFactor       *float64 `json:"profitFactor"`
	ProfitFactorReason string   `json:"profitFactorReason,omitempty"`
	Net                float64  `json:"net"`
	Expectancy         float64  `json:"expectancy"`
}

// Groupings splits the primary cost's trades. Each list is ordered by key.
// Session is the engine's session phase at the entry bar's local hour (the
// same value the annotated trade carries as reportSessionPhase); Side is the
// trade side; ExitReason is the engine's exit reason; Year is the UTC year
// of the entry time.
type Groupings struct {
	Session    []Group `json:"session"`
	Side       []Group `json:"side"`
	ExitReason []Group `json:"exitReason"`
	Year       []Group `json:"year"`
}

// DateBounds is the span of entry bars the engine ran: the first and last
// bar times (ms) of the entry series as supplied, and the bar count. The
// caller trims the series to its window and warm-up before the run, so these
// are the bounds after that trimming.
type DateBounds struct {
	FirstT int64 `json:"firstT"`
	LastT  int64 `json:"lastT"`
	Bars   int   `json:"bars"`
}

// Holdout splits the primary cost's trades at FromT: a trade with
// entryT >= FromT is holdout, every other trade is in-sample, and no trade is
// in both. Each side is summarised as its own Headline, with its own equity
// curve starting at 10000.
type Holdout struct {
	FromT    int64    `json:"fromT"`
	InSample Headline `json:"inSample"`
	Holdout  Headline `json:"holdout"`
}

// headlineFromRow maps a cost row and its trades to the headline shape.
func headlineFromRow(row CostRow, trades []engine.Trade) Headline {
	headline := Headline{
		Trades:          row.Trades,
		Net:             row.Net,
		MaxDrawdown:     row.DD,
		MaxDrawdownUnit: MaxDrawdownUnit,
	}
	if headline.MaxDrawdown < 0 {
		headline.MaxDrawdown = 0
	}
	if row.Trades > 0 {
		winRate := row.WinRate / 100
		expectancy := row.Expectancy
		headline.WinRate = &winRate
		headline.Expectancy = &expectancy
	}
	switch {
	case row.Trades == 0:
		headline.ProfitFactorReason = ProfitFactorNoTrades
	case row.PF == nil:
		headline.ProfitFactorReason = ProfitFactorNoLosses
	default:
		pf := *row.PF
		headline.ProfitFactor = &pf
	}
	for i, trade := range trades {
		entryT, exitT := int64(trade.EntryT), int64(trade.ExitT)
		if i == 0 || entryT < *headline.FirstTradeT {
			headline.FirstTradeT = &entryT
		}
		if i == 0 || exitT > *headline.LastTradeT {
			headline.LastTradeT = &exitT
		}
	}
	return headline
}

// headlineFromTrades summarises a trade subset with the cost-row formulas.
func headlineFromTrades(trades []engine.Trade) (Headline, error) {
	row, err := summarize("", 0, engine.RunResult{
		Costs:  engine.Costs{StartEquity: defaultStartEquity},
		Trades: trades,
	})
	if err != nil {
		return Headline{}, err
	}
	return headlineFromRow(row, trades), nil
}

func splitHoldout(trades []engine.Trade, fromT int64) (Holdout, error) {
	inSample := make([]engine.Trade, 0, len(trades))
	holdout := make([]engine.Trade, 0, len(trades))
	for _, trade := range trades {
		if int64(trade.EntryT) >= fromT {
			holdout = append(holdout, trade)
		} else {
			inSample = append(inSample, trade)
		}
	}
	inSampleHeadline, err := headlineFromTrades(inSample)
	if err != nil {
		return Holdout{}, fmt.Errorf("holdout in-sample: %w", err)
	}
	holdoutHeadline, err := headlineFromTrades(holdout)
	if err != nil {
		return Holdout{}, fmt.Errorf("holdout: %w", err)
	}
	return Holdout{FromT: fromT, InSample: inSampleHeadline, Holdout: holdoutHeadline}, nil
}

func dateBounds(series marketdata.Series) DateBounds {
	n := series.Len()
	if n == 0 {
		return DateBounds{}
	}
	return DateBounds{FirstT: int64(series.T[0]), LastT: int64(series.T[n-1]), Bars: n}
}

func groupTrades(trades []engine.Trade) Groupings {
	return Groupings{
		Session:    groupBy(trades, func(trade engine.Trade) string { return sessionPhase(trade.EntryT) }),
		Side:       groupBy(trades, func(trade engine.Trade) string { return trade.Side }),
		ExitReason: groupBy(trades, func(trade engine.Trade) string { return trade.Reason }),
		Year: groupBy(trades, func(trade engine.Trade) string {
			return strconv.Itoa(time.UnixMilli(int64(trade.EntryT)).UTC().Year())
		}),
	}
}

// groupBy buckets trades by key and summarises each bucket in key order.
// The result is never nil so the JSON is [] for an empty trade list.
func groupBy(trades []engine.Trade, key func(engine.Trade) string) []Group {
	buckets := make(map[string][]engine.Trade)
	for _, trade := range trades {
		k := key(trade)
		buckets[k] = append(buckets[k], trade)
	}
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	groups := make([]Group, 0, len(keys))
	for _, k := range keys {
		groups = append(groups, summarizeGroup(k, buckets[k]))
	}
	return groups
}

func summarizeGroup(key string, trades []engine.Trade) Group {
	var grossWin, grossLoss float64
	wins := 0
	for _, trade := range trades {
		if trade.PnL > 0 {
			wins++
			grossWin += trade.PnL
		} else {
			grossLoss -= trade.PnL
		}
	}
	count := len(trades)
	net := grossWin - grossLoss
	group := Group{
		Key:        key,
		Trades:     count,
		WinRate:    float64(wins) / float64(count),
		Net:        net,
		Expectancy: net / float64(count),
	}
	// Same rule as the cost row: a bucket with neither wins nor losses
	// (every trade flat) reports 0, not "no losses".
	switch {
	case grossLoss > 0:
		pf := grossWin / grossLoss
		group.ProfitFactor = &pf
	case grossWin > 0:
		group.ProfitFactorReason = ProfitFactorNoLosses
	default:
		group.ProfitFactor = new(float64)
	}
	return group
}

// sessionPhase is the engine's session phase at the entry bar's local hour;
// a negative entry time has no session and reports "other".
func sessionPhase(entryT float64) string {
	if entryT < 0 {
		return "other"
	}
	return contextcols.SessionPhaseAtHour(contextcols.LocalHour(int64(entryT)))
}
