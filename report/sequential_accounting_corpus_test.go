package report_test

// This test-only adapter consumes independently authored, read-only expectations.
// Public authority: https://github.com/spik3r/heisentick-strat/blob/b574bfc903e3c6c8303c58ef34a0587df6e54968/docs/sequential-metrics-contract.md
// It runs the existing engine once, then projects that captured accounting. It
// has no regeneration mode or alternative trading/accounting implementation.
// 29 binding cases (58 variants) qualify this corpus; one provisional case is
// diagnostic only. The eight injected-only descriptions are NOT executed here.
// Native execution and Node-hosted Go/WASM are separate gates; a pass on either
// makes no ARM64, browser, historical, consumer or release qualification claim.

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report"
)

type acctObject map[string]any
type acctViolation string

func (e acctViolation) Error() string { return string(e) }
func acctRequire(ok bool, format string, args ...any) {
	if !ok {
		panic(acctViolation(fmt.Sprintf(format, args...)))
	}
}
func acctAttempt(f func()) (err error) {
	defer func() {
		if p := recover(); p != nil {
			if e, ok := p.(acctViolation); ok {
				err = e
			} else {
				panic(p)
			}
		}
	}()
	f()
	return nil
}

// Token-level parsing rejects duplicate keys at every depth and trailing JSON.
// UseNumber preserves type and spelling: null, false, zero and "0" differ.
func acctDecode(data []byte) any {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var read func() any
	read = func() any {
		tok, err := d.Token()
		acctRequire(err == nil, "JSON token: %v", err)
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{':
				out := acctObject{}
				for d.More() {
					key, err := d.Token()
					acctRequire(err == nil, "JSON key: %v", err)
					k, ok := key.(string)
					acctRequire(ok, "nonstring object key")
					_, exists := out[k]
					acctRequire(!exists, "duplicate key %q", k)
					out[k] = read()
				}
				end, err := d.Token()
				acctRequire(err == nil && end == json.Delim('}'), "object terminator")
				return out
			case '[':
				out := []any{}
				for d.More() {
					out = append(out, read())
				}
				end, err := d.Token()
				acctRequire(err == nil && end == json.Delim(']'), "array terminator")
				return out
			default:
				acctRequire(false, "unexpected delimiter %v", delim)
			}
		}
		return tok
	}
	v := read()
	_, err := d.Token()
	acctRequire(err == io.EOF, "trailing JSON: %v", err)
	return v
}
func acctMap(v any) acctObject {
	m, ok := v.(acctObject)
	acctRequire(ok, "expected object, got %T", v)
	return m
}
func acctList(v any) []any {
	a, ok := v.([]any)
	acctRequire(ok, "expected array, got %T", v)
	return a
}
func acctText(v any) string {
	s, ok := v.(string)
	acctRequire(ok, "expected string, got %T", v)
	return s
}
func acctQ(v any) *big.Rat {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case json.Number:
		s = string(x)
	case int:
		s = strconv.Itoa(x)
	case int64:
		s = strconv.FormatInt(x, 10)
	case *big.Rat:
		return new(big.Rat).Set(x)
	default:
		acctRequire(false, "expected exact rational, got %T", v)
	}
	q, ok := new(big.Rat).SetString(s)
	acctRequire(ok && s != "", "invalid rational %q", s)
	return q
}
func acctInt(v any) int {
	n, ok := v.(json.Number)
	acctRequire(ok, "expected integer token, got %T", v)
	i, err := strconv.ParseInt(string(n), 10, 64)
	acctRequire(err == nil && int64(int(i)) == i, "invalid integer %v", v)
	return int(i)
}
func acctFloat(v any) float64 {
	f, _ := acctQ(v).Float64()
	acctRequire(!math.IsNaN(f) && !math.IsInf(f, 0), "nonfinite rational %v", v)
	return f
}
func acctDyadic(v any) float64 {
	q := acctQ(v)
	f, exact := q.Float64()
	acctRequire(exact && !math.IsInf(f, 0), "monetary control not exactly binary64: %v", v)
	return f
}

// Explicit, closed record shapes. '?' on a name permits absence; '?' on a type
// permits null. This does not rely on decoding missing fields as Go zero values.
func acctShape(v any, fields string) acctObject {
	m := acctMap(v)
	allowed := map[string]bool{}
	for _, spec := range strings.Fields(fields) {
		pair := strings.Split(spec, ":")
		acctRequire(len(pair) == 2, "bad shape %s", spec)
		key, kind := pair[0], pair[1]
		optional := strings.HasSuffix(key, "?")
		key = strings.TrimSuffix(key, "?")
		allowed[key] = true
		x, present := m[key]
		acctRequire(present || optional, "missing field %s", key)
		if !present {
			continue
		}
		nullable := strings.HasPrefix(kind, "?")
		kind = strings.TrimPrefix(kind, "?")
		if x == nil && nullable {
			continue
		}
		ok := false
		switch kind {
		case "s":
			_, ok = x.(string)
		case "q":
			_, ok = x.(string)
			if ok {
				acctQ(x)
			}
		case "i":
			acctInt(x)
			ok = true
		case "b":
			_, ok = x.(bool)
		case "o":
			_, ok = x.(acctObject)
		case "a":
			_, ok = x.([]any)
		default:
			acctRequire(false, "unknown shape type %q", kind)
		}
		acctRequire(ok, "field %s wants %s, got %T", key, kind, x)
	}
	for key := range m {
		acctRequire(allowed[key], "unexpected field %s", key)
	}
	return m
}
func acctStrings(v any) {
	for _, x := range acctList(v) {
		acctText(x)
	}
}

const acctRoot = "testdata/sequential_accounting_v1"
const acctCorpusHash = "f9751c4f85cf123f86e74ebe3712310be0588a6b6ec451c6d54e0a927adedb4b"
const acctCommit = "b574bfc903e3c6c8303c58ef34a0587df6e54968"
const acctTree = "df6da00e7bc79f8523040cdcf2597ec424a8eb19"

var acctCaseFiles = map[string]string{
	"legacy.json":      "leg.target_fees_open_marks leg.stop_fees_loser leg.stop_gap_open_fees leg.liq_gross_win_net_loss leg.liq_exact_flat leg.signal_on_last_bar leg.liq_zero_costs_flat leg.no_trades_default_equity leg.dd_split_four_trades leg.winner_then_flat leg.equity_below_zero_transient leg.equity_negative_end leg.default_start_equity_trade",
	"full.json":        "full.e1_target_fees full.e1_stop_fees full.e1_stop_gap_fees full.e1_time_exit_fees full.e1_gross_win_net_loss full.e1_exact_flat full.e2_time_exit_fees full.e2_target_fees full.e2_stop_equity_negative_end full.e1_decision_no_trade full.e1_no_next_bar",
	"refusals.json":    "ref.e1_open_at_end_entry_bar ref.e1_open_at_end_last_checked_bar ref.e2_open_at_end_last_checked_bar ref.nonpositive_start_equity ref.overflow_entry_fee",
	"provisional.json": "leg.same_bar_exit_reentry",
}

const acctInjected = "inj.realized_chain_break inj.nonfinite_equity_sum inj.drawdown_nonfinite inj.alignment_mismatch inj.negative_zero_fields inj.tiny_positive_net inj.invalid_effective_start inj.blocked_in_position"

