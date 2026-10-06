package dsl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	frozenLevelMaxBytes       = 1 << 20
	frozenLevelMaxDepth       = 64
	frozenLevelMaxNumberBytes = 1024
	frozenLevelMaxExponent    = 10000
)

// DecodeFrozenLevelConfigJSON is the lossless raw boundary for this family.
// Unlike encoding/json.Unmarshal into a map, it rejects duplicate decoded
// keys, invalid Unicode, and fractional integers before float64 conversion.
func DecodeFrozenLevelConfigJSON(raw []byte) (Config, error) {
	value, err := strictFrozenJSON(raw)
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("frozen-level config must be an object")
	}
	return NormalizeFrozenLevelConfig(Config(root))
}

func strictFrozenJSON(raw []byte) (any, error) {
	if len(raw) > frozenLevelMaxBytes {
		return nil, fmt.Errorf("frozen-level JSON exceeds 1 MiB")
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("frozen-level JSON contains invalid UTF-8")
	}
	r := frozenJSONReader{raw: raw}
	value, err := r.value(0)
	if err != nil {
		return nil, err
	}
	r.whitespace()
	if r.pos != len(raw) {
		return nil, r.error("expected EOF after one JSON document")
	}
	return value, nil
}

// strictFrozenJSONString also serves the family-local DSL scanner. The input
// must be exactly one complete double-quoted JSON string, without whitespace.
func strictFrozenJSONString(raw []byte) (string, error) {
	if len(raw) > frozenLevelMaxBytes || !utf8.Valid(raw) {
		return "", fmt.Errorf("frozen-level string exceeds 1 MiB or contains invalid UTF-8")
	}
	r := frozenJSONReader{raw: raw}
	value, err := r.quoted()
	if err != nil {
		return "", err
	}
	if r.pos != len(raw) {
		return "", r.error("unexpected data after string")
	}
	return value, nil
}

type frozenJSONReader struct {
	raw []byte
	pos int
}

func (r *frozenJSONReader) error(message string) error {
	return fmt.Errorf("frozen-level JSON byte %d: %s", r.pos, message)
}

func (r *frozenJSONReader) whitespace() {
	for r.pos < len(r.raw) {
		switch r.raw[r.pos] {
		case ' ', '\t', '\r', '\n':
			r.pos++
		default:
			return
		}
	}
}

func (r *frozenJSONReader) take(b byte) bool {
	if r.pos < len(r.raw) && r.raw[r.pos] == b {
		r.pos++
		return true
	}
	return false
}

func (r *frozenJSONReader) value(depth int) (any, error) {
	r.whitespace()
	if r.pos == len(r.raw) {
		return nil, r.error("expected a JSON value")
	}
	switch r.raw[r.pos] {
	case '{', '[':
		if depth >= frozenLevelMaxDepth {
			return nil, r.error("nesting exceeds 64")
		}
		if r.take('{') {
			return r.object(depth + 1)
		}
		r.pos++
		return r.array(depth + 1)
	case '"':
		return r.quoted()
	case 't', 'f', 'n':
		for _, literal := range []struct {
			text  string
			value any
		}{
			{"true", true}, {"false", false}, {"null", nil},
		} {
			end := r.pos + len(literal.text)
			if end <= len(r.raw) && string(r.raw[r.pos:end]) == literal.text {
				r.pos += len(literal.text)
				return literal.value, nil
			}
		}
		return nil, r.error("invalid JSON literal")
	default:
		start := r.pos
		for r.pos < len(r.raw) {
			b := r.raw[r.pos]
			if (b >= '0' && b <= '9') || b == '-' || b == '+' || b == '.' || b == 'e' || b == 'E' {
				r.pos++
				if r.pos-start > frozenLevelMaxNumberBytes {
					return nil, r.error("numeric token exceeds 1024 bytes")
				}
			} else {
				break
			}
		}
		token := string(r.raw[start:r.pos])
		if _, err := frozenNumberParts(token); err != nil {
			return nil, r.error(err.Error())
		}
		return json.Number(token), nil
	}
}

func (r *frozenJSONReader) object(depth int) (map[string]any, error) {
	result := make(map[string]any)
	r.whitespace()
	if r.take('}') {
		return result, nil
	}
	for {
		r.whitespace()
		key, err := r.quoted()
		if err != nil {
			return nil, err
		}
		if _, found := result[key]; found {
			return nil, r.error("duplicate decoded object key")
		}
		r.whitespace()
		if !r.take(':') {
			return nil, r.error("expected colon after object key")
		}
		value, err := r.value(depth)
		if err != nil {
			return nil, err
		}
		result[key] = value
		r.whitespace()
		if r.take('}') {
			return result, nil
		}
		if !r.take(',') {
			return nil, r.error("expected comma or object end")
		}
	}
}

func (r *frozenJSONReader) array(depth int) ([]any, error) {
	result := make([]any, 0)
	r.whitespace()
	if r.take(']') {
		return result, nil
	}
	for {
		value, err := r.value(depth)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
		r.whitespace()
		if r.take(']') {
			return result, nil
		}
		if !r.take(',') {
			return nil, r.error("expected comma or array end")
		}
	}
}

