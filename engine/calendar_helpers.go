package engine

import "github.com/spik3r/heisentick-strat/contextcols"

const localOffsetMS int64 = 10 * contextcols.HourMS

func localDayKey(t float64) int64 {
	return floorDivInt64(int64(t)+localOffsetMS, contextcols.DayMS)
}

func localWeekKey(t float64) int64 {
	return floorDivInt64(localDayKey(t)+3, 7)
}

func floorDivInt64(a int64, b int64) int64 {
	q := a / b
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}
