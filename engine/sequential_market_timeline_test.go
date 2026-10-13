package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSequentialMarketTimelineScan(t *testing.T) {
	const step = 3_600_000.0
	mk := func(offsets ...float64) RunFixture {
		var f RunFixture
		for _, o := range offsets {
			f.Bars = append(f.Bars, marketdata.Bar{T: 1709510400000 + o*step})
		}
		return f
	}
	// 0,1,2 contiguous; 2->5 skips two bars; 5->6 contiguous; 6->56 skips 49.
	got := ScanSequentialMarketTimeline(mk(0, 1, 2, 5, 6, 56), 3_600_000)
	if got.Bars != 6 || got.GapCount != 2 || got.GapsTruncated || len(got.Gaps) != 2 {
		t.Fatalf("timeline: %+v", got)
	}
	if g := got.Gaps[0]; g.Index != 3 || g.MissingBars != 2 || g.OpenMS-g.PreviousOpenMS != 3*step {
		t.Fatalf("first gap: %+v", g)
	}
	if g := got.Gaps[1]; g.Index != 5 || g.MissingBars != 49 {
		t.Fatalf("second gap: %+v", g)
	}
	// Duplicate and backward timestamps are validation errors elsewhere, not gaps.
	if got := ScanSequentialMarketTimeline(mk(0, 1, 1, 0), 3_600_000); got.GapCount != 0 {
		t.Fatalf("non-forward steps counted as gaps: %+v", got)
	}
	// The list is bounded; the count is not.
	offsets := make([]float64, 0, 100)
	for i := 0; i < 100; i++ {
		offsets = append(offsets, float64(2*i))
	}
	got = ScanSequentialMarketTimeline(mk(offsets...), 3_600_000)
	if got.GapCount != 99 || len(got.Gaps) != SequentialMarketMaxListedGaps || !got.GapsTruncated || got.Gaps[SequentialMarketMaxListedGaps-1].Index != SequentialMarketMaxListedGaps {
		t.Fatalf("bounded list: count %d listed %d truncated %v", got.GapCount, len(got.Gaps), got.GapsTruncated)
	}
}
