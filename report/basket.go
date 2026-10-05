package report

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"
)

// Basket reports consume caller-verified NET P&L. They do not execute strategies,
// simulate fills, infer costs, or authenticate the caller's provenance.
const BasketRequestSchema = "basket-report-request-v1"
const BasketResultSchema = "basket-report-result-v1"
const basketMSPerYear = 365.25 * 24 * 60 * 60 * 1000

// BasketTrade is one realized exit record. Partial exits are merged by entry
// identity before measurement. Missing initialSl falls back to sl. A zero risk
// distance or non-positive size has no R denominator, but still counts in cash.
// Status is closed (also the default), open, or prefix. The latter two are never
// evidence. PnL is already net of the verified costs of this run.
type BasketTrade struct {
	EntryT     int64    `json:"entryT"`
	ExitT      int64    `json:"exitT,omitempty"`
	EntryIndex *int     `json:"entryIndex,omitempty"`
	ExitIndex  int      `json:"exitIndex,omitempty"`
	Side       string   `json:"side,omitempty"`
	Tag        string   `json:"tag,omitempty"`
	Entry      float64  `json:"entry"`
	Size       float64  `json:"size"`
	PnL        float64  `json:"pnl"`
	InitialSL  *float64 `json:"initialSl"`
	SL         *float64 `json:"sl"`
	Partial    bool     `json:"partial,omitempty"`
	Status     string   `json:"status,omitempty"`
}

type BasketSpan struct {
	FirstBarT *int64 `json:"firstBarT"`
	LastBarT  *int64 `json:"lastBarT"`
}

// Identity and provenance are bounded opaque JSON objects: echoed, never
// interpreted as instructions or authenticated by this statistics-only API.
type BasketLegInput struct {
	Symbol        string          `json:"symbol"`
	TF            string          `json:"tf"`
	Bars          int             `json:"bars"`
	DataSpan      *BasketSpan     `json:"dataSpan"`
	Identity      json.RawMessage `json:"identity"`
	Provenance    json.RawMessage `json:"provenance"`
	Trades        []BasketTrade   `json:"trades"`
	HarshTrades   *[]BasketTrade  `json:"harshTrades"`
	HarshIdentity json.RawMessage `json:"harshIdentity"`
}

type BasketRequest struct {
	Schema      string           `json:"schema"`
	NowMS       int64            `json:"nowMs"`
	Identity    json.RawMessage  `json:"identity"`
	Legs        []BasketLegInput `json:"legs"`
	Bar         *BasketBar       `json:"bar,omitempty"`
	SourceIsDSL *bool            `json:"sourceIsDsl,omitempty"`
}

// A nil PF with reason no-losses represents +infinity; no trades or all-flat
// trades have PF 0. All other numeric values in the contract are finite.
type BasketRStats struct {
	Trades       int      `json:"trades"`
	WinRatePct   float64  `json:"winRatePct"`
	PFR          *float64 `json:"pfR"`
	PFRReason    string   `json:"pfRReason,omitempty"`
	SumR         float64  `json:"sumR"`
	ExpectancyR  float64  `json:"expectancyR"`
	GrossProfitR float64  `json:"grossProfitR"`
	GrossLossR   float64  `json:"grossLossR"`
	MaxDrawdownR float64  `json:"maxDrawdownR"`
}

type BasketCurrency struct {
	Trades      int      `json:"trades"`
	Wins        int      `json:"wins"`
	WinRatePct  float64  `json:"winRatePct"`
	PF          *float64 `json:"pf"`
	PFReason    string   `json:"pfReason,omitempty"`
	Net         float64  `json:"net"`
	Expectancy  float64  `json:"expectancy"`
	GrossProfit float64  `json:"grossProfit"`
	GrossLoss   float64  `json:"grossLoss"`
}

// Complete means the UTC entry calendar year precedes NowMS's UTC year. It
// does not certify that the caller supplied every bar of that calendar year.
type BasketYear struct {
	Year     int  `json:"year"`
	Complete bool `json:"complete"`
	BasketRStats
}

type BasketSpanResult struct {
	FirstBar *string  `json:"firstBar"`
	LastBar  *string  `json:"lastBar"`
	Years    *float64 `json:"years"`
}

