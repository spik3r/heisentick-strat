package frozeninput_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/frozeninput"
	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

type packet struct {
	cfg       dsl.Config
	docs      frozeninput.Documents
	artifacts []frozeninput.Artifact
}

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func sha(b []byte) string { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func value(t *testing.T, b []byte) any {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if e := d.Decode(&v); e != nil {
		t.Fatal(e)
	}
	return v
}
func object(t *testing.T, b []byte) map[string]any { return value(t, b).(map[string]any) }
func canonical(t *testing.T, v any) []byte         { return marshal(t, value(t, marshal(t, v))) }
func fresh(t *testing.T) packet {
	t.Helper()
	cfg, e := dsl.DecodeFrozenLevelConfigJSON(read(t, "compiled-config.canonical.json"))
	if e != nil {
		t.Fatal(e)
	}
	return packet{cfg: cfg, docs: frozeninput.Documents{Snapshot: read(t, "quote-snapshot.raw.json"), Source: read(t, "source-manifest.raw.json"), Ordering: read(t, "ordering-manifest.raw.json")}, artifacts: []frozeninput.Artifact{{Ref: dsl.FrozenQuoteRef{ID: "FixtureRawQuotes", Version: "v1", SHA256: "ed18adb1eec58a7813436df138f83bd84d87f7da0fa47c7ba7b1e1e65635aadf"}, Bytes: read(t, "raw-quotes.csv")}}}
}
func configRefs(p *packet) map[string]any {
	return p.cfg["frozenLevelBreakout"].(map[string]any)["inputRefs"].(map[string]any)
}
func ref(id, version string, b []byte) map[string]any {
	return map[string]any{"id": id, "version": version, "sha256": sha(b)}
}
func refMap(r dsl.FrozenQuoteRef) map[string]any {
	return map[string]any{"id": r.ID, "version": r.Version, "sha256": r.SHA256}
}

// Rebind descendants after an intentional source/order mutation. Tests that
// change a semantic field therefore reach that field rather than a stale hash.
func (p *packet) bindSource(t *testing.T, s, o map[string]any) {
	p.docs.Source = marshal(t, s)
	sr := ref(s["id"].(string), s["version"].(string), p.docs.Source)
	configRefs(p)["source"] = sr
	o["sourceRef"] = sr
	for _, v := range o["contextBindings"].([]any) {
		v.(map[string]any)["sourceRef"] = sr
	}
	p.bindOrdering(t, o)
}
func (p *packet) bindOrdering(t *testing.T, o map[string]any) {
	p.docs.Ordering = marshal(t, o)
	configRefs(p)["ordering"] = ref(o["id"].(string), o["version"].(string), p.docs.Ordering)
}
func (p *packet) bindSnapshot(t *testing.T, q, s, o map[string]any) {
	p.docs.Snapshot = marshal(t, q)
	decoded, e := dsl.DecodeFrozenQuoteSnapshotJSON(p.docs.Snapshot)
	if e != nil {
		t.Fatal(e)
	}
	qr := ref(decoded.ID, decoded.Version, p.docs.Snapshot)
	s["snapshotRef"] = qr
	s["normalizedSnapshotSHA256"] = sha(decoded.NormalizedBytes)
	o["snapshotRef"] = qr
	rows := q["rows"].([]any)
	first, last := rows[0].(map[string]any), rows[len(rows)-1].(map[string]any)
	x := o["scope"].(map[string]any)
	x["rowCount"] = len(rows)
	x["firstEvent"] = map[string]any{"eventMS": first["eventMS"], "sequence": first["sequence"]}
	x["lastEvent"] = map[string]any{"eventMS": last["eventMS"], "sequence": last["sequence"]}
	p.bindSource(t, s, o)
}
func maps(t *testing.T, p packet) (map[string]any, map[string]any, map[string]any) {
	return object(t, p.docs.Snapshot), object(t, p.docs.Source), object(t, p.docs.Ordering)
}
func admit(t *testing.T, p packet) *frozeninput.Input {
	t.Helper()
	in, e := frozeninput.Decode(p.cfg, p.docs, p.artifacts)
	if e != nil {
		t.Fatal(e)
	}
	return in
}
func reject(t *testing.T, p packet, category string) string {
	t.Helper()
	in, e := frozeninput.Decode(p.cfg, p.docs, p.artifacts)
	if in != nil || e == nil || !strings.Contains(e.Error(), category) {
		t.Fatalf("want atomic %s refusal, got input=%v error=%v", category, in, e)
	}
	return e.Error()
}
func projection(t *testing.T, in *frozeninput.Input) []byte {
	t.Helper()
	v, e := in.View()
	if e != nil {
		t.Fatal(e)
	}
	return canonical(t, v)
}

func TestFrozenInputLiteralVector(t *testing.T) {
	p := fresh(t)
	in := admit(t, p)
	id := in.Identity()
	if id.ConfigDigest != "9dcc7dbfb02ce50726b80cfc44f32e91ebef40cde6ec5014aa7cb04bbcf0e401" || id.PolicyDigest != "8a1ec4caa02f621b26e15b0f6be339e5cbb2a12fe3b9fb9149daf5159dd72623" {
		t.Fatal("literal config/policy digest mismatch")
	}
	if !bytes.Equal(id.ConfigBytes, read(t, "compiled-config.canonical.json")) || !bytes.Equal(id.PolicyBytes, read(t, "compiled-policy.canonical.json")) {
		t.Fatal("literal compiled bytes mismatch")
	}
	if !bytes.Equal(in.SnapshotDeclaration().NormalizedBytes, read(t, "quote-snapshot.normalized.json")) {
		t.Fatal("normalized snapshot differs")
	}
	if got := projection(t, in); !bytes.Equal(got, read(t, "expected-projection.canonical.json")) {
		t.Fatalf("projection mismatch: %s", got)
	}
	if in.Len() != 4 {
		t.Fatal("lost a quote")
	}
	a, _ := in.Observation(1)
	b, _ := in.Observation(2)
	if a.Quote.Event.AtMillis != b.Quote.Event.AtMillis || a.Quote.Event.Sequence >= b.Quote.Event.Sequence || a.Quote.Ask != 100.5 || b.Quote.Ask != 100.75 {
		t.Fatal("paired same-time observations changed")
	}
	s, ok := in.Source()
	if !ok || s.Side != frozenlevels.Bid || s.SHA256 != sha(p.docs.Source) || s.SHA256 == sha(p.docs.Snapshot) {
		t.Fatal("wrong primitive source projection")
	}
	total := len(id.ConfigBytes) + len(p.docs.Snapshot) + len(p.docs.Source) + len(p.docs.Ordering) + len(p.artifacts[0].Bytes)
	if total != 8674 {
		t.Fatalf("literal budget %d", total)
	}
	manifest := object(t, read(t, "DIGESTS.json"))
	for name, v := range manifest["files"].(map[string]any) {
		m := v.(map[string]any)
		raw := read(t, name)
		if sha(raw) != m["sha256"] || fmt.Sprint(len(raw)) != string(m["bytes"].(json.Number)) {
			t.Fatalf("frozen file identity %s", name)
		}
	}
	fmt.Printf("FROZEN_INPUT_VECTOR=%s\n", projection(t, in))
}

func TestFrozenInputConfigRevalidation(t *testing.T) {
	for _, which := range []string{"legacy", "missing", "unknown", "changed-profile"} {
		t.Run(which, func(t *testing.T) {
			p := fresh(t)
			if _, e := dsl.NormalizeFrozenLevelConfig(p.cfg); e != nil {
				t.Fatal(e)
			}
			switch which {
			case "legacy":
				p.cfg["setupType"] = "flagContinuation"
			case "missing":
				delete(p.cfg, "name")
			case "unknown":
				p.cfg["extra"] = true
			case "changed-profile":
				p.cfg["frozenLevelBreakout"].(map[string]any)["profile"].(map[string]any)["extra"] = true
			}
			reject(t, p, "input-config")
		})
	}
	p := fresh(t)
	old := admit(t, p).Identity()
	p.cfg["name"] = "Synthetic changed metadata"
	now := admit(t, p).Identity()
	if old.ConfigDigest == now.ConfigDigest || old.PolicyDigest != now.PolicyDigest {
		t.Fatal("metadata identity semantics changed")
	}
}

func TestFrozenInputRelationalFailures(t *testing.T) {
	cases := []struct {
		name, category string
		mutate         func(*packet, map[string]any, map[string]any, map[string]any)
	}{
		{"dataset", "input-identity", func(p *packet, q, s, o map[string]any) { s["dataset"] = "Other"; p.bindSource(t, s, o) }},
		{"normalization-hash", "input-identity", func(p *packet, q, s, o map[string]any) {
			s["normalizedSnapshotSHA256"] = strings.Repeat("a", 64)
			p.bindSource(t, s, o)
		}},
		{"calendar-ref", "input-identity", func(p *packet, q, s, o map[string]any) {
			o["calendarRef"].(map[string]any)["id"] = "Other"
			p.bindOrdering(t, o)
		}},
		{"source-id", "input-identity", func(p *packet, q, s, o map[string]any) {
			p.docs.Source = bytes.Replace(p.docs.Source, []byte("FixtureQuoteSource"), []byte("OtherQuoteSource"), 1)
		}},
		{"raw-LF", "input-artifact", func(p *packet, q, s, o map[string]any) { p.artifacts[0].Bytes = append(p.artifacts[0].Bytes, '\n') }},
		{"missing-leaf", "input-artifact", func(p *packet, q, s, o map[string]any) { p.artifacts = nil }},
		{"extra-leaf", "input-artifact", func(p *packet, q, s, o map[string]any) { p.artifacts = append(p.artifacts, p.artifacts[0]) }},
		{"byte-count", "input-artifact", func(p *packet, q, s, o map[string]any) {
			s["rawArtifact"].(map[string]any)["byteCount"] = 9007199254740991
			p.bindSource(t, s, o)
		}},
		{"record-count", "input-artifact", func(p *packet, q, s, o map[string]any) {
			s["rawArtifact"].(map[string]any)["recordCount"] = 9007199254740991
			p.bindSource(t, s, o)
		}},
		{"nonjoining-warmup", "input-windows", func(p *packet, q, s, o map[string]any) {
			s["windows"].(map[string]any)["warmup"].(map[string]any)["endMS"] = 999
			p.bindSource(t, s, o)
		}},
		{"outside-evaluation", "input-windows", func(p *packet, q, s, o map[string]any) {
			r := q["rows"].([]any)[3].(map[string]any)
			r["eventMS"] = 2000
			r["availableMS"] = 2000
			p.bindSnapshot(t, q, s, o)
		}},
		{"delayed-availability", "input-availability", func(p *packet, q, s, o map[string]any) {
			q["rows"].([]any)[0].(map[string]any)["availableMS"] = 1000
			p.bindSnapshot(t, q, s, o)
		}},
		{"duplicate-event", "input-sequence", func(p *packet, q, s, o map[string]any) {
			q["rows"].([]any)[2].(map[string]any)["sequence"] = 1
			p.bindSnapshot(t, q, s, o)
		}},
		{"reverse-event", "input-sequence", func(p *packet, q, s, o map[string]any) {
			r := q["rows"].([]any)[2].(map[string]any)
			r["eventMS"] = 998
			r["availableMS"] = 998
			p.bindSnapshot(t, q, s, o)
		}},
		{"ordinal-index", "input-sequence", func(p *packet, q, s, o map[string]any) {
			q["rows"].([]any)[1].(map[string]any)["sourceRecord"].(map[string]any)["recordIndex"] = 0
			p.bindSnapshot(t, q, s, o)
		}},
		{"locator-version", "input-sequence", func(p *packet, q, s, o map[string]any) {
			q["rows"].([]any)[1].(map[string]any)["sourceRecord"].(map[string]any)["artifactVersion"] = "v2"
			p.bindSnapshot(t, q, s, o)
		}},
		{"method", "input-sequence", func(p *packet, q, s, o map[string]any) {
			o["sequence"].(map[string]any)["method"] = "provider-value"
			o["sequence"].(map[string]any)["counterPolicy"] = "global-strict"
			p.bindOrdering(t, o)
		}},
		{"decoder", "input-sequence", func(p *packet, q, s, o map[string]any) {
			o["sequence"].(map[string]any)["decoder"].(map[string]any)["id"] = "OtherDecoder"
			p.bindOrdering(t, o)
		}},
		{"scope-count", "input-sequence", func(p *packet, q, s, o map[string]any) {
			o["scope"].(map[string]any)["rowCount"] = 9007199254740991
			p.bindOrdering(t, o)
		}},
		{"scope-first", "input-sequence", func(p *packet, q, s, o map[string]any) {
			o["scope"].(map[string]any)["firstEvent"].(map[string]any)["eventMS"] = 998
			p.bindOrdering(t, o)
		}},
		{"unresolved-evidence", "input-evidence", func(p *packet, q, s, o map[string]any) {
			o["sequence"].(map[string]any)["evidenceRefs"].([]any)[0].(map[string]any)["id"] = "Missing"
			p.bindOrdering(t, o)
		}},
		{"coverage-hole", "input-coverage", func(p *packet, q, s, o map[string]any) {
			s["coverageSegments"].([]any)[1].(map[string]any)["startMS"] = 1001
			p.bindSource(t, s, o)
		}},
		{"coverage-overlap", "input-coverage", func(p *packet, q, s, o map[string]any) {
			s["coverageSegments"].([]any)[1].(map[string]any)["startMS"] = 999
			p.bindSource(t, s, o)
		}},
		{"coverage-incomplete", "input-coverage", func(p *packet, q, s, o map[string]any) {
			a := s["coverageSegments"].([]any)
			s["coverageSegments"] = a[:len(a)-1]
			p.bindSource(t, s, o)
		}},
		{"closure-with-raw", "input-evidence", func(p *packet, q, s, o map[string]any) {
			s["coverageSegments"].([]any)[1].(map[string]any)["status"] = "closure"
			p.bindSource(t, s, o)
		}},
		{"gap-with-raw", "input-evidence", func(p *packet, q, s, o map[string]any) {
			s["coverageSegments"].([]any)[1].(map[string]any)["status"] = "gap"
			p.bindSource(t, s, o)
		}},
	}
	errors := make(map[string]string)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := fresh(t)
			q, s, o := maps(t, p)
			c.mutate(&p, q, s, o)
			errors[c.name] = reject(t, p, c.category)
		})
	}
	fmt.Printf("FROZEN_INPUT_ERRORS=%s\n", marshal(t, errors))
}

