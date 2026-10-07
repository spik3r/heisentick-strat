package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func adaptiveCLIInputs(t *testing.T) (string, string, []byte, []byte) {
	t.Helper()
	source, e := os.ReadFile(filepath.Join(testsupport.StratConformanceRoot(), "parse", "family-adaptive-volume-flag-initial.strat"))
	if e != nil {
		t.Fatal(e)
	}
	bars := []marketdata.Bar{}
	for i := 0; i < 20; i++ {
		bars = append(bars, marketdata.Bar{T: float64(i * 1800000), O: 100, H: 101, L: 99, C: 100, V: 100})
	}
	values := [][5]float64{{100, 100, 97, 99, 100}, {100, 103, 99, 102, 100}, {103, 105, 102, 104, 100}, {105, 107, 104, 106, 100}, {107, 108, 106, 107.5, 100}, {107, 107.5, 106.5, 107, 100}, {107, 107.4, 106.4, 107, 100}, {107, 107.6, 106.2, 107.3, 200}, {107, 120, 102, 119, 100}, {119, 120, 118, 119, 100}}
	for _, v := range values {
		bars = append(bars, marketdata.Bar{T: float64(len(bars) * 1800000), O: v[0], H: v[1], L: v[2], C: v[3], V: v[4]})
	}
	data := marketdata.EncodeBTB1(marketdata.SeriesFromBars(bars))
	dir := t.TempDir()
	sp, bp := filepath.Join(dir, "fixture.strat"), filepath.Join(dir, "fixture.btb1")
	if e = os.WriteFile(sp, source, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(bp, data, 0600); e != nil {
		t.Fatal(e)
	}
	return sp, bp, source, data
}

func TestAdaptiveFlagNativeCLIExecutesRawTrace(t *testing.T) {
	sp, bp, source, data := adaptiveCLIInputs(t)
	var out bytes.Buffer
	if e := run([]string{"adaptive-flag-report", "--dsl-file=" + sp, "--bars-file=" + bp}, &out); e != nil {
		t.Fatal(e)
	}
	var result map[string]any
	if e := json.Unmarshal(out.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	hash := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	if result["dslSha256"] != hash(source) || result["btb1Sha256"] != hash(data) {
		t.Fatal("byte identities missing")
	}
	run := result["run"].(map[string]any)
	orders := run["orders"].([]any)
	if len(orders) != 1 {
		t.Fatalf("nonempty order trace missing: %v", orders)
	}
	order := orders[0].(map[string]any)
	if order["signalIdx"] != float64(27) || order["fillIdx"] != float64(28) || order["exitIdx"] != float64(29) || order["exit"] != float64(119) || order["reason"] != "target-gap" {
		t.Fatalf("wrong raw lifecycle:%v", order)
	}
	for _, word := range []string{`"qty":`, `"equity":`, `"netPnl":`, `"returnPct":`, `"pointValue":`, `"size":`} {
		if strings.Contains(out.String(), word) {
			t.Fatal("unapproved economic field", word)
		}
	}
}

func TestAdaptiveFlagCLIRejectsUnknownCostsAndMalformedBytes(t *testing.T) {
	sp, bp, _, data := adaptiveCLIInputs(t)
	base := []string{"--dsl-file=" + sp, "--bars-file=" + bp}
	for _, extra := range []string{"--cost=0", "--slippage=0", "--equity=10000", "--dsl-file=" + sp, "--from=0"} {
		var out bytes.Buffer
		if e := runAdaptiveFlagReport(append(append([]string{}, base...), extra), &out); e == nil || out.Len() != 0 {
			t.Fatal("unexpected flag accepted", extra, e)
		}
	}
	for _, bad := range [][]byte{data[:15], append(append([]byte{}, data...), 0), []byte("not BTB1")} {
		if e := os.WriteFile(bp, bad, 0600); e != nil {
			t.Fatal(e)
		}
		var out bytes.Buffer
		if e := runAdaptiveFlagReport(base, &out); e == nil || out.Len() != 0 {
			t.Fatal("invalid byte input accepted", e)
		}
	}
	for _, command := range []string{"report", "grid", "forward-prefix"} {
		var out bytes.Buffer
		if e := run([]string{command, "--dsl-file=" + sp, "--symbol=SYNTHETIC", "--tf=30m", "--range=pivot"}, &out); e == nil || !strings.Contains(e.Error(), "adaptive-volume-flag-native-dedicated-runner-required") {
			t.Fatalf("generic route %s:%v", command, e)
		}
	}
}

func TestAdaptiveFlagCLIOptionalExecutionWindow(t *testing.T) {
	sp, bp, _, _ := adaptiveCLIInputs(t)
	base := []string{"--dsl-file=" + sp, "--bars-file=" + bp}
	var out bytes.Buffer
	args := append(append([]string{}, base...), "--trade-from=1970-01-01T13:30:00Z", "--trade-to=1970-01-01T14:00:00Z")
	if e := runAdaptiveFlagReport(args, &out); e != nil {
		t.Fatal(e)
	}
	var doc map[string]any
	if e := json.Unmarshal(out.Bytes(), &doc); e != nil {
		t.Fatal(e)
	}
	run := doc["run"].(map[string]any)
	window := run["executionWindow"].(map[string]any)
	if run["usedSourceRows"] != float64(28) || run["preTradeRows"] != float64(27) || run["eligibleTradeRows"] != float64(1) || window["tradeFromMs"] != float64(27*1800000) || run["terminal"].(map[string]any)["status"] != "pending" {
		t.Fatal("CLI window not honored", run)
	}
	for _, extra := range [][]string{
		{"--trade-from=1970-01-01T00:00:00Z"}, {"--trade-to=1970-01-02T00:00:00Z"},
		{"--trade-from=1970-01-01T00:00:00+00:00", "--trade-to=1970-01-02T00:00:00Z"},
		{"--trade-from=1970-01-01T00:00:01Z", "--trade-to=1970-01-02T00:00:00Z"},
		{"--trade-from=1970-01-02T00:00:00Z", "--trade-to=1970-01-01T00:00:00Z"},
	} {
		out.Reset()
		if e := runAdaptiveFlagReport(append(append([]string{}, base...), extra...), &out); e == nil || out.Len() != 0 {
			t.Fatal("bad window accepted", extra, e)
		}
	}
}
