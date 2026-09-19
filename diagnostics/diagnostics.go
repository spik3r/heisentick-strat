// Package diagnostics derives overfit-risk metrics and warnings from three
// report passes over the same strategy: full-history realistic, a recent
// window at realistic cost, and full-history harsh cost. It is a port of the
// derivation in the app's scripts/strategy/overfitReport.mjs (the metric and
// warning arithmetic, not the child-process orchestration around
// strategyCompare) and of the yearly shape in scripts/lib/yearlyStability.mjs.
// The Go package is the source of truth; the fixtures under testdata/ are the
// contract, and testdata/gen regenerates their expected values by running the
// JS functions verbatim on the same inputs.
//
// The functions are pure and do no I/O. A caller builds one Pass per report
// document (PassFromDocument) and calls Evaluate.
//
// The recent window is a recency check, not a holdout. The boundary sits
// inside the same history every screen fits on, so the number measures
// recency, not out-of-sample performance. A genuine holdout needs a boundary
// chosen before the search. The boundary is echoed in the recent-window
// warning so a review note can pin it.
//
// Units follow the JS output. PF values are ratios; Net values are in the
// account currency of the engine's pnl; PosSlicePct and BestSlicePct are
// whole percentages; RecentFrom is the boundary as given.
//
// Null handling mirrors the JS after its JSON round trip: a profit factor
// that is nil (no trades or no losses) or non-finite reads as 0, exactly as
// `Number(row.pf) || 0` does after JSON turned Infinity into null. That also
// means a worst year without losses reads as PF 0 and can raise blow-up-year;
// the port keeps that behaviour rather than fixing it here.
package diagnostics

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/spik3r/heisentick-strat/report"
)

// DefaultMinTrades is the JS `--min-trades` default.
const DefaultMinTrades = 150

// Warning codes, in the order Evaluate emits them.
const (
	CodeBestSliceConcentration = "best-slice-concentration"
	CodeRecentYearNegative     = "recent-year-negative"
	CodeHarshCostNegative      = "harsh-cost-negative"
	CodeThinSample             = "thin-sample"
	CodeRecentWindowPFDrop     = "recent-window-pf-drop"
	CodeBlowUpYear             = "blow-up-year"
)

// Warning is one overfit flag: a stable code consumers gate on and the text
// the JS console output prints.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// YearRow is one calendar year of the full-history pass: the UTC entry year,
// its net and its profit factor (nil when the year had no losing trade).
type YearRow struct {
	Year int
	Net  float64
	PF   *float64
}

// Pass is what the derivation reads from one report pass. Trades, PF and
// Net describe the primary cost row. SliceNets holds the primary-cost net of
// each slice the JS would count (those with trades, or any slice on a
// source/entry route). Years and DataEndT are read from the full pass only:
// Years are the per-year rows in ascending year order, DataEndT the last bar
// time in ms (0 when unknown) that decides whether the trailing year is
// complete.
type Pass struct {
	Trades    int
	PF        *float64
	Net       float64
	SliceNets []float64
	Years     []YearRow
	DataEndT  int64
}

// Input is the three passes plus the recent boundary. Recent and Harsh are
// nil when that compare produced no row; the matching metrics are then nil
// and their warnings do not fire. MinTrades is the thin-sample threshold;
// zero disables the warning, as `--min-trades=0` does.
type Input struct {
	Full       Pass
	Recent     *Pass
	Harsh      *Pass
	RecentFrom string
	MinTrades  int
}

// Result carries the JS report row. Fields the JS prints as "-" are nil
// here; fields the JS rounds (Net, PosSlicePct, BestSlicePct, RecentYearNet,
// PartialYearNet) are rounded the same way (half toward +inf). Fields the JS
// prints with two decimals are left unrounded.
type Result struct {
	Trades int `json:"trades"`
	// PF is the full-history profit factor (0 when nil in the report).
	PF float64 `json:"pf"`
	// Net is the full-history net, rounded to a whole unit.
	Net float64 `json:"net"`
	// PosSlicePct is the share of slices with positive net, whole percent.
	PosSlicePct int `json:"posSlicePct"`
	// BestSlicePct is the share of total positive-slice net carried by the
	// single largest positive slice, whole percent.
	BestSlicePct int `json:"bestSlicePct"`
	// Years and PositiveYears are the JS posYears "p/y" (JS prints "-" when
	// Years is 0).
	Years         int `json:"years"`
	PositiveYears int `json:"positiveYears"`
	// WorstYearPf is the profit factor of the year with the lowest net.
	WorstYearPf *float64 `json:"worstYearPf"`
	// RecentYearNet is the trailing year's net, rounded.
	RecentYearNet *float64 `json:"recentYearNet"`
	// CompletedYears counts years the data covers to their end.
	CompletedYears int `json:"completedYears"`
	// WorstCompletedYearPf is WorstYearPf over completed years only.
	WorstCompletedYearPf *float64 `json:"worstCompletedYearPf"`
	// PartialYear and PartialYearNet describe the trailing year when the
	// data does not run past its end; nil otherwise.
	PartialYear    *int     `json:"partialYear"`
	PartialYearNet *float64 `json:"partialYearNet"`
	RecentFrom     string   `json:"recentFrom"`
	// RecentPf is the recent-window profit factor; RecentDeltaPf is
	// RecentPf - PF. Both nil without a recent pass.
	RecentPf      *float64 `json:"recentPf"`
	RecentDeltaPf *float64 `json:"recentDeltaPf"`
	// HarshPf is the harsh-cost profit factor; CostDeltaPf is PF - HarshPf.
	// Both nil without a harsh pass.
	HarshPf     *float64  `json:"harshPf"`
	CostDeltaPf *float64  `json:"costDeltaPf"`
	Warnings    []Warning `json:"warnings"`
}

