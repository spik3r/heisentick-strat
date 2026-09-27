package engine

import "github.com/spik3r/heisentick-strat/contextcols"

// inFlagWholeSession is the dsl v8 `sessions(...)` gate: the whole session
// (UTC hours), not the 3-hour open window. It backs the top-level
// marketGatesOK path only (broker.marketGatesOK / family_helpers.go); the
// per-setup UseXWindow knobs some families read independently of the
// top-level gate keep their existing open-window meaning regardless of
// SessionScope (see the dsl v8 CHANGELOG/spec note for the scoping
// rationale). minMinutesLeft is measured against the whole session's end.
func inFlagWholeSession(t float64, p flagParams, minMinutesLeft float64) bool {
	if p.TradeWindowUnrestricted {
		return true
	}
	h := contextcols.HourUTC(int64(t))
	remaining := func(end float64) bool { return (end-h)*60 >= minMinutesLeft }
	inside := p.UseAsiaWindow && contextcols.WholeSessionWindow("asia", h) && remaining(8) ||
		p.UseLondonWindow && contextcols.WholeSessionWindow("london", h) && remaining(16) ||
		p.UseNYWindow && contextcols.WholeSessionWindow("ny", h) && remaining(21)
	return inside
}

func inFlagTradeWindow(t float64, p flagParams, minMinutesLeft float64) bool {
	if p.SessionScope == "whole" {
		return inFlagWholeSession(t, p, minMinutesLeft)
	}
	if p.TradeWindowUnrestricted {
		return true
	}
	h := contextcols.LocalHour(int64(t))
	inside := p.UseAsiaWindow && h >= 9 && h < 12 && (12-h)*60 >= minMinutesLeft ||
		p.UseMidWindow && h >= 12 && h < 16 && (16-h)*60 >= minMinutesLeft ||
		p.UseLondonWindow && h >= 16 && h < 19 && (19-h)*60 >= minMinutesLeft ||
		p.UseNYWindow && h >= 21 && h < 24 && (24-h)*60 >= minMinutesLeft
	return inside && inSegmentedTradeWindow(t, p, allowedWindows(p))
}

func inAdmittedTradeWindow(t float64, p flagParams, minMinutesLeft float64) bool {
	if p.TradeWindowUnrestricted {
		return true
	}
	h := contextcols.LocalHour(int64(t))
	allowed := map[string]bool{"asia": p.AdmitAsiaWindow, "mid": p.AdmitMidWindow, "london": p.AdmitLondonWindow, "ny": p.AdmitNYWindow}
	inside := p.AdmitAsiaWindow && h >= 9 && h < 12 && (12-h)*60 >= minMinutesLeft ||
		p.AdmitMidWindow && h >= 12 && h < 16 && (16-h)*60 >= minMinutesLeft ||
		p.AdmitLondonWindow && h >= 16 && h < 19 && (19-h)*60 >= minMinutesLeft ||
		p.AdmitNYWindow && h >= 21 && h < 24 && (24-h)*60 >= minMinutesLeft
	return inside && inSegmentedTradeWindow(t, p, allowed)
}