func acctKeys(m acctObject) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func acctInventory(got []string, want string) {
	w := strings.Fields(want)
	sort.Strings(w)
	g := append([]string(nil), got...)
	sort.Strings(g)
	acctRequire(reflect.DeepEqual(g, w), "inventory got %v, want %v", g, w)
}
func acctRead(path string) []byte {
	b, err := os.ReadFile(path)
	acctRequire(err == nil, "read %s: %v", path, err)
	return b
}
func acctSHA(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func acctPacket() map[string][]byte {
	files := map[string][]byte{}
	err := filepath.WalkDir(acctRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		acctRequire(d.Type().IsRegular(), "non-regular corpus file %s", path)
		rel, err := filepath.Rel(acctRoot, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = acctRead(path)
		return nil
	})
	acctRequire(err == nil, "walk corpus: %v", err)
	return files
}
func acctLoad(files map[string][]byte) []acctObject {
	names := []string{}
	for name := range files {
		names = append(names, name)
	}
	acctInventory(names, "MANIFEST.json SOURCES.json IMPORT.json README.md DERIVATIONS.md coverage.json cases/full.json cases/legacy.json cases/refusals.json cases/provisional.json cases/injected.json")
	// Pin the manifest itself as well as its content-addressed members. Rewriting
	// both a data file and its manifest cannot quietly authorize new expectations.
	acctRequire(acctSHA(files["MANIFEST.json"]) == "e6afa8333fb69c6eb8720fbed6abda0f9599cff844bc37f6c2a80e8df3300b2c", "manifest bytes changed")
	for name, b := range files {
		if strings.HasSuffix(name, ".json") {
			acctDecode(b)
		}
	}
	m := acctShape(acctDecode(files["MANIFEST.json"]), "schema:s projectionVersion:s corpus_sha256:s hash_rule:s files:o cases:i binding:i provisional:i injected_items:i")
	acctRequire(m["schema"] == "seq-accounting-public-manifest.v1" && m["projectionVersion"] == "1.1-public.1" && m["corpus_sha256"] == acctCorpusHash, "manifest identity")
	for k, want := range map[string]int{"cases": 30, "binding": 29, "provisional": 1, "injected_items": 8} {
		acctRequire(acctInt(m[k]) == want, "manifest count %s", k)
	}
	hashes := acctMap(m["files"])
	acctInventory(acctKeys(hashes), "SOURCES.json IMPORT.json README.md DERIVATIONS.md coverage.json cases/full.json cases/legacy.json cases/refusals.json cases/provisional.json cases/injected.json")
	var lines strings.Builder
	for _, name := range acctKeys(hashes) {
		h := acctText(hashes[name])
		acctRequire(acctSHA(files[name]) == h, "packet digest %s", name)
		fmt.Fprintf(&lines, "%s  %s\n", h, name)
	}
	acctRequire(acctSHA([]byte(lines.String())) == acctCorpusHash, "corpus digest")
	acctSources(acctDecode(files["SOURCES.json"]))
	all := []acctObject{}
	seen := map[string]bool{}
	for _, file := range []string{"legacy.json", "full.json", "refusals.json", "provisional.json"} {
		f := acctShape(acctDecode(files["cases/"+file]), "schema:s convention_id:s file:s marks_t_note:s cases:a")
		acctRequire(f["schema"] == "seq-accounting-public-projection.v1" && f["convention_id"] == "seq-acct.v1" && f["file"] == file, "case file identity %s", file)
		ids := []string{}
		for _, value := range acctList(f["cases"]) {
			c := acctValidateCase(value)
			id := acctText(c["id"])
			acctRequire(!seen[id], "duplicate case %s", id)
			seen[id] = true
			ids = append(ids, id)
			all = append(all, c)
			want := "binding"
			if file == "provisional.json" {
				want = "provisional"
			}
			acctRequire(c["status"] == want, "case status %s", id)
		}
		acctInventory(ids, acctCaseFiles[file])
	}
	f := acctShape(acctDecode(files["cases/injected.json"]), "schema:s convention_id:s file:s note:s items:a")
	acctRequire(f["schema"] == "seq-accounting-public-projection.v1" && f["convention_id"] == "seq-acct.v1" && f["file"] == "injected.json", "injected identity")
	ids := []string{}
	for _, x := range acctList(f["items"]) {
		item := acctShape(x, "id:s title:s test:s expected:s")
		ids = append(ids, acctText(item["id"]))
	}
	acctInventory(ids, acctInjected)
	return all
}
func acctSources(v any) {
	s := acctShape(v, "schema:s repository:s commit:s tree:s files:o strategy_sources:a")
	acctRequire(s["schema"] == "seq-accounting-public-sources.v1" && s["repository"] == "spik3r/heisentick-strat" && s["commit"] == acctCommit && s["tree"] == acctTree, "source identity")
	files := acctMap(s["files"])
	acctInventory(acctKeys(files), "docs/sequential-metrics-contract.md scripts/checks/sequential-arithmetic-schedules.json spec/schemas/sequential-backtest-result-v1.schema.json docs/sequential-source-freeze.json spec/dsl-spec-families/legacySetup9.md spec/dsl-spec-families/sequentialFull.md spec/schemas/run-fixture-v1.schema.json")
	// These are provenance pins, not a demand that future runtime sources remain
	// byte-frozen. Only the admitted strategy sources must still match exactly.
	for path, x := range files {
		p := acctShape(x, "blob:s sha256:s url:s")
		acctRequire(len(acctText(p["blob"])) == 40 && len(acctText(p["sha256"])) == 64 && p["url"] == "https://github.com/spik3r/heisentick-strat/blob/"+acctCommit+"/"+path, "source pin %s", path)
	}
	ids := []string{}
	for _, x := range acctList(s["strategy_sources"]) {
		p := acctShape(x, "strategyId:s producerPath:s sha256:s bytes:i")
		id := acctText(p["strategyId"])
		ids = append(ids, id)
		path := acctStrategyPaths[id]
		acctRequire(path != "" && p["producerPath"] == path, "strategy source path %s", id)
		b := acctRead(filepath.Join("..", path))
		acctRequire(len(b) == acctInt(p["bytes"]) && acctSHA(b) == p["sha256"], "admitted source changed %s", id)
	}
	acctInventory(ids, "dslSequentialLegacySetup9 dslSequentialFullE1 dslSequentialFullE2")
}

var acctStrategyPaths = map[string]string{
	"dslSequentialLegacySetup9": "conformance/parse/setup-legacy-setup9.strat",
	"dslSequentialFullE1":       "conformance/run/family-sequential-full-e1-long.strat",
	"dslSequentialFullE2":       "conformance/run/family-sequential-full-e2-long.strat",
}

func acctValidateCase(v any) acctObject {
	c := acctShape(v, "id:s title:s group:s profile:o series:o costs:o risk:o inputs:o mirror:o variants:o status:s initial_status:s provisional_assertions:a defects_detected:a status_history:a assertion_history:a refusal_type?:s prefix?:o provisional_reason?:s")
	p := acctShape(c["profile"], "family:s id:s source_id:s policy?:s")
	legacy := p["source_id"] == "dslSequentialLegacySetup9"
	if legacy {
		acctRequire(p["family"] == "legacy setup 9" && p["id"] == "seq.legacy.setup9.v1" && p["policy"] == nil, "legacy profile")
	} else {
		acctRequire(p["family"] == "sequential full" && p["id"] == "seq.full.public_approx.v1" && (p["policy"] == "E1" || p["policy"] == "E2") && p["source_id"] == "dslSequentialFull"+acctText(p["policy"]), "full profile")
	}
	s := acctShape(c["series"], "symbol:s timeframe:s timeframe_ms:i start_open_ms:i volume:i")
	if legacy {
		acctRequire(s["symbol"] == "XAUUSD" && s["timeframe"] == "1h" && acctInt(s["timeframe_ms"]) == 3600000, "legacy series")
	} else {
		acctRequire(s["symbol"] == "SYNTH" && s["timeframe"] == "5m" && acctInt(s["timeframe_ms"]) == 300000, "full series")
	}
	cost := acctShape(c["costs"], "feePerUnit:q slippage:q fillOn:s startEquity_input:q startEquity_effective:q")
	fill := "nextOpen"
	if legacy {
		fill = "close"
	}
	acctRequire(cost["fillOn"] == fill, "fill mode")
	// Overflow refusal supplies a finite fee not exactly representable as binary64.
	for _, k := range []string{"slippage", "startEquity_input", "startEquity_effective"} {
		acctDyadic(cost[k])
	}
	acctFloat(cost["feePerUnit"])
	r := acctShape(c["risk"], "riskUsd:i maxNotionalUsd?:i pinned_by?:s")
	if legacy {
		acctRequire(acctInt(r["riskUsd"]) == 200 && r["maxNotionalUsd"] == nil, "legacy risk")
	} else {
		acctRequire(acctInt(r["riskUsd"]) == 100 && acctInt(r["maxNotionalUsd"]) == 100000, "full risk")
	}
	input := acctShape(c["inputs"], "decision:?o decision_side_long_variant?:s")
	if input["decision"] != nil {
		acctShape(input["decision"], "index:i anchor_from:i anchor_to:i")
	}
	mirror := acctShape(c["mirror"], "constant:i exact:b")
	acctRequire(mirror["exact"] == true, "mirror must remain exact")
	for _, k := range []string{"provisional_assertions", "defects_detected"} {
		acctStrings(c[k])
	}
	for _, x := range acctList(c["status_history"]) {
		acctShape(x, "from:s to:s source:s")
	}
	for _, x := range acctList(c["assertion_history"]) {
		acctShape(x, "path:s from:s to:s source:s")
	}
	if x, ok := c["prefix"]; ok {
		acctShape(x, "from_case:s through_index:i bars_sha256:s file:s")
	}
	variants := acctShape(c["variants"], "long:o short:o")
	for _, side := range []string{"long", "short"} {
		v := acctShape(variants[side], "bars:a expected:o binary64_probe?:o")
		bars := acctList(v["bars"])
		acctRequire(len(bars) > 0, "empty bars")
		for _, x := range bars {
			b := acctShape(x, "o:q h:q l:q c:q")
			for _, k := range []string{"o", "h", "l", "c"} {
				acctDyadic(b[k])
			}
		}
		acctValidateExpected(v["expected"], legacy, len(bars))
		if probe, ok := v["binary64_probe"]; ok {
			pr := acctShape(probe, "money_identical:b money_differences:a ratio_matches_correct_rounding_in_plain_order:o returnPct_alternate_order_matches:b ratio_rational_is_dyadic:o")
			acctRequire(pr["money_identical"] == true && len(acctList(pr["money_differences"])) == 0, "packet monetary diagnostic")
			for _, name := range []string{"ratio_matches_correct_rounding_in_plain_order", "ratio_rational_is_dyadic"} {
				acctShape(pr[name], "returnPct:b maxDDpct:b winRate?:b profitFactor?:b expectancy?:b avgWin?:b avgLoss?:b avgHoldBars?:b")
			}
		}
	}
	long, short := acctList(acctMap(variants["long"])["bars"]), acctList(acctMap(variants["short"])["bars"])
	acctRequire(len(long) == len(short), "mirror bar count")
	for i := range long {
		l, r := acctMap(long[i]), acctMap(short[i])
		for _, pair := range [][2]string{{"o", "o"}, {"h", "l"}, {"l", "h"}, {"c", "c"}} {
			want := new(big.Rat).Sub(acctQ(mirror["constant"]), acctQ(l[pair[0]]))
			acctRequire(want.Cmp(acctQ(r[pair[1]])) == 0, "mirror bar %d %s", i, pair[0])
		}
	}
	return c
}
func acctValidateExpected(v any, legacy bool, bars int) {
	e := acctMap(v)
	if e["outcome"] == "refused" {
		acctShape(e, "outcome:s refusal:o")
		r := acctShape(e["refusal"], "type:s code_suggestion?:s code_binding:b at_bar_index?:i index_binding?:b reason_binding?:b no_partial_result:b held_position?:o evidence?:o")
		acctRequire(r["no_partial_result"] == true && r["code_binding"] == false, "refusal contract flags")
		if x, ok := r["held_position"]; ok {
			acctShape(x, "side:s entry_bar:i entry_fill:q size:q")
		}
		if x, ok := r["evidence"]; ok {
			acctShape(x, "quantity:s exact:q size:q exceeds:s")
		}
		return
	}
	acctRequire(e["outcome"] == "ok", "unknown expected outcome")
	common := "outcome:s trades:a marks:a headline:o trade_accounting:a run_trade_pnl:a drawdown_points:o"
	if legacy {
		acctShape(e, common+" signals:a")
		for _, x := range acctList(e["signals"]) {
			acctShape(x, "index:i side:s atr14:q stop:q target:q fill:q size:q executed:b")
		}
	} else {
		acctShape(e, common+" decision:?o order:?o")
		if e["decision"] != nil {
			d := acctShape(e["decision"], "index:i side:s atr14:q anchor:o stop_buffer:q stop:q")
			acctShape(d["anchor"], "from_index:i to_index:i price:q")
		}
		if e["order"] != nil {
			o := acctMap(e["order"])
			if o["status"] == "filled" {
				acctShape(o, "status:s entry_bar:i entry_fill:q risk_distance:q target:q size:q")
			} else {
				acctShape(o, "status:s")
				acctRequire(o["status"] == "no_next_bar", "unknown order status")
			}
		}
	}
	trades := acctList(e["trades"])
	for i, x := range trades {
		r := acctShape(x, "index:i side:s entry_bar:i exit_bar:i exit_reason:s hold_bars:i entry_fill:q exit_level:q exit_fill:q size:q gross_pnl:q entry_fee:q exit_fee:q trade_pnl:q net_pnl:q")
		acctRequire(acctInt(r["index"]) == i && acctInt(r["entry_bar"]) >= 0 && acctInt(r["exit_bar"]) < bars && acctInt(r["hold_bars"]) == acctInt(r["exit_bar"])-acctInt(r["entry_bar"]), "trade index/hold")
		for _, key := range []string{"entry_fill", "exit_level", "exit_fill", "size", "gross_pnl", "entry_fee", "exit_fee", "trade_pnl", "net_pnl"} {
			acctDyadic(r[key])
		}
	}
	marks := acctList(e["marks"])
	acctRequire(len(marks) == bars, "expected mark count")
	for i, x := range marks {
		m := acctShape(x, "index:i t:i realized:q unrealized:q equity:q replaced_by_liquidation?:b")
		acctRequire(acctInt(m["index"]) == i, "mark index")
		for _, k := range []string{"realized", "unrealized", "equity"} {
			acctDyadic(m[k])
		}
	}
	h := acctShape(e["headline"], "startEquity:q endEquity:q net:q returnPct:q worstLoss:q maxDD:q maxDDpct:q trades:i maxWinStreak:i maxLossStreak:i winRate:?q winRateReason?:s profitFactor:?q profitFactorReason?:s expectancy:?q expectancyReason?:s avgWin:?q avgWinReason?:s avgLoss:?q avgLossReason?:s avgHoldBars:?q avgHoldBarsReason?:s")
	acctRequire(acctInt(h["trades"]) == len(trades), "expected headline trade count")
	for _, k := range []string{"winRate", "profitFactor", "expectancy", "avgWin", "avgLoss", "avgHoldBars"} {
		_, reason := h[k+"Reason"]
		acctRequire(reason == (h[k] == nil), "null/reason pair %s", k)
	}
	ta, pnl := acctList(e["trade_accounting"]), acctList(e["run_trade_pnl"])
	acctRequire(len(ta) == len(trades) && len(pnl) == len(trades), "expected ledger record count")
	for i := range trades {
		a := acctShape(ta[i], "index:i entryFee:q exitFee:q netPnl:q")
		p := acctShape(pnl[i], "index:i pnl:q")
		acctRequire(acctInt(a["index"]) == i && acctInt(p["index"]) == i, "ledger record index")
	}
	dd := acctShape(e["drawdown_points"], "maxDD:o maxDDpct:o note:s")
	acctShape(dd["maxDD"], "bar:?i peak:q peak_bar:i equity:q drawdown:q")
	acctShape(dd["maxDDpct"], "bar:?i peak:q peak_bar:i equity:q drawdown:q percent:q")
}

// Semantic numbers are rationals; acctOrdered values are the prescribed exact
// binary64 results of a rounded operation sequence. They are intentionally
// different comparison modes. Neither uses an epsilon or expected regeneration.
type acctOrdered float64

func acctNumbers(v any) any {
	switch x := v.(type) {
	case string:
		if q, ok := new(big.Rat).SetString(x); ok {
			return q
		}
		return x
	case json.Number:
		return acctQ(x)
	case acctObject:
		m := acctObject{}
		for k, y := range x {
			m[k] = acctNumbers(y)
		}
		return m
	case []any:
		a := make([]any, len(x))
		for i, y := range x {
			a[i] = acctNumbers(y)
		}
		return a
	default:
		return v
	}
}
func acctEqual(got, want any, path string) {
	switch w := want.(type) {
	case acctObject:
		g, ok := got.(acctObject)
		acctRequire(ok, "%s: object got %T", path, got)
		acctRequire(reflect.DeepEqual(acctKeys(g), acctKeys(w)), "%s keys got %v, want %v", path, acctKeys(g), acctKeys(w))
		for _, k := range acctKeys(w) {
			acctEqual(g[k], w[k], path+"."+k)
		}
	case []any:
		g, ok := got.([]any)
		acctRequire(ok && len(g) == len(w), "%s array/count got %T/%d, want %d", path, got, len(g), len(w))
		for i := range w {
			acctEqual(g[i], w[i], fmt.Sprintf("%s[%d]", path, i))
		}
	case *big.Rat:
		q := acctActualQ(got)
		acctRequire(q.Cmp(w) == 0, "%s got %v, want exact %s", path, got, w.RatString())
	case acctOrdered:
		f := acctActualFloat(got)
		acctRequire(math.Float64bits(f) == math.Float64bits(float64(w)), "%s got %.17g [%016x], want ordered %.17g [%016x]", path, f, math.Float64bits(f), float64(w), math.Float64bits(float64(w)))
	default:
		acctRequire(reflect.DeepEqual(got, w), "%s got %#v (%T), want %#v (%T)", path, got, got, w, w)
	}
}
func acctActualQ(v any) *big.Rat {
	switch x := v.(type) {
	case float64:
		acctRequire(!math.IsNaN(x) && !math.IsInf(x, 0), "nonfinite actual")
		return new(big.Rat).SetFloat64(x)
	case json.Number:
		f, err := x.Float64()
		acctRequire(err == nil && !math.IsInf(f, 0), "bad actual number %s", x)
		return new(big.Rat).SetFloat64(f)
	case int:
		return new(big.Rat).SetInt64(int64(x))
	case int64:
		return new(big.Rat).SetInt64(x)
	default:
		acctRequire(false, "actual numeric type %T", v)
		return nil
	}
}
func acctActualFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		acctRequire(!math.IsNaN(x) && !math.IsInf(x, 0), "nonfinite actual")
		return x
	case json.Number:
		f, err := x.Float64()
		acctRequire(err == nil && !math.IsInf(f, 0), "bad actual number %s", x)
		return f
	}
	q := acctActualQ(v)
	f, _ := q.Float64()
	return f
}

