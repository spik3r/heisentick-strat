package engine

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const goldFlagM15 int64 = 900000
const goldFlagM30 int64 = 1800000

// GoldFlagReferenceRequest is an offline observed-row reference. FromT and ToT
// select source observations before aggregation; no pre-start history is loaded.
// CostPerFill is a diagnostic price-point deduction, never executable spread.
type GoldFlagReferenceRequest struct {
	Config      dsl.Config
	M15         marketdata.Series
	FromT, ToT  int64
	CostPerFill float64
}

type GoldFlagBar struct {
	Index              int     `json:"index"`
	OpenT              int64   `json:"openT"`
	CloseT             int64   `json:"closeT"`
	FirstObservedT     int64   `json:"firstObservedT"`
	LastObservedCloseT int64   `json:"lastObservedCloseT"`
	Open               float64 `json:"open"`
	High               float64 `json:"high"`
	Low                float64 `json:"low"`
	Close              float64 `json:"close"`
	Volume             float64 `json:"volume"`
	SourceCount        int     `json:"sourceCount"`
	Complete           bool    `json:"complete"`
}

type GoldFlagExtreme struct {
	FirstBucketT int64   `json:"firstBucketT"`
	LastBucketT  int64   `json:"lastBucketT"`
	AvailableT   int64   `json:"availableT"`
	High         float64 `json:"high"`
	Low          float64 `json:"low"`
}

type GoldFlagSignal struct {
	SignalIdx int     `json:"signalIdx"`
	Side      int     `json:"side"`
	ATR       float64 `json:"atr"`
	EdgeHi    float64 `json:"edgeHi"`
	EdgeLo    float64 `json:"edgeLo"`
	Pole      float64 `json:"pole"`
	Level     string  `json:"level"`
}

type GoldFlagSnapshot struct {
	GoldFlagBar
	ATR       *float64         `json:"atr"`
	H4        *GoldFlagExtreme `json:"h4"`
	Day       *GoldFlagExtreme `json:"day"`
	Candidate *GoldFlagSignal  `json:"candidate"`
}

// GoldFlagEventTime is an interval in assumed source timestamp coordinates.
// Actual publication latency and venue execution timing are not established.
type GoldFlagEventTime struct {
	LowerMS        int64  `json:"lowerMs"`
	UpperMS        int64  `json:"upperMs"`
	LowerInclusive bool   `json:"lowerInclusive"`
	UpperInclusive bool   `json:"upperInclusive"`
	Kind           string `json:"kind"`
}

type GoldFlagEvent struct {
	State        string            `json:"state"`
	RowIdx       int               `json:"rowIdx"`
	Time         GoldFlagEventTime `json:"time"`
	AfterEventID *int              `json:"afterEventId"`
	Basis        string            `json:"basis"`
}

type GoldFlagAmbiguity struct {
	Kind         string            `json:"kind"`
	BarIdx       int               `json:"barIdx"`
	Alternatives []string          `json:"alternatives"`
	Time         GoldFlagEventTime `json:"time"`
	// Alternatives are local OHLC-feasible states, not propagated portfolio bounds.
}

type GoldFlagClock struct {
	SignalOpenMS       int64  `json:"signalOpenMs"`
	SignalAvailableMS  int64  `json:"signalAvailableMs"`
	NextObservedOpenMS *int64 `json:"nextObservedOpenMs"`
	ExpiryRow          int    `json:"expiryRow"`
	ExpiryCloseMS      *int64 `json:"expiryCloseMs"`
	ElapsedTwoHourMS   int64  `json:"elapsedTwoHourMs"`
	TimeExitRow        *int   `json:"timeExitRow"`
	TimeExitCloseMS    *int64 `json:"timeExitCloseMs"`
	SetupGapRows       []int  `json:"setupGapRows"`
	ActiveGapRows      []int  `json:"activeGapRows"`
	PartialRows        []int  `json:"partialRows"`
}

type GoldFlagOrder struct {
	GoldFlagSignal
	EntryPx       float64             `json:"entryPx"`
	Stop          float64             `json:"stop"`
	PlanRisk      float64             `json:"planRisk"`
	Status        string              `json:"status"`
	ResolutionIdx int                 `json:"resolutionIdx"`
	FillIdx       *int                `json:"fillIdx,omitempty"`
	Entry         *float64            `json:"entry,omitempty"`
	Risk          *float64            `json:"risk,omitempty"`
	TP            *float64            `json:"tp,omitempty"`
	EntryGap      *float64            `json:"entryGap,omitempty"`
	EntryDual     bool                `json:"entryDual,omitempty"`
	ExitIdx       *int                `json:"exitIdx,omitempty"`
	Exit          *float64            `json:"exit,omitempty"`
	Reason        string              `json:"reason,omitempty"`
	GrossR        *float64            `json:"grossR,omitempty"`
	NetR          *float64            `json:"netR,omitempty"`
	Mark          *float64            `json:"mark,omitempty"`
	Ambiguities   []GoldFlagAmbiguity `json:"ambiguities"`
	Events        []GoldFlagEvent     `json:"events"`
	Clock         GoldFlagClock       `json:"clock"`
}

type GoldFlagGap struct {
	RowIdx          int   `json:"rowIdx"`
	FromMS          int64 `json:"fromMs"`
	ToMS            int64 `json:"toMs"`
	MissingM30Slots int64 `json:"missingM30Slots"`
}

type GoldFlagReferenceResult struct {
	Schema          string             `json:"schema"`
	Policy          string             `json:"policy"`
	EvidenceStatus  string             `json:"evidenceStatus"`
	FromT           int64              `json:"fromT"`
	ToT             int64              `json:"toT"`
	CostPerFill     float64            `json:"costPerFill"`
	UsedSourceRows  int                `json:"usedSourceRows"`
	Snapshots       []GoldFlagSnapshot `json:"snapshots"`
	Orders          []GoldFlagOrder    `json:"orders"`
	Gaps            []GoldFlagGap      `json:"gaps"`
	TerminalOrderID *int               `json:"terminalOrderId"`
	Assumptions     []string           `json:"assumptions"`
}
