package dsl

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Clock range breakout shared contract (spec/dsl-spec-families/clockRangeBreakout.md).
// The parser and the engine both validate through these helpers so a hand-built
// config cannot reach the engine with values the parser would have rejected.

const (
	// ClockMinOffsetMinutes and ClockMaxOffsetMinutes bound the declared
	// fixed UTC offset: UTC-12:00 through UTC+14:00.
	ClockMinOffsetMinutes = -720
	ClockMaxOffsetMinutes = 840

	minutesPerDay = 24 * 60
)

// ClockRangeSpec is the decoded clock range breakout configuration. Minutes
// are minutes after midnight on the declared-clock range date.
type ClockRangeSpec struct {
	UTCOffsetMinutes int
	RangeStartMinute int
	RangeEndMinute   int
	ExpireMinute     int
	CloseMinute      int
	BufferPips       float64
	StopPercent      float64
	AllowLong        bool
	AllowShort       bool
}

// ResolvedExpiryMinute is the expiry as minutes after range-date midnight. A
// wall time earlier than range end belongs to the next clock day.
func (s ClockRangeSpec) ResolvedExpiryMinute() int {
	return resolveClockWallMinute(s.ExpireMinute, s.RangeEndMinute)
}

// ResolvedCloseMinute is the clock close as minutes after range-date midnight.
func (s ClockRangeSpec) ResolvedCloseMinute() int {
	return resolveClockWallMinute(s.CloseMinute, s.RangeEndMinute)
}

func resolveClockWallMinute(minute, rangeEnd int) int {
	if minute < rangeEnd {
		return minute + minutesPerDay
	}
	return minute
}

// Validate checks every ordering and bound the contract states. It does not
// check routes; see AlignmentError.
func (s ClockRangeSpec) Validate() error {
	if s.UTCOffsetMinutes < ClockMinOffsetMinutes || s.UTCOffsetMinutes > ClockMaxOffsetMinutes {
		return fmt.Errorf("clock offset must be within UTC-12:00 and UTC+14:00")
	}
	for _, field := range []struct {
		name   string
		minute int
	}{
		{"range start", s.RangeStartMinute}, {"range end", s.RangeEndMinute},
		{"orders expire", s.ExpireMinute}, {"close positions", s.CloseMinute},
	} {
		if field.minute < 0 || field.minute >= minutesPerDay {
			return fmt.Errorf("%s must be within 00:00 and 23:59", field.name)
		}
	}
	if s.RangeStartMinute >= s.RangeEndMinute {
		return fmt.Errorf("range start must be before range end on the same clock date")
	}
	if s.ExpireMinute == s.RangeEndMinute {
		return fmt.Errorf("orders expire must differ from range end; require range end < expiry <= close")
	}
	if s.CloseMinute == s.RangeEndMinute {
		return fmt.Errorf("close positions must differ from range end; require range end < expiry <= close")
	}
	if s.ResolvedExpiryMinute() > s.ResolvedCloseMinute() {
		return fmt.Errorf("orders expire must not be after close positions; require range end < expiry <= close")
	}
	if math.IsNaN(s.BufferPips) || math.IsInf(s.BufferPips, 0) || s.BufferPips < 0 {
		return fmt.Errorf("buffer must be finite and nonnegative")
	}
	if math.IsNaN(s.StopPercent) || math.IsInf(s.StopPercent, 0) || !(s.StopPercent > 0) || s.StopPercent > 100 {
		return fmt.Errorf("stop percent must be finite and within (0, 100]")
	}
	if !s.AllowLong && !s.AllowShort {
		return fmt.Errorf("at least one side must be allowed")
	}
	return nil
}

