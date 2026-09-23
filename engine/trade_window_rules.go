package engine

import "github.com/spik3r/heisentick-strat/contextcols"

func inFlagTradeWindow(t float64, p flagParams, minMinutesLeft float64) bool {
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
