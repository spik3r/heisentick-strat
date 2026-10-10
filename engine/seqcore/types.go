package seqcore

import (
	"fmt"
	"math"
)

const (
	ProfileID        = "seq.full.public_approx.v1"
	FreezeID         = "seq-core-freeze.v1"
	EventSchema      = "seq-core-events.v1"
	CheckpointSchema = "seq-core-checkpoint.v1"
	GapPolicy        = "synthetic_unexpected_gap.v1"
)

// Config qualifies only the accepted default profile. Start with DefaultConfig.
type Config struct {
	SetupCount        int  `json:"setup_count"`
	SetupLookback     int  `json:"setup_lookback"`
	CountdownCount    int  `json:"countdown_count"`
	CountdownLookback int  `json:"countdown_lookback"`
	RecycleAt         int  `json:"recycle_at"`
	Deferral13Vs8     bool `json:"deferral_13_vs_8"`
	Qualifier8Vs5     bool `json:"qualifier_8_vs_5"`
	Intersection      bool `json:"intersection"`
	Combo             bool `json:"combo"`
	RiskLevel         bool `json:"risk_level"`
	TDSTCancel        bool `json:"tdst_cancel"`
}

func DefaultConfig() Config {
	return Config{SetupCount: 9, SetupLookback: 4, CountdownCount: 13, CountdownLookback: 2, RecycleAt: 22, Deferral13Vs8: true}
}

type UnsupportedConfigError struct {
	Field  string `json:"field"`
	Value  any    `json:"value"`
	Reason string `json:"reason"`
}

func (e *UnsupportedConfigError) Error() string {
	return fmt.Sprintf("unsupported_config: %s=%v", e.Field, e.Value)
}
func validateConfig(c Config) error {
	d := DefaultConfig()
	fields := []struct {
		name      string
		got, want any
	}{
		{"setup_count", c.SetupCount, d.SetupCount}, {"setup_lookback", c.SetupLookback, d.SetupLookback},
		{"countdown_count", c.CountdownCount, d.CountdownCount}, {"countdown_lookback", c.CountdownLookback, d.CountdownLookback},
		{"recycle_at", c.RecycleAt, d.RecycleAt}, {"deferral_13_vs_8", c.Deferral13Vs8, d.Deferral13Vs8},
		{"qualifier_8_vs_5", c.Qualifier8Vs5, false}, {"intersection", c.Intersection, false}, {"combo", c.Combo, false},
		{"risk_level", c.RiskLevel, false}, {"tdst_cancel", c.TDSTCancel, false},
	}
	for _, f := range fields {
		if f.got != f.want {
			return &UnsupportedConfigError{f.name, f.got, "unsupported_config"}
		}
	}
	return nil
}

type Source struct {
	Symbol    string  `json:"symbol"`
	Timeframe string  `json:"timeframe"`
	DataHash  *string `json:"data_hash"`
}

// Identity is compared literally on restore. ConfigHash is caller-supplied and opaque.
// TimeframeMS is the actual bar duration and is included in timeframe identity.
type Identity struct {
	SchemaVersion string `json:"schema_version"`
	ProfileID     string `json:"profile_id"`
	FreezeID      string `json:"freeze_id"`
	ConfigHash    string `json:"config_hash"`
	Source        Source `json:"source"`
	TimeframeMS   int64  `json:"timeframe_ms"`
}

func DefaultIdentity(source Source, configHash string, timeframeMS int64) Identity {
	return Identity{CheckpointSchema, ProfileID, FreezeID, configHash, source, timeframeMS}
}

type Bar struct {
	OpenMS int64   `json:"open_ms"`
	O      float64 `json:"o"`
	H      float64 `json:"h"`
	L      float64 `json:"l"`
	C      float64 `json:"c"`
}