// Codes returns the warning codes in emission order.
func (result Result) Codes() []string {
	codes := make([]string, len(result.Warnings))
	for i, warning := range result.Warnings {
		codes[i] = warning.Code
	}
	return codes
}

// Evaluate derives the overfit metrics and warnings for one strategy.
func Evaluate(in Input) Result {
	full := in.Full
	slices := sliceStats(full.SliceNets)
	yearly := yearlyShape(full.Years, full.DataEndT)
	fullPf := numberOrZero(full.PF)

	var recentPf, harshPf, harshNet *float64
	if in.Recent != nil {
		recentPf = ptr(numberOrZero(in.Recent.PF))
	}
	if in.Harsh != nil {
		harshPf = ptr(numberOrZero(in.Harsh.PF))
		harshNet = ptr(finiteOrZero(in.Harsh.Net))
	}
	var costDeltaPf, recentDeltaPf *float64
	if harshPf != nil {
		costDeltaPf = ptr(fullPf - *harshPf)
	}
	if recentPf != nil {
		recentDeltaPf = ptr(*recentPf - fullPf)
	}

	warnings := []Warning{}
	warn := func(code, text string) {
		warnings = append(warnings, Warning{Code: code, Message: text})
	}
	if slices.bestSharePct > 70 {
		warn(CodeBestSliceConcentration, fmt.Sprintf("best slice %s%% of net", jsToFixed(slices.bestSharePct, 0)))
	}
	// Gated on the last completed calendar year; the trailing partial year is
	// reported but not gated on. The Go report always carries the completed
	// fields, so the JS fallback to trailing-year fields is not needed.
	if yearly.recentCompletedYear != nil && yearly.recentCompletedYearNet < 0 {
		warn(CodeRecentYearNegative, fmt.Sprintf("recent year (%d) negative", *yearly.recentCompletedYear))
	}
	if harshNet != nil && *harshNet <= 0 {
		warn(CodeHarshCostNegative, "harsh cost flips negative")
	}
	if full.Trades < in.MinTrades {
		warn(CodeThinSample, fmt.Sprintf("thin sample (%dt < %d)", full.Trades, in.MinTrades))
	}
	// The wording avoids the substring "year": the app's promotion bar still
	// matches warning text for payloads written before codes existed.
	if recentPf != nil && fullPf > 0 && *recentPf < fullPf*0.7 {
		warn(CodeRecentWindowPFDrop, fmt.Sprintf("recent-window PF %s << full %s (from %s)", jsToFixed(*recentPf, 2), jsToFixed(fullPf, 2), in.RecentFrom))
	}
	gatedWorstPf := numberOrZero(yearly.worstCompletedYearPf)
	if gatedWorstPf < 0.5 && yearly.completedYears >= 3 {
		warn(CodeBlowUpYear, fmt.Sprintf("blow-up year PF %s", jsToFixed(gatedWorstPf, 2)))
	}

	result := Result{
		Trades:               full.Trades,
		PF:                   fullPf,
		Net:                  jsRound(full.Net),
		PosSlicePct:          int(jsRound(slices.positiveRatio * 100)),
		BestSlicePct:         int(jsRound(slices.bestSharePct)),
		Years:                yearly.years,
		PositiveYears:        yearly.positiveYears,
		WorstYearPf:          yearly.worstYearPf,
		CompletedYears:       yearly.completedYears,
		WorstCompletedYearPf: yearly.worstCompletedYearPf,
		PartialYear:          yearly.partialYear,
		RecentFrom:           in.RecentFrom,
		RecentPf:             recentPf,
		RecentDeltaPf:        recentDeltaPf,
		HarshPf:              harshPf,
		CostDeltaPf:          costDeltaPf,
		Warnings:             warnings,
	}
	if yearly.recentYearNet != nil {
		result.RecentYearNet = ptr(jsRound(*yearly.recentYearNet))
	}
	if yearly.partialYearNet != nil {
		result.PartialYearNet = ptr(jsRound(*yearly.partialYearNet))
	}
	return result
}

