package frozencontrol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/frozeninput"
	"github.com/spik3r/heisentick-strat/engine/frozenlevels"
)

func TestFrozenSyntheticTraceFixtures(t *testing.T) {
	for _, name := range fixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			w := witnessFor(t, name)
			f := w.f
			r := f.request()
			for i, source := range r.Sources {
				parsed, e := dsl.Parse(source)
				if e != nil || len(parsed.Errors) > 0 {
					t.Fatal("actual compiler", e, parsed.Errors)
				}
				id, e := dsl.FrozenLevelIdentity(parsed.Config)
				if e != nil {
					t.Fatal(e)
				}
				expected := f.Controls[i]
				if string(id.ConfigBytes) != expected.Config || string(id.PolicyBytes) != expected.Policy || id.ConfigDigest != expected.ConfigDigest || id.PolicyDigest != expected.PolicyDigest {
					t.Fatal("pre-coordinator config identities changed")
				}
			}
			got, e := replay(r, w)
			if e != nil {
				t.Fatal(e)
			}
			if !w.verified {
				t.Fatal("witness was not verified")
			}
			if got.InputEvidence.RunReady || got.InputEvidence.ContinuityVerified || got.InputEvidence.SourceOrderVerified || got.InputEvidence.CompletedContext != "unqualified" {
				t.Fatal("admitted evidence upgraded")
			}
			if got.Source.SHA256 != f.Identities["source"] || got.Source.Side != frozenlevels.Bid || got.CostsSHA256 != f.Identities["costs"] || got.OrderingProof != "frozen-ordering:"+f.Identities["ordering"] {
				t.Fatal("wrong source/cost/order identity")
			}
			if !reflect.DeepEqual(got.Context, f.Context) {
				t.Fatalf("context differs from pre-coordinator primitive freeze\ngot %s\nwant %s", encoded(got.Context), encoded(f.Context))
			}
			if !reflect.DeepEqual(got.Lifecycle, f.Lifecycle) {
				t.Fatal("lifecycle changed from pre-coordinator freeze")
			}
			if got.Status != f.Expected.Terminal || len(got.Managers) != 3 {
				t.Fatalf("terminal %s want %s", got.Status, f.Expected.Terminal)
			}
			for _, m := range got.Managers {
				if m.Counters != f.Counters[m.Mode] || !reflect.DeepEqual(m.Events, f.ManagerEvents[m.Mode]) {
					t.Fatalf("%s primitive counters/events changed", m.Mode)
				}
				trade := m.Trade
				rationalEqual(t, trade.EntryFeeUSD, f.Expected.EntryFeeUSD)
				rationalEqual(t, trade.ExitFeeUSD, f.Expected.ExitFeeUSD)
				if f.Expected.EntryCount == 0 {
					if trade.Entry != nil || trade.Exit != nil || len(trade.Ledger) != 0 || trade.NetR != nil || trade.Status != "not-entered" {
						t.Fatal("rejected common opportunity filled or charged")
					}
					continue
				}
				if trade.Entry == nil || trade.Entry.At != (frozenlevels.EventKey{AtMillis: 175500002, Sequence: 39}) || trade.Entry.Price != 100 {
					t.Fatal("common fill changed")
				}
				rationalEqual(t, trade.QuantityOz, "0.1")
				entrySide, exitSide := "ask", "bid"
				if f.Direction == "short" {
					entrySide, exitSide = "bid", "ask"
				}
				if trade.Entry.Side != entrySide {
					t.Fatal("wrong executable entry side")
				}
				if f.Variant == "unknown-path" {
					if trade.Status != "unresolved" || trade.Exit != nil || trade.NetR != nil || trade.NetUSD != nil || trade.GrossUSD != nil || len(trade.Ledger) != 1 {
						t.Fatal("unknown path invented a close/return")
					}
					rationalEqual(t, trade.CashDeltaUSD, "-0.003")
					if m.Mode != "none" && m.Counters.Accepted != 1 {
						t.Fatal("unknown path rejected or filtered before its valid prefix")
					}
					if len(m.Events) == 0 || m.Events[len(m.Events)-1].Event.Sequence != 42 || m.Events[len(m.Events)-1].Kind != "unresolved" {
						t.Fatal("unknown path not terminal exactly on41->42")
					}
					continue
				}
				if trade.Exit == nil || trade.Exit.Side != exitSide || trade.NetUSD == nil || trade.NetR == nil || trade.GrossUSD == nil || trade.Status != "closed" {
					t.Fatal("missing side-aware close")
				}
				rationalEqual(t, priceText(trade.Exit.Price), f.Expected.ExitPrice[m.Mode])
				rationalEqual(t, *trade.NetR, f.Expected.NetR[m.Mode])
				rationalEqual(t, *trade.NetUSD, f.Expected.NetR[m.Mode])
				rationalEqual(t, trade.CashDeltaUSD, *trade.NetUSD)
				// Independently sum ledger records and compute economic conservation;
				// the entry fee appears in closed net exactly once, not only in cash.
				sum := new(big.Rat)
				for _, entry := range trade.Ledger {
					x, ok := new(big.Rat).SetString(entry.USD)
					if !ok {
						t.Fatal("non-exact ledger")
					}
					sum.Add(sum, x)
				}
				net, _ := new(big.Rat).SetString(*trade.NetUSD)
				if sum.Cmp(net) != 0 || len(trade.Ledger) != 3 {
					t.Fatal("cash/trade fee reconciliation failed")
				}
				gross, _ := new(big.Rat).SetString(*trade.GrossUSD)
				inFee, _ := new(big.Rat).SetString(trade.EntryFeeUSD)
				outFee, _ := new(big.Rat).SetString(trade.ExitFeeUSD)
				gross.Sub(gross, inFee).Sub(gross, outFee)
				if gross.Cmp(net) != 0 {
					t.Fatal("closed-trade fee loss or double charge")
				}
			}
			if f.Variant == "same-ms" && got.Reason != "context-not-strictly-before-fill" {
				t.Fatal("same-ms reason or selective waiting changed")
			}
			if f.Variant == "missing-opening" && got.Reason != "missing-prior-opening" {
				t.Fatal("missing opening was silently supplied")
			}
			fmt.Printf("SYNTHETIC_TRACE=%s\n", encoded(got))
		})
	}
}

