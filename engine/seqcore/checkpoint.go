package seqcore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// checkpointV1 is a bounded, self-contained representation, not an event log.
// Identity/header keys are public contract; state fields are versioned here.
type checkpointV1 struct {
	Identity
	StateVersion           int       `json:"state_version"`
	Config                 Config    `json:"config"`
	LastBarIndex           int64     `json:"last_bar_index"`
	LastOpenMS             *int64    `json:"last_open_ms"`
	LastEventSeq           int64     `json:"last_event_seq"`
	InitDiagnosticsEmitted bool      `json:"init_diagnostics_emitted"`
	PreviousComparison     int       `json:"previous_comparison"`
	History                []Bar     `json:"history"`
	Buy                    sideState `json:"buy"`
	Sell                   sideState `json:"sell"`
}

// Checkpoint serializes after a closed bar (or before the first bar). A terminal
// engine cannot create a new checkpoint; use the bytes saved before its error.
func (e *Engine) Checkpoint() ([]byte, error) {
	if e.terminal != nil {
		x := *e.terminal
		return nil, &x
	}
	c := checkpointV1{Identity: e.identity, StateVersion: 1, Config: e.config, LastBarIndex: e.lastIndex, LastOpenMS: e.lastOpen, LastEventSeq: e.lastEventSeq, InitDiagnosticsEmitted: e.initDiagnosticsEmitted, PreviousComparison: e.previousComparison, History: e.history, Buy: e.sides[0], Sell: e.sides[1]}
	return json.Marshal(c)
}

// Restore checks the required wire shape, compares identity in the frozen
// precedence order, and validates state invariants. It consumes no bar and never
// changes the supplied bytes. Checkpoints are trusted application state, not
// authenticated source data or a proof that every prior transition was reachable.
// nextOpenMS is checked here and must be used for the caller's next Step.
func Restore(data []byte, requested Identity, nextOpenMS int64) (*Engine, error) {
	if err := validateCheckpointShape(data); err != nil {
		return nil, err
	}
	var c checkpointV1
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return nil, &InvalidCheckpointError{"JSON: " + err.Error()}
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, &InvalidCheckpointError{"trailing JSON"}
	}
	saved := c.Identity
	fields := []struct {
		name  string
		match bool
	}{
		{"schema_version", saved.SchemaVersion == requested.SchemaVersion},
		{"profile_id", saved.ProfileID == requested.ProfileID},
		{"freeze_id", saved.FreezeID == requested.FreezeID},
		{"config_hash", saved.ConfigHash == requested.ConfigHash},
		{"source.symbol", saved.Source.Symbol == requested.Source.Symbol},
		{"source.timeframe", saved.Source.Timeframe == requested.Source.Timeframe && saved.TimeframeMS == requested.TimeframeMS},
		{"source.data_hash", equalPtr(saved.Source.DataHash, requested.Source.DataHash)},
	}
	for _, f := range fields {
		if !f.match {
			return nil, &CheckpointIdentityError{Field: f.name}
		}
	}
	if c.LastOpenMS != nil && nextOpenMS <= *c.LastOpenMS {
		kind := "out_of_order_bar"
		if nextOpenMS == *c.LastOpenMS {
			kind = "duplicate_bar"
		}
		return nil, &CheckpointIdentityError{Field: "open_time", Kind: kind}
	}
	if err := validateIdentity(saved); err != nil {
		return nil, err
	}
	if err := validateCheckpoint(c); err != nil {
		return nil, err
	}
	e := &Engine{config: c.Config, identity: cloneIdentity(saved), lastIndex: c.LastBarIndex, lastOpen: c.LastOpenMS, lastEventSeq: c.LastEventSeq, initDiagnosticsEmitted: c.InitDiagnosticsEmitted, previousComparison: c.PreviousComparison, history: c.History, sides: [2]sideState{c.Buy, c.Sell}}
	return e, nil
}