func provider(t *testing.T, policy string, seq []int64) packet {
	p := fresh(t)
	q, s, o := maps(t, p)
	q["sequenceMethod"] = "provider-value"
	for i, v := range q["rows"].([]any) {
		row := v.(map[string]any)
		delete(row, "sourceRecord")
		row["sequence"] = seq[i]
	}
	x := o["sequence"].(map[string]any)
	x["method"] = "provider-value"
	x["counterPolicy"] = policy
	p.bindSnapshot(t, q, s, o)
	return p
}
func TestFrozenInputProviderSequence(t *testing.T) {
	for _, p := range []packet{provider(t, "global-strict", []int64{7, 8, 9, 10}), provider(t, "per-event-millisecond", []int64{7, 0, 1, 0})} {
		in := admit(t, p)
		if in.Len() != 4 || in.SnapshotDeclaration().Rows[0].SourceRecord != nil {
			t.Fatal("provider projection changed")
		}
	}
	reject(t, provider(t, "global-strict", []int64{7, 0, 1, 2}), "input-sequence")
	reject(t, provider(t, "per-event-millisecond", []int64{7, 0, 0, 2}), "input-sequence")
	p := fresh(t)
	_, s, o := maps(t, p)
	o["sequence"].(map[string]any)["evidenceState"] = "unqualified"
	o["sequence"].(map[string]any)["evidenceRefs"] = []any{}
	p.bindSource(t, s, o)
	in := admit(t, p)
	if in.Evidence().SequenceEvidenceState != "unqualified" || in.Evidence().SourceOrderVerified || in.Evidence().RunReady {
		t.Fatal("unqualified order upgraded")
	}
}

