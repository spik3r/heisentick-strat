package marketdata

import (
	"testing"
	"time"
)

func utcMS(t *testing.T, s string) int64 {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v.UnixMilli()
}

func TestResearchLocalMinuteClocks(t *testing.T) {
	for _, tc := range []struct {
		date, zone string
		minute     int
		want       string
	}{
		{"2026-01-15", "America/New_York", 15*60 + 30, "2026-01-15T20:30:00Z"},
		{"2026-07-15", "America/New_York", 15*60 + 30, "2026-07-15T19:30:00Z"},
		{"2026-03-06", "America/New_York", 16 * 60, "2026-03-06T21:00:00Z"},
		{"2026-03-09", "America/New_York", 16 * 60, "2026-03-09T20:00:00Z"},
		{"2026-10-30", "America/New_York", 16 * 60, "2026-10-30T20:00:00Z"},
		{"2026-11-02", "America/New_York", 16 * 60, "2026-11-02T21:00:00Z"},
		{"2026-01-15", "Europe/London", 15 * 60, "2026-01-15T15:00:00Z"},
		{"2026-07-15", "Europe/London", 15 * 60, "2026-07-15T14:00:00Z"},
		{"2026-03-09", "Europe/London", 15 * 60, "2026-03-09T15:00:00Z"},
		{"2026-03-30", "Europe/London", 15 * 60, "2026-03-30T14:00:00Z"},
		{"2026-10-26", "Europe/London", 15 * 60, "2026-10-26T15:00:00Z"},
		{"2026-01-15", "Asia/Tokyo", 9*60 + 56, "2026-01-15T00:56:00Z"},
		{"2026-07-15", "Asia/Tokyo", 10*60 + 26, "2026-07-15T01:26:00Z"},
		{"2026-01-01", "Asia/Tokyo", 0, "2025-12-31T15:00:00Z"},
		{"2026-07-15", "UTC", 21 * 60, "2026-07-15T21:00:00Z"},
	} {
		t.Run(tc.date+"/"+tc.zone+"/"+tc.want, func(t *testing.T) {
			got, err := ResearchLocalMinute(tc.date, tc.zone, tc.minute)
			if err != nil || got != utcMS(t, tc.want) {
				t.Fatalf("got %v %v, want %s", got, err, tc.want)
			}
		})
	}
}

func TestResearchLocalMinuteRefusesNormalization(t *testing.T) {
	for _, tc := range []struct {
		date, zone string
		minute     int
	}{
		{"2026-02-29", "UTC", 0}, {"2026-01-01", "UTC", -1}, {"2026-01-01", "UTC", 1440},
		{"1999-12-31", "UTC", 0}, {"2100-01-01", "UTC", 0}, {"2026-1-1", "UTC", 0},
		{"2026-01-01", "Local", 0}, {"2026-01-01", "Australia/Lord_Howe", 0},
		{"2026-03-08", "America/New_York", 2*60 + 30},
		{"2026-11-01", "America/New_York", 1*60 + 30},
		{"2026-03-29", "Europe/London", 1*60 + 30},
		{"2026-10-25", "Europe/London", 1*60 + 30},
	} {
		if _, err := ResearchLocalMinute(tc.date, tc.zone, tc.minute); err == nil {
			t.Errorf("normalized invalid/ambiguous clock: %+v", tc)
		}
	}
}

func TestResearchConditionalOverlapIsNotVenueEvidence(t *testing.T) {
	// The original corrected plan's overlap identity holds ONLY under its
	// hypothetical fixed 21:00 UTC anchor, not actual source/venue calendars.
	for _, tc := range []struct {
		date  string
		equal bool
	}{{"2026-01-15", true}, {"2026-07-15", false}} {
		fixed, _ := ResearchLocalMinute(tc.date, "UTC", 20*60+30)
		cash, _ := ResearchLocalMinute(tc.date, "America/New_York", 15*60+30)
		if (fixed == cash) != tc.equal {
			t.Fatalf("conditional overlap wrong on %s", tc.date)
		}
	}
}

func TestResearchSevenCellSyntheticWindows(t *testing.T) {
	// These are invented calendar intervals, prices and latencies. In
	// particular the fixed A1 clock is not a supplied broker-calendar claim.
	cells := []struct {
		id, zone   string
		start, end int
	}{
		{"V1-XAU", "UTC", 20*60 + 30, 21 * 60}, {"V2-XAU", "UTC", 20*60 + 30, 21 * 60},
		{"V3-XAU", "America/New_York", 15*60 + 30, 16 * 60}, {"V4-XAU", "America/New_York", 15*60 + 30, 16 * 60},
		{"V5-XAU", "Europe/London", 15 * 60, 15*60 + 4}, {"V1-XAG", "UTC", 20*60 + 30, 21 * 60},
		{"Tokyo-USDJPY", "Asia/Tokyo", 9*60 + 56, 10*60 + 26},
	}
	for _, cell := range cells {
		t.Run(cell.id, func(t *testing.T) {
			start, err := ResearchLocalMinute("2026-07-15", cell.zone, cell.start)
			if err != nil {
				t.Fatal(err)
			}
			end, err := ResearchLocalMinute("2026-07-15", cell.zone, cell.end)
			if err != nil {
				t.Fatal(err)
			}
			w := ResearchWindow{Source: ResearchSession{start - 3600000, end + 60000}, Execution: ResearchSession{start - 3600000, end + 30000},
				DecisionMS: start, EntrySubmitMS: start + 1000, EntryDeadlineMS: start + 6000,
				ExitSubmitMS: end - 60000, ExitDeadlineMS: end - 55000, MaxSignalAgeMS: 1000, MaxQuoteAgeMS: 1000}
			rows := []ResearchQuote{{start - 1, start - 1, 100, 101}, {start + 1000, start + 1001, 102, 103}, {end - 60000, end - 59999, 104, 105}}
			result, err := InspectResearchWindow(mustResearchQuotes(t, rows), w)
			if err != nil || result.Status != "available" {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}
