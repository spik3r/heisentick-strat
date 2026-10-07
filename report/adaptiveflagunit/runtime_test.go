package adaptiveflagunit

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

func runtimeInputs(t *testing.T) (string, string, string, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testsupport", "testdata", "adaptive-flag-runtime-corpus-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Sources map[string]struct {
			Text string `json:"text"`
		}
		Assets map[string]struct {
			Payload string `json:"payload"`
		}
		Cases []struct {
			Source   string `json:"sourceSha256"`
			Metadata string `json:"metadataJSON"`
			Bars     string `json:"btb1Sha256"`
		}
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	c := corpus.Cases[0]
	bars, err := base64.StdEncoding.DecodeString(corpus.Assets[c.Bars].Payload)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(Request{RequestSchema, "UNIT_POINT_VALUE_1", "BINARY64_ORDERED_V1", "RAW", DataSource{"INVENTED_ADAPTIVE_FLAG_RUNTIME_CORPUS_V1", "af392754f48672cfe31d7161454dfc35ee73977e4c38f0a72b21c15be6fb2f97"}})
	return c.Metadata, string(request), corpus.Sources[c.Source].Text, bars
}
func runtimeBars(times []float64) []byte {
	n := len(times)
	b := make([]byte, 16+n*48)
	binary.LittleEndian.PutUint32(b, 0x31425442)
	binary.LittleEndian.PutUint32(b[4:], 1)
	binary.LittleEndian.PutUint32(b[8:], uint32(n))
	binary.LittleEndian.PutUint32(b[12:], 6)
	for i, stamp := range times {
		for col, v := range []float64{stamp, 100, 101, 99, 100, 1} {
			binary.LittleEndian.PutUint64(b[16+(col*n+i)*8:], math.Float64bits(v))
		}
	}
	return b
}
func runtimeMetadata(t *testing.T, start, end *int64) string {
	t.Helper()
	var window any
	if start != nil {
		window = map[string]int64{"tradeFromMs": *start, "tradeToMs": *end}
	}
	b, err := json.Marshal(map[string]any{"schema": adaptiveflag.RuntimeRequestSchema, "numericalPolicy": "BINARY64_ORDERED_V1", "symbol": "XAUUSD", "timeframe": "M30", "executionWindow": window})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestUnitRuntimeSingleOwnedRawAndExactComposition(t *testing.T) {
	rawMeta, unitMeta, source, bars := runtimeInputs(t)
	raw, err := adaptiveflag.BuildRuntime(rawMeta, source, bars)
	if err != nil {
		t.Fatal(err)
	}
	output, err := BuildRuntime(rawMeta, unitMeta, source, bars)
	if err != nil {
		t.Fatal(ErrorJSON(err))
	}
	marker := []byte(`,"raw":`)
	offset := bytes.Index(output, marker) + len(marker)
	if offset < len(marker) || !bytes.Equal(output[offset:offset+len(raw)], raw) || !bytes.HasPrefix(output[offset+len(raw):], []byte(`,"projection":`)) {
		t.Fatal("raw byte segment changed")
	}
	var envelope Envelope
	if err = json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Schema != EnvelopeSchema || envelope.ProjectionRequestSHA256 != rawBytesSHA([]byte(unitMeta)) || envelope.RawEnvelopeSHA256 != rawBytesSHA(raw) || envelope.Projection.Manifest.RawEnvelopeSHA256 != rawBytesSHA(raw) {
		t.Fatal("hash domains")
	}
	if !json.Valid(output) || output[len(output)-1] != '\n' || raw[len(raw)-1] != '\n' {
		t.Fatal("complete newline documents")
	}
	// A whitespace-only request edit has its own exact identity without changing economics.
	changed, err := BuildRuntime(rawMeta, " \n"+unitMeta+" \t", source, bars)
	if err != nil {
		t.Fatal(err)
	}
	var other Envelope
	if err = json.Unmarshal(changed, &other); err != nil {
		t.Fatal(err)
	}
	if envelope.ProjectionRequestSHA256 == other.ProjectionRequestSHA256 || !reflect.DeepEqual(envelope.Projection, other.Projection) || !bytes.Equal(envelope.Raw, other.Raw) {
		t.Fatal("request bytes/projection domain")
	}
}

func TestUnitRuntimeBoundsBeforeRawExecution(t *testing.T) {
	_, unitMeta, source, _ := runtimeInputs(t)
	for _, n := range []int{1024, 1025} {
		times := make([]float64, n)
		for i := range times {
			times[i] = float64(i * 1800000)
		}
		start, end := int64(n*1800000), int64((n+1)*1800000)
		p, err := PrepareRuntime(runtimeMetadata(t, &start, &end), unitMeta, source)
		if err != nil {
			t.Fatal(err)
		}
		data := runtimeBars(times)
		got, days, limit, err := p.preflight(data)
		if n == 1025 {
			if coreCode(t, err) != "resource_limit" {
				t.Fatal(err)
			}
			continue
		}
		if err != nil || got != 1024 || days != 1 || limit > MaxOutputBytes {
			t.Fatalf("preflight %d %d %d %v", got, days, limit, err)
		}
		output, err := p.Build(data)
		if err != nil {
			t.Fatal(ErrorJSON(err))
		}
		var doc Envelope
		json.Unmarshal(output, &doc)
		if len(doc.Projection.Marks) != 0 || len(doc.Projection.Daily) != 1 || doc.Projection.Manifest.WarmupRowsExcluded != 1024 {
			t.Fatal("all supplied warmup must be counted")
		}
	}
	// Excess retained count wins before poisoned OHLC reaches raw execution.
	times := make([]float64, 1025)
	for i := range times {
		times[i] = float64(i * 1800000)
	}
	data := runtimeBars(times)
	binary.LittleEndian.PutUint64(data[16+len(times)*8:], math.Float64bits(math.NaN()))
	_, err := BuildRuntime(runtimeMetadata(t, nil, nil), unitMeta, source, data)
	if coreCode(t, err) != "resource_limit" {
		t.Fatal(err)
	}
}

func TestUnitRuntimeCalendarAndYearBoundaries(t *testing.T) {
	_, unitMeta, source, _ := runtimeInputs(t)
	for _, days := range []int64{366, 367} {
		start, end := int64(0), days*unitDayMS
		p, err := PrepareRuntime(runtimeMetadata(t, &start, &end), unitMeta, source)
		if days == 367 {
			if coreCode(t, err) != "resource_limit" {
				t.Fatal(err)
			}
			continue
		}
		output, err := p.Build(runtimeBars([]float64{0}))
		if err != nil {
			t.Fatal(ErrorJSON(err))
		}
		var doc Envelope
		json.Unmarshal(output, &doc)
		if len(doc.Projection.Daily) != 366 {
			t.Fatal("calendar day labels")
		}
	}
	for _, times := range [][]float64{{0, float64(365 * unitDayMS)}, {0, float64(366 * unitDayMS)}} {
		p, err := PrepareRuntime(runtimeMetadata(t, nil, nil), unitMeta, source)
		if err != nil {
			t.Fatal(err)
		}
		_, days, _, err := p.preflight(runtimeBars(times))
		if times[1] == float64(366*unitDayMS) {
			if coreCode(t, err) != "resource_limit" {
				t.Fatal(err)
			}
		} else if err != nil || days != 366 {
			t.Fatal(days, err)
		}
	}
	p, err := PrepareRuntime(runtimeMetadata(t, nil, nil), unitMeta, source)
	if err != nil {
		t.Fatal(err)
	}
	output, err := p.Build(runtimeBars([]float64{float64(MaxCalendarEndpointMS - 1800000)}))
	if err != nil {
		t.Fatal(ErrorJSON(err))
	}
	var doc Envelope
	json.Unmarshal(output, &doc)
	if len(doc.Projection.Daily) != 1 || doc.Projection.Daily[0].DateUTC != "9999-12-31" {
		t.Fatal("exclusive year endpoint")
	}
	_, err = p.Build(runtimeBars([]float64{float64(MaxCalendarEndpointMS)}))
	if coreCode(t, err) != "projection_domain_rejected" {
		t.Fatal(err)
	}
}

func TestUnitRuntimeIgnoredSuffixOrderAndRawValueAdmission(t *testing.T) {
	_, unitMeta, source, _ := runtimeInputs(t)
	start, end := int64(0), int64(1800000)
	meta := runtimeMetadata(t, &start, &end)
	p, err := PrepareRuntime(meta, unitMeta, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, boundary := range []float64{math.Inf(1), float64(end) + 0.25, 1e100} {
		data := runtimeBars([]float64{0, boundary, math.NaN()})
		n := 3
		for row := 1; row < n; row++ {
			for col := 1; col < 6; col++ {
				binary.LittleEndian.PutUint64(data[16+(col*n+row)*8:], math.Float64bits(math.NaN()))
			}
		}
		output, err := p.Build(data)
		if err != nil {
			t.Fatal(ErrorJSON(err))
		}
		raw, err := adaptiveflag.BuildRuntime(meta, source, data)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(output, append(append([]byte(`,"raw":`), raw...), []byte(`,"projection":`)...)) {
			t.Fatal("ignored suffix changed raw")
		}
	}
	for _, stamp := range []float64{math.NaN(), math.Inf(-1), -1, 0.25} {
		_, err = p.Build(runtimeBars([]float64{stamp}))
		if coreCode(t, err) != "raw_input_rejected" {
			t.Fatal(err)
		}
	}
	data := runtimeBars([]float64{0})
	binary.LittleEndian.PutUint64(data[24:], math.Float64bits(math.NaN()))
	_, err = p.Build(data)
	if coreCode(t, err) != "raw_input_rejected" {
		t.Fatal(ErrorJSON(err))
	}
	good, err := p.Build(runtimeBars([]float64{0}))
	if err != nil || !json.Valid(good) {
		t.Fatal("recovery", err)
	}
}

func TestUnitRuntimePreparationAndOwnership(t *testing.T) {
	rawMeta, unitMeta, source, bars := runtimeInputs(t)
	p, err := PrepareRuntime(rawMeta, unitMeta, source)
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), bars...)
	a, err := p.Build(bars)
	if err != nil {
		t.Fatal(err)
	}
	saved := append([]byte(nil), a...)
	for i := range bars {
		bars[i] = 0
	}
	if !bytes.Equal(a, saved) {
		t.Fatal("input alias")
	}
	a[0] = '!'
	b, err := p.Build(original)
	if err != nil || !bytes.Equal(b, saved) {
		t.Fatal("output/prepared alias", err)
	}
	var empty PreparedRuntime
	_, err = empty.Build(original)
	if coreCode(t, err) != "projection_request_rejected" {
		t.Fatal(err)
	}
	_, err = BuildRuntime(rawMeta, strings.Replace(unitMeta, `"RAW"`, `"CUSTOM"`, 1), source, nil)
	if coreCode(t, err) != "projection_request_rejected" {
		t.Fatal("metadata before header", err)
	}
	_, err = BuildRuntime(rawMeta, unitMeta, "setup { x x x }", nil)
	if coreCode(t, err) != "raw_source_rejected" {
		t.Fatal("source before header", err)
	}
	for _, phase := range []string{"request", "resource"} {
		err = ProjectionMetadataTransportError(adaptiveflag.Rejection(phase, "transport"))
		want := "projection_request_rejected"
		if phase == "resource" {
			want = "resource_limit"
		}
		if coreCode(t, err) != want {
			t.Fatal(err)
		}
	}
}
