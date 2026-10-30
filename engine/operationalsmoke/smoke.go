// Package operationalsmoke produces intents for deliberately unprofitable
// operational canaries. It does not execute orders or qualify any runtime route.
package operationalsmoke

import (
	"errors"
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// Profile identifies one of the two existing canaries. The historical BTC ID
// says ThreeCandle, but its current rule uses exactly two closed candles.
type Profile string

const (
	BTCUSD    Profile = "forwardExecutionSmokeThreeCandleBtcusdOneMinute"
	AnySymbol Profile = "forwardExecutionSmokeTwoCandleAnySymbolOneMinute"
	Warning           = "Operational execution smoke test only. Intentionally bad and ineligible for research promotion or book membership."
)

// Definition carries warnings, not route admission or execution capability.
type Definition struct {
	ID                Profile
	Name              string
	ReviewNote        string
	Unprofitable      bool
	OperationalCanary bool
}

// Describe rejects every ID outside the two explicitly supported profiles.
func Describe(profile Profile) (Definition, error) {
	var name string
	switch profile {
	case BTCUSD:
		name = "BAD TEST ONLY — BTCUSD 1m Two-Candle Streak"
	case AnySymbol:
		name = "BAD TEST ONLY — Any-Symbol 1m Two-Candle Streak"
	default:
		return Definition{}, ErrUnknownProfile
	}
	return Definition{profile, name, Warning, true, true}, nil
}

// Side is the requested direction, independent of the parent engine's types.
type Side string

const (
	Long  Side = "long"
	Short Side = "short"
)

var (
	ErrUnknownProfile       = errors.New("operational smoke: unknown profile")
	ErrUnsupportedParameter = errors.New("operational smoke: unsupported parameter")
	ErrInvalidBar           = errors.New("operational smoke: nonfinite open or close")
	ErrInvalidBracket       = errors.New("operational smoke: nonfinite bracket")
)

// Parameters are numeric overrides of profile defaults. Nil means omitted;
// a pointer to zero means explicitly zero. Distances, risk and size must be
// finite and nonnegative. JS coercions/null and negative values are unsupported.
// Absolute distances belong only to BTCUSD; fractional distances only to
// AnySymbol. Cross-profile fields are explicitly refused, never ignored.
type Parameters struct {
	StopDistance      *float64
	TargetDistance    *float64
	StopDistancePct   *float64
	TargetDistancePct *float64
	RiskUSD           *float64
	Size              *float64
}

// Intent contains absolute signal-close-anchored prices. SignalClose is not a
// fill. The broker owns sizing, fills, slippage, exits and accounting. RiskUSD
// is always present, including zero; Size is optional, including explicit zero.
type Intent struct {
	Side        Side
	SignalClose float64
	SL          float64
	TP          float64
	RiskUSD     float64
	Size        *float64
	Tag         string
}

// Evaluate consumes the caller's adjacent indexed closed bars i-1 and i.
// A nil bar represents insufficient history. It neither checks timestamps nor
// selects bars, determines whether bars are closed, admits routes or remembers
// previous calls. The runtime/journal owns admission and idempotency.
// The bool distinguishes an emitted intent from ordinary no-signal results;
// errors explicitly refuse unsupported profiles, parameters or numeric inputs.
func Evaluate(profile Profile, previous, current *marketdata.Bar, hasPosition bool, parameters Parameters) (Intent, bool, error) {
	stop, target, risk, err := resolve(profile, parameters)
	if err != nil {
		return Intent{}, false, err
	}
	if hasPosition || previous == nil || current == nil {
		return Intent{}, false, nil
	}
	if !finite(previous.O) || !finite(previous.C) || !finite(current.O) || !finite(current.C) {
		return Intent{}, false, ErrInvalidBar
	}
	var side Side
	var tag string
	// Long is evaluated before short, as in the ordered canary rules.
	if previous.C > previous.O && current.C > current.O {
		side, tag = Long, "FORWARD-SMOKE-2-BULLISH"
	} else if previous.C < previous.O && current.C < current.O {
		side, tag = Short, "FORWARD-SMOKE-2-BEARISH"
	} else {
		return Intent{}, false, nil
	}
	if profile == AnySymbol {
		stop *= math.Abs(current.C)
		target *= math.Abs(current.C)
	}
	sign := 1.0
	if side == Short {
		sign = -1
	}
	intent := Intent{
		Side: side, SignalClose: current.C,
		SL: current.C - sign*stop, TP: current.C + sign*target,
		RiskUSD: risk, Tag: tag,
	}
	if !finite(intent.SL) || !finite(intent.TP) {
		return Intent{}, false, ErrInvalidBracket
	}
	if parameters.Size != nil {
		size := *parameters.Size
		intent.Size = &size // Do not alias caller-owned mutable parameters.
	}
	return intent, true, nil
}

func resolve(profile Profile, p Parameters) (stop, target, risk float64, err error) {
	var stopOverride, targetOverride *float64
	switch profile {
	case BTCUSD:
		if p.StopDistancePct != nil || p.TargetDistancePct != nil {
			return 0, 0, 0, fmt.Errorf("%w: fractional distances for BTCUSD", ErrUnsupportedParameter)
		}
		stop, target = 200, 200
		stopOverride, targetOverride = p.StopDistance, p.TargetDistance
	case AnySymbol:
		if p.StopDistance != nil || p.TargetDistance != nil {
			return 0, 0, 0, fmt.Errorf("%w: absolute distances for AnySymbol", ErrUnsupportedParameter)
		}
		stop, target = 0.0025, 0.0025
		stopOverride, targetOverride = p.StopDistancePct, p.TargetDistancePct
	default:
		return 0, 0, 0, ErrUnknownProfile
	}
	risk = 100
	for _, field := range []struct {
		name  string
		value *float64
	}{
		{"stop distance", stopOverride}, {"target distance", targetOverride},
		{"riskUsd", p.RiskUSD}, {"size", p.Size},
	} {
		if field.value != nil && (!finite(*field.value) || *field.value < 0) {
			return 0, 0, 0, fmt.Errorf("%w: %s must be finite and nonnegative", ErrUnsupportedParameter, field.name)
		}
	}
	if stopOverride != nil {
		stop = *stopOverride
	}
	if targetOverride != nil {
		target = *targetOverride
	}
	if p.RiskUSD != nil {
		risk = *p.RiskUSD
	}
	return stop, target, risk, nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
