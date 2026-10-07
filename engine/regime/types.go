// Package regime executes the fixed Regime Engine offline interpretation.
// It is deliberately separate from the generic broker: neither Pine parity,
// executable broker quotes nor browser/live adoption is implied.
package regime

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	M5MS       int64 = 300000
	M30MS      int64 = 1800000
	SourceMode       = "source-like-v1"
	AuditMode        = "audit-baseline-v1"
)

// Request supplies a complete historical watermark, not a live-data prefix.
// All three endpoints are UTC M30 boundaries. The M5 candle ending exactly
// at TradeToT must be observed. Earlier context starts exactly at WarmupFromT.
type Request struct {
	Config                            dsl.Config
	M5                                marketdata.Series
	WarmupFromT, TradeFromT, TradeToT int64
	Costs                             Costs
}

type Costs struct {
	Spread         float64 `json:"spread"`
	FeePerUnitSide float64 `json:"feePerUnitSide"`
	InitialEquity  float64 `json:"initialEquity"`
}

type NativeBar struct {
	BucketT        int64   `json:"bucketT"`
	FirstObservedT int64   `json:"firstObservedT"`
	CloseT         int64   `json:"closeT"`
	Open           float64 `json:"open"`
	High           float64 `json:"high"`
	Low            float64 `json:"low"`
	Close          float64 `json:"close"`
	Volume         float64 `json:"volume"`
	Count          int     `json:"count"`
	Complete       bool    `json:"complete"`
}

type IndicatorRow struct {
	NativeBar
	HMA9       *float64 `json:"hma9"`
	HMA21      *float64 `json:"hma21"`
	HMA25      *float64 `json:"hma25"`
	ATR10      *float64 `json:"atr10"`
	ATR14      *float64 `json:"atr14"`
	Supertrend *float64 `json:"supertrend"`
	Regime     int      `json:"regime"`
	VolumeMean *float64 `json:"volumeMean"`
	VolumeOK   bool     `json:"volumeOK"`
	Complete40 bool     `json:"complete40"`
	Cross      int      `json:"cross"`
	Candidate  int      `json:"candidate"`
}

type Signal struct {
	Time        int64   `json:"time"`
	Direction   int     `json:"direction"`
	Anchor      float64 `json:"anchor"`
	ATR         float64 `json:"atr"`
	Supertrend  float64 `json:"supertrend"`
	Regime      int     `json:"regime"`
	Volume      float64 `json:"volume"`
	VolumeRatio float64 `json:"volumeRatio"`
	Accepted    bool    `json:"accepted"`
}

type Position struct {
	Signal                Signal   `json:"signal"`
	EntryBucketT          int64    `json:"entryBucketT"`
	EntryT                int64    `json:"entryT"`
	Entry                 float64  `json:"entry"`
	Quantity              float64  `json:"quantity"`
	EntryFee              float64  `json:"entryFee"`
	AnchorError           float64  `json:"anchorError"`
	NakedEntryBar         bool     `json:"nakedEntryBar"`
	ShadowStopHit         bool     `json:"shadowStopHit"`
	SL                    *float64 `json:"sl"`
	TP                    *float64 `json:"tp"`
	FirstLiveSL           *float64 `json:"firstLiveSL"`
	FirstLiveTP           *float64 `json:"firstLiveTP"`
	FirstLiveT            int64    `json:"firstLiveT"`
	InitialDistance       *float64 `json:"initialDistance"`
	ForcedReason          string   `json:"forcedReason"`
	StopWidenings         int      `json:"stopWidenings"`
	TargetChanges         int      `json:"targetChanges"`
	MarketableTargetEdits int      `json:"marketableTargetEdits"`
	PartialBars           int      `json:"partialBars"`
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
	Time             int64    `json:"time"`
	EntryT           int64    `json:"entryT"`
	Direction        int      `json:"direction"`
	Close            float64  `json:"close"`
	ATR              float64  `json:"atr"`
	Supertrend       float64  `json:"supertrend"`
	OldSL            *float64 `json:"oldSL"`
	OldTP            *float64 `json:"oldTP"`
	NewSL            float64  `json:"newSL"`
	NewTP            float64  `json:"newTP"`
	StopWidened      bool     `json:"stopWidened"`
	TargetChanged    bool     `json:"targetChanged"`
	TargetMarketable bool     `json:"targetMarketable"`
	ForcedReason     string   `json:"forcedReason"`
}

type Summary struct {
	Trades                      int      `json:"trades"`
	Net                         float64  `json:"closedNet"`
	GrossProfit                 float64  `json:"closedGrossProfit"`
	GrossLoss                   float64  `json:"closedGrossLoss"`
	PF                          *float64 `json:"closedPF"`
	EqualUnitPF                 *float64 `json:"closedEqualUnitPF"`
	ClosedEquityDD              float64  `json:"closedEquityDrawdown"`
	WinRate                     *float64 `json:"winRate"`
	NetWithoutBest5             float64  `json:"netWithoutBest5"`
	OpenEntryFee                float64  `json:"openEntryFee"`
	CashEquity                  float64  `json:"cashEquityAfterEntryFees"`
	ClosedEquity                float64  `json:"closedEquity"`
	UnrealizedGross             float64  `json:"unrealizedGross"`
	HypotheticalClosingFee      float64  `json:"hypotheticalClosingFee"`
	MarkedEquity                float64  `json:"markedEquityBeforeHypotheticalClosingFee"`
	MarkedEquityAfterClosingFee float64  `json:"markedEquityAfterHypotheticalClosingFee"`
	FinalMarkT                  int64    `json:"finalMarkT"`
	FinalLiquidationReference   *float64 `json:"finalLiquidationReference"`
	PartialEntryFills           int      `json:"partialEntryFills"`
	PartialPositionBars         int      `json:"partialPositionBars"`
	MarketableTargetEdits       int      `json:"marketableTargetEdits"`
}

type Result struct {
	Symbol             string         `json:"symbol"`
	SourceTimeframe    string         `json:"sourceTimeframe"`
	NativeTimeframe    string         `json:"nativeTimeframe"`
	Schema             string         `json:"schema"`
	Mode               string         `json:"mode"`
	ExecutionModel     string         `json:"executionModel"`
	QuantityModel      string         `json:"quantityModel"`
	CostComplete       bool           `json:"costComplete"`
	PineParityVerified bool           `json:"pineParityVerified"`
	WarmupFromT        int64          `json:"warmupFromT"`
	TradeFromT         int64          `json:"tradeFromT"`
	TradeToT           int64          `json:"tradeToT"`
	UsedM5Rows         int            `json:"usedM5Rows"`
	Costs              Costs          `json:"costs"`
	Assumptions        []string       `json:"assumptions"`
	Indicators         []IndicatorRow `json:"indicators"`
	Signals            []Signal       `json:"signals"`
	Trades             []Trade        `json:"trades"`
	Edits              []OrderEdit    `json:"edits"`
	OpenPosition       *Position      `json:"openPosition"`
	PendingSignal      *Signal        `json:"pendingSignal"`
	Summary            Summary        `json:"summary"`
}
