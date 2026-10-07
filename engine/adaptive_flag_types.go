package engine

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	AdaptiveFlagExecutionSemantics = "delayed-first-stop-activation-v1"
	AdaptiveFlagSourcePineSHA256   = "3102bc71b810eb5559479f6910994d93d02899d90b32f17e1c71a02aa60a4819"
	AdaptiveFlagReferenceSHA256    = "0d99e7162984a28a0f5d63138b6bfba453d8cc6f659edc3aba6bde3cf9a4bcec"
)

// AdaptiveFlagRequest accepts one explicit observed timeframe, with no provider,
// aggregation, economics, or alternate execution-policy defaults.
type AdaptiveFlagRequest struct {
	Config           dsl.Config
	Series           marketdata.Series
	Window           *AdaptiveFlagExecutionWindow
	ResearchAblation AdaptiveFlagResearchAblation
}

// AdaptiveFlagExecutionWindow is evaluation metadata, not a signal setting.
// Row opens in [TradeFromMS, TradeToMS) may create orders; earlier retained
// rows warm indicators while the broker stays flat. The end truncates input.
type AdaptiveFlagExecutionWindow struct {
	TradeFromMS int64 `json:"tradeFromMs"`
	TradeToMS   int64 `json:"tradeToMs"`
}

type AdaptiveFlagBar struct {
	Index  int     `json:"index"`
	OpenT  int64   `json:"openT"`
	CloseT int64   `json:"closeT"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type AdaptiveFlagPivot struct {
	Index           int     `json:"index"`
	ConfirmationIdx int     `json:"confirmationIdx"`
	Price           float64 `json:"price"`
}

type AdaptiveFlagSignal struct {
	SignalIdx                    int     `json:"signalIdx"`
	Side                         string  `json:"side"`
	Trigger                      float64 `json:"trigger"`
	Stop                         float64 `json:"stop"`
	Target                       float64 `json:"target"`
	PlannedTriggerToStopDistance float64 `json:"plannedTriggerToStopDistance"`
}

// AdaptiveFlagSnapshot records causal source predicates on every observed close,
// including closes where existing order state prevents a new signal from arming.
// Candidate means the source predicates pass, not that an order was submitted.
type AdaptiveFlagSnapshot struct {
	AdaptiveFlagBar
	TrueRange       float64             `json:"trueRange"`
	ATR             *float64            `json:"atr"`
	FastEMA         float64             `json:"fastEma"`
	SlowEMA         float64             `json:"slowEma"`
	VolumeSMA       *float64            `json:"volumeSma"`
	ConfirmedHigh   *AdaptiveFlagPivot  `json:"confirmedHigh"`
	ConfirmedLow    *AdaptiveFlagPivot  `json:"confirmedLow"`
	LastHigh        *AdaptiveFlagPivot  `json:"lastHigh"`
	LastLow         *AdaptiveFlagPivot  `json:"lastLow"`
	PoleEndpointIdx int                 `json:"poleEndpointIdx"`
	PoleHigh        float64             `json:"poleHigh"`
	PoleLow         float64             `json:"poleLow"`
	BullHeight      float64             `json:"bullHeight"`
	BearHeight      float64             `json:"bearHeight"`
	BullImpulse     bool                `json:"bullImpulse"`
	BearImpulse     bool                `json:"bearImpulse"`
	SinceHigh       int                 `json:"sinceHigh"`
	FlagBars        int                 `json:"flagBars"`
	FlagStartIdx    int                 `json:"flagStartIdx"`
	FlagHigh        float64             `json:"flagHigh"`
	FlagLow         float64             `json:"flagLow"`
	FlagWidth       float64             `json:"flagWidth"`
	BullRetrace     *float64            `json:"bullRetrace"`
	BearRetrace     *float64            `json:"bearRetrace"`
	TrendBull       bool                `json:"trendBull"`
	TrendBear       bool                `json:"trendBear"`
	VolumeOK        bool                `json:"volumeOk"`
	BullValid       bool                `json:"bullValid"`
	BearValid       bool                `json:"bearValid"`
	Candidate       *AdaptiveFlagSignal `json:"candidate"`
}

// AdaptiveFlagEventTime uses nominal bar-open/close coordinates. Intrabar events
// have only an interval and model ordering; no tick timestamp is manufactured.
type AdaptiveFlagEventTime struct {
	LowerMS        int64  `json:"lowerMs"`
	UpperMS        int64  `json:"upperMs"`
	LowerInclusive bool   `json:"lowerInclusive"`
	UpperInclusive bool   `json:"upperInclusive"`
	Kind           string `json:"kind"`
}

type AdaptiveFlagEvent struct {
	ID      int                   `json:"id"`
	OrderID int                   `json:"orderId"`
	RowIdx  int                   `json:"rowIdx"`
	State   string                `json:"state"`
	Time    AdaptiveFlagEventTime `json:"time"`
	Price   *float64              `json:"price"`
	Basis   string                `json:"basis"`
}

type AdaptiveFlagQueuedExit struct {
	Reason          string `json:"reason"`
	CreationIdx     int    `json:"creationIdx"`
	CreationCloseMS int64  `json:"creationCloseMs"`
	NextOpenIdx     int    `json:"nextOpenIdx"`
}

type AdaptiveFlagOrder struct {
	AdaptiveFlagSignal
	ID                       int      `json:"id"`
	Status                   string   `json:"status"`
	PendingAge               int      `json:"pendingAge"`
	FillOpportunities        int      `json:"fillOpportunities"`
	FillIdx                  *int     `json:"fillIdx"`
	Entry                    *float64 `json:"entry"`
	EntryAtOpen              bool     `json:"entryAtOpen"`
	ActualFillToStopDistance *float64 `json:"actualFillToStopDistance"`
	BracketCreationIdx       *int     `json:"bracketCreationIdx"`
	ExitIdx                  *int     `json:"exitIdx"`
	Exit                     *float64 `json:"exit"`
	Reason                   string   `json:"reason"`
	EventIDs                 []int    `json:"eventIds"`
}

// State is sampled after each observed close. A terminal state is retained as
// pending/open/queued; there is deliberately no end-of-input liquidation.
type AdaptiveFlagState struct {
	RowIdx          int                     `json:"rowIdx"`
	CloseMS         int64                   `json:"closeMs"`
	Status          string                  `json:"status"`
	PendingOrderID  *int                    `json:"pendingOrderId"`
	PositionOrderID *int                    `json:"positionOrderId"`
	PendingAge      int                     `json:"pendingAge"`
	QueuedExit      *AdaptiveFlagQueuedExit `json:"queuedExit"`
}

type AdaptiveFlagGap struct {
	RowIdx       int   `json:"rowIdx"`
	FromMS       int64 `json:"fromMs"`
	ToMS         int64 `json:"toMs"`
	MissingSlots int64 `json:"missingSlots"`
}

type AdaptiveFlagResult struct {
	Schema                  string                       `json:"schema"`
	Policy                  string                       `json:"policy"`
	ExecutionSemantics      string                       `json:"executionSemantics"`
	NumericalPolicy         string                       `json:"numericalPolicy"`
	EvidenceStatus          string                       `json:"evidenceStatus"`
	ArithmeticQualification string                       `json:"arithmeticQualification"`
	SourcePineSHA256        string                       `json:"sourcePineSha256"`
	ReferenceSHA256         string                       `json:"referenceSha256"`
	ConfigSHA256            string                       `json:"configSha256"`
	InputSHA256             string                       `json:"inputSha256"`
	IdentityEncoding        string                       `json:"identityEncoding"`
	EffectiveConfig         dsl.AdaptiveFlagSpec         `json:"effectiveConfig"`
	ExecutionWindow         *AdaptiveFlagExecutionWindow `json:"executionWindow"`
	ProvidedSourceRows      int                          `json:"providedSourceRows"`
	IgnoredSuffixRows       int                          `json:"ignoredSuffixRows"`
	FirstRetainedOpenMS     int64                        `json:"firstRetainedOpenMs"`
	LastRetainedOpenMS      int64                        `json:"lastRetainedOpenMs"`
	LastRetainedCloseMS     int64                        `json:"lastRetainedCloseMs"`
	PreTradeRows            int                          `json:"preTradeRows"`
	EligibleTradeRows       int                          `json:"eligibleTradeRows"`
	UsedSourceRows          int                          `json:"usedSourceRows"`
	TimeframeMS             int64                        `json:"timeframeMs"`
	Snapshots               []AdaptiveFlagSnapshot       `json:"snapshots"`
	Orders                  []AdaptiveFlagOrder          `json:"orders"`
	Events                  []AdaptiveFlagEvent          `json:"events"`
	States                  []AdaptiveFlagState          `json:"states"`
	Gaps                    []AdaptiveFlagGap            `json:"gaps"`
	Terminal                AdaptiveFlagState            `json:"terminal"`
	Assumptions             []string                     `json:"assumptions"`

	ResearchPolicy       *AdaptiveFlagResearchPolicy   `json:"researchPolicy,omitempty"`
	ResearchPolicySHA256 string                        `json:"researchPolicySha256,omitempty"`
	ResearchProducer     *AdaptiveFlagResearchProducer `json:"researchProducer,omitempty"`
}
