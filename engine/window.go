package engine

import (
	"fmt"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// ExecutionWindow separates the bars needed to prepare causal indicators
// from the bars on which the strategy may open or manage trades. The supplied
// market series must begin at ContextFromT when it is set. TradeFromT is the
// first inclusive entry-series timestamp and TradeToT is the exclusive upper
// endpoint. A TradeToT one bar beyond the last supplied bar is valid.
//
// A nil window preserves the historical behaviour: every supplied bar is
// tradable and the final supplied bar is the liquidation bar.
type ExecutionWindow struct {
	ContextFromT *int64
	TradeFromT   *int64
	TradeToT     *int64
}

// ExecutionBounds are the resolved inclusive indexes for an
// ExecutionWindow. They are exported so report adapters can describe the
// actual tradable date bounds without reconstructing engine semantics.
type ExecutionBounds struct {
	ContextStart int
	TradeStart   int
	TradeEnd     int
}

// ResolveExecutionWindow validates and resolves a window against the entry
// series. TradeFromT and ContextFromT must identify existing bars exactly.
// TradeToT must identify an existing bar (which is excluded) or the exact
// next-bar timestamp after the last supplied bar. Callers must not silently
// round a requested boundary to a nearby bar.
func ResolveExecutionWindow(series marketdata.Series, window *ExecutionWindow) (ExecutionBounds, error) {
	n := series.Len()
	if n == 0 {
		return ExecutionBounds{}, fmt.Errorf("execution window requires a non-empty market series")
	}
	find := func(label string, value *int64, fallback int) (int, error) {
		if value == nil {
			return fallback, nil
		}
		for index, timestamp := range series.T {
			if int64(timestamp) == *value {
				return index, nil
			}
		}
		return 0, fmt.Errorf("execution window %s timestamp %d is absent from market series", label, *value)
	}
	contextStart, err := find("contextFromT", func() *int64 {
		if window == nil {
			return nil
		}
		return window.ContextFromT
	}(), 0)
	if err != nil {
		return ExecutionBounds{}, err
	}
	if contextStart != 0 {
		return ExecutionBounds{}, fmt.Errorf("execution window contextFromT must identify the first supplied market bar (index 0), got index %d", contextStart)
	}
	tradeStart, err := find("tradeFromT", func() *int64 {
		if window == nil {
			return nil
		}
		return window.TradeFromT
	}(), 0)
	if err != nil {
		return ExecutionBounds{}, err
	}
	tradeEnd := n - 1
	if window != nil && window.TradeToT != nil {
		endpoint := *window.TradeToT
		found := false
		for index, timestamp := range series.T {
			if int64(timestamp) == endpoint {
				tradeEnd = index - 1
				found = true
				break
			}
		}
		if !found {
			if n < 2 {
				return ExecutionBounds{}, fmt.Errorf("execution window tradeToT exclusive timestamp %d is absent from market series", endpoint)
			}
			step := int64(series.T[n-1] - series.T[n-2])
			if step <= 0 || int64(series.T[n-1])+step != endpoint {
				return ExecutionBounds{}, fmt.Errorf("execution window tradeToT exclusive timestamp %d is absent from market series and is not the next bar after the supplied series", endpoint)
			}
			tradeEnd = n - 1
		}
	}
	if tradeStart < contextStart {
		return ExecutionBounds{}, fmt.Errorf("execution window tradeFromT index %d precedes context start index %d", tradeStart, contextStart)
	}
	if tradeEnd < tradeStart {
		return ExecutionBounds{}, fmt.Errorf("execution window tradeToT exclusive endpoint %d precedes tradeFromT index %d", tradeEnd+1, tradeStart)
	}
	return ExecutionBounds{ContextStart: contextStart, TradeStart: tradeStart, TradeEnd: tradeEnd}, nil
}
