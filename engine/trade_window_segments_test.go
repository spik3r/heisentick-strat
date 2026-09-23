package engine

import (
	"testing"
	"time"
)

func fixedLocalHour(hour, minute int) float64 {
	return float64(time.Date(2026, time.January, 5, hour-10, minute, 0, 0, time.UTC).UnixMilli())
}

func TestSegmentedTradeWindowBoundariesIncludeMidFourthHourOnlyForAll(t *testing.T) {
	allowed := map[string]bool{"mid": true}
	cases := []struct {
		name     string
		segments []string
		hour     int
		minute   int
		want     bool
	}{
		{"open start", []string{"mid.open"}, 12, 0, true},
		{"open end", []string{"mid.open"}, 13, 0, false},
		{"middle start", []string{"mid.middle"}, 13, 0, true},
		{"middle end", []string{"mid.middle"}, 14, 0, false},
		{"close start", []string{"mid.close"}, 14, 0, true},
		{"close final minute", []string{"mid.close"}, 14, 59, true},
		{"fourth hour excluded from close", []string{"mid.close"}, 15, 0, false},
		{"fourth hour admitted by all", []string{"mid.all"}, 15, 0, true},
		{"window end excluded", []string{"mid.all"}, 16, 0, false},
		{"disabled session rejects segment", []string{"london.open"}, 16, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := flagParams{TradeWindowSegments: tc.segments}
			if got := inSegmentedTradeWindow(fixedLocalHour(tc.hour, tc.minute), params, allowed); got != tc.want {
				t.Fatalf("inSegmentedTradeWindow() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSegmentAndMinuteRangeIntersect(t *testing.T) {
	params := flagParams{
		TradeWindowSegments:       []string{"mid.open", "mid.middle"},
		TradeWindowMinuteFrom:     30,
		TradeWindowMinuteTo:       90,
		TradeWindowMinuteRangeSet: true,
	}
	allowed := map[string]bool{"mid": true}
	for _, tc := range []struct {
		hour, minute int
		want         bool
	}{{12, 29, false}, {12, 30, true}, {13, 29, true}, {13, 30, false}} {
		if got := inSegmentedTradeWindow(fixedLocalHour(tc.hour, tc.minute), params, allowed); got != tc.want {
			t.Fatalf("%02d:%02d = %v, want %v", tc.hour, tc.minute, got, tc.want)
		}
	}
}

func TestLondonCloseSemanticFixtureRejectsMiddleAndAdmitsClose(t *testing.T) {
	trade := runSemanticCase(t, "pm-trade-window-london-close")
	if trade.EntryIndex != 47 || trade.EntryT != 1767772800000 || trade.Entry != 2025 {
		t.Fatalf("entry = %v @ %d (%v), want 2025 @ 47 (1767772800000)", trade.Entry, trade.EntryIndex, trade.EntryT)
	}
	if trade.InitialSL != 1997.5 || trade.InitialTP != 2052.5 || trade.ExitIndex != 50 || trade.Exit != 2052.5 || trade.Reason != "tp" {
		t.Fatalf("trade = %+v, want derived stop, target and target exit", trade)
	}
}
