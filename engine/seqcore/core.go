package seqcore

import (
	"fmt"
	"math"
)

type sideState struct {
	SideSnapshot
	Bar8Close *float64 `json:"bar8_close"`
}

// Engine owns one ordered synthetic stream. Returned values do not alias its state.
type Engine struct {
	config                 Config
	identity               Identity
	lastIndex              int64
	lastOpen               *int64
	lastEventSeq           int64
	initDiagnosticsEmitted bool
	// Only four prior bars are needed. A gap clears this segment-local history.
	history            []Bar
	previousComparison int
	sides              [2]sideState
	terminal           *BarOrderError
}

func New(config Config, identity Identity) (*Engine, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	if err := validateIdentity(identity); err != nil {
		return nil, err
	}
	e := &Engine{config: config, identity: cloneIdentity(identity), lastIndex: -1}
	for i := range e.sides {
		e.clearCountdown(i)
	}
	return e, nil
}

func validateIdentity(id Identity) error {
	if id.SchemaVersion != CheckpointSchema {
		return &CheckpointIdentityError{Field: "schema_version"}
	}
	if id.ProfileID != ProfileID {
		return &CheckpointIdentityError{Field: "profile_id"}
	}
	if id.FreezeID != FreezeID {
		return &CheckpointIdentityError{Field: "freeze_id"}
	}
	if id.TimeframeMS <= 0 {
		return &InvalidInputError{"timeframe_ms must be positive"}
	}
	return nil
}
func cloneIdentity(id Identity) Identity { id.Source.DataHash = copyPtr(id.Source.DataHash); return id }
func (e *Engine) Identity() Identity     { return cloneIdentity(e.identity) }

func Run(config Config, identity Identity, bars []Bar) (Trace, error) {
	e, err := New(config, identity)
	if err != nil {
		return Trace{}, err
	}
	return e.Run(bars)
}

// Run stops on the first error and returns only the preceding successful bars.
func (e *Engine) Run(bars []Bar) (Trace, error) {
	out := Trace{Events: []Event{}, Snapshots: []Snapshot{}}
	for _, b := range bars {
		snapshot, events, err := e.Step(b)
		if err != nil {
			return out, err
		}
		out.Events = append(out.Events, events...)
		out.Snapshots = append(out.Snapshots, *snapshot)
	}
	return out, nil
}

