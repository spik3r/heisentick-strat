package contextcols

import "github.com/spik3r/heisentick-strat/marketdata"

const (
	sessionBiasModeNone  int8 = 0
	sessionBiasModeUp    int8 = 1
	sessionBiasModeDown  int8 = 2
	sessionBiasModeMixed int8 = 3
	sessionBiasModePause int8 = 4
)

type SessionBiasEntry struct {
	Mode            int8
	Score           int8
	DisplacementATR float64
	BodyNetATR      float64
	VWAPSlopeATR    float64
	State           int8
}

type SessionBiasEntryColumns struct {
	Mode            []int8
	Score           []int8
	DisplacementATR []float64
	BodyNetATR      []float64
	VWAPSlopeATR    []float64
	State           []int8
}

type SessionBiasSessions struct {
	Asia   SessionBiasEntry
	London SessionBiasEntry
	NY     SessionBiasEntry
}

type SessionBiasSessionColumns struct {
	Asia   SessionBiasEntryColumns
	London SessionBiasEntryColumns
	NY     SessionBiasEntryColumns
}

type SessionBiasRow struct {
	Session SessionBiasSessions
	Window  SessionBiasSessions
}

type SessionBiasColumns struct {
	Session SessionBiasSessionColumns
	Window  SessionBiasSessionColumns
}

type biasTracker struct {
	active   bool
	open     float64
	bodyNet  float64
	pv       float64
	vol      float64
	prevVWAP float64
	last     SessionBiasEntry
	hasLast  bool
}

type biasTrackers struct {
	Asia   biasTracker
	London biasTracker
	NY     biasTracker
}

func makeSessionBiasEntryColumns(n int) SessionBiasEntryColumns {
	return SessionBiasEntryColumns{
		Mode:            make([]int8, n),
		Score:           make([]int8, n),
		DisplacementATR: nanSlice(n),
		BodyNetATR:      nanSlice(n),
		VWAPSlopeATR:    nanSlice(n),
		State:           make([]int8, n),
	}
}

func makeSessionBiasSessionColumns(n int) SessionBiasSessionColumns {
	return SessionBiasSessionColumns{
		Asia:   makeSessionBiasEntryColumns(n),
		London: makeSessionBiasEntryColumns(n),
		NY:     makeSessionBiasEntryColumns(n),
	}
}

func makeSessionBiasColumns(n int) SessionBiasColumns {
	return SessionBiasColumns{
		Session: makeSessionBiasSessionColumns(n),
		Window:  makeSessionBiasSessionColumns(n),
	}
}

func ComputeSessionBias(series marketdata.Series, atr []float64) []SessionBiasRow {
	n := series.Len()
	out := make([]SessionBiasRow, n)
	sessionT := biasTrackers{}
	windowT := biasTrackers{}
	for i := 0; i < n; i++ {
		t := int64(series.T[i])
		hourUTC := HourUTC(t)
		windowName := TradeWindowName(t)
		out[i].Session.Asia = sessionT.Asia.step(series, atr, i, hourUTC >= sessionAsia.start && hourUTC < sessionAsia.end)
		out[i].Session.London = sessionT.London.step(series, atr, i, hourUTC >= sessionLondon.start && hourUTC < sessionLondon.end)
		out[i].Session.NY = sessionT.NY.step(series, atr, i, hourUTC >= sessionNY.start && hourUTC < sessionNY.end)
		out[i].Window.Asia = windowT.Asia.step(series, atr, i, windowName == "asia")
		out[i].Window.London = windowT.London.step(series, atr, i, windowName == "london")
		out[i].Window.NY = windowT.NY.step(series, atr, i, windowName == "ny")
	}
	return out
}