// Reflection visits every exported field, including fields omitted at zero in
// JSON. Only named, explicitly reviewed non-JSON fields may be excluded. Full
// object comparison below fails closed when a numeric or categorical path grows.
func acctReflect(v any) any { return acctReflectValue(reflect.ValueOf(v), "$") }
func acctReflectValue(v reflect.Value, path string) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return acctReflectValue(v.Elem(), path)
	}
	switch v.Kind() {
	case reflect.Struct:
		m := acctObject{}
		typ := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := typ.Field(i)
			acctRequire(f.IsExported(), "unhandled private reflection field %s.%s", path, f.Name)
			tag := strings.Split(f.Tag.Get("json"), ",")
			acctRequire(tag[0] != "" && tag[0] != "-", "unhandled reflection path %s.%s", path, f.Name)
			if len(tag) > 1 && tag[1] == "omitempty" && v.Field(i).IsZero() {
				continue
			}
			m[tag[0]] = acctReflectValue(v.Field(i), path+"."+tag[0])
		}
		return m
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		acctRequire(v.Type().Key().Kind() == reflect.String, "nonstring reflection map %s", path)
		m := acctObject{}
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			m[key] = acctReflectValue(iter.Value(), path+"."+key)
		}
		return m
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = acctReflectValue(v.Index(i), fmt.Sprintf("%s[%d]", path, i))
		}
		return out
	case reflect.Float64:
		return v.Float()
	case reflect.Int, reflect.Int64:
		return v.Int()
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	default:
		acctRequire(false, "unhandled reflection kind %s at %s", v.Kind(), path)
		return nil
	}
}
func acctJSON(v any) any {
	b, err := json.Marshal(v)
	acctRequire(err == nil, "serialize: %v", err)
	return acctDecode(b)
}
func acctFiniteNormalized(v any, path string) {
	switch x := v.(type) {
	case acctObject:
		for k, y := range x {
			acctFiniteNormalized(y, path+"."+k)
		}
	case []any:
		for i, y := range x {
			acctFiniteNormalized(y, fmt.Sprintf("%s[%d]", path, i))
		}
	case float64:
		acctRequire(!math.IsNaN(x) && !math.IsInf(x, 0) && !(x == 0 && math.Signbit(x)), "invalid exported float %s: %v", path, x)
	case json.Number:
		acctRequire(!strings.HasPrefix(string(x), "-0") || acctActualFloat(x) != 0, "negative zero JSON %s", path)
		acctActualFloat(x)
	}
}

