package seqcore_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	sc "github.com/spik3r/heisentick-strat/engine/seqcore"
)

func applyIdentityOverride(t *testing.T, id *sc.Identity, field string, value any) {
	t.Helper()
	switch field {
	case "schema_version":
		id.SchemaVersion = value.(string)
	case "profile_id":
		id.ProfileID = value.(string)
	case "freeze_id":
		id.FreezeID = value.(string)
	case "config_hash":
		id.ConfigHash = value.(string)
	case "source.symbol":
		id.Source.Symbol = value.(string)
	case "source.timeframe":
		id.Source.Timeframe = value.(string)
	case "source.timeframe_ms":
		id.TimeframeMS = int64(value.(float64))
	case "source.data_hash":
		if value == nil {
			id.Source.DataHash = nil
		} else {
			s := value.(string)
			id.Source.DataHash = &s
		}
	default:
		t.Fatalf("unknown fixture identity override %q", field)
	}
}
func assertIdentityError(t *testing.T, e *sc.Engine, err error, field string) {
	t.Helper()
	var typed *sc.CheckpointIdentityError
	if e != nil || !errors.As(err, &typed) {
		t.Fatalf("got engine=%v, error=%T %v; want nil and *CheckpointIdentityError(%s)", e, err, err, field)
	}
	if typed.Field != field {
		t.Fatalf("first mismatch %q, want %q", typed.Field, field)
	}
}
func assertRestoreOrderError(t *testing.T, e *sc.Engine, err error, kind string) {
	t.Helper()
	assertIdentityError(t, e, err, "open_time")
	var typed *sc.CheckpointIdentityError
	if !errors.As(err, &typed) || typed.Kind != kind {
		t.Fatalf("restore order error=%+v, want kind %s", typed, kind)
	}
}
func testCheckpointFixture(t *testing.T, c fixtureCase) {
	t.Helper()
	cp := c.Expected.Checkpoint
	id := fixtureIdentity(c)
	engine := newEngine(t, id)
	if _, err := engine.Run(c.Bars[:cp.AfterBarIndex+1]); err != nil {
		t.Fatal(err)
	}
	data, wire := checkpointJSON(t, engine)
	sparseJSON(t, "checkpoint header", wire, cp.Saved)
	originalBytes := append([]byte{}, data...)
	requested := id
	for field, value := range cp.ResumeWith {
		applyIdentityOverride(t, &requested, field, value)
	}
	next := c.Bars[cp.AfterBarIndex+1].OpenMS
	restored, err := sc.Restore(data, requested, next)
	mismatch := c.Expected.Expect["mismatch_field"].(string)
	if mismatch == "open_time" {
		if restored != nil {
			t.Fatal("invalid next time yielded engine")
		}
		kind := "out_of_order_bar"
		if next == c.Bars[cp.AfterBarIndex].OpenMS {
			kind = "duplicate_bar"
		}
		assertRestoreOrderError(t, restored, err, kind)
	} else {
		assertIdentityError(t, restored, err, mismatch)
	}
	if !bytes.Equal(data, originalBytes) {
		t.Fatal("Restore mutated supplied checkpoint bytes")
	}
	// Rejection cannot consume state, events or sequence. Reuse the very same
	// serialized checkpoint with the accepted identity and corrected next time.
	validNext := c.Bars[cp.AfterBarIndex]
	validNext.OpenMS += c.Series.TimeframeMS
	restored, err = sc.Restore(data, id, validNext.OpenMS)
	if err != nil {
		t.Fatal(err)
	}
	got, events, err := restored.Step(validNext)
	if err != nil {
		t.Fatal(err)
	}
	want, wantEvents, err := engine.Step(validNext)
	if err != nil {
		t.Fatal(err)
	}
	equalJSON(t, "post-rejection reusable snapshot", got, want)
	equalJSON(t, "post-rejection reusable events", events, wantEvents)
}

