package marketdata

import (
	"fmt"
	"time"
)

// ResearchLocalMinute resolves an exact wall-clock minute for the current
// research clocks, not a trading calendar. It refuses normalization of invalid,
// nonexistent or ambiguous local times. The supported 2000-2099 clocks have
// only one-hour DST folds; other zones/epochs require a reviewed extension.
// The caller must pin the actual resolved IANA tzdb source/version/hash in its run manifest.
func ResearchLocalMinute(date, zone string, minuteOfDay int) (int64, error) {
	switch zone {
	case "UTC", "America/New_York", "Europe/London", "Asia/Tokyo":
	default:
		return 0, fmt.Errorf("unsupported research time zone %q", zone)
	}
	d, err := time.Parse("2006-01-02", date)
	if err != nil || d.Year() < 2000 || d.Year() > 2099 || minuteOfDay < 0 || minuteOfDay >= 24*60 {
		return 0, fmt.Errorf("invalid research date or minute")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return 0, fmt.Errorf("load research time zone: %w", err)
	}
	hour, minute := minuteOfDay/60, minuteOfDay%60
	t := time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, loc)
	wallMatches := func(candidate time.Time) bool {
		v := candidate.In(loc)
		return v.Year() == d.Year() && v.Month() == d.Month() && v.Day() == d.Day() &&
			v.Hour() == hour && v.Minute() == minute
	}
	if !wallMatches(t) {
		return 0, fmt.Errorf("nonexistent local research minute")
	}
	if wallMatches(t.Add(time.Hour)) || wallMatches(t.Add(-time.Hour)) {
		return 0, fmt.Errorf("ambiguous local research minute")
	}
	return t.UnixMilli(), nil
}
