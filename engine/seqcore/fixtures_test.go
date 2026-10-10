package seqcore_test

// The oracle is the owner-accepted freeze and unchanged v2 JSON corpus. These
// tests use only the public API; no production transitions compute expectations.
import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"testing"

	sc "github.com/spik3r/heisentick-strat/engine/seqcore"
)

type fixtureFile struct {
	Schema           string        `json:"schema"`
	FreezeID         string        `json:"freeze_id"`
	EventSchema      string        `json:"event_schema"`
	CheckpointSchema string        `json:"checkpoint_schema"`
	ProfileID        string        `json:"profile_id"`
	Cases            []fixtureCase `json:"cases"`
}
type fixtureCase struct {
	ID     string `json:"id"`
	Side   string `json:"side"`
	Status string `json:"status"`
	Series struct {
		Symbol      string  `json:"symbol"`
		Timeframe   string  `json:"timeframe"`
		TimeframeMS int64   `json:"timeframe_ms"`
		StartOpenMS int64   `json:"start_open_ms"`
		OpenMS      []int64 `json:"open_ms"`
		GapPolicy   string  `json:"gap_policy"`
	} `json:"series"`
	Bars     []sc.Bar `json:"bars"`
	Expected struct {
		Events       []map[string]any          `json:"events"`
		Snapshots    map[string]map[string]any `json:"snapshots"`
		NeverEmitted []string                  `json:"never_emitted"`
		Config       json.RawMessage           `json:"config"`
		Error        map[string]any            `json:"error"`
		Restart      *struct {
			Boundaries            string  `json:"boundaries"`
			EventsIdentical       bool    `json:"events_identical_incl_event_seq"`
			NoRepeatedDiagnostics bool    `json:"init_diagnostics_not_repeated"`
			LastEventSeq          []int64 `json:"last_event_seq_after_bar"`
		} `json:"restart"`
		Checkpoint *struct {
			AfterBarIndex int            `json:"after_bar_index"`
			Saved         map[string]any `json:"saved"`
			ResumeWith    map[string]any `json:"resume_with"`
		} `json:"checkpoint"`
		Expect          map[string]any   `json:"expect"`
		AgeTable        *fixtureAgeTable `json:"age_table"`
		Windows         []fixtureWindow  `json:"windows"`
		InitDiagnostics map[string]any   `json:"init_diagnostics"`
	} `json:"expected"`
}

// These hashes pin the exact 24 source files at ab98cedc96c8415175b9658c4ea6491bfb5d6918.
var fixtureSHA256 = map[string]string{
	"bar_order_error.json":                     "97fb0cc4359c3707cb62096453fb936890cdb3e24694dd7f611899ef0e79341e",
	"checkpoint_identity.json":                 "ac4e4a84871136e86a4720e7ac5517fccca80eb06d072d58e259389cb4e8b327",
	"config_unsupported.json":                  "cb669752c1a1f93c15e87da11c26d9bcbc8e3dc3d2d0c060f55d1f1693e79dfe",
	"countdown_nonconsecutive.json":            "597f00d7667dba285e1f4920aa8af62583c617df73b1b41845553fcddcf3e087",
	"countdown_overlap.json":                   "f54d13860a4d7a15f632e8cd90de83a004aef3b8a54133c937b7c25f31f5a782",
	"countdown_start_on_setup9.json":           "0f29a2d1f9a27c861e0a98b40c9762a38e486d41390f938630c218f8b647a21a",
	"data_gap_reset.json":                      "277ff04d2c8909f363590722116a5853315744866239e2514b42e7254f1edc08",
	"delayed_perfection.json":                  "4528e77c2ee7d11aab314d9b37dc214ebc234a08667cc5d52eadfb3072dafc32",
	"eight_vs_five_off.json":                   "406f85563fd43537f43902be50fbfdbf9ba9c9a20b3dc387ba27dcfd4e5059c2",
	"equality_and_failed_flip.json":            "0aed398c01e009436acc4da4664ec281b381c2c0f7c4352eca44ee9289e52a6c",
	"event_order.json":                         "84dde7b5c9b1a1679e1564401cdc172a727a1c34bc5afbf72f851bab5d595eea",
	"inclusive_equality.json":                  "96626baeed271fec2a6974799151354279707b029cf2ca01d2d8ff928d8ea9c0",
	"independent_streams.json":                 "a4f2d2def41b41647d013a3423f08292bc39bdf52164618a1fd101e28e68d6a3",
	"init_diagnostics.json":                    "88521122645e264465487987252f3147fc45e99383f233ba34257dd52ce18a46",
	"opposite_setup_cancels_before_count.json": "5ec46c566425dd47d45bae7af013cbee5d713a64bd53035c4a0c2c28a806d4e7",
	"perfection_threshold.json":                "79c7e77ee322f1f6233318b4c7d7e14cfa296dd559d72c5ab8e9f2c9581a0480",
	"same_side_setup_replaces_pending.json":    "f90a57abbfa3a772b6b030a1c15d8ccd3fb62603ac1e487bfd709c54b493f666",
	"second_imperfect_setup.json":              "240e53996b7916b0e0cbed76a7974630deb1cfd5c3dfcd1dde1dd7903f0fc5e2",
	"setup_22_recycle.json":                    "07c626b6ef5a16e9d51a051c51e1cd8e802b51e490d2532c2ee8bbbe96de9d77",
	"tdst_unsupported.json":                    "ce286478245f26e190bcc1f6410fbc47786a9f9e95f14fcd066acec041cc9a9a",
	"terminal_13_deferral.json":                "c1c6fbc3e12de5c5210bd42c4de55db01b562bf100ff857f2458e94b2efbfec7",
	"terminal_13_setup_22_collision.json":      "831f52c4533c416c56a2febb4b63dbfc63aba3c4233b2dfb987c7912c9e2f50c",
	"warmup.json":                              "dd96c9d50224475cf8006378d906e5c352f9aa40ad5b86ec6c1753618b076d69",
	"window_policy.json":                       "d807a9561b72b47a153bf4b8f6e927851b428442a73bc6aedb0924dea2f4db5a",
}

