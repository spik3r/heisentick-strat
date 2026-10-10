package engine

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
)

// legacySetup9Raw holds a profile's strategy parameters the way the archived
// JavaScript received them, before its `||` and `??` fallbacks. A nil field is
// an undefined JavaScript parameter.
type legacySetup9Raw struct {
	SetupCount, StopBufferATR, RMultiple *float64
	SeasonalityEdge, MinSamples          *float64
	RequirePerfection                    bool
}

// legacySetup9Profile is a frozen, resolved profile. There is no way to
// author a different value from a Strat source: the profile ID selects it.
type legacySetup9Profile struct {
	ID                string
	SetupCount        float64
	StopBufferATR     float64
	RMultiple         float64
	RequirePerfection bool
	Seasonal          bool
	SeasonalityEdge   float64
	MinSamples        float64
	LongTag, ShortTag string
}

// Context requested by dslTDSeasonalReversal: ATR length 14 (the context
// default) and one intraday filter over 90 days with a classification
// threshold of 5 samples. The threshold only labels the context row; the
// strategy gate uses MinSamples.
const (
	legacySetup9ATRLen            = 14
	legacySetup9SeasonLookbackDay = 90
	legacySetup9SeasonCtxMinSamp  = 5
	legacySetup9SeasonKey         = "90d"
)

func legacyNum(v float64) *float64 { return &v }

// legacyOr mirrors JavaScript `value || fallback` for a numeric parameter:
// undefined, 0 and NaN take the fallback.
func legacyOr(value *float64, fallback float64) float64 {
	if value == nil || *value == 0 || math.IsNaN(*value) {
		return fallback
	}
	return *value
}

// legacyNullish mirrors `value ?? fallback`: only undefined takes the fallback.
func legacyNullish(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func (raw legacySetup9Raw) resolve(id string, seasonal bool, longTag, shortTag string) legacySetup9Profile {
	p := legacySetup9Profile{
		ID:                id,
		SetupCount:        legacyOr(raw.SetupCount, 9),
		StopBufferATR:     legacyOr(raw.StopBufferATR, 0.5),
		RequirePerfection: raw.RequirePerfection,
		Seasonal:          seasonal,
		LongTag:           longTag,
		ShortTag:          shortTag,
	}
	// dslTDSequential passes rMultiple straight to riskReward: no fallback.
	// An undefined value would give NaN; both profiles define it.
	if raw.RMultiple == nil {
		p.RMultiple = math.NaN()
	} else {
		p.RMultiple = *raw.RMultiple
	}
	if seasonal {
		p.SeasonalityEdge = legacyNullish(raw.SeasonalityEdge, 10)
		p.MinSamples = legacyOr(raw.MinSamples, 10)
	}
	return p
}

var legacySetup9Profiles = map[string]legacySetup9Profile{
	dsl.LegacySetup9Profile: legacySetup9Raw{
		SetupCount: legacyNum(9), StopBufferATR: legacyNum(0.5), RMultiple: legacyNum(2),
	}.resolve(dsl.LegacySetup9Profile, false, "TD-BUY-9", "TD-SELL-9"),
	dsl.LegacySetup9PerfSeasonalProfile: legacySetup9Raw{
		SetupCount: legacyNum(9), StopBufferATR: legacyNum(0.5), RMultiple: legacyNum(2),
		RequirePerfection: true, SeasonalityEdge: legacyNum(10), MinSamples: legacyNum(10),
	}.resolve(dsl.LegacySetup9PerfSeasonalProfile, true, "TD-BUY-9-SEASONAL", "TD-SELL-9-SEASONAL"),
}

type legacySetup9Params struct {
	Enabled bool
	Profile legacySetup9Profile
	cache   *legacySetup9CounterCache
}

func legacySetup9ParamsFromConfig(cfg dsl.Config) legacySetup9Params {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyLegacySetup9) {
		return legacySetup9Params{}
	}
	id, _ := mapValue(cfg, "legacySetup9")["profile"].(string)
	profile, ok := legacySetup9Profiles[id]
	if !ok {
		return legacySetup9Params{}
	}
	return legacySetup9Params{Enabled: true, Profile: profile, cache: &legacySetup9CounterCache{}}
}

// validateLegacySetup9Execution refuses configurations the frozen profiles
// cannot honour: an unknown profile, or any fill other than the legacy
// signal-bar close. The HT-231 common next-open policy needs its own reviewed
// rules (fill-based stop and target), so it is not approximated here.
func validateLegacySetup9Execution(cfg dsl.Config, costs Costs) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyLegacySetup9) {
		return nil
	}
	id, _ := mapValue(cfg, "legacySetup9")["profile"].(string)
	if _, ok := legacySetup9Profiles[id]; !ok {
		return fmt.Errorf("legacy setup 9 requires a known sequential profile, got %q", id)
	}
	return legacySetup9FillError(id, costs)
}

func legacySetup9FillError(profileID string, costs Costs) error {
	if fillOn := costs.normalized().FillOn; fillOn != "close" {
		return fmt.Errorf("legacy setup 9 profile %s supports only costs.fillOn=close, got %q", profileID, fillOn)
	}
	return nil
}

// legacySetup9SeasonalitySpecs returns the context the seasonal profile asks
// for. It is requested by the family rather than authored as a filter, so the
// generic seasonality gates never see it.
func legacySetup9SeasonalitySpecs(cfg dsl.Config) []contextcols.SeasonalitySpec {
	params := legacySetup9ParamsFromConfig(cfg)
	if !params.Enabled || !params.Profile.Seasonal {
		return nil
	}
	return []contextcols.SeasonalitySpec{{
		Dimension: "intraday", Lookback: legacySetup9SeasonKey, LookbackDays: legacySetup9SeasonLookbackDay,
		MinSamples: legacySetup9SeasonCtxMinSamp,
	}}
}
