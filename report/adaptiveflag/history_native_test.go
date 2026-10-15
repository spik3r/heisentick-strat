//go:build !js && !wasm

package adaptiveflag

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestHistoryRawMatchesPortableWithoutRoundTrip(t *testing.T) {
	for _, bundle := range []string{"INITIAL", "TWEAKED", "SNAPSHOT_C"} {
		for _, tf := range []string{"M30", "H1"} {
			for _, short := range []bool{false, true} {
				source, data := testSource(t, bundle, tf), testBars(tf, short)
				metadata := testMetadata(tf, nil)
				p, err := PrepareHistory(HistoryProfile, metadata, source)
				if err != nil {
					t.Fatal(err)
				}
				got, err := p.Build(data)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := BuildRuntime(metadata, source, data)
				if err != nil {
					t.Fatal(err)
				}
				var old runtimeDocument
				if err = json.Unmarshal(raw, &old); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got.Run, old.Run) || got.DSLSHA256 != old.DSLSHA256 || got.ConfigSHA256 != old.ConfigSHA256 || got.BTB1SHA256 != old.BTB1SHA256 || got.Schema == old.Schema {
					t.Fatal("raw model drift or schema conflation")
				}
				if len(got.Run.Orders) == 0 {
					t.Fatal("empty side control")
				}
			}
		}
	}
}

func TestHistoryRawAdmissionAndContinuousWarmup(t *testing.T) {
	source := testSource(t, "INITIAL", "M30")
	bars := make([]marketdata.Bar, 2500)
	for i := range bars {
		price := 100 + float64(i%37)/8
		bars[i] = marketdata.Bar{T: float64(i * 1800000), O: price, H: price + 1, L: price - 1, C: price, V: 100}
	}
	data := marketdata.EncodeBTB1(marketdata.SeriesFromBars(bars))
	window := &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 2000 * 1800000, TradeToMS: 2500 * 1800000}
	p, err := PrepareHistory(HistoryProfile, testMetadata("M30", window), source)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Build(data)
	if err != nil {
		t.Fatal(err)
	}
	original, err := BuildLegacy([]byte(source), data, Options{Window: window})
	if err != nil {
		t.Fatal(err)
	}
	var want legacyDocument
	if err = json.Unmarshal(original, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Run, want.Run) || got.Run.PreTradeRows != 2000 || len(got.Run.Snapshots) != 2500 {
		t.Fatal("continuous state or warmup changed")
	}
	trimmed := marketdata.EncodeBTB1(marketdata.SeriesFromBars(bars[1500:]))
	alternate, err := p.Build(trimmed)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(got.Run.Snapshots[2000], alternate.Run.Snapshots[500]) {
		t.Fatal("warmup/reset control ineffective")
	}
	if _, err = PrepareHistory("", testMetadata("M30", nil), source); err == nil {
		t.Fatal("implicit profile")
	}
	if _, err = BuildRuntime(testMetadata("M30", nil), source, data); err != nil {
		t.Fatal("raw portable4096 cap should remain distinct")
	}
	// Cutoff precedes validation exactly as in the existing route.
	altered := append([]byte(nil), data...)
	binary.LittleEndian.PutUint64(altered[16+2499*8:], math.Float64bits(math.Inf(1)))
	earlier := &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: 2499 * 1800000}
	p, err = PrepareHistory(HistoryProfile, testMetadata("M30", earlier), source)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := p.Preflight(altered); err != nil || n != 2499 {
		t.Fatalf("ignored suffix: %d %v", n, err)
	}
}

