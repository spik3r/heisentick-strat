package engine

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed japan-settlement-holidays.json
var japanHolidayJSON []byte

type japanHolidaySource struct {
	Source       string   `json:"source"`
	SourceSHA256 string   `json:"sourceSha256"`
	Dates        []string `json:"dates"`
}

const japanCalendarFrom = "2021-01-01"
const japanCalendarThrough = "2027-12-31"

func japanCalendarEndExclusive() time.Time {
	through, err := time.Parse("2006-01-02", japanCalendarThrough)
	if err != nil {
		panic("invalid Japan settlement calendar bound")
	}
	return through.AddDate(0, 0, 1)
}

var japanHolidayDates = func() map[string]bool {
	var source japanHolidaySource
	if err := json.Unmarshal(japanHolidayJSON, &source); err != nil {
		panic(err)
	}
	if source.SourceSHA256 != "cec37a743c96995cdb9cb52b685c9003634682a9b0e1a640a6b9b96881fe964a" {
		panic("Japan holiday source digest mismatch")
	}
	set := map[string]bool{}
	for _, d := range source.Dates {
		set[d] = true
	}
	return set
}()

func isJapanSettlementDate(d time.Time) bool {
	first := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	for _, day := range []int{5, 10, 15, 20, 25, last} {
		candidate := time.Date(d.Year(), d.Month(), day, 0, 0, 0, 0, time.UTC)
		if candidate.Weekday() == time.Saturday {
			candidate = candidate.AddDate(0, 0, -1)
		}
		if candidate.Weekday() == time.Sunday {
			candidate = candidate.AddDate(0, 0, -2)
		}
		if !candidate.Equal(d) {
			continue
		}
		key := candidate.Format("2006-01-02")
		if candidate.Month() == time.January && candidate.Day() <= 3 || candidate.Month() == time.December && candidate.Day() == 31 || japanHolidayDates[key] {
			return false
		}
		return true
	}
	if japanHolidayDates[d.Format("2006-01-02")] {
		return false
	}
	return false
}

func scheduledDateRange(seriesMin, seriesMax float64) (time.Time, time.Time, error) {
	if seriesMin <= 0 || seriesMax < seriesMin {
		return time.Time{}, time.Time{}, fmt.Errorf("scheduled trade requires valid UTC timestamps")
	}
	from := time.UnixMilli(int64(seriesMin)).UTC()
	to := time.UnixMilli(int64(seriesMax)).UTC()
	lo, _ := time.Parse("2006-01-02", japanCalendarFrom)
	if from.Before(lo) || !to.Before(japanCalendarEndExclusive()) {
		return time.Time{}, time.Time{}, fmt.Errorf("scheduled data falls outside reviewed Japan calendar coverage %s..%s", japanCalendarFrom, japanCalendarThrough)
	}
	return from, to, nil
}
