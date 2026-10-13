//go:build !js && !wasm

package adaptiveflag

import (
	"encoding"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	HistoryProfile                    = "adaptive-history-v1"
	HistoryRawSchema                  = "strat-adaptive-volume-flag-history-raw-v1"
	HistoryMaxRows                    = 70000
	HistoryMaxInputBytes              = 16 + HistoryMaxRows*6*8
	HistoryMaxCalendarDays            = 2200
	HistoryMaxInternalJSONBytes       = 256 * 1024 * 1024
	historyCalendarEndpointMS   int64 = 253402300800000
)

// HistoryDocument is a freshly owned native result, never a caller ledger input.
// Its compact serialization has a separate hash domain from portable Stage A.
type HistoryDocument struct {
	Schema        string                    `json:"schema"`
	Profile       string                    `json:"profile"`
	Request       RuntimeRequest            `json:"request"`
	RequestSHA256 string                    `json:"requestSha256"`
	DSLSHA256     string                    `json:"dslSha256"`
	ConfigSHA256  string                    `json:"configSha256"`
	BTB1SHA256    string                    `json:"btb1Sha256"`
	RawOnly       bool                      `json:"rawOnly"`
	Economics     string                    `json:"economics"`
	Config        dsl.Config                `json:"config"`
	Run           engine.AdaptiveFlagResult `json:"run"`
}

type PreparedHistory struct{ runtime *PreparedRuntime }

func PrepareHistory(profile, metadata, source string) (*PreparedHistory, error) {
	if profile != HistoryProfile {
		return nil, Rejection("request", "explicit adaptive-history-v1 native profile required")
	}
	p, err := PrepareRuntime(metadata, source)
	if err != nil {
		return nil, err
	}
	if w := p.request.ExecutionWindow; w != nil {
		if err := historyCalendar(w.TradeFromMS, w.TradeToMS); err != nil {
			return nil, err
		}
	}
	return &PreparedHistory{p}, nil
}

func historyCalendar(start, end int64) error {
	if start < 0 || end <= start || end > historyCalendarEndpointMS {
		return Rejection("request", "native history window is outside UTC 1970..9999")
	}
	if (end-1)/86400000-start/86400000+1 > HistoryMaxCalendarDays {
		return Rejection("resource", "native history window exceeds 2200 UTC date labels")
	}
	return nil
}

// ValidateHistoryHeader may inspect only the header, before a bounded full read.
func ValidateHistoryHeader(header []byte, actualBytes int) error {
	if len(header) < 16 || actualBytes < 16 || actualBytes > HistoryMaxInputBytes {
		return Rejection("resource", "native history BTB1 length outside bounded profile")
	}
	if binary.LittleEndian.Uint32(header[:4]) != 0x31425442 || binary.LittleEndian.Uint32(header[4:8]) != 1 {
		return Rejection("input", "native history requires BTB1 version 1")
	}
	n := uint64(binary.LittleEndian.Uint32(header[8:12]))
	if n == 0 || n > HistoryMaxRows || binary.LittleEndian.Uint32(header[12:16]) != 6 {
		return Rejection("input", "native history requires 1..70000 supplied rows and exactly six columns")
	}
	if uint64(actualBytes) != 16+n*48 {
		return Rejection("input", "native history BTB1 exact byte length mismatch")
	}
	return nil
}

