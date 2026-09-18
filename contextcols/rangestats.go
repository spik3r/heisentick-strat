package contextcols

import "github.com/spik3r/heisentick-strat/marketdata"

const (
	rangeStatLive       int8 = 1
	rangeStatComplete   int8 = 2
	rangeStatNotStarted int8 = 3
)

type RangeStatEntry struct {
	Range   float64
	Avg     float64
	UsedPct float64
	Hi      float64
	Lo      float64
	State   int8
}

type RangeStatEntryColumns struct {
	Range   []float64
	Avg     []float64
	UsedPct []float64
	Hi      []float64
	Lo      []float64
	State   []int8
}

type RangeStatSessionColumns struct {
	Asia   RangeStatEntryColumns
	London RangeStatEntryColumns
	NY     RangeStatEntryColumns
}

type RangeStatColumns struct {
	Day     RangeStatEntryColumns
	Week    RangeStatEntryColumns
	Session RangeStatSessionColumns
	Window  RangeStatSessionColumns
}

type RangeStatsRow struct {
	Day     RangeStatEntry
	Week    RangeStatEntry
	Session RangeStatSessions
	Window  RangeStatSessions
}

type RangeStatSessions struct {
	Asia   RangeStatEntry
	London RangeStatEntry
	NY     RangeStatEntry
}

type rangeTracker struct {
	q          []float64
	sum        float64
	hi         float64
	lo         float64
	avgAtStart float64
	active     bool
	last       RangeStatEntry
	hasLast    bool
}

func makeRangeStatEntryColumns(n int) RangeStatEntryColumns {
	return RangeStatEntryColumns{
		Range:   nanSlice(n),
		Avg:     nanSlice(n),
		UsedPct: nanSlice(n),
		Hi:      nanSlice(n),
		Lo:      nanSlice(n),
		State:   make([]int8, n),
	}
}

func makeRangeStatSessionColumns(n int) RangeStatSessionColumns {
	return RangeStatSessionColumns{
		Asia:   makeRangeStatEntryColumns(n),
		London: makeRangeStatEntryColumns(n),
		NY:     makeRangeStatEntryColumns(n),
	}
}

func makeRangeStatColumns(n int) RangeStatColumns {
	return RangeStatColumns{
		Day:     makeRangeStatEntryColumns(n),
		Week:    makeRangeStatEntryColumns(n),
		Session: makeRangeStatSessionColumns(n),
		Window:  makeRangeStatSessionColumns(n),
	}
}

func ComputeRangeStats(series marketdata.Series, lookback int) []RangeStatsRow {
	n := series.Len()
	out := make([]RangeStatsRow, n)
	dayT := rangeTracker{}
	weekT := rangeTracker{}
	sessionT := RangeStatTrackers{}
	windowT := RangeStatTrackers{}
	var curDay, curWeek int64
	hasDay := false
	hasWeek := false

	for i := 0; i < n; i++ {
		t := int64(series.T[i])
		dayKey := UTCDayKey(t)
		weekKey := UTCWeekKey(t)
		hourUTC := HourUTC(t)
		windowName := TradeWindowName(t)

		if !hasDay || dayKey != curDay {
			if hasDay {
				dayT.finalize(lookback)
			}
			curDay = dayKey
			hasDay = true
			out[i].Day = dayT.live(series, i, true)
		} else {
			out[i].Day = dayT.live(series, i, false)
		}

		if !hasWeek || weekKey != curWeek {
			if hasWeek {
				weekT.finalize(lookback)
			}
			curWeek = weekKey
			hasWeek = true
			out[i].Week = weekT.live(series, i, true)
		} else {
			out[i].Week = weekT.live(series, i, false)
		}

		out[i].Session.Asia = sessionT.Asia.step(series, i, lookback, hourUTC >= sessionAsia.start && hourUTC < sessionAsia.end)
		out[i].Session.London = sessionT.London.step(series, i, lookback, hourUTC >= sessionLondon.start && hourUTC < sessionLondon.end)
		out[i].Session.NY = sessionT.NY.step(series, i, lookback, hourUTC >= sessionNY.start && hourUTC < sessionNY.end)
		out[i].Window.Asia = windowT.Asia.step(series, i, lookback, windowName == "asia")
		out[i].Window.London = windowT.London.step(series, i, lookback, windowName == "london")
		out[i].Window.NY = windowT.NY.step(series, i, lookback, windowName == "ny")
	}
	return out
}

