package dsl

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

const (
	AdaptiveFlagPolicy                  = "DELAYED_OHLC_REFERENCE_V1"
	AdaptiveFlagNumericalPolicy         = "BINARY64_ORDERED_V1"
	AdaptiveFlagDedicatedRunnerRequired = "adaptive-volume-flag-native-dedicated-runner-required"
	adaptiveFlagContractVersion         = "adaptive-volume-flag-config-v1"
	// Explicit resource limits, not parameter validation or trading recommendations.
	adaptiveFlagMaxLength = 1000000
)

// AdaptiveFlagRules is a value-only snapshot. No parser/runner mutates global defaults.
type AdaptiveFlagRules struct {
	PivotSensitivity  int     `json:"pivotSensitivity"`
	MinPoleATR        float64 `json:"minPoleATR"`
	MinFlagBars       int     `json:"minFlagBars"`
	MaxFlagBars       int     `json:"maxFlagBars"`
	MaxFlagRetrace    float64 `json:"maxFlagRetrace"`
	UseVolumeFilter   bool    `json:"useVolumeFilter"`
	UseEMATrend       bool    `json:"useEMATrend"`
	FastEMALen        int     `json:"fastEMALen"`
	SlowEMALen        int     `json:"slowEMALen"`
	TargetR           float64 `json:"targetR"`
	ATRStopMult       float64 `json:"atrStopMult"`
	ValidBars         int     `json:"validBars"`
	MaxHold           int     `json:"maxHold"`
	ATRLen            int     `json:"atrLen"`
	VolumeSMALen      int     `json:"volumeSMALen"`
	VolumeSMAMult     float64 `json:"volumeSMAMult"`
	FlagWidthPoleMult float64 `json:"flagWidthPoleMult"`
	EntryBufferATR    float64 `json:"entryBufferATR"`
}

type AdaptiveFlagSpec struct {
	Policy          string            `json:"policy"`
	NumericalPolicy string            `json:"numericalPolicy"`
	Timeframe       string            `json:"timeframe"`
	Bundle          string            `json:"bundle"`
	Rules           AdaptiveFlagRules `json:"rules"`
}

// AdaptiveFlagPreset names input identity only, not empirical qualification.
func AdaptiveFlagPreset(bundle string) (AdaptiveFlagRules, error) {
	r := AdaptiveFlagRules{
		PivotSensitivity: 3, MinPoleATR: 1.8, MinFlagBars: 3, MaxFlagBars: 16, MaxFlagRetrace: .50,
		UseVolumeFilter: true, UseEMATrend: true, FastEMALen: 50, SlowEMALen: 200, TargetR: 2.5,
		ATRStopMult: 1.2, ValidBars: 12, MaxHold: 60, ATRLen: 14, VolumeSMALen: 20,
		VolumeSMAMult: .9, FlagWidthPoleMult: .55, EntryBufferATR: .05,
	}
	switch bundle {
	case "INITIAL":
	case "TWEAKED", "SNAPSHOT_C":
		r.MinFlagBars, r.MaxFlagBars, r.FastEMALen, r.SlowEMALen, r.ValidBars, r.ATRStopMult = 5, 21, 55, 144, 20, 2.2
		if bundle == "SNAPSHOT_C" {
			r.ATRStopMult = 2.0
		}
	default:
		return AdaptiveFlagRules{}, fmt.Errorf("unknown adaptive flag preset %q", bundle)
	}
	return r, nil
}

func (r AdaptiveFlagRules) projection() map[string]any {
	return map[string]any{
		"pivotSensitivity": int64(r.PivotSensitivity), "minPoleATR": r.MinPoleATR,
		"minFlagBars": int64(r.MinFlagBars), "maxFlagBars": int64(r.MaxFlagBars), "maxFlagRetrace": r.MaxFlagRetrace,
		"useVolumeFilter": r.UseVolumeFilter, "useEMATrend": r.UseEMATrend,
		"fastEMALen": int64(r.FastEMALen), "slowEMALen": int64(r.SlowEMALen), "targetR": r.TargetR,
		"atrStopMult": r.ATRStopMult, "validBars": int64(r.ValidBars), "maxHold": int64(r.MaxHold),
		"atrLen": int64(r.ATRLen), "volumeSMALen": int64(r.VolumeSMALen), "volumeSMAMult": r.VolumeSMAMult,
		"flagWidthPoleMult": r.FlagWidthPoleMult, "entryBufferATR": r.EntryBufferATR,
	}
}