func acctFixture(c acctObject, side string) engine.RunFixture {
	p, s, cost := acctMap(c["profile"]), acctMap(c["series"]), acctMap(c["costs"])
	f := engine.RunFixture{Schema: "dsl-conformance-run-fixture-v1", Case: acctText(c["id"]) + "." + side, StrategyID: acctText(p["source_id"]), Symbol: acctText(s["symbol"]), Timeframe: acctText(s["timeframe"]), RangeMethod: "zone", Costs: engine.Costs{FeePerUnit: acctFloat(cost["feePerUnit"]), Slippage: acctDyadic(cost["slippage"]), StartEquity: acctDyadic(cost["startEquity_input"]), FillOn: acctText(cost["fillOn"])}}
	v := acctMap(acctMap(c["variants"])[side])
	for i, x := range acctList(v["bars"]) {
		b := acctMap(x)
		t := float64(acctInt(s["start_open_ms"]) + i*acctInt(s["timeframe_ms"]))
		bar := marketdata.Bar{T: t, O: acctDyadic(b["o"]), H: acctDyadic(b["h"]), L: acctDyadic(b["l"]), C: acctDyadic(b["c"]), V: float64(acctInt(s["volume"]))}
		f.Bars = append(f.Bars, bar)
		f.RawBars = append(f.RawBars, []float64{bar.T, bar.O, bar.H, bar.L, bar.C, bar.V})
	}
	return f
}
func acctRound(q *big.Rat) float64 {
	f, _ := q.Float64()
	acctRequire(!math.IsInf(f, 0), "ordered result overflow")
	if f == 0 {
		return 0
	}
	return f
}
func acctPercent(n, d *big.Rat) acctOrdered {
	// The public schedule rounds division BEFORE multiplication by 100. The
	// rational oracle implements those two round-to-nearest/ties-even operations
	// independently of the host compiler's expression contraction choices.
	quotient := acctRound(new(big.Rat).Quo(n, d))
	return acctOrdered(acctRound(new(big.Rat).Mul(new(big.Rat).SetFloat64(quotient), big.NewRat(100, 1))))
}
func acctHeadline(e acctObject) acctObject {
	h := acctMap(e["headline"])
	out := acctMap(acctNumbers(h))
	trades := acctList(e["trades"])
	n := int64(len(trades))
	wins := int64(0)
	sumWin, sumLoss, hold := new(big.Rat), new(big.Rat), new(big.Rat)
	for _, x := range trades {
		t := acctMap(x)
		net := acctQ(t["net_pnl"])
		if net.Sign() > 0 {
			wins++
			sumWin.Add(sumWin, net)
		} else {
			sumLoss.Sub(sumLoss, net)
		}
		hold.Add(hold, acctQ(t["hold_bars"]))
	}
	ratios := map[string]*big.Rat{"returnPct": new(big.Rat).Mul(new(big.Rat).Quo(acctQ(h["net"]), acctQ(h["startEquity"])), big.NewRat(100, 1))}
	if n > 0 {
		ratios["winRate"] = big.NewRat(wins*100, n)
		ratios["expectancy"] = new(big.Rat).Quo(acctQ(h["net"]), big.NewRat(n, 1))
		ratios["avgHoldBars"] = new(big.Rat).Quo(hold, big.NewRat(n, 1))
		if wins > 0 {
			ratios["avgWin"] = new(big.Rat).Quo(sumWin, big.NewRat(wins, 1))
		}
		if n > wins {
			ratios["avgLoss"] = new(big.Rat).Quo(new(big.Rat).Neg(sumLoss), big.NewRat(n-wins, 1))
		}
		if sumLoss.Sign() > 0 {
			ratios["profitFactor"] = new(big.Rat).Quo(sumWin, sumLoss)
		} else if sumWin.Sign() == 0 {
			ratios["profitFactor"] = new(big.Rat)
		}
	}
	peak := acctQ(h["startEquity"])
	maxPct := new(big.Rat)
	orderedMax := acctOrdered(0)
	for _, x := range acctList(e["marks"]) {
		m := acctMap(x)
		equity := acctQ(m["equity"])
		if equity.Cmp(peak) > 0 {
			peak = equity
		}
		dd := new(big.Rat).Sub(peak, equity)
		pct := new(big.Rat).Mul(new(big.Rat).Quo(dd, peak), big.NewRat(100, 1))
		if pct.Cmp(maxPct) > 0 {
			maxPct = pct
		}
		ordered := acctPercent(dd, peak)
		if ordered > orderedMax {
			orderedMax = ordered
		}
	}
	ratios["maxDDpct"] = maxPct
	for name, q := range ratios {
		acctRequire(h[name] != nil && acctQ(h[name]).Cmp(q) == 0, "authored rational meaning differs: %s=%v, derivation=%s", name, h[name], q.RatString())
		out[name] = acctOrdered(acctRound(q))
	}
	out["returnPct"] = acctPercent(acctQ(h["net"]), acctQ(h["startEquity"]))
	out["maxDDpct"] = orderedMax
	if n > 0 {
		out["winRate"] = acctPercent(big.NewRat(wins, 1), big.NewRat(n, 1))
	}
	return out
}
func acctDrawdown(e acctObject, a engine.SequentialAccounting) {
	// Drawdown-point descriptions are derivations, not additional runtime fields.
	// Reconcile them against the actual marks, keeping currency and percent
	// maxima independent and preserving the null bar when the maximum is zero.
	peak, peakBar := acctQ(acctMap(e["headline"])["startEquity"]), -1
	ddmax, pctmax := new(big.Rat), new(big.Rat)
	base := acctObject{"bar": nil, "peak": new(big.Rat).Set(peak), "peak_bar": big.NewRat(-1, 1), "equity": new(big.Rat).Set(peak), "drawdown": new(big.Rat)}
	dollar, percent := acctMap(acctNumbers(base)), acctMap(acctNumbers(base))
	percent["percent"] = new(big.Rat)
	for i, m := range a.Marks {
		eq := new(big.Rat).SetFloat64(m.Equity)
		if eq.Cmp(peak) > 0 {
			peak, peakBar = eq, i
		}
		dd := new(big.Rat).Sub(peak, eq)
		pct := new(big.Rat).Mul(new(big.Rat).Quo(dd, peak), big.NewRat(100, 1))
		point := func() acctObject {
			return acctObject{"bar": big.NewRat(int64(i), 1), "peak": new(big.Rat).Set(peak), "peak_bar": big.NewRat(int64(peakBar), 1), "equity": eq, "drawdown": dd}
		}
		if dd.Cmp(ddmax) > 0 {
			ddmax = dd
			dollar = point()
		}
		if pct.Cmp(pctmax) > 0 {
			pctmax = pct
			percent = point()
			percent["percent"] = pct
		}
	}
	// Here both sides are rational derivations; no binary64 rounding is applied.
	expected := acctMap(e["drawdown_points"])
	acctRationalEqual(dollar, acctNumbers(expected["maxDD"]), "drawdown_points.maxDD")
	acctRationalEqual(percent, acctNumbers(expected["maxDDpct"]), "drawdown_points.maxDDpct")
}
func acctRationalEqual(a, b any, path string) {
	switch x := b.(type) {
	case *big.Rat:
		y, ok := a.(*big.Rat)
		acctRequire(ok && y.Cmp(x) == 0, "%s rational got %v, want %v", path, a, b)
	case acctObject:
		y := acctMap(a)
		acctRequire(reflect.DeepEqual(acctKeys(y), acctKeys(x)), "%s rational keys", path)
		for k, v := range x {
			acctRationalEqual(y[k], v, path+"."+k)
		}
	default:
		acctRequire(reflect.DeepEqual(a, b), "%s got %v, want %v", path, a, b)
	}
}
func acctLegacyNumbers(v any) any {
	switch x := v.(type) {
	case *big.Rat:
		f := acctRound(x)
		text := strconv.FormatFloat(f, 'g', 15, 64)
		rounded, err := strconv.ParseFloat(text, 64)
		acctRequire(err == nil, "legacy number %s", text)
		if rounded == 0 {
			rounded = 0
		}
		return acctOrdered(rounded)
	case acctObject:
		m := acctObject{}
		for k, y := range x {
			m[k] = acctLegacyNumbers(y)
		}
		return m
	case []any:
		a := make([]any, len(x))
		for i, y := range x {
			a[i] = acctLegacyNumbers(y)
		}
		return a
	default:
		return v
	}
}
func acctExpectedRun(c, e acctObject, side string, f engine.RunFixture) acctObject {
	p := acctMap(c["profile"])
	cost := acctMap(c["costs"])
	legacy := p["policy"] == nil
	run := acctObject{"schema": "dsl-conformance-trades-v1", "case": f.Case, "strategyId": f.StrategyID, "symbol": f.Symbol, "timeframe": f.Timeframe, "higherTimeframe": nil, "rangeMethod": "zone", "costs": acctNumbers(acctObject{"feePerUnit": cost["feePerUnit"], "slippage": cost["slippage"], "fillOn": cost["fillOn"], "startEquity": cost["startEquity_effective"]}), "tradeCount": acctQ(acctMap(e["headline"])["trades"]), "trades": []any{}}
	var opportunity, decision, order acctObject
	if !legacy {
		opportunities := []any{}
		if e["decision"] != nil {
			decision, order = acctMap(e["decision"]), acctMap(e["order"])
			di := acctInt(decision["index"])
			input := acctMap(acctMap(c["inputs"])["decision"])
			from := acctInt(input["anchor_from"])
			setup := from + 8
			direction, trigger := "buy", "perf"
			if side == "short" {
				direction = "sell"
			}
			if p["policy"] == "E2" {
				trigger = "cd13"
			}
			episode := fmt.Sprintf("%s:%s:%s:%s:%.0f", p["id"], f.Symbol, f.Timeframe, direction, f.Bars[setup].T)
			opportunity = acctObject{"id": episode + ":" + trigger, "episodeId": episode, "trigger": trigger, "side": side, "setupIndex": big.NewRat(int64(setup), 1), "setupFirstIndex": acctQ(input["anchor_from"]), "decisionIndex": acctQ(decision["index"]), "decisionOpenMs": new(big.Rat).SetFloat64(f.Bars[di].T), "decisionMs": new(big.Rat).SetFloat64(f.Bars[di].T + float64(acctInt(acctMap(c["series"])["timeframe_ms"]))), "nextOpenIndex": big.NewRat(int64(di+1), 1), "stop": acctQ(decision["stop"]), "signalAtr": acctQ(decision["atr14"]), "capBinds": false}
			if order["status"] == "filled" {
				opportunity["status"] = "filled"
				for k, key := range map[string]string{"fillIndex": "entry_bar", "fill": "entry_fill", "target": "target", "riskDistance": "risk_distance", "size": "size"} {
					opportunity[k] = acctQ(order[key])
				}
			} else {
				opportunity["status"] = "rejected"
				opportunity["reason"] = "no_next_bar"
			}
			opportunities = append(opportunities, opportunity)
		}
		run["sequentialFull"] = acctObject{"schema": "sequential-full-execution.v1", "profile": p["id"], "policy": p["policy"], "symbol": f.Symbol, "timeframe": f.Timeframe, "opportunities": opportunities}
	}
	trades := acctList(e["trades"])
	signals := []any{}
	if legacy {
		signals = acctList(e["signals"])
		acctRequire(len(signals) == len(trades), "legacy executed signal count")
	}
	for i, x := range trades {
		tr := acctMap(x)
		ei, xi := acctInt(tr["entry_bar"]), acctInt(tr["exit_bar"])
		meta := acctObject{"profile": p["id"]}
		stop, target := any(nil), any(nil)
		tag := ""
		if legacy {
			s := acctMap(signals[i])
			acctRequire(s["executed"] == true && s["side"] == tr["side"] && acctInt(s["index"]) == ei && acctQ(s["fill"]).Cmp(acctQ(tr["entry_fill"])) == 0 && acctQ(s["size"]).Cmp(acctQ(tr["size"])) == 0, "legacy signal/trade identity")
			meta["setup"] = "legacySetup9"
			meta["signalAtr"], meta["signalIndex"] = acctQ(s["atr14"]), acctQ(s["index"])
			stop, target = acctQ(s["stop"]), acctQ(s["target"])
			tag = "TD-BUY-9"
			if tr["side"] == "short" {
				tag = "TD-SELL-9"
			}
		} else {
			meta["policy"], meta["episodeId"], meta["opportunityId"] = p["policy"], opportunity["episodeId"], opportunity["id"]
			meta["signalAtr"], meta["signalIndex"] = acctQ(decision["atr14"]), acctQ(decision["index"])
			stop, target = acctQ(decision["stop"]), acctQ(order["target"])
			tag = "SEQUENTIAL-" + acctText(p["policy"])
		}
		reason := acctText(tr["exit_reason"])
		if mapped, ok := map[string]string{"target": "tp", "stop": "sl", "sl-gap": "sl", "time_exit": "time"}[reason]; ok {
			reason = mapped
		}
		t := acctObject{"entry": acctQ(tr["entry_fill"]), "entryIndex": acctQ(tr["entry_bar"]), "entryT": new(big.Rat).SetFloat64(f.Bars[ei].T), "exit": acctQ(tr["exit_fill"]), "exitIndex": acctQ(tr["exit_bar"]), "exitT": new(big.Rat).SetFloat64(f.Bars[xi].T), "initialSl": stop, "initialTp": target, "sl": stop, "tp": target, "size": acctQ(tr["size"]), "points": new(big.Rat).Quo(acctQ(tr["gross_pnl"]), acctQ(tr["size"])), "pnl": acctQ(tr["trade_pnl"]), "side": tr["side"], "reason": reason, "tag": tag, "meta": meta}
		// Retained RunResult has its own 15-digit numeric projection. Raw accounting
		// comparisons below never go through this serialization relation.
		run["trades"] = append(acctList(run["trades"]), acctLegacyNumbers(t))
	}
	return run
}

