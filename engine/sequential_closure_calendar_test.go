package engine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const calendarStep = int64(3_600_000)

// calendarJSON assembles a document with an independently computed digest.
func calendarJSON(fields map[string]string, pairs [][2]int64, hash string) string {
	canon := make([]string, len(pairs))
	items := make([]string, len(pairs))
	for i, p := range pairs {
		canon[i] = fmt.Sprintf("[%d,%d]", p[0], p[1])
		items[i] = fmt.Sprintf(`{"startMs":%d,"endMs":%d}`, p[0], p[1])
	}
	canonical := fmt.Sprintf(`{"schema":%s,"calendarId":%s,"provider":%s,"symbol":%s,"timeframe":%s,"version":%s,"closures":[%s]}`,
		fields["schema"], fields["calendarId"], fields["provider"], fields["symbol"], fields["timeframe"], fields["version"], strings.Join(canon, ","))
	if hash == "" {
		sum := sha256.Sum256([]byte(canonical))
		hash = hex.EncodeToString(sum[:])
	}
	return fmt.Sprintf(`{"schema":%s,"calendarId":%s,"provider":%s,"symbol":%s,"timeframe":%s,"version":%s,"closures":[%s],"sha256":"%s"}`,
		fields["schema"], fields["calendarId"], fields["provider"], fields["symbol"], fields["timeframe"], fields["version"], strings.Join(items, ","), hash)
}

func baseCalendarFields() map[string]string {
	return map[string]string{"schema": `"sequential-closure-calendar-v1"`, "calendarId": `"cal-1"`, "provider": `"venue-a"`, "symbol": `"XAUUSD"`, "timeframe": `"1h"`, "version": `"7"`}
}

func TestSequentialClosureCalendarDigestMatchesIndependentCanonicalForm(t *testing.T) {
	pairs := [][2]int64{{10 * calendarStep, 12 * calendarStep}, {40 * calendarStep, 44 * calendarStep}}
	c, err := engine.DecodeSequentialClosureCalendar(calendarJSON(baseCalendarFields(), pairs, ""))
	if err != nil {
		t.Fatal(err)
	}
	// The pinned digest of this exact canonical string, computed once with
	// `printf '%s' ... | shasum -a 256`, guards the canonical form itself.
	canonical := fmt.Sprintf(`{"schema":"sequential-closure-calendar-v1","calendarId":"cal-1","provider":"venue-a","symbol":"XAUUSD","timeframe":"1h","version":"7","closures":[[%d,%d],[%d,%d]]}`, pairs[0][0], pairs[0][1], pairs[1][0], pairs[1][1])
	sum := sha256.Sum256([]byte(canonical))
	if c.SHA256 != hex.EncodeToString(sum[:]) || engine.SequentialClosureCalendarSHA256(c) != c.SHA256 {
		t.Fatalf("digest %s, canonical %s", c.SHA256, hex.EncodeToString(sum[:]))
	}
	empty, err := engine.DecodeSequentialClosureCalendar(calendarJSON(baseCalendarFields(), nil, ""))
	if err != nil || len(empty.Closures) != 0 {
		t.Fatalf("empty calendar: %v", err)
	}
}