// ClockRangeTimeframeMinutes returns the bar duration in minutes for a
// timeframe label. Only whole-minute durations dividing 24 hours are
// supported, so grid alignment does not depend on the calendar date.
func ClockRangeTimeframeMinutes(timeframe string) (int, bool) {
	match := clockTimeframePattern.FindStringSubmatch(strings.ToLower(strings.TrimSpace(timeframe)))
	if match == nil {
		return 0, false
	}
	count, err := strconv.Atoi(match[1])
	if err != nil || count <= 0 || count > minutesPerDay {
		return 0, false
	}
	minutes := count
	switch match[2] {
	case "h":
		minutes = count * 60
	case "d":
		minutes = count * minutesPerDay
	}
	if minutes > minutesPerDay || minutesPerDay%minutes != 0 {
		return 0, false
	}
	return minutes, true
}

var clockTimeframePattern = regexp.MustCompile(`^([0-9]{1,4})(m|h|d)$`)

type clockBoundary struct {
	name   string
	minute int
}

func (s ClockRangeSpec) boundaries() []clockBoundary {
	return []clockBoundary{
		{"range start", s.RangeStartMinute},
		{"range end", s.RangeEndMinute},
		{"orders expire", s.ResolvedExpiryMinute()},
		{"close positions", s.ResolvedCloseMinute()},
	}
}

// AlignmentError names the first of range start, range end, expiry and close
// that is not on the UTC grid of timeframe, or nil when all four align.
// Declared-clock midnight is a whole UTC-minute offset away from UTC midnight;
// since every supported timeframe divides 24 hours, the check is date-free.
func (s ClockRangeSpec) AlignmentError(timeframe string) error {
	minutes, ok := ClockRangeTimeframeMinutes(timeframe)
	if !ok {
		return fmt.Errorf("unsupported timeframe %q: use a whole-minute timeframe that divides 24 hours, such as 1m, 5m, 15m, 30m, 1h or 4h", timeframe)
	}
	for _, boundary := range s.boundaries() {
		utcMinute := boundary.minute - s.UTCOffsetMinutes
		if ((utcMinute%minutes)+minutes)%minutes != 0 {
			return fmt.Errorf("%s %s (%s) is not aligned to the %s UTC bar grid", boundary.name, FormatClockWall(boundary.minute), FormatClockOffset(s.UTCOffsetMinutes), timeframe)
		}
	}
	return nil
}

// FormatClockWall renders minutes after midnight as HH:MM, wrapping the next
// clock day, for diagnostics.
func FormatClockWall(minute int) string {
	minute = ((minute % minutesPerDay) + minutesPerDay) % minutesPerDay
	return fmt.Sprintf("%02d:%02d", minute/60, minute%60)
}

// FormatClockOffset renders a fixed offset as UTC+H or UTC+H:MM.
func FormatClockOffset(offsetMinutes int) string {
	sign := "+"
	if offsetMinutes < 0 {
		sign = "-"
		offsetMinutes = -offsetMinutes
	}
	if offsetMinutes%60 == 0 {
		return fmt.Sprintf("UTC%s%d", sign, offsetMinutes/60)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, offsetMinutes/60, offsetMinutes%60)
}

// ReviewedInstrumentPip is the reviewed pip registry shared by the compiler
// and the engine. Fixed-pip money logic must fail closed on an absent symbol.
func ReviewedInstrumentPip(symbol string) (float64, bool) {
	switch symbol {
	case "XAUUSD":
		return 0.1, true
	case "USDJPY", "GBPJPY", "LIGHTCMDUSD", "BRENTCMDUSD", "BTCUSDT":
		return 0.01, true
	case "GASCMDUSD":
		return 0.001, true
	case "AUS200", "NAS100", "US500":
		return 1, true
	case "EURUSD", "GBPUSD", "AUDUSD", "EURGBP":
		return 0.0001, true
	default:
		return 0, false
	}
}

// ClockRangeRoute is one effective symbol/timeframe route. Symbol is empty
// when the strategy declares timeframes but no symbols.
type ClockRangeRoute struct {
	Symbol    string
	Timeframe string
}

func (r ClockRangeRoute) String() string {
	if r.Symbol == "" {
		return r.Timeframe
	}
	return r.Symbol + " " + r.Timeframe
}