func rich(t *testing.T) packet {
	p := fresh(t)
	_, s, o := maps(t, p)
	support := []any{}
	for _, kind := range []string{"coverage", "closure", "ordering"} {
		raw := []byte{0, 255, 1, 2}
		r := dsl.FrozenQuoteRef{ID: "Synthetic-" + kind, Version: "v1", SHA256: sha(raw)}
		p.artifacts = append(p.artifacts, frozeninput.Artifact{Ref: r, Bytes: raw})
		support = append(support, map[string]any{"ref": refMap(r), "kind": kind, "scope": map[string]any{"startMS": 0, "endMS": 2000}})
	}
	s["supportingArtifacts"] = support
	segments := s["coverageSegments"].([]any)
	for _, item := range []struct {
		index  int
		status string
		leaf   int
	}{{2, "gap", 1}, {4, "closure", 2}} {
		m := segments[item.index].(map[string]any)
		m["status"] = item.status
		m["evidenceRefs"] = []any{refMap(p.artifacts[item.leaf].Ref)}
	}
	o["sequence"].(map[string]any)["evidenceRefs"] = []any{refMap(p.artifacts[3].Ref)}
	contexts := []any{}
	for i, kind := range []string{"stored-clock-M30", "rolling-six-M5", "completed-clock-M15", "prior-broker-opening"} {
		contexts = append(contexts, map[string]any{"windowId": fmt.Sprintf("Window%d", i), "kind": kind, "sourceRef": o["sourceRef"], "calendarRef": o["calendarRef"], "sourceSide": "bid", "interval": map[string]any{"startMS": 0, "endMS": 500 + i}, "evidenceState": "unqualified", "evidenceRefs": []any{refMap(p.artifacts[0].Ref)}})
	}
	o["contextBindings"] = contexts
	p.bindSource(t, s, o)
	return p
}
func TestFrozenInputEvidenceAndContexts(t *testing.T) {
	p := rich(t)
	in := admit(t, p)
	if len(in.OrderingDeclaration().ContextBindings) != 4 || in.Evidence().CompletedContext != "unqualified" || in.Evidence().ContinuityVerified {
		t.Fatal("context or continuity upgraded")
	}
	for _, which := range []string{"quote-in-gap", "quote-in-closure", "insufficient-scope", "duplicate-leaf", "duplicate-context", "context-scope", "context-source", "context-calendar", "context-qualified", "missing-closure-evidence"} {
		t.Run(which, func(t *testing.T) {
			p := rich(t)
			q, s, o := maps(t, p)
			category := "input-context"
			switch which {
			case "quote-in-gap", "quote-in-closure":
				idx := 1
				status, leaf := "gap", 1
				if which == "quote-in-closure" {
					status, leaf = "closure", 2
				}
				m := s["coverageSegments"].([]any)[idx].(map[string]any)
				m["status"] = status
				m["evidenceRefs"] = []any{refMap(p.artifacts[leaf].Ref)}
				category = "input-coverage"
			case "insufficient-scope":
				s["supportingArtifacts"].([]any)[2].(map[string]any)["scope"].(map[string]any)["endMS"] = 1000
				category = "input-evidence"
			case "duplicate-leaf":
				s["supportingArtifacts"].([]any)[0].(map[string]any)["ref"] = refMap(p.artifacts[0].Ref)
				category = "input-artifact"
			case "duplicate-context":
				a := o["contextBindings"].([]any)
				a[1].(map[string]any)["windowId"] = "Window0"
			case "context-scope":
				o["contextBindings"].([]any)[0].(map[string]any)["interval"].(map[string]any)["endMS"] = 2001
			case "context-source":
				o["contextBindings"].([]any)[0].(map[string]any)["sourceRef"] = ref("Other", "v1", []byte("other"))
				p.bindOrdering(t, o)
				reject(t, p, category)
				return
			case "context-calendar":
				o["contextBindings"].([]any)[0].(map[string]any)["calendarRef"] = ref("Other", "v1", []byte("other"))
			case "context-qualified":
				o["contextBindings"].([]any)[0].(map[string]any)["evidenceState"] = "verified"
				category = "input-ordering"
			case "missing-closure-evidence":
				s["coverageSegments"].([]any)[4].(map[string]any)["evidenceRefs"] = []any{}
				category = "input-source"
			}
			_ = q
			p.bindSource(t, s, o)
			reject(t, p, category)
		})
	}
}

