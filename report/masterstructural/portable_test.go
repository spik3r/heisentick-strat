package masterstructural

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine/master"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestPortableReportNumericalProvenance(t *testing.T) {
	for _, mode := range []string{master.SourceMode, master.ProtectedMode} {
		options := Options{TradeFromT: 40 * master.M30MS, TradeToT: 500 * master.M30MS}
		source := sourceFor(mode)
		data := marketdata.EncodeBTB1(inventedRows(500))
		legacy, err := Build([]byte(source), data, options)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := BuildPortableV1(requestJSON(options), source, data)
		if err != nil {
			t.Fatal(err)
		}
		var doc PortableDocument
		if err = json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if doc.Schema != "strat-master-structural-portable-cli-v1" || doc.Arithmetic.Contract != master.PortableArithmeticContract || doc.Arithmetic.ImplicitContraction || doc.Arithmetic.TickQuantization || doc.Run.ArithmeticContract != master.PortableArithmeticContract || doc.Run.Schema != "strat-master-structural-portable-report-v1" || len(doc.Run.Trades) == 0 {
			t.Fatal("missing portable provenance/execution")
		}
		again, err := Build([]byte(source), data, options)
		if err != nil || !bytes.Equal(legacy, again) {
			t.Fatal("portable invocation altered legacy", err)
		}
	}
}

func TestPortableEnvelopeFixedAllowance(t *testing.T) {
	doc := portableDocument(nil, nil, nil, nil, master.Result{})
	// Subtract the complete encoded run/config including their indentation. All
	// remaining fields, separators and newline must fit the preflight allowance.
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	run, _ := json.MarshalIndent(doc.Run, "  ", "  ")
	if uint64(len(raw)+1-len(run)) > portableEnvelopeAllowance {
		t.Fatal("portable fixed envelope allowance drift")
	}
}

func TestPortableSpreadOriginalTokenAdmission(t *testing.T) {
	for _, token := range []string{"0", "-0", "0.000", "-0.000", "1", "1.000"} {
		v, err := DecodePortableSpread(token)
		if err != nil {
			t.Fatalf("%s: %v", token, err)
		}
		if strings.HasPrefix(token, "-") && !math.Signbit(v) {
			t.Fatal("lost negative zero")
		}
	}
	for _, token := range []string{"", "+0", "00", "01", " 0", "0 ", "0/1", ".0", "1e0", "1e-999", "-1e-999", "0.99999999999999999", "1.00000000000000001", "0.0000000000000000001", "NaN", "Inf", "null", "true", "0,1", "0{}", strings.Repeat("0", 65)} {
		if _, err := DecodePortableSpread(token); err == nil {
			t.Fatalf("admitted %q", token)
		}
	}
}

func TestPortableReportRefusesLegacySchemaAndArithmeticFailure(t *testing.T) {
	options := Options{TradeFromT: 40 * master.M30MS, TradeToT: 50 * master.M30MS}
	source := sourceFor(master.SourceMode)
	data := marketdata.EncodeBTB1(inventedRows(50))
	meta := requestJSON(options)
	for _, raw := range []string{strings.Replace(meta, RuntimeSchema, "master-structural-runtime-request-v1", 1), strings.Replace(meta, master.PortableArithmeticContract, "future-v2", 1)} {
		if out, err := BuildPortableV1(raw, source, data); err == nil || out != nil {
			t.Fatal("legacy/unknown contract admitted")
		}
	}
	s := inventedRows(50)
	for i := range s.T {
		s.O[i] = math.MaxFloat64
		s.H[i] = math.MaxFloat64
		s.L[i] = 1
		s.C[i] = math.MaxFloat64
	}
	if out, err := BuildPortableV1(meta, source, marketdata.EncodeBTB1(s)); err == nil || out != nil {
		t.Fatal("partial success on arithmetic failure")
	}
	if out, err := BuildPortableV1(meta, source+"#\xff", data); err == nil || out != nil {
		t.Fatal("invalid UTF8 admitted")
	}
}

func TestPortableReportHalfProductWarmupRefusal(t *testing.T) {
	s := inventedRows(4)
	for i := range s.T {
		s.O[i] = 1e308
		s.H[i] = 1e308
		s.L[i] = 1e308
		s.C[i] = 1e308
	}
	options := Options{TradeFromT: master.M30MS, TradeToT: 4 * master.M30MS}
	raw, err := BuildPortableV1(requestJSON(options), sourceFor(master.SourceMode), marketdata.EncodeBTB1(s))
	if err == nil || raw != nil || !strings.Contains(err.Error(), "nonfinite multiply") {
		t.Fatalf("bounded report hid warmup overflow: %v", err)
	}
}
