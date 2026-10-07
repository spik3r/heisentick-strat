package masterstructural

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/master"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestRuntimeMetadataClosedAdmission(t *testing.T) {
	options := Options{TradeFromT: 40 * master.M30MS, TradeToT: 50 * master.M30MS}
	raw := requestJSON(options)
	if got, err := DecodeRuntimeRequest(raw); err != nil || got != options {
		t.Fatalf("%+v %v", got, err)
	}
	for _, token := range []string{"-0", "0.000", "1.000"} {
		got, err := DecodeRuntimeRequest(strings.Replace(raw, `"spread":0`, `"spread":`+token, 1))
		if err != nil || (token == "-0" && !math.Signbit(got.Spread)) {
			t.Fatalf("exact plain decimal %s: %+v %v", token, got, err)
		}
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatal(err)
	}
	bad := []string{"null", "[]", "{}", raw + "{}", raw + "x", raw[:len(raw)-1] + `,"spread":0}`, raw[:len(raw)-1] + `,"\u0073pread":0}`, strings.Repeat(" ", MaxMetadataBytes) + raw, "\xff" + raw}
	for key := range values {
		for _, kind := range []string{"missing", "null", "alias", "object", "array"} {
			copy := map[string]any{}
			for k, v := range values {
				copy[k] = v
			}
			switch kind {
			case "missing":
				delete(copy, key)
			case "null":
				copy[key] = nil
			case "alias":
				copy[strings.ToUpper(key)] = copy[key]
				delete(copy, key)
			case "object":
				copy[key] = map[string]any{}
			case "array":
				copy[key] = []any{}
			}
			b, _ := json.Marshal(copy)
			bad = append(bad, string(b))
		}
	}
	for _, token := range []string{"1.00000000000000001", "0.0000000000000000000000000000001", "1e-999", "1e0", "1e999999999999999999999"} {
		bad = append(bad, strings.Replace(raw, `"spread":0`, `"spread":`+token, 1))
	}
	for _, candidate := range bad {
		if _, err := DecodeRuntimeRequest(candidate); err == nil {
			t.Fatalf("admitted %q", candidate)
		}
	}
}

func TestRuntimeHeaderPreallocationBounds(t *testing.T) {
	data := marketdata.EncodeBTB1(inventedRows(50))
	if err := ValidateRuntimeHeader(data[:16], len(data)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"magic", "version", "columns", "zero", "overflow", "suffix", "short", "resource"} {
		header := append([]byte(nil), data[:16]...)
		length := len(data)
		switch kind {
		case "magic":
			header[0] = 0
		case "version":
			header[4] = 2
		case "columns":
			header[12] = 5
		case "zero":
			binary.LittleEndian.PutUint32(header[8:], 0)
		case "overflow":
			binary.LittleEndian.PutUint32(header[8:], math.MaxUint32)
		case "suffix":
			length++
		case "short":
			header = header[:15]
		case "resource":
			length = MaxInputBytes + 1
		}
		if err := ValidateRuntimeHeader(header, length); err == nil {
			t.Errorf("accepted %s", kind)
		}
	}
	header := append([]byte(nil), data[:16]...)
	binary.LittleEndian.PutUint32(header[8:], MaxRows)
	if err := ValidateRuntimeHeader(header, MaxInputBytes); err != nil {
		t.Fatalf("exact transport boundary: %v", err)
	}
}

func TestRuntimePreflightUsesObservedBucketsNotRowsDividedBySix(t *testing.T) {
	count := 20000
	s := marketdata.NewSeries(count)
	for i := range s.T {
		s.T[i] = float64(int64(i) * master.M30MS)
		s.O[i] = 100
		s.H[i] = 101
		s.L[i] = 99
		s.C[i] = 100
		s.V[i] = 1
	}
	// Each observed source row occupies its own M30 bucket. A naive rows/6
	// estimate would incorrectly admit this report; no engine run is needed.
	cfg, err := dsl.Parse(sourceFor(master.SourceMode))
	if err != nil {
		t.Fatal(err)
	}
	options := Options{TradeFromT: master.M30MS, TradeToT: int64(count) * master.M30MS}
	err = preflightRuntime(marketdata.EncodeBTB1(s), cfg.Config, options)
	if err == nil || !strings.Contains(err.Error(), "conservative output bound") {
		t.Fatalf("missing preallocation refusal: %v", err)
	}
	bound, err := runtimeOutputBound(cfg.Config, 100, 13, 60)
	if err != nil || bound >= MaxOutputBytes {
		t.Fatalf("small report bound %d: %v", bound, err)
	}
	// Source-controlled escaping is counted using exact config JSON bytes.
	cfg.Config["name"] = strings.Repeat("<", 1000)
	bigger, err := runtimeOutputBound(cfg.Config, 100, 13, 60)
	if err != nil || bigger < bound+5000 {
		t.Fatalf("config escape expansion omitted: %d %d %v", bound, bigger, err)
	}
}

func TestRuntimeRejectsNonfiniteAndNegativeVolumeBeforeRun(t *testing.T) {
	options := Options{TradeFromT: 40 * master.M30MS, TradeToT: 50 * master.M30MS}
	for _, future := range []bool{false, true} {
		for column := 0; column < 6; column++ {
			s := inventedRows(60)
			data := marketdata.EncodeBTB1(s)
			row := 1
			if future {
				row = 330
			}
			binary.LittleEndian.PutUint64(data[16+(column*s.Len()+row)*8:], math.Float64bits(math.NaN()))
			if raw, err := BuildPortableV1(requestJSON(options), sourceFor(master.SourceMode), data); err == nil || raw != nil {
				t.Fatalf("nonfinite col%d future%v", column, future)
			}
		}
	}
	s := inventedRows(50)
	s.V[1] = -1
	if raw, err := BuildPortableV1(requestJSON(options), sourceFor(master.SourceMode), marketdata.EncodeBTB1(s)); err == nil || raw != nil {
		t.Fatal("negative volume")
	}
}