func TestHistoryRawRowsCalendarAndMalformedBounds(t *testing.T) {
	source := testSource(t, "INITIAL", "M30")
	p, err := PrepareHistory(HistoryProfile, testMetadata("M30", nil), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{70000, 70001} {
		data := make([]byte, 16+n*48)
		binary.LittleEndian.PutUint32(data, 0x31425442)
		binary.LittleEndian.PutUint32(data[4:], 1)
		binary.LittleEndian.PutUint32(data[8:], uint32(n))
		binary.LittleEndian.PutUint32(data[12:], 6)
		for i := 0; i < n; i++ {
			for col, v := range []float64{float64(i * 1800000), 100, 101, 99, 100, 1} {
				binary.LittleEndian.PutUint64(data[16+(col*n+i)*8:], math.Float64bits(v))
			}
		}
		count, err := p.Preflight(data)
		if n == 70000 && (err != nil || count != n) {
			t.Fatal(count, err)
		}
		if n == 70001 && err == nil {
			t.Fatal("70001 rows admitted")
		}
	}
	for _, days := range []int64{2200, 2201} {
		_, err := PrepareHistory(HistoryProfile, testMetadata("M30", &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: days * 86400000}), source)
		if (err == nil) != (days == 2200) {
			t.Fatal(days, err)
		}
	}
	valid := testBars("M30", false)
	for _, bad := range [][]byte{nil, valid[:15], valid[:len(valid)-1], append(append([]byte{}, valid...), 0)} {
		if _, err := p.Preflight(bad); err == nil {
			t.Fatal("malformed bytes admitted")
		}
	}
	for _, offset := range []int{0, 4, 12} {
		bad := append([]byte(nil), valid...)
		binary.LittleEndian.PutUint32(bad[offset:], 99)
		if _, err := p.Preflight(bad); err == nil {
			t.Fatal("bad header admitted")
		}
	}
}

func TestHistoryLongH1FixedProfileAdmission(t *testing.T) {
	profile := HistoryLongH1Profile
	for _, tf := range []string{"M30", "H1"} {
		_, err := PrepareHistory(profile, testMetadata(tf, nil), testSource(t, "INITIAL", tf))
		if (tf == "H1") != (err == nil) {
			t.Fatalf("long profile timeframe %s: %v", tf, err)
		}
	}
	source := testSource(t, "INITIAL", "H1")
	for _, days := range []int64{9000, 9001} {
		window := &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: days * 86400000}
		_, err := PrepareHistory(profile, testMetadata("H1", window), source)
		if (days == 9000) != (err == nil) {
			t.Fatalf("long calendar days %d: %v", days, err)
		}
	}
	for _, rows := range []int{HistoryLongH1MaxRows, HistoryLongH1MaxRows + 1} {
		n := uint32(rows)
		header := make([]byte, 16)
		binary.LittleEndian.PutUint32(header, 0x31425442)
		binary.LittleEndian.PutUint32(header[4:], 1)
		binary.LittleEndian.PutUint32(header[8:], n)
		binary.LittleEndian.PutUint32(header[12:], 6)
		err := ValidateHistoryHeaderWithProfile(profile, header, 16+rows*48)
		if (rows == HistoryLongH1MaxRows) != (err == nil) {
			t.Fatalf("long header rows %d: %v", rows, err)
		}
	}
	// The original fixed profile still refuses the long header.
	header := make([]byte, 16)
	binary.LittleEndian.PutUint32(header, 0x31425442)
	binary.LittleEndian.PutUint32(header[4:], 1)
	binary.LittleEndian.PutUint32(header[8:], HistoryLongH1MaxRows)
	binary.LittleEndian.PutUint32(header[12:], 6)
	if err := ValidateHistoryHeaderWithProfile(HistoryProfile, header, 16+HistoryLongH1MaxRows*48); err == nil {
		t.Fatal("original profile admitted long row count")
	}
	if _, err := historyInternalJSONBoundWithLimit(141397, HistoryMaxInternalJSONBytes, HistoryLongH1MaxRows); err == nil {
		t.Fatal("original internal cap admitted the 141,397-row proof")
	}
	if _, err := historyInternalJSONBoundWithLimit(141397, HistoryLongH1MaxJSONBytes, HistoryLongH1MaxRows); err != nil {
		t.Fatal("long profile internal cap refused 141,397-row proof", err)
	}
	if _, err := historyInternalJSONBoundWithLimit(HistoryLongH1MaxRows, HistoryLongH1MaxJSONBytes, HistoryLongH1MaxRows); err != nil {
		t.Fatal("long profile maximum row proof refused under 512 MiB", err)
	}
}

