package frozeninput_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func rawDecoder(kind string, b []byte) error {
	switch kind {
	case "snapshot":
		_, e := dsl.DecodeFrozenQuoteSnapshotJSON(b)
		return e
	case "source":
		_, e := dsl.DecodeFrozenQuoteSourceJSON(b)
		return e
	default:
		_, e := dsl.DecodeFrozenQuoteOrderingJSON(b)
		return e
	}
}
func TestFrozenInputNestedRawShapes(t *testing.T) {
	count := 0
	for _, p := range []packet{fresh(t), rich(t), provider(t, "per-event-millisecond", []int64{7, 0, 1, 0})} {
		for _, doc := range []struct {
			kind string
			b    []byte
		}{{"snapshot", p.docs.Snapshot}, {"source", p.docs.Source}, {"ordering", p.docs.Ordering}} {
			root := value(t, doc.b)
			if e := rawDecoder(doc.kind, doc.b); e != nil {
				t.Fatal(e)
			}
			var walk func(any, string)
			walk = func(v any, path string) {
				switch node := v.(type) {
				case map[string]any:
					node["unknownField"] = true
					if e := rawDecoder(doc.kind, marshal(t, root)); e == nil {
						t.Fatalf("accepted extra %s.%s", doc.kind, path)
					}
					count++
					delete(node, "unknownField")
					for _, key := range sortedKeys(node) {
						old := node[key]
						delete(node, key)
						if e := rawDecoder(doc.kind, marshal(t, root)); e == nil {
							t.Fatalf("accepted missing %s.%s.%s", doc.kind, path, key)
						}
						count++
						node[key] = nil
						if e := rawDecoder(doc.kind, marshal(t, root)); e == nil {
							t.Fatalf("accepted null %s.%s.%s", doc.kind, path, key)
						}
						count++
						node[key] = true
						if e := rawDecoder(doc.kind, marshal(t, root)); e == nil {
							t.Fatalf("accepted boolean coercion %s.%s.%s", doc.kind, path, key)
						}
						count++
						node[key] = old
						walk(old, path+"."+key)
					}
				case []any:
					for i, child := range node {
						walk(child, fmt.Sprintf("%s[%d]", path, i))
					}
				}
			}
			walk(root, "")
		}
	}
	if count != 1743 {
		t.Fatalf("unexpectedly small raw mutation suite: %d", count)
	}
	fmt.Printf("FROZEN_INPUT_SHAPES=%d\n", count)
}

func TestFrozenInputEveryIntegerFieldIsLossless(t *testing.T) {
	p := rich(t)
	count := 0
	for _, doc := range []struct {
		kind string
		raw  []byte
	}{{"snapshot", p.docs.Snapshot}, {"source", p.docs.Source}, {"ordering", p.docs.Ordering}} {
		root := value(t, doc.raw)
		var walk func(any)
		walk = func(v any) {
			switch node := v.(type) {
			case map[string]any:
				for _, key := range sortedKeys(node) {
					old := node[key]
					if _, ok := old.(json.Number); ok && key != "bid" && key != "ask" {
						for _, token := range []string{"-0.0e0", "0.00000000000000001", "9007199254740991.00000000000000001", "9007199254740992"} {
							node[key] = json.Number(token)
							if e := rawDecoder(doc.kind, marshal(t, root)); e == nil {
								t.Fatalf("accepted lossy integer %s.%s=%s", doc.kind, key, token)
							}
							count++
						}
						node[key] = old
					}
					walk(old)
				}
			case []any:
				for _, child := range node {
					walk(child)
				}
			}
		}
		walk(root)
	}
	if count < 200 {
		t.Fatalf("missing integer coverage: %d", count)
	}
}

func TestFrozenInputDuplicateAndTransport(t *testing.T) {
	p := rich(t)
	for _, doc := range []struct {
		kind string
		b    []byte
	}{{"snapshot", p.docs.Snapshot}, {"source", p.docs.Source}, {"ordering", p.docs.Ordering}} {
		root := value(t, doc.b)
		var probe func(any)
		probe = func(v any) {
			switch node := v.(type) {
			case map[string]any:
				for _, key := range sortedKeys(node) {
					old := node[key]
					marker := "UNIQUE_DUPLICATE_MARKER"
					node[key] = marker
					raw := marshal(t, root)
					needle := marshal(t, marker)
					keyBytes := marshal(t, key)
					replacement := append(marshal(t, old), ',')
					replacement = append(replacement, keyBytes...)
					replacement = append(replacement, ':')
					replacement = append(replacement, marshal(t, old)...)
					bad := bytes.Replace(raw, needle, replacement, 1)
					if e := rawDecoder(doc.kind, bad); e == nil || !strings.Contains(e.Error(), "duplicate") {
						t.Fatalf("duplicate %s %s: %v", doc.kind, key, e)
					}
					node[key] = old
					probe(old)
				}
			case []any:
				for _, child := range node {
					probe(child)
				}
			}
		}
		probe(root)
		for _, bad := range [][]byte{append([]byte{0xef, 0xbb, 0xbf}, doc.b...), append(append([]byte(nil), doc.b...), []byte("{}")...), append([]byte{0xff}, doc.b...), bytes.Replace(doc.b, []byte(`"id":`), []byte(`"id":"\ud800","other":`), 1), bytes.Replace(doc.b, []byte(`"id":`), []byte(`"id":"\udc00","other":`), 1)} {
			if e := rawDecoder(doc.kind, bad); e == nil {
				t.Fatalf("accepted malformed transport %s", doc.kind)
			}
		}
	}
	// Decoded aliases collide even when the literal JSON key bytes differ.
	bad := bytes.Replace(p.docs.Snapshot, []byte(`"id":`), []byte(`"\u0069d":"Duplicate","id":`), 1)
	if e := rawDecoder("snapshot", bad); e == nil || !strings.Contains(e.Error(), "duplicate") {
		t.Fatalf("escaped duplicate: %v", e)
	}
}