func TestCheckpointEveryAdjacentMismatchPrecedence(t *testing.T) {
	c := caseNamed(t, "init_diagnostics.restart_with_events.buy")
	id := fixtureIdentity(c)
	engine := newEngine(t, id)
	if _, err := engine.Run(c.Bars[:15]); err != nil {
		t.Fatal(err)
	}
	data, _ := checkpointJSON(t, engine)
	next := c.Bars[15].OpenMS
	fields := []string{"schema_version", "profile_id", "freeze_id", "config_hash", "source.symbol", "source.timeframe", "source.data_hash", "open_time"}
	for i := 0; i < len(fields)-1; i++ {
		t.Run(fields[i]+"_before_"+fields[i+1], func(t *testing.T) {
			request := id
			open := next
			for _, field := range fields[i : i+2] {
				if field == "open_time" {
					open = c.Bars[14].OpenMS
				} else {
					applyIdentityOverride(t, &request, field, "different")
				}
			}
			restored, err := sc.Restore(data, request, open)
			assertIdentityError(t, restored, err, fields[i])
		})
	}
	t.Run("all_fields", func(t *testing.T) {
		request := id
		for _, field := range fields[:len(fields)-1] {
			applyIdentityOverride(t, &request, field, "different")
		}
		restored, err := sc.Restore(data, request, c.Bars[14].OpenMS-1)
		assertIdentityError(t, restored, err, "schema_version")
	})
	t.Run("actual_duration_is_timeframe_identity", func(t *testing.T) {
		request := id
		request.TimeframeMS++
		hash := "different"
		request.Source.DataHash = &hash
		restored, err := sc.Restore(data, request, c.Bars[14].OpenMS)
		assertIdentityError(t, restored, err, "source.timeframe")
	})
}

func TestCheckpointDataHashPresenceAndOpaqueConfiguration(t *testing.T) {
	a, b, empty := "hash-a", "hash-b", ""
	cases := []struct {
		name             string
		saved, requested *string
		accepted         bool
	}{
		{"both_absent", nil, nil, true}, {"both_present_equal", &a, &a, true},
		{"absent_to_present", nil, &a, false}, {"present_to_absent", &a, nil, false},
		{"present_to_different", &a, &b, false}, {"absent_to_empty_present", nil, &empty, false},
		{"empty_present_to_absent", &empty, nil, false}, {"both_empty_present", &empty, &empty, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := sc.DefaultIdentity(sc.Source{Symbol: "SYN", Timeframe: "5m", DataHash: tc.saved}, "opaque caller supplied / config identity", 300000)
			e := newEngine(t, id)
			bar := sc.Bar{OpenMS: 1000, O: 10, H: 11, L: 9, C: 10}
			if _, _, err := e.Step(bar); err != nil {
				t.Fatal(err)
			}
			data, _ := checkpointJSON(t, e)
			requested := id
			requested.Source.DataHash = tc.requested
			restored, err := sc.Restore(data, requested, 301000)
			if !tc.accepted {
				assertIdentityError(t, restored, err, "source.data_hash")
				return
			}
			if err != nil {
				t.Fatalf("equal literal identity rejected: %v", err)
			}
			equalJSON(t, "literal identity", restored.Identity(), id)
			requested.ConfigHash += " "
			restored, err = sc.Restore(data, requested, 301000)
			assertIdentityError(t, restored, err, "config_hash")
		})
	}
}

func TestCheckpointBeforeFirstBar(t *testing.T) {
	c := caseNamed(t, "init_diagnostics.restart_with_events.buy")
	id := fixtureIdentity(c)
	e := newEngine(t, id)
	data, wire := checkpointJSON(t, e)
	sparseJSON(t, "empty checkpoint", wire, map[string]any{"last_open_ms": nil, "last_event_seq": float64(0), "init_diagnostics_emitted": false})
	restored, err := sc.Restore(data, id, c.Bars[0].OpenMS)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restored.Run(c.Bars)
	if err != nil {
		t.Fatal(err)
	}
	want, err := e.Run(c.Bars)
	if err != nil {
		t.Fatal(err)
	}
	compareTrace(t, "empty checkpoint resume", got, want)
	assertDiagnostics(t, got.Events)
}

