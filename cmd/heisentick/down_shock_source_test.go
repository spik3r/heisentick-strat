package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestDownShockCLIReportLoadsAndUsesSource(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "XAUUSD"), 0o755); err != nil {
		t.Fatal(err)
	}
	chart, source := testsupport.DownShockBars()
	dslPath := filepath.Join(root, "source.strat")
	for path, data := range map[string][]byte{
		dslPath:                                  []byte(testsupport.DownShockSource),
		filepath.Join(root, "XAUUSD", "1m.bin"):  marketdata.EncodeBTB1(marketdata.SeriesFromBars(chart)),
		filepath.Join(root, "XAUUSD", "15m.bin"): marketdata.EncodeBTB1(marketdata.SeriesFromBars(source)),
	} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"report", "--dsl-file=" + dslPath, "--symbol=XAUUSD", "--tf=1m", "--range=zone",
		"--data-root=" + root, "--slippage=0", "--include-trades=1", "--json-only=1"}
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	var document report.Document
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Warnings) != 0 || len(document.Costs) != 1 || document.Costs[0].Trades != 1 ||
		math.Abs(document.Costs[0].Net-37.2341666666695) > 1e-10 || len(document.Slices) != 1 {
		t.Fatalf("native report lost the admitted source: %s", out.String())
	}
	slice := document.Slices[0]
	if slice.SourceBars == nil || *slice.SourceBars != 461 || slice.Trades == nil || len(*slice.Trades) != 1 {
		t.Fatalf("slice: %+v", slice)
	}
	trade := (*slice.Trades)[0]
	if trade.Side != "long" || trade.EntryIndex != 1 || trade.EntryT != chart[1].T || trade.Entry != 98.2 || trade.Reason != "tp" ||
		trade.Meta["sourceRow"] != float64(460) || trade.Meta["sourceCloseT"] != chart[1].T {
		t.Fatalf("trade: %+v", trade)
	}
	assertNoTemporaryInputPaths(t, out.String(), dslPath, root)
	// A missing source file must remain an error, never a successful zero report.
	if err := os.Remove(filepath.Join(root, "XAUUSD", "15m.bin")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run(args, &out); err == nil || out.Len() != 0 {
		t.Fatalf("missing source: error=%v output=%q", err, out.String())
	}
}
