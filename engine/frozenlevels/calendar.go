package frozenlevels

import (
	"fmt"
	"time"
)

type DayStatus string

const (
	TradingDay DayStatus = "trading"
	ClosedDay  DayStatus = "closed"
)

type BrokerDay struct {
	Date        string    `json:"date"`
	StartMillis int64     `json:"startMillis"`
	EndMillis   int64     `json:"endMillis"`
	Status      DayStatus `json:"status"`
}

// Calendar is an explicitly qualified finite day ledger, not a guessed DST
// recurrence or a UTC-day alias. Every date including closures must be present.
type Calendar struct {
	ID            string      `json:"id"`
	Version       string      `json:"version"`
	Qualification string      `json:"qualification"`
	Days          []BrokerDay `json:"days"`
}

func (c Calendar) Validate() error {
	if c.ID == "" || c.Version == "" || c.Qualification == "" || len(c.Days) == 0 {
		return fmt.Errorf("unqualified-calendar")
	}
	var previousDate time.Time
	for i, d := range c.Days {
		date, err := time.Parse("2006-01-02", d.Date)
		if err != nil || date.Format("2006-01-02") != d.Date || !validMillis(d.StartMillis) || !validMillis(d.EndMillis) {
			return fmt.Errorf("invalid-calendar-day")
		}
		duration := d.EndMillis - d.StartMillis
		if duration != 23*60*60*1000 && duration != 24*60*60*1000 && duration != 25*60*60*1000 {
			return fmt.Errorf("invalid-calendar-day-duration")
		}
		offset := date.UnixMilli() - d.StartMillis
		endOffset := date.AddDate(0, 0, 1).UnixMilli() - d.EndMillis
		if offset < -12*60*60*1000 || offset > 14*60*60*1000 || offset%60000 != 0 || endOffset < -12*60*60*1000 || endOffset > 14*60*60*1000 || endOffset%60000 != 0 {
			return fmt.Errorf("invalid-calendar-offset")
		}
		if d.Status != TradingDay && d.Status != ClosedDay {
			return fmt.Errorf("unknown-calendar-day-status")
		}
		if i > 0 && (c.Days[i-1].EndMillis != d.StartMillis || !date.Equal(previousDate.AddDate(0, 0, 1))) {
			return fmt.Errorf("calendar-gap-or-overlap")
		}
		previousDate = date
	}
	return nil
}
func (c Calendar) dayIndex(at int64) (int, error) {
	if err := c.Validate(); err != nil {
		return 0, err
	}
	if !validMillis(at) {
		return 0, fmt.Errorf("invalid-decision-time")
	}
	for i, d := range c.Days {
		if d.StartMillis <= at && at < d.EndMillis {
			return i, nil
		}
	}
	return 0, fmt.Errorf("calendar-outside-qualified-coverage")
}
func (c Calendar) DayAt(at int64) (BrokerDay, error) {
	i, e := c.dayIndex(at)
	if e != nil {
		return BrokerDay{}, e
	}
	return c.Days[i], nil
}

type Opening struct {
	Date            string   `json:"date"`
	Event           EventKey `json:"event"`
	KnownAtMillis   int64    `json:"knownAtMillis"`
	Price           float64  `json:"price"`
	Source          Source   `json:"source"`
	CalendarID      string   `json:"calendarId"`
	CalendarVersion string   `json:"calendarVersion"`
	// Qualification names the external proof of the FIRST qualified opening.
	// A late first downloaded quote alone is not sufficient evidence.
	Qualification string `json:"qualification"`
}
type FrozenOpening struct {
	ID             string  `json:"id"`
	Version        string  `json:"version"`
	FrozenAtMillis int64   `json:"frozenAtMillis"`
	CalendarDigest string  `json:"calendarDigest"`
	Opening        Opening `json:"opening"`
}

// FreezePriorOpening skips verified closed days only. It never skips a missing
// trading-day opening, substitutes today's open, or consults a later quote.
func FreezePriorOpening(c Calendar, at int64, openings []Opening) (FrozenOpening, error) {
	idx, err := c.dayIndex(at)
	if err != nil {
		return FrozenOpening{}, err
	}
	if c.Days[idx].Status != TradingDay {
		return FrozenOpening{}, fmt.Errorf("decision-on-closed-day")
	}
	idx--
	for idx >= 0 && c.Days[idx].Status == ClosedDay {
		idx--
	}
	if idx < 0 {
		return FrozenOpening{}, fmt.Errorf("missing-prior-trading-day")
	}
	day := c.Days[idx]
	var found *Opening
	for i := range openings {
		o := &openings[i]
		if o.Date != day.Date {
			continue
		}
		if found != nil {
			return FrozenOpening{}, fmt.Errorf("duplicate-prior-opening")
		}
		found = o
	}
	if found == nil {
		return FrozenOpening{}, fmt.Errorf("missing-prior-opening")
	}
	o := *found
	if err := o.Source.validate(); err != nil {
		return FrozenOpening{}, err
	}
	if o.Source.Side != Bid || !o.Event.valid() || o.Event.AtMillis < day.StartMillis || o.Event.AtMillis >= day.EndMillis || !validMillis(o.KnownAtMillis) || o.KnownAtMillis < o.Event.AtMillis || o.KnownAtMillis > at || !finitePositive(o.Price) || o.CalendarID != c.ID || o.CalendarVersion != c.Version || o.Qualification == "" {
		return FrozenOpening{}, fmt.Errorf("unqualified-prior-opening")
	}
	f := FrozenOpening{Version: ContractVersion, FrozenAtMillis: at, CalendarDigest: identity(c), Opening: o}
	f.ID = identity(f)
	return f, nil
}
