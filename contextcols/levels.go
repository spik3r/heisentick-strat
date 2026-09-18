package contextcols

import (
	"sort"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const overnightStartUTC = 20

type OHLCColumns struct {
	O []float64
	H []float64
	L []float64
	C []float64
}

type PriorSessionColumns struct {
	Asia   OHLCColumns
	London OHLCColumns
	NY     OHLCColumns
}

type CamarillaColumns struct {
	R3 []float64
	R4 []float64
	S3 []float64
	S4 []float64
}

type OvernightRangeColumns struct {
	High     []float64
	Low      []float64
	Mid      []float64
	Width    []float64
	WidthATR []float64
}

type sessionRun struct {
	Start int64
	End   int64
	O     float64
	H     float64
	L     float64
	C     float64
}

type sessionRunsByName struct {
	Asia   []sessionRun
	London []sessionRun
	NY     []sessionRun
}

type sessionRunPointers struct {
	Asia   int
	London int
	NY     int
}

func makeOHLCColumns(n int) OHLCColumns {
	return OHLCColumns{O: nanSlice(n), H: nanSlice(n), L: nanSlice(n), C: nanSlice(n)}
}

func makePriorSessionColumns(n int) PriorSessionColumns {
	return PriorSessionColumns{
		Asia:   makeOHLCColumns(n),
		London: makeOHLCColumns(n),
		NY:     makeOHLCColumns(n),
	}
}

func makeCamarillaColumns(n int) CamarillaColumns {
	return CamarillaColumns{R3: nanSlice(n), R4: nanSlice(n), S3: nanSlice(n), S4: nanSlice(n)}
}

func makeOvernightRangeColumns(n int) OvernightRangeColumns {
	return OvernightRangeColumns{
		High:     nanSlice(n),
		Low:      nanSlice(n),
		Mid:      nanSlice(n),
		Width:    nanSlice(n),
		WidthATR: nanSlice(n),
	}
}

func sortedDayKeys(days map[int64]daySummary) []int64 {
	keys := make([]int64, 0, len(days))
	for key := range days {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func previousDays(days map[int64]daySummary) map[int64]daySummary {
	keys := sortedDayKeys(days)
	prev := make(map[int64]daySummary, len(keys))
	for i := 1; i < len(keys); i++ {
		prev[keys[i]] = days[keys[i-1]]
	}
	return prev
}

func previousPreviousDays(days map[int64]daySummary) map[int64]daySummary {
	keys := sortedDayKeys(days)
	prevPrev := make(map[int64]daySummary, len(keys))
	for i := 2; i < len(keys); i++ {
		prevPrev[keys[i]] = days[keys[i-2]]
	}
	return prevPrev
}

func priorDayRangeAverages(days map[int64]daySummary, lookback int) map[int64]float64 {
	keys := sortedDayKeys(days)
	out := make(map[int64]float64, len(keys))
	window := make([]float64, 0, lookback)
	sum := 0.0
	for _, key := range keys {
		if len(window) > 0 {
			out[key] = sum / float64(len(window))
		}
		day := days[key]
		r := day.H - day.L
		window = append(window, r)
		sum += r
		if len(window) > lookback {
			sum -= window[0]
			window = window[1:]
		}
	}
	return out
}

func writeCamarilla(cols CamarillaColumns, i int, prior daySummary) {
	r := prior.H - prior.L
	if !isFinite(r) || r <= 0 || !isFinite(prior.C) {
		return
	}
	cols.R3[i] = prior.C + r*1.1/4
	cols.R4[i] = prior.C + r*1.1/2
	cols.S3[i] = prior.C - r*1.1/4
	cols.S4[i] = prior.C - r*1.1/2
}

func buildSessionRuns(series marketdata.Series, days map[int64]daySummary) sessionRunsByName {
	keys := sortedDayKeys(days)
	return sessionRunsByName{
		Asia:   collectSessionRuns(series, days, keys, sessionAsia),
		London: collectSessionRuns(series, days, keys, sessionLondon),
		NY:     collectSessionRuns(series, days, keys, sessionNY),
	}
}

func collectSessionRuns(series marketdata.Series, days map[int64]daySummary, keys []int64, window sessionWindow) []sessionRun {
	runs := make([]sessionRun, 0, len(keys))
	for _, key := range keys {
		day := days[key]
		start := key*DayMS + int64(window.start*float64(HourMS))
		end := key*DayMS + int64(window.end*float64(HourMS))
		var run sessionRun
		has := false
		for i := day.FirstIdx; i <= day.LastIdx; i++ {
			t := int64(series.T[i])
			if t < start || t >= end {
				continue
			}
			if !has {
				run = sessionRun{Start: start, End: end, O: series.O[i], H: series.H[i], L: series.L[i], C: series.C[i]}
				has = true
				continue
			}
			run.H = max(run.H, series.H[i])
			run.L = min(run.L, series.L[i])
			run.C = series.C[i]
		}
		if has {
			runs = append(runs, run)
		}
	}
	return runs
}

func writePriorSessions(cols PriorSessionColumns, i int, t int64, runs sessionRunsByName, ptr sessionRunPointers) sessionRunPointers {
	ptr.Asia = writePriorSession(cols.Asia, i, t, runs.Asia, ptr.Asia)
	ptr.London = writePriorSession(cols.London, i, t, runs.London, ptr.London)
	ptr.NY = writePriorSession(cols.NY, i, t, runs.NY, ptr.NY)
	return ptr
}

func writePriorSession(cols OHLCColumns, i int, t int64, runs []sessionRun, ptr int) int {
	for ptr+1 < len(runs) && runs[ptr+1].End <= t {
		ptr++
	}
	if ptr >= 0 {
		run := runs[ptr]
		cols.O[i] = run.O
		cols.H[i] = run.H
		cols.L[i] = run.L
		cols.C[i] = run.C
	}
	return ptr
}

func buildOvernightRuns(series marketdata.Series, dayKeys []int64) []sessionRun {
	runs := make([]sessionRun, 0, len(dayKeys))
	ptr := 0
	segmentLen := int64(12 * HourMS)
	for _, key := range dayKeys {
		start := key*DayMS + overnightStartUTC*HourMS
		end := start + segmentLen
		for ptr < series.Len() && int64(series.T[ptr]) < start {
			ptr++
		}
		has := false
		var run sessionRun
		for j := ptr; j < series.Len() && int64(series.T[j]) < end; j++ {
			if !has {
				run = sessionRun{Start: start, End: end, O: series.O[j], H: series.H[j], L: series.L[j], C: series.C[j]}
				has = true
				continue
			}
			run.H = max(run.H, series.H[j])
			run.L = min(run.L, series.L[j])
			run.C = series.C[j]
		}
		if has {
			runs = append(runs, run)
		}
	}
	return runs
}

func writeOvernightRange(cols OvernightRangeColumns, i int, t int64, atr float64, runs []sessionRun, ptr int) int {
	for ptr+1 < len(runs) && runs[ptr+1].End <= t {
		ptr++
	}
	if ptr < 0 {
		return ptr
	}
	run := runs[ptr]
	width := run.H - run.L
	cols.High[i] = run.H
	cols.Low[i] = run.L
	cols.Mid[i] = (run.H + run.L) / 2
	cols.Width[i] = width
	if isFinite(atr) && atr > 0 {
		cols.WidthATR[i] = width / atr
	}
	return ptr
}