func loadFixtures(t *testing.T) []fixtureCase {
	t.Helper()
	paths, err := filepath.Glob("testdata/seq-core-fixtures.v2/*.json")
	if err != nil {
		t.Fatal(err)
	}
	allPaths := paths
	paths = nil
	for _, path := range allPaths {
		if filepath.Base(path) != "SOURCES.json" {
			paths = append(paths, path)
		}
	}
	if len(paths) != 24 || len(fixtureSHA256) != 24 {
		t.Fatalf("fixture inventory: got %d files, want exactly 24", len(paths))
	}
	var cases []fixtureCase
	seen := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		expected, ok := fixtureSHA256[filepath.Base(path)]
		if !ok {
			t.Fatalf("unexpected fixture file %s", path)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != expected {
			t.Fatalf("fixture bytes changed: %s: got %s, want %s", path, got, expected)
		}
		var f fixtureFile
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if f.Schema != "seq-core-fixtures.v2" || f.FreezeID != sc.FreezeID || f.ProfileID != sc.ProfileID || f.EventSchema != sc.EventSchema || f.CheckpointSchema != sc.CheckpointSchema {
			t.Fatalf("wrong contract identity in %s", path)
		}
		for _, c := range f.Cases {
			if c.Status != "ready" {
				t.Fatalf("case %s is not ready: %s; corpus cases must never silently skip", c.ID, c.Status)
			}
			if seen[c.ID] {
				t.Fatalf("duplicate case id %s", c.ID)
			}
			seen[c.ID] = true
			if c.Series.GapPolicy != sc.GapPolicy {
				t.Fatalf("%s: wrong gap policy", c.ID)
			}
			if len(c.Series.OpenMS) != 0 && len(c.Series.OpenMS) != len(c.Bars) {
				t.Fatalf("%s: open-time length mismatch", c.ID)
			}
			for i := range c.Bars {
				c.Bars[i].OpenMS = c.Series.StartOpenMS + int64(i)*c.Series.TimeframeMS
				if len(c.Series.OpenMS) != 0 {
					c.Bars[i].OpenMS = c.Series.OpenMS[i]
				}
			}
			cases = append(cases, c)
		}
	}
	if len(cases) != 143 {
		t.Fatalf("got %d fixture cases, want 143", len(cases))
	}
	return cases
}

