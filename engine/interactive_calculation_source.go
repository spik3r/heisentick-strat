package engine

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const InteractiveCalculationSourceSchema = "strategy-calculation-source-v1"

func calculationSourceSchema(source string) string {
	if source == "" {
		return ""
	}
	return InteractiveCalculationSourceSchema
}

// heikinAshiSeries transforms only strategy calculation OHLC. Timestamps and
// provider volume stay aligned with the raw input. The first supplied bar is
// the seed; gaps and session boundaries do not reset it. A suffix can never
// change the candles already computed for a prefix. Separate chart and HTF
// inputs each have their own seed; HTF is never synthesized from chart bars.
func heikinAshiSeries(raw marketdata.Series) (marketdata.Series, error) {
	if err := validateSeriesShape("Heikin-Ashi input", raw); err != nil {
		return marketdata.Series{}, err
	}
	if raw.Len() == 0 {
		return marketdata.Series{}, fmt.Errorf("%w: empty Heikin-Ashi calculation series", ErrInteractiveUnsupported)
	}
	out := marketdata.NewSeries(raw.Len())
	if len(raw.V) == 0 {
		out.V = nil
	}
	for i := range raw.T {
		o, h, l, c := raw.O[i], raw.H[i], raw.L[i], raw.C[i]
		if !isFiniteDerivedOutput(o) || !isFiniteDerivedOutput(h) ||
			!isFiniteDerivedOutput(l) || !isFiniteDerivedOutput(c) ||
			h < math.Max(o, c) || l > math.Min(o, c) || h < l {
			return marketdata.Series{}, fmt.Errorf("%w: invalid raw OHLC at Heikin-Ashi bar %d", ErrInteractiveUnsupported, i)
		}
		out.T[i] = raw.T[i]
		if len(raw.V) != 0 {
			out.V[i] = raw.V[i]
		}
		out.C[i] = (o + h + l + c) / 4
		if i == 0 {
			out.O[i] = (o + c) / 2
		} else {
			out.O[i] = (out.O[i-1] + out.C[i-1]) / 2
		}
		out.H[i] = math.Max(h, math.Max(out.O[i], out.C[i]))
		out.L[i] = math.Min(l, math.Min(out.O[i], out.C[i]))
		if !isFiniteDerivedOutput(out.O[i]) || !isFiniteDerivedOutput(out.H[i]) ||
			!isFiniteDerivedOutput(out.L[i]) || !isFiniteDerivedOutput(out.C[i]) {
			return marketdata.Series{}, fmt.Errorf("%w: non-finite Heikin-Ashi bar %d", ErrInteractiveUnsupported, i)
		}
	}
	return out, nil
}
