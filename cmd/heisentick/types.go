package main

import (
	"encoding/json"

	"github.com/spik3r/heisentick-strat/engine"
)

const reportTradesSchema = "btgo-report-trades-v1"

// sourceEntryReportProvenance is emitted only for an authored source/entry
// route, so ordinary single-timeframe reports retain their existing shape.
type sourceEntryReportProvenance struct {
	SourceTimeframe *string `json:"sourceTimeframe,omitempty"`
	EntryTimeframe  *string `json:"entryTimeframe,omitempty"`
	SourceBars      *int    `json:"sourceBars,omitempty"`
	SourceHTF       *string `json:"sourceHtf,omitempty"`
	SourceHTFBars   *int    `json:"sourceHtfBars,omitempty"`
}

type costRow struct {
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

type sliceRow struct {
	Symbol string  `json:"symbol"`
	TF     string  `json:"tf"`
	Range  string  `json:"range"`
	Bars   int     `json:"bars"`
	HTF    *string `json:"htf"`
	sourceEntryReportProvenance
	Costs []costRow `json:"costs"`
	// Pointer keeps trades absent by default but emits [] for an enabled empty run.
	Trades *[]reportTrade `json:"trades,omitempty"`
}

// reportTrade preserves the conformance trade record while adding stable
// report-only annotations consumed by rich JS report groupings.
type reportTrade struct {
	engine.Trade
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

func (trade reportTrade) MarshalJSON() ([]byte, error) {
	type reportTradeAlias reportTrade
	if !trade.NoTarget {
		return json.Marshal(reportTradeAlias(trade))
	}
	return json.Marshal(struct {
		reportTradeAlias
		InitialTP any `json:"initialTp"`
		TP        any `json:"tp"`
	}{reportTradeAlias: reportTradeAlias(trade), InitialTP: nil, TP: nil})
}

type reportPayload struct {
	Symbols          []string                `json:"symbols"`
	TFs              []string                `json:"tfs"`
	Strategy         string                  `json:"strategy"`
	GeneratedAt      string                  `json:"generatedAt"`
	Range            string                  `json:"range"`
	Bars             int                     `json:"bars"`
	PrimaryCost      primaryCost             `json:"primaryCost"`
	Slices           []sliceRow              `json:"slices"`
	Costs            []costRow               `json:"costs"`
	Warnings         []string                `json:"warnings"`
	ActiveSessions   []string                `json:"activeSessions"`
	StrategyTypeTags reportStrategyTypeTags  `json:"strategyTypeTags"`
	Options          map[string]string       `json:"options,omitempty"`
	TradeSchema      string                  `json:"tradeSchema,omitempty"`
	EvidenceEnvelope *reportEvidenceEnvelope `json:"evidenceEnvelope,omitempty"`
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

type primaryCost struct {
	Index int    `json:"index"`
	Label string `json:"label"`
}

type gridVariant struct {
	Index  int                `json:"index"`
	Params map[string]float64 `json:"params"`
	Costs  []costRow          `json:"costs"`
}

type gridPayload struct {
	Symbols  []string             `json:"symbols"`
	TFs      []string             `json:"tfs"`
	Strategy string               `json:"strategy"`
	Range    string               `json:"range"`
	Bars     int                  `json:"bars"`
	HTF      *string              `json:"htf,omitempty"`
	Sets     map[string][]float64 `json:"sets"`
	Variants []gridVariant        `json:"variants"`
	Warnings []string             `json:"warnings"`
	Options  map[string]string    `json:"options,omitempty"`
}
