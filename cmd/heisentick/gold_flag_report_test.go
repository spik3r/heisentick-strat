package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const goldFlagCLISource = `dsl v7
strategy "Native synthetic CLI" { description "Invented bars" }
market { goldflag timeframe M30 from M15 }
setup { type: gold flag reference
goldflag policy PR388_CAUSAL_STRESS_V1
}`

func goldFlagCLIInputs(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "synthetic.strat")
	if e := os.WriteFile(source, []byte(goldFlagCLISource), 0600); e != nil {
		t.Fatal(e)
	}
	s := marketdata.NewSeries(300)
	for i := range s.T {
		s.T[i] = float64(i * 900000)
		s.O[i] = 100
		s.H[i] = 101
		s.L[i] = 99
		s.C[i] = 100
		s.V[i] = 1
	}
	data := filepath.Join(dir, "15m.bin")
	if e := os.WriteFile(data, marketdata.EncodeBTB1(s), 0600); e != nil {
		t.Fatal(e)
	}
	stamp := func(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339) }
	return []string{"--dsl-file=" + source, "--m15-file=" + data, "--from=" + stamp(0), "--to=" + stamp(150*1800000), "--cost=0"}
}
func TestGoldFlagCLICompleteEnvelopeAndGenericRefusal(t *testing.T) {
	args := goldFlagCLIInputs(t)
	var out bytes.Buffer
	if e := run(append([]string{"gold-flag-report"}, args...), &out); e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	if e := json.Unmarshal(out.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	if v["schema"] != "strat-gold-flag-reference-cli-v1" || len(v["dataSha256"].(string)) != 64 || len(v["configSha256"].(string)) != 64 {
		t.Fatal("bad provenance")
	}
	r := v["run"].(map[string]any)
	if r["evidenceStatus"] != "deterministic-coarse-OHLC-stress-scenario-not-observed-execution" || r["terminalOrderId"] != nil {
		t.Fatal("unsupported claims")
	}
	for _, command := range []string{"report", "grid", "forward-prefix"} {
		out.Reset()
		e := run([]string{command, args[0]}, &out)
		if e == nil || !strings.Contains(e.Error(), dsl.GoldFlagReferenceDedicatedRunnerRequired) || out.Len() != 0 {
			t.Fatalf("%s:%v output%s", command, e, out.String())
		}
	}
}
func TestGoldFlagCLIRejectsBadFlagsAndBTB1(t *testing.T) {
	for _, extra := range [][]string{{"--cost=1"}, {"--unknown=1"}, {"--m15-file"}, {"--cost=NaN"}} {
		args := append(goldFlagCLIInputs(t), extra...)
		var out bytes.Buffer
		if e := runGoldFlagReport(args, &out); e == nil || out.Len() != 0 {
			t.Fatalf("bad flags:%v", extra)
		}
	}
	args := goldFlagCLIInputs(t)
	args[4] = "--cost=0.2"
	var out bytes.Buffer
	if e := runGoldFlagReport(args, &out); e == nil || out.Len() != 0 {
		t.Fatal("unfrozen cost accepted")
	}
	args = goldFlagCLIInputs(t)
	path := strings.TrimPrefix(args[1], "--m15-file=")
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	data[12] = 5
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	if e = runGoldFlagReport(args, &out); e == nil || out.Len() != 0 {
		t.Fatal("missing volume silently defaulted")
	}
}
