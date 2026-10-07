// Package adaptiveflagunit freezes B0 unit-report shapes and resource limits.
// It deliberately contains no projector, admission parser, strategy call or transport.
package adaptiveflagunit

import (
	"encoding/json"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

const (
	RequestSchema         = "adaptive-flag-unit-request-v1"
	EnvelopeSchema        = "strat-adaptive-volume-flag-unit-runtime-v1"
	ProjectionSchema      = "adaptive-unit-projection-go-v1"
	ErrorSchema           = "adaptive-flag-unit-error-v1"
	ReferenceSourceSHA256 = "76c58c46f80217baf68fbe4071077b61134a78bde1b12b9ee3439a0de51eadce"
	ReferenceCommonSHA256 = "0bbfb21c5ab6a121215ba5ec250b3f2fcbabed3a0cf357387a93fb538877bba8"
)

type DataSource struct {
	ID           string `json:"id" unitmax:"128"`
	SourceSHA256 string `json:"sourceSha256" unitmax:"64"`
}
type Request struct {
	Schema          string     `json:"schema" unitmax:"64"`
	Scenario        string     `json:"scenario" unitmax:"32"`
	NumericalPolicy string     `json:"numericalPolicy" unitmax:"32"`
	CostPolicy      string     `json:"costPolicy" unitmax:"32"`
	DataSource      DataSource `json:"dataSource"`
}
type CostPolicy struct {
	Name       string   `json:"name" unitmax:"32"`
	PerFill    float64  `json:"per_fill"`
	Spread     *float64 `json:"spread"`
	Commission *float64 `json:"commission"`
	Aggregate  *float64 `json:"aggregate"`
}
type EconomicPolicy struct {
	Schema        string     `json:"schema" unitmax:"64"`
	Scenario      string     `json:"scenario" unitmax:"32"`
	PointValue    float64    `json:"pointValue"`
	PlannedRiskU  float64    `json:"plannedRiskU"`
	CostPolicy    CostPolicy `json:"costPolicy"`
	FundingStatus string     `json:"fundingStatus" unitmax:"32"`
}
type NumericalPolicy struct {
	Schema      string `json:"schema" unitmax:"64"`
	Policy      string `json:"policy" unitmax:"32"`
	Sums        string `json:"sums" unitmax:"64"`
	Operations  string `json:"operations" unitmax:"64"`
	Comparisons string `json:"comparisons" unitmax:"32"`
}
type BuildIdentity struct {
	GoVersion   string `json:"goVersion" unitmax:"64"`
	Compiler    string `json:"compiler" unitmax:"64"`
	GOOS        string `json:"goos" unitmax:"16"`
	GOARCH      string `json:"goarch" unitmax:"16"`
	VCSRevision string `json:"vcsRevision" unitmax:"40"`
	VCSModified *bool  `json:"vcsModified"`
	Verified    bool   `json:"verified"`
}
type EffectiveWindow struct {
	TradeFromMS int64  `json:"tradeFromMs"`
	TradeToMS   int64  `json:"tradeToMs"`
	Basis       string `json:"basis" unitmax:"128"`
}
type Manifest struct {
	ContractID                 string               `json:"contractId" unitmax:"64"`
	ReferenceSourceSHA256      string               `json:"referenceSourceSha256" unitmax:"64"`
	ReferenceCommonSHA256      string               `json:"referenceCommonSha256" unitmax:"64"`
	Scenario                   string               `json:"scenario" unitmax:"32"`
	Units                      string               `json:"units" unitmax:"128"`
	PointValue                 float64              `json:"pointValue"`
	PlannedRiskU               float64              `json:"plannedRiskU"`
	StartingCumulativePnLU     float64              `json:"startingCumulativePnlU"`
	NumericalPolicy            string               `json:"numericalPolicy" unitmax:"32"`
	CostPolicy                 CostPolicy           `json:"costPolicy"`
	EconomicPolicy             EconomicPolicy       `json:"economicPolicy"`
	GoEconomicPolicySHA256     string               `json:"goEconomicPolicySha256" unitmax:"64"`
	NumericalDefinition        NumericalPolicy      `json:"numericalDefinition"`
	GoNumericalPolicySHA256    string               `json:"goNumericalPolicySha256" unitmax:"64"`
	FundingStatus              string               `json:"fundingStatus" unitmax:"32"`
	FundingCashU               *float64             `json:"fundingCashU"`
	EconomicsQualification     string               `json:"economicsQualification" unitmax:"192"`
	RawEnvelopeSHA256          string               `json:"rawEnvelopeSha256" unitmax:"64"`
	RawInputSHA256             string               `json:"rawInputSha256" unitmax:"64"`
	RawConfigSHA256            string               `json:"rawConfigSha256" unitmax:"64"`
	SourcePineSHA256           string               `json:"sourcePineSha256" unitmax:"64"`
	DSLSHA256                  string               `json:"dslSha256" unitmax:"64"`
	CompiledConfigSHA256       string               `json:"compiledConfigSha256" unitmax:"64"`
	BTB1SHA256                 string               `json:"btb1Sha256" unitmax:"64"`
	EffectiveConfig            dsl.AdaptiveFlagSpec `json:"effectiveConfig"`
	ExecutionSemantics         string               `json:"executionSemantics" unitmax:"64"`
	ExecutionWindow            EffectiveWindow      `json:"executionWindow"`
	WarmupRowsExcluded         int                  `json:"warmupRowsExcludedFromAccounting"`
	DataSource                 DataSource           `json:"dataSource"`
	DataSourceStatus           string               `json:"dataSourceStatus" unitmax:"32"`
	BuildIdentity              BuildIdentity        `json:"buildIdentity"`
	RegistryEnabled            bool                 `json:"registryEnabled"`
	CostsNeverModifyRawPhysics bool                 `json:"costsNeverModifyRawPhysics"`
	LiquidationIsHypothetical  bool                 `json:"liquidationIsHypothetical"`
}

type ProjectedOrder struct {
	OrderID          int      `json:"order_id"`
	EpisodeID        string   `json:"episode_id" unitmax:"64"`
	CandidateID      string   `json:"candidate_id" unitmax:"64"`
	Side             string   `json:"side" unitmax:"5"`
	CreationRowIndex int      `json:"creation_row_index"`
	RawTrigger       float64  `json:"raw_trigger"`
	FrozenStop       float64  `json:"frozen_stop"`
	FrozenTarget     float64  `json:"frozen_target"`
	PlannedDistance  float64  `json:"planned_distance"`
	Quantity         *float64 `json:"quantity"`
	PlannedRiskU     *float64 `json:"planned_risk_U"`
	RejectionReason  *string  `json:"rejection_reason" unitmax:"20"`
	// Omitted before a fill. Double pointers distinguish omitted from emitted null.
	RawEntry                  *float64  `json:"raw_entry,omitempty"`
	FillRiskRawU              *float64  `json:"fill_risk_raw_U,omitempty"`
	DisplayProxyAdjustedEntry **float64 `json:"display_proxy_adjusted_entry,omitempty"`
	FillRiskEffectiveU        **float64 `json:"fill_risk_effective_U,omitempty"`
}
type InvalidRiskRejection struct {
	OrderID int    `json:"order_id"`
	Reason  string `json:"reason" unitmax:"20" unitliteral:"invalid_planned_risk"`
}
type CostEvent struct {
	OrderID                int                          `json:"order_id"`
	EventID                int                          `json:"event_id"`
	Leg                    string                       `json:"leg" unitmax:"5"`
	RowIndex               int                          `json:"row_index"`
	Time                   engine.AdaptiveFlagEventTime `json:"time"`
	TotalU                 float64                      `json:"total_U"`
	SpreadProxyU           *float64                     `json:"spread_proxy_U"`
	CommissionProxyU       *float64                     `json:"commission_proxy_U"`
	UnattributedAggregateU *float64                     `json:"unattributed_aggregate_U"`
}
type ExposureGap struct {
	FromMS     int64 `json:"from_ms"`
	ToMS       int64 `json:"to_ms"`
	DurationMS int64 `json:"duration_ms"`
}
type Exposure struct {
	EntryTimeBounds               engine.AdaptiveFlagEventTime `json:"entry_time_bounds"`
	EndpointTimeBounds            engine.AdaptiveFlagEventTime `json:"endpoint_time_bounds"`
	EntryBarOpenMS                int64                        `json:"entry_bar_open_ms"`
	EndpointBarOpenMS             int64                        `json:"endpoint_bar_open_ms"`
	ElapsedMSLower                int64                        `json:"elapsed_ms_lower"`
	ElapsedMSUpper                int64                        `json:"elapsed_ms_upper"`
	ObservedBarsWithExposure      int                          `json:"observed_bars_with_exposure"`
	QuoteGapsBridged              []ExposureGap                `json:"quote_gaps_bridged"`
	UTCMidnightsDefinitelyCrossed []int64                      `json:"utc_midnights_definitely_crossed"`
	UTCMidnightsPossiblyCrossed   []int64                      `json:"utc_midnights_possibly_crossed"`
	RolloverLabel                 string                       `json:"rollover_label" unitmax:"64"`
	IsOpenPosition                bool                         `json:"is_open_position"`
}
type ClosedTrade struct {
	ProjectedOrder
	RawExit                  float64                      `json:"raw_exit"`
	Reason                   string                       `json:"reason" unitmax:"32"`
	GrossU                   float64                      `json:"gross_U"`
	EntryModelCostU          float64                      `json:"entry_model_cost_U"`
	ExitModelCostU           float64                      `json:"exit_model_cost_U"`
	NetU                     float64                      `json:"net_U"`
	NetR                     float64                      `json:"net_R"`
	DisplayProxyAdjustedExit *float64                     `json:"display_proxy_adjusted_exit"`
	EntryEventID             int                          `json:"entry_event_id"`
	ExitEventID              int                          `json:"exit_event_id"`
	EntryEventTime           engine.AdaptiveFlagEventTime `json:"entry_event_time"`
	ExitEventTime            engine.AdaptiveFlagEventTime `json:"exit_event_time"`
	EntryPhase               string                       `json:"entry_phase" unitmax:"12"`
	ExitPhase                string                       `json:"exit_phase" unitmax:"12"`
	EntryYearCohort          string                       `json:"entry_year_cohort" unitmax:"4"`
	Exposure                 Exposure                     `json:"exposure"`
}
type Summary struct {
	ClosedTrades         int      `json:"closed_trades"`
	Wins                 int      `json:"wins"`
	Losses               int      `json:"losses"`
	ZeroNetTrades        int      `json:"zero_net_trades"`
	NetR                 float64  `json:"net_R"`
	MeanNetR             *float64 `json:"mean_net_R"`
	PositiveNetR         float64  `json:"positive_net_R"`
	AbsoluteNegativeNetR float64  `json:"absolute_negative_net_R"`
	ProfitFactor         *float64 `json:"profit_factor"`
	ProfitFactorStatus   string   `json:"profit_factor_status" unitmax:"27"`
}
type CalendarYear struct {
	CalendarYearMarkedChangeR float64 `json:"calendar_year_marked_change_R"`
}
type Mark struct {
	RowIndex                   int                            `json:"row_index"`
	BarOpenMS                  int64                          `json:"bar_open_ms"`
	NominalCloseMS             int64                          `json:"nominal_close_ms"`
	RawClose                   float64                        `json:"raw_close"`
	GrossRealizedClosedU       float64                        `json:"gross_realized_closed_U"`
	ModeledExecutionCostsPaidU float64                        `json:"modeled_execution_costs_paid_U"`
	OpenRawGrossU              float64                        `json:"open_raw_gross_U"`
	OpenEntryModelCostU        float64                        `json:"open_entry_model_cost_U"`
	ClosedTradeNetU            float64                        `json:"closed_trade_net_U"`
	MarkedPnLU                 float64                        `json:"marked_pnl_U"`
	MarkedPnLR                 float64                        `json:"marked_pnl_R"`
	ReconciliationDeltaU       float64                        `json:"reconciliation_delta_U"`
	CloseMarkedDrawdownR       float64                        `json:"close_marked_drawdown_R"`
	OpenOrderID                *int                           `json:"open_order_id"`
	PendingOrderID             *int                           `json:"pending_order_id"`
	QueuedExit                 *engine.AdaptiveFlagQueuedExit `json:"queued_exit"`
}
type Daily struct {
	DateUTC            string  `json:"date_utc" unitmax:"10"`
	AsOfMS             int64   `json:"as_of_ms"`
	MarkedPnLR         float64 `json:"marked_pnl_R"`
	DailyMarkedChangeR float64 `json:"daily_marked_change_R"`
	CarriedNoQuote     bool    `json:"carried_no_quote"`
	LastQuoteCloseMS   *int64  `json:"last_quote_close_ms"`
	QuoteGapDurationMS int64   `json:"quote_gap_duration_ms"`
}
type Terminal struct {
	NativeState                    engine.AdaptiveFlagState `json:"native_state"`
	MarkedPnLR                     float64                  `json:"marked_pnl_R"`
	HypotheticalLiquidationPnLR    float64                  `json:"hypothetical_liquidation_pnl_R"`
	RemainingHypotheticalExitCostU float64                  `json:"remaining_hypothetical_exit_cost_U"`
	OpenPosition                   *ProjectedOrder          `json:"open_position"`
	Exposure                       *Exposure                `json:"exposure"`
	ActualClosedTradeCount         int                      `json:"actual_closed_trade_count"`
}
type Projection struct {
	Schema                       string                  `json:"schema" unitmax:"64"`
	SourceAuthority              string                  `json:"source_authority" unitmax:"64"`
	Manifest                     Manifest                `json:"manifest"`
	Orders                       []ProjectedOrder        `json:"orders"`
	InvalidPlannedRiskRejections []InvalidRiskRejection  `json:"invalid_planned_risk_rejections"`
	CostEvents                   []CostEvent             `json:"cost_events"`
	ClosedTrades                 []ClosedTrade           `json:"closed_trades"`
	Summary                      Summary                 `json:"summary"`
	ByEntryYearCohort            map[string]Summary      `json:"by_entry_year_cohort"`
	ByCalendarYearMarkedChange   map[string]CalendarYear `json:"by_calendar_year_marked_change"`
	Marks                        []Mark                  `json:"marks"`
	Daily                        []Daily                 `json:"daily"`
	CloseMarkedMaxDrawdownR      float64                 `json:"close_marked_max_drawdown_R"`
	ClosedTradeMaxDrawdownR      float64                 `json:"closed_trade_max_drawdown_R"`
	Terminal                     Terminal                `json:"terminal"`
}

// Envelope is a schema/shape declaration only. B1 must compose the owned Raw
// byte segment verbatim; encoding/json Marshal of this declaration is NOT that
// implementation, because RawMessage marshaling would compact the raw segment.
type Envelope struct {
	Schema                  string          `json:"schema" unitmax:"64"`
	ProjectionRequest       Request         `json:"projectionRequest"`
	ProjectionRequestSHA256 string          `json:"projectionRequestSha256" unitmax:"64"`
	RawEnvelopeSHA256       string          `json:"rawEnvelopeSha256" unitmax:"64"`
	Raw                     json.RawMessage `json:"raw"`
	Projection              Projection      `json:"projection"`
}
type ErrorLocation struct {
	Operation *string `json:"operation" unitmax:"48"`
	RowIndex  *int    `json:"rowIndex"`
	OrderID   *int    `json:"orderId"`
	EventID   *int    `json:"eventId"`
}
type ErrorDetail struct {
	Code     string        `json:"code" unitmax:"32"`
	Phase    string        `json:"phase" unitmax:"10"`
	Message  string        `json:"message" unitmax:"1024"`
	Location ErrorLocation `json:"location"`
}
type ErrorEnvelope struct {
	Schema string      `json:"schema" unitmax:"64"`
	Error  ErrorDetail `json:"error"`
}
