//go:build !js && !wasm

package adaptiveflagunit

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

func TestHistoryFullFrozenInventedProjectionParity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testsupport", "testdata", "adaptive-flag-runtime-corpus-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if rawBytesSHA(data) != "af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97" {
		t.Fatal("corpus changed")
	}
	var corpus struct {
		Sources map[string]struct {
			Text string `json:"text"`
		}
		Assets map[string]struct {
			Payload string `json:"payload"`
		}
		Cases []struct {
			ID       string `json:"id"`
			Source   string `json:"sourceSha256"`
			Metadata string `json:"metadataJSON"`
			Bars     string `json:"btb1Sha256"`
		}
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, c := range corpus.Cases {
		for _, cost := range []string{"RAW", "RAZOR_PROXY_BASIC", "RAZOR_PROXY_HARSH_AGGREGATE"} {
			t.Run(c.ID+"/"+cost, func(t *testing.T) {
				bars, err := base64.StdEncoding.DecodeString(corpus.Assets[c.Bars].Payload)
				if err != nil {
					t.Fatal(err)
				}
				request := unitRequest(cost)
				meta, _ := json.Marshal(request)
				source := corpus.Sources[c.Source].Text
				old, err := BuildRuntime(c.Metadata, string(meta), source, bars)
				if err != nil {
					t.Fatal(ErrorJSON(err))
				}
				fresh, err := BuildHistory(adaptiveflag.HistoryProfile, c.Metadata, string(meta), source, bars)
				if err != nil {
					t.Fatal(ErrorJSON(err))
				}
				var portable Envelope
				var history HistoryEnvelope
				if err = json.Unmarshal(old, &portable); err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(fresh, &history); err != nil {
					t.Fatal(err)
				}
				if history.Schema != HistoryEnvelopeSchema || history.Profile != adaptiveflag.HistoryProfile || history.RawEnvelopeSchema != adaptiveflag.HistoryRawSchema || history.RawEnvelopeSHA256 == portable.RawEnvelopeSHA256 || history.RawEnvelopeSHA256 != history.Projection.Manifest.RawEnvelopeSHA256 {
					t.Fatal("native hash domain")
				}
				if history.ProjectionRequestSHA256 != rawBytesSHA(meta) || history.RawRequestSHA256 != rawBytesSHA([]byte(c.Metadata)) {
					t.Fatal("exact request identity")
				}
				// Sole declared projection difference: a separately versioned compact raw
				// envelope is hashed, rather than portable Stage A indented raw bytes.
				portable.Projection.Manifest.RawEnvelopeSHA256 = ""
				history.Projection.Manifest.RawEnvelopeSHA256 = ""
				a, _ := json.Marshal(portable.Projection)
				b, _ := json.Marshal(history.Projection)
				if !bytes.Equal(a, b) {
					t.Fatal("projection economics, ordering or identity drift")
				}
				var fields map[string]json.RawMessage
				json.Unmarshal(fresh, &fields)
				if _, ok := fields["raw"]; ok {
					t.Fatal("duplicated full raw envelope")
				}
				if fresh[len(fresh)-1] != '\n' {
					t.Fatal("incomplete document")
				}
				count++
			})
		}
	}
	if count != 288 {
		t.Fatal("expected288 full comparisons", count)
	}
}

func TestHistoryLargerWarmupAndPortableRefusalPreserved(t *testing.T) {
	_, unitMeta, source, _ := runtimeInputs(t)
	times := make([]float64, 2500)
	for i := range times {
		times[i] = float64(i * 1800000)
	}
	start, end := int64(2000*1800000), int64(2500*1800000)
	metadata := runtimeMetadata(t, &start, &end)
	bars := runtimeBars(times)
	if _, err := BuildRuntime(metadata, unitMeta, source, bars); coreCode(t, err) != "resource_limit" {
		t.Fatal(err)
	}
	data, err := BuildHistory(adaptiveflag.HistoryProfile, metadata, unitMeta, source, bars)
	if err != nil {
		t.Fatal(ErrorJSON(err))
	}
	var doc HistoryEnvelope
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Projection.Manifest.WarmupRowsExcluded != 2000 || len(doc.Projection.Marks) != 500 || doc.Projection.Terminal.NativeState.RowIdx != 2499 {
		t.Fatal("warmup/trading state was reset or trimmed")
	}
	for _, days := range []int64{2200, 2201} {
		s, e := int64(0), days*86400000
		_, err := PrepareHistory(adaptiveflag.HistoryProfile, runtimeMetadata(t, &s, &e), unitMeta, source)
		if (err == nil) != (days == 2200) {
			t.Fatal(days, err)
		}
	}
	for _, profile := range []string{"", "BASIC", "adaptive-history-v2"} {
		if _, err := PrepareHistory(profile, metadata, unitMeta, source); err == nil {
			t.Fatal("implicit/unknown profile")
		}
	}
}