func caseNamed(t *testing.T, id string) fixtureCase {
	t.Helper()
	for _, c := range loadFixtures(t) {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("missing fixture %s", id)
	return fixtureCase{}
}
func fixtureIdentity(c fixtureCase) sc.Identity {
	return sc.DefaultIdentity(sc.Source{Symbol: c.Series.Symbol, Timeframe: c.Series.Timeframe}, "cfg-default", c.Series.TimeframeMS)
}
func newEngine(t *testing.T, id sc.Identity) *sc.Engine {
	t.Helper()
	e, err := sc.New(sc.DefaultConfig(), id)
	if err != nil {
		t.Fatal(err)
	}
	if e == nil {
		t.Fatal("New returned nil engine without error")
	}
	return e
}
func jsonValue(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var x any
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatal(err)
	}
	return x
}
func jsonBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func equalJSON(t *testing.T, label string, got, want any) {
	t.Helper()
	g, w := jsonValue(t, got), jsonValue(t, want)
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("%s\ngot:  %s\nwant: %s", label, jsonBytes(t, g), jsonBytes(t, w))
	}
}
func sparseJSON(t *testing.T, path string, got, want any) {
	t.Helper()
	wm, isMap := want.(map[string]any)
	if !isMap {
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %#v, want %#v", path, got, want)
		}
		return
	}
	gm, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("%s: got %T, want object", path, got)
	}
	keys := make([]string, 0, len(wm))
	for key := range wm {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, exists := gm[key]
		if !exists {
			t.Fatalf("%s.%s: missing field", path, key)
		}
		sparseJSON(t, path+"."+key, value, wm[key])
	}
}
func compareTrace(t *testing.T, label string, got, want sc.Trace) {
	t.Helper()
	if len(got.Events) != len(want.Events) || len(got.Snapshots) != len(want.Snapshots) {
		t.Fatalf("%s lengths: got events=%d snapshots=%d; want events=%d snapshots=%d", label, len(got.Events), len(got.Snapshots), len(want.Events), len(want.Snapshots))
	}
	for i := range want.Events {
		equalJSON(t, fmt.Sprintf("%s event %d", label, i), got.Events[i], want.Events[i])
	}
	for i := range want.Snapshots {
		equalJSON(t, fmt.Sprintf("%s snapshot %d", label, i), got.Snapshots[i], want.Snapshots[i])
	}
}
func prefixTrace(full sc.Trace, n int) sc.Trace {
	if n > len(full.Snapshots) {
		n = len(full.Snapshots)
	}
	out := sc.Trace{Snapshots: full.Snapshots[:n]}
	for _, event := range full.Events {
		if event.BarIndex < int64(n) {
			out.Events = append(out.Events, event)
		}
	}
	return out
}
func joinTrace(a, b sc.Trace) sc.Trace {
	return sc.Trace{Events: append(append([]sc.Event{}, a.Events...), b.Events...), Snapshots: append(append([]sc.Snapshot{}, a.Snapshots...), b.Snapshots...)}
}
func assertBarOrderError(t *testing.T, err error, kind string, index int64, previous, open int64) {
	t.Helper()
	var typed *sc.BarOrderError
	if !errors.As(err, &typed) {
		t.Fatalf("got %T %v, want *BarOrderError", err, err)
	}
	if typed.Kind != kind || typed.BarIndex != index || typed.PreviousOpenMS != previous || typed.OpenMS != open {
		t.Fatalf("wrong bar-order error: %+v; want %s index=%d previous=%d open=%d", typed, kind, index, previous, open)
	}
}
func checkpointJSON(t *testing.T, e *sc.Engine) ([]byte, map[string]any) {
	t.Helper()
	data, err := e.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("checkpoint is not JSON: %v", err)
	}
	// Re-encoding deliberately changes field ordering and whitespace. Restore must
	// actually deserialize the portable JSON state, not rely on in-memory objects.
	portable, err := json.MarshalIndent(wire, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return portable, wire
}

func TestFrozenFixtureCorpus(t *testing.T) {
	cases := loadFixtures(t)
	counts := map[string]int{}
	for _, c := range cases {
		category := "stream"
		switch {
		case c.Expected.Error["type"] == "UnsupportedConfigError":
			category = "unsupported"
		case c.Expected.Checkpoint != nil:
			category = "checkpoint"
		case c.Expected.AgeTable != nil:
			category = "age_table"
		}
		counts[category]++
		t.Run(c.ID, func(t *testing.T) {
			switch category {
			case "unsupported":
				testUnsupportedFixture(t, c)
			case "checkpoint":
				testCheckpointFixture(t, c)
			case "age_table":
				testAgeTable(t, *c.Expected.AgeTable)
			default:
				testStreamFixture(t, c)
			}
		})
	}
	equalJSON(t, "case coverage", counts, map[string]int{"stream": 96, "unsupported": 20, "checkpoint": 26, "age_table": 1})
}