// ClockRangeRoutes lists the effective routes of cfg: explicit slices, else
// the symbols x timeframes allowlists including the shared timeframe default.
func ClockRangeRoutes(cfg Config) []ClockRangeRoute {
	if slices, ok := cfg["slices"].([]any); ok && len(slices) > 0 {
		routes := make([]ClockRangeRoute, 0, len(slices))
		for _, raw := range slices {
			slice, _ := raw.(map[string]any)
			symbol, _ := slice["symbol"].(string)
			timeframe, _ := slice["tf"].(string)
			routes = append(routes, ClockRangeRoute{Symbol: symbol, Timeframe: timeframe})
		}
		return routes
	}
	symbols := clockStringList(cfg["symbols"])
	timeframes := clockStringList(cfg["timeframes"])
	if len(symbols) == 0 {
		symbols = []string{""}
	}
	routes := make([]ClockRangeRoute, 0, len(symbols)*len(timeframes))
	for _, symbol := range symbols {
		for _, timeframe := range timeframes {
			routes = append(routes, ClockRangeRoute{Symbol: symbol, Timeframe: timeframe})
		}
	}
	return routes
}

func clockStringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

// DecodeClockRangeBreakout strictly decodes the family fields of cfg and
// validates them. It rejects hand-built configs the parser would not emit.
func DecodeClockRangeBreakout(cfg Config) (ClockRangeSpec, error) {
	var spec ClockRangeSpec
	if cfg["setupType"] != string(FamilyClockRangeBreakout) {
		return spec, fmt.Errorf("clock range breakout setup required")
	}
	clock, ok := cfg["clock"].(map[string]any)
	if !ok {
		return spec, fmt.Errorf("clock range breakout requires clock.utcOffsetMinutes")
	}
	family, ok := cfg["clockRangeBreakout"].(map[string]any)
	if !ok {
		return spec, fmt.Errorf("clock range breakout requires clockRangeBreakout config")
	}
	stop, ok := cfg["stop"].(map[string]any)
	if !ok || stop["type"] != "percent" {
		return spec, fmt.Errorf("clock range breakout requires stop.type percent")
	}
	var err error
	if spec.UTCOffsetMinutes, err = clockConfigInt(clock, "utcOffsetMinutes"); err != nil {
		return spec, err
	}
	for _, field := range []struct {
		key  string
		into *int
	}{
		{"rangeStartMinute", &spec.RangeStartMinute}, {"rangeEndMinute", &spec.RangeEndMinute},
		{"expireMinute", &spec.ExpireMinute}, {"closeMinute", &spec.CloseMinute},
	} {
		if *field.into, err = clockConfigInt(family, field.key); err != nil {
			return spec, err
		}
	}
	if spec.BufferPips, err = clockConfigFloat(family, "bufferPips"); err != nil {
		return spec, err
	}
	if spec.StopPercent, err = clockConfigFloat(stop, "percent"); err != nil {
		return spec, err
	}
	long, longOK := clockConfigFlag(cfg["allowLong"])
	short, shortOK := clockConfigFlag(cfg["allowShort"])
	if !longOK || !shortOK {
		return spec, fmt.Errorf("allowLong and allowShort must be 0 or 1")
	}
	spec.AllowLong, spec.AllowShort = long, short
	return spec, spec.Validate()
}

func clockConfigInt(values map[string]any, key string) (int, error) {
	value, err := clockConfigFloat(values, key)
	if err != nil {
		return 0, err
	}
	if value != math.Trunc(value) || math.Abs(value) > 1e9 {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return int(value), nil
}

func clockConfigFloat(values map[string]any, key string) (float64, error) {
	var value float64
	switch typed := values[key].(type) {
	case int:
		value = float64(typed)
	case int64:
		value = float64(typed)
	case float64:
		value = typed
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s must be finite", key)
	}
	return value, nil
}

func clockConfigFlag(value any) (bool, bool) {
	switch typed := value.(type) {
	case int:
		return typed == 1, typed == 0 || typed == 1
	case int64:
		return typed == 1, typed == 0 || typed == 1
	case float64:
		return typed == 1, typed == 0 || typed == 1
	case bool:
		return typed, true
	default:
		return false, false
	}
}
