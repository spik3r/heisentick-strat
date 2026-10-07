// Package master executes the fixed v10 Master Structural offline reference.
// It is a separate candidate from regime/v9, not Pine or broker parity.
package master

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/regime"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	M5MS                = regime.M5MS
	M30MS               = regime.M30MS
	H4MS          int64 = 14400000
	SourceMode          = dsl.MasterStructuralSourceHistoricalReference
	ProtectedMode       = dsl.MasterStructuralProtectedStableReference
)

type Request struct {
	Config                            dsl.Config
	M5                                marketdata.Series
	WarmupFromT, TradeFromT, TradeToT int64
	Costs                             Costs
}
type Costs = regime.Costs
type NativeBar = regime.NativeBar

type H4Row struct {
	NativeBar
	M30Count   int      `json:"m30Count"`
	ATR10      *float64 `json:"atr10"`
	Supertrend *float64 `json:"supertrend"`
	Regime     int      `json:"regime"`
}
type H4Snapshot struct {
	AvailableT int64    `json:"availableT"`
	Regime     int      `json:"regime"`
	Supertrend *float64 `json:"supertrend"`
}
type IndicatorRow struct {
	regime.IndicatorRow
	ATRPercent         *float64    `json:"atrPercent"`
	HistoricalH4       *H4Snapshot `json:"historicalH4"`
	StableH4           *H4Snapshot `json:"stableH4"`
	SourceCandidate    int         `json:"sourceCandidate"`
	ProtectedCandidate int         `json:"protectedCandidate"`
}
type Signal struct {
	regime.Signal
	High       float64    `json:"high"`
	Low        float64    `json:"low"`
	H4         H4Snapshot `json:"h4"`
	ATRPercent float64    `json:"atrPercent"`
}
type Position struct {
	Signal                      Signal   `json:"signal"`
	EntryBucketT                int64    `json:"entryBucketT"`
	EntryT                      int64    `json:"entryT"`
	Entry                       float64  `json:"entry"`
	Quantity                    float64  `json:"quantity"`
	EntryFee                    float64  `json:"entryFee"`
	AnchorError                 float64  `json:"anchorError"`
	NakedEntryBar               bool     `json:"nakedEntryBar"`
	SL                          *float64 `json:"sl"`
	TP                          *float64 `json:"tp"`
	FirstLiveSL                 *float64 `json:"firstLiveSL"`
	FirstLiveTP                 *float64 `json:"firstLiveTP"`
	FirstLiveT                  int64    `json:"firstLiveT"`
	InitialDistance             *float64 `json:"initialDistance"`
	ForcedReason                string   `json:"forcedReason"`
	StopWidenings               int      `json:"stopWidenings"`
	TargetChanges               int      `json:"targetChanges"`
	MarketableTargetEdits       int      `json:"marketableTargetEdits"`
	PartialBars                 int      `json:"partialBars"`
	MaxHigh                     float64  `json:"maxHigh"`
	MinLow                      float64  `json:"minLow"`
	PostHigh                    *float64 `json:"postHigh"`
	PostLow                     *float64 `json:"postLow"`
	Locked                      bool     `json:"locked"`
	LockTransitions             int      `json:"lockTransitions"`
	LockDeactivations           int      `json:"lockDeactivations"`
	HypotheticalLockRelaxations int      `json:"hypotheticalLockRelaxations"`
	RejectedWorseST             int      `json:"rejectedWorseST"`
	PreentryOnlyActivations     int      `json:"preentryOnlyActivations"`
	NonprofitLockEdits          int      `json:"nonprofitLockEdits"`
}
type Trade struct {
	Position
	ExitT        int64   `json:"exitT"`
	Exit         float64 `json:"exit"`
	Reason       string  `json:"reason"`
	GrossPerUnit float64 `json:"grossPerUnit"`
	NetPerUnit   float64 `json:"netPerUnit"`
	Net          float64 `json:"net"`
	ExitFee      float64 `json:"exitFee"`
	EquityAfter  float64 `json:"equityAfter"`
}
type OrderEdit struct {
	regime.OrderEdit
	Entry                      float64 `json:"entry"`
	Anchor                     float64 `json:"anchor"`
	UnlockedCandidate          float64 `json:"unlockedCandidate"`
	PreviousLocked             bool    `json:"previousLocked"`
	Locked                     bool    `json:"locked"`
	BecameLocked               bool    `json:"becameLocked"`
	Deactivated                bool    `json:"deactivated"`
	HypotheticalLockRelaxation bool    `json:"hypotheticalLockRelaxation"`
	RejectedWorseST            bool    `json:"rejectedWorseST"`
	PreentryOnlyActivation     bool    `json:"preentryOnlyActivation"`
	PostHigh                   float64 `json:"postHigh"`
	PostLow                    float64 `json:"postLow"`
}
type Summary = regime.Summary
type MonthlySummary struct {
	EntryMonth  string   `json:"entryMonth"`
	Trades      int      `json:"trades"`
	Net         float64  `json:"closedNet"`
	GrossProfit float64  `json:"closedGrossProfit"`
	GrossLoss   float64  `json:"closedGrossLoss"`
	PF          *float64 `json:"closedPF"`
	EqualUnitPF *float64 `json:"closedEqualUnitPF"`
}
type Result struct {
	ArithmeticContract string           `json:"arithmeticContract,omitempty"`
	Symbol             string           `json:"symbol"`
	SourceTimeframe    string           `json:"sourceTimeframe"`
	NativeTimeframe    string           `json:"nativeTimeframe"`
	Schema             string           `json:"schema"`
	Mode               string           `json:"mode"`
	ExecutionModel     string           `json:"executionModel"`
	QuantityModel      string           `json:"quantityModel"`
	PriceModel         string           `json:"priceModel"`
	CostComplete       bool             `json:"costComplete"`
	PineParityVerified bool             `json:"pineParityVerified"`
	WarmupFromT        int64            `json:"warmupFromT"`
	TradeFromT         int64            `json:"tradeFromT"`
	TradeToT           int64            `json:"tradeToT"`
	UsedM5Rows         int              `json:"usedM5Rows"`
	Costs              Costs            `json:"costs"`
	Assumptions        []string         `json:"assumptions"`
	Indicators         []IndicatorRow   `json:"indicators"`
	H4                 []H4Row          `json:"h4"`
	Signals            []Signal         `json:"signals"`
	Trades             []Trade          `json:"trades"`
	Edits              []OrderEdit      `json:"edits"`
	OpenPosition       *Position        `json:"openPosition"`
	PendingSignal      *Signal          `json:"pendingSignal"`
	Summary            Summary          `json:"summary"`
	Monthly            []MonthlySummary `json:"monthlyByEntry"`
}
