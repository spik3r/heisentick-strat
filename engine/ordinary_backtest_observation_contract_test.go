package engine

// Synthetic reference-consumer probes only. Nothing in this file produces an
// engine observation, runs a strategy, grants capability, or changes runtime.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

var observationProposalKinds = []string{"signal-unavailable", "no-signal", "managing-position", "cooldown", "rejected-side", "rejected-atr", "rejected-stop", "rejected-entry-gates", "accepted-order", "filled-order", "pending-at-end"}

type observationProposalChange struct {
	Path   []string
	Value  any
	Delete bool
}
type observationProposalCase struct {
	Name, Expected, Wire string
	Changes              []observationProposalChange
	Context              map[string]any
}
type observationProposalCorpus struct {
	Purpose, Source, RequestJSON, DataJSON, ConfigJSON, ReportJSON string
	Base                                                           map[string]any
	Cases                                                          []observationProposalCase
}

func observationProposalRead(t *testing.T, path string, out any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		t.Fatal(err)
	}
}
func observationProposalHash(text string) string {
	d := sha256.Sum256([]byte(text))
	return hex.EncodeToString(d[:])
}
func observationProposalIdentity(text string) map[string]any {
	return map[string]any{"sha256": observationProposalHash(text), "utf8Bytes": json.Number(strconv.Itoa(len(text)))}
}
func observationProposalClone(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
func observationProposalSet(t *testing.T, root map[string]any, c observationProposalChange) {
	t.Helper()
	var current any = root
	for i, key := range c.Path {
		last := i == len(c.Path)-1
		switch x := current.(type) {
		case map[string]any:
			if last {
				if c.Delete {
					delete(x, key)
				} else {
					x[key] = c.Value
				}
				return
			}
			current = x[key]
		case []any:
			n, err := strconv.Atoi(key)
			if err != nil || n < 0 || n >= len(x) {
				t.Fatalf("bad fixture path %v", c.Path)
			}
			if last {
				if c.Delete {
					t.Fatal("fixture array deletion unsupported")
				}
				x[n] = c.Value
				return
			}
			current = x[n]
		default:
			t.Fatalf("bad fixture path %v", c.Path)
		}
	}
	t.Fatal("empty fixture change path")
}

func TestOrdinaryObservationContractProposal(t *testing.T) {
	var corpus observationProposalCorpus
	observationProposalRead(t, "testdata/ordinary-backtest-observation-contract.json", &corpus)
	if !strings.Contains(corpus.Purpose, "synthetic-contract-reference-only") || len(corpus.Cases) < 70 {
		t.Fatal("missing explicit synthetic corpus")
	}
	var schema map[string]any
	observationProposalRead(t, "../spec/schemas/ordinary-backtest-observation-v1.schema.json", &schema)
	if schema["$id"] != "urn:heisentick:strat:ordinary-backtest-observation:v1" {
		t.Fatal("wrong proposal schema")
	}
	observationProposalSchemaKeywords(t, schema)
	names := map[string]bool{}
	for _, p := range corpus.Cases {
		t.Run(p.Name, func(t *testing.T) {
			if p.Name == "" || names[p.Name] {
				t.Fatal("duplicate/empty fixture name")
			}
			names[p.Name] = true
			envelope := observationProposalClone(t, corpus.Base)
			for _, c := range p.Changes {
				observationProposalSet(t, envelope, c)
			}
			wire, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			switch p.Wire {
			case "":
			case "duplicate":
				wire = append([]byte(`{"status":"complete",`), wire[1:]...)
			case "escaped-duplicate":
				wire = append([]byte(`{"\u0073tatus":"complete",`), wire[1:]...)
			case "lone-surrogate":
				wire = bytes.Replace(wire, []byte(`"synthetic-test-only"`), []byte(`"\ud800"`), 1)
			case "invalid-utf8":
				wire = bytes.Replace(wire, []byte("synthetic-test-only"), []byte{0xff}, 1)
			case "trailing-document":
				wire = append(wire, []byte(" {}")...)
			case "over-bytes":
				wire = append(wire, bytes.Repeat([]byte(" "), 65537)...)
			case "over-depth":
				wire = []byte(strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17))
			case "over-nodes":
				wire = []byte("[" + strings.Repeat("0,", 8192) + "0]")
			case "integer-exponent":
				wire = bytes.Replace(wire, []byte(`"visited":10`), []byte(`"visited":100e-1`), 1)
				wire = bytes.Replace(wire, []byte(`"created":6`), []byte(`"created":6.000`), 1)
				wire = bytes.Replace(wire, []byte(`"barT":0`), []byte(`"barT":0e99`), 1)
			case "fractional-lexeme":
				wire = bytes.Replace(wire, []byte(`"visited":10`), []byte(`"visited":10.0000000000000000000000001`), 1)
			case "nonfinite":
				wire = bytes.Replace(wire, []byte(`"visited":10`), []byte(`"visited":1e999`), 1)
			default:
				t.Fatal("unknown fixture wire mutation")
			}
			got := observationProposalAccept(wire, schema, corpus, p.Context)
			if got != p.Expected {
				t.Fatalf("got %s, want %s", got, p.Expected)
			}
		})
	}
}