type BasketLeg struct {
	Symbol                  string            `json:"symbol"`
	TF                      string            `json:"tf"`
	Bars                    int               `json:"bars"`
	DataSpan                *BasketSpanResult `json:"dataSpan"`
	Identity                json.RawMessage   `json:"identity"`
	Provenance              json.RawMessage   `json:"provenance"`
	HarshIdentity           json.RawMessage   `json:"harshIdentity"`
	Trades                  int               `json:"trades"`
	UnmeasurableTrades      int               `json:"unmeasurableTrades"`
	ExcludedTrades          int               `json:"excludedTrades"`
	TradesPerYear           *float64          `json:"tradesPerYear"`
	R                       BasketRStats      `json:"r"`
	Currency                BasketCurrency    `json:"currency"`
	HarshUnmeasurableTrades int               `json:"harshUnmeasurableTrades"`
	HarshExcludedTrades     int               `json:"harshExcludedTrades"`
	MissingHarsh            bool              `json:"missingHarsh"`
	Harsh                   *BasketRStats     `json:"harsh"`
	Years                   []BasketYear      `json:"years"`
	RecentNegativeYearRun   int               `json:"recentNegativeYearRun"`
}

type BasketPooledYear struct {
	Year     int     `json:"year"`
	Complete bool    `json:"complete"`
	Trades   int     `json:"trades"`
	SumR     float64 `json:"sumR"`
}

type BasketPooledR struct {
	Trades       int      `json:"trades"`
	PFR          *float64 `json:"pfR"`
	PFRReason    string   `json:"pfRReason,omitempty"`
	SumR         float64  `json:"sumR"`
	ExpectancyR  float64  `json:"expectancyR"`
	GrossProfitR float64  `json:"grossProfitR"`
	GrossLossR   float64  `json:"grossLossR"`
}

type BasketPool struct {
	Symbols                 []string           `json:"symbols"`
	TFs                     []string           `json:"tfs"`
	Legs                    int                `json:"legs"`
	Trades                  int                `json:"trades"`
	UnmeasurableTrades      int                `json:"unmeasurableTrades"`
	ExcludedTrades          int                `json:"excludedTrades"`
	PFR                     *float64           `json:"pfR"`
	PFRReason               string             `json:"pfRReason,omitempty"`
	SumR                    float64            `json:"sumR"`
	ExpectancyR             float64            `json:"expectancyR"`
	GrossProfitR            float64            `json:"grossProfitR"`
	GrossLossR              float64            `json:"grossLossR"`
	Currency                BasketCurrency     `json:"currency"`
	HarshUnmeasurableTrades int                `json:"harshUnmeasurableTrades"`
	HarshExcludedTrades     int                `json:"harshExcludedTrades"`
	MissingHarsh            bool               `json:"missingHarsh"`
	Harsh                   *BasketPooledR     `json:"harsh"`
	Years                   []BasketPooledYear `json:"years"`
	NegativeCompleteYears   []int              `json:"negativeCompleteYears"`
	YearStable              bool               `json:"yearStable"`
}

type BasketSymbol struct {
	Symbol                string         `json:"symbol"`
	Legs                  int            `json:"legs"`
	Trades                int            `json:"trades"`
	UnmeasurableTrades    int            `json:"unmeasurableTrades"`
	TradesPerYear         *float64       `json:"tradesPerYear"`
	R                     BasketPooledR  `json:"r"`
	Harsh                 *BasketPooledR `json:"harsh"`
	MissingHarsh          bool           `json:"missingHarsh"`
	RecentNegativeYearRun int            `json:"recentNegativeYearRun"`
}

type BasketResult struct {
	Schema        string          `json:"schema"`
	RequestSHA256 string          `json:"requestSha256"`
	Verdict       *BasketVerdict  `json:"verdict,omitempty"`
	NowMS         int64           `json:"nowMs"`
	Identity      json.RawMessage `json:"identity"`
	Legs          []BasketLeg     `json:"legs"`
	Aggregate     BasketPool      `json:"aggregate"`
	Symbols       []BasketSymbol  `json:"symbols"`
}

func basketPF(profit, loss float64) (*float64, string) {
	if loss > 0 {
		v := profit / loss
		return &v, ""
	}
	if profit > 0 {
		return nil, ProfitFactorNoLosses
	}
	return new(float64), ""
}

func basketRealizedR(trade BasketTrade) *float64 {
	stop := trade.InitialSL
	if stop == nil {
		stop = trade.SL
	}
	if stop == nil || !basketFinite(*stop) {
		return nil
	}
	risk := math.Abs(trade.Entry-*stop) * trade.Size
	if !(risk > 0) || !basketFinite(risk) {
		return nil
	}
	value := trade.PnL / risk
	if !basketFinite(value) {
		return nil
	}
	return &value
}