func acctCheckSuccess(c, e acctObject, side string, f engine.RunFixture, run engine.RunResult, a engine.SequentialAccounting, m report.SequentialMetrics) {
	h, p := acctMap(e["headline"]), acctMap(c["profile"])
	acctRequire(len(run.ClockRangeDays) == 0 && run.TimedAudit == nil, "unrelated run audit")
	for _, t := range run.Trades {
		acctRequire(!t.Partial && t.Rule == "" && !t.NoTarget && !t.NoStop, "unexpected trade flags")
	}
	acctEqual(acctJSON(run), acctExpectedRun(c, e, side, f), "run.serialization")
	rawTrades, events := []any{}, []any{}
	balance := new(big.Rat)
	for i, x := range acctList(e["trades"]) {
		t := acctMap(x)
		ei, xi := acctInt(t["entry_bar"]), acctInt(t["exit_bar"])
		rawTrades = append(rawTrades, acctObject{"tradeIndex": big.NewRat(int64(i), 1), "entryIndex": acctQ(t["entry_bar"]), "exitIndex": acctQ(t["exit_bar"]), "entryT": new(big.Rat).SetFloat64(f.Bars[ei].T), "exitT": new(big.Rat).SetFloat64(f.Bars[xi].T), "side": t["side"], "entry": acctQ(t["entry_fill"]), "exit": acctQ(t["exit_fill"]), "size": acctQ(t["size"]), "points": new(big.Rat).Quo(acctQ(t["gross_pnl"]), acctQ(t["size"])), "entryFee": acctQ(t["entry_fee"]), "exitFee": acctQ(t["exit_fee"]), "exitCredit": acctQ(t["trade_pnl"]), "netPnl": acctQ(t["net_pnl"])})
		// This proof is restricted to this corpus's exact dyadic monetary controls:
		// both the fee product and each cumulative rational balance are representable
		// without rounding. It does not impose an unfused floating scalar replay on
		// general broker snapshots (which may use ARM64 fused entry arithmetic).
		feeProduct := new(big.Rat).Mul(new(big.Rat).SetFloat64(f.Costs.FeePerUnit), acctQ(t["size"]))
		acctDyadic(feeProduct)
		acctRequire(feeProduct.Cmp(acctQ(t["entry_fee"])) == 0, "exact dyadic fee operand proof")
		for j, kind := range []string{"entry", "exit"} {
			index, amount := ei, acctQ(t["entry_fee"])
			change := new(big.Rat).Neg(amount)
			if j == 1 {
				index, amount = xi, acctQ(t["trade_pnl"])
				change = amount
			}
			before := new(big.Rat).Set(balance)
			balance = new(big.Rat).Add(balance, change)
			acctDyadic(balance)
			events = append(events, acctObject{"kind": kind, "tradeIndex": big.NewRat(int64(i), 1), "index": big.NewRat(int64(index), 1), "t": new(big.Rat).SetFloat64(f.Bars[index].T), "realizedBefore": before, "realizedAfter": new(big.Rat).Set(balance), "amount": amount})
		}
	}
	marks := []any{}
	for i, x := range acctList(e["marks"]) {
		mark := acctMap(x)
		marks = append(marks, acctNumbers(acctObject{"index": mark["index"], "t": mark["t"], "realized": mark["realized"], "unrealized": mark["unrealized"], "equity": mark["equity"]}))
		if replaced, ok := mark["replaced_by_liquidation"]; ok {
			acctRequire(replaced == true && i == len(f.Bars)-1 && len(run.Trades) > 0 && run.Trades[len(run.Trades)-1].Reason == "end-of-test", "liquidation replacement annotation")
		}
	}
	raw := acctObject{"profile": p["id"], "startEquity": acctQ(h["startEquity"]), "endEquity": acctQ(h["endEquity"]), "finalRealized": acctQ(h["net"]), "marks": marks, "trades": rawTrades, "events": events}
	if policy, ok := p["policy"]; ok {
		raw["policy"] = policy
	}
	acctEqual(acctReflect(a), raw, "accounting.raw")
	// Independently assert actual state continuity rather than replaying nominal
	// event debits with a separately rounded floating arithmetic expression.
	previous := 0.0
	for i, event := range a.Events {
		acctRequire(math.Float64bits(event.RealizedBefore) == math.Float64bits(previous), "event %d mutation continuity", i)
		previous = event.RealizedAfter
	}
	acctRequire(math.Float64bits(previous) == math.Float64bits(a.FinalRealized), "terminal mutation continuity")
	for i, t := range a.Trades {
		expected := acctMap(acctList(e["trades"])[i])
		direction := int64(1)
		if t.Side == "short" {
			direction = -1
		}
		level := new(big.Rat).Add(new(big.Rat).SetFloat64(t.Exit), new(big.Rat).Mul(new(big.Rat).SetFloat64(f.Costs.Slippage), big.NewRat(direction, 1)))
		acctRequire(level.Cmp(acctQ(expected["exit_level"])) == 0, "exit level %d", i)
	}
	equity := []any{}
	for _, x := range acctList(e["marks"]) {
		mark := acctMap(x)
		equity = append(equity, acctNumbers(acctObject{"index": mark["index"], "t": mark["t"], "equity": mark["equity"]}))
	}
	metrics := acctObject{"basis": "sequential-broker-close-mtm-v1", "headline": acctHeadline(e), "tradeAccounting": acctNumbers(e["trade_accounting"]), "equity": equity}
	actual := acctReflect(m)
	acctEqual(actual, metrics, "metrics.raw")
	acctFiniteNormalized(actual, "metrics")
	serialized := acctJSON(m)
	acctEqual(serialized, metrics, "metrics.shortest-roundtrip")
	acctFiniteNormalized(serialized, "metrics.JSON")
	// The extra authored retained-PnL inventory is checked in its own right.
	pnl := []any{}
	for i, t := range run.Trades {
		pnl = append(pnl, acctObject{"index": i, "pnl": t.PnL})
	}
	acctEqual(pnl, acctLegacyNumbers(acctNumbers(e["run_trade_pnl"])), "run_trade_pnl")
	acctDrawdown(e, a)
	if p["policy"] != nil && e["decision"] != nil {
		d := acctMap(e["decision"])
		anchor := acctMap(d["anchor"])
		input := acctMap(acctMap(c["inputs"])["decision"])
		from, to := acctInt(anchor["from_index"]), acctInt(anchor["to_index"])
		acctRequire(from == acctInt(input["anchor_from"]) && to == acctInt(input["anchor_to"]) && to == acctInt(d["index"]), "decision anchor indices")
		extreme := f.Bars[from].L
		if side == "short" {
			extreme = f.Bars[from].H
		}
		for _, bar := range f.Bars[from : to+1] {
			if side == "long" && bar.L < extreme {
				extreme = bar.L
			}
			if side == "short" && bar.H > extreme {
				extreme = bar.H
			}
		}
		acctEqual(extreme, acctQ(anchor["price"]), "anchor.price")
		opportunity := run.SequentialFull.Opportunities[0]
		buffer := new(big.Rat).Sub(new(big.Rat).SetFloat64(extreme), new(big.Rat).SetFloat64(opportunity.Stop))
		if side == "short" {
			buffer.Neg(buffer)
		}
		acctRequire(buffer.Cmp(acctQ(d["stop_buffer"])) == 0, "stop buffer")
	}
}
func acctRun(c acctObject, side string) {
	f := acctFixture(c, side)
	source := string(acctRead(filepath.Join("..", acctStrategyPaths[f.StrategyID])))
	e := acctMap(acctMap(acctMap(c["variants"])[side])["expected"])
	run, a, err := engine.RunSequentialFixtureWithAccounting(f, source)
	if e["outcome"] == "refused" {
		acctCheckRefusal(e, run, a, err)
		return
	}
	acctRequire(err == nil, "execution: %v", err)
	metrics, err := report.ProjectSequentialMetrics(run, a)
	acctRequire(err == nil, "projection: %v", err)
	acctCheckSuccess(c, e, side, f, run, a, metrics)
}

