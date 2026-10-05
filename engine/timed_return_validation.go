package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

var timedHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func isTimedReturn(cfg dsl.Config) bool {
	return cfg != nil && cfg["setupType"] == string(dsl.FamilyTimedReturn)
}

// A supplied calendar is an explicit timed-return request, never ignorable
// metadata on another family. Check this before generic preparation/execution.
func validateTimedCalendarFamily(cfg dsl.Config, calendar *TimedReturnCalendar) error {
	if calendar != nil && !isTimedReturn(cfg) {
		return fmt.Errorf("timed calendar requires the timed-return family")
	}
	return nil
}

func timedDuration(tf string) (int64, error) {
	minutes := map[string]int64{"1m": 1, "5m": 5, "15m": 15, "30m": 30, "1h": 60}
	m, ok := minutes[tf]
	if !ok {
		return 0, fmt.Errorf("unsupported timed-return timeframe %q", tf)
	}
	return m * 60000, nil
}

func timedDate(s string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", s)
	if err != nil || d.Format("2006-01-02") != s || d.Year() < 2000 || d.Year() > 2099 {
		return time.Time{}, fmt.Errorf("invalid timed calendar date %q", s)
	}
	return d, nil
}

func validateTimedSeries(s marketdata.Series, delta int64) error {
	if s.Len() == 0 {
		return fmt.Errorf("timed series is empty")
	}
	if err := validateSeriesShape("timed", s); err != nil {
		return err
	}
	if len(s.V) != 0 && len(s.V) != s.Len() {
		return fmt.Errorf("timed volume length mismatch")
	}
	for i, t := range s.T {
		if !isFinite(t) || t <= 0 || t > 4102444800000 || math.Trunc(t) != t || int64(t)%delta != 0 || i > 0 && t <= s.T[i-1] {
			return fmt.Errorf("timed invalid/duplicate/unordered timestamp at %d", i)
		}
		for _, v := range []float64{s.O[i], s.H[i], s.L[i], s.C[i]} {
			if !isFinite(v) || v <= 0 {
				return fmt.Errorf("timed nonpositive/nonfinite OHLC at %d", i)
			}
		}
		if s.L[i] > s.H[i] || s.O[i] < s.L[i] || s.O[i] > s.H[i] || s.C[i] < s.L[i] || s.C[i] > s.H[i] {
			return fmt.Errorf("timed invalid OHLC at %d", i)
		}
		if len(s.V) > 0 && (!isFinite(s.V[i]) || s.V[i] < 0) {
			return fmt.Errorf("timed invalid volume at %d", i)
		}
	}
	return nil
}

func validateTimedCosts(c Costs) error {
	if c.FillOn != "nextOpen" {
		return fmt.Errorf("timed return requires explicit costs.fillOn=nextOpen")
	}
	if c.FeePerUnit != 0 {
		return fmt.Errorf("timed return requires feePerUnit=0; commission accounting is unsupported")
	}
	if !isFinite(c.Slippage) || !isFinite(c.SlippageBps) || c.Slippage < 0 || c.SlippageBps < 0 || !isFinite(c.StartEquity) || c.StartEquity < 0 {
		return fmt.Errorf("invalid timed execution costs")
	}
	return nil
}

