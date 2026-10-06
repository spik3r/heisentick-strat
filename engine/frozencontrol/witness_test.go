package frozencontrol

// All witness construction and authority implementations stay in this test
// file. None is present in a production Go/WASM binary or exported API.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/frozeninput"
	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

const frozenManifestSHA = "6326a0f106a941e15cae34a86514badee3278bdc055a7e346e111210bfd03938"

type fixtureControl struct{ Mode, DSL, Config, Policy, Assumptions, ConfigDigest, PolicyDigest string }
type expectation struct {
	EntryCount                                                                      int
	EntryPrice                                                                      *string
	InitialStop, Target                                                             float64
	RiskBudgetUSD, QuantityOz, EntryFeeUSD, ExitFeeUSD, Terminal, KnownCashDeltaUSD string
	NetR, ExitPrice                                                                 map[string]string
}
type fixtureFile struct {
	Name, Direction, Variant, RawCSV, Snapshot, NormalizedSnapshot, Source, Ordering, Calendar, Schedule, Costs string
	Controls                                                                                                    []fixtureControl
	Context                                                                                                     contexts
	Lifecycle                                                                                                   []frozenlevels.LevelEvent
	ManagerEvents                                                                                               map[string][]frozenlevels.LockEvent
	Counters                                                                                                    map[string]frozenlevels.LockCounters
	Expected                                                                                                    expectation
	Identities                                                                                                  map[string]string
}

