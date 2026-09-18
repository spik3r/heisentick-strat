package engine

import (
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const schedulerHour = float64(60 * 60 * 1000)

func schedulerSeries(times []float64) marketdata.Series {
	return marketdata.Series{T: times, O: make([]float64, len(times)), H: make([]float64, len(times)), L: make([]float64, len(times)), C: make([]float64, len(times))}
}

func TestScheduleSourceEventsCausalBoundaryGapAndIdempotence(t *testing.T) {
	source := schedulerSeries([]float64{0, 4 * schedulerHour, 8 * schedulerHour})
	chartTimes := make([]float64, 16)
	for i := range chartTimes {
		chartTimes[i] = float64(i) * 15 * 60 * 1000
	}
	chartTimes = append(chartTimes, 12*schedulerHour, 12*schedulerHour+15*60*1000)
	entries := ScheduleSourceEvents(source, schedulerSeries(chartTimes), []int{0, 0, 1})
	want := []ScheduledEntry{{SourceIndex: 0, ChartIndex: 16, SourceClose: 4 * schedulerHour, EntryTime: 12*schedulerHour + 15*60*1000}, {SourceIndex: 1, ChartIndex: 16, SourceClose: 8 * schedulerHour, EntryTime: 12*schedulerHour + 15*60*1000}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %#v, want %#v", entries, want)
	}
}