func validateTimedCalendar(c *TimedReturnCalendar, zone string) (*time.Location, map[string]int, error) {
	if c == nil || c.Schema != "timed-return-calendar-v1" || c.Timezone != zone {
		return nil, nil, fmt.Errorf("matching timed-return-calendar-v1 sidecar required")
	}
	for _, h := range []string{c.ReferenceSourceSHA256, c.ExecutionSourceSHA256, c.TimezoneDataSHA256} {
		if !timedHash.MatchString(h) {
			return nil, nil, fmt.Errorf("timed calendar requires explicit SHA-256 identities")
		}
	}
	if len(c.TimezoneData) == 0 || len(c.TimezoneData) > 65536 {
		return nil, nil, fmt.Errorf("timed TZif size must be 1..65536 bytes")
	}
	h := sha256.Sum256(c.TimezoneData)
	if hex.EncodeToString(h[:]) != c.TimezoneDataSHA256 {
		return nil, nil, fmt.Errorf("timed TZif hash mismatch")
	}
	loc, err := time.LoadLocationFromTZData(zone, c.TimezoneData)
	if err != nil {
		return nil, nil, fmt.Errorf("timed TZif: %w", err)
	}
	from, err := timedDate(c.TradeFromDate)
	if err != nil {
		return nil, nil, err
	}
	to, err := timedDate(c.TradeToDateExclusive)
	// Only the exclusive upper bound may fall just outside the supported years.
	if c.TradeToDateExclusive == "2100-01-01" {
		to, err = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), nil
	}
	if err != nil || !from.Before(to) {
		return nil, nil, fmt.Errorf("invalid bounded timed trade dates")
	}
	if len(c.Sessions) == 0 || len(c.Sessions) > 36600 {
		return nil, nil, fmt.Errorf("timed calendar row count is invalid")
	}
	indices := map[string]int{}
	previousOpen := ""
	var last time.Time
	for i, row := range c.Sessions {
		d, err := timedDate(row.Date)
		if err != nil {
			return nil, nil, err
		}
		if i > 0 && !d.Equal(last.AddDate(0, 0, 1)) {
			return nil, nil, fmt.Errorf("timed calendar must contain consecutive dates at %s", row.Date)
		}
		last = d
		indices[row.Date] = i
		switch row.Kind {
		case "closed":
			if row.Open != "" || row.Close != "" || row.PreviousSessionDate != "" {
				return nil, nil, fmt.Errorf("closed reference row has session fields: %s", row.Date)
			}
		case "full", "early":
			if row.PreviousSessionDate != previousOpen {
				return nil, nil, fmt.Errorf("reference predecessor mismatch: %s", row.Date)
			}
			o, e := timedResolve(row.Date, row.Open, loc)
			if e != nil {
				return nil, nil, e
			}
			cl, e := timedResolve(row.Date, row.Close, loc)
			if e != nil || o >= cl {
				return nil, nil, fmt.Errorf("invalid reference session: %s", row.Date)
			}
			previousOpen = row.Date
		default:
			return nil, nil, fmt.Errorf("unknown reference kind: %s", row.Date)
		}
	}
	if _, ok := indices[c.TradeFromDate]; !ok {
		return nil, nil, fmt.Errorf("trade start outside calendar")
	}
	if _, ok := indices[to.AddDate(0, 0, -1).Format("2006-01-02")]; !ok {
		return nil, nil, fmt.Errorf("trade end outside calendar")
	}
	if len(c.QuotedIntervals) == 0 || len(c.QuotedIntervals) > 200000 {
		return nil, nil, fmt.Errorf("quoted execution intervals required")
	}
	var previousEnd int64
	for _, q := range c.QuotedIntervals {
		if q.FromT <= 0 || q.ToT > 4102444800000 || q.FromT >= q.ToT || q.FromT < previousEnd {
			return nil, nil, fmt.Errorf("quoted intervals must be sorted nonoverlapping half-open UTC intervals")
		}
		previousEnd = q.ToT
	}
	return loc, indices, nil
}

func timedResolve(date, clock string, loc *time.Location) (int64, error) {
	m, err := dsl.TimedClockMinute(clock)
	if err != nil {
		return 0, err
	}
	return marketdata.ResearchLocalMinuteInLocation(date, loc, m)
}

func timedQuoted(c *TimedReturnCalendar, t int64) bool {
	i := sort.Search(len(c.QuotedIntervals), func(i int) bool { return c.QuotedIntervals[i].ToT > t })
	return i < len(c.QuotedIntervals) && c.QuotedIntervals[i].FromT <= t
}

func timedIndex(s marketdata.Series, t int64) int {
	i := sort.Search(len(s.T), func(i int) bool { return s.T[i] >= float64(t) })
	if i == len(s.T) || s.T[i] != float64(t) {
		return -1
	}
	return i
}
