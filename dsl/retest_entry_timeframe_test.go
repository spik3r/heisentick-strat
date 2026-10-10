package dsl

import (
	"reflect"
	"strings"
	"testing"
)

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

func TestRetestOnEntryTimeframeFinalFamilyValidation(t *testing.T) {
	for _, compact := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			setup := "type: supply demand\n"
			if enabled {
				setup += "retest on entry timeframe\n"
			}
			setup += "type: break retest"
			block := "setup {\n" + setup + "\nsource timeframe 4h\n}"
			if compact {
				block = "setup { " + strings.ReplaceAll(setup, "\n", " ") + " source timeframe 4h }"
			}
			text := "dsl v7\nstrategy \"Retest final family\" { description \"x\" }\nmarket conditions { slices(XAUUSD 15m) }\n" + block + "\nentryTf 15m\n"
			result, err := Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			if enabled && !containsError(result.Errors, "retest on entry timeframe requires supply demand") {
				t.Fatalf("compact=%v: active contradictory family accepted: %v", compact, result.Errors)
			}
			if !enabled && len(result.Errors) != 0 {
				t.Fatalf("compact=%v: disabled unrelated family changed: %v", compact, result.Errors)
			}
		}
	}
}

func TestRetestOnEntryTimeframeCompactKeepsPhrase(t *testing.T) {
	for _, entry := range []string{"15m", "30m", "1h"} {
		source := "dsl v7\nstrategy \"Compact retest\" { description \"x\" }\nmarket conditions { slices(XAUUSD " + entry + ") }\nsetup { type: supply demand source timeframe 4h retest on entry timeframe }\nentryTf " + entry
		parsed, err := Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		sd, _ := parsed.Config["supplyDemand"].(map[string]any)
		if len(parsed.Errors) != 0 || sd["retestOnEntryTimeframe"] != 1 {
			t.Fatalf("compact %s lost mode: flag=%v errors=%v", entry, sd["retestOnEntryTimeframe"], parsed.Errors)
		}
		multiline := strings.Replace(source, "setup { type: supply demand source timeframe 4h retest on entry timeframe }", "setup {\n type: supply demand\n source timeframe 4h\n retest on entry timeframe\n}", 1)
		ordinary, err := Parse(multiline)
		if err != nil || len(ordinary.Errors) != 0 || !reflect.DeepEqual(parsed.Config, ordinary.Config) {
			t.Fatalf("compact/multiline %s differ: err=%v errors=%v", entry, err, ordinary.Errors)
		}
	}
}

func TestRetestCompactPhraseDoesNotAbsorbEntryDirective(t *testing.T) {
	for _, phrase := range []string{"retest on entry timeframe", "retest  on\tENTRY timeframe"} {
		body := phrase + " entry distance 0.2 ATR"
		got := splitInlineBody(body, directiveStartsForSection("setup"), "setup")
		if len(got) != 2 || strings.TrimSpace(got[0]) != phrase || strings.TrimSpace(got[1]) != "entry distance 0.2 ATR" {
			t.Fatalf("phrase/entry boundaries changed: %q", got)
		}
	}
}