func equalPtr[T comparable](a, b *T) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// A missing field is malformed state, not an identity disagreement. In particular,
// JSON null must not silently turn a required number or boolean into its zero value.
func validateCheckpointShape(data []byte) error {
	object := func(data []byte, path string, required, nullable []string) (map[string]json.RawMessage, error) {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(data, &m); err != nil || m == nil {
			return nil, &InvalidCheckpointError{path + " must be an object"}
		}
		allowedNull := map[string]bool{}
		for _, k := range nullable {
			allowedNull[k] = true
		}
		for _, k := range required {
			v, ok := m[k]
			if !ok {
				return nil, &InvalidCheckpointError{"missing " + path + "." + k}
			}
			if bytes.Equal(bytes.TrimSpace(v), []byte("null")) && !allowedNull[k] {
				return nil, &InvalidCheckpointError{"null " + path + "." + k}
			}
		}
		return m, nil
	}
	root, err := object(data, "checkpoint", []string{"schema_version", "profile_id", "freeze_id", "config_hash", "source", "timeframe_ms", "state_version", "config", "last_bar_index", "last_open_ms", "last_event_seq", "init_diagnostics_emitted", "previous_comparison", "history", "buy", "sell"}, []string{"last_open_ms", "history"})
	if err != nil {
		return err
	}
	if _, err = object(root["source"], "source", []string{"symbol", "timeframe", "data_hash"}, []string{"data_hash"}); err != nil {
		return err
	}
	if _, err = object(root["config"], "config", []string{"setup_count", "setup_lookback", "countdown_count", "countdown_lookback", "recycle_at", "deferral_13_vs_8", "qualifier_8_vs_5", "intersection", "combo", "risk_level", "tdst_cancel"}, nil); err != nil {
		return err
	}
	for _, name := range []string{"buy", "sell"} {
		side, err := object(root[name], name, []string{"setup_run", "setup_active", "last_completed_setup", "countdown", "bar8_close"}, []string{"last_completed_setup", "bar8_close"})
		if err != nil {
			return err
		}
		if _, err = object(side["countdown"], name+".countdown", []string{"state", "count", "setup_episode_id", "bar5_index", "bar8_index", "bar13_index"}, []string{"setup_episode_id", "bar5_index", "bar8_index", "bar13_index"}); err != nil {
			return err
		}
		if !bytes.Equal(bytes.TrimSpace(side["last_completed_setup"]), []byte("null")) {
			if _, err = object(side["last_completed_setup"], name+".last_completed_setup", []string{"episode_id", "completion_bar_index", "completion_bar_open_ms", "completion_decision_ms", "bar6_extreme", "bar7_extreme", "perfected", "perfected_at_bar_index", "perfected_at_bar_open_ms", "perfected_at_decision_ms"}, []string{"perfected_at_bar_index", "perfected_at_bar_open_ms", "perfected_at_decision_ms"}); err != nil {
				return err
			}
		}
	}
	var history []json.RawMessage
	if err = json.Unmarshal(root["history"], &history); err != nil {
		return &InvalidCheckpointError{"history must be an array or null"}
	}
	for _, bar := range history {
		if _, err = object(bar, "history bar", []string{"open_ms", "o", "h", "l", "c"}, nil); err != nil {
			return err
		}
	}
	return nil
}