// Preflight preserves cutoff-before-validation and all supplied warmup. There is
// no chunk, tail selection, interpolation or substitute clock/calendar.
func (p *PreparedHistory) Preflight(data []byte) (int, error) {
	if p == nil || p.runtime == nil || !p.runtime.ready {
		return 0, Rejection("request", "native history source was not prepared")
	}
	if err := ValidateHistoryHeader(data, len(data)); err != nil {
		return 0, err
	}
	n := int(binary.LittleEndian.Uint32(data[8:12]))
	value := func(col, row int) float64 {
		at := 16 + (col*n+row)*8
		return math.Float64frombits(binary.LittleEndian.Uint64(data[at : at+8]))
	}
	step := int64(1800000)
	if p.runtime.request.Timeframe == "H1" {
		step = 3600000
	}
	previous, first, last, retained := -1.0, int64(0), int64(0), 0
	for row := 0; row < n; row++ {
		stamp := value(0, row)
		if w := p.runtime.request.ExecutionWindow; w != nil && stamp >= float64(w.TradeToMS) {
			break
		}
		if math.IsNaN(stamp) || math.IsInf(stamp, 0) || stamp < 0 || stamp > float64(historyCalendarEndpointMS-step) || math.Trunc(stamp) != stamp || int64(stamp)%step != 0 || stamp <= previous {
			return 0, Rejection("input", "native history invalid retained timestamp")
		}
		var values [5]float64
		for col := 1; col < 6; col++ {
			v := value(col, row)
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > MaxInputMagnitude {
				return 0, Rejection("input", "native history retained OHLCV outside finite magnitude domain")
			}
			values[col-1] = v
		}
		o, h, l, c, v := values[0], values[1], values[2], values[3], values[4]
		if l <= 0 || o < l || o > h || c < l || c > h || v < 0 {
			return 0, Rejection("input", "native history invalid retained OHLCV")
		}
		if retained == 0 {
			first = int64(stamp)
		}
		last = int64(stamp)
		previous = stamp
		retained++
	}
	if retained == 0 {
		return 0, Rejection("input", "native history requires a nonempty retained prefix")
	}
	start, end := first, last+step
	if w := p.runtime.request.ExecutionWindow; w != nil {
		start, end = w.TradeFromMS, w.TradeToMS
	}
	if err := historyCalendar(start, end); err != nil {
		return 0, err
	}
	return retained, nil
}

// Build invokes the unchanged raw engine once. It deliberately does not marshal
// a large result: the native unit owner admits actual fields and byte bounds first.
func (p *PreparedHistory) Build(data []byte) (*HistoryDocument, error) {
	n, err := p.Preflight(data)
	if err != nil {
		return nil, err
	}
	// The unchanged engine marshals snapshots for finite-value validation and
	// ordered six-number rows for identity. Bound both before entering it.
	if _, err := historyInternalJSONBound(n); err != nil {
		return nil, Rejection("resource", "native history internal JSON bound exceeded")
	}
	r := p.runtime
	series, err := marketdata.DecodeBTB1(data)
	if err != nil {
		return nil, Rejection("input", err.Error())
	}
	result, err := engine.RunAdaptiveVolumeFlagWithResultGuard(engine.AdaptiveFlagRequest{Config: r.config, Series: series, Window: r.request.ExecutionWindow}, historyResultGuard)
	if err != nil {
		if _, ok := err.(*runtimeRejection); ok {
			return nil, err
		}
		return nil, Rejection("execution", err.Error())
	}
	if len(result.Snapshots) != n || result.ResearchPolicy != nil || result.ResearchPolicySHA256 != "" || result.ResearchProducer != nil {
		return nil, Rejection("execution", "native history baseline result identity mismatch")
	}
	config, err := json.Marshal(r.config)
	if err != nil {
		return nil, Rejection("execution", "native history config serialization failed")
	}
	// Only the small admitted config is round-tripped for private ownership,
	// never the large raw ledger or numerical state. A returned typed document
	// must not provide a mutable alias into its reusable prepared source.
	var ownedConfig dsl.Config
	if err := json.Unmarshal(config, &ownedConfig); err != nil {
		return nil, Rejection("execution", "native history config ownership failed")
	}
	request := r.request
	if request.ExecutionWindow != nil {
		window := *request.ExecutionWindow
		request.ExecutionWindow = &window
	}
	return &HistoryDocument{HistoryRawSchema, HistoryProfile, request, r.requestHash, hash([]byte(r.source)), hash(config), hash(data), true, "raw-model-only", ownedConfig, result}, nil
}

// This closed proof covers the engine's repeated internal compact-JSON
// allocations before entry. New kinds/string roles fail closed pending review.
func historyInternalTypeBound(typ reflect.Type, role string) (uint64, error) {
	return historyInternalTypeBoundAt(typ, role, 0)
}