func file(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func fixtureNames(t *testing.T) []string {
	t.Helper()
	b := file(t, "FROZEN_MANIFEST.json")
	if hash(b) != frozenManifestSHA {
		t.Fatal("pre-coordinator freeze changed")
	}
	var m struct {
		Files map[string]struct{ SHA256 string }
	}
	if e := json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	names := []string{}
	for name, entry := range m.Files {
		if hash(file(t, name)) != entry.SHA256 {
			t.Fatal("frozen case changed", name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) != 14 {
		t.Fatal("missing frozen cases")
	}
	return names
}
func loadFixture(t *testing.T, name string) fixtureFile {
	t.Helper()
	found := false
	for _, known := range fixtureNames(t) {
		if name == known {
			found = true
		}
	}
	if !found {
		t.Fatal("unregistered test fixture")
	}
	var f fixtureFile
	if e := json.Unmarshal(file(t, name), &f); e != nil {
		t.Fatal(e)
	}
	return f
}
func (f fixtureFile) request() request {
	r := request{Documents: frozeninput.Documents{Snapshot: []byte(f.Snapshot), Source: []byte(f.Source), Ordering: []byte(f.Ordering)}, Artifacts: []frozeninput.Artifact{{Ref: dsl.FrozenQuoteRef{ID: "SyntheticRawQuotes", Version: "v1", SHA256: hash([]byte(f.RawCSV))}, Bytes: []byte(f.RawCSV)}}, Calendar: []byte(f.Calendar), Schedule: []byte(f.Schedule), Costs: []byte(f.Costs), RiskBudgetUSD: "1", Side: frozenlevels.Long}
	if f.Direction == "short" {
		r.Side = frozenlevels.Short
	}
	for i, c := range f.Controls {
		r.Sources[i] = c.DSL
		r.Assumptions[i] = []byte(c.Assumptions)
	}
	return r
}

type syntheticWitness struct {
	f        fixtureFile
	verified bool
	records  []frozenlevels.Quote
}

func witnessFor(t *testing.T, name string) *syntheticWitness {
	return &syntheticWitness{f: loadFixture(t, name)}
}

// correspondence is an actual decoder for this fixed invented three-column
// format. It rejects blank/dropped/extra/reordered records and preserves raw
// numeric tokens until the reviewed strict snapshot decoder validates them.
// The original bytes determine ordinals. No arbitrary provider decoder exists.
func correspondence(in *frozeninput.Input, raw []byte) ([]frozenlevels.Quote, error) {
	lines := strings.Split(string(raw), "\n")
	if len(lines) < 3 || lines[0] != "eventMS,bid,ask" || lines[len(lines)-1] != "" {
		return nil, fmt.Errorf("raw-correspondence: invalid header/final-LF")
	}
	snap := in.SnapshotDeclaration()
	rows := []any{}
	for i, line := range lines[1 : len(lines)-1] {
		columns := strings.Split(line, ",")
		if len(columns) != 3 || line == "" || strings.ContainsAny(line, "\r\" \t") {
			return nil, fmt.Errorf("raw-correspondence: malformed data record")
		}
		rows = append(rows, map[string]any{"eventMS": json.Number(columns[0]), "sequence": int64(i), "availableMS": json.Number(columns[0]), "bid": json.Number(columns[1]), "ask": json.Number(columns[2]), "sourceRecord": map[string]any{"artifactId": "SyntheticRawQuotes", "artifactVersion": "v1", "recordIndex": int64(i)}})
	}
	v := map[string]any{"schema": snap.Schema, "id": snap.ID, "version": snap.Version, "dataset": snap.Dataset, "symbol": snap.Symbol, "priceUnits": snap.PriceUnits, "sides": snap.Sides, "sequenceMethod": snap.SequenceMethod, "rows": rows}
	b, e := json.Marshal(v)
	if e != nil {
		return nil, fmt.Errorf("raw-correspondence: %w", e)
	}
	decoded, e := dsl.DecodeFrozenQuoteSnapshotJSON(b)
	if e != nil {
		return nil, fmt.Errorf("raw-correspondence: %w", e)
	}
	if !bytes.Equal(decoded.NormalizedBytes, snap.NormalizedBytes) {
		return nil, fmt.Errorf("raw-correspondence: original records do not match snapshot")
	}
	out := make([]frozenlevels.Quote, len(decoded.Rows))
	for i, r := range decoded.Rows {
		out[i] = frozenlevels.Quote{Event: frozenlevels.EventKey{AtMillis: r.EventMS, Sequence: r.Sequence}, AvailableAtMillis: r.AvailableMS, Bid: r.Bid, Ask: r.Ask}
	}
	return out, nil
}
func (w *syntheticWitness) verify(in *frozeninput.Input, r request) error {
	if w == nil {
		return fmt.Errorf("missing-test-only-synthetic-authority")
	}
	w.verified = false
	w.records = nil
	if len(r.Artifacts) != 1 {
		return fmt.Errorf("synthetic-witness-artifact-count")
	}
	records, e := correspondence(in, r.Artifacts[0].Bytes)
	if e != nil {
		return e
	}
	// The semantic correspondence check above runs even when an attacker has
	// coherently rebound all descendant hashes of a mismatching original leaf.
	f := w.f
	wantSide := frozenlevels.Long
	if f.Direction == "short" {
		wantSide = frozenlevels.Short
	}
	if r.Side != wantSide || string(r.Artifacts[0].Bytes) != f.RawCSV || string(r.Documents.Snapshot) != f.Snapshot || string(r.Documents.Source) != f.Source || string(r.Documents.Ordering) != f.Ordering || string(r.Calendar) != f.Calendar || string(r.Schedule) != f.Schedule {
		return fmt.Errorf("synthetic-witness-literal-path-mismatch")
	}
	if len(records) != 45 || records[0].Event.AtMillis != 86400000 || records[0].Bid != 3800 {
		return fmt.Errorf("synthetic-witness-opening-path")
	}
	for i := 0; i < 9; i++ {
		for j, offset := range []int64{0, 100000, 200000, 299999} {
			q := records[1+i*4+j]
			if q.Event.AtMillis != 172800000+int64(i)*300000+offset {
				return fmt.Errorf("synthetic-witness-bucket-path")
			}
		}
	}
	var cal frozenlevels.Calendar
	if e = json.Unmarshal(r.Calendar, &cal); e != nil {
		return e
	}
	if e = cal.Validate(); e != nil {
		return e
	}
	if len(cal.Days) != 2 || cal.Days[0].StartMillis != records[0].Event.AtMillis || cal.Days[0].EndMillis != 172800000 || cal.Days[1].StartMillis != 172800000 || cal.Days[1].EndMillis != 259200000 {
		return fmt.Errorf("synthetic-witness-calendar")
	}
	for i, source := range r.Sources {
		parsed, e := dsl.Parse(source)
		if e != nil || len(parsed.Errors) > 0 {
			return fmt.Errorf("synthetic-witness-config")
		}
		id, e := dsl.FrozenLevelIdentity(parsed.Config)
		if e != nil {
			return e
		}
		var a struct{ Schema, ID, Version, PolicyDigest, FixtureRiskBudgetUSD, Authority string }
		if e = json.Unmarshal(r.Assumptions[i], &a); e != nil {
			return e
		}
		ref := inputRef(parsed.Config, "assumptions")
		if a.Schema != "synthetic-control-assumptions-v1" || a.ID != ref.ID || a.Version != ref.Version || a.PolicyDigest != id.PolicyDigest || a.FixtureRiskBudgetUSD != r.RiskBudgetUSD || a.Authority != "test-only-fixture" {
			return fmt.Errorf("synthetic-witness-assumptions-policy")
		}
	}
	w.records = records
	w.verified = true
	return nil
}
func (w *syntheticWitness) calendar() (frozenlevels.Calendar, error) {
	if w == nil || !w.verified {
		return frozenlevels.Calendar{}, fmt.Errorf("unverified-test-witness")
	}
	var c frozenlevels.Calendar
	e := json.Unmarshal([]byte(w.f.Calendar), &c)
	return c, e
}
func (w *syntheticWitness) openings(source frozenlevels.Source) ([]frozenlevels.Opening, error) {
	c, e := w.calendar()
	if e != nil {
		return nil, e
	}
	if w.f.Variant == "missing-opening" {
		return nil, nil
	}
	q := w.records[0]
	return []frozenlevels.Opening{{Date: "1970-01-02", Event: q.Event, KnownAtMillis: q.Event.AtMillis, Price: q.Bid, Source: source, CalendarID: c.ID, CalendarVersion: c.Version, Qualification: "test-only-all-changes-first-record-at-day-start"}}, nil
}
func (w *syntheticWitness) contextKey(start, end int64) (frozenlevels.EventKey, error) {
	if w == nil || !w.verified || start < 172800000 || end > 175500000 || end-start != 300000 || start%300000 != 0 {
		return frozenlevels.EventKey{}, fmt.Errorf("unverified-synthetic-context-scope")
	}
	var last frozenlevels.EventKey
	count := 0
	for _, q := range w.records {
		if q.Event.AtMillis >= start && q.Event.AtMillis < end {
			last = q.Event
			count++
		}
	}
	if count != 4 {
		return frozenlevels.EventKey{}, fmt.Errorf("unverified-synthetic-context-records")
	}
	return frozenlevels.EventKey{AtMillis: end, Sequence: last.Sequence}, nil
}
func (w *syntheticWitness) transition(previous, current frozenlevels.EventKey) error {
	if w == nil || !w.verified {
		return fmt.Errorf("unverified-test-witness")
	}
	if !current.After(previous) || current.Sequence < 0 || current.Sequence >= int64(len(w.records)) || w.records[current.Sequence].Event != current {
		return fmt.Errorf("unverified-synthetic-source-transition")
	}
	// This is a prefix witness, not an all-or-nothing retrospective filter.
	// Unknown observations remain in the admitted input; no hold is assumed
	// across transition41->42 and no later quote can restore the live position.
	source, e := dsl.DecodeFrozenQuoteSourceJSON([]byte(w.f.Source))
	if e != nil {
		return e
	}
	for _, s := range source.CoverageSegments {
		if s.Status != "complete-claimed" && previous.AtMillis < s.EndMS && current.AtMillis >= s.StartMS {
			return fmt.Errorf("unknown-synthetic-path")
		}
	}
	return nil
}

func rationalEqual(t *testing.T, got, want string) {
	t.Helper()
	a, e := rational(got)
	if e != nil {
		t.Fatal(e)
	}
	b, e := rational(want)
	if e != nil {
		t.Fatal(e)
	}
	if a.Cmp(b) != 0 {
		t.Fatalf("%s != %s", got, want)
	}
}
func priceText(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }
