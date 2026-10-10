package dsl

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SequentialFullProfileID             = "seq.full.public_approx.v1"
	SequentialFullContractVersion       = "sequential-full-config-v1"
	SequentialFullUnsupportedConfigCode = "unsupported_config"
)

// SequentialFullSpec is the closed, single-route synthetic E1/E2 policy.
// Risk and the absolute notional ceiling are authored inputs, never study defaults.
type SequentialFullSpec struct {
	Profile, Policy, Symbol, Timeframe string
	RiskUSD, MaxNotionalUSD            float64
}

// SequentialFullConfigError is shared by source and direct-config admission.
// Code is stable across native and serialized diagnostics; Reason is explanatory.
type SequentialFullConfigError struct {
	Code   string `json:"code"`
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func (e *SequentialFullConfigError) Error() string {
	return fmt.Sprintf("%s: sequentialFull %s: %s", e.Code, e.Field, e.Reason)
}

func sequentialFullConfigError(field, reason string) error {
	return &SequentialFullConfigError{SequentialFullUnsupportedConfigCode, field, reason}
}

func sequentialFullIdentity(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

func sequentialFullMarker(s string) bool {
	id := sequentialFullIdentity(s)
	return strings.Contains(id, "sequentialfull") || strings.Contains(id, "seqfull")
}

// IsSequentialFullReserved detects malformed and relabeled family intent before
// generic defaults, empty-data and off-route shortcuts. Root metadata and
// well-shaped route labels are opaque; a full profile hidden in another
// family's object still reserves it.
func IsSequentialFullReserved(cfg Config) bool {
	var visit func(any, int) bool
	visit = func(value any, depth int) bool {
		// A cyclic or excessively nested hand-built config cannot establish a
		// safe fallback. The closed decoder rejects it without recursive decoding.
		if depth > 32 {
			return true
		}
		switch v := value.(type) {
		case string:
			return sequentialFullMarker(v)
		case FamilyID:
			return sequentialFullMarker(string(v))
		case Config:
			return visit(map[string]any(v), depth)
		case map[string]any:
			for key, child := range v {
				if sequentialFullMarker(key) {
					return true
				}
				if depth == 0 && (key == "name" || key == "description") {
					if text, ok := child.(string); ok && utf8.ValidString(text) {
						continue
					}
				}
				if depth == 0 && sequentialFullOpaqueRouteLabels(key, child) {
					continue
				}
				if sequentialFullIdentity(key) == "setuptype" {
					if sequentialFullDiscriminator(child) {
						return true
					}
				}
				if visit(child, depth+1) {
					return true
				}
			}
		case []any:
			for _, child := range v {
				if visit(child, depth+1) {
					return true
				}
			}
		case []string:
			for _, child := range v {
				if sequentialFullMarker(child) {
					return true
				}
			}
		default:
			// Direct native configs may contain named strings, typed maps or
			// typed slices instead of JSON's map[string]any and []any. They do
			// not create an escape hatch for reserved payloads.
			rv := reflect.ValueOf(value)
			if !rv.IsValid() {
				return false
			}
			switch rv.Kind() {
			case reflect.String:
				return sequentialFullMarker(rv.String())
			case reflect.Map:
				iter := rv.MapRange()
				for iter.Next() {
					key, child := iter.Key(), iter.Value().Interface()
					// Interface keys can contain strings or named string types.
					// Inspect each entry independently: normalizing a whole map
					// could collapse distinct native keys and lose a marker.
					for key.Kind() == reflect.Interface && !key.IsNil() {
						key = key.Elem()
					}
					if key.Kind() == reflect.String {
						if visit(map[string]any{key.String(): child}, depth+1) {
							return true
						}
					} else if visit(iter.Key().Interface(), depth+1) || visit(child, depth+1) {
						return true
					}
				}
			case reflect.Array, reflect.Slice:
				for i := 0; i < rv.Len(); i++ {
					if visit(rv.Index(i).Interface(), depth+1) {
						return true
					}
				}
			case reflect.Struct:
				for i := 0; i < rv.NumField(); i++ {
					field := rv.Field(i)
					if !field.CanInterface() {
						// Unknown inaccessible structural state cannot prove that
						// generic fallback is safe. Never use unsafe or Stringer.
						return true
					}
					info := rv.Type().Field(i)
					if visit(map[string]any{info.Name: field.Interface()}, depth+1) {
						return true
					}
					if tag := strings.Split(info.Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
						if visit(map[string]any{tag: field.Interface()}, depth+1) {
							return true
						}
					}
				}
			case reflect.Pointer, reflect.Interface:
				if !rv.IsNil() {
					return visit(rv.Elem().Interface(), depth+1)
				}
			}
		}
		return false
	}
	return visit(map[string]any(cfg), 0)
}

func sequentialFullDiscriminator(value any) bool {
	family := reflect.ValueOf(value)
	for depth := 0; family.IsValid(); depth++ {
		if depth > 32 {
			return true
		}
		if family.Kind() != reflect.Pointer && family.Kind() != reflect.Interface {
			return family.Kind() == reflect.String && strings.Contains(sequentialFullIdentity(family.String()), "sequential")
		}
		if family.IsNil() {
			return false
		}
		family = family.Elem()
	}
	return false
}

func sequentialFullOpaqueRouteLabels(key string, value any) bool {
	if key != "symbols" && key != "timeframes" && key != "slices" {
		return false
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false
	}
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()
		if key != "slices" {
			if text, ok := item.(string); !ok || text == "" {
				return false
			}
			continue
		}
		route, ok := item.(map[string]any)
		if !ok || len(route) != 2 {
			return false
		}
		if _, ok := route["symbol"].(string); !ok {
			return false
		}
		if _, ok := route["tf"].(string); !ok {
			return false
		}
	}
	return true
}

func sequentialFullExactKeys(m map[string]any, path string, keys ...string) error {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
		if _, ok := m[key]; !ok {
			return sequentialFullConfigError(path+"."+key, "required")
		}
	}
	unknown := []string{}
	for key := range m {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return sequentialFullConfigError(path+"."+unknown[0], "unknown field")
	}
	return nil
}

var sequentialFullSymbol = regexp.MustCompile(`^[A-Z][A-Z0-9_.-]{0,31}$`)

// SequentialFullTimeframeMS is the bounded synthetic route capability set.
// Fixed durations make no exchange-calendar or historical-data claim.
func SequentialFullTimeframeMS(timeframe string) (int64, bool) {
	minutes := int64(0)
	switch timeframe {
	case "1m":
		minutes = 1
	case "5m":
		minutes = 5
	case "15m":
		minutes = 15
	case "30m":
		minutes = 30
	case "1h":
		minutes = 60
	case "4h":
		minutes = 240
	case "1d":
		minutes = 1440
	default:
		return 0, false
	}
	return minutes * 60000, true
}

// DecodeSequentialFullConfig validates every field without generic defaults.
// Call it even when a caller supplies Config directly rather than parsing source.
func DecodeSequentialFullConfig(cfg Config) (SequentialFullSpec, error) {
	var spec SequentialFullSpec
	if err := sequentialFullExactKeys(map[string]any(cfg), "config", "dslVersion", "name", "description", "setupType", "sequentialFull"); err != nil {
		return spec, err
	}
	if err := regimeMatchFixed(cfg["dslVersion"], int64(7), "dslVersion"); err != nil {
		return spec, sequentialFullConfigError("dslVersion", err.Error())
	}
	family := cfg["setupType"]
	if id, ok := family.(FamilyID); ok {
		family = string(id)
	}
	if family != string(FamilySequentialFull) {
		return spec, sequentialFullConfigError("setupType", "must equal sequentialFull")
	}
	for _, key := range []string{"name", "description"} {
		value, err := frozenConfigString(cfg[key], key)
		if err != nil || key == "name" && strings.TrimSpace(value) == "" {
			return spec, sequentialFullConfigError(key, "require a Unicode string; strategy name must not be empty")
		}
	}
	m, err := frozenConfigObject(cfg["sequentialFull"], "sequentialFull")
	if err != nil {
		return spec, sequentialFullConfigError("sequentialFull", err.Error())
	}
	if err := sequentialFullExactKeys(m, "sequentialFull", "contractVersion", "profile", "policy", "symbol", "timeframe", "riskUsd", "maxNotionalUsd"); err != nil {
		return spec, err
	}
	if m["contractVersion"] != SequentialFullContractVersion {
		return spec, sequentialFullConfigError("sequentialFull.contractVersion", "must equal "+SequentialFullContractVersion)
	}
	for _, field := range []struct {
		key  string
		into *string
	}{{"profile", &spec.Profile}, {"policy", &spec.Policy}, {"symbol", &spec.Symbol}, {"timeframe", &spec.Timeframe}} {
		*field.into, err = frozenConfigString(m[field.key], field.key)
		if err != nil {
			return SequentialFullSpec{}, sequentialFullConfigError("sequentialFull."+field.key, err.Error())
		}
	}
	if spec.Profile != SequentialFullProfileID {
		return SequentialFullSpec{}, sequentialFullConfigError("sequentialFull.profile", "must equal "+SequentialFullProfileID)
	}
	if spec.Policy != "E1" && spec.Policy != "E2" {
		return SequentialFullSpec{}, sequentialFullConfigError("sequentialFull.policy", "only E1 and E2 are supported")
	}
	if !sequentialFullSymbol.MatchString(spec.Symbol) {
		return SequentialFullSpec{}, sequentialFullConfigError("sequentialFull.symbol", "require an explicit uppercase symbol identifier of 1 to 32 characters")
	}
	if _, ok := SequentialFullTimeframeMS(spec.Timeframe); !ok {
		return SequentialFullSpec{}, sequentialFullConfigError("sequentialFull.timeframe", "supported synthetic timeframes: 1m, 5m, 15m, 30m, 1h, 4h, 1d")
	}
	for _, field := range []struct {
		key  string
		into *float64
	}{{"riskUsd", &spec.RiskUSD}, {"maxNotionalUsd", &spec.MaxNotionalUSD}} {
		*field.into, err = frozenConfigDecimal(m[field.key], field.key)
		if err != nil {
			return SequentialFullSpec{}, sequentialFullConfigError("sequentialFull."+field.key, err.Error())
		}
	}
	return spec, nil
}