func testUnsupportedFixture(t *testing.T, c fixtureCase) {
	t.Helper()
	config := sc.DefaultConfig()
	if err := json.Unmarshal(c.Expected.Config, &config); err != nil {
		t.Fatal(err)
	}
	e, err := sc.New(config, fixtureIdentity(c))
	var typed *sc.UnsupportedConfigError
	if e != nil || !errors.As(err, &typed) {
		t.Fatalf("construction got engine=%v error=%T %v; want nil and *UnsupportedConfigError", e, err, err)
	}
	equalJSON(t, "unsupported error", typed, map[string]any{"field": c.Expected.Error["field"], "value": c.Expected.Error["value"], "reason": "unsupported_config"})
	trace, err := sc.Run(config, fixtureIdentity(c), c.Bars)
	if !errors.As(err, &typed) || len(trace.Events) != 0 || len(trace.Snapshots) != 0 {
		t.Fatalf("unsupported batch consumed input: trace=%+v err=%v", trace, err)
	}
	// A rejected construction cannot consume the sequence or suppress diagnostics
	// in a subsequent independently constructed valid engine.
	fresh := newEngine(t, fixtureIdentity(c))
	_, events, err := fresh.Step(c.Bars[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].EventSeq != 1 || events[1].EventSeq != 2 {
		t.Fatalf("construction rejection consumed event sequence: %+v", events)
	}
}

func testStreamFixture(t *testing.T, c fixtureCase) {
	t.Helper()
	id := fixtureIdentity(c)
	e := newEngine(t, id)
	var stream sc.Trace
	var checkpoints [][]byte
	errorAt := -1
	kind := ""
	if value, ok := c.Expected.Error["at_bar_index"]; ok {
		errorAt = int(value.(float64))
		kind = c.Expected.Error["kind"].(string)
	}
	for i, b := range c.Bars {
		snapshot, events, err := e.Step(b)
		if i == errorAt {
			assertBarOrderError(t, err, kind, int64(i), c.Bars[i-1].OpenMS, b.OpenMS)
			if snapshot != nil || len(events) != 0 {
				t.Fatalf("failed bar emitted data: snapshot=%+v events=%+v", snapshot, events)
			}
			for _, later := range c.Bars[i+1:] {
				s, ev, terminalErr := e.Step(later)
				if terminalErr == nil || s != nil || len(ev) != 0 {
					t.Fatalf("terminal engine accepted later bar: snapshot=%+v events=%+v err=%v", s, ev, terminalErr)
				}
			}
			break
		}
		if err != nil {
			t.Fatalf("bar %d: %v", i, err)
		}
		if snapshot == nil {
			t.Fatalf("bar %d: nil snapshot", i)
		}
		stream.Snapshots = append(stream.Snapshots, *snapshot)
		stream.Events = append(stream.Events, events...)
		data, wire := checkpointJSON(t, e)
		checkpoints = append(checkpoints, data)
		if wire["last_event_seq"] != float64(len(stream.Events)) || wire["last_open_ms"] != float64(b.OpenMS) || wire["init_diagnostics_emitted"] != true {
			t.Fatalf("bar %d: wrong checkpoint header %v", i, wire)
		}
		if c.Expected.Restart == nil || i >= len(c.Expected.Restart.LastEventSeq) {
			t.Fatalf("missing restart oracle at bar %d", i)
		}
		if wire["last_event_seq"] != float64(c.Expected.Restart.LastEventSeq[i]) {
			t.Fatalf("bar %d event_seq differs from fixture", i)
		}
	}
	if c.Expected.Restart == nil || c.Expected.Restart.Boundaries != "every_bar" || !c.Expected.Restart.EventsIdentical || !c.Expected.Restart.NoRepeatedDiagnostics || len(c.Expected.Restart.LastEventSeq) != len(stream.Snapshots) {
		t.Fatal("missing or incomplete restart requirements")
	}
	if len(stream.Events) != len(c.Expected.Events) {
		t.Fatalf("complete stream: got %d events, want %d", len(stream.Events), len(c.Expected.Events))
	}
	for i, want := range c.Expected.Events {
		expected := make(map[string]any, len(want))
		for key, value := range want {
			if key != "optional_detail" {
				expected[key] = value
			}
		}
		equalJSON(t, fmt.Sprintf("fixture event %d", i), stream.Events[i], expected)
	}
	for _, event := range stream.Events {
		for _, forbidden := range c.Expected.NeverEmitted {
			if event.Type == forbidden {
				t.Fatalf("emitted forbidden event %s", forbidden)
			}
		}
	}
	for key, want := range c.Expected.Snapshots {
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 || i >= len(stream.Snapshots) {
			t.Fatalf("invalid snapshot oracle index %s", key)
		}
		sparseJSON(t, "snapshot["+key+"]", jsonValue(t, stream.Snapshots[i]), want)
	}
	for i, s := range stream.Snapshots {
		if s.BarIndex != int64(i) || s.BarOpenMS != c.Bars[i].OpenMS || s.DecisionMS != c.Bars[i].OpenMS+c.Series.TimeframeMS {
			t.Fatalf("snapshot %d timestamp/index mismatch: %+v", i, s)
		}
		if s.Buy.SetupActive != (s.Buy.SetupRun > 0) || s.Sell.SetupActive != (s.Sell.SetupRun > 0) {
			t.Fatalf("snapshot %d setup_active does not reflect setup_run", i)
		}
	}
	checkRunError := func(t *testing.T, err error, n int) {
		t.Helper()
		if errorAt >= 0 && n > errorAt {
			assertBarOrderError(t, err, kind, int64(errorAt), c.Bars[errorAt-1].OpenMS, c.Bars[errorAt].OpenMS)
		} else if err != nil {
			t.Fatal(err)
		}
	}
	batch, err := sc.Run(sc.DefaultConfig(), id, c.Bars)
	checkRunError(t, err, len(c.Bars))
	compareTrace(t, "package Run", batch, stream)
	batch, err = newEngine(t, id).Run(c.Bars)
	checkRunError(t, err, len(c.Bars))
	compareTrace(t, "engine Run", batch, stream)
	for n := 0; n <= len(c.Bars); n++ {
		t.Run(fmt.Sprintf("prefix_%d", n), func(t *testing.T) {
			got, err := sc.Run(sc.DefaultConfig(), id, c.Bars[:n])
			checkRunError(t, err, n)
			compareTrace(t, "prefix", got, prefixTrace(stream, n))
		})
	}
	for k, data := range checkpoints {
		n := k + 1
		t.Run(fmt.Sprintf("restart_after_%d", k), func(t *testing.T) {
			nextOpen := c.Bars[k].OpenMS + c.Series.TimeframeMS
			if n < len(c.Bars) {
				nextOpen = c.Bars[n].OpenMS
			}
			restored, err := sc.Restore(data, id, nextOpen)
			if n == errorAt {
				if restored != nil {
					t.Fatal("Restore returned engine for invalid next time")
				}
				assertRestoreOrderError(t, restored, err, kind)
				corrected := c.Bars[n]
				corrected.OpenMS = int64(c.Expected.Error["resume_open_ms_for_bar_15"].(float64))
				restored, err = sc.Restore(data, id, corrected.OpenMS)
				if err != nil {
					t.Fatalf("pre-error checkpoint no longer usable: %v", err)
				}
				s, ev, err := restored.Step(corrected)
				if err != nil || s == nil {
					t.Fatalf("repaired continuation: %v", err)
				}
				control := newEngine(t, id)
				before, err := control.Run(c.Bars[:n])
				if err != nil {
					t.Fatal(err)
				}
				cs, ce, err := control.Step(corrected)
				if err != nil {
					t.Fatal(err)
				}
				equalJSON(t, "reusable checkpoint snapshot", s, cs)
				equalJSON(t, "reusable checkpoint events", ev, ce)
				compareTrace(t, "unconsumed prefix", before, prefixTrace(stream, n))
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if restored == nil {
				t.Fatal("nil restored engine")
			}
			equalJSON(t, "restored identity", restored.Identity(), id)
			rest, err := restored.Run(c.Bars[n:])
			checkRunError(t, err, len(c.Bars))
			compareTrace(t, "checkpoint continuation", joinTrace(prefixTrace(stream, n), rest), stream)
		})
	}
	if c.Expected.InitDiagnostics != nil {
		assertDiagnostics(t, stream.Events)
	}
	testWindowProbes(t, c, stream)
}

func assertDiagnostics(t *testing.T, events []sc.Event) {
	t.Helper()
	var diagnostics []sc.Event
	for _, e := range events {
		if e.Type == "diagnostic" {
			diagnostics = append(diagnostics, e)
		}
	}
	if len(diagnostics) != 2 {
		t.Fatalf("got %d diagnostics, want exactly two", len(diagnostics))
	}
	for i, reason := range []string{"tdst_cancel_unsupported", "unsupported_public_formula"} {
		e := diagnostics[i]
		if e.EventSeq != int64(i+1) || e.BarIndex != 0 || e.Phase != "P0" || e.Side != "both" || e.Reason != reason {
			t.Fatalf("wrong initialization diagnostic %+v", e)
		}
	}
}
