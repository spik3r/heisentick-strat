package dsl

import "testing"

func parseRetestEntry(t *testing.T, entryTf string, source string, retest bool) ParseResult {
	t.Helper()
	text := "dsl v7\nstrategy \"Retest entry\" { description \"x\" }\nmarket conditions {\n  slices(XAUUSD " + entryTf + ")\n}\nsetup {\n  type: supply demand\n"
	if source != "" {
		text += "  source timeframe " + source + "\n"
	}
	if retest {
		text += "  retest on entry timeframe\n"
	}
	text += "}\nentryTf " + entryTf + "\n"
	result, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return result
}

func TestRetestOnEntryTimeframeParses(t *testing.T) {
	for _, entry := range []string{"15m", "30m", "1h"} {
		result := parseRetestEntry(t, entry, "4h", true)
		if len(result.Errors) != 0 {
			t.Fatalf("4h -> %s errors = %v", entry, result.Errors)
		}
		sd, _ := result.Config["supplyDemand"].(map[string]any)
		if sd["retestOnEntryTimeframe"] != 1 {
			t.Fatalf("supplyDemand = %#v", sd)
		}
	}
	plain := parseRetestEntry(t, "15m", "4h", false)
	if sd, _ := plain.Config["supplyDemand"].(map[string]any); sd["retestOnEntryTimeframe"] != nil {
		t.Fatalf("flag set without the phrase: %#v", sd)
	}
}

func TestRetestOnEntryTimeframeRouteErrors(t *testing.T) {
	if got := parseRetestEntry(t, "1h", "4h", false); !containsError(got.Errors, "entryTf 1h is supported only with source timeframe") {
		t.Fatalf("1h without the phrase should stay unsupported: %v", got.Errors)
	}
	for _, tc := range []struct{ entry, source string }{{"5m", "1h"}, {"1m", "15m"}, {"15m", "1h"}, {"15m", ""}} {
		if got := parseRetestEntry(t, tc.entry, tc.source, true); !containsError(got.Errors, "retest on entry timeframe requires source timeframe 4h and entryTf 15m, 30m or 1h") {
			t.Fatalf("%s from %q: %v", tc.entry, tc.source, got.Errors)
		}
	}
}
