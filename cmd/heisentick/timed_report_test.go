package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func timedCLIInputs(t *testing.T) ([]string, engine.RunFixture, []byte) {
	t.Helper()
	name := "family-timed-return"
	f, err := engine.LoadRunFixture("../../conformance/run/" + name + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, f.Symbol), 0755); err != nil {
		t.Fatal(err)
	}
	data := marketdata.EncodeBTB1(marketdata.SeriesFromBars(f.Bars))
	if err := os.WriteFile(filepath.Join(root, f.Symbol, f.Timeframe+".bin"), data, 0644); err != nil {
		t.Fatal(err)
	}
	calendar, _ := json.Marshal(f.TimedCalendar)
	path := filepath.Join(root, "calendar.json")
	if err := os.WriteFile(path, calendar, 0644); err != nil {
		t.Fatal(err)
	}
	return []string{"timed-report", "--dsl-file=../../conformance/run/" + name + ".strat", "--calendar-file=" + path, "--symbol=" + f.Symbol, "--tf=" + f.Timeframe, "--data-root=" + root, "--slippage=0.1"}, f, data
}

func TestTimedReportActualCLIFlatPriceTwoFills(t *testing.T) {
	args, f, data := timedCLIInputs(t)
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Schema       string           `json:"schema"`
		DataHash     string           `json:"dataSha256"`
		DSLHash      string           `json:"dslSha256"`
		CalendarHash string           `json:"calendarFileSha256"`
		Net          float64          `json:"netPriceUnits"`
		CostComplete bool             `json:"costComplete"`
		Run          engine.RunResult `json:"run"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	if result.Schema != "strat-timed-return-report-v1" || result.DataHash != hex.EncodeToString(h[:]) || len(result.DSLHash) != 64 || len(result.CalendarHash) != 64 || result.CostComplete || result.Run.TradeCount != 1 || math.Abs(result.Net+0.2) > 1e-10 {
		t.Fatalf("%+v", result)
	}
	tr := result.Run.Trades[0]
	if tr.Size != 1 || math.Abs(tr.Entry-100.1) > 1e-10 || math.Abs(tr.Exit-99.9) > 1e-10 || tr.EntryT != f.Bars[121].T || tr.ExitT != f.Bars[181].T {
		t.Fatalf("%+v", tr)
	}
	// Zero slippage is a distinct declared cost assumption, not a default.
	args[len(args)-1] = "--slippage=0"
	out.Reset()
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Net != 0 {
		t.Fatalf("%v %+v", err, result)
	}
}

func TestTimedReportRefusesUnsupportedOrMissingFlags(t *testing.T) {
	args, _, _ := timedCLIInputs(t)
	for _, flag := range []string{"--fee-per-unit=1", "--from=2026-06-01", "--symbol=OTHERUSD", "--slippage=0.2", "--force-route=1", "--calendar-file=unused", "--include-trades=1"} {
		var out bytes.Buffer
		if err := run(append(append([]string{}, args...), flag), &out); err == nil || out.Len() != 0 {
			t.Fatalf("flag %s: %v %s", flag, err, out.String())
		}
	}
	var out bytes.Buffer
	if err := run(args[:len(args)-1], &out); err == nil {
		t.Fatal("missing explicit slippage accepted")
	}
	args[len(args)-1] = "--slippage=NaN"
	if err := run(args, &out); err == nil || out.Len() != 0 {
		t.Fatalf("%v %s", err, out.String())
	}
}

func TestTimedReportRefusesMalformedSidecarAndExecutionCoverage(t *testing.T) {
	args, f, _ := timedCLIInputs(t)
	calendarPath := strings.TrimPrefix(args[2], "--calendar-file=")
	raw, _ := json.Marshal(f.TimedCalendar)
	unknown := append([]byte(`{"ignoredCondition":true,`), raw[1:]...)
	for _, bad := range [][]byte{unknown, append(append([]byte{}, raw...), []byte(" {}")...)} {
		if err := os.WriteFile(calendarPath, bad, 0644); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := run(args, &out); err == nil || out.Len() != 0 {
			t.Fatal("malformed sidecar accepted")
		}
	}
	if err := os.WriteFile(calendarPath, raw, 0644); err != nil {
		t.Fatal(err)
	}
	f.Bars = f.Bars[:len(f.Bars)-1]
	root := strings.TrimPrefix(args[5], "--data-root=")
	if err := os.WriteFile(filepath.Join(root, f.Symbol, f.Timeframe+".bin"), marketdata.EncodeBTB1(marketdata.SeriesFromBars(f.Bars)), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(args, &out); err == nil || !strings.Contains(err.Error(), "dataset inadmissible") || out.Len() != 0 {
		t.Fatalf("%v %s", err, out.String())
	}
}

func TestTimedReportCostsRequireExplicitValues(t *testing.T) {
	args, _, _ := timedCLIInputs(t)
	withoutSlip := args[:len(args)-1]
	for _, tail := range [][]string{
		{"--slippage"}, {"--slippage", "--slippage-bps=0"}, {"--slippage="}, {"--slippage", ""},
		{"--slippage=0.1", "--slippage-bps"}, {"--slippage=0.1", "--slippage-bps="},
		{"--slippage=0.1", "--slippage-bps", "--unknown=1"},
	} {
		var out bytes.Buffer
		err := run(append(append([]string{}, withoutSlip...), tail...), &out)
		if err == nil || !strings.Contains(err.Error(), "explicit") || out.Len() != 0 {
			t.Fatalf("tail %q: %v %s", tail, err, out.String())
		}
	}
	// The space-separated form is explicitly valued and remains supported.
	var out bytes.Buffer
	err := run(append(append([]string{}, withoutSlip...), "--slippage", "0.1", "--slippage-bps", "0"), &out)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Net float64 `json:"netPriceUnits"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || math.Abs(result.Net+0.2) > 1e-10 {
		t.Fatalf("%v %+v", err, result)
	}
}
