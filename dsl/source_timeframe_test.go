package dsl

import (
	"strings"
	"testing"
)

func TestSourceTimeframeParserParity(t *testing.T) {
	parse := func(version string, section string, line string) ParseResult {
		t.Helper()
		result, err := Parse("dsl " + version + "\nstrategy \"Source Timeframe\"\n" + section + " {\n  " + line + "\n}\n")
		if err != nil {
			t.Fatalf("Parse(): %v", err)
		}
		return result
	}

	for _, section := range []string{"setup", "filters"} {
		result := parse("v7", section, "source timeframe 4h")
		if len(result.Errors) != 0 {
			t.Fatalf("%s errors = %v", section, result.Errors)
		}
		if got := result.Config["sourceTimeframe"]; got != "4h" {
			t.Fatalf("%s sourceTimeframe = %#v, want 4h", section, got)
		}
	}

	inline, err := Parse("dsl v7\nstrategy \"Inline source timeframe\"\nfilters { side long only source timeframe 1h }")
	if err != nil {
		t.Fatalf("Parse inline filters: %v", err)
	}
	if len(inline.Errors) != 0 || inline.Config["sourceTimeframe"] != "1h" || inline.Config["allowLong"] != 1 || inline.Config["allowShort"] != 0 {
		t.Fatalf("inline filters result = %#v errors = %v", inline.Config, inline.Errors)
	}

	absent := parse("v7", "filters", "side long only")
	if _, ok := absent.Config["sourceTimeframe"]; ok {
		t.Fatalf("sourceTimeframe should be omitted when unspecified: %#v", absent.Config["sourceTimeframe"])
	}

	for _, section := range []string{"market conditions", "entry", "strategy"} {
		result := parse("v7", section, "source timeframe 4h")
		if !containsError(result.Errors, "only allowed in setup { ... } or filters { ... }") {
			t.Fatalf("%s errors = %v", section, result.Errors)
		}
		if !hasLineColumnDiagnostic(result.Diagnostics, 4, 3) {
			t.Fatalf("%s diagnostics = %#v", section, result.Diagnostics)
		}
	}

	topLevel, err := Parse("dsl v7\nstrategy \"Top-level source timeframe\"\nsource timeframe 4h")
	if err != nil {
		t.Fatalf("Parse top-level source timeframe: %v", err)
	}
	if !containsError(topLevel.Errors, "only allowed in setup { ... } or filters { ... }") || !hasLineColumnDiagnostic(topLevel.Diagnostics, 3, 1) {
		t.Fatalf("top-level result = %#v errors = %v", topLevel.Diagnostics, topLevel.Errors)
	}

	if result := parse("v6", "setup", "source timeframe 4h"); !containsError(result.Errors, "available only in dsl v7") {
		t.Fatalf("v6 errors = %v", result.Errors)
	}
	for _, line := range []string{"source timeframe auto", "source timeframe current", "source timeframe daily", "source timeframe weekly", "source timeframe 4h extra"} {
		if result := parse("v7", "setup", line); !containsError(result.Errors, "must be exactly one of") {
			t.Fatalf("%q errors = %v", line, result.Errors)
		}
	}

	result, err := Parse(`dsl v7
strategy "Supply-demand source"
setup {
  type: supply demand
  source timeframe 4h
  source at least 1.2 ATR within 3 candles
}`)
	if err != nil {
		t.Fatalf("Parse supply-demand: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("supply-demand errors = %v", result.Errors)
	}
	if got := result.Config["sourceTimeframe"]; got != "4h" {
		t.Fatalf("sourceTimeframe = %#v, want 4h", got)
	}
	supplyDemand := result.Config["supplyDemand"].(map[string]any)
	if supplyDemand["minSourceAtr"] != 1.2 || supplyDemand["sourceCandles"] != float64(3) {
		t.Fatalf("supplyDemand source config = %#v", supplyDemand)
	}
}

func TestSourceEntryTimeframeParserParity(t *testing.T) {
	for _, route := range []struct{ source, entry string }{{"4h", "30m"}, {"1h", "5m"}, {"15m", "1m"}} {
		result, err := Parse("dsl v7\nstrategy \"Source entry canonical\"\nslices XAUUSD " + route.entry + "\nsetup { type: flag continuation source timeframe " + route.source + " }\nentryTf " + route.entry)
		if err != nil || len(result.Errors) != 0 || result.Config["entryTf"] != route.entry {
			t.Fatalf("route=%s->%s result=%#v errors=%v err=%v", route.source, route.entry, result.Config, result.Errors, err)
		}
	}
	for _, route := range []struct{ source, entry, expected string }{{"1h", "30m", "4h"}, {"4h", "5m", "1h"}, {"1h", "1m", "15m"}} {
		result, err := Parse("dsl v7\nstrategy \"Source entry rejected\"\nsetup { type: flag continuation source timeframe " + route.source + " }\nentryTf " + route.entry)
		if err != nil || !containsError(result.Errors, "supported only with source timeframe "+route.expected) {
			t.Fatalf("route=%s->%s result=%#v errors=%v err=%v", route.source, route.entry, result.Config, result.Errors, err)
		}
	}
	unsupported, err := Parse("dsl v7\nstrategy \"C5 rejected\"\nentryTf 2m")
	if err != nil || !containsError(unsupported.Errors, "entryTf 2m is unsupported") {
		t.Fatalf("unsupported result=%#v errors=%v err=%v", unsupported.Config, unsupported.Errors, err)
	}
	otherSymbol, err := Parse("dsl v7\nstrategy \"Source entry FX\"\nslices EURUSD 30m\nsetup { type: flag continuation source timeframe 4h }\nentryTf 30m")
	if err != nil || len(otherSymbol.Errors) != 0 {
		t.Fatalf("other symbol result=%#v errors=%v err=%v", otherSymbol.Config, otherSymbol.Errors, err)
	}
}

func containsError(errors []string, want string) bool {
	for _, err := range errors {
		if strings.Contains(err, want) {
			return true
		}
	}
	return false
}

func hasLineColumnDiagnostic(diagnostics []Diagnostic, line int, column int) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == DiagnosticError && diagnostic.Line != nil && diagnostic.Column != nil && *diagnostic.Line == line && *diagnostic.Column == column {
			return true
		}
	}
	return false
}
