package engine

import "fmt"

const sequentialFullAuditSchema = "sequential-full-execution.v1"

// SequentialFullStatus describes one opportunity, including decisions that
// deliberately did not create a broker position. Pending is internal-prefix
// state only; a finalized run resolves a terminal decision as no_next_bar.
type SequentialFullStatus string

const (
	SequentialFullPending  SequentialFullStatus = "pending"
	SequentialFullRejected SequentialFullStatus = "rejected"
	SequentialFullFilled   SequentialFullStatus = "filled"
)

type SequentialFullReason string

const (
	SequentialFullWarmup       SequentialFullReason = "warmup"
	SequentialFullExpired      SequentialFullReason = "signal_expired"
	SequentialFullOccupied     SequentialFullReason = "blocked_in_position"
	SequentialFullSimultaneous SequentialFullReason = "simultaneous_signal"
	SequentialFullNoNextBar    SequentialFullReason = "no_next_bar"
	SequentialFullWrongSide    SequentialFullReason = "stop_breached_at_entry"
)

// SequentialFullOpportunity binds a close decision to the admitting Setup,
// frozen stop inputs, optional fill, and the reason for any rejected entry.
// All prices are raw broker price units; quantity has no instrument multiplier.
type SequentialFullOpportunity struct {
	ID              string               `json:"id"`
	EpisodeID       string               `json:"episodeId"`
	Trigger         string               `json:"trigger"`
	Side            string               `json:"side"`
	SetupIndex      int                  `json:"setupIndex"`
	SetupFirstIndex int                  `json:"setupFirstIndex"`
	DecisionIndex   int                  `json:"decisionIndex"`
	DecisionOpenMS  int64                `json:"decisionOpenMs"`
	DecisionMS      int64                `json:"decisionMs"`
	NextOpenIndex   int                  `json:"nextOpenIndex"`
	Status          SequentialFullStatus `json:"status"`
	Reason          SequentialFullReason `json:"reason,omitempty"`
	Stop            float64              `json:"stop"`
	SignalATR       float64              `json:"signalAtr"`
	FillIndex       *int                 `json:"fillIndex,omitempty"`
	Fill            *float64             `json:"fill,omitempty"`
	Target          *float64             `json:"target,omitempty"`
	RiskDistance    *float64             `json:"riskDistance,omitempty"`
	Size            *float64             `json:"size,omitempty"`
	CapBinds        bool                 `json:"capBinds"`
}

// SequentialFullAudit is an execution audit, not historical qualification.
// It preserves the broker's existing PnL versus realized entry-fee convention.
type SequentialFullAudit struct {
	Schema        string                      `json:"schema"`
	Profile       string                      `json:"profile"`
	Policy        string                      `json:"policy"`
	Symbol        string                      `json:"symbol"`
	Timeframe     string                      `json:"timeframe"`
	Opportunities []SequentialFullOpportunity `json:"opportunities"`
}

// SequentialFullExecutionError refuses unsupported terminal exposure or an
// invalid derivation. Callers must propagate it rather than return empty success.
type SequentialFullExecutionError struct {
	Kind          string `json:"kind"`
	Field         string `json:"field"`
	BarIndex      int    `json:"barIndex"`
	OpportunityID string `json:"opportunityId,omitempty"`
}

func (e *SequentialFullExecutionError) Error() string {
	return fmt.Sprintf("%s: %s at bar %d", e.Kind, e.Field, e.BarIndex)
}