func basketRStats(values []float64) BasketRStats {
	result := BasketRStats{Trades: len(values)}
	wins, equity, peak := 0, 0.0, 0.0
	for _, v := range values {
		if v > 0 {
			result.GrossProfitR += v
			wins++
		} else {
			result.GrossLossR -= v
		}
		equity += v
		peak = math.Max(peak, equity)
		result.MaxDrawdownR = math.Max(result.MaxDrawdownR, peak-equity)
	}
	result.PFR, result.PFRReason = basketPF(result.GrossProfitR, result.GrossLossR)
	result.SumR = result.GrossProfitR - result.GrossLossR
	if result.Trades > 0 {
		result.WinRatePct = float64(wins) / float64(result.Trades) * 100
		result.ExpectancyR = result.SumR / float64(result.Trades)
	}
	return result
}

func basketCurrencyStats(trades []BasketTrade) BasketCurrency {
	result := BasketCurrency{Trades: len(trades)}
	for _, t := range trades {
		if t.PnL > 0 {
			result.GrossProfit += t.PnL
			result.Wins++
		} else {
			result.GrossLoss -= t.PnL
		}
	}
	result.finish()
	return result
}

func (c *BasketCurrency) finish() {
	c.PF, c.PFReason = basketPF(c.GrossProfit, c.GrossLoss)
	c.Net = c.GrossProfit - c.GrossLoss
	if c.Trades > 0 {
		c.WinRatePct = float64(c.Wins) / float64(c.Trades) * 100
		c.Expectancy = c.Net / float64(c.Trades)
	}
}

// Preserve exit-record order (including the first occurrence of a merged
// entry). It defines the per-leg drawdown curve, just as the reporting input.
func basketReportingTrades(input []BasketTrade) ([]BasketTrade, int) {
	trades := make([]BasketTrade, 0, len(input))
	excluded, partial := 0, false
	for _, t := range input {
		if t.Status == "open" || t.Status == "prefix" {
			excluded++
			continue
		}
		trades = append(trades, t)
		partial = partial || t.Partial
	}
	if !partial {
		return trades, excluded
	}
	type entryKey struct {
		Index     int
		T         int64
		Side, Tag string
		Price     float64
	}
	partialKeys := map[entryKey]bool{}
	for _, t := range trades {
		if !t.Partial {
			continue
		}
		index := -1
		if t.EntryIndex != nil {
			index = *t.EntryIndex
		}
		partialKeys[entryKey{index, t.EntryT, t.Side, t.Tag, t.Entry}] = true
	}
	keys := map[entryKey]int{}
	merged := make([]BasketTrade, 0, len(trades))
	for _, t := range trades {
		index := -1
		if t.EntryIndex != nil {
			index = *t.EntryIndex
		}
		key := entryKey{index, t.EntryT, t.Side, t.Tag, t.Entry}
		if !partialKeys[key] {
			merged = append(merged, t)
			continue
		}
		i, ok := keys[key]
		if !ok {
			keys[key] = len(merged)
			merged = append(merged, t)
			continue
		}
		current := &merged[i]
		current.PnL += t.PnL
		current.Size += t.Size
		if t.ExitIndex > current.ExitIndex {
			current.ExitIndex, current.ExitT = t.ExitIndex, t.ExitT
			if t.SL != nil {
				current.SL = t.SL
			}
		} else if t.ExitIndex == current.ExitIndex && t.ExitT >= current.ExitT {
			current.ExitT = t.ExitT
		}
		current.Partial = false
	}
	return merged, excluded
}

func basketYears(trades []BasketTrade, currentYear int) []BasketYear {
	byYear := map[int][]float64{}
	for _, trade := range trades {
		if r := basketRealizedR(trade); r != nil {
			year := time.UnixMilli(trade.EntryT).UTC().Year()
			byYear[year] = append(byYear[year], *r)
		}
	}
	years := make([]int, 0, len(byYear))
	for year := range byYear {
		years = append(years, year)
	}
	sort.Ints(years)
	result := make([]BasketYear, 0, len(years))
	for _, year := range years {
		result = append(result, BasketYear{year, year < currentYear, basketRStats(byYear[year])})
	}
	return result
}

// Inspect only the last two observed completed-year buckets. Missing calendar
// years are not invented, and this is not an unbounded consecutive-year streak.
func basketRecentNegativeYears(years []BasketYear) int {
	complete := make([]BasketYear, 0, len(years))
	for _, year := range years {
		if year.Complete {
			complete = append(complete, year)
		}
	}
	run := 0
	for i := len(complete) - 1; i >= 0 && run < 2; i-- {
		if complete[i].SumR >= 0 {
			break
		}
		run++
	}
	return run
}