func TestHistoryInternalSerializationBoundAndOwnership(t *testing.T) {
	maximum, err := historyInternalJSONBound(70000)
	if err != nil || maximum > HistoryMaxInternalJSONBytes {
		t.Fatal(maximum, err)
	}
	var extreme engine.AdaptiveFlagSnapshot
	var fill func(reflect.Value)
	fill = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer:
			v.Set(reflect.New(v.Type().Elem()))
			fill(v.Elem())
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				fill(v.Field(i))
			}
		case reflect.Int, reflect.Int64:
			v.SetInt(math.MinInt64)
		case reflect.Float64:
			v.SetFloat(-math.MaxFloat64)
		case reflect.String:
			v.SetString("\x00\x00\x00\x00\x00\x00\x00\x00")
		}
	}
	fill(reflect.ValueOf(&extreme).Elem())
	extremeJSON, err := json.Marshal(extreme)
	if err != nil {
		t.Fatal(err)
	}
	one, err := historyInternalTypeBound(reflect.TypeOf(extreme), "")
	if err != nil || uint64(len(extremeJSON)) > one {
		t.Fatal("maximum-width internal bound", one, len(extremeJSON), err)
	}
	for _, n := range []int{0, 70001} {
		if _, err := historyInternalJSONBound(n); err == nil {
			t.Fatal("unbounded internal row count")
		}
	}
	for _, typ := range []reflect.Type{reflect.TypeOf([]float64{}), reflect.TypeOf(map[string]int{}), reflect.TypeOf(struct {
		Future string `json:"future"`
	}{})} {
		if _, err := historyInternalTypeBound(typ, ""); err == nil {
			t.Fatal("type drift admitted", typ)
		}
	}
	window := &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: 30 * 1800000}
	p, err := PrepareHistory(HistoryProfile, testMetadata("M30", window), testSource(t, "INITIAL", "M30"))
	if err != nil {
		t.Fatal(err)
	}
	data := testBars("M30", false)
	a, err := p.Build(data)
	if err != nil {
		t.Fatal(err)
	}
	baseline, _ := json.Marshal(a)
	rows, _ := json.Marshal(a.Run.Snapshots)
	bound, err := historyInternalJSONBound(len(a.Run.Snapshots))
	if err != nil || uint64(len(rows)) > bound {
		t.Fatal("internal snapshot bound", err)
	}
	// Returned config and request are owned, including nested config members.
	a.Request.ExecutionWindow.TradeToMS = 1800000
	a.Config["setup"] = map[string]any{"family": "changed"}
	for key, value := range a.Config {
		if nested, ok := value.(map[string]any); ok {
			nested["mutated"] = true
			a.Config[key] = nested
		}
	}
	b, err := p.Build(data)
	if err != nil {
		t.Fatal(err)
	}
	fresh, _ := json.Marshal(b)
	if string(baseline) != string(fresh) {
		t.Fatal("returned result aliases prepared source")
	}
}

