package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const rrReportDSL = `dsl v7
strategy "Range reversion report test" { description "Dedicated report command fixture" }
market {
 rangereversion timeframe M30
 rangereversion source H4 availability next-native-row
}
setup {
 type: range reversion
 rangereversion policy DELAYED_PINE_OHLC_V1
 rangereversion bounds chart 2
 rangereversion candle-color true
 rangereversion htf-ema false 200
 rangereversion vector-gates false 14 30 48
 rangereversion range-expansion false 1.1
 rangereversion atr 2
 rangereversion stop-atr 0.6
 rangereversion target-r 2
 rangereversion cooldown 2
 rangereversion breakeven false 1 10
 rangereversion tick-size 0.01
}
`

func rrWriteBTB1(t *testing.T, path string, bars []marketdata.Bar) {
	t.Helper()
	if err := os.WriteFile(path, marketdata.EncodeBTB1(marketdata.SeriesFromBars(bars)), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRangeReversionReportRequiresExplicitInputsAndHashes(t *testing.T) {
	dir := t.TempDir()
	dslPath, entryPath, sourcePath := filepath.Join(dir, "r.strat"), filepath.Join(dir, "entry.btb1"), filepath.Join(dir, "source.btb1")
	if err := os.WriteFile(dslPath, []byte(rrReportDSL), 0600); err != nil {
		t.Fatal(err)
	}
	entry := []marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 1800000, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 3600000, O: 101, H: 103, L: 98, C: 100, V: 1}, {T: 5400000, O: 100, H: 109, L: 99, C: 106, V: 1}, {T: 7200000, O: 100.2, H: 101, L: 99, C: 100, V: 1}}
	source := []marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 14400000, O: 100, H: 102, L: 98, C: 100, V: 1}, {T: 43200000, O: 100, H: 103, L: 97, C: 100, V: 1}}
	rrWriteBTB1(t, entryPath, entry)
	rrWriteBTB1(t, sourcePath, source)
	args := []string{"--dsl-file=" + dslPath, "--entry-bars-file=" + entryPath, "--source-bars-file=" + sourcePath, "--trade-from=1970-01-01T00:00:00Z", "--trade-to=1970-01-01T05:00:00Z", "--slippage-per-fill=0", "--commission-per-unit-side=0", "--units=1"}
	var out bytes.Buffer
	if err := runRangeReversionReport(args, &out); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Schema     string `json:"schema"`
		EntryHash  string `json:"entryBtb1Sha256"`
		SourceHash string `json:"sourceBtb1Sha256"`
		Run        struct {
			Trades []json.RawMessage `json:"trades"`
		} `json:"run"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Schema != "strat-range-reversion-report-v1" || envelope.EntryHash == "" || envelope.SourceHash == "" || len(envelope.Run.Trades) != 1 {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestRangeReversionReportRejectsUnknownFlagsWithoutPartialJSON(t *testing.T) {
	var out bytes.Buffer
	err := runRangeReversionReport([]string{"--dsl-file=x", "--entry-bars-file=y", "--source-bars-file=z", "--trade-from=1970-01-01T00:00:00Z", "--trade-to=1970-01-01T01:00:00Z", "--slippage-per-fill=0", "--commission-per-unit-side=0", "--units=1", "--cost=0"}, &out)
	if err == nil || !strings.Contains(err.Error(), "unknown") || out.Len() != 0 {
		t.Fatalf("err=%v output=%q", err, out.String())
	}
}

func TestRangeReversionReportDailyBarsAreRequiredOnlyForEnabledGate(t *testing.T) {
	dir := t.TempDir()
	dslPath := filepath.Join(dir, "daily.strat")
	entryPath := filepath.Join(dir, "entry.btb1")
	sourcePath := filepath.Join(dir, "source.btb1")
	dailyPath := filepath.Join(dir, "daily.btb1")
	dailyDSL := strings.Replace(rrReportDSL, "rangereversion atr 2", "rangereversion daily-chop 2 38.2 61.8\n rangereversion atr 2", 1)
	if err := os.WriteFile(dslPath, []byte(dailyDSL), 0600); err != nil {
		t.Fatal(err)
	}
	entry := []marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 1800000, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 3600000, O: 101, H: 103, L: 98, C: 100, V: 1}, {T: 5400000, O: 100, H: 109, L: 99, C: 106, V: 1}, {T: 7200000, O: 100.2, H: 101, L: 99, C: 100, V: 1}}
	source := []marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 14400000, O: 100, H: 102, L: 98, C: 100, V: 1}, {T: 43200000, O: 100, H: 103, L: 97, C: 100, V: 1}}
	daily := []marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 86400000, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 172800000, O: 100, H: 101, L: 99, C: 100, V: 1}}
	for path, bars := range map[string][]marketdata.Bar{entryPath: entry, sourcePath: source, dailyPath: daily} {
		rrWriteBTB1(t, path, bars)
	}
	args := []string{"--dsl-file=" + dslPath, "--entry-bars-file=" + entryPath, "--source-bars-file=" + sourcePath, "--trade-from=1970-01-01T00:00:00Z", "--trade-to=1970-01-01T05:00:00Z", "--slippage-per-fill=0", "--commission-per-unit-side=0", "--units=1"}
	var out bytes.Buffer
	if err := runRangeReversionReport(args, &out); err == nil || !strings.Contains(err.Error(), "daily-bars-file") || out.Len() != 0 {
		t.Fatalf("missing daily input error=%v output=%q", err, out.String())
	}
	args = append(args, "--daily-bars-file="+dailyPath)
	if err := runRangeReversionReport(args, &out); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		DailyHash string `json:"dailyBtb1Sha256"`
		Run       struct {
			DailyHash string `json:"dailySha256"`
			Input     int    `json:"inputDailyBars"`
			Used      int    `json:"usedDailyBars"`
		} `json:"run"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.DailyHash == "" || envelope.Run.DailyHash == "" || envelope.Run.Input != 3 || envelope.Run.Used != 1 {
		t.Fatalf("daily input hashes/clipping not reported: %+v", envelope)
	}

	if err := os.WriteFile(dslPath, []byte(rrReportDSL), 0600); err != nil {
		t.Fatal(err)
	}
	var disabledOut bytes.Buffer
	if err := runRangeReversionReport(args, &disabledOut); err == nil || !strings.Contains(err.Error(), "only accepted") || disabledOut.Len() != 0 {
		t.Fatalf("unexpected daily input error=%v output=%q", err, disabledOut.String())
	}
}
