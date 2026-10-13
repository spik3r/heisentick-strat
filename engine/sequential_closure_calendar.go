package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// A closure calendar is the only way a hole in the bars is accepted. It is a
// caller-supplied, versioned, hashed document tied to a provider, a symbol and
// a timeframe. The engine never infers a closure from missing candles and never
// ships a calendar of its own. A hole is accepted only if one declared interval
// equals it exactly: it starts at the first missing bar open and ends at the
// next observed bar open. Any other hole refuses the whole run.
//
// Canonical hash input (compact JSON, this key order, ASCII identifiers, no
// "sha256" key): {"schema","calendarId","provider","symbol","timeframe",
// "version","closures":[[startMs,endMs],...]}. The document must carry the
// lowercase hex SHA-256 of those bytes.

const (
	SequentialClosureCalendarSchema = "sequential-closure-calendar-v1"
	// SequentialCalendarMaxBytes bounds one calendar document.
	SequentialCalendarMaxBytes    = 4 << 20
	sequentialCalendarMaxClosures = 100000
	sequentialCalendarMaxMS       = int64(1<<53 - 1)
)

var (
	sequentialCalendarIdent  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	sequentialCalendarSymbol = regexp.MustCompile(`^[A-Z][A-Z0-9_.-]{0,31}$`)
	sequentialCalendarHash   = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// SequentialClosure is the half-open UTC interval [StartMS, EndMS) of bar opens
// that the provider did not publish because the market was closed.
type SequentialClosure struct {
	StartMS int64 `json:"startMs"`
	EndMS   int64 `json:"endMs"`
}

type SequentialClosureCalendar struct {
	Schema     string              `json:"schema"`
	CalendarID string              `json:"calendarId"`
	Provider   string              `json:"provider"`
	Symbol     string              `json:"symbol"`
	Timeframe  string              `json:"timeframe"`
	Version    string              `json:"version"`
	Closures   []SequentialClosure `json:"closures"`
	SHA256     string              `json:"sha256"`
}

// SequentialCalendarError is a malformed or contradictory calendar. It is
// distinct from an undeclared gap, which stays a SequentialFullExecutionError.
type SequentialCalendarError struct {
	Kind  string `json:"kind"`
	Field string `json:"field"`
	Index int    `json:"index"`
}

func (e *SequentialCalendarError) Error() string {
	return fmt.Sprintf("%s: %s (index %d)", e.Kind, e.Field, e.Index)
}

func sequentialCalendarError(field string, index int) error {
	return &SequentialCalendarError{"invalid-sequential-calendar", field, index}
}

// SequentialClosureCalendarSHA256 returns the digest the document must carry.
func SequentialClosureCalendarSHA256(c SequentialClosureCalendar) string {
	pairs := make([][2]int64, len(c.Closures))
	for i, closure := range c.Closures {
		pairs[i] = [2]int64{closure.StartMS, closure.EndMS}
	}
	raw, _ := json.Marshal(struct {
		Schema     string     `json:"schema"`
		CalendarID string     `json:"calendarId"`
		Provider   string     `json:"provider"`
		Symbol     string     `json:"symbol"`
		Timeframe  string     `json:"timeframe"`
		Version    string     `json:"version"`
		Closures   [][2]int64 `json:"closures"`
	}{c.Schema, c.CalendarID, c.Provider, c.Symbol, c.Timeframe, c.Version, pairs})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// DecodeSequentialClosureCalendar parses and validates a calendar document. It
// refuses duplicate keys, unknown fields, fractional or oversize integers,
// unsorted or overlapping closures, closures that are not a whole number of
// timeframe steps, and a digest that does not match the content.
func DecodeSequentialClosureCalendar(raw string) (SequentialClosureCalendar, error) {
	var c SequentialClosureCalendar
	if len(raw) == 0 || len(raw) > SequentialCalendarMaxBytes {
		return c, sequentialCalendarError("calendar.size", -1)
	}
	if err := sequentialCalendarWalk([]byte(raw)); err != nil {
		return c, err
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return SequentialClosureCalendar{}, sequentialCalendarError("calendar.json", -1)
	}
	if c.Closures == nil {
		c.Closures = []SequentialClosure{}
	}
	if c.Schema != SequentialClosureCalendarSchema {
		return SequentialClosureCalendar{}, sequentialCalendarError("calendar.schema", -1)
	}
	for _, field := range []struct{ name, value string }{{"calendarId", c.CalendarID}, {"provider", c.Provider}, {"version", c.Version}} {
		if !sequentialCalendarIdent.MatchString(field.value) {
			return SequentialClosureCalendar{}, sequentialCalendarError("calendar."+field.name, -1)
		}
	}
	if !sequentialCalendarSymbol.MatchString(c.Symbol) {
		return SequentialClosureCalendar{}, sequentialCalendarError("calendar.symbol", -1)
	}
	step, ok := dsl.SequentialFullTimeframeMS(c.Timeframe)
	if !ok {
		return SequentialClosureCalendar{}, sequentialCalendarError("calendar.timeframe", -1)
	}
	if len(c.Closures) > sequentialCalendarMaxClosures {
		return SequentialClosureCalendar{}, sequentialCalendarError("calendar.closures.count", -1)
	}
	for i, closure := range c.Closures {
		switch {
		case closure.StartMS < 0 || closure.EndMS > sequentialCalendarMaxMS || closure.EndMS <= closure.StartMS:
			return SequentialClosureCalendar{}, sequentialCalendarError("calendar.closures.interval", i)
		case (closure.EndMS-closure.StartMS)%step != 0:
			return SequentialClosureCalendar{}, sequentialCalendarError("calendar.closures.grid", i)
		case i > 0 && closure.StartMS <= c.Closures[i-1].EndMS:
			// Touching or overlapping closures must be one interval.
			return SequentialClosureCalendar{}, sequentialCalendarError("calendar.closures.order", i)
		}
	}
	if !sequentialCalendarHash.MatchString(c.SHA256) || c.SHA256 != SequentialClosureCalendarSHA256(c) {
		return SequentialClosureCalendar{}, sequentialCalendarError("calendar.sha256", -1)
	}
	return c, nil
}

// sequentialCalendarKeys is the exact, case-sensitive grammar of a calendar
// document: these root keys and, inside each closure, these two keys. Go's
// encoding/json matches keys case-insensitively and would let "Closures" or
// "StartMs" stand in for, or override, the schema-defined field, so the
// grammar is enforced here on the decoded key strings before any struct decoding.
var (
	sequentialCalendarRootKeys    = []string{"schema", "calendarId", "provider", "symbol", "timeframe", "version", "closures", "sha256"}
	sequentialCalendarClosureKeys = []string{"startMs", "endMs"}
)

// sequentialCalendarWalk rejects what struct decoding would hide: duplicate or
// unknown or case-variant keys (including through unicode escapes, which the
// token decoder resolves first), missing required keys at the root and in every
// closure, values of the wrong JSON type, fractional or oversize integers, nulls
// and trailing values.
func sequentialCalendarWalk(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	// object reads one object whose keys must be exactly allowed (all required).
	// value(key) validates one member's value.
	var object func(path string, allowed []string, value func(key string) error) error
	object = func(path string, allowed []string, value func(key string) error) error {
		token, err := decoder.Token()
		if err != nil || token != json.Delim('{') {
			return sequentialCalendarError("calendar.json", -1)
		}
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok {
				return sequentialCalendarError("calendar.json", -1)
			}
			if seen[key] {
				return sequentialCalendarError("calendar.duplicate-key", -1)
			}
			seen[key] = true
			known := false
			for _, name := range allowed {
				known = known || name == key
			}
			if !known {
				return sequentialCalendarError("calendar.unknown-key", -1)
			}
			if err := value(key); err != nil {
				return err
			}
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
			return sequentialCalendarError("calendar.json", -1)
		}
		for _, name := range allowed {
			if !seen[name] {
				return sequentialCalendarError(path+"."+name, -1)
			}
		}
		return nil
	}
	str := func() error {
		token, err := decoder.Token()
		if _, ok := token.(string); err != nil || !ok {
			return sequentialCalendarError("calendar.type", -1)
		}
		return nil
	}
	integer := func() error {
		token, err := decoder.Token()
		number, ok := token.(json.Number)
		if err != nil {
			return sequentialCalendarError("calendar.json", -1)
		}
		if !ok {
			return sequentialCalendarError("calendar.type", -1)
		}
		if !sequentialExactJSONTimestamp(string(number)) {
			return sequentialCalendarError("calendar.number", -1)
		}
		return nil
	}
	closure := func() error {
		return object("calendar.closures", sequentialCalendarClosureKeys, func(string) error { return integer() })
	}
	closures := func() error {
		if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
			return sequentialCalendarError("calendar.type", -1)
		}
		for decoder.More() {
			if err := closure(); err != nil {
				return err
			}
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
			return sequentialCalendarError("calendar.json", -1)
		}
		return nil
	}
	err := object("calendar", sequentialCalendarRootKeys, func(key string) error {
		if key == "closures" {
			return closures()
		}
		return str()
	})
	if err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return sequentialCalendarError("calendar.trailing", -1)
	}
	return nil
}

