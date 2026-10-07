package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

const adaptiveFlagBridgeMetadata = `{"schema":"adaptive-flag-runtime-request-v1","numericalPolicy":"BINARY64_ORDERED_V1","symbol":"XAUUSD","timeframe":"M30","executionWindow":null}`

func adaptiveFlagBridgeData(step int) []byte {
	series := marketdata.NewSeries(4)
	for i := range series.T {
		series.T[i] = float64(i * step)
		series.O[i], series.H[i], series.L[i], series.C[i], series.V[i] = 100, 101, 99, 100, 1
	}
	return marketdata.EncodeBTB1(series)
}

func TestDedicatedAdaptiveFlagBridgeUsesSharedReport(t *testing.T) {
	for _, bundle := range []string{"initial", "tweaked", "snapshot_c"} {
		source, err := os.ReadFile("../../conformance/parse/family-adaptive-volume-flag-" + bundle + ".strat")
		if err != nil {
			t.Fatal(err)
		}
		for _, timeframe := range []string{"M30", "H1"} {
			t.Run(bundle+"/"+timeframe, func(t *testing.T) {
				metadata := strings.Replace(adaptiveFlagBridgeMetadata, "M30", timeframe, 1)
				text := strings.Replace(string(source), "timeframe M30", "timeframe "+timeframe, 1)
				step := 1800000
				if timeframe == "H1" {
					step *= 2
				}
				data := adaptiveFlagBridgeData(step)
				got, err := runAdaptiveFlagReportBTB1(metadata, text, data)
				if err != nil {
					t.Fatal(err)
				}
				want, err := adaptiveflag.BuildRuntime(metadata, text, data)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("shared report byte mismatch: %v", err)
				}
				var doc struct {
					Schema    string `json:"schema"`
					RawOnly   bool   `json:"rawOnly"`
					Economics string `json:"economics"`
				}
				if err := json.Unmarshal(got, &doc); err != nil || doc.Schema != "strat-adaptive-volume-flag-runtime-v1" || !doc.RawOnly || doc.Economics != "unavailable-stage-a" {
					t.Fatalf("invalid dedicated raw envelope: %+v; %v", doc, err)
				}
			})
		}
	}
}

func TestDedicatedAdaptiveFlagBridgeRejectsWithoutFallback(t *testing.T) {
	for _, source := range []string{
		"", "dsl v7\nstrategy \"Legacy\"\nsetup {type: flag continuation}",
		adaptiveFlagUnsupportedSource + "\nunknown",
		strings.Replace(adaptiveFlagUnsupportedSource, "bundle INITIAL", "bundle CUSTOM", 1),
		strings.Replace(adaptiveFlagUnsupportedSource, "targetR 2.5", "targetR 2.6", 1),
		strings.Replace(adaptiveFlagUnsupportedSource, "timeframe M30", "timeframe H1", 1),
		adaptiveFlagUnsupportedSource + strings.Repeat(" ", 4097),
	} {
		got, err := runAdaptiveFlagReportBTB1(adaptiveFlagBridgeMetadata, source, adaptiveFlagBridgeData(1800000))
		if err == nil || got != nil {
			t.Fatal("dedicated route accepted unsupported source or emitted a partial report")
		}
	}
	for _, key := range []string{"equity", "spread", "researchAblation", "aggregation"} {
		metadata := strings.TrimSuffix(adaptiveFlagBridgeMetadata, "}") + `,"` + key + `":0}`
		if got, err := runAdaptiveFlagReportBTB1(metadata, adaptiveFlagUnsupportedSource, adaptiveFlagBridgeData(1800000)); err == nil || got != nil {
			t.Fatalf("accepted forbidden request key %s", key)
		}
	}
}
