package dsl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	// RegimeEngineDedicatedRunnerRequired marks the native offline execution boundary.
	RegimeEngineDedicatedRunnerRequired = "regime-engine-native-dedicated-runner-required"
	RegimeEngineSourceLikeV1            = "source-like-v1"
	RegimeEngineAuditBaselineV1         = "audit-baseline-v1"
	regimeEngineContractVersion         = "regime-engine-config-v1"
)

// RegimeEngineSpec is the closed execution policy. Mode is the only variable
// policy setting; all signal, sizing and terminal rules are fixed by v1.
type RegimeEngineSpec struct{ Mode string }

// RegimeEngineProfile returns an independent projection of the fixed v1 rules.
// No caller can mutate a later parse or validation through this map.
func RegimeEngineProfile() map[string]any {
	return map[string]any{
		"id": "v9-floor-half-v1", "symbol": "XAUUSD",
		"sourceTimeframe": "M5", "nativeTimeframe": "M30",
		"hma":        map[string]any{"fast": int64(9), "slow": int64(21), "diagnostic": int64(25), "halfLength": "floor", "sqrtLength": "round-half-up"},
		"supertrend": map[string]any{"atrLength": int64(10), "factor": int64(3), "smoothing": "wilder-rma-sma-seed"},
		"volume":     map[string]any{"smaLength": int64(20), "comparison": "strict-greater", "includeCurrent": true},
		"atr":        map[string]any{"length": int64(14), "smoothing": "wilder-rma-sma-seed"},
		"stopATR":    int64(2), "targetATR": 3.5,
		"completeObservedBars": int64(40), "completeness": "six-exact-M5-slots",
		"notionalFraction": 0.1, "pointValue": int64(1), "quantity": "continuous",
		"feePerUnitSide": 0.5, "terminal": "keep-open",
	}
}

func (s RegimeEngineSpec) projection() map[string]any {
	return map[string]any{"contractVersion": regimeEngineContractVersion, "mode": s.Mode, "profile": RegimeEngineProfile()}
}

// IsRegimeEngineReserved identifies intent before validation. A reserved key
// still belongs to this family when null, scalar, case-varied, or attached to an
// old setupType. Discriminator aliases reserve the family but are not accepted
// by the closed decoder. Call this before generic defaults or preparation.
func IsRegimeEngineReserved(cfg Config) bool {
	for key, value := range cfg {
		if regimeIdentity(key) == "regimeengine" {
			return true
		}
		if regimeIdentity(key) != "setuptype" {
			continue
		}
		var family string
		switch v := value.(type) {
		case string:
			family = v
		case FamilyID:
			family = string(v)
		}
		if regimeIdentity(family) == "regimeengine" {
			return true
		}
	}
	return false
}

func regimeIdentity(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n', '-', '_':
			return -1
		}
		return r
	}, strings.ToLower(s))
}

// DecodeRegimeEngine validates the entire closed config without inheriting any
// generic-engine defaults. For raw JSON use DecodeRegimeEngineConfigJSON first
// to detect duplicate keys and preserve numbers before map conversion.
func DecodeRegimeEngine(cfg Config) (RegimeEngineSpec, error) {
	var spec RegimeEngineSpec
	if err := frozenExactKeys(map[string]any(cfg), "config", "dslVersion", "name", "description", "setupType", "regimeEngine"); err != nil {
		return spec, err
	}
	if err := regimeMatchFixed(cfg["dslVersion"], int64(7), "dslVersion"); err != nil {
		return spec, err
	}
	family := cfg["setupType"]
	if id, ok := family.(FamilyID); ok {
		family = string(id)
	}
	if family != string(FamilyRegimeEngine) {
		return spec, fmt.Errorf("setupType must equal regimeEngine")
	}
	name, err := frozenConfigString(cfg["name"], "name")
	if err != nil {
		return spec, err
	}
	if name == "" {
		return spec, fmt.Errorf("name must not be empty")
	}
	if _, err := frozenConfigString(cfg["description"], "description"); err != nil {
		return spec, err
	}
	m, err := frozenConfigObject(cfg["regimeEngine"], "regimeEngine")
	if err != nil {
		return spec, err
	}
	if err := frozenExactKeys(m, "regimeEngine", "contractVersion", "mode", "profile"); err != nil {
		return spec, err
	}
	if err := regimeMatchFixed(m["contractVersion"], regimeEngineContractVersion, "regimeEngine.contractVersion"); err != nil {
		return spec, err
	}
	spec.Mode, err = frozenConfigString(m["mode"], "regimeEngine.mode")
	if err != nil {
		return spec, err
	}
	switch spec.Mode {
	case RegimeEngineSourceLikeV1, RegimeEngineAuditBaselineV1:
	default:
		return RegimeEngineSpec{}, fmt.Errorf("regimeEngine.mode must be source-like-v1 or audit-baseline-v1")
	}
	if err := regimeMatchFixed(m["profile"], RegimeEngineProfile(), "regimeEngine.profile"); err != nil {
		return RegimeEngineSpec{}, err
	}
	return spec, nil
}

// DecodeRegimeEngineConfigJSON rejects duplicate decoded keys, unknown/null
// fields, invalid Unicode, trailing documents and inexact numeric mutations.
// The shared strict JSON reader does not run either family's parser or defaults.
func DecodeRegimeEngineConfigJSON(raw []byte) (Config, error) {
	value, err := strictFrozenJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("regime engine config: %w", err)
	}
	m, err := frozenConfigObject(value, "config")
	if err != nil {
		return nil, err
	}
	spec, err := DecodeRegimeEngine(Config(m))
	if err != nil {
		return nil, err
	}
	return Config{"dslVersion": int64(7), "name": m["name"], "description": m["description"], "setupType": string(FamilyRegimeEngine), "regimeEngine": spec.projection()}, nil
}

func regimeMatchFixed(value, fixed any, path string) error {
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
			if err := regimeMatchFixed(actual[k], expected[k], path+"."+k); err != nil {
				return err
			}
		}
		return nil
	case string:
		if actual, ok := value.(string); ok && actual == expected {
			return nil
		}
	case bool:
		if actual, ok := value.(bool); ok && actual == expected {
			return nil
		}
	case int64, float64:
		// Compare decimal values before any float conversion. Thus a raw JSON
		// 14.000000000000000001 cannot silently turn into the fixed length 14.
		actual, err := regimeNumberParts(value)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		want, _ := regimeNumberParts(fixed)
		if actual == want {
			return nil
		}
	}
	return fmt.Errorf("%s must match the fixed v1 value and type", path)
}

func regimeNumberParts(value any) (frozenDecimalParts, error) {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
	default:
		return frozenDecimalParts{}, fmt.Errorf("expected a finite JSON number")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return frozenDecimalParts{}, err
	}
	parts, err := frozenNumberParts(string(raw))
	if err != nil {
		return parts, err
	}
	for strings.HasSuffix(parts.digits, "0") {
		parts.digits = strings.TrimSuffix(parts.digits, "0")
		parts.scale++
	}
	return parts, nil
}