// This explicitly bounded schema-subset evaluator is test-only. Unknown keywords
// fail the test rather than silently weakening schema checks. No production API.
func observationProposalSchemaKeywords(t *testing.T, schema map[string]any) {
	t.Helper()
	allowed := map[string]bool{}
	for _, k := range []string{"$schema", "$id", "$comment", "$ref", "$defs", "type", "const", "enum", "minimum", "maximum", "minLength", "maxLength", "pattern", "additionalProperties", "required", "properties", "items", "maxItems", "anyOf"} {
		allowed[k] = true
	}
	var walk func(map[string]any)
	walk = func(s map[string]any) {
		for k, v := range s {
			if !allowed[k] {
				t.Fatalf("unimplemented schema keyword %s", k)
			}
			switch k {
			case "$defs", "properties":
				for _, child := range v.(map[string]any) {
					walk(child.(map[string]any))
				}
			case "items":
				walk(v.(map[string]any))
			case "anyOf":
				for _, child := range v.([]any) {
					walk(child.(map[string]any))
				}
			case "$ref":
				r, ok := v.(string)
				if !ok || !strings.HasPrefix(r, "#/$defs/") || schema["$defs"].(map[string]any)[strings.TrimPrefix(r, "#/$defs/")] == nil {
					t.Fatalf("unresolved/nonlocal ref %v", v)
				}
			}
		}
	}
	walk(schema)
}
func observationProposalNumeric(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, e := strconv.ParseFloat(string(n), 64)
	return f, e == nil
}
func observationProposalValidate(v any, s, root map[string]any) bool {
	if r, ok := s["$ref"].(string); ok {
		return observationProposalValidate(v, root["$defs"].(map[string]any)[strings.TrimPrefix(r, "#/$defs/")].(map[string]any), root)
	}
	if alts, ok := s["anyOf"].([]any); ok {
		for _, alt := range alts {
			if observationProposalValidate(v, alt.(map[string]any), root) {
				return true
			}
		}
		return false
	}
	if want, ok := s["const"]; ok && !reflect.DeepEqual(v, want) {
		return false
	}
	if values, ok := s["enum"].([]any); ok {
		found := false
		for _, x := range values {
			if reflect.DeepEqual(v, x) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if typ, ok := s["type"].(string); ok {
		switch typ {
		case "null":
			if v != nil {
				return false
			}
		case "object":
			if _, ok := v.(map[string]any); !ok {
				return false
			}
		case "array":
			if _, ok := v.([]any); !ok {
				return false
			}
		case "string":
			if _, ok := v.(string); !ok {
				return false
			}
		case "integer":
			n, ok := v.(json.Number)
			if !ok || !ordinaryExactSafeInteger(string(n)) {
				return false
			}
		default:
			return false
		}
	}
	if m, ok := v.(map[string]any); ok {
		props, _ := s["properties"].(map[string]any)
		if required, ok := s["required"].([]any); ok {
			for _, key := range required {
				if _, ok := m[key.(string)]; !ok {
					return false
				}
			}
		}
		for key, value := range m {
			child, known := props[key]
			if !known {
				if s["additionalProperties"] == false {
					return false
				}
				continue
			}
			if !observationProposalValidate(value, child.(map[string]any), root) {
				return false
			}
		}
	}
	if a, ok := v.([]any); ok {
		if n, ok := observationProposalNumeric(s["maxItems"]); ok && len(a) > int(n) {
			return false
		}
		if item, ok := s["items"].(map[string]any); ok {
			for _, x := range a {
				if !observationProposalValidate(x, item, root) {
					return false
				}
			}
		}
	}
	if text, ok := v.(string); ok {
		n := utf8.RuneCountInString(text)
		if min, ok := observationProposalNumeric(s["minLength"]); ok && n < int(min) {
			return false
		}
		if max, ok := observationProposalNumeric(s["maxLength"]); ok && n > int(max) {
			return false
		}
		if p, ok := s["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(text) {
			return false
		}
	}
	if n, ok := observationProposalNumeric(v); ok {
		if min, ok := observationProposalNumeric(s["minimum"]); ok && n < min {
			return false
		}
		if max, ok := observationProposalNumeric(s["maximum"]); ok && n > max {
			return false
		}
	}
	return true
}
func observationProposalInt(v any) int            { f, _ := observationProposalNumeric(v); return int(f) }
func observationProposalMap(v any) map[string]any { return v.(map[string]any) }
func observationProposalSum(m map[string]any, keys ...string) int {
	n := 0
	for _, k := range keys {
		n += observationProposalInt(m[k])
	}
	return n
}
func observationProposalContext(ctx map[string]any, key string, fallback any) any {
	if v, ok := ctx[key]; ok {
		return v
	}
	return fallback
}

func observationProposalAccept(wire []byte, schema map[string]any, c observationProposalCorpus, ctx map[string]any) string {
	// Synthetic host lifecycle guard, not a timing/worker implementation. An actual
	// consumer must repeat these checks immediately before publication after decode.
	if observationProposalContext(ctx, "active", true) != true || observationProposalContext(ctx, "currentInvocation", "synthetic.invocation.1") != "synthetic.invocation.1" {
		return "inactive"
	}
	now := observationProposalInt(observationProposalContext(ctx, "now", json.Number("0")))
	deadline := observationProposalInt(observationProposalContext(ctx, "deadline", json.Number("100")))
	if now >= deadline {
		return "deadline"
	}
	if err := ordinaryJSONScan(wire, 65536, 16, 8192); err != nil {
		return "wire"
	}
	dec := json.NewDecoder(bytes.NewReader(wire))
	dec.UseNumber()
	var envelope any
	if err := dec.Decode(&envelope); err != nil {
		return "wire"
	}
	if !observationProposalValidate(envelope, schema, schema) {
		return "schema"
	}
	e := observationProposalMap(envelope)
	binding := observationProposalCloneNoTest(c.Base["binding"].(map[string]any))
	for key, text := range map[string]string{"source": c.Source, "request": c.RequestJSON, "data": c.DataJSON} {
		suffix, _ := ctx[key+"Suffix"].(string)
		binding[key] = observationProposalIdentity(text + suffix)
	}
	for key, text := range map[string]string{"config": c.ConfigJSON, "report": c.ReportJSON} {
		suffix, _ := ctx[key+"Suffix"].(string)
		binding[key+"Sha256"] = observationProposalHash(text + suffix)
	}
	if !observationProposalEqual(e["binding"], binding) || e["family"] != observationProposalContext(ctx, "family", "priceMomentum") || !observationProposalEqual(e["exampleLimit"], observationProposalContext(ctx, "exampleLimit", json.Number("16"))) {
		return "binding"
	}
	var data struct{ Times []json.Number }
	if err := json.Unmarshal([]byte(c.DataJSON), &data); err != nil {
		return "binding"
	}
	if observationProposalInt(e["barCount"]) != len(data.Times) {
		return "binding"
	}
	requested := observationProposalContext(ctx, "requested", true) == true
	status := e["status"].(string)
	if status == "not-requested" && requested || status != "not-requested" && !requested {
		return "status"
	}
	if status != "complete" {
		validReason := status == "not-requested" && e["reason"] == "not-requested" || status == "unsupported" && e["reason"] == "coverage-unavailable" || status == "incomplete" && (e["reason"] == "observer-limit" || e["reason"] == "observer-failed")
		if !validReason || e["counters"] != nil || len(e["examples"].([]any)) != 0 || observationProposalInt(e["omittedExampleKinds"]) != 0 {
			return "status"
		}
		return "unavailable"
	}
	if e["family"] != "priceMomentum" || e["counters"] == nil {
		return "status"
	}
	counters := observationProposalMap(e["counters"])
	bars := observationProposalMap(counters["bars"])
	candidates := observationProposalMap(counters["candidates"])
	orders := observationProposalMap(counters["orders"])
	if e["reason"] != nil || observationProposalInt(bars["visited"]) != len(data.Times) ||
		observationProposalInt(bars["visited"]) != observationProposalSum(bars, "managingPosition", "cooldown", "signalUnavailable", "noSignal", "candidates") ||
		!observationProposalEqual(bars["candidates"], candidates["created"]) || observationProposalInt(candidates["created"]) != observationProposalSum(candidates, "rejectedSide", "rejectedATR", "rejectedStop", "rejectedEntryGates", "acceptedOrders") ||
		!observationProposalEqual(candidates["acceptedOrders"], orders["accepted"]) || observationProposalInt(orders["accepted"]) != observationProposalSum(orders, "filled", "pendingAtEnd") {
		return "conservation"
	}
	counts := []int{observationProposalInt(bars["signalUnavailable"]), observationProposalInt(bars["noSignal"]), observationProposalInt(bars["managingPosition"]), observationProposalInt(bars["cooldown"]), observationProposalInt(candidates["rejectedSide"]), observationProposalInt(candidates["rejectedATR"]), observationProposalInt(candidates["rejectedStop"]), observationProposalInt(candidates["rejectedEntryGates"]), observationProposalInt(candidates["acceptedOrders"]), observationProposalInt(orders["filled"]), observationProposalInt(orders["pendingAtEnd"])}
	positive := 0
	for _, n := range counts {
		if n > 0 {
			positive++
		}
	}
	examples := e["examples"].([]any)
	want := min(positive, observationProposalInt(e["exampleLimit"]))
	if len(examples) != want || observationProposalInt(e["omittedExampleKinds"]) != positive-want {
		return "examples"
	}
	seen := map[string]bool{}
	handlerBars := map[int]int{}
	orderDisposition := map[int]int{}
	earliestAccepted := -1
	candidateSides := map[int]string{}
	candidateAccepted := map[int]bool{}
	priorBar, priorKind := -1, -1
	for _, raw := range examples {
		x := observationProposalMap(raw)
		kind := x["kind"].(string)
		rank := -1
		for i, k := range observationProposalKinds {
			if k == kind {
				rank = i
				break
			}
		}
		bar := observationProposalInt(x["barIndex"])
		if seen[kind] || rank < 0 || counts[rank] == 0 || bar >= len(data.Times) || !observationProposalEqual(x["barT"], data.Times[bar]) || bar < priorBar || bar == priorBar && rank <= priorKind {
			return "examples"
		}
		seen[kind] = true
		if rank <= 8 {
			if _, exists := handlerBars[bar]; exists {
				return "examples"
			}
			handlerBars[bar] = rank
		}
		priorBar, priorKind = bar, rank
		if rank < 4 {
			if x["signalIndex"] != nil || x["candidateId"] != nil || x["side"] != nil || x["consumed"] != nil {
				return "examples"
			}
		} else {
			if x["signalIndex"] == nil || x["side"] == nil {
				return "examples"
			}
			signal := observationProposalInt(x["signalIndex"])
			if signal > bar || rank <= 8 && signal != bar || rank == 10 && (bar != len(data.Times)-1 || signal != bar) || x["candidateId"] != fmt.Sprintf("pm:%d:%s", signal, x["side"]) || x["consumed"] != (rank >= 8) {
				return "examples"
			}
		}
		if rank >= 4 {
			signal := observationProposalInt(x["signalIndex"])
			side := x["side"].(string)
			accepted := rank >= 8
			if prior, ok := candidateSides[signal]; ok && (prior != side || candidateAccepted[signal] != accepted) {
				return "examples"
			}
			candidateSides[signal] = side
			candidateAccepted[signal] = accepted
			if rank == 8 {
				earliestAccepted = signal
			}
			if rank >= 9 {
				if prior, exists := orderDisposition[signal]; exists && prior != rank {
					return "examples"
				}
				orderDisposition[signal] = rank
			}
		}
		if x["sourceRange"] != nil {
			r := observationProposalMap(x["sourceRange"])
			start, end := observationProposalInt(r["startByte"]), observationProposalInt(r["endByte"])
			if start > end || end > len(c.Source) {
				return "range"
			}
			for _, n := range []int{start, end} {
				if !utf8.ValidString(c.Source[:n]) || n > 0 && n < len(c.Source) && c.Source[n-1] == '\r' && c.Source[n] == '\n' {
					return "range"
				}
			}
		}
	}
	acceptedSamples := 0
	for signal, accepted := range candidateAccepted {
		if handler, exists := handlerBars[signal]; exists && handler < 4 {
			return "examples"
		}
		if accepted {
			acceptedSamples++
			if earliestAccepted >= 0 && signal < earliestAccepted {
				return "examples"
			}
		}
	}
	if len(candidateAccepted) > observationProposalInt(candidates["created"]) || acceptedSamples > observationProposalInt(orders["accepted"]) {
		return "examples"
	}
	return "ok"
}
func observationProposalCloneNoTest(v map[string]any) map[string]any {
	raw, _ := json.Marshal(v)
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	_ = dec.Decode(&out)
	return out
}

func TestOrdinaryObservationProposalLeavesV1Closed(t *testing.T) {
	// Existing strict request parser must reject the new transport shape, not
	// silently enable observation. This parses invented text; it never runs it.
	source := "invented source\n"
	for _, field := range []string{"observations", "observationProfile", "exampleLimit"} {
		request := strings.Replace(ordinarySyntheticRequest, `"schema":`, `"`+field+`":true,"schema":`, 1)
		capture, err := DecodeOrdinaryBacktestRequest("run", request, source)
		if capture != nil || err == nil {
			t.Fatalf("v1 accepted observer field %s", field)
		}
	}
	var corpus observationProposalCorpus
	observationProposalRead(t, "testdata/ordinary-backtest-observation-contract.json", &corpus)
	wire, _ := json.Marshal(corpus.Base)
	capture, err := DecodeOrdinaryBacktestRequest("run", string(wire), source)
	if capture != nil || err == nil {
		t.Fatal("v1 accepted observation envelope")
	}
}

func TestOrdinaryObservationProposalSyntheticExampleSelection(t *testing.T) {
	// This test-only reference selection is not an engine recorder. It establishes
	// deterministic contract behavior only for the supplied invented event list.
	events := []struct {
		kind string
		bar  int
	}{{"no-signal", 4}, {"rejected-atr", 3}, {"no-signal", 1}, {"rejected-atr", 5}, {"accepted-order", 3}, {"signal-unavailable", 0}}
	first := map[string]int{}
	for _, event := range events {
		bar, exists := first[event.kind]
		if !exists || event.bar < bar {
			first[event.kind] = event.bar
		}
	}
	type item struct {
		kind      string
		bar, rank int
	}
	items := []item{}
	for rank, kind := range observationProposalKinds {
		if bar, ok := first[kind]; ok {
			items = append(items, item{kind, bar, rank})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].bar != items[j].bar {
			return items[i].bar < items[j].bar
		}
		return items[i].rank < items[j].rank
	})
	got := []string{}
	for _, item := range items {
		got = append(got, item.kind)
	}
	want := []string{"signal-unavailable", "no-signal", "rejected-atr", "accepted-order"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selection %v", got)
	}
	for limit := 0; limit <= 16; limit++ {
		n := min(limit, len(items))
		if len(items[:n])+len(items[n:]) != 4 {
			t.Fatal("truncation conservation")
		}
	}
}

// Schema-qualified envelope numbers are exact safe integers, so equivalent
// decimal/exponent lexemes compare by value rather than lexical spelling.
func observationProposalEqual(a, b any) bool {
	if x, ok := observationProposalNumeric(a); ok {
		y, other := observationProposalNumeric(b)
		return other && x == y
	}
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			other, present := y[k]
			if !present || !observationProposalEqual(v, other) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i, v := range x {
			if !observationProposalEqual(v, y[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}