func admitted(t *testing.T, r request) *frozeninput.Input {
	t.Helper()
	p, e := dsl.Parse(r.Sources[0])
	if e != nil || len(p.Errors) > 0 {
		t.Fatal(e, p.Errors)
	}
	in, e := frozeninput.Decode(p.Config, r.Documents, r.Artifacts)
	if e != nil {
		t.Fatal(e)
	}
	return in
}
func rawMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e := d.Decode(&m); e != nil {
		t.Fatal(e)
	}
	return m
}
func rebindRaw(t *testing.T, r request, raw []byte) request {
	t.Helper()
	oldSource, oldOrdering := hash(r.Documents.Source), hash(r.Documents.Ordering)
	oldRaw := r.Artifacts[0].Ref.SHA256
	var changeDigest func(any, string, string)
	changeDigest = func(v any, old, next string) {
		switch x := v.(type) {
		case map[string]any:
			for k, value := range x {
				if k == "sha256" && value == old {
					x[k] = next
				} else {
					changeDigest(value, old, next)
				}
			}
		case []any:
			for _, child := range x {
				changeDigest(child, old, next)
			}
		}
	}
	source := rawMap(t, r.Documents.Source)
	changeDigest(source, oldRaw, hash(raw))
	sraw := source["rawArtifact"].(map[string]any)
	sraw["byteCount"] = len(raw)
	sraw["ref"].(map[string]any)["sha256"] = hash(raw)
	r.Documents.Source = encoded(source)
	order := rawMap(t, r.Documents.Ordering)
	changeDigest(order, oldRaw, hash(raw))
	order["sourceRef"].(map[string]any)["sha256"] = hash(r.Documents.Source)
	order["sequence"].(map[string]any)["rawArtifactRef"].(map[string]any)["sha256"] = hash(raw)
	for _, c := range order["contextBindings"].([]any) {
		c.(map[string]any)["sourceRef"].(map[string]any)["sha256"] = hash(r.Documents.Source)
	}
	r.Documents.Ordering = encoded(order)
	r.Artifacts[0].Bytes = raw
	r.Artifacts[0].Ref.SHA256 = hash(raw)
	for i, s := range r.Sources {
		r.Sources[i] = strings.ReplaceAll(strings.ReplaceAll(s, oldSource, hash(r.Documents.Source)), oldOrdering, hash(r.Documents.Ordering))
	}
	return r
}
func TestFrozenSyntheticWitnessRawCorrespondence(t *testing.T) {
	f := loadFixture(t, "long-base.json")
	mutations := map[string]func([]byte) []byte{
		"price": func(b []byte) []byte {
			return bytes.Replace(b, []byte("86400000,3800,3800.02"), []byte("86400000,3799,3800.02"), 1)
		},
		"missing-record": func(b []byte) []byte {
			lines := strings.Split(string(b), "\n")
			return []byte(strings.Join(append(lines[:5], lines[6:]...), "\n"))
		},
		"reordered": func(b []byte) []byte {
			lines := strings.Split(string(b), "\n")
			lines[4], lines[5] = lines[5], lines[4]
			return []byte(strings.Join(lines, "\n"))
		},
		"float-hidden-cross": func(b []byte) []byte {
			return bytes.Replace(b, []byte("99.4,99.42"), []byte("99.42000000000000001,99.42"), 1)
		},
		"blank-record": func(b []byte) []byte { return append(b, '\n') },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			r := rebindRaw(t, f.request(), mutate([]byte(f.RawCSV)))
			in := admitted(t, r)
			if in.Evidence().RunReady {
				t.Fatal("input itself qualified data")
			}
			got, e := replay(r, witnessFor(t, "long-base.json"))
			if e == nil || got != nil || !strings.Contains(e.Error(), "raw-correspondence") {
				t.Fatalf("coherent hashes bypassed original-record proof: %v", e)
			}
		})
	}
	// Numeric aliases can describe the same normalized records. This check is
	// consistency only: it does not mint a witness for new arbitrary bytes.
	r := f.request()
	in := admitted(t, r)
	alias := bytes.Replace([]byte(f.RawCSV), []byte("86400000,3800,3800.02"), []byte("8.64e7,3800.0,3800.020"), 1)
	if _, e := correspondence(in, alias); e != nil {
		t.Fatal(e)
	}
}