// AdaptiveFlagConfig constructs a fresh explicit projection; callers must decode
// it before execution. Invalid CUSTOM settings are not corrected or defaulted.
func AdaptiveFlagConfig(name, description, timeframe, bundle string, rules AdaptiveFlagRules) Config {
	return Config{"dslVersion": int64(7), "name": name, "description": description,
		"setupType": string(FamilyAdaptiveVolumeFlag), "adaptiveVolumeFlag": map[string]any{
			"contractVersion": adaptiveFlagContractVersion, "policy": AdaptiveFlagPolicy, "numericalPolicy": AdaptiveFlagNumericalPolicy,
			"timeframe": timeframe, "bundle": bundle, "rules": rules.projection(),
		}}
}

// Malformed discriminator punctuation is reservation-only, never an accepted
// configuration spelling. Keep this stricter guard private to the new family.
func adaptiveFlagDiscriminatorIdentity(key string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, strings.ToLower(key))
}

func IsAdaptiveVolumeFlagReserved(cfg Config) bool {
	for key, value := range cfg {
		if adaptiveFlagSelectorStem(key) {
			return true
		}
		if adaptiveFlagDiscriminatorIdentity(key) != "setuptype" {
			continue
		}
		var family string
		switch v := value.(type) {
		case string:
			family = v
		case FamilyID:
			family = string(v)
		}
		if adaptiveFlagSelectorStem(family) {
			return true
		}
	}
	return false
}

