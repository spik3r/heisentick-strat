package dsl

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	// GoldFlagReferenceDedicatedRunnerRequired marks the native offline boundary.
	GoldFlagReferenceDedicatedRunnerRequired = "gold-flag-reference-native-dedicated-runner-required"
	GoldFlagReferencePolicy                  = "PR388_CAUSAL_STRESS_V1"
	goldFlagReferenceContractVersion         = "gold-flag-reference-config-v1"
)

// GoldFlagReferenceSpec identifies one indivisible policy, with no tunable fields.
type GoldFlagReferenceSpec struct{ Policy string }

// GoldFlagReferenceProfile returns a fresh, complete closed policy projection.
// Diagnostic costs are report inputs; they never modify execution or sizing.
func GoldFlagReferenceProfile() map[string]any {
	return map[string]any{
		"id": GoldFlagReferencePolicy, "symbol": "XAUUSD",
		"sourceTimeframe": "M15", "nativeTimeframe": "M30",
		"clock": map[string]any{
			"selection": "explicit-half-open-M30-aligned-range-before-aggregation", "preStartHistory": false,
			"timezone": "UTC", "nativePhaseMinutes": int64(0),
			"sourceRows": "strict-unique-exact-M15-opens", "aggregation": "first-max-min-last-sum",
			"interpolation": false, "partialBars": "retain", "wholeMissingBars": "skip-without-reset",
			"availability": "nominal-bucket-close", "decision": "M30-nominal-close",
			"missingDataCancelsPending": false, "entryTimestamp": "first-actually-observed-M15-open",
		},
		"atr": map[string]any{
			"length": int64(14), "smoothing": "wilder-rma", "seed": "mean-TR-indices-1-through-14",
			"firstValidIndex": int64(14), "trueRange": "max-high-low-abs-high-prevClose-abs-low-prevClose",
			"includeCurrent": true,
		},
		"pattern": map[string]any{
			"poleBars": int64(6), "flagBars": int64(6), "extremaTies": "first-observed-occurrence",
			"poleWindow": "six-observed-rows-immediately-before-flag", "flagWindow": "six-observed-rows-ending-at-signal",
			"poleMinimumATR": int64(2), "flagMaximumPoleFraction": 0.6,
			"flagMaximumATR": 1.5, "retraceMaximumPoleFraction": 0.5,
			"thresholds": "inclusive", "direction": "pole-extrema-order",
			"long":  "first-pole-low-before-first-pole-high-and-flagLow-at-least-poleHigh-minus-half-pole",
			"short": "first-pole-high-before-first-pole-low-and-flagHigh-at-most-poleLow-plus-half-pole",
		},
		"context": map[string]any{
			"source": "observed-M30", "aggregation": "first-max-min-last-sum",
			"h4":        map[string]any{"lookback": int64(12), "phaseHoursUTC": int64(0), "availability": "bucket-start-plus-4h"},
			"daily":     map[string]any{"lookback": int64(5), "phaseHoursUTC": int64(0), "availability": "next-UTC-midnight"},
			"selection": "latest-nominal-close-less-or-equal-decision-close", "partialBuckets": "retain-at-nominal-close",
			"gate":  "either-qualifying-directional-extreme",
			"long":  "flagHigh-greater-or-equal-contextHigh-minus-signalATR",
			"short": "flagLow-less-or-equal-contextLow-plus-signalATR", "distanceMaximumATR": int64(1),
			"deATRFilter": false, "volumeFilter": false,
		},
		"signal": map[string]any{
			"entryBufferATR": 0.1, "oppositeStopBufferATR": 0.1, "minimumRiskATR": 0.4,
			"atrAnchor": "frozen-signal-ATR", "flatOnly": true, "onePosition": true,
			"cooldownObservedRows": int64(6), "cooldownIncludesInvalidRisk": true,
			"nextEligibleSignal": "prior-signal-index-plus-7",
			"warmupSignals":      "do-not-carry", "endCloseNewEntry": false,
		},
		"pending": map[string]any{
			"activation": "next-observed-row", "expiryObservedRows": int64(4),
			"eligibleOffsets":         "signal-plus-1-through-signal-plus-4-inclusive",
			"missingDataCancellation": false, "terminal": "retain-pending",
		},
		"execution": map[string]any{
			"referenceUnits": int64(1), "equityFeedback": false, "initialBracket": "attached-at-fill", "stopUpdates": "none",
			"openingGap":      "open-before-intrabar-at-actual-entry",
			"openingOpposite": "cancel-before-later-intrabar-entry", "intrabarOppositeOnly": "cancel", "risk": "actual-fill-to-fixed-signal-stop",
			"targetR": int64(2), "targetAnchor": "actual-fill", "openingTarget": "cap-at-target",
			"ambiguousPolicy": "entry-first-stop-first",
			"scenario":        "deterministic-coarse-OHLC-stress-scenario-not-observed-execution", "feasibleAlternatives": "retain",
			"timeExitObservedRows": int64(24), "timeExitIndex": "fill-index-plus-24",
			"timeExitPriority": "bracket-before-time-exit", "exitRow": "skip-new-signal",
			"terminalPosition": "retain-open", "terminalLiquidation": false,
			"priceArithmetic": "continuous-float64-reference", "tickRounding": false,
		},
		"evidence": map[string]any{
			"observedOpening": "point-[t,t]", "provenNonopening": "open-interval-(observedOpen,lastObservedClose)",
			"coarselySavedZeroGapFill": "half-open-[observedOpen,lastObservedClose)",
			"nominalClose":             "point-[close,close]", "latency": "unverified",
			"alternatives": "local-OHLC-feasible-not-propagated-portfolio-bounds",
		},
		"costs": map[string]any{
			"purpose": "diagnostic-stress-only", "selection": "explicit-external-report-request",
			"unit": "points-per-fill", "values": []any{int64(0), 0.06, 0.15, 0.25, 0.5},
			"effectsOnFillsBracketsRiskSignalsSizing": false,
		},
		"routes": map[string]any{"nativeOffline": true, "generic": false, "grid": false, "prefix": false, "wasm": false},
	}
}