func (r *frozenJSONReader) quoted() (string, error) {
	if !r.take('"') {
		return "", r.error("expected double-quoted string")
	}
	var decoded []byte
	for r.pos < len(r.raw) {
		b := r.raw[r.pos]
		r.pos++
		switch {
		case b == '"':
			return string(decoded), nil
		case b < 0x20:
			return "", r.error("unescaped string control character")
		case b != '\\':
			decoded = append(decoded, b)
		default:
			if r.pos == len(r.raw) {
				return "", r.error("incomplete string escape")
			}
			escape := r.raw[r.pos]
			r.pos++
			switch escape {
			case '"', '\\', '/':
				decoded = append(decoded, escape)
			case 'b':
				decoded = append(decoded, '\b')
			case 'f':
				decoded = append(decoded, '\f')
			case 'n':
				decoded = append(decoded, '\n')
			case 'r':
				decoded = append(decoded, '\r')
			case 't':
				decoded = append(decoded, '\t')
			case 'u':
				runeValue, err := r.hexRune()
				if err != nil {
					return "", err
				}
				if runeValue >= 0xD800 && runeValue <= 0xDBFF {
					if !r.take('\\') || !r.take('u') {
						return "", r.error("high surrogate requires a low surrogate")
					}
					low, err := r.hexRune()
					if err != nil || low < 0xDC00 || low > 0xDFFF {
						return "", r.error("invalid low surrogate")
					}
					runeValue = utf16.DecodeRune(runeValue, low)
				} else if runeValue >= 0xDC00 && runeValue <= 0xDFFF {
					return "", r.error("unpaired low surrogate")
				}
				decoded = utf8.AppendRune(decoded, runeValue)
			default:
				return "", r.error("invalid string escape")
			}
		}
	}
	return "", r.error("unterminated string")
}

func (r *frozenJSONReader) hexRune() (rune, error) {
	var result rune
	for i := 0; i < 4; i++ {
		if r.pos == len(r.raw) {
			return 0, r.error("incomplete Unicode escape")
		}
		b := r.raw[r.pos]
		r.pos++
		result *= 16
		switch {
		case b >= '0' && b <= '9':
			result += rune(b - '0')
		case b >= 'a' && b <= 'f':
			result += rune(b - 'a' + 10)
		case b >= 'A' && b <= 'F':
			result += rune(b - 'A' + 10)
		default:
			return 0, r.error("invalid Unicode escape")
		}
	}
	return result, nil
}

type frozenDecimalParts struct {
	negative bool
	digits   string
	scale    int // Exact value is signed digits * 10^scale.
}

// frozenNumberParts checks JSON number grammar and resource limits without
// first converting through a floating-point representation or a large power.
func frozenNumberParts(token string) (frozenDecimalParts, error) {
	var result frozenDecimalParts
	bad := func() (frozenDecimalParts, error) { return result, fmt.Errorf("invalid or oversized JSON number") }
	if len(token) == 0 || len(token) > frozenLevelMaxNumberBytes {
		return bad()
	}
	i := 0
	if token[i] == '-' {
		result.negative = true
		i++
	}
	if i == len(token) {
		return bad()
	}
	start := i
	if token[i] == '0' {
		i++
	} else if token[i] >= '1' && token[i] <= '9' {
		for i < len(token) && token[i] >= '0' && token[i] <= '9' {
			i++
		}
	} else {
		return bad()
	}
	result.digits = token[start:i]
	if i < len(token) && token[i] == '.' {
		i++
		start = i
		for i < len(token) && token[i] >= '0' && token[i] <= '9' {
			i++
		}
		if start == i {
			return bad()
		}
		result.digits += token[start:i]
		result.scale = -(i - start)
	}
	if i < len(token) && (token[i] == 'e' || token[i] == 'E') {
		i++
		negativeExponent := false
		if i < len(token) && (token[i] == '+' || token[i] == '-') {
			negativeExponent = token[i] == '-'
			i++
		}
		start = i
		exponent := 0
		for i < len(token) && token[i] >= '0' && token[i] <= '9' {
			exponent = exponent*10 + int(token[i]-'0')
			if exponent > frozenLevelMaxExponent {
				return bad()
			}
			i++
		}
		if start == i {
			return bad()
		}
		if negativeExponent {
			exponent = -exponent
		}
		result.scale += exponent
	}
	if i != len(token) {
		return bad()
	}
	result.digits = strings.TrimLeft(result.digits, "0")
	if result.negative && result.digits == "" {
		return result, fmt.Errorf("negative zero is forbidden")
	}
	return result, nil
}

func frozenExactInteger(token string) (int64, error) {
	parts, err := frozenNumberParts(token)
	if err != nil {
		return 0, err
	}
	if parts.digits == "" {
		return 0, nil
	}
	digits := parts.digits
	if parts.scale < 0 {
		cut := len(digits) + parts.scale
		if cut <= 0 || strings.Trim(digits[cut:], "0") != "" {
			return 0, fmt.Errorf("number must be an exact integer")
		}
		digits = digits[:cut]
	} else {
		if len(digits)+parts.scale > 16 {
			return 0, fmt.Errorf("integer exceeds the safe JSON range")
		}
		digits += strings.Repeat("0", parts.scale)
	}
	if len(digits) > 16 {
		return 0, fmt.Errorf("integer exceeds the safe JSON range")
	}
	value, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || value > frozenLevelMaxSafeInteger {
		return 0, fmt.Errorf("integer exceeds the safe JSON range")
	}
	if parts.negative {
		value = -value
	}
	return value, nil
}
