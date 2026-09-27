package contextcols

import (
	"sync"
	"time"
)

const (
	HourMS      int64 = 3_600_000
	DayMS       int64 = 86_400_000
	localOffset int64 = 10 * HourMS
	phaseOther  int8  = 6
	phaseOpen   int8  = 1
	phaseMiddle int8  = 2
	phaseLunch  int8  = 3
	phaseClose  int8  = 4
	phaseNight  int8  = 5
)

type sessionWindow struct {
	start float64
	end   float64
}

var (
	sessionAsia   = sessionWindow{start: 0, end: 8}
	sessionLondon = sessionWindow{start: 7, end: 16}
	sessionNY     = sessionWindow{start: 12, end: 21}
)

// Whole-session (dsl v8 `sessions(...)`) hour boundaries, UTC. These match
// the hand-written strategy engine/strategies/dslDailySndRetestXauusdFourHour.js
// SESSION_HOURS exactly (asia 00:00-08:00, london 08:00-16:00, ny
// 13:00-21:00) and are deliberately a separate table from sessionAsia/
// sessionLondon/sessionNY above: that table backs prior-session H/L level
// and range/bias context (rangestats.go, sessionbias.go, levels.go,
// columns.go) and uses different boundaries (london 07-16, ny 12-21) for
// that unrelated purpose. Do not merge the two tables.
var (
	wholeSessionAsia   = sessionWindow{start: 0, end: 8}
	wholeSessionLondon = sessionWindow{start: 8, end: 16}
	wholeSessionNY     = sessionWindow{start: 13, end: 21}
)

// WholeSessionWindow reports whether hourUTC falls inside the named whole
// session (asia/london/ny). Unknown names return false.
func WholeSessionWindow(name string, hourUTC float64) bool {
	var w sessionWindow
	switch name {
	case "asia":
		w = wholeSessionAsia
	case "london":
		w = wholeSessionLondon
	case "ny":
		w = wholeSessionNY
	default:
		return false
	}
	return hourUTC >= w.start && hourUTC < w.end
}

// UTCDayKey matches engine/context/sessionContext.js.
func UTCDayKey(t int64) int64 {
	return floorDiv(t, DayMS)
}

// HourUTC returns the fractional UTC hour for a Unix-millisecond timestamp.
func HourUTC(t int64) float64 {
	mod := t % DayMS
	if mod < 0 {
		mod += DayMS
	}
	return float64(mod) / float64(HourMS)
}

// LocalHour matches engine/dsl/sessions.js (UTC+10 display-local time).
func LocalHour(t int64) float64 {
	mod := (t + localOffset) % DayMS
	if mod < 0 {
		mod += DayMS
	}
	return float64(mod) / float64(HourMS)
}

var newYorkLocationOnce sync.Once
var newYorkLocation *time.Location

// NewYorkHour matches engine/dsl/sessions.js newYorkHour: the wall-clock hour
// in America/New_York, including EST/EDT transitions.
func NewYorkHour(t int64) int {
	if t%HourMS != 0 {
		t -= t % HourMS
	}
	newYorkLocationOnce.Do(func() {
		loc, err := time.LoadLocation("America/New_York")
		if err != nil {
			loc = time.FixedZone("EST", -5*int(HourMS))
		}
		newYorkLocation = loc
	})
	return time.UnixMilli(t).In(newYorkLocation).Hour()
}

// SessionPhaseAtHour matches JS sessionPhaseAtHour.
func SessionPhaseAtHour(hourLocal float64) string {
	switch {
	case hourLocal < 0 || hourLocal >= 24:
		return "other"
	case hourLocal < 9:
		return "overnight"
	case hourLocal < 12:
		return "open"
	case hourLocal < 14:
		return "lunch"
	case hourLocal < 18:
		return "middle"
	case hourLocal < 24:
		return "close"
	default:
		return "other"
	}
}

// SessionPhaseCode maps phase names to the JS Int8 codes.
func SessionPhaseCode(phase string) int8 {
	switch phase {
	case "open":
		return phaseOpen
	case "middle":
		return phaseMiddle
	case "lunch":
		return phaseLunch
	case "close":
		return phaseClose
	case "overnight":
		return phaseNight
	default:
		return phaseOther
	}
}

func sessionFlag(hourUTC float64, window sessionWindow) int8 {
	if hourUTC >= window.start && hourUTC < window.end {
		return 1
	}
	return 0
}

func floorDiv(a int64, b int64) int64 {
	q := a / b
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}