func validateCheckpoint(c checkpointV1) error {
	bad := func(reason string) error { return &InvalidCheckpointError{reason} }
	if c.StateVersion != 1 {
		return bad("unsupported state_version")
	}
	if err := validateConfig(c.Config); err != nil {
		return bad("unsupported saved config")
	}
	if c.LastBarIndex < -1 || c.LastBarIndex > math.MaxInt64-1 || c.LastEventSeq < 0 {
		return bad("invalid index or event sequence")
	}
	if c.PreviousComparison < -1 || c.PreviousComparison > 1 {
		return bad("invalid previous comparison")
	}
	if len(c.History) > 4 {
		return bad("history exceeds four bars")
	}
	if c.LastOpenMS == nil {
		if c.LastBarIndex != -1 || c.LastEventSeq != 0 || c.InitDiagnosticsEmitted || len(c.History) != 0 || c.PreviousComparison != 0 {
			return bad("inconsistent empty stream")
		}
	} else {
		if c.LastBarIndex < 0 || c.LastEventSeq < 2 || !c.InitDiagnosticsEmitted || len(c.History) == 0 || int64(len(c.History)) > c.LastBarIndex+1 {
			return bad("inconsistent stream header")
		}
		if *c.LastOpenMS > math.MaxInt64-c.TimeframeMS {
			return bad("saved decision time overflow")
		}
		if c.History[len(c.History)-1].OpenMS != *c.LastOpenMS {
			return bad("history does not end at last_open_ms")
		}
	}
	for i, b := range c.History {
		if !validBar(b) || b.OpenMS > math.MaxInt64-c.TimeframeMS {
			return bad("invalid history bar")
		}
		if i > 0 && (b.OpenMS <= c.History[i-1].OpenMS || b.OpenMS > c.History[i-1].OpenMS+c.TimeframeMS) {
			return bad("noncontiguous segment history")
		}
	}
	if len(c.History) < 4 && c.PreviousComparison != 0 {
		return bad("comparison before warmup")
	}
	sides := [2]sideState{c.Buy, c.Sell}
	if unfinished(c.Buy.Countdown) && unfinished(c.Sell.Countdown) {
		return bad("both Countdowns unfinished")
	}
	for i, s := range sides {
		if s.SetupRun < 0 || s.SetupRun > c.LastBarIndex+1 || s.SetupActive != (s.SetupRun > 0) {
			return bad("invalid Setup run")
		}
		direction := -1
		if i == 1 {
			direction = 1
		}
		if s.SetupRun > 0 && (len(c.History) < 4 || c.PreviousComparison != direction) {
			return bad("Setup run contradicts comparison")
		}
		if setup := s.LastCompletedSetup; setup != nil {
			if c.LastOpenMS == nil || setup.CompletionBarIndex < 0 || setup.CompletionBarIndex > c.LastBarIndex || setup.CompletionBarOpenMS > *c.LastOpenMS || setup.CompletionBarOpenMS > math.MaxInt64-c.TimeframeMS || setup.CompletionDecisionMS != setup.CompletionBarOpenMS+c.TimeframeMS || !finite(setup.Bar6Extreme) || !finite(setup.Bar7Extreme) {
				return bad("invalid completed Setup")
			}
			wantID := fmt.Sprintf("%s:%s:%s:%s:%d", c.ProfileID, c.Source.Symbol, c.Source.Timeframe, sideName(i), setup.CompletionBarOpenMS)
			if setup.EpisodeID != wantID {
				return bad("invalid Setup identity")
			}
			all := setup.PerfectedAtBarIndex != nil && setup.PerfectedAtBarOpenMS != nil && setup.PerfectedAtDecisionMS != nil
			any := setup.PerfectedAtBarIndex != nil || setup.PerfectedAtBarOpenMS != nil || setup.PerfectedAtDecisionMS != nil
			if setup.Perfected != all || !setup.Perfected && any {
				return bad("invalid perfection references")
			}
			if all && (*setup.PerfectedAtBarIndex < setup.CompletionBarIndex || *setup.PerfectedAtBarIndex > c.LastBarIndex || *setup.PerfectedAtBarOpenMS < setup.CompletionBarOpenMS || *setup.PerfectedAtBarOpenMS > *c.LastOpenMS || *setup.PerfectedAtBarOpenMS > math.MaxInt64-c.TimeframeMS || *setup.PerfectedAtDecisionMS != *setup.PerfectedAtBarOpenMS+c.TimeframeMS) {
				return bad("invalid perfection times")
			}
		}
		d := s.Countdown
		switch d.State {
		case "idle":
			if d.Count != 0 || d.SetupEpisodeID != nil {
				return bad("idle Countdown has state")
			}
		case "active":
			if d.Count < 0 || d.Count > 12 {
				return bad("invalid active count")
			}
		case "deferred":
			if d.Count != 12 {
				return bad("invalid deferred count")
			}
		case "completed":
			if d.Count != 13 {
				return bad("invalid completed count")
			}
		default:
			return bad("invalid Countdown state")
		}
		if d.State != "idle" {
			prefix := fmt.Sprintf("%s:%s:%s:%s:", c.ProfileID, c.Source.Symbol, c.Source.Timeframe, sideName(i))
			if d.SetupEpisodeID == nil || !strings.HasPrefix(*d.SetupEpisodeID, prefix) || len(*d.SetupEpisodeID) == len(prefix) {
				return bad("missing Countdown identity")
			}
			if c.LastOpenMS == nil || unfinished(d) && s.LastCompletedSetup == nil {
				return bad("Countdown without Setup")
			}
			suffix := strings.TrimPrefix(*d.SetupEpisodeID, prefix)
			admittedOpen, err := strconv.ParseInt(suffix, 10, 64)
			if err != nil || strconv.FormatInt(admittedOpen, 10) != suffix || admittedOpen > *c.LastOpenMS || s.LastCompletedSetup != nil && admittedOpen > s.LastCompletedSetup.CompletionBarOpenMS {
				return bad("invalid Countdown admission timestamp")
			}
		}
		refs := []struct {
			count int
			p     *int64
		}{{5, d.Bar5Index}, {8, d.Bar8Index}, {13, d.Bar13Index}}
		last := int64(-1)
		for _, r := range refs {
			if (d.Count >= r.count) != (r.p != nil) {
				return bad("inconsistent Countdown references")
			}
			if r.p != nil {
				if *r.p < 0 || *r.p > c.LastBarIndex || *r.p <= last {
					return bad("invalid Countdown reference index")
				}
				last = *r.p
			}
		}
		if (d.Count >= 8) != (s.Bar8Close != nil) || s.Bar8Close != nil && !finite(*s.Bar8Close) {
			return bad("inconsistent Countdown bar-8 close")
		}
	}
	return nil
}