func (t *biasTracker) step(series marketdata.Series, atr []float64, i int, active bool) SessionBiasEntry {
	if active {
		fresh := !t.active
		t.active = true
		entry := t.live(series, atr, i, fresh)
		t.last = entry
		t.hasLast = true
		return entry
	}
	if t.active {
		t.active = false
	}
	if t.hasLast {
		entry := t.last
		entry.State = rangeStatComplete
		return entry
	}
	return SessionBiasEntry{
		Mode:            sessionBiasModeNone,
		Score:           0,
		DisplacementATR: nan(),
		BodyNetATR:      nan(),
		VWAPSlopeATR:    nan(),
		State:           rangeStatNotStarted,
	}
}

func (t *biasTracker) live(series marketdata.Series, atr []float64, i int, fresh bool) SessionBiasEntry {
	vol := 1.0
	if isFinite(series.V[i]) && series.V[i] > 0 {
		vol = series.V[i]
	}
	if fresh {
		t.open = series.O[i]
		t.bodyNet = 0
		t.pv = 0
		t.vol = 0
		t.prevVWAP = nan()
	}
	t.bodyNet += series.C[i] - series.O[i]
	t.pv += series.C[i] * vol
	t.vol += vol
	vwap := nan()
	if t.vol > 0 {
		vwap = t.pv / t.vol
	}
	currentATR := atr[i]
	displacementATR := nan()
	bodyNetATR := nan()
	vwapSlopeATR := nan()
	if isFinite(currentATR) && currentATR > 0 {
		displacementATR = (series.C[i] - t.open) / currentATR
		bodyNetATR = t.bodyNet / currentATR
		if isFinite(t.prevVWAP) {
			vwapSlopeATR = (vwap - t.prevVWAP) / currentATR
		}
	}
	t.prevVWAP = vwap
	score := int8(0)
	if displacementATR > 0.25 {
		score++
	}
	if displacementATR < -0.25 {
		score--
	}
	if bodyNetATR > 0.2 {
		score++
	}
	if bodyNetATR < -0.2 {
		score--
	}
	if vwapSlopeATR > 0.02 {
		score++
	}
	if vwapSlopeATR < -0.02 {
		score--
	}
	return SessionBiasEntry{
		Mode:            biasMode(score, displacementATR, bodyNetATR),
		Score:           score,
		DisplacementATR: displacementATR,
		BodyNetATR:      bodyNetATR,
		VWAPSlopeATR:    vwapSlopeATR,
		State:           rangeStatLive,
	}
}

func biasMode(score int8, displacementATR float64, bodyNetATR float64) int8 {
	if score >= 2 {
		return sessionBiasModeUp
	}
	if score <= -2 {
		return sessionBiasModeDown
	}
	if absOrZero(displacementATR) < 0.15 && absOrZero(bodyNetATR) < 0.15 {
		return sessionBiasModePause
	}
	return sessionBiasModeMixed
}

func absOrZero(value float64) float64 {
	if !isFinite(value) {
		return 0
	}
	if value < 0 {
		return -value
	}
	return value
}

func writeSessionBias(cols SessionBiasColumns, i int, row SessionBiasRow) {
	writeSessionBiasEntry(cols.Session.Asia, i, row.Session.Asia)
	writeSessionBiasEntry(cols.Session.London, i, row.Session.London)
	writeSessionBiasEntry(cols.Session.NY, i, row.Session.NY)
	writeSessionBiasEntry(cols.Window.Asia, i, row.Window.Asia)
	writeSessionBiasEntry(cols.Window.London, i, row.Window.London)
	writeSessionBiasEntry(cols.Window.NY, i, row.Window.NY)
}

func writeSessionBiasEntry(cols SessionBiasEntryColumns, i int, entry SessionBiasEntry) {
	cols.Mode[i] = entry.Mode
	cols.Score[i] = entry.Score
	cols.DisplacementATR[i] = entry.DisplacementATR
	cols.BodyNetATR[i] = entry.BodyNetATR
	cols.VWAPSlopeATR[i] = entry.VWAPSlopeATR
	cols.State[i] = entry.State
}