// Step consumes a closed bar. Duplicate/out-of-order input permanently stops this
// instance; a checkpoint taken before the error remains usable by Restore.
func (e *Engine) Step(b Bar) (*Snapshot, []Event, error) {
	if e.terminal != nil {
		x := *e.terminal
		return nil, nil, &x
	}
	if e.lastOpen != nil && b.OpenMS <= *e.lastOpen {
		kind := "out_of_order_bar"
		if b.OpenMS == *e.lastOpen {
			kind = "duplicate_bar"
		}
		e.terminal = &BarOrderError{kind, e.lastIndex + 1, *e.lastOpen, b.OpenMS}
		x := *e.terminal
		return nil, nil, &x
	}
	if !validBar(b) {
		return nil, nil, &InvalidInputError{"OHLC must be finite and low <= open,close <= high"}
	}
	if b.OpenMS > math.MaxInt64-e.identity.TimeframeMS {
		return nil, nil, &InvalidInputError{"decision time overflows int64"}
	}
	if e.lastEventSeq > math.MaxInt64-16 {
		return nil, nil, &InvalidInputError{"event sequence exhausted"}
	}
	if e.lastIndex == math.MaxInt64-1 {
		return nil, nil, &InvalidInputError{"bar index exhausted"}
	}
	e.lastIndex++
	events := []Event{}
	emit := func(phase, kind, side, reason, episode string, detail Detail) {
		e.lastEventSeq++
		events = append(events, Event{EventSeq: e.lastEventSeq, BarIndex: e.lastIndex, Phase: phase, Type: kind, Side: side, Reason: reason, EpisodeID: episode, Detail: detail, BarOpenMS: b.OpenMS, DecisionMS: b.OpenMS + e.identity.TimeframeMS})
	}
	if e.lastOpen != nil && b.OpenMS > *e.lastOpen+e.identity.TimeframeMS {
		dropped := []string{}
		seen := map[string]bool{}
		add := func(id string) {
			if !seen[id] {
				seen[id] = true
				dropped = append(dropped, id)
			}
		}
		for i := range e.sides {
			s := &e.sides[i]
			if s.LastCompletedSetup != nil && !s.LastCompletedSetup.Perfected {
				add(s.LastCompletedSetup.EpisodeID)
			}
			if unfinished(s.Countdown) {
				add(*s.Countdown.SetupEpisodeID)
				e.clearCountdown(i)
			}
			s.SetupRun = 0
			s.SetupActive = false
			s.LastCompletedSetup = nil
		}
		e.history = nil
		e.previousComparison = 0
		emit("P0", "data_gap", "both", "data_gap", "", DroppedEpisodesDetail{dropped})
	}
	if !e.initDiagnosticsEmitted {
		emit("P0", "diagnostic", "both", "tdst_cancel_unsupported", "", nil)
		emit("P0", "diagnostic", "both", "unsupported_public_formula", "", nil)
		e.initDiagnosticsEmitted = true
	}
	comparison := 0
	if len(e.history) == 4 {
		if b.C < e.history[0].C {
			comparison = -1
		} else if b.C > e.history[0].C {
			comparison = 1
		}
	}
	completed := [2]bool{}
	for i := range e.sides {
		s := &e.sides[i]
		direction := -1
		if i == 1 {
			direction = 1
		}
		if comparison == direction {
			if s.SetupRun > 0 {
				s.SetupRun++
			} else if e.previousComparison == -direction {
				s.SetupRun = 1
			}
		} else {
			s.SetupRun = 0
		}
		s.SetupActive = s.SetupRun > 0
		side := sideName(i)
		if s.SetupRun == 9 {
			completed[i] = true
			id := fmt.Sprintf("%s:%s:%s:%s:%d", e.identity.ProfileID, e.identity.Source.Symbol, e.identity.Source.Timeframe, side, b.OpenMS)
			var replacement Detail
			if s.LastCompletedSetup != nil {
				replacement = ReplacementDetail{s.LastCompletedSetup.EpisodeID}
			}
			setup := &CompletedSetup{EpisodeID: id, CompletionBarIndex: e.lastIndex, CompletionBarOpenMS: b.OpenMS, CompletionDecisionMS: b.OpenMS + e.identity.TimeframeMS, Bar6Extreme: extreme(e.history[1], i), Bar7Extreme: extreme(e.history[2], i)}
			s.LastCompletedSetup = setup
			emit("P1", "setup_complete", side, "", id, replacement)
			if perfects(extreme(e.history[3], i), setup, i) || perfects(extreme(b, i), setup, i) {
				e.markPerfected(setup, b)
				emit("P1", "perfection", side, "", id, nil)
			}
			if s.Countdown.State == "idle" || s.Countdown.State == "completed" {
				e.clearCountdown(i)
				s.Countdown.State = "active"
				s.Countdown.SetupEpisodeID = ptr(id)
				emit("P1", "countdown_start", side, "", id, nil)
			}
		} else if s.LastCompletedSetup != nil && !s.LastCompletedSetup.Perfected && perfects(extreme(b, i), s.LastCompletedSetup, i) {
			e.markPerfected(s.LastCompletedSetup, b)
			emit("P1", "delayed_perfection", side, "", s.LastCompletedSetup.EpisodeID, nil)
		}
	}
	e.previousComparison = comparison
	for i := range e.sides {
		c := &e.sides[i].Countdown
		if completed[1-i] && unfinished(*c) {
			emit("P2", "countdown_cancel", sideName(i), "opposite_setup_complete", *c.SetupEpisodeID, references(*c))
			e.clearCountdown(i)
		}
	}
	for i := range e.sides {
		s := &e.sides[i]
		c := &s.Countdown
		qualifies := false
		if unfinished(*c) && len(e.history) >= 2 {
			prior := e.history[len(e.history)-2]
			qualifies = (i == 0 && b.C <= prior.L) || (i == 1 && b.C >= prior.H)
		}
		if qualifies {
			terminalOK := c.Count != 12 || (i == 0 && b.L <= *s.Bar8Close) || (i == 1 && b.H >= *s.Bar8Close)
			if !terminalOK {
				c.State = "deferred"
				emit("P3", "countdown_defer", sideName(i), "deferral_13_vs_8", *c.SetupEpisodeID, CountDetail{c.Count})
			} else {
				c.Count++
				switch c.Count {
				case 5:
					c.Bar5Index = ptr(e.lastIndex)
				case 8:
					c.Bar8Index = ptr(e.lastIndex)
					s.Bar8Close = ptr(b.C)
				case 13:
					c.Bar13Index = ptr(e.lastIndex)
				}
				emit("P3", "countdown_count", sideName(i), "", *c.SetupEpisodeID, CountDetail{c.Count})
				if c.Count == 13 {
					c.State = "completed"
					emit("P3", "countdown_complete", sideName(i), "", *c.SetupEpisodeID, CountDetail{c.Count})
				}
			}
		}
		if s.SetupRun == 22 && c.State != "idle" {
			emit("P3", "countdown_recycle", sideName(i), "recycle_22", *c.SetupEpisodeID, references(*c))
			e.clearCountdown(i)
		}
	}
	e.lastOpen = ptr(b.OpenMS)
	if len(e.history) == 4 {
		copy(e.history, e.history[1:])
		e.history[3] = b
	} else {
		e.history = append(e.history, b)
	}
	snapshot := &Snapshot{BarIndex: e.lastIndex, BarOpenMS: b.OpenMS, DecisionMS: b.OpenMS + e.identity.TimeframeMS, PhaseOrderVersion: EventSchema, Buy: cloneSide(e.sides[0].SideSnapshot), Sell: cloneSide(e.sides[1].SideSnapshot)}
	return snapshot, events, nil
}

func sideName(i int) string {
	if i == 0 {
		return "buy"
	}
	return "sell"
}
func extreme(b Bar, i int) float64 {
	if i == 0 {
		return b.L
	}
	return b.H
}
func perfects(v float64, s *CompletedSetup, i int) bool {
	if i == 0 {
		return v < s.Bar6Extreme && v < s.Bar7Extreme
	}
	return v > s.Bar6Extreme && v > s.Bar7Extreme
}
func unfinished(c Countdown) bool { return c.State == "active" || c.State == "deferred" }
func references(c Countdown) ReferenceDetail {
	return ReferenceDetail{c.Count, copyPtr(c.Bar5Index), copyPtr(c.Bar8Index), copyPtr(c.Bar13Index)}
}
func (e *Engine) clearCountdown(i int) {
	e.sides[i].Countdown = Countdown{State: "idle"}
	e.sides[i].Bar8Close = nil
}
func (e *Engine) markPerfected(s *CompletedSetup, b Bar) {
	s.Perfected = true
	s.PerfectedAtBarIndex = ptr(e.lastIndex)
	s.PerfectedAtBarOpenMS = ptr(b.OpenMS)
	s.PerfectedAtDecisionMS = ptr(b.OpenMS + e.identity.TimeframeMS)
}