func TestFrozenInputOwnershipAndZeroValue(t *testing.T) {
	p := rich(t)
	in := admit(t, p)
	before := projection(t, in)
	id := in.Identity()
	docs := in.Documents()
	snapshot := in.SnapshotDeclaration()
	source := in.SourceDeclaration()
	ordering := in.OrderingDeclaration()
	artifacts := in.Artifacts()
	cfg, e := in.Config()
	if e != nil {
		t.Fatal(e)
	}
	view, e := in.View()
	if e != nil {
		t.Fatal(e)
	}
	p.cfg["name"] = "mutated"
	p.cfg["frozenLevelBreakout"].(map[string]any)["profile"].(map[string]any)["mutated"] = true
	p.docs.Snapshot[0] = '!'
	p.docs.Source[0] = '!'
	p.docs.Ordering[0] = '!'
	p.artifacts[0].Bytes[0] = '!'
	p.artifacts[0].Ref.ID = "mutated"
	id.ConfigBytes[0] = '!'
	id.PolicyBytes[0] = '!'
	docs.Snapshot[0] = '!'
	docs.Source[0] = '!'
	docs.Ordering[0] = '!'
	snapshot.NormalizedBytes[0] = '!'
	snapshot.Rows[0].SourceRecord.RecordIndex = 999
	snapshot.Rows[0].Bid = 999
	source.SupportingArtifacts[0].Ref.ID = "mutated"
	source.CoverageSegments[1].EvidenceRefs[0].ID = "mutated"
	ordering.Sequence.EvidenceRefs[0].ID = "mutated"
	ordering.ContextBindings[0].EvidenceRefs[0].ID = "mutated"
	artifacts[0].Bytes[0] = '!'
	cfg["name"] = "mutated"
	cfg["frozenLevelBreakout"].(map[string]any)["inputRefs"].(map[string]any)["source"].(map[string]any)["id"] = "mutated"
	view.CoverageSegments[1].EvidenceRefs[0].ID = "mutated"
	view.ContextBindings[0].EvidenceRefs[0].ID = "mutated"
	view.Observations[0].Quote.Ask = 999
	if !bytes.Equal(before, projection(t, in)) || in.Identity().ConfigDigest != id.ConfigDigest || in.Documents().Snapshot[0] == '!' || in.Artifacts()[0].Bytes[0] == '!' || in.SnapshotDeclaration().Rows[0].SourceRecord.RecordIndex != 0 {
		t.Fatal("retained alias mutation")
	}
	for _, index := range []int{-1, 4, 999} {
		if _, ok := in.Observation(index); ok {
			t.Fatal("invented out-of-range observation")
		}
	}
	for _, zero := range []*frozeninput.Input{nil, {}} {
		if zero.Len() != 0 || zero.Evidence().Status != "" || zero.OrderingProof() != "" {
			t.Fatal("zero value admitted")
		}
		if _, ok := zero.Source(); ok {
			t.Fatal("zero source admitted")
		}
		if _, ok := zero.Observation(0); ok {
			t.Fatal("zero quote admitted")
		}
		if _, e := zero.View(); e == nil {
			t.Fatal("zero view admitted")
		}
		if _, e := zero.Config(); e == nil {
			t.Fatal("zero config admitted")
		}
	}
}