func adaptiveFlagDecimal(value any, path string, positive bool) (float64, error) {
	parts, err := regimeNumberParts(value)
	if err != nil || parts.negative {
		return 0, fmt.Errorf("%s requires a nonnegative finite number", path)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseFloat(string(raw), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || math.Signbit(n) || n < 0 || (positive && n == 0) || (parts.digits != "" && n == 0) {
		return 0, fmt.Errorf("%s is outside the supported finite float64 range", path)
	}
	return n, nil
}

// DecodeAdaptiveVolumeFlag retains strict named-preset identity before any
// float64 conversion. CUSTOM decimals explicitly use nearest float64 values.
func DecodeAdaptiveVolumeFlag(cfg Config) (AdaptiveFlagSpec, error) {
	var s AdaptiveFlagSpec
	if err := frozenExactKeys(map[string]any(cfg), "config", "dslVersion", "name", "description", "setupType", "adaptiveVolumeFlag"); err != nil {
		return s, err
	}
	if err := regimeMatchFixed(cfg["dslVersion"], int64(7), "dslVersion"); err != nil {
		return s, err
	}
	family := cfg["setupType"]
	if f, ok := family.(FamilyID); ok {
		family = string(f)
	}
	if family != string(FamilyAdaptiveVolumeFlag) {
		return s, fmt.Errorf("setupType must equal adaptiveVolumeFlag")
	}
	name, err := frozenConfigString(cfg["name"], "name")
	if err != nil || strings.TrimSpace(name) == "" {
		return s, fmt.Errorf("name must be a nonempty Unicode string")
	}
	if _, err = frozenConfigString(cfg["description"], "description"); err != nil {
		return s, err
	}
	m, err := frozenConfigObject(cfg["adaptiveVolumeFlag"], "adaptiveVolumeFlag")
	if err != nil {
		return s, err
	}
	if err = frozenExactKeys(m, "adaptiveVolumeFlag", "contractVersion", "policy", "numericalPolicy", "timeframe", "bundle", "rules"); err != nil {
		return s, err
	}
	if err = regimeMatchFixed(m["contractVersion"], adaptiveFlagContractVersion, "contractVersion"); err != nil {
		return s, err
	}
	if err = regimeMatchFixed(m["policy"], AdaptiveFlagPolicy, "policy"); err != nil {
		return s, err
	}
	if err = regimeMatchFixed(m["numericalPolicy"], AdaptiveFlagNumericalPolicy, "numericalPolicy"); err != nil {
		return s, err
	}
	s.Policy = AdaptiveFlagPolicy
	s.NumericalPolicy = AdaptiveFlagNumericalPolicy
	s.Timeframe, err = frozenConfigString(m["timeframe"], "timeframe")
	if err != nil {
		return s, err
	}
	if s.Timeframe != "M30" && s.Timeframe != "H1" {
		return s, fmt.Errorf("adaptive timeframe must equal M30 or H1")
	}
	s.Bundle, err = frozenConfigString(m["bundle"], "bundle")
	if err != nil {
		return s, err
	}
	rules, err := frozenConfigObject(m["rules"], "rules")
	if err != nil {
		return s, err
	}
	keys := []string{"pivotSensitivity", "minPoleATR", "minFlagBars", "maxFlagBars", "maxFlagRetrace", "useVolumeFilter", "useEMATrend", "fastEMALen", "slowEMALen", "targetR", "atrStopMult", "validBars", "maxHold", "atrLen", "volumeSMALen", "volumeSMAMult", "flagWidthPoleMult", "entryBufferATR"}
	if err = frozenExactKeys(rules, "rules", keys...); err != nil {
		return s, err
	}
	if s.Bundle != "CUSTOM" {
		preset, e := AdaptiveFlagPreset(s.Bundle)
		if e != nil {
			return s, e
		}
		if e = regimeMatchFixed(rules, preset.projection(), "rules"); e != nil {
			return s, fmt.Errorf("%s input identity mismatch: %w", s.Bundle, e)
		}
	}
	ints := map[string]*int{"pivotSensitivity": &s.Rules.PivotSensitivity, "minFlagBars": &s.Rules.MinFlagBars, "maxFlagBars": &s.Rules.MaxFlagBars, "fastEMALen": &s.Rules.FastEMALen, "slowEMALen": &s.Rules.SlowEMALen, "validBars": &s.Rules.ValidBars, "maxHold": &s.Rules.MaxHold, "atrLen": &s.Rules.ATRLen, "volumeSMALen": &s.Rules.VolumeSMALen}
	for key, ptr := range ints {
		n, e := frozenConfigInteger(rules[key], "rules."+key, 1, adaptiveFlagMaxLength)
		if e != nil {
			return s, e
		}
		*ptr = int(n)
	}
	nums := map[string]*float64{"minPoleATR": &s.Rules.MinPoleATR, "maxFlagRetrace": &s.Rules.MaxFlagRetrace, "targetR": &s.Rules.TargetR, "atrStopMult": &s.Rules.ATRStopMult, "volumeSMAMult": &s.Rules.VolumeSMAMult, "flagWidthPoleMult": &s.Rules.FlagWidthPoleMult, "entryBufferATR": &s.Rules.EntryBufferATR}
	for key, ptr := range nums {
		n, e := adaptiveFlagDecimal(rules[key], "rules."+key, key == "minPoleATR" || key == "targetR" || key == "atrStopMult")
		if e != nil {
			return s, e
		}
		*ptr = n
	}
	for key, ptr := range map[string]*bool{"useVolumeFilter": &s.Rules.UseVolumeFilter, "useEMATrend": &s.Rules.UseEMATrend} {
		v, ok := rules[key].(bool)
		if !ok {
			return s, fmt.Errorf("rules.%s must be boolean", key)
		}
		*ptr = v
	}
	if s.Rules.MinFlagBars > s.Rules.MaxFlagBars {
		return s, fmt.Errorf("minimum flag bars exceeds maximum")
	}
	return s, nil
}

func DecodeAdaptiveVolumeFlagConfigJSON(raw []byte) (Config, error) {
	v, err := strictFrozenJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("adaptive flag config: %w", err)
	}
	m, err := frozenConfigObject(v, "config")
	if err != nil {
		return nil, err
	}
	s, err := DecodeAdaptiveVolumeFlag(Config(m))
	if err != nil {
		return nil, err
	}
	return AdaptiveFlagConfig(m["name"].(string), m["description"].(string), s.Timeframe, s.Bundle, s.Rules), nil
}
