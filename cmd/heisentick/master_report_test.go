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

const masterCLISource = `dsl v7
strategy "Native synthetic CLI" { description "Invented bars" }
market { master timeframe M30 from M5 }
setup { type: master structural
master profile v10-phase0-floor-half-reference-v1
master mode SOURCE_HISTORICAL_REFERENCE
}`

func masterCLIInputs(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "synthetic.strat")
	if e := os.WriteFile(source, []byte(masterCLISource), 0600); e != nil {
		t.Fatal(e)
	}
	s := marketdata.NewSeries(300)
	for i := range s.T {
		s.T[i] = float64(i * 300000)
		s.O[i] = 100
		s.H[i] = 101
		s.L[i] = 99
		s.C[i] = 100
		s.V[i] = 1
	}
	data := filepath.Join(dir, "5m.bin")
	if e := os.WriteFile(data, marketdata.EncodeBTB1(s), 0600); e != nil {
		t.Fatal(e)
	}
	stamp := func(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339) }
	return []string{"--dsl-file=" + source, "--m5-file=" + data, "--warmup-from=" + stamp(0), "--trade-from=" + stamp(40*1800000), "--trade-to=" + stamp(50*1800000), "--spread=0"}
}
func TestMasterCLICompleteEnvelopeAndGenericRefusal(t *testing.T) {
	args := masterCLIInputs(t)
	var out bytes.Buffer
	if e := run(append([]string{"master-report"}, args...), &out); e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	if e := json.Unmarshal(out.Bytes(), &v); e != nil {
		t.Fatal(e)
	}
	if v["schema"] != "strat-master-structural-cli-v1" || len(v["dataSha256"].(string)) != 64 || len(v["configSha256"].(string)) != 64 {
		t.Fatal("bad provenance")
	}
	r := v["run"].(map[string]any)
	if r["pineParityVerified"] != false || r["costComplete"] != false || r["openPosition"] != nil {
		t.Fatal("unsupported claims")
	}
	for _, command := range []string{"report", "grid", "forward-prefix"} {
		out.Reset()
		e := run([]string{command, args[0]}, &out)
		if e == nil || !strings.Contains(e.Error(), dsl.MasterStructuralDedicatedRunnerRequired) || out.Len() != 0 {
			t.Fatalf("%s:%v output%s", command, e, out.String())
		}
	}
}
func TestMasterCLIRejectsBadFlagsAndBTB1(t *testing.T) {
	for _, extra := range [][]string{{"--spread=1"}, {"--unknown=1"}, {"--m5-file"}, {"--spread=NaN"}} {
		args := append(masterCLIInputs(t), extra...)
		var out bytes.Buffer
		if e := runMasterReport(args, &out); e == nil || out.Len() != 0 {
			t.Fatalf("bad flags:%v", extra)
		}
	}
	args := masterCLIInputs(t)
	args[5] = "--spread=0.2"
	var out bytes.Buffer
	if e := runMasterReport(args, &out); e == nil || out.Len() != 0 {
		t.Fatal("unfrozen cost accepted")
	}
	args = masterCLIInputs(t)
	path := strings.TrimPrefix(args[1], "--m5-file=")
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	data[12] = 5
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	if e = runMasterReport(args, &out); e == nil || out.Len() != 0 {
		t.Fatal("missing volume silently defaulted")
	}
}
