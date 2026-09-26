// Package report builds the strategy report document the heisentick CLI
// prints and the report Lambda stores: cost rows, the opt-in annotated trade
// list, the evidence envelope, and the M2 statistics (headline, groupings,
// date bounds, holdout split).
//
// The engine can prepare context bars before a separate inclusive tradable
// interval, then closes an open position at that interval's final bar (exit
// reason "end-of-test"). It models slippage only, with fills on the close and a
// start equity of 10000.
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spik3r/heisentick-strat/engine"
)

// TradesSchema names the annotated trade record emitted with IncludeTrades.
const TradesSchema = "btgo-report-trades-v1"

// WriteJSON writes a document (or any payload) as two-space indented JSON
// followed by a newline: the CLI's output encoding.
func WriteJSON(out io.Writer, payload any) error {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", encoded)
	return err
}

// sourceEntryReportProvenance is emitted only for an authored source/entry
// route, so ordinary single-timeframe reports retain their existing shape.
type sourceEntryReportProvenance struct {
	SourceTimeframe *string `json:"sourceTimeframe,omitempty"`
	EntryTimeframe  *string `json:"entryTimeframe,omitempty"`
	SourceBars      *int    `json:"sourceBars,omitempty"`
	SourceHTF       *string `json:"sourceHtf,omitempty"`
	SourceHTFBars   *int    `json:"sourceHtfBars,omitempty"`
}

// CostRow is the summary of one cost mode. Net and Expectancy are in the
// account currency of the engine's pnl; WinRate is a percentage; DD is the
// largest peak-to-trough equity drop as a percentage of the final peak.
type CostRow struct {
	Label       string   `json:"label"`
	Slippage    float64  `json:"slippage"`
	SlippageBps float64  `json:"slippageBps,omitempty"`
	Trades      int      `json:"trades"`
	WinRate     float64  `json:"winRate"`
	PF          *float64 `json:"pf"`
	Net         float64  `json:"net"`
	Expectancy  float64  `json:"expectancy"`
	DD          float64  `json:"dd"`
}

// Slice is one route's rows and, when requested, its annotated trades.
type Slice struct {
	Symbol string  `json:"symbol"`
	TF     string  `json:"tf"`
	Range  string  `json:"range"`
	Bars   int     `json:"bars"`
	HTF    *string `json:"htf"`
	sourceEntryReportProvenance
	Costs []CostRow `json:"costs"`
	// Pointer keeps trades absent by default but emits [] for an enabled empty run.
	Trades *[]Trade `json:"trades,omitempty"`
}

// Trade preserves the conformance trade record while adding stable
// report-only annotations consumed by rich JS report groupings.
type Trade struct {
	engine.Trade
	// SignalID identifies the closed decision bar that produced this trade's
	// entry (ComputeSignalID); empty when the caller supplied no strategy
	// identity (only annotateReportTrades' unit tests today).
	SignalID string `json:"signalId,omitempty"`
	// Route identity mirrors the report-only fields attached by the JS report
	// before it combines primary-cost trades from separate routes.
	ReportSymbol          string   `json:"reportSymbol"`
	ReportTF              string   `json:"reportTf"`
	ReportRange           string   `json:"reportRange"`
	ReportSlice           string   `json:"reportSlice"`
	ReportSessionPhase    string   `json:"reportSessionPhase"`
	ReportPreviousOutcome string   `json:"reportPreviousOutcome"`
	ReportOpenLocation    string   `json:"reportOpenLocation"`
	ReportPriorDayType    string   `json:"reportPriorDayType"`
	ReportLocalHour       int      `json:"reportLocalHour"`
	ReportLocalWeekday    string   `json:"reportLocalWeekday"`
	ReportEntryMonth      string   `json:"reportEntryMonth"`
	ReportMfeR            *float64 `json:"reportMfeR,omitempty"`
	ReportMaeR            *float64 `json:"reportMaeR,omitempty"`
	ReportTimeToMfeBars   *int     `json:"reportTimeToMfeBars,omitempty"`
	ReportPostExitMfeR    *float64 `json:"reportPostExitMfeR,omitempty"`
	ReportPostExitMaeR    *float64 `json:"reportPostExitMaeR,omitempty"`
}

func (trade Trade) MarshalJSON() ([]byte, error) {
	type reportTradeAlias Trade
	type noTargetTrade struct {
		reportTradeAlias
		InitialTP any `json:"initialTp"`
		TP        any `json:"tp"`
	}
	type noStopTrade struct {
		reportTradeAlias
		InitialSL any `json:"initialSl"`
		SL        any `json:"sl"`
	}
	type noBracketTrade struct {
		reportTradeAlias
		InitialSL any `json:"initialSl"`
		InitialTP any `json:"initialTp"`
		SL        any `json:"sl"`
		TP        any `json:"tp"`
	}
	switch {
	case trade.NoStop && trade.NoTarget:
		return json.Marshal(noBracketTrade{reportTradeAlias: reportTradeAlias(trade)})
	case trade.NoStop:
		return json.Marshal(noStopTrade{reportTradeAlias: reportTradeAlias(trade)})
	case trade.NoTarget:
		return json.Marshal(noTargetTrade{reportTradeAlias: reportTradeAlias(trade)})
	default:
		return json.Marshal(reportTradeAlias(trade))
	}
}

