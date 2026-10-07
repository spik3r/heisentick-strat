package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
	"github.com/spik3r/heisentick-strat/report/adaptiveflagunit"
)

const adaptiveFlagUnitBridgeProjection = `{"schema":"adaptive-flag-unit-request-v1","scenario":"UNIT_POINT_VALUE_1","numericalPolicy":"BINARY64_ORDERED_V1","costPolicy":"RAW","dataSource":{"id":"invented-unit-bridge","sourceSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`

func TestAdaptiveFlagUnitBridgeOwnedRawAndSharedBuilder(t *testing.T) {
	for _, bundle := range []string{"initial", "tweaked", "snapshot_c"} {
		text, err := os.ReadFile("../../conformance/parse/family-adaptive-volume-flag-" + bundle + ".strat")
		if err != nil {
			t.Fatal(err)
		}
		for _, timeframe := range []string{"M30", "H1"} {
			for _, policy := range []string{"RAW", "RAZOR_PROXY_BASIC", "RAZOR_PROXY_HARSH_AGGREGATE"} {
				t.Run(bundle+"/"+timeframe+"/"+policy, func(t *testing.T) {
					metadata := strings.Replace(adaptiveFlagBridgeMetadata, "M30", timeframe, 1)
					source := strings.Replace(string(text), "timeframe M30", "timeframe "+timeframe, 1)
					projection := strings.Replace(adaptiveFlagUnitBridgeProjection, `"RAW"`, `"`+policy+`"`, 1)
					step := 1800000
					if timeframe == "H1" {
						step *= 2
					}
					data := adaptiveFlagBridgeData(step)
					got, err := runAdaptiveFlagUnitReportBTB1(metadata, projection, source, data)
					if err != nil {
						t.Fatal(err)
					}
					want, err := adaptiveflagunit.BuildRuntime(metadata, projection, source, data)
					if err != nil || !bytes.Equal(got, want) {
						t.Fatalf("shared builder differs: %v", err)
					}
					raw, err := adaptiveflag.BuildRuntime(metadata, source, data)
					if err != nil {
						t.Fatal(err)
					}
					var doc struct {
						Schema string `json:"schema"`
						RawSHA string `json:"rawEnvelopeSha256"`
					}
					sum := sha256.Sum256(raw)
					if err := json.Unmarshal(got, &doc); err != nil || doc.Schema != adaptiveflagunit.EnvelopeSchema || doc.RawSHA != hex.EncodeToString(sum[:]) {
						t.Fatalf("invalid owned envelope: %+v %v", doc, err)
					}
					segment := append(append([]byte(`"raw":`), raw...), []byte(`,"projection":`)...)
					if !bytes.Contains(got, segment) {
						t.Fatal("original raw bytes, including newline, were changed")
					}
				})
			}
		}
	}
}

func TestAdaptiveFlagUnitBridgeRejectsBeforeBars(t *testing.T) {
	source, err := os.ReadFile("../../conformance/parse/family-adaptive-volume-flag-initial.strat")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, raw, projection, source, code string }{
		{"raw", `{}`, adaptiveFlagUnitBridgeProjection, string(source), "raw_request_rejected"},
		{"projection", adaptiveFlagBridgeMetadata, `{}`, string(source), "projection_request_rejected"},
		{"duplicate", adaptiveFlagBridgeMetadata, strings.Replace(adaptiveFlagUnitBridgeProjection, `"schema":`, `"sch\u0065ma":"adaptive-flag-unit-request-v1","schema":`, 1), string(source), "projection_request_rejected"},
		{"G", adaptiveFlagBridgeMetadata, adaptiveFlagUnitBridgeProjection, strings.Replace(string(source), "bundle INITIAL", "bundle G1", 1), "raw_source_rejected"},
		{"CUSTOM", adaptiveFlagBridgeMetadata, adaptiveFlagUnitBridgeProjection, strings.Replace(string(source), "bundle INITIAL", "bundle CUSTOM", 1), "raw_source_rejected"},
		{"diagnostics", adaptiveFlagBridgeMetadata, adaptiveFlagUnitBridgeProjection, string(source) + "\nunknown", "raw_source_rejected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := runAdaptiveFlagUnitReportBTB1(test.raw, test.projection, test.source, nil)
			if err == nil || got != nil {
				t.Fatal("accepted invalid request or emitted partial output")
			}
			var failure adaptiveflagunit.ErrorEnvelope
			if json.Unmarshal([]byte(adaptiveflagunit.ErrorJSON(err)), &failure) != nil || failure.Error.Code != test.code {
				t.Fatalf("wrong refusal before bars: %s", adaptiveflagunit.ErrorJSON(err))
			}
			if _, err := runAdaptiveFlagUnitReportBTB1(adaptiveFlagBridgeMetadata, adaptiveFlagUnitBridgeProjection, string(source), adaptiveFlagBridgeData(1800000)); err != nil {
				t.Fatalf("recovery failed: %v", err)
			}
		})
	}
}
