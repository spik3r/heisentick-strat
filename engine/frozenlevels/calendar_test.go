package frozenlevels

import (
	"reflect"
	"testing"
	"time"
)

func syntheticCalendar() Calendar {
	c := Calendar{ID: "synthetic-broker-calendar", Version: "fixture-v1", Qualification: "invented explicit closure and transition ledger"}
	date, _ := time.Parse("2006-01-02", "2026-03-06")
	start := ms("2026-03-05T22:00:00Z")
	for i := 0; i < 6; i++ {
		dur := int64(24 * 60 * 60 * 1000)
		if i == 2 {
			dur -= 60 * 60 * 1000
		}
		status := TradingDay
		if i == 1 || i == 2 || i == 4 {
			status = ClosedDay
		}
		c.Days = append(c.Days, BrokerDay{Date: date.AddDate(0, 0, i).Format("2006-01-02"), StartMillis: start, EndMillis: start + dur, Status: status})
		start += dur
	}
	return c
}
func opening(c Calendar, date string, price float64) Opening {
	var day BrokerDay
	for _, d := range c.Days {
		if d.Date == date {
			day = d
			break
		}
	}
	return Opening{Date: date, Event: EventKey{AtMillis: day.StartMillis, Sequence: 1}, KnownAtMillis: day.StartMillis, Price: price, Source: source(), CalendarID: c.ID, CalendarVersion: c.Version, Qualification: "synthetic first qualified quote"}
}
func frozenOpening(t *testing.T, price float64) FrozenOpening {
	t.Helper()
	c := syntheticCalendar()
	f, e := FreezePriorOpening(c, ms("2026-03-09T00:00:00Z")-1, []Opening{opening(c, "2026-03-06", price)})
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func TestCalendarTransitionWeekendsHolidayAndPriorOpen(t *testing.T) {
	c := syntheticCalendar()
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ at, date string }{
		{"2026-03-08T20:59:59.999Z", "2026-03-08"}, {"2026-03-08T21:00:00Z", "2026-03-09"}, {"2026-03-09T21:00:00Z", "2026-03-10"},
	} {
		d, e := c.DayAt(ms(tc.at))
		if e != nil || d.Date != tc.date {
			t.Fatalf("day at %s: %+v %v", tc.at, d, e)
		}
	}
	o := opening(c, "2026-03-06", 3800)
	f, e := FreezePriorOpening(c, ms("2026-03-09T00:00:00Z"), []Opening{o})
	if e != nil || f.Opening.Date != "2026-03-06" {
		t.Fatalf("weekend: %+v %v", f, e)
	}
	o = opening(c, "2026-03-09", 3819)
	f, e = FreezePriorOpening(c, ms("2026-03-11T00:00:00Z"), []Opening{o})
	if e != nil || f.Opening.Date != "2026-03-09" {
		t.Fatalf("holiday: %+v %v", f, e)
	}
	// Current-day and future observations cannot substitute or change the reference.
	original := f
	f, e = FreezePriorOpening(c, ms("2026-03-11T00:00:00Z"), []Opening{opening(c, "2026-03-11", 99999), o})
	if e != nil || !reflect.DeepEqual(original, f) {
		t.Fatal("future context leakage")
	}
}
func TestCalendarMissingAndUnqualifiedInputs(t *testing.T) {
	c := syntheticCalendar()
	at := ms("2026-03-09T00:00:00Z")
	o := opening(c, "2026-03-06", 3800)
	for _, stamp := range []int64{c.Days[0].StartMillis - 1, c.Days[len(c.Days)-1].EndMillis, maxExactMillis + 1} {
		if _, e := c.DayAt(stamp); e == nil {
			t.Fatal("calendar extrapolated")
		}
	}
	if _, e := FreezePriorOpening(c, c.Days[0].StartMillis, []Opening{o}); e == nil {
		t.Fatal("invented earlier calendar")
	}
	if _, e := FreezePriorOpening(c, c.Days[1].StartMillis, []Opening{o}); e == nil {
		t.Fatal("traded closed day")
	}
	for _, list := range [][]Opening{nil, {opening(c, "2026-03-09", 3800)}, {o, o}} {
		if _, e := FreezePriorOpening(c, at, list); e == nil {
			t.Fatal("accepted missing or duplicated opening")
		}
	}
	for name, mutate := range map[string]func(*Opening){
		"late": func(o *Opening) { o.KnownAtMillis = at + 1 }, "wrong calendar": func(o *Opening) { o.CalendarVersion = "other" }, "no proof": func(o *Opening) { o.Qualification = "" }, "ask": func(o *Opening) { o.Source.Side = Ask }, "unknown side": func(o *Opening) { o.Source.Side = "trade" }, "wrong date time": func(o *Opening) { o.Event.AtMillis = at }, "negative price": func(o *Opening) { o.Price = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := o
			mutate(&bad)
			if _, e := FreezePriorOpening(c, at, []Opening{bad}); e == nil {
				t.Fatal("accepted unqualified opening")
			}
		})
	}
	for name, mutate := range map[string]func(*Calendar){
		"gap": func(c *Calendar) { c.Days = append(c.Days[:1], c.Days[2:]...) }, "overlap": func(c *Calendar) { c.Days[1].StartMillis-- }, "unknown day": func(c *Calendar) { c.Days[0].Status = "unknown" }, "no qualification": func(c *Calendar) { c.Qualification = "" }, "date gap": func(c *Calendar) { c.Days[1].Date = "2026-03-12" }, "wrong offset": func(c *Calendar) { c.Days[0].StartMillis -= 24 * 60 * 60 * 1000 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := syntheticCalendar()
			mutate(&bad)
			if e := bad.Validate(); e == nil {
				t.Fatal("accepted invalid calendar")
			}
		})
	}
	// A missing preceding TRADING day is not skipped like a verified closure.
	c.Days[2].Status = TradingDay
	if _, e := FreezePriorOpening(c, at, []Opening{o}); e == nil {
		t.Fatal("silently skipped missing trading day")
	}
}
func TestScaleCentHalfEvenAndGrid(t *testing.T) {
	cases := []struct {
		prior, entry, grid float64
		side               Direction
		distance, stop     float64
	}{
		{3800, 100, .01, Long, 1, 101}, {3800, 100, .01, Short, 1, 99},
		{3819, 100, .01, Long, 1, 101},       // 1.005 -> 1.00, even cent
		{3857, 100, .01, Long, 1.02, 101.02}, // 1.015 -> 1.02
		{3800, 100.009, .01, Long, 1, 101}, {3800, 100.009, .01, Short, 1, 99.01},
		{3800, 100.009, .001, Long, 1, 101.009},
	}
	for _, tc := range cases {
		got, e := PriorOpenScaleLocation(frozenOpening(t, tc.prior), tc.entry, tc.side, tc.grid)
		if e != nil || got.CentDistance != tc.distance || got.Stop != tc.stop {
			t.Fatalf("%+v: got %+v %v", tc, got, e)
		}
	}
	f := frozenOpening(t, 1)
	if _, e := PriorOpenScaleLocation(f, 100, Long, .01); e == nil {
		t.Fatal("zero cent lock was profitable")
	}
	f = frozenOpening(t, 3800)
	f.Opening.Price++
	if _, e := PriorOpenScaleLocation(f, 100, Long, .01); e == nil {
		t.Fatal("accepted mutated context")
	}
}