func TestSequentialAccountingCorpus(t *testing.T) {
	if err := acctAttempt(acctLayouts); err != nil {
		t.Fatal(err)
	}
	var cases []acctObject
	if err := acctAttempt(func() { cases = acctLoad(acctPacket()) }); err != nil {
		t.Fatal(err)
	}
	binding, diagnostic := 0, 0
	for _, c := range cases {
		for _, side := range []string{"long", "short"} {
			id := acctText(c["id"]) + "/" + side
			if c["status"] == "provisional" {
				diagnostic++
				t.Run("provisional-diagnostic/"+id, func(t *testing.T) {
					if err := acctAttempt(func() { acctRun(c, side) }); err != nil {
						t.Logf("PROVISIONAL diagnostic mismatch (not a binding assertion): %v", err)
					} else {
						t.Log("PROVISIONAL diagnostic matched; not promoted to binding")
					}
				})
				continue
			}
			binding++
			t.Run("binding/"+id, func(t *testing.T) {
				if err := acctAttempt(func() { acctRun(c, side) }); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	if binding != 58 || diagnostic != 2 {
		t.Fatalf("executed inventory: binding=%d, diagnostic=%d", binding, diagnostic)
	}
	t.Logf("Executed 58 binding variants and 2 explicitly provisional diagnostics. Eight injected-only descriptions were inventoried but NOT executed. No unexecuted architecture is qualified.")
}

func TestSequentialAccountingCorpusAdapterGuards(t *testing.T) {
	reject := func(name string, f func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if err := acctAttempt(f); err == nil {
				t.Fatal("corruption accepted")
			}
		})
	}
	for name, text := range map[string]string{
		"duplicate-root":   `{"a":1,"a":2}`,
		"duplicate-nested": `{"x":[{"a":1,"\u0061":2}]}`,
		"trailing-value":   `{} []`,
		"nonfinite-token":  `{"x":NaN}`,
		"malformed-array":  `[1,]`,
	} {
		text := text
		reject(name, func() { acctDecode([]byte(text)) })
	}
	reject("inventory-missing", func() { acctInventory([]string{"a"}, "a b") })
	reject("inventory-extra", func() { acctInventory([]string{"a", "b", "c"}, "a b") })
	reject("inventory-duplicate", func() { acctInventory([]string{"a", "a"}, "a b") })
	packet := acctPacket()
	for _, name := range []string{"MANIFEST.json", "SOURCES.json", "cases/full.json", "cases/legacy.json", "cases/provisional.json", "cases/refusals.json", "cases/injected.json", "coverage.json", "IMPORT.json", "README.md", "DERIVATIONS.md"} {
		name := name
		reject("digest/"+name, func() {
			copy := map[string][]byte{}
			for k, v := range packet {
				copy[k] = v
			}
			copy[name] = append(append([]byte(nil), copy[name]...), byte(' '))
			acctLoad(copy)
		})
	}
	reject("file-missing", func() {
		copy := map[string][]byte{}
		for k, v := range packet {
			if k != "cases/full.json" {
				copy[k] = v
			}
		}
		acctLoad(copy)
	})
	reject("file-extra", func() {
		copy := map[string][]byte{}
		for k, v := range packet {
			copy[k] = v
		}
		copy["unexpected.json"] = []byte(`{}`)
		acctLoad(copy)
	})
	cases := acctLoad(packet)
	original := cases[0]
	clone := func() acctObject { return acctMap(acctJSON(original)) }
	schemaMutations := map[string]func(acctObject){
		"missing-bars":    func(c acctObject) { delete(acctMap(acctMap(c["variants"])["long"]), "bars") },
		"extra-variant":   func(c acctObject) { acctMap(c["variants"])["flat"] = acctMap(c["variants"])["long"] },
		"missing-variant": func(c acctObject) { delete(acctMap(c["variants"]), "short") },
		"mistyped-bar": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			acctMap(acctList(v["bars"])[0])["c"] = json.Number("224")
		},
		"null-bar":          func(c acctObject) { v := acctMap(acctMap(c["variants"])["long"]); acctList(v["bars"])[0] = nil },
		"bad-rational":      func(c acctObject) { acctMap(c["costs"])["slippage"] = "1/0" },
		"nondyadic-control": func(c acctObject) { acctMap(c["costs"])["slippage"] = "1/10" },
		"fractional-index": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			acctMap(acctList(acctMap(v["expected"])["trades"])[0])["index"] = json.Number("0.5")
		},
		"boolean-count": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			acctMap(acctMap(v["expected"])["headline"])["trades"] = true
		},
		"missing-mark": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			e := acctMap(v["expected"])
			e["marks"] = acctList(e["marks"])[1:]
		},
		"extra-trade": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			e := acctMap(v["expected"])
			e["trades"] = append(acctList(e["trades"]), acctList(e["trades"])[0])
		},
		"duplicate-record-index": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			e := acctMap(v["expected"])
			acctMap(acctList(e["marks"])[1])["index"] = json.Number("0")
		},
		"missing-null-ratio": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			delete(acctMap(acctMap(v["expected"])["headline"]), "profitFactor")
		},
		"defined-ratio-with-reason": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			acctMap(acctMap(v["expected"])["headline"])["winRateReason"] = "no-trades"
		},
		"unknown-expected-field": func(c acctObject) {
			v := acctMap(acctMap(c["variants"])["long"])
			acctMap(v["expected"])["future_number"] = "0"
		},
	}
	for name, mutate := range schemaMutations {
		mutate := mutate
		reject("schema/"+name, func() { c := clone(); mutate(c); acctValidateCase(c) })
	}
	for name, got := range map[string]any{
		"extra-key":      acctObject{"net": float64(0), "future": float64(0)},
		"missing-key":    acctObject{},
		"numeric-string": acctObject{"net": "0"},
		"numeric-bool":   acctObject{"net": false},
		"numeric-null":   acctObject{"net": nil},
		"nextafter":      acctObject{"net": math.SmallestNonzeroFloat64},
	} {
		got := got
		reject("comparator/"+name, func() { acctEqual(got, acctObject{"net": new(big.Rat)}, "guard") })
	}
	reject("array-extra", func() { acctEqual([]any{float64(0), float64(0)}, []any{new(big.Rat)}, "guard") })
	reject("array-missing", func() { acctEqual([]any{}, []any{new(big.Rat)}, "guard") })
	reject("array-null", func() { acctEqual(nil, []any{}, "guard") })
	reject("ordered-zero-sign", func() { acctEqual(math.Copysign(0, -1), acctOrdered(0), "guard") })
	reject("normalized-zero-sign", func() { acctFiniteNormalized(acctObject{"net": math.Copysign(0, -1)}, "guard") })
	reject("reflection-extra-float", func() {
		v := struct {
			Net    float64 `json:"net"`
			Future float64 `json:"future"`
		}{}
		acctEqual(acctReflect(v), acctObject{"net": new(big.Rat)}, "guard")
	})
	reject("reflection-ignored-field", func() {
		acctReflect(struct {
			Hidden float64 `json:"-"`
		}{})
	})
	reject("reflection-missing-tag", func() { acctReflect(struct{ Future float64 }{}) })
	reject("reflection-new-kind", func() {
		acctReflect(struct {
			Future float32 `json:"future"`
		}{})
	})
	reject("reflection-nil-record-list", func() {
		acctEqual(acctReflect(struct {
			Marks []engine.SequentialEquityMark `json:"marks"`
		}{}), acctObject{"marks": []any{}}, "guard")
	})
	// Record corruption is tested beyond packet hashes and shape admission: these
	// edits remain well-typed and must fail the real execution-output comparator.
	for name, mutate := range map[string]func(acctObject){
		"headline-money":     func(e acctObject) { acctMap(e["headline"])["net"] = "368.75000000000001" },
		"mark-money":         func(e acctObject) { acctMap(acctList(e["marks"])[12])["realized"] = "0" },
		"mark-time":          func(e acctObject) { acctMap(acctList(e["marks"])[12])["t"] = json.Number("1767614400001") },
		"entry-fee":          func(e acctObject) { acctMap(acctList(e["trade_accounting"])[0])["entryFee"] = "0" },
		"exit-credit":        func(e acctObject) { acctMap(acctList(e["run_trade_pnl"])[0])["pnl"] = "368.75" },
		"all-fee-net":        func(e acctObject) { acctMap(acctList(e["trades"])[0])["net_pnl"] = "371.875" },
		"categorical-reason": func(e acctObject) { acctMap(acctList(e["trades"])[0])["exit_reason"] = "sl" },
		"hold-bars":          func(e acctObject) { acctMap(acctList(e["trades"])[0])["hold_bars"] = json.Number("3") },
		"drawdown-location":  func(e acctObject) { acctMap(acctMap(e["drawdown_points"])["maxDD"])["bar"] = json.Number("13") },
	} {
		mutate := mutate
		reject("records/"+name, func() {
			c := clone()
			e := acctMap(acctMap(acctMap(c["variants"])["long"])["expected"])
			mutate(e)
			acctRun(c, "long")
		})
	}
}

