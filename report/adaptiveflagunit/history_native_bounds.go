//go:build !js && !wasm

package adaptiveflagunit

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

// The raw and unit native paths share the same bounded value proof.
func historyJSONBound(value any) (uint64, error) { return adaptiveflag.HistoryJSONBound(value) }

// Actual order and exposure counts are known before projection allocations.
// Full per-type maxima (not observed token widths) cover every constructed item.
func historyProjectionBound(run *engine.AdaptiveFlagResult, ctx validationContext) (uint64, error) {
	n, o, d, e, g := uint64(len(run.Snapshots)), uint64(len(run.Orders)), uint64(ctx.Days), uint64(ctx.Midnights), uint64(ctx.Gaps)
	if n < 1 || n > 70000 || o > n || d < 1 || d > HistoryMaxCalendarDays || e > HistoryMaxExposureMidnights || g > HistoryMaxExposureGaps {
		return 0, fmt.Errorf("native history projection cardinality bound")
	}
	terms, err := ActualTypeBounds()
	if err != nil {
		return 0, err
	}
	limit := terms.Fixed + 65536 + o*(terms.Orders+terms.ClosedTrades+2*terms.CostEvents+terms.Rejections) + n*terms.Marks + d*(terms.Daily+terms.EntryYear+terms.CalendarYear) + e*terms.Midnights + g*terms.Gaps
	if limit > HistoryMaxOutputBytes {
		return 0, fmt.Errorf("native history projection conservative bound exceeds 256 MiB")
	}
	return limit, nil
}

func historyLimits() projectionLimits {
	return projectionLimits{rows: 70000, days: HistoryMaxCalendarDays, midnights: HistoryMaxExposureMidnights, gaps: HistoryMaxExposureGaps, maxOutput: HistoryMaxOutputBytes,
		rawBound: func(n uint64) (uint64, error) {
			if n < 1 || n > 70000 {
				return 0, fmt.Errorf("native history row bound")
			}
			return HistoryMaxOutputBytes, nil
		},
		outputBound: historyProjectionBound,
		checkBeforeSerialize: func(p *Projection, limit uint64) error {
			bound, err := historyJSONBound(p)
			if err != nil || bound > limit {
				return Rejection("resource_limit", "output_bound", "native history projection violates its pre-serialization bound")
			}
			return nil
		},
	}
}