func historyInternalTypeBoundAt(typ reflect.Type, role string, depth int) (uint64, error) {
	if depth > 16 {
		return 0, fmt.Errorf("internal type recursion bound")
	}
	for _, contract := range []reflect.Type{reflect.TypeOf((*json.Marshaler)(nil)).Elem(), reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()} {
		if typ.Implements(contract) || (typ.Kind() != reflect.Pointer && reflect.PointerTo(typ).Implements(contract)) {
			return 0, fmt.Errorf("unreviewed internal JSON marshaler")
		}
	}
	switch typ.Kind() {
	case reflect.Pointer:
		value, err := historyInternalTypeBoundAt(typ.Elem(), role, depth+1)
		return value + 4, err
	case reflect.Bool:
		return 5, nil
	case reflect.Int, reflect.Int64:
		return 20, nil
	case reflect.Float64:
		return 32, nil
	case reflect.String:
		if role == "Side" {
			return 6*8 + 2, nil
		}
		return 0, fmt.Errorf("unreviewed internal snapshot string %s", role)
	case reflect.Struct:
		sum := uint64(2)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" {
				continue
			}
			parts := strings.Split(field.Tag.Get("json"), ",")
			for _, option := range parts[1:] {
				if option != "omitempty" {
					return 0, fmt.Errorf("unreviewed internal JSON option %s", option)
				}
			}
			name := parts[0]
			if name == "-" {
				continue
			}
			value, err := historyInternalTypeBoundAt(field.Type, field.Name, depth+1)
			if err != nil {
				return 0, err
			}
			if name == "" && !field.Anonymous {
				return 0, fmt.Errorf("unreviewed internal field %s", field.Name)
			}
			if name != "" {
				value += uint64(len(strconv.Quote(name))) + 2
			}
			sum += value
		}
		return sum, nil
	default:
		return 0, fmt.Errorf("unreviewed internal snapshot type %s", typ)
	}
}

func historyInternalJSONBound(n int) (uint64, error) {
	if n < 1 || n > HistoryMaxRows {
		return 0, fmt.Errorf("native history internal row bound")
	}
	one, err := historyInternalTypeBound(reflect.TypeOf(engine.AdaptiveFlagSnapshot{}), "")
	if err != nil {
		return 0, err
	}
	snapshots := 2 + uint64(n)*(one+1)
	inputRows := 2 + uint64(n)*(2+6*(32+1)+1)
	if inputRows > snapshots {
		snapshots = inputRows
	}
	// Named presets also have a small effective-config identity allocation.
	config, err := boundJSON(reflect.TypeOf(dsl.AdaptiveFlagSpec{}), 1, "", 1)
	if err != nil {
		return 0, err
	}
	if config > snapshots {
		snapshots = config
	}
	if snapshots > HistoryMaxInternalJSONBytes {
		return 0, fmt.Errorf("internal JSON allocation exceeds256MiB")
	}
	return snapshots, nil
}

// historyResultGuard is side-effect-free. It runs on the actual complete engine
// result before that engine's final serializer, not after a large allocation.
func historyResultGuard(result engine.AdaptiveFlagResult) error {
	n := len(result.Snapshots)
	if n < 1 || n > HistoryMaxRows || len(result.States) != n || len(result.Orders) > n || len(result.Events) > 4*n || len(result.Gaps) > n || len(result.Assumptions) > 13 {
		return Rejection("resource", "native history complete result collection bound exceeded")
	}
	for _, order := range result.Orders {
		if len(order.EventIDs) > 26 {
			return Rejection("resource", "native history order event-link bound exceeded")
		}
	}
	if _, err := HistoryJSONBound(result); err != nil {
		return Rejection("resource", "native history complete result pre-serialization bound rejected")
	}
	return nil
}

