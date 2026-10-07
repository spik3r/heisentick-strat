package adaptiveflag

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func testSource(t *testing.T, bundle, tf string) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(testsupport.StratConformanceRoot(), "parse", "family-adaptive-volume-flag-"+strings.ToLower(bundle)+".strat"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(source), "timeframe M30", "timeframe "+tf)
}
func testMetadata(tf string, window *engine.AdaptiveFlagExecutionWindow) string {
	raw, _ := json.Marshal(RuntimeRequest{RuntimeRequestSchema, "BINARY64_ORDERED_V1", "XAUUSD", tf, window})
	return string(raw)
}
func testBars(tf string, short bool) []byte {
	step := float64(1800000)
	if tf == "H1" {
		step = 3600000
	}
	bars := []marketdata.Bar{}
	add := func(o, h, l, c, v float64) {
		bars = append(bars, marketdata.Bar{T: float64(len(bars)) * step, O: o, H: h, L: l, C: c, V: v})
	}
	if short {
		for i := 0; i < 20; i++ {
			add(120, 121, 119, 120, 100)
		}
		add(140, 150, 139, 140, 100)
		add(120, 130, 115, 120, 100)
		add(105, 110, 102, 105, 100)
		add(100, 105, 97, 100, 100)
		for len(bars) < 45 {
			add(100, 102, 99, 100, 100)
		}
		add(96, 100, 90, 92, 100)
		add(30, 31, 29, 30, 100)
	} else {
		for i := 0; i < 20; i++ {
			add(100, 101, 99, 100, 100)
		}
		for _, v := range [][5]float64{{100, 100, 97, 99, 100}, {100, 103, 99, 102, 100}, {103, 105, 102, 104, 100}, {105, 107, 104, 106, 100}, {107, 108, 106, 107.5, 100}, {107, 107.5, 106.5, 107, 100}, {107, 107.4, 106.4, 107, 100}, {107, 107.6, 106.2, 107.3, 200}, {107, 120, 102, 119, 100}, {135, 136, 134, 135, 100}} {
			add(v[0], v[1], v[2], v[3], v[4])
		}
	}
	return marketdata.EncodeBTB1(marketdata.SeriesFromBars(bars))
}
func TestRuntimeSixRoutesNonemptyBothSidesAndLegacyRawEquality(t *testing.T) {
	for _, bundle := range []string{"INITIAL", "TWEAKED", "SNAPSHOT_C"} {
		for _, tf := range []string{"M30", "H1"} {
			for _, short := range []bool{false, true} {
				t.Run(fmt.Sprint(bundle, tf, short), func(t *testing.T) {
					source, data := testSource(t, bundle, tf), testBars(tf, short)
					metadata := testMetadata(tf, nil)
					got, err := BuildRuntime(metadata, source, data)
					if err != nil {
						t.Fatal(err)
					}
					legacy, err := BuildLegacy([]byte(source), data, Options{})
					if err != nil {
						t.Fatal(err)
					}
					var runtime runtimeDocument
					var old legacyDocument
					if json.Unmarshal(got, &runtime) != nil || json.Unmarshal(legacy, &old) != nil {
						t.Fatal("invalid JSON")
					}
					a, _ := json.Marshal(runtime.Run)
					b, _ := json.Marshal(old.Run)
					if !bytes.Equal(a, b) {
						t.Fatal("raw native core drift")
					}
					if runtime.Schema != RuntimeEnvelopeSchema || !runtime.RawOnly || runtime.Economics != "unavailable-stage-a" || runtime.RequestSHA256 != hash([]byte(metadata)) || runtime.DSLSHA256 != hash([]byte(source)) || runtime.BTB1SHA256 != hash(data) || runtime.ConfigSHA256 != old.ConfigSHA256 {
						t.Fatal("identity/contract missing")
					}
					side := "long"
					if short {
						side = "short"
					}
					found := false
					for _, o := range runtime.Run.Orders {
						if o.Side == side && o.FillIdx != nil && o.ExitIdx != nil {
							found = true
						}
					}
					if !found {
						t.Fatalf("missing nonempty %s lifecycle: %+v", side, runtime.Run.Orders)
					}
				})
			}
		}
	}
}
func TestRuntimeStrictMetadataAndRawOnlyAdmission(t *testing.T) {
	base := testMetadata("M30", nil)
	for _, key := range []string{"equity", "startEquity", "statistics", "pnl", "returnPct", "costs", "risk", "quantity", "fee", "spread", "slippage", "fillOn", "researchAblation", "checkpoint", "resume", "sourceTf", "htfBars", "aggregation"} {
		if _, err := DecodeRuntimeRequest(strings.TrimSuffix(base, "}") + `,"` + key + `":0}`); err == nil {
			t.Fatal("accepted", key)
		}
	}
	bad := []string{base + " {}", strings.TrimSuffix(base, "}") + `,"schema":"adaptive-flag-runtime-request-v1"}`, strings.TrimSuffix(base, "}") + `,"\u0073chema":"adaptive-flag-runtime-request-v1"}`, strings.Replace(base, `"XAUUSD"`, `"\ud800"`, 1), strings.Replace(base, `"M30"`, `"30m"`, 1), strings.Repeat(" ", 4096) + base, string([]byte{0xff})}
	for _, s := range bad {
		if _, err := DecodeRuntimeRequest(s); err == nil {
			t.Fatal("accepted invalid metadata")
		}
	}
	w := testMetadata("M30", &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: 3600000})
	for _, s := range []string{strings.Replace(w, "3600000", "3.6e6", 1), strings.Replace(w, "3600000", "3600000.0", 1), strings.Replace(w, `"tradeFromMs":0`, `"tradeFromMs":-0`, 1), strings.Replace(w, `"tradeFromMs":0`, `"tradeFromMs":1`, 1), strings.Replace(w, "3600000", "0", 1)} {
		if _, err := DecodeRuntimeRequest(s); err == nil {
			t.Fatal("accepted invalid window", s)
		}
	}
	if _, err := DecodeRuntimeRequest(strings.Replace(base, `"schema"`, `"\u0073chema"`, 1)); err != nil {
		t.Fatal(err)
	}
}
func TestRuntimeStrictSourceBeforeInputAndProfiles(t *testing.T) {
	metadata, source := testMetadata("M30", nil), testSource(t, "SNAPSHOT_C", "M30")
	for _, bad := range []string{"dsl v7", strings.Replace(source, "bundle SNAPSHOT_C", "bundle CUSTOM", 1), strings.Replace(source, "useEMATrend true", "useEMATrend false", 1), source + "\nsetup { type: clock range breakout }", source + "\nresearch-ablation G1", strings.Repeat("x", MaxSourceBytes+1)} {
		raw, err := BuildRuntime(metadata, bad, nil)
		if err == nil || raw != nil {
			t.Fatal("admitted invalid source/default report")
		}
	}
	if _, err := PrepareRuntime(testMetadata("H1", nil), source); err == nil {
		t.Fatal("timeframe mismatch")
	}
	padded := source + strings.Repeat(" ", MaxSourceBytes-len(source))
	if _, err := PrepareRuntime(metadata, padded); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRuntime(metadata, padded+" "); err == nil {
		t.Fatal("oversize accepted")
	}
	if raw, err := new(PreparedRuntime).Build(nil); err == nil || raw != nil {
		t.Fatal("unprepared runtime accepted")
	}
}
func TestRuntimeWindowPreservesIgnoredSuffixAndTerminalPrefix(t *testing.T) {
	source := testSource(t, "INITIAL", "M30")
	data := testBars("M30", false)
	count := int(binary.LittleEndian.Uint32(data[8:12]))
	window := &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: 28 * 1800000}
	metadata := testMetadata("M30", window)
	run := func(d []byte) runtimeDocument {
		raw, err := BuildRuntime(metadata, source, d)
		if err != nil {
			t.Fatal(err)
		}
		var v runtimeDocument
		if err = json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	old := run(data)
	future := append([]byte(nil), data...)
	for col := 1; col < 6; col++ {
		binary.LittleEndian.PutUint64(future[16+(col*count+28)*8:], math.Float64bits(math.NaN()))
	}
	binary.LittleEndian.PutUint64(future[16+28*8:], math.Float64bits(math.Inf(1)))
	now := run(future)
	a, _ := json.Marshal(old.Run)
	b, _ := json.Marshal(now.Run)
	if !bytes.Equal(a, b) || old.BTB1SHA256 == now.BTB1SHA256 || now.Run.Terminal.Status != "pending" {
		t.Fatal("suffix/terminal semantics changed")
	}
	binary.LittleEndian.PutUint64(future[16+27*8:], math.Float64bits(math.NaN()))
	if _, err := BuildRuntime(metadata, source, future); err == nil {
		t.Fatal("invalid retained time accepted")
	}
	warmup := testMetadata("M30", &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 40 * 1800000, TradeToMS: 50 * 1800000})
	raw, err := BuildRuntime(warmup, source, data)
	if err != nil {
		t.Fatal(err)
	}
	var got runtimeDocument
	json.Unmarshal(raw, &got)
	if got.Run.EligibleTradeRows != 0 || got.Run.Terminal.Status != "flat" {
		t.Fatal("warmup order leakage")
	}
}
func TestRuntimeInputBoundsMagnitudeAndHeaders(t *testing.T) {
	source, meta := testSource(t, "INITIAL", "M30"), testMetadata("M30", nil)
	makeData := func(n int, v float64) []byte {
		rows := make([]marketdata.Bar, n)
		for i := range rows {
			rows[i] = marketdata.Bar{T: float64(i * 1800000), O: v, H: v, L: v, C: v, V: math.Copysign(0, -1)}
		}
		return marketdata.EncodeBTB1(marketdata.SeriesFromBars(rows))
	}
	for _, v := range []float64{MaxInputMagnitude, math.SmallestNonzeroFloat64} {
		if _, err := BuildRuntime(meta, source, makeData(1, v)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := BuildRuntime(meta, source, makeData(1, math.Nextafter(MaxInputMagnitude, math.Inf(1)))); err == nil {
		t.Fatal("magnitude cap ignored")
	}
	if _, err := BuildRuntime(meta, source, makeData(MaxRetainedRows+1, 100)); err == nil {
		t.Fatal("retained cap ignored")
	}
	all := makeData(MaxProvidedRows, 100)
	prefix := testMetadata("M30", &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: 1800000})
	if _, err := BuildRuntime(prefix, source, all); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{all[:15], append(append([]byte{}, all...), 0), makeData(MaxProvidedRows+1, 100)} {
		if _, err := BuildRuntime(prefix, source, data); err == nil {
			t.Fatal("envelope limit ignored")
		}
	}
	for _, word := range []struct {
		offset int
		value  uint32
	}{{0, 0}, {4, 2}, {8, 0}, {12, 5}, {12, 7}} {
		bad := append([]byte(nil), testBars("M30", false)...)
		binary.LittleEndian.PutUint32(bad[word.offset:], word.value)
		if _, err := BuildRuntime(meta, source, bad); err == nil {
			t.Fatal("header accepted")
		}
	}
}
func TestRuntimeBoundAndErrorEnvelope(t *testing.T) {
	bound, err := RuntimeOutputBound(4096)
	if err != nil || bound != 93195064 || bound >= MaxOutputBytes {
		t.Fatal(bound, err)
	}
	for _, n := range []uint64{0, 4097, math.MaxUint64} {
		if _, err := RuntimeOutputBound(n); err == nil {
			t.Fatal("invalid row bound")
		}
	}
	var got struct {
		Error struct{ Code, Phase, Message string }
	}
	json.Unmarshal([]byte(ErrorJSON(Rejection("input", strings.Repeat("🙂", 1000)))), &got)
	if got.Error.Code != "ADAPTIVE_RUNTIME_REJECTED" || got.Error.Phase != "input" || len(got.Error.Message) > 1024 {
		t.Fatal("bad error envelope")
	}
	if sha256.Sum256([]byte(testMetadata("M30", nil))) == [32]byte{} {
		t.Fatal("hash sanity")
	}
}
