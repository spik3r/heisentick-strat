package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
)

func TestPreparedRunReportTradeContextUsesCanonicalLabelsAndBounds(t *testing.T) {
	run := &PreparedRun{cols: contextcols.Columns{
		OpenLocation: []int8{1, 0},
		PriorDayType: []int8{2, 0},
	}}
	got, ok := run.ReportTradeContext(0)
	if !ok || got.OpenLocation != "nearPDH" || got.PriorDayType != "trend" {
		t.Fatalf("context = %#v, ok=%v", got, ok)
	}
	unknown, ok := run.ReportTradeContext(1)
	if !ok || unknown.OpenLocation != "other" || unknown.PriorDayType != "" {
		t.Fatalf("unknown codes = %#v, ok=%v", unknown, ok)
	}
	for _, index := range []int{-1, 2} {
		if _, ok := run.ReportTradeContext(index); ok {
			t.Fatalf("index %d unexpectedly resolved", index)
		}
	}
}
