// Package adaptivestudy is an assumed retrospective OHLC study, not Pine parity.
package adaptivestudy

import "github.com/spik3r/heisentick-strat/engine/regimeadaptive"

const Profile = "regime-adaptive-v4.2-go-flat-risk-study-v1"
const M30Hash = "a04fad3f6daa2b53addcd4b8b6fe6ba9c49afd329b7785d167724763d628d92f"
const M5Hash = "1be64a5005f93bd8c70d2a61d408928a803537b9eddbd386d9ac53d33285e9ca"
const Duration int64 = 1800000

type Bar = regimeadaptive.Bar
type Config struct {
	Candidate string `json:"candidate"`
	Cost      string `json:"cost"`
	Stress    string `json:"stress"`
	StartT    int64  `json:"startT"`
	EndT      int64  `json:"endT"`
}
type Order struct {
	ID           string  `json:"id"`
	Side         int     `json:"side"`
	Branch       string  `json:"branch"`
	SignalBarT   int64   `json:"signalBarT"`
	SignalCloseT int64   `json:"signalCloseT"`
	SignalClose  float64 `json:"signalClose"`
	SignalEquity float64 `json:"signalEquity"`
	ATR          float64 `json:"atr"`
	Basis        float64 `json:"basis"`
	Qty          float64 `json:"qty"`
	PlannedRisk  float64 `json:"plannedRisk"`
}
type Position struct {
	Order     Order   `json:"order"`
	EntryBarT int64   `json:"entryBarT"`
	Entry     float64 `json:"entry"`
	Stop      float64 `json:"stop"`
	Target    float64 `json:"target"`
	EntryFee  float64 `json:"entryFee"`
}
type Trade struct {
	Position  Position `json:"position"`
	ExitBarT  int64    `json:"exitBarT"`
	ExitStage string   `json:"exitStage"`
	IntrabarT *int64   `json:"intrabarT"`
	Exit      float64  `json:"exit"`
	Reason    string   `json:"reason"`
	Gross     float64  `json:"gross"`
	ExitFee   float64  `json:"exitFee"`
	Fees      float64  `json:"fees"`
	Net       float64  `json:"net"`
	R         float64  `json:"r"`
}
type Event struct {
	BarT      int64   `json:"barT"`
	Stage     string  `json:"stage"`
	IntrabarT *int64  `json:"intrabarT"`
	OrderID   string  `json:"orderId"`
	Reason    string  `json:"reason"`
	Qty       float64 `json:"qty"`
	Price     float64 `json:"price"`
	Fee       float64 `json:"fee"`
}
type EquityPoint struct {
	T          int64   `json:"t"`
	Balance    float64 `json:"balance"`
	Unrealized float64 `json:"unrealized"`
	Equity     float64 `json:"equity"`
	Open       bool    `json:"open"`
	Gap        bool    `json:"gap"`
}
type Diagnostics struct {
	RawSignals          int                `json:"rawSignals"`
	Occupied            int                `json:"occupied"`
	BothBand            int                `json:"bothBand"`
	RejectedQty         int                `json:"rejectedQty"`
	RejectedCap         int                `json:"rejectedCap"`
	RejectedCost        int                `json:"rejectedCost"`
	RejectedTarget      int                `json:"rejectedTarget"`
	PendingEntryGaps    int                `json:"pendingEntryGaps"`
	SkippedEntryGaps    int                `json:"skippedEntryGaps"`
	GapCount            int                `json:"gapCount"`
	MissingIntervals    int64              `json:"missingIntervals"`
	HeldGaps            int                `json:"heldGaps"`
	HeldGapMS           int64              `json:"heldGapMs"`
	OvernightBoundaries int                `json:"overnightBoundaries"`
	Collisions          int                `json:"collisions"`
	EntryBarExits       int                `json:"entryBarExits"`
	GapStops            int                `json:"gapStops"`
	BranchSignals       map[string]int     `json:"branchSignals"`
	BranchFills         map[string]int     `json:"branchFills"`
	BranchNet           map[string]float64 `json:"branchNet"`
}
type Terminal struct {
	T                           int64     `json:"t"`
	LastClosedBarT              int64     `json:"lastClosedBarT"`
	Balance                     float64   `json:"balance"`
	GrossRealized               float64   `json:"grossRealized"`
	Fees                        float64   `json:"fees"`
	Unrealized                  float64   `json:"unrealized"`
	Equity                      float64   `json:"equity"`
	LiquidationEquivalentEquity float64   `json:"liquidationEquivalentEquity"`
	ModeledExitCost             float64   `json:"modeledExitCost"`
	Open                        *Position `json:"openPosition"`
	Pending                     *Order    `json:"pendingOrder"`
	Mark                        float64   `json:"mark"`
}
type Metrics struct {
	TradeCount   int      `json:"tradeCount"`
	NetReturn    float64  `json:"netReturn"`
	MaxDrawdown  float64  `json:"maxDrawdown"`
	Wins         int      `json:"wins"`
	Losses       int      `json:"losses"`
	NetR         float64  `json:"netR"`
	ProfitFactor *float64 `json:"profitFactor"`
}
type Result struct {
	Schema      string        `json:"schema"`
	Profile     string        `json:"profile"`
	Status      string        `json:"status"`
	Config      Config        `json:"config"`
	SourceHash  string        `json:"sourceHash"`
	DataHash    string        `json:"dataHash"`
	Warmup      int           `json:"warmup"`
	Bars        int           `json:"bars"`
	Trades      []Trade       `json:"trades"`
	Events      []Event       `json:"events"`
	Equity      []EquityPoint `json:"equity"`
	Terminal    Terminal      `json:"terminal"`
	Diagnostics Diagnostics   `json:"diagnostics"`
	Metrics     Metrics       `json:"metrics"`
	Limitations []string      `json:"limitations"`
}
