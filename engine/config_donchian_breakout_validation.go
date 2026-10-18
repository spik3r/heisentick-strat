package engine

import (
	"fmt"
	"math"
	"strconv"

	"github.com/spik3r/heisentick-strat/dsl"
)

// donchianCount checks the original numeric representation before conversion.
// In particular, int/int64 MaxInt must not be rounded through float64, whose
// representation of MaxInt64 is the first out-of-range integer, 2^63.
func donchianCount(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, v > 0
	case int64:
		if v <= 0 || v > int64(int(^uint(0)>>1)) {
			return 0, false
		}
		return int(v), true
	case float32:
		return donchianFloatCount(float64(v))
	case float64:
		return donchianFloatCount(v)
	default:
		return 0, false
	}
}

func donchianFloatCount(v float64) (int, bool) {
	if !isFinite(v) || v < 1 || math.Trunc(v) != v || v >= math.Ldexp(1, strconv.IntSize-1) {
		return 0, false
	}
	return int(v), true
}

func donchianCountValue(m map[string]any, key string, fallback int) int {
	if count, ok := donchianCount(m[key]); ok {
		return count
	}
	return fallback
}

func donchianNumber(value any) (float64, bool) {
	var n float64
	switch v := value.(type) {
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case float32:
		n = float64(v)
	case float64:
		n = v
	default:
		return 0, false
	}
	return n, isFinite(n)
}

// A missing optional map retains the family default; an explicitly malformed
// map must not turn into that default through mapValue's permissive fallback.
func donchianConfigMap(cfg dsl.Config, key string) (map[string]any, error) {
	if raw, present := cfg[key]; present {
		switch value := raw.(type) {
		case map[string]any:
			if value != nil {
				return value, nil
			}
		case dsl.Config:
			if value != nil {
				return map[string]any(value), nil
			}
		}
		return nil, fmt.Errorf("donchian breakout %s must be an object", key)
	}
	return nil, nil
}

// validateDonchianBreakoutConfig validates authored values before the generic
// config decoder can replace malformed values with defaults. Other families
// keep their existing contracts and validators.
func validateDonchianBreakoutConfig(cfg dsl.Config) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyDonchianBreakout) {
		return nil
	}
	family, err := donchianConfigMap(cfg, "donchianBreakout")
	if err != nil {
		return err
	}
	if _, ok := donchianCount(family["channelBars"]); !ok {
		return fmt.Errorf("donchian breakout channelBars requires a positive integer representable by the engine")
	}
	if raw, present := family["trendEmaLen"]; present {
		if n, numeric := donchianNumber(raw); !numeric || n != 0 {
			if _, ok := donchianCount(raw); !ok {
				return fmt.Errorf("donchian breakout trendEmaLen requires zero or a positive integer representable by the engine")
			}
		}
	}
	stop, err := donchianConfigMap(cfg, "stop")
	if err != nil {
		return err
	}
	if raw, present := stop["type"]; present {
		if value, ok := raw.(string); !ok || value != "atr" {
			return fmt.Errorf("donchian breakout supports only a fixed ATR stop")
		}
	}
	if raw, present := stop["atrMult"]; present {
		if n, ok := donchianNumber(raw); !ok || n <= 0 {
			return fmt.Errorf("donchian breakout atrMult requires a finite positive number")
		}
	}
	for _, key := range []string{"paddingAtr", "minAtr", "maxAtr", "extremeCandles"} {
		if raw, present := stop[key]; present {
			if n, ok := donchianNumber(raw); !ok || n != 0 {
				return fmt.Errorf("donchian breakout does not support stop %s overrides", key)
			}
		}
	}
	target, err := donchianConfigMap(cfg, "target")
	if err != nil {
		return err
	}
	if raw, present := target["dcbR"]; present {
		if n, ok := donchianNumber(raw); !ok || n < 0 {
			return fmt.Errorf("donchian breakout dcbR requires a finite nonnegative number")
		}
	}
	entry, err := donchianConfigMap(cfg, "entryMode")
	if err != nil {
		return err
	}
	if raw, present := entry["type"]; present {
		if value, ok := raw.(string); !ok || value != "market" {
			return fmt.Errorf("donchian breakout supports only market entry mode")
		}
	}
	if raw, present := cfg["entryTf"]; present {
		if value, ok := raw.(string); !ok || value != "current" {
			return fmt.Errorf("donchian breakout supports only the current entry timeframe")
		}
	}
	if raw, present := cfg["sourceTimeframe"]; present {
		if value, ok := raw.(string); !ok || value != "" {
			return fmt.Errorf("donchian breakout does not support source timeframe execution")
		}
	}
	return nil
}
