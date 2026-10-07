package dsl

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	// MasterStructuralDedicatedRunnerRequired marks the native offline boundary.
	MasterStructuralDedicatedRunnerRequired   = "master-structural-native-dedicated-runner-required"
	MasterStructuralSourceHistoricalReference = "SOURCE_HISTORICAL_REFERENCE"
	MasterStructuralProtectedStableReference  = "PROTECTED_STABLE_REFERENCE"
	masterStructuralContractVersion           = "master-structural-config-v1"
)

// MasterStructuralSpec is the closed v10 execution policy. The two modes are
// indivisible policy bundles, not independent H4 and management settings.
type MasterStructuralSpec struct{ Mode string }

// MasterStructuralProfile returns an independent fixed projection for mode.
// An unsupported mode has no profile and returns nil. No caller can mutate a
// later parse through the returned map. All prices are continuous reference
// float64 values: the source Pine's mintick rounding is deliberately excluded.
func MasterStructuralProfile(mode string) map[string]any {
	var horizon string
	var lifecycle map[string]any
	switch mode {
	case MasterStructuralSourceHistoricalReference:
		horizon = "M30-decision-close"
		lifecycle = map[string]any{
			"anchor": "signal-close", "atr": "current-ATR14",
			"extrema":        "signal-and-post-fill-separately",
			"initialBracket": "after-entry-bar-close", "entryBarProtection": false,
			"activation": "non-sticky-current-ATR", "activationAnchor": "signal-close",
			"preActivationStop": "directional-best-of-anchor-2ATR-and-chart-ST",
			"activatedStop":     "bare-chart-ST", "stopRatchet": false,
			"target":         "signal-close-plus-directional-4-current-ATR",
			"regimeFlipExit": "none", "wrongSideStop": "next-observed-open-market-exit",
			"marketableTarget": "non-latched-next-open-or-path-limit",
		}
	case MasterStructuralProtectedStableReference:
		horizon = "M30-bar-open"
		lifecycle = map[string]any{
			"anchor": "actual-fill", "atr": "frozen-signal-ATR14",
			"extrema":        "post-fill-only",
			"initialBracket": "attached-to-entry-translated-from-fill", "entryBarProtection": true,
			"initialStopDistance": "min-2-signal-ATR-positive-directional-signal-close-to-ST-else-2-signal-ATR",
			"activation":          "sticky-frozen-signal-ATR", "activationAnchor": "actual-fill",
			"preActivationStop": "unchanged-initial-stop",
			"activatedStop":     "same-regime-proper-side-chart-ST", "stopRatchet": true,
			"target":                    "actual-fill-plus-directional-4-signal-ATR-fixed",
			"regimeFlipExit":            "completed-adverse-transition-next-observed-open",
			"regimeFlipExitReason":      "regime_flip_market_exit",
			"pendingRegimeExitBracket":  "remains-live",
			"pendingForcedExitPriority": "before-opening-bracket",
		}
	default:
		return nil
	}
	return map[string]any{
		"id": "v10-phase0-floor-half-reference-v1", "symbol": "XAUUSD",
		"sourceTimeframe": "M5", "nativeTimeframe": "M30",
		"clock": map[string]any{
			"timezone": "UTC", "nativePhaseMinutes": int64(0), "sourceRows": "strict-unique-exact-M5-opens",
			"aggregation": "first-max-min-last-sum", "interpolation": false,
			"partialBars": "retain-in-indicator-history", "wholeMissingBars": "do-not-reset-observed-readiness",
			"warmupTradeEnd": "strictly-ordered-exact-M30-boundaries", "coverage": "first-open-at-warmup-last-close-at-exclusive-end",
			"entryTimestamp": "first-actually-observed-M5-open", "pendingEntryOnPartialBar": "fill-without-future-completeness-cancel",
		},
		"hma": map[string]any{
			"fast":       int64(9),
			"slow":       int64(21),
			"diagnostic": int64(25),
			"halfLength": "floor",
			"sqrtLength": "round-half-up",
		},
		"supertrend": map[string]any{
			"atrLength":        int64(10),
			"factor":           int64(3),
			"smoothing":        "wilder-rma-sma-seed",
			"recurrence":       "qualified-v9",
			"bullishDirection": int64(-1),
			"bearishDirection": int64(1),
			"initialDirection": int64(1),
		},
		"volume": map[string]any{"smaLength": int64(20), "comparison": "strict-greater", "includeCurrent": true},
		"atr":    map[string]any{"length": int64(14), "smoothing": "wilder-rma-sma-seed"},
		"signal": map[string]any{
			"cross":                "strict-signed-previous-equality-qualifies",
			"chartSTAlignment":     true,
			"h4STAlignment":        true,
			"atrPercentMinimum":    0.08,
			"atrPercentComparison": "greater-or-equal",
			"flatOnly":             true,
			"entry":                "next-observed-native-open",
			"warmupSignals":        "do-not-carry",
			"endCloseNewEntry":     false,
		},
		"h4": map[string]any{
			"timeframe":           "H4",
			"source":              "observed-M30",
			"phaseHoursUTC":       int64(0),
			"aggregation":         "first-max-min-last-sum",
			"count":               "sum-M5-counts",
			"requireComplete":     false,
			"nominalAvailability": "bucket-start-plus-4h",
			"selection":           "latest-nominal-close-less-or-equal-horizon",
			"horizon":             horizon,
			"boundaryOrder":       "H4-finalization-before-source-M30-decision",
			"supertrend":          "same-chart-recurrence-and-parameters",
			"gate":                "mapped-direction-alone",
			"unavailableLine":     "null",
		},
		"completeObservedBars": int64(40), "completeness": "six-exact-M5-slots",
		"stopATR": int64(2), "activationATR": int64(2), "activationComparison": "greater-or-equal", "targetATR": int64(4),
		"initialEquityUSD": int64(10000), "notionalFraction": 0.1, "pointValue": int64(1),
		"quantity": map[string]any{
			"method":                    "floor-to-step",
			"step":                      0.1,
			"epsilonBeforeStepDivision": 1e-12,
			"formula":                   "floor((equity*0.1/actualEntry+1e-12)/0.1)*0.1",
			"nonpositive":               "terminate-quantity_below_step-before-fee-or-position",
		},
		"pricePrecision": map[string]any{"arithmetic": "continuous-float64-reference", "tickRounding": false},
		"feePerUnitSide": 0.5, "entryFee": "debit-immediately", "exitCashflow": "gross-less-exit-fee",
		"execution": map[string]any{
			"path":                  "native-M30-OHLC",
			"nearerExtreme":         "strict-high-nearer-else-low-first-including-ties",
			"openingGapFill":        "actual-observed-open-target-improvement-allowed",
			"intrabarFillTimestamp": "native-close-interval-censored",
			"longEntry":             "reference-open-plus-spread",
			"shortExit":             "reference-OHLC-plus-spread",
			"shortEntryLongExit":    "reference-bid-style",
			"slippage":              int64(0),
			"terminal":              "keep-open",
		},
		"diagnostics": map[string]any{
			"comparisonThreshold":        1e-9,
			"executionTolerance":         "none",
			"sourceLockSwitchRelaxation": "hypothetical-identical-current-inputs",
			"stopWidening":               "actual-vs-prior-live-stop",
			"protectedWorseST":           "rejected-candidate-only",
		},
		"lifecycle": lifecycle,
	}
}