// PassFromDocument reads a Pass from one report document: the primary cost
// row, the primary net of every slice with trades (or on a source/entry
// route), the year groupings and the last bar time. It returns an error when
// the document lacks the primary cost row or the groupings the derivation
// needs.
func PassFromDocument(doc report.Document) (Pass, error) {
	index := doc.PrimaryCost.Index
	if index < 0 || index >= len(doc.Costs) {
		return Pass{}, fmt.Errorf("diagnostics: primary cost index %d outside %d cost rows", index, len(doc.Costs))
	}
	if doc.Groupings == nil {
		return Pass{}, errors.New("diagnostics: document has no groupings; build it with the report package")
	}
	primary := doc.Costs[index]
	pass := Pass{Trades: primary.Trades, PF: primary.PF, Net: primary.Net}
	for i, slice := range doc.Slices {
		if index >= len(slice.Costs) {
			return Pass{}, fmt.Errorf("diagnostics: slice %d has %d cost rows, primary index is %d", i, len(slice.Costs), index)
		}
		cost := slice.Costs[index]
		if cost.Trades > 0 || slice.SourceTimeframe != nil {
			pass.SliceNets = append(pass.SliceNets, cost.Net)
		}
	}
	for _, group := range doc.Groupings.Year {
		year, err := strconv.Atoi(group.Key)
		if err != nil {
			return Pass{}, fmt.Errorf("diagnostics: year group key %q: %w", group.Key, err)
		}
		pass.Years = append(pass.Years, YearRow{Year: year, Net: group.Net, PF: group.ProfitFactor})
	}
	if doc.DateBounds != nil {
		pass.DataEndT = doc.DateBounds.LastT
	}
	return pass, nil
}

type sliceSummary struct {
	count         int
	positiveRatio float64
	bestSharePct  float64
}

// sliceStats mirrors the JS sliceStats: share of total positive-slice net
// carried by the largest positive slice.
func sliceStats(nets []float64) sliceSummary {
	if len(nets) == 0 {
		return sliceSummary{}
	}
	positives, posNet, bestNet := 0, 0.0, 0.0
	for _, net := range nets {
		if net > 0 {
			positives++
			posNet += net
			bestNet = math.Max(bestNet, net)
		}
	}
	summary := sliceSummary{
		count:         len(nets),
		positiveRatio: float64(positives) / float64(len(nets)),
	}
	if posNet > 0 {
		summary.bestSharePct = (bestNet / posNet) * 100
	}
	return summary
}

type yearlySummary struct {
	years                  int
	positiveYears          int
	worstYearPf            *float64
	recentYearNet          *float64
	completedYears         int
	worstCompletedYearPf   *float64
	recentCompletedYear    *int
	recentCompletedYearNet float64
	partialYear            *int
	partialYearNet         *float64
}

// yearlyShape mirrors scripts/lib/yearlyStability.mjs. A year is complete
// when the data runs past 1 January of the following year; the worst year is
// the one with the lowest net, and its PF is reported.
func yearlyShape(rows []YearRow, dataEndT int64) yearlySummary {
	if len(rows) == 0 {
		return yearlySummary{worstYearPf: ptr(0.0), recentYearNet: ptr(0.0), worstCompletedYearPf: ptr(0.0)}
	}
	trailing := rows[len(rows)-1]
	nextYear := time.Date(trailing.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	trailingComplete := dataEndT >= nextYear
	completed := rows
	if !trailingComplete {
		completed = rows[:len(rows)-1]
	}
	summary := yearlySummary{
		years:                len(rows),
		positiveYears:        countPositive(rows),
		worstYearPf:          worstByNet(rows).PF,
		recentYearNet:        ptr(trailing.Net),
		completedYears:       len(completed),
		worstCompletedYearPf: ptr(0.0),
	}
	if len(completed) > 0 {
		recent := completed[len(completed)-1]
		summary.worstCompletedYearPf = worstByNet(completed).PF
		summary.recentCompletedYear = ptr(recent.Year)
		summary.recentCompletedYearNet = recent.Net
	}
	if !trailingComplete {
		summary.partialYear = ptr(trailing.Year)
		summary.partialYearNet = ptr(trailing.Net)
	}
	return summary
}

func countPositive(rows []YearRow) int {
	count := 0
	for _, row := range rows {
		if row.Net > 0 {
			count++
		}
	}
	return count
}

// worstByNet keeps the first of equal-net rows, as the JS reduce does.
func worstByNet(rows []YearRow) YearRow {
	worst := rows[0]
	for _, row := range rows[1:] {
		if row.Net < worst.Net {
			worst = row
		}
	}
	return worst
}

// numberOrZero is `Number(x) || 0` after a JSON round trip: nil, NaN and
// infinities read as 0.
func numberOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return finiteOrZero(*value)
}

func finiteOrZero(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

// jsRound is Math.round: halves round toward +inf.
func jsRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	floor := math.Floor(x)
	if x-floor >= 0.5 {
		return floor + 1
	}
	return floor
}

func ptr[T any](value T) *T {
	return &value
}