// Match decimal toFixed(2) rounding of the exact input float, including binary
// values just below a decimal midpoint (2.675) and exact ties (0.125). Scaling
// the float first would incorrectly round some near-threshold frequencies.
func basketRound2(v float64) *float64 {
	scaled := new(big.Rat).SetFloat64(math.Abs(v))
	scaled.Mul(scaled, big.NewRat(100, 1))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(scaled.Num(), scaled.Denom(), remainder)
	if remainder.Mul(remainder, big.NewInt(2)).Cmp(scaled.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if v < 0 {
		quotient.Neg(quotient)
	}
	rounded, _ := new(big.Rat).SetFrac(quotient, big.NewInt(100)).Float64()
	return &rounded
}

func basketISO(t *int64) *string {
	if t == nil {
		return nil
	}
	s := time.UnixMilli(*t).UTC().Format("2006-01-02T15:04:05.000Z")
	return &s
}

func basketSummarizeLeg(input BasketLegInput, currentYear int) BasketLeg {
	trades, excluded := basketReportingTrades(input.Trades)
	result := BasketLeg{Symbol: input.Symbol, TF: input.TF, Bars: input.Bars, Identity: input.Identity, Provenance: input.Provenance, HarshIdentity: input.HarshIdentity, Trades: len(trades), ExcludedTrades: excluded}
	values := []float64{}
	for _, t := range trades {
		if r := basketRealizedR(t); r != nil {
			values = append(values, *r)
		} else {
			result.UnmeasurableTrades++
		}
	}
	result.R = basketRStats(values)
	result.Currency = basketCurrencyStats(trades)
	result.Years = basketYears(trades, currentYear)
	result.RecentNegativeYearRun = basketRecentNegativeYears(result.Years)
	if input.HarshTrades != nil {
		harsh, harshExcluded := basketReportingTrades(*input.HarshTrades)
		result.HarshExcludedTrades = harshExcluded
		values = []float64{}
		for _, t := range harsh {
			if r := basketRealizedR(t); r != nil {
				values = append(values, *r)
			} else {
				result.HarshUnmeasurableTrades++
			}
		}
		stats := basketRStats(values)
		result.Harsh = &stats
	}
	result.MissingHarsh = result.Trades > 0 && (result.Harsh == nil || result.Harsh.Trades == 0) || result.HarshUnmeasurableTrades > 0 || result.HarshExcludedTrades > 0
	if span := input.DataSpan; span != nil {
		result.DataSpan = &BasketSpanResult{FirstBar: basketISO(span.FirstBarT), LastBar: basketISO(span.LastBarT)}
		if span.FirstBarT != nil && span.LastBarT != nil {
			years := math.Max(0, float64(*span.LastBarT-*span.FirstBarT)) / basketMSPerYear
			result.DataSpan.Years = basketRound2(years)
			if years > 0 {
				result.TradesPerYear = basketRound2(float64(len(trades)) / years)
			}
		}
	}
	return result
}

func basketPool(legs []BasketLeg, currentYear int) BasketPool {
	result := BasketPool{Symbols: []string{}, TFs: []string{}, Legs: len(legs), Years: []BasketPooledYear{}, NegativeCompleteYears: []int{}, YearStable: true}
	symbols, tfs := map[string]bool{}, map[string]bool{}
	byYear := map[int]*BasketPooledYear{}
	harsh := BasketPooledR{}
	for _, leg := range legs {
		if !symbols[leg.Symbol] {
			symbols[leg.Symbol] = true
			result.Symbols = append(result.Symbols, leg.Symbol)
		}
		if !tfs[leg.TF] {
			tfs[leg.TF] = true
			result.TFs = append(result.TFs, leg.TF)
		}
		result.Trades += leg.Trades
		result.UnmeasurableTrades += leg.UnmeasurableTrades
		result.ExcludedTrades += leg.ExcludedTrades
		result.HarshUnmeasurableTrades += leg.HarshUnmeasurableTrades
		result.HarshExcludedTrades += leg.HarshExcludedTrades
		result.MissingHarsh = result.MissingHarsh || leg.MissingHarsh
		result.GrossProfitR += leg.R.GrossProfitR
		result.GrossLossR += leg.R.GrossLossR
		result.Currency.Trades += leg.Currency.Trades
		result.Currency.Wins += leg.Currency.Wins
		result.Currency.GrossProfit += leg.Currency.GrossProfit
		result.Currency.GrossLoss += leg.Currency.GrossLoss
		if leg.Harsh != nil {
			harsh.Trades += leg.Harsh.Trades
			harsh.GrossProfitR += leg.Harsh.GrossProfitR
			harsh.GrossLossR += leg.Harsh.GrossLossR
		}
		for _, y := range leg.Years {
			bucket := byYear[y.Year]
			if bucket == nil {
				bucket = &BasketPooledYear{Year: y.Year, Complete: y.Year < currentYear}
				byYear[y.Year] = bucket
			}
			bucket.Trades += y.Trades
			bucket.SumR += y.SumR
			// Pooling has no cross-leg trade ordering. Its years intentionally expose
			// no synthetic drawdown or win rate; those are per-leg sequence metrics.
		}
	}
	result.PFR, result.PFRReason = basketPF(result.GrossProfitR, result.GrossLossR)
	result.SumR = result.GrossProfitR - result.GrossLossR
	// Aggregate expectancy retains every closed reporting trade in the divisor,
	// including unmeasurable trades; per-leg R expectancy uses measurable trades.
	if result.Trades > 0 {
		result.ExpectancyR = result.SumR / float64(result.Trades)
	}
	result.Currency.finish()
	if harsh.Trades > 0 {
		harsh.PFR, harsh.PFRReason = basketPF(harsh.GrossProfitR, harsh.GrossLossR)
		harsh.SumR = harsh.GrossProfitR - harsh.GrossLossR
		harsh.ExpectancyR = harsh.SumR / float64(harsh.Trades)
		result.Harsh = &harsh
	}
	years := make([]int, 0, len(byYear))
	for year := range byYear {
		years = append(years, year)
	}
	sort.Ints(years)
	for _, year := range years {
		y := byYear[year]
		result.Years = append(result.Years, *y)
		if y.Complete && y.SumR < 0 {
			result.NegativeCompleteYears = append(result.NegativeCompleteYears, year)
			result.YearStable = false
		}
	}
	return result
}

// BuildBasketReport validates and computes a deterministic report. NowMS is
// required input: no wall clock, random state, strategy registry, or I/O is used.
func BuildBasketReport(request BasketRequest) (BasketResult, error) {
	if err := validateBasketRequest(request); err != nil {
		return BasketResult{}, err
	}
	currentYear := time.UnixMilli(request.NowMS).UTC().Year()
	result := BasketResult{Schema: BasketResultSchema, NowMS: request.NowMS, Identity: request.Identity, Legs: []BasketLeg{}, Symbols: []BasketSymbol{}}
	for _, input := range request.Legs {
		result.Legs = append(result.Legs, basketSummarizeLeg(input, currentYear))
	}
	result.Aggregate = basketPool(result.Legs, currentYear)
	for _, symbol := range result.Aggregate.Symbols {
		legs := []BasketLeg{}
		offeredYears, missing := 0.0, false
		for _, leg := range result.Legs {
			if leg.Symbol == symbol {
				legs = append(legs, leg)
				if leg.DataSpan != nil && leg.DataSpan.Years != nil {
					offeredYears = math.Max(offeredYears, *leg.DataSpan.Years)
				}
				missing = missing || leg.MissingHarsh
			}
		}
		pooled := basketPool(legs, currentYear)
		row := BasketSymbol{Symbol: symbol, Legs: len(legs), Trades: pooled.Trades, UnmeasurableTrades: pooled.UnmeasurableTrades, Harsh: pooled.Harsh, MissingHarsh: missing, RecentNegativeYearRun: basketRecentNegativePooledYears(pooled.Years)}
		row.R = BasketPooledR{Trades: pooled.Trades - pooled.UnmeasurableTrades, PFR: pooled.PFR, PFRReason: pooled.PFRReason, SumR: pooled.SumR, ExpectancyR: pooled.ExpectancyR, GrossProfitR: pooled.GrossProfitR, GrossLossR: pooled.GrossLossR}
		if offeredYears > 0 {
			row.TradesPerYear = basketRound2(float64(pooled.Trades) / offeredYears)
		}
		if missing {
			row.Harsh = nil
		}
		result.Symbols = append(result.Symbols, row)
	}
	if request.Bar != nil {
		verdict := evaluateBasketBar(result, *request.Bar, request.SourceIsDSL)
		result.Verdict = &verdict
	}
	// A finite input can still overflow when summed. Fail instead of emitting a
	// partial report, null arithmetic, or JSON's unsupported infinity values.
	if _, err := json.Marshal(result); err != nil {
		return BasketResult{}, fmt.Errorf("basket report numeric overflow: %w", err)
	}
	return result, nil
}

func basketFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func basketRecentNegativePooledYears(years []BasketPooledYear) int {
	converted := make([]BasketYear, 0, len(years))
	for _, y := range years {
		converted = append(converted, BasketYear{Year: y.Year, Complete: y.Complete, BasketRStats: BasketRStats{SumR: y.SumR}})
	}
	return basketRecentNegativeYears(converted)
}
