package adaptiveflagunit

import (
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

// Private resource plumbing only. The public portable wrappers always construct
// this exact original profile; only native-tagged code can construct history.
type projectionLimits struct {
	rows, days, midnights, gaps int
	maxOutput                   uint64
	rawBound                    func(uint64) (uint64, error)
	outputBound                 func(*engine.AdaptiveFlagResult, validationContext) (uint64, error)
	checkBeforeSerialize        func(*Projection, uint64) error
}

func portableLimits() projectionLimits {
	return projectionLimits{
		rows: MaxRetainedRows, days: MaxCalendarDays, midnights: MaxExposureMidnights, gaps: MaxExposureGaps,
		maxOutput: MaxOutputBytes, rawBound: adaptiveflag.RuntimeOutputBound,
		outputBound: func(run *engine.AdaptiveFlagResult, ctx validationContext) (uint64, error) {
			return GeneratedOutputBound(uint64(len(run.Snapshots)), uint64(ctx.Days), uint64(ctx.Midnights), uint64(ctx.Gaps))
		},
	}
}