func TestSequentialClosureCalendarRefusals(t *testing.T) {
	good := [][2]int64{{10 * calendarStep, 12 * calendarStep}, {40 * calendarStep, 44 * calendarStep}}
	with := func(key, value string) map[string]string { f := baseCalendarFields(); f[key] = value; return f }
	valid := calendarJSON(baseCalendarFields(), good, "")
	cases := []struct {
		name, doc, field string
	}{
		{"empty", "", "calendar.size"},
		{"not JSON", "{", "calendar.json"},
		{"trailing value", valid + " {}", "calendar.trailing"},
		{"duplicate key", strings.Replace(valid, `"version":"7",`, `"version":"7","version":"8",`, 1), "calendar.duplicate-key"},
		{"unknown field", strings.Replace(valid, `"version":"7",`, `"version":"7","tz":"UTC",`, 1), "calendar.json"},
		{"null", strings.Replace(valid, `"provider":"venue-a"`, `"provider":null`, 1), "calendar.null"},
		{"fractional ms", strings.Replace(valid, fmt.Sprint(10*calendarStep), fmt.Sprint(10*calendarStep)+".5", 1), "calendar.number"},
		{"exponent form of an integer", strings.Replace(valid, fmt.Sprint(10*calendarStep), "3.6e7", 1), "calendar.json"},
		{"ms as string", strings.Replace(valid, fmt.Sprintf(`"startMs":%d`, 10*calendarStep), fmt.Sprintf(`"startMs":"%d"`, 10*calendarStep), 1), "calendar.json"},
		{"wrong schema", calendarJSON(with("schema", `"sequential-closure-calendar-v2"`), good, ""), "calendar.schema"},
		{"bad id characters", calendarJSON(with("calendarId", `"cal 1"`), good, ""), "calendar.calendarId"},
		{"empty provider", calendarJSON(with("provider", `""`), good, ""), "calendar.provider"},
		{"lowercase symbol", calendarJSON(with("symbol", `"xauusd"`), good, ""), "calendar.symbol"},
		{"two hour timeframe", calendarJSON(with("timeframe", `"2h"`), good, ""), "calendar.timeframe"},
		{"empty interval", calendarJSON(baseCalendarFields(), [][2]int64{{5 * calendarStep, 5 * calendarStep}}, ""), "calendar.closures.interval"},
		{"reversed interval", calendarJSON(baseCalendarFields(), [][2]int64{{6 * calendarStep, 5 * calendarStep}}, ""), "calendar.closures.interval"},
		{"negative start", calendarJSON(baseCalendarFields(), [][2]int64{{-calendarStep, calendarStep}}, ""), "calendar.number"},
		{"half step length", calendarJSON(baseCalendarFields(), [][2]int64{{5 * calendarStep, 6*calendarStep + calendarStep/2}}, ""), "calendar.closures.grid"},
		{"unsorted", calendarJSON(baseCalendarFields(), [][2]int64{good[1], good[0]}, ""), "calendar.closures.order"},
		{"overlapping", calendarJSON(baseCalendarFields(), [][2]int64{{10 * calendarStep, 14 * calendarStep}, {12 * calendarStep, 16 * calendarStep}}, ""), "calendar.closures.order"},
		{"touching is one interval", calendarJSON(baseCalendarFields(), [][2]int64{{10 * calendarStep, 12 * calendarStep}, {12 * calendarStep, 14 * calendarStep}}, ""), "calendar.closures.order"},
		{"end beyond 2^53", calendarJSON(baseCalendarFields(), [][2]int64{{9_007_199_254_740_000, 9_007_199_254_740_992}}, ""), "calendar.number"},
		{"digest of other content", calendarJSON(baseCalendarFields(), good, strings.Repeat("0", 64)), "calendar.sha256"},
		{"digest in capitals", calendarJSON(baseCalendarFields(), good, strings.ToUpper(strings.Split(strings.Split(valid, `"sha256":"`)[1], `"`)[0])), "calendar.sha256"},
		{"digest missing", strings.Replace(valid, `,"sha256"`, `,"sha257"`, 1), "calendar.json"},
		{"edited after hashing", strings.Replace(valid, `"version":"7"`, `"version":"8"`, 1), "calendar.sha256"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := engine.DecodeSequentialClosureCalendar(c.doc)
			var typed *engine.SequentialCalendarError
			if !errors.As(err, &typed) || typed.Kind != "invalid-sequential-calendar" || typed.Field != c.field {
				t.Fatalf("error %v, want invalid-sequential-calendar / %s", err, c.field)
			}
		})
	}
	// Oversize and over-count.
	if _, err := engine.DecodeSequentialClosureCalendar(strings.Repeat(" ", engine.SequentialCalendarMaxBytes+1)); err == nil {
		t.Fatal("oversize accepted")
	}
	many := make([][2]int64, 100001)
	for i := range many {
		many[i] = [2]int64{int64(2*i) * calendarStep, int64(2*i+1) * calendarStep}
	}
	if _, err := engine.DecodeSequentialClosureCalendar(calendarJSON(baseCalendarFields(), many, "")); err == nil {
		t.Fatal("100001 closures accepted")
	}
}