func TestCalendarFallTransitionAndEndOffsetBounds(t *testing.T) {
	c := Calendar{ID: "synthetic-fall", Version: "v1", Qualification: "invented", Days: []BrokerDay{
		{Date: "2026-11-01", StartMillis: ms("2026-10-31T21:00:00Z"), EndMillis: ms("2026-11-01T22:00:00Z"), Status: ClosedDay},
		{Date: "2026-11-02", StartMillis: ms("2026-11-01T22:00:00Z"), EndMillis: ms("2026-11-02T22:00:00Z"), Status: TradingDay},
	}}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	before, e := c.DayAt(ms("2026-11-01T21:59:59.999Z"))
	if e != nil || before.Date != "2026-11-01" {
		t.Fatal("fall boundary", e)
	}
	after, e := c.DayAt(ms("2026-11-01T22:00:00Z"))
	if e != nil || after.Date != "2026-11-02" {
		t.Fatal("fall boundary", e)
	}
	// A final supplied day may not transition outside the admitted offset range.
	c.Days = []BrokerDay{{Date: "2026-11-01", StartMillis: ms("2026-10-31T10:00:00Z"), EndMillis: ms("2026-11-01T09:00:00Z"), Status: TradingDay}}
	if e := c.Validate(); e == nil {
		t.Fatal("accepted final offset +15")
	}
}
