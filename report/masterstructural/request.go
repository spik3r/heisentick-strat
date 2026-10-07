package masterstructural

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/master"
)

const (
	RuntimeSchema    = "master-structural-portable-runtime-request-v1"
	MaxMetadataBytes = 4096
	MaxSourceBytes   = 65536
	MaxRows          = 100000
	MaxInputBytes    = 16 + MaxRows*6*8
	MaxOutputBytes   = 128 * 1024 * 1024
	maxExactInteger  = 9007199254740991
)

// DecodeRuntimeRequest rejects duplicate decoded keys, aliases, missing/null
// values, trailing documents and unknown fields before any market-data copy.
func DecodeRuntimeRequest(raw string) (Options, error) {
	var options Options
	if len(raw) > MaxMetadataBytes || !utf8.ValidString(raw) {
		return options, fmt.Errorf("master runtime metadata must be valid UTF-8 within %d bytes", MaxMetadataBytes)
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return options, fmt.Errorf("master runtime metadata requires an object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return options, fmt.Errorf("master runtime metadata: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok || seen[key] {
			return options, fmt.Errorf("master runtime duplicate/invalid metadata key")
		}
		seen[key] = true
		value, err := decoder.Token()
		if err != nil {
			return options, fmt.Errorf("master runtime metadata: %w", err)
		}
		switch key {
		case "schema":
			if value != RuntimeSchema {
				return options, fmt.Errorf("master runtime metadata schema must equal %s", RuntimeSchema)
			}
		case "arithmeticContract":
			if value != master.PortableArithmeticContract {
				return options, fmt.Errorf("master runtime arithmeticContract must equal %s", master.PortableArithmeticContract)
			}
		case "warmupFromT", "tradeFromT", "tradeToT":
			number, ok := value.(json.Number)
			if !ok {
				return options, fmt.Errorf("master runtime %s requires an integer", key)
			}
			t, err := number.Int64()
			if err != nil || t < 0 || t > maxExactInteger || t%master.M30MS != 0 {
				return options, fmt.Errorf("master runtime %s requires an exact nonnegative safe-integer UTC M30 boundary", key)
			}
			switch key {
			case "warmupFromT":
				options.WarmupFromT = t
			case "tradeFromT":
				options.TradeFromT = t
			case "tradeToT":
				options.TradeToT = t
			}
		case "spread":
			number, ok := value.(json.Number)
			if !ok {
				return options, fmt.Errorf("master runtime spread requires 0 or 1")
			}
			options.Spread, err = DecodePortableSpread(number.String())
			if err != nil {
				return options, err
			}
		default:
			return options, fmt.Errorf("master runtime unknown metadata field %q", key)
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return options, fmt.Errorf("master runtime malformed metadata object")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return options, fmt.Errorf("master runtime metadata has trailing input")
	}
	for _, key := range []string{"schema", "arithmeticContract", "warmupFromT", "tradeFromT", "tradeToT", "spread"} {
		if !seen[key] {
			return options, fmt.Errorf("master runtime missing metadata field %s", key)
		}
	}
	if options.WarmupFromT >= options.TradeFromT || options.TradeFromT >= options.TradeToT {
		return options, fmt.Errorf("master runtime requires warmup < trade from < trade to")
	}
	return options, nil
}

// DecodePortableSpread validates the original token before float conversion.
// Both native flags and JSON metadata use this exact closed admission rule.
func DecodePortableSpread(token string) (float64, error) {
	if len(token) == 0 || len(token) > 64 || strings.TrimSpace(token) != token || strings.ContainsAny(token, "eE") {
		return 0, fmt.Errorf("master runtime spread requires plain decimal 0 or 1")
	}
	decoder := json.NewDecoder(strings.NewReader(token))
	decoder.UseNumber()
	value, err := decoder.Token()
	number, ok := value.(json.Number)
	if err != nil || !ok || number.String() != token {
		return 0, fmt.Errorf("master runtime spread requires exact 0 or 1")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return 0, fmt.Errorf("master runtime spread requires exact 0 or 1")
	}
	// Token length and absence of exponents bound arbitrary-precision work.
	exact, ok := new(big.Rat).SetString(token)
	if !ok || (exact.Sign() != 0 && exact.Cmp(big.NewRat(1, 1)) != 0) {
		return 0, fmt.Errorf("master runtime spread requires exact 0 or 1")
	}
	return number.Float64() // Preserve admitted negative zero.
}

// ValidateRuntimeHeader accepts only a bounded complete six-column BTB1. The
// WASM adapter calls it with a 16-byte header before copying the full view.
func ValidateRuntimeHeader(header []byte, actualBytes int) error {
	if actualBytes < 16 || actualBytes > MaxInputBytes || len(header) < 16 {
		return fmt.Errorf("master runtime BTB1 byte length outside [16,%d]", MaxInputBytes)
	}
	if binary.LittleEndian.Uint32(header[:4]) != 0x31425442 || binary.LittleEndian.Uint32(header[4:8]) != 1 {
		return fmt.Errorf("master runtime requires BTB1 version 1")
	}
	count := uint64(binary.LittleEndian.Uint32(header[8:12]))
	if count == 0 || count > MaxRows || binary.LittleEndian.Uint32(header[12:16]) != 6 {
		return fmt.Errorf("master runtime requires 1..%d rows and exactly six BTB1 columns", MaxRows)
	}
	if uint64(actualBytes) != 16+count*6*8 {
		return fmt.Errorf("master runtime BTB1 byte length mismatch")
	}
	return nil
}

func preflightRuntime(data []byte, cfg dsl.Config, options Options) error {
	count := int(binary.LittleEndian.Uint32(data[8:12]))
	value := func(column, row int) float64 {
		at := 16 + (column*count+row)*8
		return math.Float64frombits(binary.LittleEndian.Uint64(data[at : at+8]))
	}
	var nativeRows, higherRows, tradeRows uint64
	previous, lastNative, lastH4 := -1., int64(-1), int64(-1)
	for row := 0; row < count; row++ {
		t := value(0, row)
		if math.IsNaN(t) || math.IsInf(t, 0) || t < 0 || t > maxExactInteger || math.Trunc(t) != t || int64(t)%master.M5MS != 0 || t <= previous {
			return fmt.Errorf("master runtime invalid or unordered M5 timestamp at row %d", row)
		}
		previous = t
		// All transmitted values are finite, including unused future history.
		for column := 1; column < 6; column++ {
			v := value(column, row)
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("master runtime nonfinite OHLCV at row %d", row)
			}
		}
		if value(5, row) < 0 {
			return fmt.Errorf("master runtime negative volume at row %d", row)
		}
		stamp := int64(t)
		if stamp < options.WarmupFromT || stamp >= options.TradeToT {
			continue
		}
		bucket := stamp / master.M30MS * master.M30MS
		if bucket != lastNative {
			nativeRows++
			if bucket >= options.TradeFromT {
				tradeRows++
			}
			lastNative = bucket
		}
		h4 := stamp / master.H4MS * master.H4MS
		if h4 != lastH4 && h4+master.H4MS <= options.TradeToT {
			higherRows++
			lastH4 = h4
		}
	}
	bound, err := runtimeOutputBound(cfg, nativeRows, higherRows, tradeRows)
	if err != nil {
		return err
	}
	if bound > MaxOutputBytes {
		return fmt.Errorf("master runtime conservative output bound %d exceeds %d bytes", bound, MaxOutputBytes)
	}
	return nil
}

// runtimeOutputBound covers BOTH master.Run's compact JSON validity allocation
// and the final pretty envelope before either allocation happens. Every output
// slice has an explicit cardinality bound from execute/buildIndicators: at most
// one signal, closed trade and edit per trading M30 row; each monthly group
// contains a trade; and there is at most one terminal position/pending signal.
// The fixed engine has exactly 12 assumption strings. Every other result string
// is a fixed literal <=256 UTF-8 bytes, a fixed exit reason, or an entry-month
// label <=16 bytes for admitted safe-integer timestamps. Source-derived strings
// occur ONLY in cfg, whose bounded exact encoding is included separately.
func runtimeOutputBound(cfg dsl.Config, nativeRows, higherRows, tradeRows uint64) (uint64, error) {
	slices := map[reflect.Type]uint64{
		reflect.TypeOf([]master.IndicatorRow{}):   nativeRows,
		reflect.TypeOf([]master.H4Row{}):          higherRows,
		reflect.TypeOf([]master.Signal{}):         tradeRows,
		reflect.TypeOf([]master.Trade{}):          tradeRows,
		reflect.TypeOf([]master.OrderEdit{}):      tradeRows,
		reflect.TypeOf([]master.MonthlySummary{}): tradeRows,
		reflect.TypeOf([]string{}):                12,
	}
	result, err := maximumJSON(reflect.TypeOf(master.Result{}), 1, slices)
	if err != nil {
		return 0, err
	}
	config, err := json.MarshalIndent(cfg, "  ", "  ")
	if err != nil {
		return 0, err
	}
	// 4096 covers all fixed portable envelope/provenance fields, including
	// schemas, three 64-byte hashes, punctuation, indentation and newline.
	// TestPortableEnvelopeFixedAllowance independently checks this allowance.
	return result + uint64(len(config)) + portableEnvelopeAllowance, nil
}

// maximumJSON is a deliberately loose upper bound for the known finite report
// types, not a typical-row estimate. 32 bytes bounds every finite JSON float64
// and integer. Strings include 6x escaping expansion plus quotes. Struct field
// names are similarly bounded and all whitespace is counted. Anonymous fields
// are treated as extra nested objects, overcounting both keys and indentation.
// Unknown dynamic types/slices fail closed when the report contract changes.
func maximumJSON(t reflect.Type, depth uint64, slices map[reflect.Type]uint64) (uint64, error) {
	switch t.Kind() {
	case reflect.Pointer:
		return maximumJSON(t.Elem(), depth, slices)
	case reflect.Bool:
		return 5, nil
	case reflect.Int, reflect.Int64, reflect.Float64:
		return 32, nil
	case reflect.String:
		return 6*256 + 2, nil
	case reflect.Struct:
		n := uint64(2) + 2*depth + 1
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			v, err := maximumJSON(field.Type, depth+1, slices)
			if err != nil {
				return 0, err
			}
			n += 2*(depth+1) + uint64(6*len(name)+2) + 2 + v + 2
		}
		return n, nil
	case reflect.Slice:
		count, ok := slices[t]
		if !ok {
			return 0, fmt.Errorf("master runtime unbounded report slice %s", t)
		}
		v, err := maximumJSON(t.Elem(), depth+1, slices)
		if err != nil {
			return 0, err
		}
		return 2 + 2*depth + 1 + count*(2*(depth+1)+v+2), nil
	default:
		return 0, fmt.Errorf("master runtime unbounded report type %s", t)
	}
}
