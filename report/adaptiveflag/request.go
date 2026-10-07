package adaptiveflag

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/engine"
)

const RuntimeRequestSchema = "adaptive-flag-runtime-request-v1"
const RuntimeEnvelopeSchema = "strat-adaptive-volume-flag-runtime-v1"
const MaxMetadataBytes = 4096
const MaxProvidedRows = 16384
const MaxRetainedRows = 4096
const MaxInputBytes = 16 + MaxProvidedRows*6*8
const MaxOutputBytes = 128 * 1024 * 1024
const MaxInputMagnitude = 1e100
const maxExactInteger int64 = 9007199254740991

type RuntimeRequest struct {
	Schema          string                              `json:"schema"`
	NumericalPolicy string                              `json:"numericalPolicy"`
	Symbol          string                              `json:"symbol"`
	Timeframe       string                              `json:"timeframe"`
	ExecutionWindow *engine.AdaptiveFlagExecutionWindow `json:"executionWindow"`
}

// DecodeRuntimeRequest preserves duplicate-key and raw integer evidence. Every
// admitted string is an exact ASCII key/constant, so repaired Unicode cannot
// equal an admitted value. Raw UTF-8 is checked before JSON token conversion.
func DecodeRuntimeRequest(raw string) (RuntimeRequest, error) {
	var out RuntimeRequest
	fail := func(message string) (RuntimeRequest, error) { return RuntimeRequest{}, Rejection("request", message) }
	if len(raw) > MaxMetadataBytes {
		return RuntimeRequest{}, Rejection("resource", "adaptive runtime metadata exceeds 4096 bytes")
	}
	if !utf8.ValidString(raw) {
		return fail("adaptive runtime metadata requires valid UTF-8")
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return fail("adaptive runtime metadata requires one object")
	}
	seen := map[string]bool{}
	for d.More() {
		keyToken, e := d.Token()
		if e != nil {
			return fail(e.Error())
		}
		key, ok := keyToken.(string)
		if !ok || seen[key] {
			return fail("adaptive runtime duplicate/invalid metadata key")
		}
		seen[key] = true
		switch key {
		case "schema", "numericalPolicy", "symbol", "timeframe":
			value, e := d.Token()
			if e != nil {
				return fail(e.Error())
			}
			text, ok := value.(string)
			if !ok {
				return fail("adaptive runtime " + key + " requires its exact string")
			}
			switch key {
			case "schema":
				out.Schema = text
			case "numericalPolicy":
				out.NumericalPolicy = text
			case "symbol":
				out.Symbol = text
			case "timeframe":
				out.Timeframe = text
			}
		case "executionWindow":
			window, e := decodeWindow(d)
			if e != nil {
				return fail(e.Error())
			}
			out.ExecutionWindow = window
		default:
			return fail("adaptive runtime unknown metadata field " + key)
		}
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return fail("adaptive runtime malformed metadata object")
	}
	if _, err = d.Token(); err != io.EOF {
		return fail("adaptive runtime metadata has trailing input")
	}
	for _, key := range []string{"schema", "numericalPolicy", "symbol", "timeframe", "executionWindow"} {
		if !seen[key] {
			return fail("adaptive runtime missing metadata field " + key)
		}
	}
	if out.Schema != RuntimeRequestSchema || out.NumericalPolicy != "BINARY64_ORDERED_V1" || out.Symbol != "XAUUSD" || (out.Timeframe != "M30" && out.Timeframe != "H1") {
		return fail("adaptive runtime schema, numerical policy, symbol or timeframe is unsupported")
	}
	if out.ExecutionWindow != nil {
		w := out.ExecutionWindow
		step := int64(1800000)
		if out.Timeframe == "H1" {
			step = 3600000
		}
		if w.TradeFromMS >= w.TradeToMS || w.TradeFromMS%step != 0 || w.TradeToMS%step != 0 {
			return fail("adaptive runtime window requires increasing exact UTC timeframe boundaries")
		}
	}
	return out, nil
}
func decodeWindow(d *json.Decoder) (*engine.AdaptiveFlagExecutionWindow, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, nil
	}
	if token != json.Delim('{') {
		return nil, fmt.Errorf("adaptive runtime window requires null or an object")
	}
	seen := map[string]bool{}
	out := &engine.AdaptiveFlagExecutionWindow{}
	for d.More() {
		keyToken, e := d.Token()
		if e != nil {
			return nil, e
		}
		key, ok := keyToken.(string)
		if !ok || seen[key] || (key != "tradeFromMs" && key != "tradeToMs") {
			return nil, fmt.Errorf("adaptive runtime window has duplicate/unknown key")
		}
		seen[key] = true
		value, e := d.Token()
		if e != nil {
			return nil, e
		}
		number, ok := value.(json.Number)
		if !ok {
			return nil, fmt.Errorf("adaptive runtime window requires unsigned integer tokens")
		}
		token := number.String()
		if len(token) == 0 || (len(token) > 1 && token[0] == '0') {
			return nil, fmt.Errorf("adaptive runtime window requires unsigned integer tokens")
		}
		for _, r := range token {
			if r < '0' || r > '9' {
				return nil, fmt.Errorf("adaptive runtime window requires unsigned integer tokens")
			}
		}
		n, e := number.Int64()
		if e != nil || n < 0 || n > maxExactInteger {
			return nil, fmt.Errorf("adaptive runtime window requires exact-safe nonnegative milliseconds")
		}
		if key == "tradeFromMs" {
			out.TradeFromMS = n
		} else {
			out.TradeToMS = n
		}
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') || !seen["tradeFromMs"] || !seen["tradeToMs"] {
		return nil, fmt.Errorf("adaptive runtime incomplete window")
	}
	return out, nil
}