func TestFrozenInputRawNumberAndPriceBoundary(t *testing.T) {
	p := fresh(t)
	original := p.docs.Snapshot
	for _, number := range []string{"-0", "-0.0", "-0e3", "9007199254740992", "9007199254740991.00000000000000001", "999.00000000000000001", "1e10001", "1e-10001", strings.Repeat("9", 1025), "null", `"999"`, "NaN", "Infinity"} {
		bad := bytes.Replace(original, []byte(`"eventMS":999`), []byte(`"eventMS":`+number), 1)
		if _, e := dsl.DecodeFrozenQuoteSnapshotJSON(bad); e == nil {
			t.Fatalf("accepted exact integer token %s", number)
		}
	}
	for _, number := range []string{"999", "999.0", "9.99e2", "0.999e3"} {
		raw := bytes.Replace(original, []byte(`"eventMS":999`), []byte(`"eventMS":`+number), 1)
		s, e := dsl.DecodeFrozenQuoteSnapshotJSON(raw)
		if e != nil || !bytes.Equal(s.NormalizedBytes, read(t, "quote-snapshot.normalized.json")) {
			t.Fatalf("integer alias %s: %v", number, e)
		}
	}
	for _, number := range []string{"0", "-0", "-1", "1e309", "1e-10000", "null", `"99.5"`, "NaN", "Infinity"} {
		bad := bytes.Replace(original, []byte(`"bid":99.50`), []byte(`"bid":`+number), 1)
		if _, e := dsl.DecodeFrozenQuoteSnapshotJSON(bad); e == nil {
			t.Fatalf("accepted price %s", number)
		}
	}
	// The raw crossing is smaller than one float64 ULP. Preserve it all the way
	// to the public constructor with every hash otherwise consistent.
	p = fresh(t)
	p.docs.Snapshot = bytes.Replace(p.docs.Snapshot, []byte(`"bid":99.50`), []byte(`"bid":100.00000000000000001`), 1)
	_, s, o := maps(t, p)
	hypothetical := object(t, read(t, "quote-snapshot.normalized.json"))
	hypothetical["rows"].([]any)[0].(map[string]any)["bid"] = 100
	qr := ref("FixtureSnapshot", "v1", p.docs.Snapshot)
	s["snapshotRef"] = qr
	s["normalizedSnapshotSHA256"] = sha(marshal(t, hypothetical))
	o["snapshotRef"] = qr
	p.bindSource(t, s, o)
	reject(t, p, "raw bid exceeds ask")
	// Non-crossed sub-precision spread may normalize equal; retained raw bytes
	// and false qualification keep that representation from becoming a claim.
	p = fresh(t)
	p.docs.Snapshot = bytes.Replace(p.docs.Snapshot, []byte(`"bid":99.50`), []byte(`"bid":100`), 1)
	p.docs.Snapshot = bytes.Replace(p.docs.Snapshot, []byte(`"ask":100.00`), []byte(`"ask":100.00000000000000001`), 1)
	_, s, o = maps(t, p)
	decoded, e := dsl.DecodeFrozenQuoteSnapshotJSON(p.docs.Snapshot)
	if e != nil {
		t.Fatal(e)
	}
	qr = ref("FixtureSnapshot", "v1", p.docs.Snapshot)
	s["snapshotRef"] = qr
	s["normalizedSnapshotSHA256"] = sha(decoded.NormalizedBytes)
	o["snapshotRef"] = qr
	p.bindSource(t, s, o)
	in := admit(t, p)
	row, _ := in.Observation(0)
	if row.Quote.Bid != row.Quote.Ask || in.Evidence().RunReady || !bytes.Contains(in.Documents().Snapshot, []byte("100.00000000000000001")) {
		t.Fatal("raw spread representation changed")
	}
}

func TestFrozenInputRawLimits(t *testing.T) {
	for _, kind := range []string{"snapshot", "source", "ordering"} {
		for _, raw := range [][]byte{bytes.Repeat([]byte(" "), (1<<20)+1), []byte(strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65)), []byte(`{"number":1e10001}`)} {
			if e := rawDecoder(kind, raw); e == nil {
				t.Fatalf("accepted resource-limit input %s", kind)
			}
		}
	}
	// Literal well-formed surrogate pairs/UTF-8 reach field grammar and are
	// refused as registry identities, not silently repaired by transport.
	for _, token := range []string{`"\ud83d\ude80"`, `"�"`} {
		p := fresh(t)
		p.docs.Snapshot = bytes.Replace(p.docs.Snapshot, []byte(`"FixtureSnapshot"`), []byte(token), 1)
		e := rawDecoder("snapshot", p.docs.Snapshot)
		if e == nil || !strings.Contains(e.Error(), "ASCII registry") {
			t.Fatalf("valid Unicode transport: %v", e)
		}
	}
}