func TestFrozenInputBudgetAndMaximumTime(t *testing.T) {
	p := fresh(t)
	in := admit(t, p)
	size := len(in.Identity().ConfigBytes) + len(p.docs.Snapshot) + len(p.docs.Source) + len(p.docs.Ordering) + len(p.artifacts[0].Bytes)
	// Snapshot whitespace is semantically inert but part of its raw identity.
	p.docs.Snapshot = append(p.docs.Snapshot, bytes.Repeat([]byte(" "), frozeninput.MaxInputBytes-size)...)
	_, s, o := maps(t, p)
	qr := ref("FixtureSnapshot", "v1", p.docs.Snapshot)
	s["snapshotRef"] = qr
	o["snapshotRef"] = qr
	p.bindSource(t, s, o)
	// Source/order were originally newline-terminated, while the mutation helper
	// emits canonical no-LF maps. Restore their two bytes via snapshot padding.
	for {
		id, e := dsl.FrozenLevelIdentity(p.cfg)
		if e != nil {
			t.Fatal(e)
		}
		n := len(id.ConfigBytes) + len(p.docs.Snapshot) + len(p.docs.Source) + len(p.docs.Ordering) + len(p.artifacts[0].Bytes)
		if n == frozeninput.MaxInputBytes {
			break
		}
		if n > frozeninput.MaxInputBytes {
			t.Fatal("budget setup overflow")
		}
		p.docs.Snapshot = append(p.docs.Snapshot, bytes.Repeat([]byte(" "), frozeninput.MaxInputBytes-n)...)
		qr = ref("FixtureSnapshot", "v1", p.docs.Snapshot)
		s["snapshotRef"] = qr
		o["snapshotRef"] = qr
		p.bindSource(t, s, o)
	}
	admit(t, p)
	p.docs.Snapshot = append(p.docs.Snapshot, ' ')
	reject(t, p, "input-size")
	p = fresh(t)
	q, s, o := maps(t, p)
	r := q["rows"].([]any)[3].(map[string]any)
	r["eventMS"] = json.Number("9007199254740991")
	r["availableMS"] = json.Number("9007199254740991")
	s["windows"].(map[string]any)["evaluation"].(map[string]any)["endMS"] = json.Number("9007199254740991")
	p.bindSnapshot(t, q, s, o)
	reject(t, p, "input-windows")
}