func TestSequentialAccountingCorpusRatioOrder(t *testing.T) {
	cases := acctLoad(acctPacket())
	distinguish := 0
	for _, c := range cases {
		for _, side := range []string{"long", "short"} {
			e := acctMap(acctMap(acctMap(c["variants"])[side])["expected"])
			if e["outcome"] != "ok" {
				continue
			}
			h := acctMap(e["headline"])
			ordered := acctHeadline(e)
			for _, name := range []string{"returnPct", "maxDDpct", "winRate", "profitFactor", "expectancy", "avgWin", "avgLoss", "avgHoldBars"} {
				if h[name] == nil {
					continue
				}
				want := ordered[name].(acctOrdered)
				once := acctRound(acctQ(h[name]))
				if math.Float64bits(once) != math.Float64bits(float64(want)) {
					distinguish++
					if err := acctAttempt(func() { acctEqual(once, want, "wrong single-rounding schedule") }); err == nil {
						t.Fatalf("%s/%s %s accepted wrong schedule", c["id"], side, name)
					}
				}
			}
		}
	}
	if distinguish != 8 {
		t.Fatalf("operation-order witnesses=%d, want 8 preserved drawdown variants", distinguish)
	}
	// Rational values must not be mistaken for a rounded float or JSON string.
	if err := acctAttempt(func() { acctEqual(1.0/3, acctQ("1/3"), "semantic rational") }); err == nil {
		t.Fatal("rounded 1/3 accepted as exact rational")
	}
}

// Object comparison detects data-path drift. This separate type inventory also
// detects newly added omitempty fields whose zero value would otherwise hide
// them, and fields excluded by custom JSON serialization. A new exported path
// needs an explicit adapter decision even when every current case leaves it zero.
func acctFields(v any, want string) {
	typ := reflect.TypeOf(v)
	got := []string{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		acctRequire(f.IsExported(), "unreviewed private field %s.%s", typ, f.Name)
		got = append(got, f.Name+":"+f.Type.String())
	}
	acctInventory(got, want)
}
func acctLayouts() {
	for _, x := range []struct {
		value  any
		fields string
	}{
		{engine.SequentialAccountingError{}, "Kind:string Field:string BarIndex:int"},
		{engine.SequentialFullExecutionError{}, "Kind:string Field:string BarIndex:int OpportunityID:string"},
		{engine.SequentialAccounting{}, "Profile:string Policy:string StartEquity:float64 EndEquity:float64 FinalRealized:float64 Marks:[]engine.SequentialEquityMark Trades:[]engine.SequentialTradeAccounting Events:[]engine.SequentialAccountingEvent"},
		{engine.SequentialEquityMark{}, "Index:int T:float64 Realized:float64 Unrealized:float64 Equity:float64"},
		{engine.SequentialTradeAccounting{}, "TradeIndex:int EntryIndex:int ExitIndex:int EntryT:float64 ExitT:float64 Side:string Entry:float64 Exit:float64 Size:float64 Points:float64 EntryFee:float64 ExitFee:float64 ExitCredit:float64 NetPnL:float64"},
		{engine.SequentialAccountingEvent{}, "Kind:string TradeIndex:int Index:int T:float64 RealizedBefore:float64 RealizedAfter:float64 Amount:float64"},
		{report.SequentialMetrics{}, "Basis:string Headline:report.SequentialHeadline TradeAccounting:[]report.SequentialTradeAccounting Equity:[]report.SequentialEquityPoint"},
		{report.SequentialHeadline{}, "StartEquity:float64 EndEquity:float64 Net:float64 ReturnPct:float64 Trades:int WinRate:*float64 WinRateReason:string ProfitFactor:*float64 ProfitFactorReason:string Expectancy:*float64 ExpectancyReason:string AvgWin:*float64 AvgWinReason:string AvgLoss:*float64 AvgLossReason:string WorstLoss:float64 MaxDD:float64 MaxDDpct:float64 MaxWinStreak:int MaxLossStreak:int AvgHoldBars:*float64 AvgHoldBarsReason:string"},
		{report.SequentialTradeAccounting{}, "Index:int EntryFee:float64 ExitFee:float64 NetPnL:float64"},
		{report.SequentialEquityPoint{}, "Index:int T:float64 Equity:float64"},
		{engine.Costs{}, "FeePerUnit:float64 FillOn:string Slippage:float64 SlippageBps:float64 StartEquity:float64"},
		{engine.RunResult{}, "SequentialFull:*engine.SequentialFullAudit ClockRangeDays:[]engine.ClockRangeDay TimedAudit:*engine.TimedReturnAudit Case:string Costs:engine.Costs HigherTimeframe:string RangeMethod:string Schema:string StrategyID:string Symbol:string Timeframe:string TradeCount:int Trades:[]engine.Trade"},
		{engine.Trade{}, "Entry:float64 EntryIndex:int EntryT:float64 Exit:float64 ExitIndex:int ExitT:float64 InitialSL:float64 InitialTP:float64 Meta:engine.TradeMeta Partial:bool PnL:float64 Points:float64 Reason:string Rule:string Side:string Size:float64 SL:float64 Tag:string TP:float64 NoTarget:bool NoStop:bool"},
		{engine.SequentialFullAudit{}, "Schema:string Profile:string Policy:string Symbol:string Timeframe:string Opportunities:[]engine.SequentialFullOpportunity"},
		{engine.SequentialFullOpportunity{}, "ID:string EpisodeID:string Trigger:string Side:string SetupIndex:int SetupFirstIndex:int DecisionIndex:int DecisionOpenMS:int64 DecisionMS:int64 NextOpenIndex:int Status:engine.SequentialFullStatus Reason:engine.SequentialFullReason Stop:float64 SignalATR:float64 FillIndex:*int Fill:*float64 Target:*float64 RiskDistance:*float64 Size:*float64 CapBinds:bool"},
	} {
		acctFields(x.value, x.fields)
	}
}
func TestSequentialAccountingCorpusReflectionContract(t *testing.T) {
	if err := acctAttempt(acctLayouts); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"new-omitted-float": struct {
			Net    float64 `json:"net"`
			Future float64 `json:"future,omitempty"`
		}{},
		"new-nil-pointer": struct {
			Net    float64  `json:"net"`
			Future *float64 `json:"future,omitempty"`
		}{},
		"new-empty-slice": struct {
			Net    float64   `json:"net"`
			Future []float64 `json:"future,omitempty"`
		}{},
		"changed-number-type": struct {
			Net int `json:"net"`
		}{},
	} {
		t.Run(name, func(t *testing.T) {
			if err := acctAttempt(func() { acctFields(value, "Net:float64") }); err == nil {
				t.Fatal("unreviewed reflection path/type accepted")
			}
		})
	}
}

