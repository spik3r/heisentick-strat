package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report/masterstructural"
)

func TestDedicatedMasterBridgeUsesSharedReport(t *testing.T) {
	series := marketdata.NewSeries(300)
	for i := range series.T {
		series.T[i] = float64(i * 300000)
		series.O[i] = 100
		series.H[i] = 101
		series.L[i] = 99
		series.C[i] = 100
		series.V[i] = 1
	}
	data := marketdata.EncodeBTB1(series)
	metadata := `{"schema":"master-structural-portable-runtime-request-v1","arithmeticContract":"master-binary64-separated-v1","warmupFromT":0,"tradeFromT":72000000,"tradeToT":90000000,"spread":0}`
	got, err := runMasterPortableReportBTB1(metadata, masterUnsupportedSource, data)
	if err != nil {
		t.Fatal(err)
	}
	want, err := masterstructural.BuildPortableV1(metadata, masterUnsupportedSource, data)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("shared report byte mismatch: %v", err)
	}
	var document map[string]any
	if err = json.Unmarshal(got, &document); err != nil {
		t.Fatal(err)
	}
	if document["schema"] != "strat-master-structural-portable-cli-v1" {
		t.Fatal("changed schema")
	}
	for _, source := range []string{"dsl v7\nstrategy \"Legacy\"\nsetup {type: flag continuation}", masterUnsupportedSource + "\nunknown"} {
		if raw, err := runMasterPortableReportBTB1(metadata, source, data); err == nil || raw != nil {
			t.Fatal("dedicated admission fallback")
		}
	}
}