func (s GoldFlagReferenceSpec) projection() map[string]any {
	return map[string]any{"contractVersion": goldFlagReferenceContractVersion, "policy": s.Policy, "profile": GoldFlagReferenceProfile()}
}

// IsGoldFlagReferenceReserved detects family/object intent before generic
// defaults, dispatch or empty-data shortcuts. Malformed aliases remain reserved
// but are never accepted by DecodeGoldFlagReference.
func IsGoldFlagReferenceReserved(cfg Config) bool {
	for key, value := range cfg {
		if goldFlagIdentity(key) == "goldflagreference" {
			return true
		}
		if goldFlagIdentity(key) != "setuptype" {
			continue
		}
		var family string
		switch v := value.(type) {
		case string:
			family = v
		case FamilyID:
			family = string(v)
		}
		if goldFlagIdentity(family) == "goldflagreference" {
			return true
		}
	}
	return false
}

// DecodeGoldFlagReference validates the complete closed configuration without
// generic defaults. Raw JSON must use DecodeGoldFlagReferenceConfigJSON before
// map conversion to retain duplicate-key and exact-number checks.
func DecodeGoldFlagReference(cfg Config) (GoldFlagReferenceSpec, error) {
	var spec GoldFlagReferenceSpec
	if err := frozenExactKeys(map[string]any(cfg), "config", "dslVersion", "name", "description", "setupType", "goldFlagReference"); err != nil {
		return spec, err
	}
	if err := regimeMatchFixed(cfg["dslVersion"], int64(7), "dslVersion"); err != nil {
		return spec, err
	}
	family := cfg["setupType"]
	if id, ok := family.(FamilyID); ok {
		family = string(id)
	}
	if family != string(FamilyGoldFlagReference) {
		return spec, fmt.Errorf("setupType must equal goldFlagReference")
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
	m, err := frozenConfigObject(cfg["goldFlagReference"], "goldFlagReference")
	if err != nil {
		return spec, err
	}
	if err := frozenExactKeys(m, "goldFlagReference", "contractVersion", "policy", "profile"); err != nil {
		return spec, err
	}
	if err := regimeMatchFixed(m["contractVersion"], goldFlagReferenceContractVersion, "goldFlagReference.contractVersion"); err != nil {
		return spec, err
	}
	spec.Policy, err = frozenConfigString(m["policy"], "goldFlagReference.policy")
	if err != nil {
		return spec, err
	}
	if spec.Policy != GoldFlagReferencePolicy {
		return GoldFlagReferenceSpec{}, fmt.Errorf("goldFlagReference.policy must equal %s", GoldFlagReferencePolicy)
	}
	if err := goldFlagMatchFixed(m["profile"], GoldFlagReferenceProfile(), "goldFlagReference.profile"); err != nil {
		return GoldFlagReferenceSpec{}, err
	}
	return spec, nil
}

// DecodeGoldFlagReferenceConfigJSON rejects duplicate decoded keys, malformed
// Unicode, trailing documents and numeric mutations before float conversion.
func DecodeGoldFlagReferenceConfigJSON(raw []byte) (Config, error) {
	value, err := strictFrozenJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("gold flag reference config: %w", err)
	}
	m, err := frozenConfigObject(value, "config")
	if err != nil {
		return nil, err
	}
	spec, err := DecodeGoldFlagReference(Config(m))
	if err != nil {
		return nil, err
	}
	return Config{"dslVersion": int64(7), "name": m["name"], "description": m["description"], "setupType": string(FamilyGoldFlagReference), "goldFlagReference": spec.projection()}, nil
}

// goldFlagIdentity reserves malformed spellings without accepting them as aliases.
func goldFlagIdentity(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' || r == '_' {
			return -1
		}
		return r
	}, strings.ToLower(value))
}

// goldFlagMatchFixed extends the shared exact scalar matcher only for fixed
// arrays, retaining exact keys and types at every nested projection level.
func goldFlagMatchFixed(value, fixed any, path string) error {
	switch expected := fixed.(type) {
	case map[string]any:
		actual, err := frozenConfigObject(value, path)
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(expected))
		for key := range expected {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if err := frozenExactKeys(actual, path, keys...); err != nil {
			return err
		}
		for _, key := range keys {
			if err := goldFlagMatchFixed(actual[key], expected[key], path+"."+key); err != nil {
				return err
			}
		}
		return nil
	case []any:
		actual, ok := value.([]any)
		if !ok || len(actual) != len(expected) {
			return fmt.Errorf("%s must match the fixed array", path)
		}
		for i := range expected {
			if err := goldFlagMatchFixed(actual[i], expected[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	default:
		return regimeMatchFixed(value, fixed, path)
	}
}