func acctCheckRefusal(e acctObject, run engine.RunResult, a engine.SequentialAccounting, err error) {
	acctRequire(err != nil, "expected refusal, got success")
	acctRequire(reflect.DeepEqual(run, engine.RunResult{}) && reflect.DeepEqual(a, engine.SequentialAccounting{}), "refusal leaked partial execution/accounting")
	// Corpus advisory code/field/index/precedence labels remain nonbinding. The
	// Go error family must nevertheless correspond to the intended failure cause.
	var execution *engine.SequentialFullExecutionError
	var accounting *engine.SequentialAccountingError
	switch acctMap(e["refusal"])["type"] {
	case "incomplete-terminal-run":
		acctRequire(errors.As(err, &execution) && execution != nil, "expected typed terminal refusal, got %T", err)
	case "start-equity-not-positive", "nonfinite-value":
		acctRequire(errors.As(err, &accounting) && accounting != nil, "expected typed accounting refusal, got %T", err)
	default:
		acctRequire(false, "unrecognized refusal type %v", acctMap(e["refusal"])["type"])
	}
}

// This is a separately authorized candidate regression for the published public
// source at acctCommit, NOT authority from the independently authored corpus.
// The corpus's code_binding/index_binding/reason_binding flags remain false.
// Changes to candidate refusal identity need explicit regression review; they
// must never be disguised as promotion of the corpus's advisory suggestions.
func acctCandidateRefusalIdentity(c acctObject, side string, f engine.RunFixture, err error) {
	acctRequire(err != nil, "candidate refusal unexpectedly succeeded")
	id := acctText(c["id"])
	switch id {
	case "ref.nonpositive_start_equity", "ref.overflow_entry_fee":
		var got *engine.SequentialAccountingError
		acctRequire(errors.As(err, &got) && got != nil, "candidate accounting error family: %T", err)
		field, index := "startEquity", -1
		if id == "ref.overflow_entry_fee" {
			field, index = "entry", 12
		}
		acctEqual(acctReflect(*got), acctObject{"kind": "invalid-sequential-accounting", "field": field, "barIndex": big.NewRat(int64(index), 1)}, "candidate.refusal")
	case "ref.e1_open_at_end_entry_bar", "ref.e1_open_at_end_last_checked_bar", "ref.e2_open_at_end_last_checked_bar":
		var got *engine.SequentialFullExecutionError
		acctRequire(errors.As(err, &got) && got != nil, "candidate terminal error family: %T", err)
		p := acctMap(c["profile"])
		decision := acctMap(acctMap(c["inputs"])["decision"])
		setup := acctInt(decision["anchor_from"]) + 8
		direction, trigger := "buy", "perf"
		if side == "short" {
			direction = "sell"
		}
		if p["policy"] == "E2" {
			trigger = "cd13"
		}
		opportunity := fmt.Sprintf("%s:%s:%s:%s:%.0f:%s", p["id"], f.Symbol, f.Timeframe, direction, f.Bars[setup].T, trigger)
		acctEqual(acctReflect(*got), acctObject{"kind": "unsupported-incomplete-terminal-run", "field": "held position", "barIndex": big.NewRat(int64(len(f.Bars)-1), 1), "opportunityId": opportunity}, "candidate.refusal")
	default:
		acctRequire(false, "unreviewed candidate refusal %s", id)
	}
}
func TestSequentialAccountingCorpusCandidateRefusalIdentity(t *testing.T) {
	cases := acctLoad(acctPacket())
	count := 0
	for _, c := range cases {
		for _, side := range []string{"long", "short"} {
			e := acctMap(acctMap(acctMap(c["variants"])[side])["expected"])
			if e["outcome"] != "refused" {
				continue
			}
			count++
			t.Run(acctText(c["id"])+"/"+side, func(t *testing.T) {
				if err := acctAttempt(func() {
					f := acctFixture(c, side)
					run, a, err := engine.RunSequentialFixtureWithAccounting(f, string(acctRead(filepath.Join("..", acctStrategyPaths[f.StrategyID]))))
					acctCheckRefusal(e, run, a, err)
					acctCandidateRefusalIdentity(c, side, f, err)
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	if count != 10 {
		t.Fatalf("candidate refusal regressions=%d, want 10", count)
	}
	t.Log("10 exact published-candidate refusal identities checked separately; corpus advisory labels remain nonbinding")
}
func TestSequentialAccountingCorpusRefusalGuards(t *testing.T) {
	cases := acctLoad(acctPacket())
	var terminal, accounting acctObject
	for _, c := range cases {
		if c["id"] == "ref.e1_open_at_end_entry_bar" {
			terminal = c
		}
		if c["id"] == "ref.nonpositive_start_equity" {
			accounting = c
		}
	}
	for name, c := range map[string]acctObject{"terminal": terminal, "accounting": accounting} {
		f := acctFixture(c, "long")
		e := acctMap(acctMap(acctMap(c["variants"])["long"])["expected"])
		_, _, valid := engine.RunSequentialFixtureWithAccounting(f, string(acctRead(filepath.Join("..", acctStrategyPaths[f.StrategyID]))))
		reject := func(suffix string, check func()) {
			t.Run(name+"/"+suffix, func(t *testing.T) {
				if err := acctAttempt(check); err == nil {
					t.Fatal("invalid refusal accepted")
				}
			})
		}
		reject("empty-success", func() { acctCheckRefusal(e, engine.RunResult{}, engine.SequentialAccounting{}, nil) })
		reject("untyped-error", func() {
			acctCheckRefusal(e, engine.RunResult{}, engine.SequentialAccounting{}, fmt.Errorf("unsupported-incomplete-terminal-run"))
		})
		reject("partial-run", func() { acctCheckRefusal(e, engine.RunResult{TradeCount: 1}, engine.SequentialAccounting{}, valid) })
		reject("partial-accounting", func() {
			acctCheckRefusal(e, engine.RunResult{}, engine.SequentialAccounting{Marks: []engine.SequentialEquityMark{{Index: 0}}}, valid)
		})
		wrongFamily := error(&engine.SequentialFullExecutionError{Kind: "invalid-sequential-input"})
		if name == "terminal" {
			wrongFamily = &engine.SequentialAccountingError{Kind: "invalid-sequential-accounting"}
		}
		reject("wrong-error-family", func() { acctCheckRefusal(e, engine.RunResult{}, engine.SequentialAccounting{}, wrongFamily) })
		mutations := map[string]error{}
		if name == "terminal" {
			original := *(valid.(*engine.SequentialFullExecutionError))
			for key := range map[string]bool{"empty-code": true, "wrong-kind": true, "wrong-field": true, "wrong-index": true, "wrong-opportunity": true} {
				m := original
				switch key {
				case "empty-code":
					m.Kind = ""
				case "wrong-kind":
					m.Kind = "invalid-sequential-input"
				case "wrong-field":
					m.Field = "bars"
				case "wrong-index":
					m.BarIndex--
				case "wrong-opportunity":
					m.OpportunityID = ""
				}
				mutations[key] = &m
			}
		} else {
			original := *(valid.(*engine.SequentialAccountingError))
			for key := range map[string]bool{"empty-code": true, "wrong-kind": true, "wrong-field": true, "wrong-index": true} {
				m := original
				switch key {
				case "empty-code":
					m.Kind = ""
				case "wrong-kind":
					m.Kind = "invalid-sequential-input"
				case "wrong-field":
					m.Field = "entry"
				case "wrong-index":
					m.BarIndex = 12
				}
				mutations[key] = &m
			}
		}
		for key, mutated := range mutations {
			mutated := mutated
			reject("candidate/"+key, func() { acctCandidateRefusalIdentity(c, "long", f, mutated) })
		}
	}
}
