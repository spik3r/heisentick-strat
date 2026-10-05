package dsl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func parseOpeningRangeTest(t *testing.T, version, directives string) ParseResult {
	t.Helper()
	result, err := Parse("dsl " + version + "\nsetup {\n  type: opening range breakout\n" + directives + "\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOpeningRangeDurationUnits(t *testing.T) {
	cases := []struct {
		name, directive        string
		candles, minutes, slot float64
	}{
		{"documented_minutes", "opening range first 15 minutes in (london)", 0, 15, 0},
		{"singular_minute", "opening range first 1 minute", 0, 1, 0},
		{"candles", "opening range first 3 candles", 3, 0, 0},
		{"singular_candle", "opening range first 1 candle", 1, 0, 0},
		{"legacy_bars", "opening range first 3 bars", 3, 0, 0},
		{"legacy_bar", "opening range first 1 bar", 1, 0, 0},
		{"case_insensitive", "OpEnInG RANGE FIRST 15 MINUTES", 0, 15, 0},
		{"whole_decimal", "opening range first 15.0 minutes", 0, 15, 0},
		{"whole_exponent", "opening range first 15e0 minutes", 0, 15, 0},
		{"utc_slots", "opening range every 3 hours UTC", 0, 15, 180},
		{"utc_one_hour", "opening range every 1 hours UTC", 0, 15, 60},
		{"utc_full_day", "opening range every 24 hours UTC", 0, 15, 1440},
		{"utc_sessions", "opening range every 3 hours UTC in (london)", 0, 15, 180},
		{"minutes_replace_candles", "opening range first 3 candles\nopening range first 15 minutes", 0, 15, 0},
		{"candles_replace_minutes", "opening range first 15 minutes\nopening range first 3 candles", 3, 0, 0},
		{"slots_replace_minutes", "opening range first 30 minutes\nopening range every 3 hours UTC", 0, 15, 180},
		// A duration directive does not change the separately selected UTC clock.
		{"minutes_keep_slot_clock", "opening range every 3 hours UTC\nopening range first 30 minutes", 0, 30, 180},
		{"candles_keep_slot_clock", "opening range every 3 hours UTC\nopening range first 3 candles", 3, 0, 180},
	}
	for _, version := range []string{"v6", "v7"} {
		for _, tc := range cases {
			t.Run(version+"/"+tc.name, func(t *testing.T) {
				result := parseOpeningRangeTest(t, version, tc.directive)
				if len(result.Errors) != 0 || len(result.Warnings) != 0 {
					t.Fatalf("unexpected diagnostics: %v / %v", result.Errors, result.Warnings)
				}
				orb := result.Config["openingRangeBreakout"].(map[string]any)
				if orb["firstCandles"] != tc.candles || orb["firstMinutes"] != tc.minutes {
					t.Fatalf("range = %#v, want candles=%v minutes=%v", orb, tc.candles, tc.minutes)
				}
				if tc.slot == 0 {
					if _, exists := orb["utcSlotMinutes"]; exists {
						t.Fatalf("unexpected slot clock: %#v", orb)
					}
				} else if orb["utcSlotMinutes"] != tc.slot {
					t.Fatalf("slot minutes = %v, want %v", orb["utcSlotMinutes"], tc.slot)
				}
				// The native result and JSON/WASM-facing result retain numeric keys.
				encoded, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var decoded struct {
					Config map[string]any `json:"cfg"`
				}
				if err := json.Unmarshal(encoded, &decoded); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(orb, decoded.Config["openingRangeBreakout"]) {
					t.Fatalf("ORB JSON round trip changed fields: %s", encoded)
				}
			})
		}
	}
}

func TestOpeningRangeDurationSessions(t *testing.T) {
	for _, unit := range []string{"minutes", "candles"} {
		result := parseOpeningRangeTest(t, "v7", "opening range first 15 "+unit+" in (london)")
		if len(result.Errors) != 0 {
			t.Fatal(result.Errors)
		}
		orb := result.Config["openingRangeBreakout"].(map[string]any)
		if !reflect.DeepEqual(orb["openingSessions"], []any{"london"}) {
			t.Fatalf("opening sessions = %#v", orb["openingSessions"])
		}
		want := map[string]any{"asia": 0, "mid": 0, "london": 1, "ny": 0}
		if !reflect.DeepEqual(result.Config["sessions"], want) {
			t.Fatalf("sessions = %#v, want %#v", result.Config["sessions"], want)
		}
	}
}

func TestOpeningRangeRejectsMalformedDuration(t *testing.T) {
	cases := []string{
		"opening range", "opening range first", "opening range first minutes",
		"opening range first 15", "opening range first minutes 15", "opening range 15 candles",
		"opening range first 15 seconds", "opening range first 15 hours", "opening range first 15 min",
		"opening range first 15 minutes candles", "opening range first 15 candles minutes",
		"opening range first 15 minutes every 3 hours UTC", "opening range first 15 minutes in ()",
		"opening range first 15 30 minutes", "opening range first 15R minutes", "opening range first 15% minutes",
		"opening range every 3 minutes UTC", "opening range every 3 hours", "opening range every hours UTC 3",
		"opening range every 3 hours UTC first 15 minutes", "opening range every 3R hours UTC",
		"opening range every 3 hours UTC in ()",
	}
	for _, value := range []string{"0", "-1", "0.5", "1.5", "NaN", "Inf", "-Inf", "1e999", "9223372036854775808", "not-a-number"} {
		for _, unit := range []string{"minutes", "candles"} {
			cases = append(cases, "opening range first "+value+" "+unit)
		}
	}
	for _, value := range []string{"0", "-1", "1.5", "25", "NaN", "Inf", "1e999"} {
		cases = append(cases, "opening range every "+value+" hours UTC")
	}
	for _, version := range []string{"v6", "v7"} {
		for _, directive := range cases {
			t.Run(version+"/"+directive, func(t *testing.T) {
				// A bad replacement must not partially mutate a valid duration or sessions.
				prefix := "opening range first 3 candles in (ny)"
				before := parseOpeningRangeTest(t, version, prefix)
				result := parseOpeningRangeTest(t, version, prefix+"\n  "+directive)
				if len(result.Errors) != 1 || len(result.Diagnostics) != 1 {
					t.Fatalf("want one error, got %v / %#v", result.Errors, result.Diagnostics)
				}
				diagnostic := result.Diagnostics[0]
				if diagnostic.Severity != DiagnosticError || diagnostic.Line == nil || *diagnostic.Line != 5 || diagnostic.Column == nil || *diagnostic.Column != 3 || !strings.Contains(diagnostic.Message, "opening range") {
					t.Fatalf("unexpected diagnostic: %#v", diagnostic)
				}
				if !reflect.DeepEqual(result.Config, before.Config) {
					t.Fatal("invalid duration changed configuration")
				}
			})
		}
	}
}

func TestOpeningRangeDefaultFieldsUnchanged(t *testing.T) {
	result := parseOpeningRangeTest(t, "v7", "")
	want := map[string]any{"firstMinutes": 0}
	if !reflect.DeepEqual(result.Config["openingRangeBreakout"], want) {
		t.Fatalf("default ORB = %#v, want %#v", result.Config["openingRangeBreakout"], want)
	}
}