// SequentialAppliedClosure is a declared closure that explained an observed hole.
type SequentialAppliedClosure struct {
	ClosureIndex int   `json:"closureIndex"`
	BeforeIndex  int   `json:"beforeIndex"`
	StartMS      int64 `json:"startMs"`
	EndMS        int64 `json:"endMs"`
	MissingBars  int   `json:"missingBars"`
}

// BindRoute checks the calendar belongs to the authored route.
func (c SequentialClosureCalendar) BindRoute(route SequentialMarketRoute) error {
	if c.Symbol != route.Symbol {
		return sequentialCalendarError("calendar.symbol", -1)
	}
	if c.Timeframe != route.Timeframe {
		return sequentialCalendarError("calendar.timeframe", -1)
	}
	return nil
}

// closureIndexFor returns the declared closure that equals the hole between two
// consecutive observed bar opens, or -1.
func (c SequentialClosureCalendar) closureIndexFor(previousOpen, open, step int64) int {
	start := previousOpen + step
	i := sort.Search(len(c.Closures), func(i int) bool { return c.Closures[i].StartMS >= start })
	if i < len(c.Closures) && c.Closures[i].StartMS == start && c.Closures[i].EndMS == open {
		return i
	}
	return -1
}

// MatchBars marks each bar that follows a declared closure. It returns a typed
// gap error for the first hole no closure equals, and a calendar error if a
// declared closure contains an observed bar (the bars then contradict it).
// bars must already be validated: strictly increasing integral opens at least
// one step apart.
func (c SequentialClosureCalendar) MatchBars(bars []marketdata.Bar, step int64) ([]bool, []SequentialAppliedClosure, error) {
	after := make([]bool, len(bars))
	applied := []SequentialAppliedClosure{}
	for i := 1; i < len(bars); i++ {
		previous, open := int64(bars[i-1].T), int64(bars[i].T)
		if open-previous <= step {
			continue
		}
		index := c.closureIndexFor(previous, open, step)
		if index < 0 {
			return nil, nil, &SequentialFullExecutionError{Kind: "unsupported-sequential-time-gap", Field: "series.timestamp", BarIndex: i}
		}
		after[i] = true
		applied = append(applied, SequentialAppliedClosure{index, i, c.Closures[index].StartMS, c.Closures[index].EndMS, int((open-previous)/step) - 1})
	}
	for k, closure := range c.Closures {
		first := sort.Search(len(bars), func(i int) bool { return int64(bars[i].T) >= closure.StartMS })
		if first < len(bars) && int64(bars[first].T) < closure.EndMS {
			return nil, nil, sequentialCalendarError("closures.contains-bar", k)
		}
	}
	return after, applied, nil
}