func TestFrozenInputEmptyWarmupAndSafeEndpoint(t *testing.T) {
	p := fresh(t)
	q, s, o := maps(t, p)
	first := q["rows"].([]any)[0].(map[string]any)
	first["eventMS"], first["availableMS"] = 1000, 1000
	s["windows"].(map[string]any)["warmup"] = map[string]any{"startMS": 1000, "endMS": 1000}
	s["coverageSegments"] = s["coverageSegments"].([]any)[1:]
	o["scope"].(map[string]any)["startMS"] = 1000
	p.bindSnapshot(t, q, s, o)
	if in := admit(t, p); in.Evidence().CompletedContext != "unqualified" || in.Evidence().RunReady {
		t.Fatal("empty warmup upgraded")
	}

	const maximum int64 = 9007199254740991
	p = fresh(t)
	q, s, o = maps(t, p)
	for i, v := range q["rows"].([]any) {
		row := v.(map[string]any)
		at := maximum - 3
		if i == 1 || i == 2 {
			at = maximum - 2
		}
		if i == 3 {
			at = maximum - 1
		}
		row["eventMS"], row["availableMS"] = at, at
	}
	s["windows"] = map[string]any{"warmup": map[string]any{"startMS": maximum - 3, "endMS": maximum - 2}, "evaluation": map[string]any{"startMS": maximum - 2, "endMS": maximum}}
	s["coverageSegments"] = []any{map[string]any{"startMS": maximum - 3, "endMS": maximum, "status": "unknown", "evidenceRefs": []any{}}}
	o["scope"].(map[string]any)["startMS"], o["scope"].(map[string]any)["endMS"] = maximum-3, maximum
	p.bindSnapshot(t, q, s, o)
	in := admit(t, p)
	last, ok := in.Observation(3)
	if !ok || last.Quote.Event.AtMillis != maximum-1 || in.SourceDeclaration().Windows.Evaluation.EndMS != maximum {
		t.Fatal("safe endpoint lost integer precision")
	}
}

// Deterministic key order makes mutation traces identical across Go targets.
func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