func TestCheckpointRestoredRepeatedlyDoesNotDuplicateDiagnostics(t *testing.T) {
	bars := independentSlide(40)
	// Two unexpected gaps. Initialization belongs to the run, not a segment.
	for i := 16; i < len(bars); i++ {
		bars[i].OpenMS += 300000
	}
	for i := 29; i < len(bars); i++ {
		bars[i].OpenMS += 600000
	}
	id := independentIdentity()
	engine := newEngine(t, id)
	var got sc.Trace
	for i, bar := range bars {
		s, ev, err := engine.Step(bar)
		if err != nil {
			t.Fatal(err)
		}
		got.Snapshots = append(got.Snapshots, *s)
		got.Events = append(got.Events, ev...)
		data, wire := checkpointJSON(t, engine)
		if wire["init_diagnostics_emitted"] != true {
			t.Fatalf("bar %d lost initialization flag", i)
		}
		next := bar.OpenMS + 300000
		if i+1 < len(bars) {
			next = bars[i+1].OpenMS
		}
		engine, err = sc.Restore(data, id, next)
		if err != nil {
			t.Fatalf("boundary %d: %v", i, err)
		}
	}
	want, err := sc.Run(sc.DefaultConfig(), id, bars)
	if err != nil {
		t.Fatal(err)
	}
	compareTrace(t, "restart every successive bar", got, want)
	assertDiagnostics(t, got.Events)
	gaps := 0
	for _, ev := range got.Events {
		if ev.Type == "data_gap" {
			gaps++
		}
	}
	if gaps != 2 {
		t.Fatal(fmt.Sprintf("got %d gap events, want 2", gaps))
	}
}

// These corruption probes describe the version-1 serialization envelope, not
// lifecycle expectations. A damaged blob is never a partly usable engine.
func TestMalformedCheckpointRejectedWithoutEngine(t *testing.T) {
	id := independentIdentity()
	e := newEngine(t, id)
	bars := independentSlide(22)
	if _, err := e.Run(bars); err != nil {
		t.Fatal(err)
	}
	data, _ := checkpointJSON(t, e)
	next := bars[len(bars)-1].OpenMS + 300000
	type mutation struct {
		name  string
		apply func(map[string]any)
	}
	mutations := []mutation{
		{"state_version", func(m map[string]any) { m["state_version"] = 99 }},
		{"negative_sequence", func(m map[string]any) { m["last_event_seq"] = -1 }},
		{"lost_initialization_flag", func(m map[string]any) { m["init_diagnostics_emitted"] = false }},
		{"invalid_previous_comparison", func(m map[string]any) { m["previous_comparison"] = 42 }},
		{"unsupported_saved_config", func(m map[string]any) { m["config"].(map[string]any)["combo"] = true }},
		{"unknown_header_field", func(m map[string]any) { m["unexpected_field"] = true }},
		{"too_much_history", func(m map[string]any) { h := m["history"].([]any); m["history"] = append([]any{h[0]}, h...) }},
		{"invalid_history_ohlc", func(m map[string]any) { m["history"].([]any)[0].(map[string]any)["h"] = -1000 }},
		{"active_count_thirteen", func(m map[string]any) { checkpointCountdown(m)["count"] = 13 }},
		{"deferred_count_below_twelve", func(m map[string]any) { checkpointCountdown(m)["state"] = "deferred" }},
		{"completed_count_below_thirteen", func(m map[string]any) { checkpointCountdown(m)["state"] = "completed" }},
		{"unknown_countdown_state", func(m map[string]any) { checkpointCountdown(m)["state"] = "paused" }},
		{"nonnumeric_countdown_episode_suffix", func(m map[string]any) {
			checkpointCountdown(m)["setup_episode_id"] = "seq.full.public_approx.v1:SYN:5m:buy:not-a-timestamp"
		}},
		{"future_countdown_episode_suffix", func(m map[string]any) {
			checkpointCountdown(m)["setup_episode_id"] = fmt.Sprintf("seq.full.public_approx.v1:SYN:5m:buy:%d", next+300000)
		}},
		{"missing_countdown_identity", func(m map[string]any) { checkpointCountdown(m)["setup_episode_id"] = nil }},
		{"missing_count_five_reference", func(m map[string]any) { checkpointCountdown(m)["bar5_index"] = nil }},
		{"future_count_eight_reference", func(m map[string]any) { checkpointCountdown(m)["bar8_index"] = 99999 }},
		{"premature_count_thirteen_reference", func(m map[string]any) { checkpointCountdown(m)["bar13_index"] = 21 }},
		{"inconsistent_setup_active", func(m map[string]any) { m["buy"].(map[string]any)["setup_active"] = false }},
	}
	for _, key := range []string{"schema_version", "profile_id", "freeze_id", "config_hash", "source", "timeframe_ms", "state_version", "config", "last_bar_index", "last_open_ms", "last_event_seq", "init_diagnostics_emitted", "previous_comparison", "history", "buy", "sell"} {
		key := key
		mutations = append(mutations, mutation{"missing_" + key, func(m map[string]any) { delete(m, key) }})
	}
	assertInvalid := func(t *testing.T, b []byte) {
		t.Helper()
		restored, err := sc.Restore(b, id, next)
		var typed *sc.InvalidCheckpointError
		if restored != nil || !errors.As(err, &typed) {
			t.Fatalf("damaged checkpoint returned engine=%v, error=%T %v; want nil engine and *InvalidCheckpointError", restored, err, err)
		}
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			var wire map[string]any
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatal(err)
			}
			test.apply(wire)
			assertInvalid(t, jsonBytes(t, wire))
		})
	}
	for name, blob := range map[string][]byte{"empty": {}, "truncated": data[:len(data)/2], "trailing_json": append(append([]byte{}, data...), []byte(" {}")...), "null": []byte("null"), "array": []byte("[]")} {
		t.Run(name, func(t *testing.T) { assertInvalid(t, blob) })
	}
	// Invalid attempts do not poison the original serialized checkpoint.
	restored, err := sc.Restore(data, id, next)
	if err != nil || restored == nil {
		t.Fatalf("original checkpoint no longer reusable: %v", err)
	}
}
func checkpointCountdown(m map[string]any) map[string]any {
	return m["buy"].(map[string]any)["countdown"].(map[string]any)
}