func TestFrozenSyntheticAuthorityAndCommonAdmission(t *testing.T) {
	f := loadFixture(t, "long-base.json")
	r := f.request()
	for _, w := range []fixtureAuthority{nil, (*syntheticWitness)(nil)} {
		got, e := replay(r, w)
		if e == nil || got != nil {
			t.Fatal("missing witness admitted")
		}
	}
	for _, name := range []string{"metadata-label", "huge-risk-exponent", "budget", "wrong-side", "manager-policy", "payload-hash", "missing-source"} {
		t.Run(name, func(t *testing.T) {
			r := f.request()
			switch name {
			case "metadata-label":
				r.RiskBudgetUSD = "qualified"
			case "huge-risk-exponent":
				r.RiskBudgetUSD = "1e1000000000"
			case "budget":
				r.Costs = make([]byte, frozeninput.MaxInputBytes)
			case "wrong-side":
				r.Side = 0
			case "manager-policy":
				r.Sources[1] = strings.Replace(r.Sources[1], "frozen activation 1 R", "frozen activation 2 R", 1)
			case "payload-hash":
				r.Calendar = append(r.Calendar, '\n')
			case "missing-source":
				r.Artifacts = nil
			}
			got, e := replay(r, witnessFor(t, "long-base.json"))
			if e == nil || got != nil {
				t.Fatalf("%s admitted", name)
			}
		})
	}
	// An unverified witness object is not a label-based authority. The private
	// construction path must verify it during replay before any context use.
	w := witnessFor(t, "long-base.json")
	if _, e := w.calendar(); e == nil {
		t.Fatal("unverified calendar")
	}
	if _, e := w.contextKey(172800000, 173100000); e == nil {
		t.Fatal("unverified context")
	}
	if e := w.transition(frozenlevels.EventKey{}, frozenlevels.EventKey{AtMillis: 1, Sequence: 1}); e == nil {
		t.Fatal("unverified transition")
	}
}

func TestFrozenSyntheticExactStopProjection(t *testing.T) {
	for _, c := range []struct {
		side                  frozenlevels.Direction
		low, high, grid, want float64
	}{{frozenlevels.Long, 90.019, 133.77, .01, 90}, {frozenlevels.Short, 66.23, 109.981, .01, 110}} {
		got, e := structuralStop(frozenlevels.Window{Low: c.low, High: c.high}, c.side, c.grid)
		if e != nil || got != c.want {
			t.Fatalf("outward structural grid: %v %v", got, e)
		}
	}
	if _, e := structuralStop(frozenlevels.Window{Low: 90.01, High: 110}, frozenlevels.Long, .3333333333333333); e == nil || !strings.Contains(e.Error(), "unrepresentable-exact") {
		t.Fatal("inexact projected stop silently changed grid", e)
	}
}

func TestFrozenSyntheticNoProductionWitnessOrRunner(t *testing.T) {
	entries, e := os.ReadDir(".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		f, e := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Name.IsExported() {
				t.Fatalf("unexpected public runner/constructor: %s", fn.Name.Name)
			}
			if fn.Recv != nil && (fn.Name.Name == "verify" || fn.Name.Name == "contextKey" || fn.Name.Name == "transition") {
				t.Fatalf("production witness authority: %s", fn.Name.Name)
			}
		}
	}
}