func (s MasterStructuralSpec) projection() map[string]any {
	return map[string]any{"contractVersion": masterStructuralContractVersion, "mode": s.Mode, "profile": MasterStructuralProfile(s.Mode)}
}

// IsMasterStructuralReserved detects family/object intent before generic
// defaults, dispatch or empty-data shortcuts. Malformed aliases remain reserved
// but are never accepted by DecodeMasterStructural.
func IsMasterStructuralReserved(cfg Config) bool {
	for key, value := range cfg {
		if masterIdentity(key) == "masterstructural" {
			return true
		}
		if masterIdentity(key) != "setuptype" {
			continue
		}
		var family string
		switch v := value.(type) {
		case string:
			family = v
		case FamilyID:
			family = string(v)
		}
		if masterIdentity(family) == "masterstructural" {
			return true
		}
	}
	return false
}

// DecodeMasterStructural validates the complete closed configuration without
// generic defaults. Raw JSON must use DecodeMasterStructuralConfigJSON before
// map conversion to retain duplicate-key and exact-number checks.
func DecodeMasterStructural(cfg Config) (MasterStructuralSpec, error) {
	var spec MasterStructuralSpec
	if err := frozenExactKeys(map[string]any(cfg), "config", "dslVersion", "name", "description", "setupType", "masterStructural"); err != nil {
		return spec, err
	}
	if err := regimeMatchFixed(cfg["dslVersion"], int64(7), "dslVersion"); err != nil {
		return spec, err
	}
	family := cfg["setupType"]
	if id, ok := family.(FamilyID); ok {
		family = string(id)
	}
	if family != string(FamilyMasterStructural) {
		return spec, fmt.Errorf("setupType must equal masterStructural")
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
	m, err := frozenConfigObject(cfg["masterStructural"], "masterStructural")
	if err != nil {
		return spec, err
	}
	if err := frozenExactKeys(m, "masterStructural", "contractVersion", "mode", "profile"); err != nil {
		return spec, err
	}
	if err := regimeMatchFixed(m["contractVersion"], masterStructuralContractVersion, "masterStructural.contractVersion"); err != nil {
		return spec, err
	}
	spec.Mode, err = frozenConfigString(m["mode"], "masterStructural.mode")
	if err != nil {
		return spec, err
	}
	switch spec.Mode {
	case MasterStructuralSourceHistoricalReference, MasterStructuralProtectedStableReference:
	default:
		return MasterStructuralSpec{}, fmt.Errorf("masterStructural.mode must be SOURCE_HISTORICAL_REFERENCE or PROTECTED_STABLE_REFERENCE")
	}
	if err := regimeMatchFixed(m["profile"], MasterStructuralProfile(spec.Mode), "masterStructural.profile"); err != nil {
		return MasterStructuralSpec{}, err
	}
	return spec, nil
}

// DecodeMasterStructuralConfigJSON rejects duplicate decoded keys, malformed
// Unicode, trailing documents and numeric mutations before float conversion.
func DecodeMasterStructuralConfigJSON(raw []byte) (Config, error) {
	value, err := strictFrozenJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("master structural config: %w", err)
	}
	m, err := frozenConfigObject(value, "config")
	if err != nil {
		return nil, err
	}
	spec, err := DecodeMasterStructural(Config(m))
	if err != nil {
		return nil, err
	}
	return Config{"dslVersion": int64(7), "name": m["name"], "description": m["description"], "setupType": string(FamilyMasterStructural), "masterStructural": spec.projection()}, nil
}

// masterIdentity is intentionally local: reserved v10 intent recognizes the
// legacy whitespace vocabulary without changing either v9 decoder or profile.
func masterIdentity(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' || r == '_' {
			return -1
		}
		return r
	}, strings.ToLower(value))
}