// Bound actual owned values before any large JSON serializer allocation. This
// is an upper bound, not a typical-row estimate: all strings reserve six bytes
// per input UTF-8 byte, all numeric tokens their full reviewed widths, all
// omitted fields are counted, and embedded struct braces are overcounted.
// Custom marshalers and newly unhandled kinds fail closed. No map/array is
// produced here, and traversal itself is capped independently of the byte cap.
func HistoryJSONBound(value any) (uint64, error) {
	nodes := 0
	marshaler := reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshaler := reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	var walk func(reflect.Value, int) (uint64, error)
	add := func(a, b uint64) (uint64, error) {
		if a > HistoryMaxInternalJSONBytes || b > HistoryMaxInternalJSONBytes-a {
			return 0, fmt.Errorf("native history JSON byte bound exceeds 256 MiB")
		}
		return a + b, nil
	}
	stringBound := func(s string) (uint64, error) {
		if !utf8.ValidString(s) {
			return 0, fmt.Errorf("invalid UTF-8 in native history output")
		}
		if uint64(len(s)) > (HistoryMaxInternalJSONBytes-2)/6 {
			return 0, fmt.Errorf("native history string exceeds byte bound")
		}
		return 6*uint64(len(s)) + 2, nil
	}
	walk = func(v reflect.Value, depth int) (uint64, error) {
		nodes++
		if nodes > 20000000 || depth > 32 {
			return 0, fmt.Errorf("native history value traversal bound exceeded")
		}
		if !v.IsValid() {
			return 4, nil
		}
		if v.Type().Implements(marshaler) || v.Type().Implements(textMarshaler) || (v.Kind() != reflect.Pointer && (reflect.PointerTo(v.Type()).Implements(marshaler) || reflect.PointerTo(v.Type()).Implements(textMarshaler))) {
			return 0, fmt.Errorf("unreviewed native history JSON marshaler")
		}
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if v.IsNil() {
				return 4, nil
			}
			return walk(v.Elem(), depth+1)
		case reflect.Bool:
			return 5, nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return 20, nil
		case reflect.Float32, reflect.Float64:
			if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
				return 0, fmt.Errorf("nonfinite native history output")
			}
			return 32, nil
		case reflect.String:
			return stringBound(v.String())
		case reflect.Struct:
			total := uint64(2)
			typ := v.Type()
			for i := 0; i < v.NumField(); i++ {
				f := typ.Field(i)
				if f.PkgPath != "" {
					continue
				}
				parts := strings.Split(f.Tag.Get("json"), ",")
				for _, option := range parts[1:] {
					if option != "omitempty" {
						return 0, fmt.Errorf("unreviewed JSON tag option %s", option)
					}
				}
				name := parts[0]
				if name == "-" {
					continue
				}
				if name == "" && !f.Anonymous {
					return 0, fmt.Errorf("unreviewed untagged history field %s", f.Name)
				}
				term, err := walk(v.Field(i), depth+1)
				if err != nil {
					return 0, err
				}
				if name != "" {
					term += uint64(len(strconv.Quote(name))) + 2
				}
				total, err = add(total, term)
				if err != nil {
					return 0, err
				}
			}
			return total, nil
		case reflect.Slice, reflect.Array:
			if v.Kind() == reflect.Slice && v.IsNil() {
				return 4, nil
			}
			if v.Type().Elem().Kind() == reflect.Uint8 {
				return 0, fmt.Errorf("unreviewed base64 history byte collection")
			}
			if v.Len() > 280000 {
				return 0, fmt.Errorf("native history collection traversal limit")
			}
			total := uint64(2)
			for i := 0; i < v.Len(); i++ {
				term, err := walk(v.Index(i), depth+1)
				if err != nil {
					return 0, err
				}
				total, err = add(total, term+1)
				if err != nil {
					return 0, err
				}
			}
			return total, nil
		case reflect.Map:
			if v.IsNil() {
				return 4, nil
			}
			if v.Type().Key().Kind() != reflect.String || v.Len() > 70000 {
				return 0, fmt.Errorf("unreviewed history map")
			}
			total := uint64(2)
			iter := v.MapRange()
			for iter.Next() {
				key, err := stringBound(iter.Key().String())
				if err != nil {
					return 0, err
				}
				term, err := walk(iter.Value(), depth+1)
				if err != nil {
					return 0, err
				}
				total, err = add(total, key+term+2)
				if err != nil {
					return 0, err
				}
			}
			return total, nil
		default:
			return 0, fmt.Errorf("unreviewed native history output kind %s", v.Kind())
		}
	}
	return walk(reflect.ValueOf(value), 0)
}