func TestExhaustedCheckpointHeadersNeverWrapOrMutate(t *testing.T) {
	id := independentIdentity()
	e := newEngine(t, id)
	bars := independentSlide(22)
	if _, err := e.Run(bars); err != nil {
		t.Fatal(err)
	}
	data, _ := checkpointJSON(t, e)
	for _, test := range []struct{ name, field, value string }{
		{"event_sequence", "last_event_seq", "9223372036854775807"},
		{"bar_index", "last_bar_index", "9223372036854775806"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var wire map[string]any
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatal(err)
			}
			// json.Number is essential: float64 rounds these adjacent int64 boundaries.
			wire[test.field] = json.Number(test.value)
			exhausted := jsonBytes(t, wire)
			next := bars[len(bars)-1]
			next.OpenMS += 300000
			restored, err := sc.Restore(exhausted, id, next.OpenMS)
			if err != nil {
				var typed *sc.InvalidCheckpointError
				if restored != nil || !errors.As(err, &typed) {
					t.Fatalf("unsafe exhausted-state rejection: engine=%v err=%T %v", restored, err, err)
				}
				return
			}
			if restored == nil {
				t.Fatal("nil engine without error")
			}
			// Compare raw JSON bytes here too; decoding the boundary into float64
			// would hide a one-unit change in the counters we are protecting.
			before, err := restored.Checkpoint()
			if err != nil {
				t.Fatal(err)
			}
			snapshot, events, err := restored.Step(next)
			var typed *sc.InvalidInputError
			if snapshot != nil || len(events) != 0 || !errors.As(err, &typed) {
				t.Fatalf("exhausted Step must reject before output: snapshot=%+v events=%+v err=%T %v", snapshot, events, err, err)
			}
			after, err := restored.Checkpoint()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("exhausted Step mutated checkpoint\nbefore %s\nafter %s", before, after)
			}
		})
	}
}
