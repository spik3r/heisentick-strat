package dsl

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"unicode/utf8"
)

const (
	// FrozenLevelExecutionUnimplemented is the stable compile-only diagnostic.
	FrozenLevelExecutionUnimplemented       = "frozen-level-quote-execution-unimplemented"
	frozenLevelContractVersion              = "frozen-level-compiler-v1"
	frozenLevelControlProfile               = "stored-m30-lock-isolation-v1"
	frozenLevelMaxSafeInteger         int64 = 9007199254740991
	frozenLevelMaxTimeoutMinutes      int64 = 150119987579
)

// FrozenLevelInputRef identifies immutable payload bytes. Compilation checks
// structure only; it neither fetches the payload nor qualifies it for a run.
type FrozenLevelInputRef struct {
	ID      string
	Version string
	SHA256  string
}

// FrozenLevelInputRefs is the complete set of mandatory input identities.
type FrozenLevelInputRefs struct {
	Source, Calendar, Ordering, Schedule, Costs, Assumptions FrozenLevelInputRef
}

// FrozenLevelSpec is the typed bridge for the variable settings. Its map
// projection always supplies the validated, immutable contract and profile.
type FrozenLevelSpec struct {
	LockMode            string
	ActivationR         float64
	TimeoutMinutes      int64
	PriceGrid           float64
	MaxPredecessorAgeMS int64
	MaxReceiptAgeMS     int64
	EntryLatencyMS      int64
	AmendmentLatencyMS  int64
	InputRefs           FrozenLevelInputRefs
}

// FrozenLevelIdentityResult contains derived data outside the hashed config.
// Bytes are encoding/json.Marshal output, without whitespace or a final LF.
type FrozenLevelIdentityResult struct {
	ConfigBytes  []byte
	PolicyBytes  []byte
	ConfigDigest string
	PolicyDigest string
}

// This reviewed constant is copied from design candidate
// 749a511c589a33b19064ab6d73f6da6be70ce05dd1df20adbb6cda1d7198e754.
//
//go:embed frozen_level_profile.json
var frozenLevelProfileJSON []byte

var frozenLevelProfileValue = func() map[string]any {
	value, err := strictFrozenJSON(frozenLevelProfileJSON)
	if err != nil {
		panic("invalid frozen-level constant profile: " + err.Error())
	}
	return frozenProfileConstant(value).(map[string]any)
}()

func frozenProfileConstant(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			out[k] = frozenProfileConstant(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = frozenProfileConstant(child)
		}
		return out
	case json.Number:
		n, err := frozenExactInteger(string(v))
		if err != nil {
			panic("invalid frozen-level constant integer: " + err.Error())
		}
		return n
	default:
		return value
	}
}

// FrozenLevelProfile returns an independent deep copy of the fixed profile.
func FrozenLevelProfile() map[string]any {
	return frozenCloneValue(frozenLevelProfileValue).(map[string]any)
}

func frozenCloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			out[k] = frozenCloneValue(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = frozenCloneValue(child)
		}
		return out
	default:
		return value
	}
}

// NormalizeFrozenLevelConfig validates the exact five-key root and projects
// it through a typed spec into independent maps. Integer fields intentionally
// reject float32/float64: a previous unsafe decode may have rounded a fraction.
// Raw JSON callers must use DecodeFrozenLevelConfigJSON instead of Unmarshal.
func NormalizeFrozenLevelConfig(cfg Config) (Config, error) {
	if err := frozenExactKeys(cfg, "config", "dslVersion", "setupType", "name", "description", "frozenLevelBreakout"); err != nil {
		return nil, err
	}
	version, err := frozenConfigInteger(cfg["dslVersion"], "dslVersion", 7, 7)
	if err != nil {
		return nil, err
	}
	family := cfg["setupType"]
	if typed, ok := family.(FamilyID); ok {
		family = string(typed)
	}
	if value, ok := family.(string); !ok || value != string(FamilyFrozenLevelBreakout) {
		return nil, fmt.Errorf("setupType must be frozenLevelBreakout")
	}
	name, err := frozenConfigString(cfg["name"], "name")
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("name must not be empty")
	}
	description, err := frozenConfigString(cfg["description"], "description")
	if err != nil {
		return nil, err
	}
	spec, err := frozenDecodeSpec(cfg["frozenLevelBreakout"])
	if err != nil {
		return nil, err
	}
	return Config{
		"dslVersion":          version,
		"setupType":           string(FamilyFrozenLevelBreakout),
		"name":                name,
		"description":         description,
		"frozenLevelBreakout": spec.projection(),
	}, nil
}

func frozenDecodeSpec(value any) (FrozenLevelSpec, error) {
	var spec FrozenLevelSpec
	m, err := frozenConfigObject(value, "frozenLevelBreakout")
	if err != nil {
		return spec, err
	}
	if err := frozenExactKeys(m, "frozenLevelBreakout", "contractVersion", "controlProfile", "profile", "lockMode", "activationR", "timeoutMinutes", "priceGrid", "maxPredecessorAgeMS", "maxReceiptAgeMS", "entryLatencyMS", "amendmentLatencyMS", "inputRefs"); err != nil {
		return spec, err
	}
	for _, field := range []struct{ key, expected string }{
		{"contractVersion", frozenLevelContractVersion}, {"controlProfile", frozenLevelControlProfile},
	} {
		if v, ok := m[field.key].(string); !ok || v != field.expected {
			return spec, fmt.Errorf("%s must equal %s", field.key, field.expected)
		}
	}
	if err := frozenMatchFixed(m["profile"], frozenLevelProfileValue, "profile"); err != nil {
		return spec, err
	}
	if spec.LockMode, err = frozenConfigString(m["lockMode"], "lockMode"); err != nil {
		return spec, err
	}
	switch spec.LockMode {
	case "none", "scale", "pivot":
	default:
		return spec, fmt.Errorf("lockMode must be none, scale, or pivot")
	}
	for _, field := range []struct {
		key  string
		into *float64
	}{
		{"activationR", &spec.ActivationR}, {"priceGrid", &spec.PriceGrid},
	} {
		if *field.into, err = frozenConfigDecimal(m[field.key], field.key); err != nil {
			return spec, err
		}
	}
	for _, field := range []struct {
		key      string
		into     *int64
		min, max int64
	}{
		{"timeoutMinutes", &spec.TimeoutMinutes, 1, frozenLevelMaxTimeoutMinutes},
		{"maxPredecessorAgeMS", &spec.MaxPredecessorAgeMS, 1, frozenLevelMaxSafeInteger},
		{"maxReceiptAgeMS", &spec.MaxReceiptAgeMS, 1, frozenLevelMaxSafeInteger},
		{"entryLatencyMS", &spec.EntryLatencyMS, 0, frozenLevelMaxSafeInteger},
		{"amendmentLatencyMS", &spec.AmendmentLatencyMS, 0, frozenLevelMaxSafeInteger},
	} {
		if *field.into, err = frozenConfigInteger(m[field.key], field.key, field.min, field.max); err != nil {
			return spec, err
		}
	}
	refs, err := frozenConfigObject(m["inputRefs"], "inputRefs")
	if err != nil {
		return spec, err
	}
	if err := frozenExactKeys(refs, "inputRefs", "source", "calendar", "ordering", "schedule", "costs", "assumptions"); err != nil {
		return spec, err
	}
	for _, field := range []struct {
		key  string
		into *FrozenLevelInputRef
	}{
		{"source", &spec.InputRefs.Source}, {"calendar", &spec.InputRefs.Calendar},
		{"ordering", &spec.InputRefs.Ordering}, {"schedule", &spec.InputRefs.Schedule},
		{"costs", &spec.InputRefs.Costs}, {"assumptions", &spec.InputRefs.Assumptions},
	} {
		if *field.into, err = frozenDecodeRef(refs[field.key], "inputRefs."+field.key); err != nil {
			return spec, err
		}
	}
	return spec, nil
}

func (spec FrozenLevelSpec) projection() map[string]any {
	return map[string]any{
		"contractVersion": frozenLevelContractVersion, "controlProfile": frozenLevelControlProfile,
		"profile": FrozenLevelProfile(), "lockMode": spec.LockMode,
		"activationR": spec.ActivationR, "timeoutMinutes": spec.TimeoutMinutes, "priceGrid": spec.PriceGrid,
		"maxPredecessorAgeMS": spec.MaxPredecessorAgeMS, "maxReceiptAgeMS": spec.MaxReceiptAgeMS,
		"entryLatencyMS": spec.EntryLatencyMS, "amendmentLatencyMS": spec.AmendmentLatencyMS,
		"inputRefs": map[string]any{
			"source": spec.InputRefs.Source.projection(), "calendar": spec.InputRefs.Calendar.projection(),
			"ordering": spec.InputRefs.Ordering.projection(), "schedule": spec.InputRefs.Schedule.projection(),
			"costs": spec.InputRefs.Costs.projection(), "assumptions": spec.InputRefs.Assumptions.projection(),
		},
	}
}

func (ref FrozenLevelInputRef) projection() map[string]any {
	return map[string]any{"id": ref.ID, "version": ref.Version, "sha256": ref.SHA256}
}

func frozenDecodeRef(value any, path string) (FrozenLevelInputRef, error) {
	var ref FrozenLevelInputRef
	m, err := frozenConfigObject(value, path)
	if err != nil {
		return ref, err
	}
	if err := frozenExactKeys(m, path, "id", "version", "sha256"); err != nil {
		return ref, err
	}
	for _, field := range []struct {
		key  string
		into *string
		max  int
	}{
		{"id", &ref.ID, 64}, {"version", &ref.Version, 32},
	} {
		v, err := frozenConfigString(m[field.key], path+"."+field.key)
		if err != nil {
			return ref, err
		}
		if len(v) < 1 || len(v) > field.max {
			return ref, fmt.Errorf("%s.%s has an invalid length", path, field.key)
		}
		for i := 0; i < len(v); i++ {
			b := v[i]
			alphanumeric := b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
			if !alphanumeric && (i == 0 || (b != '.' && b != '_' && b != '-')) {
				return ref, fmt.Errorf("%s.%s must be an ASCII registry identity", path, field.key)
			}
		}
		*field.into = v
	}
	ref.SHA256, err = frozenConfigString(m["sha256"], path+".sha256")
	if err != nil {
		return ref, err
	}
	if len(ref.SHA256) != 64 {
		return ref, fmt.Errorf("%s.sha256 must be 64 lowercase hex characters", path)
	}
	for _, b := range []byte(ref.SHA256) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return ref, fmt.Errorf("%s.sha256 must be 64 lowercase hex characters", path)
		}
	}
	return ref, nil
}

func frozenConfigObject(value any, path string) (map[string]any, error) {
	switch m := value.(type) {
	case map[string]any:
		if m != nil {
			return m, nil
		}
	case Config:
		if m != nil {
			return map[string]any(m), nil
		}
	}
	return nil, fmt.Errorf("%s must be an object", path)
}

func frozenExactKeys(m map[string]any, path string, keys ...string) error {
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			return fmt.Errorf("%s.%s is required", path, key)
		}
	}
	if len(m) != len(keys) {
		return fmt.Errorf("%s contains unknown fields", path)
	}
	return nil
}

func frozenConfigString(value any, path string) (string, error) {
	v, ok := value.(string)
	if !ok || !utf8.ValidString(v) {
		return "", fmt.Errorf("%s must be a well-formed Unicode string", path)
	}
	return v, nil
}