func TestHistoryBoundsBeforeSerialization(t *testing.T) {
	type extreme struct {
		Text     string  `json:"text"`
		Integer  int64   `json:"integer"`
		Number   float64 `json:"number"`
		Optional *string `json:"optional,omitempty"`
	}
	for _, v := range []float64{math.MaxFloat64, -math.MaxFloat64, math.SmallestNonzeroFloat64, math.Copysign(0, -1)} {
		s := strings.Repeat("\x00\x1f\"\\<>&\u2028\u2029", 128)
		value := extreme{s, math.MinInt64, v, &s}
		bound, err := historyJSONBound(value)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if uint64(len(encoded)) > bound {
			t.Fatal("escaping/numeric bound underestimated")
		}
	}
	if _, err := historyJSONBound(extreme{Text: strings.Repeat("\x00", HistoryMaxOutputBytes/6+1)}); err == nil {
		t.Fatal("oversize escaped value admitted before marshal")
	}
	if _, err := historyJSONBound(json.RawMessage(`{"unbounded":true}`)); err == nil {
		t.Fatal("custom marshaler admitted")
	}
	if _, err := historyJSONBound(map[string]any{"bad": math.Inf(1)}); err == nil {
		t.Fatal("nonfinite token admitted")
	}
	run := engine.AdaptiveFlagResult{Snapshots: make([]engine.AdaptiveFlagSnapshot, 70000), Orders: make([]engine.AdaptiveFlagOrder, 70000)}
	if _, err := historyProjectionBound(&run, validationContext{Days: 2200, Midnights: 30000, Gaps: 70000}); err == nil {
		t.Fatal("dense theoretical result allocated beyond256MiB")
	}
}

func TestHistoryExposureCapsAreGlobalAndOldCapsRemain(t *testing.T) {
	limits := historyLimits()
	ctx := validationContext{Midnights: HistoryMaxExposureMidnights}
	entry := engine.AdaptiveFlagEvent{Time: engine.AdaptiveFlagEventTime{LowerMS: 0, UpperMS: 0, UpperInclusive: true}}
	end := engine.AdaptiveFlagEvent{Time: engine.AdaptiveFlagEventTime{LowerMS: 86400000, UpperMS: 86400000, UpperInclusive: true}}
	if err := countNativeExposureWithLimits(&ctx, entry, end, nil, limits); err == nil {
		t.Fatal("global midpoint allowance bypass")
	}
	ctx = validationContext{Midnights: MaxExposureMidnights}
	if err := countNativeExposure(&ctx, entry, end, nil); err == nil {
		t.Fatal("portable midnight limit changed")
	}
	ctx = validationContext{Gaps: HistoryMaxExposureGaps}
	rows := []engine.AdaptiveFlagSnapshot{{AdaptiveFlagBar: engine.AdaptiveFlagBar{CloseT: 1800000}}, {AdaptiveFlagBar: engine.AdaptiveFlagBar{OpenT: 3600000}}}
	end.RowIdx = 1
	end.Time = entry.Time
	if err := countNativeExposureWithLimits(&ctx, entry, end, rows, limits); err == nil {
		t.Fatal("global gap allowance bypass")
	}
	raw := handOwned(t, "hand-long-closed")
	a, err := projectCore(raw, unitRequest("RAW"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := projectWithLimits(raw, unitRequest("RAW"), historyLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("resource profile altered hand accounting")
	}
}