func TestHistoryCompleteResultGuardCollectionsAndNoSideEffects(t *testing.T) {
	p, err := PrepareHistory(HistoryProfile, testMetadata("M30", nil), testSource(t, "INITIAL", "M30"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := p.Build(testBars("M30", false))
	if err != nil {
		t.Fatal(err)
	}
	raw := doc.Run
	before, _ := json.Marshal(raw)
	if err := historyResultGuard(raw); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(raw)
	if string(before) != string(after) {
		t.Fatal("native guard mutated complete result")
	}
	// Each complete-result collection receives its own count guard. This is
	// distinct from (and earlier than) later ordered projection validation.
	n := len(raw.Snapshots)
	cases := map[string]func(*engine.AdaptiveFlagResult){
		"snapshots": func(r *engine.AdaptiveFlagResult) { r.Snapshots = nil },
		"states": func(r *engine.AdaptiveFlagResult) {
			r.States = append(append([]engine.AdaptiveFlagState{}, r.States...), engine.AdaptiveFlagState{})
		},
		"orders":      func(r *engine.AdaptiveFlagResult) { r.Orders = make([]engine.AdaptiveFlagOrder, n+1) },
		"events":      func(r *engine.AdaptiveFlagResult) { r.Events = make([]engine.AdaptiveFlagEvent, 4*n+1) },
		"gaps":        func(r *engine.AdaptiveFlagResult) { r.Gaps = make([]engine.AdaptiveFlagGap, n+1) },
		"assumptions": func(r *engine.AdaptiveFlagResult) { r.Assumptions = make([]string, 14) },
		"event links": func(r *engine.AdaptiveFlagResult) {
			r.Orders = append([]engine.AdaptiveFlagOrder{}, r.Orders...)
			r.Orders[0].EventIDs = make([]int, 27)
		},
		"nonfinite": func(r *engine.AdaptiveFlagResult) {
			r.Snapshots = append([]engine.AdaptiveFlagSnapshot{}, r.Snapshots...)
			r.Snapshots[0].Open = math.Inf(1)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			copy := raw
			mutate(&copy)
			if err := historyResultGuard(copy); err == nil {
				t.Fatal("complete-result resource/nonfinite guard skipped")
			}
		})
	}
	oversize := raw
	oversize.Assumptions = []string{strings.Repeat("\x00", HistoryMaxInternalJSONBytes/6+1)}
	if err := historyResultGuard(oversize); err == nil {
		t.Fatal("complete-result byte bound was deferred until encoding")
	}
	// Dynamic value traversal charges every repeated collection, even when an
	// object remains well under the global cap. A newly empty list is not free.
	baseBound, err := HistoryJSONBound(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Snapshots", "Orders", "Events", "States", "Gaps", "Assumptions"} {
		copy := raw
		v := reflect.ValueOf(&copy).Elem().FieldByName(field)
		if v.Len() == 0 {
			v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		} else {
			v.Set(v.Slice(0, 0))
		}
		changed, err := HistoryJSONBound(copy)
		if err != nil {
			t.Fatal(err)
		}
		if changed == baseBound {
			t.Fatal("collection omitted from byte bound", field)
		}
	}
}

type historyTextMarshalerProbe string

func (historyTextMarshalerProbe) MarshalText() ([]byte, error) {
	panic("bound must never invoke a custom text marshaler")
}

func TestHistoryValueGuardRejectsUnreviewedEncodings(t *testing.T) {
	for _, value := range []any{json.RawMessage(`{"custom":1}`), historyTextMarshalerProbe("small"), make(chan int), struct {
		Value int `json:"value,string"`
	}{1}} {
		if _, err := HistoryJSONBound(value); err == nil {
			t.Fatalf("unreviewed encoding admitted: %T", value)
		}
	}
	if _, err := historyInternalTypeBound(reflect.TypeOf(historyTextMarshalerProbe("small")), "Side"); err == nil {
		t.Fatal("custom internal encoding admitted")
	}
	type recursive struct {
		Next *recursive `json:"next"`
	}
	cycle := &recursive{}
	cycle.Next = cycle
	if _, err := HistoryJSONBound(cycle); err == nil {
		t.Fatal("cyclic value traversal admitted")
	}
	if _, err := historyInternalTypeBound(reflect.TypeOf(recursive{}), ""); err == nil {
		t.Fatal("cyclic type traversal admitted")
	}
}