func (s SequentialFullSpec) projection() map[string]any {
	return map[string]any{
		"contractVersion": SequentialFullContractVersion, "profile": s.Profile, "policy": s.Policy,
		"symbol": s.Symbol, "timeframe": s.Timeframe, "riskUsd": s.RiskUSD, "maxNotionalUsd": s.MaxNotionalUSD,
	}
}

// DecodeSequentialFullConfigJSON additionally rejects duplicate decoded keys,
// malformed Unicode, trailing documents and non-finite or inexact fixed numbers.
func DecodeSequentialFullConfigJSON(raw []byte) (Config, error) {
	value, err := strictFrozenJSON(raw)
	if err != nil {
		return nil, sequentialFullConfigError("config", err.Error())
	}
	m, err := frozenConfigObject(value, "config")
	if err != nil {
		return nil, sequentialFullConfigError("config", err.Error())
	}
	spec, err := DecodeSequentialFullConfig(Config(m))
	if err != nil {
		return nil, err
	}
	return Config{"dslVersion": int64(7), "name": m["name"], "description": m["description"], "setupType": string(FamilySequentialFull), "sequentialFull": spec.projection()}, nil
}

// SequentialFullParseError preserves the native refusal type when execution
// receives ordinary parse diagnostics. Unrelated parser failures return nil.
func SequentialFullParseError(result ParseResult) error {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == DiagnosticError && diagnostic.Code == SequentialFullUnsupportedConfigCode {
			return sequentialFullConfigError("source", diagnostic.Message)
		}
	}
	return nil
}