func hourBars(offsets ...int64) []marketdata.Bar {
	bars := make([]marketdata.Bar, len(offsets))
	for i, o := range offsets {
		bars[i] = marketdata.Bar{T: float64(o * calendarStep), O: 1, H: 1, L: 1, C: 1}
	}
	return bars
}

func TestSequentialClosureCalendarMatchesHolesExactly(t *testing.T) {
	decode := func(pairs ...[2]int64) engine.SequentialClosureCalendar {
		c, err := engine.DecodeSequentialClosureCalendar(calendarJSON(baseCalendarFields(), pairs, ""))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	hole := [2]int64{3 * calendarStep, 10 * calendarStep} // bars 0,1,2 then 10: missing 3..9
	bars := hourBars(0, 1, 2, 10, 11)
	after, applied, err := decode(hole).MatchBars(bars, calendarStep)
	if err != nil || !after[3] || after[0] || after[4] || len(applied) != 1 || applied[0].MissingBars != 7 || applied[0].BeforeIndex != 3 {
		t.Fatalf("exact match: %v %v %+v", err, after, applied)
	}
	// Closures far outside the data are allowed and unused.
	if _, applied, err := decode(hole, [2]int64{100 * calendarStep, 110 * calendarStep}).MatchBars(bars, calendarStep); err != nil || len(applied) != 1 {
		t.Fatalf("closure outside the window: %v", err)
	}
	for name, c := range map[string]engine.SequentialClosureCalendar{
		"starts one step late":      decode([2]int64{4 * calendarStep, 10 * calendarStep}),
		"ends one step early":       decode([2]int64{3 * calendarStep, 9 * calendarStep}),
		"covers more than the hole": decode([2]int64{3 * calendarStep, 11 * calendarStep}),
		"starts before the hole":    decode([2]int64{2 * calendarStep, 10 * calendarStep}),
		"declares a different hole": decode([2]int64{20 * calendarStep, 30 * calendarStep}),
		"no closures":               decode(),
	} {
		_, _, err := c.MatchBars(bars, calendarStep)
		var gap *engine.SequentialFullExecutionError
		if !errors.As(err, &gap) || gap.Kind != "unsupported-sequential-time-gap" || gap.BarIndex != 3 {
			t.Fatalf("%s: %v, want an undeclared gap at bar 3", name, err)
		}
	}
	// One declared and one not: the refusal names the undeclared one.
	_, _, err = decode(hole).MatchBars(hourBars(0, 1, 2, 10, 11, 15), calendarStep)
	var gap *engine.SequentialFullExecutionError
	if !errors.As(err, &gap) || gap.BarIndex != 5 {
		t.Fatalf("second hole: %v", err)
	}
	// A declared closure may not contain an observed bar, even if the holes line up elsewhere.
	_, _, err = decode([2]int64{9 * calendarStep, 13 * calendarStep}).MatchBars(hourBars(0, 1, 2, 10, 11), calendarStep)
	var typed *engine.SequentialCalendarError
	if err == nil || (!errors.As(err, &typed) && !errors.As(err, &gap)) {
		t.Fatalf("closure containing bars: %v", err)
	}
	if _, _, err := decode([2]int64{10 * calendarStep, 11 * calendarStep}).MatchBars(bars, calendarStep); err == nil {
		t.Fatal("a closure over an observed bar was accepted")
	}
}
