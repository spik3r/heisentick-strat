package engine

import "github.com/spik3r/heisentick-strat/contextcols"

func inFlagTradeWindow(t float64, p flagParams, minMinutesLeft float64) bool {
	if p.TradeWindowUnrestricted {
		return true
	}
	h := contextcols.LocalHour(int64(t))
	if p.UseAsiaWindow && h >= 9 && h < 12 && (12-h)*60 >= minMinutesLeft {
		return true
	}
	if p.UseMidWindow && h >= 12 && h < 16 && (16-h)*60 >= minMinutesLeft {
		return true
	}
	if p.UseLondonWindow && h >= 16 && h < 19 && (19-h)*60 >= minMinutesLeft {
		return true
	}
	if p.UseNYWindow && h >= 21 && h < 24 && (24-h)*60 >= minMinutesLeft {
		return true
	}
	return false
}

func inAdmittedTradeWindow(t float64, p flagParams, minMinutesLeft float64) bool {
	if p.TradeWindowUnrestricted {
		return true
	}
	h := contextcols.LocalHour(int64(t))
	if p.AdmitAsiaWindow && h >= 9 && h < 12 && (12-h)*60 >= minMinutesLeft {
		return true
	}
	if p.AdmitMidWindow && h >= 12 && h < 16 && (16-h)*60 >= minMinutesLeft {
		return true
	}
	if p.AdmitLondonWindow && h >= 16 && h < 19 && (19-h)*60 >= minMinutesLeft {
		return true
	}
	if p.AdmitNYWindow && h >= 21 && h < 24 && (24-h)*60 >= minMinutesLeft {
		return true
	}
	return false
}