func validBar(b Bar) bool {
	return finite(b.O) && finite(b.H) && finite(b.L) && finite(b.C) && b.L <= math.Min(b.O, b.C) && math.Max(b.O, b.C) <= b.H
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

type BarOrderError struct {
	Kind           string `json:"kind"`
	BarIndex       int64  `json:"bar_index"`
	PreviousOpenMS int64  `json:"previous_open_ms"`
	OpenMS         int64  `json:"open_ms"`
}

func (e *BarOrderError) Error() string {
	return fmt.Sprintf("%s at bar %d: %d is not after %d", e.Kind, e.BarIndex, e.OpenMS, e.PreviousOpenMS)
}

type InvalidInputError struct{ Reason string }

func (e *InvalidInputError) Error() string { return "invalid seqcore input: " + e.Reason }

type CheckpointIdentityError struct {
	Field string `json:"mismatch_field"`
	Kind  string `json:"kind,omitempty"`
}

func (e *CheckpointIdentityError) Error() string { return "checkpoint identity mismatch: " + e.Field }

type InvalidCheckpointError struct{ Reason string }

func (e *InvalidCheckpointError) Error() string { return "invalid seqcore checkpoint: " + e.Reason }

// Detail variants preserve absent fields and explicit null references in the wire contract.
type Detail interface{ eventDetail() }
type CountDetail struct {
	Count int `json:"count"`
}

func (CountDetail) eventDetail() {}

type ReferenceDetail struct {
	Count      int    `json:"count"`
	Bar5Index  *int64 `json:"bar5_index"`
	Bar8Index  *int64 `json:"bar8_index"`
	Bar13Index *int64 `json:"bar13_index"`
}

func (ReferenceDetail) eventDetail() {}

type ReplacementDetail struct {
	ReplacedEpisodeID string `json:"replaced_episode_id"`
}

func (ReplacementDetail) eventDetail() {}

type DroppedEpisodesDetail struct {
	DroppedEpisodes []string `json:"dropped_episodes"`
}

func (DroppedEpisodesDetail) eventDetail() {}

type Event struct {
	EventSeq   int64  `json:"event_seq"`
	BarIndex   int64  `json:"bar_index"`
	Phase      string `json:"phase"`
	Type       string `json:"type"`
	Side       string `json:"side"`
	Reason     string `json:"reason,omitempty"`
	EpisodeID  string `json:"episode_id,omitempty"`
	Detail     Detail `json:"detail,omitempty"`
	BarOpenMS  int64  `json:"bar_open_ms"`
	DecisionMS int64  `json:"decision_ms"`
}

type CompletedSetup struct {
	EpisodeID             string  `json:"episode_id"`
	CompletionBarIndex    int64   `json:"completion_bar_index"`
	CompletionBarOpenMS   int64   `json:"completion_bar_open_ms"`
	CompletionDecisionMS  int64   `json:"completion_decision_ms"`
	Bar6Extreme           float64 `json:"bar6_extreme"`
	Bar7Extreme           float64 `json:"bar7_extreme"`
	Perfected             bool    `json:"perfected"`
	PerfectedAtBarIndex   *int64  `json:"perfected_at_bar_index"`
	PerfectedAtBarOpenMS  *int64  `json:"perfected_at_bar_open_ms"`
	PerfectedAtDecisionMS *int64  `json:"perfected_at_decision_ms"`
}
type Countdown struct {
	State          string  `json:"state"`
	Count          int     `json:"count"`
	SetupEpisodeID *string `json:"setup_episode_id"`
	Bar5Index      *int64  `json:"bar5_index"`
	Bar8Index      *int64  `json:"bar8_index"`
	Bar13Index     *int64  `json:"bar13_index"`
}
type SideSnapshot struct {
	SetupRun           int64           `json:"setup_run"`
	SetupActive        bool            `json:"setup_active"`
	LastCompletedSetup *CompletedSetup `json:"last_completed_setup"`
	Countdown          Countdown       `json:"countdown"`
}
type Snapshot struct {
	BarIndex          int64        `json:"bar_index"`
	BarOpenMS         int64        `json:"bar_open_ms"`
	DecisionMS        int64        `json:"decision_ms"`
	PhaseOrderVersion string       `json:"phase_order_version"`
	Buy               SideSnapshot `json:"buy"`
	Sell              SideSnapshot `json:"sell"`
}
type Trace struct {
	Events    []Event    `json:"events"`
	Snapshots []Snapshot `json:"snapshots"`
}

func ptr[T any](v T) *T { return &v }
func copyPtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	return ptr(*p)
}
func cloneCountdown(c Countdown) Countdown {
	c.SetupEpisodeID = copyPtr(c.SetupEpisodeID)
	c.Bar5Index = copyPtr(c.Bar5Index)
	c.Bar8Index = copyPtr(c.Bar8Index)
	c.Bar13Index = copyPtr(c.Bar13Index)
	return c
}
func cloneSide(s SideSnapshot) SideSnapshot {
	s.Countdown = cloneCountdown(s.Countdown)
	if s.LastCompletedSetup != nil {
		x := *s.LastCompletedSetup
		x.PerfectedAtBarIndex = copyPtr(x.PerfectedAtBarIndex)
		x.PerfectedAtBarOpenMS = copyPtr(x.PerfectedAtBarOpenMS)
		x.PerfectedAtDecisionMS = copyPtr(x.PerfectedAtDecisionMS)
		s.LastCompletedSetup = &x
	}
	return s
}