type RangeStatTrackers struct {
	Asia   rangeTracker
	London rangeTracker
	NY     rangeTracker
}

func UTCWeekKey(t int64) int64 {
	return floorDiv(UTCDayKey(t)+3, 7)
}

func TradeWindowName(t int64) string {
	h := LocalHour(t)
	switch {
	case h >= 9 && h < 12:
		return "asia"
	case h >= 12 && h < 16:
		return "mid"
	case h >= 16 && h < 19:
		return "london"
	case h >= 21 && h < 24:
		return "ny"
	default:
		return ""
	}
}

func (t *rangeTracker) avg() float64 {
	if len(t.q) == 0 {
		return nan()
	}
	return t.sum / float64(len(t.q))
}

func (t *rangeTracker) live(series marketdata.Series, i int, fresh bool) RangeStatEntry {
	if fresh {
		t.hi = series.H[i]
		t.lo = series.L[i]
		t.avgAtStart = t.avg()
	} else {
		t.hi = max(t.hi, series.H[i])
		t.lo = min(t.lo, series.L[i])
	}
	r := t.hi - t.lo
	return RangeStatEntry{Range: r, Avg: t.avgAtStart, UsedPct: rangeUsedPct(r, t.avgAtStart), Hi: t.hi, Lo: t.lo, State: rangeStatLive}
}

func (t *rangeTracker) finalize(lookback int) {
	r := t.hi - t.lo
	t.last = RangeStatEntry{Range: r, Avg: t.avgAtStart, UsedPct: rangeUsedPct(r, t.avgAtStart), Hi: t.hi, Lo: t.lo, State: rangeStatComplete}
	t.hasLast = true
	t.q = append(t.q, r)
	t.sum += r
	if len(t.q) > lookback {
		t.sum -= t.q[0]
		t.q = t.q[1:]
	}
}

func (t *rangeTracker) step(series marketdata.Series, i int, lookback int, active bool) RangeStatEntry {
	if active {
		fresh := !t.active
		t.active = true
		return t.live(series, i, fresh)
	}
	if t.active {
		t.finalize(lookback)
		t.active = false
	}
	if t.hasLast {
		return t.last
	}
	return RangeStatEntry{Range: nan(), Avg: nan(), UsedPct: nan(), Hi: nan(), Lo: nan(), State: rangeStatNotStarted}
}

func rangeUsedPct(r float64, avg float64) float64 {
	if isFinite(avg) && avg > 0 && isFinite(r) {
		return (r / avg) * 100
	}
	return nan()
}

func writeRangeStats(cols RangeStatColumns, i int, row RangeStatsRow) {
	writeRangeStatEntry(cols.Day, i, row.Day)
	writeRangeStatEntry(cols.Week, i, row.Week)
	writeRangeStatEntry(cols.Session.Asia, i, row.Session.Asia)
	writeRangeStatEntry(cols.Session.London, i, row.Session.London)
	writeRangeStatEntry(cols.Session.NY, i, row.Session.NY)
	writeRangeStatEntry(cols.Window.Asia, i, row.Window.Asia)
	writeRangeStatEntry(cols.Window.London, i, row.Window.London)
	writeRangeStatEntry(cols.Window.NY, i, row.Window.NY)
}

func writeRangeStatEntry(cols RangeStatEntryColumns, i int, entry RangeStatEntry) {
	cols.Range[i] = entry.Range
	cols.Avg[i] = entry.Avg
	cols.UsedPct[i] = entry.UsedPct
	cols.Hi[i] = entry.Hi
	cols.Lo[i] = entry.Lo
	cols.State[i] = entry.State
}