func frozenConfigInteger(value any, path string, min, max int64) (int64, error) {
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int8:
		n = int64(v)
	case int16:
		n = int64(v)
	case int32:
		n = int64(v)
	case int64:
		n = v
	case uint:
		if uint64(v) > uint64(frozenLevelMaxSafeInteger) {
			return 0, fmt.Errorf("%s exceeds the safe JSON integer range", path)
		}
		n = int64(v)
	case uint8:
		n = int64(v)
	case uint16:
		n = int64(v)
	case uint32:
		n = int64(v)
	case uint64:
		if v > uint64(frozenLevelMaxSafeInteger) {
			return 0, fmt.Errorf("%s exceeds the safe JSON integer range", path)
		}
		n = int64(v)
	case json.Number:
		var err error
		n, err = frozenExactInteger(string(v))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
	default:
		return 0, fmt.Errorf("%s requires a lossless integer type or json.Number", path)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("%s must be an integer in [%d, %d]", path, min, max)
	}
	return n, nil
}

func frozenConfigDecimal(value any, path string) (float64, error) {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case json.Number:
		if _, err := frozenNumberParts(string(v)); err != nil {
			return 0, fmt.Errorf("%s: %w", path, err)
		}
		var err error
		n, err = strconv.ParseFloat(string(v), 64)
		if err != nil {
			return 0, fmt.Errorf("%s is outside finite float64 range", path)
		}
	default:
		integer, err := frozenConfigInteger(value, path, 1, frozenLevelMaxSafeInteger)
		if err != nil {
			return 0, err
		}
		n = float64(integer)
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0, fmt.Errorf("%s must normalize to a finite positive float64", path)
	}
	return n, nil
}

func frozenMatchFixed(value, fixed any, path string) error {
	switch expected := fixed.(type) {
	case map[string]any:
		actual, err := frozenConfigObject(value, path)
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(expected))
		for k := range expected {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if err := frozenExactKeys(actual, path, keys...); err != nil {
			return err
		}
		for _, k := range keys {
			if err := frozenMatchFixed(actual[k], expected[k], path+"."+k); err != nil {
				return err
			}
		}
		return nil
	case []any:
		actual, ok := value.([]any)
		if !ok || len(actual) != len(expected) {
			return fmt.Errorf("%s must match the fixed profile array", path)
		}
		for i := range expected {
			if err := frozenMatchFixed(actual[i], expected[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case int64:
		_, err := frozenConfigInteger(value, path, expected, expected)
		return err
	case string:
		if actual, ok := value.(string); ok && actual == expected {
			return nil
		}
	case bool:
		if actual, ok := value.(bool); ok && actual == expected {
			return nil
		}
	}
	return fmt.Errorf("%s must match the fixed profile value and type", path)
}

// FrozenLevelIdentity validates before hashing. Policy identity excludes only
// strategy metadata and the entire assumptions reference, preventing a cycle
// when that reference's payload binds the policy digest.
func FrozenLevelIdentity(cfg Config) (FrozenLevelIdentityResult, error) {
	var result FrozenLevelIdentityResult
	normalized, err := NormalizeFrozenLevelConfig(cfg)
	if err != nil {
		return result, err
	}
	configBytes, err := json.Marshal(normalized)
	if err != nil {
		return result, err
	}
	// This spec belongs to our independent projection, never to the caller.
	spec := normalized["frozenLevelBreakout"].(map[string]any)
	delete(spec["inputRefs"].(map[string]any), "assumptions")
	policy := map[string]any{
		"dslVersion": normalized["dslVersion"], "setupType": normalized["setupType"],
		"frozenLevelBreakout": spec,
	}
	policyBytes, err := json.Marshal(policy)
	if err != nil {
		return result, err
	}
	configHash, policyHash := sha256.Sum256(configBytes), sha256.Sum256(policyBytes)
	return FrozenLevelIdentityResult{
		ConfigBytes: configBytes, PolicyBytes: policyBytes,
		ConfigDigest: hex.EncodeToString(configHash[:]), PolicyDigest: hex.EncodeToString(policyHash[:]),
	}, nil
}