// Document is the report. The fields through EvidenceEnvelope are the CLI's
// established JSON payload; Headline, Groupings, DateBounds and Holdout are
// additive and omitted when nil (the CLI emits them behind --evidence=1).
type Document struct {
	Symbols          []string                `json:"symbols"`
	TFs              []string                `json:"tfs"`
	Strategy         string                  `json:"strategy"`
	GeneratedAt      string                  `json:"generatedAt"`
	Range            string                  `json:"range"`
	Bars             int                     `json:"bars"`
	PrimaryCost      PrimaryCost             `json:"primaryCost"`
	Slices           []Slice                 `json:"slices"`
	Costs            []CostRow               `json:"costs"`
	Warnings         []string                `json:"warnings"`
	ActiveSessions   []string                `json:"activeSessions"`
	StrategyTypeTags reportStrategyTypeTags  `json:"strategyTypeTags"`
	Options          map[string]string       `json:"options,omitempty"`
	TradeSchema      string                  `json:"tradeSchema,omitempty"`
	EvidenceEnvelope *reportEvidenceEnvelope `json:"evidenceEnvelope,omitempty"`
	// Headline summarises the primary cost row and its trades.
	Headline *Headline `json:"headline,omitempty"`
	// Groupings splits the primary cost's trades by session, side, exit
	// reason and entry year.
	Groupings *Groupings `json:"groupings,omitempty"`
	// DateBounds is the span of tradable entry bars the engine ran. Context bars
	// supplied before the execution window are excluded.
	DateBounds *DateBounds `json:"dateBounds,omitempty"`
	// Holdout is set only when the request named a holdout boundary.
	Holdout *Holdout `json:"holdout,omitempty"`
}

// reportEvidenceEnvelope is the single-route core shared with the JS
// strategy-report artifact. Trade-level groupings remain intentionally outside
// this bounded parity slice.
type reportEvidenceEnvelope struct {
	Schema        string                    `json:"schema"`
	SchemaVersion int                       `json:"schemaVersion"`
	Artifact      string                    `json:"artifact"`
	Source        string                    `json:"source"`
	GeneratedAt   string                    `json:"generatedAt"`
	Strategy      reportEvidenceStrategy    `json:"strategy"`
	Route         reportEvidenceRoute       `json:"route"`
	Costs         reportEvidenceCosts       `json:"costs"`
	Summary       reportEvidenceSummary     `json:"summary"`
	Diagnostics   reportEvidenceDiagnostics `json:"diagnostics"`
	Commands      any                       `json:"commands"`
	Checks        any                       `json:"checks"`
	Aggregate     reportEvidenceAggregate   `json:"aggregate"`
	Rows          []reportEvidenceRow       `json:"rows"`
}

type reportEvidenceStrategy struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Params any    `json:"params"`
}

type reportEvidenceRoute struct {
	Symbol       string   `json:"symbol"`
	TF           string   `json:"tf"`
	Symbols      []string `json:"symbols"`
	TFs          []string `json:"tfs"`
	RangeMethod  string   `json:"rangeMethod"`
	RangeRequest string   `json:"rangeRequest"`
	RouteMode    string   `json:"routeMode"`
	ForceRoute   bool     `json:"forceRoute"`
	RouteState   any      `json:"routeState"`
}

type reportEvidenceCosts struct {
	Label            string   `json:"label"`
	Slippage         float64  `json:"slippage"`
	SlippageBps      float64  `json:"slippageBps,omitempty"`
	Commission       float64  `json:"commission"`
	PrimaryCostIndex int      `json:"primaryCostIndex"`
	PrimaryCostLabel string   `json:"primaryCostLabel"`
	CostLabels       []string `json:"costLabels"`
}

type reportEvidenceSummary struct {
	Trades         int      `json:"trades"`
	WinRatePct     float64  `json:"winRatePct"`
	ProfitFactor   *float64 `json:"profitFactor"`
	NetPnl         float64  `json:"netPnl"`
	ExpectancyPnl  float64  `json:"expectancyPnl"`
	MaxDrawdownPnl any      `json:"maxDrawdownPnl"`
	MaxDrawdownPct float64  `json:"maxDrawdownPct"`
	ReturnPct      any      `json:"returnPct"`
}

type reportEvidenceDiagnostics struct {
	Warnings         []string               `json:"warnings"`
	FilterSummary    any                    `json:"filterSummary"`
	StrategyTypeTags reportStrategyTypeTags `json:"strategyTypeTags"`
}

type reportStrategyTypeTags struct {
	SessionBiasFilterCount int `json:"sessionBiasFilterCount"`
	SeasonalityFilterCount int `json:"seasonalityFilterCount"`
	VPAFilterCount         int `json:"vpaFilterCount"`
}

type reportEvidenceAggregate struct {
	Kind                  string   `json:"kind"`
	RouteSliceCount       int      `json:"routeSliceCount"`
	ActiveRouteSliceCount int      `json:"activeRouteSliceCount"`
	Bars                  int      `json:"bars"`
	Symbols               []string `json:"symbols"`
	TFs                   []string `json:"tfs"`
	RangeMethods          []string `json:"rangeMethods"`
	CostLabels            []string `json:"costLabels"`
	PrimaryCostIndex      int      `json:"primaryCostIndex"`
	PrimaryCostLabel      string   `json:"primaryCostLabel"`
}

type reportEvidenceRow struct {
	Symbol      string  `json:"symbol"`
	TF          string  `json:"tf"`
	RangeMethod string  `json:"rangeMethod"`
	Bars        int     `json:"bars"`
	HTF         *string `json:"htf"`
	sourceEntryReportProvenance
	Status  string                 `json:"status"`
	Costs   reportEvidenceRowCosts `json:"costs"`
	Summary reportEvidenceSummary  `json:"summary"`
}

type reportEvidenceRowCosts struct {
	Label       string  `json:"label"`
	Slippage    float64 `json:"slippage"`
	SlippageBps float64 `json:"slippageBps,omitempty"`
	Commission  float64 `json:"commission"`
}

// PrimaryCost names the cost row the headline, trade list and envelope use.
type PrimaryCost struct {
	Index int    `json:"index"`
	Label string `json:"label"`
}
