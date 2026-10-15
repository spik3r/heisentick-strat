//go:build !js && !wasm

package adaptiveflagunit

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

// The raw and unit native paths share the same bounded value proof.
func historyJSONBound(value any) (uint64, error) {
	return adaptiveflag.HistoryJSONBound(value)
}

func historyJSONBoundForProfile(profile string, value any) (uint64, error) {
	return adaptiveflag.HistoryJSONBoundForProfile(profile, value)
}

func historyMaxOutputBytes(profile string) uint64 {
	if profile == adaptiveflag.HistoryLongH1Profile {
		return HistoryLongH1MaxOutputBytes
	}
	return HistoryMaxOutputBytes
}

// Actual order and exposure counts are known before projection allocations.
// Full per-type maxima (not observed token widths) cover every constructed item.
func historyProjectionBound(run *engine.AdaptiveFlagResult, ctx validationContext) (uint64, error) {
	return historyProjectionBoundWithLimits(run, ctx, adaptiveflag.HistoryProfile)
}

func historyProjectionBoundWithLimits(run *engine.AdaptiveFlagResult, ctx validationContext, profile string) (uint64, error) {
	n, o, d, e, g := uint64(len(run.Snapshots)), uint64(len(run.Orders)), uint64(ctx.Days), uint64(ctx.Midnights), uint64(ctx.Gaps)
	rows, days, midnights, gaps := profileCaps(profile)
	if n < 1 || n > uint64(rows) || o > n || d < 1 || d > uint64(days) || e > uint64(midnights) || g > uint64(gaps) {
		return 0, fmt.Errorf("native history projection cardinality bound")
	}
	terms, err := ActualTypeBounds()
	if err != nil {
		return 0, err
	}
	limit := terms.Fixed + 65536 + o*(terms.Orders+terms.ClosedTrades+2*terms.CostEvents+terms.Rejections) + n*terms.Marks + d*(terms.Daily+terms.EntryYear+terms.CalendarYear) + e*terms.Midnights + g*terms.Gaps
	if limit > historyMaxOutputBytes(profile) {
		return 0, fmt.Errorf("native history projection conservative bound exceeds fixed profile cap")
	}
	return limit, nil
}

func historyLimits() projectionLimits { return historyLimitsFor(adaptiveflag.HistoryProfile) }

func profileCaps(profile string) (rows, days, midnights, gaps int) {
	if profile == adaptiveflag.HistoryLongH1Profile {
		return 150000, 9000, 120000, 150000
	}
	return 70000, HistoryMaxCalendarDays, HistoryMaxExposureMidnights, HistoryMaxExposureGaps
}

func historyLimitsFor(profile string) projectionLimits {
	rows, days, midnights, gaps := profileCaps(profile)
	maxOutput := historyMaxOutputBytes(profile)
	return projectionLimits{rows: rows, days: days, midnights: midnights, gaps: gaps, maxOutput: maxOutput,
		rawBound: func(n uint64) (uint64, error) {
			if n < 1 || n > uint64(rows) {
				return 0, fmt.Errorf("native history row bound")
			}
			return maxOutput, nil
		},
		outputBound: func(run *engine.AdaptiveFlagResult, ctx validationContext) (uint64, error) {
			return historyProjectionBoundWithLimits(run, ctx, profile)
		},
		checkBeforeSerialize: func(p *Projection, limit uint64) error {
			bound, err := historyJSONBoundForProfile(profile, p)
			if err != nil || bound > limit {
				return Rejection("resource_limit", "output_bound", "native history projection violates its pre-serialization bound")
			}
			return nil
		},
	}
}
