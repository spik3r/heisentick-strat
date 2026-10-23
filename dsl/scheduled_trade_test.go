package dsl

import (
	"strings"
	"testing"
)

const validScheduledTradeSource = `dsl v7
strategy "Tokyo settlement short"
market conditions {
 slices(USDJPY 5m)
}
setup {
 type: scheduled trade
 scheduled timezone UTC
 scheduled calendar japan-settlement
 scheduled entry 00:55
 scheduled exit 01:30
 scheduled side short
 scheduled stop 20 pips
 scheduled quantity 1 unit
}`

func TestScheduledTradeParsesClosedContract(t *testing.T) {
	got, err := Parse(validScheduledTradeSource)
	if err != nil || len(got.Errors) != 0 {
		t.Fatalf("Parse errors = %v; err = %v", got.Errors, err)
	}
	spec, err := DecodeScheduledTrade(got.Config)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Side != "short" || spec.Entry != "00:55" || spec.Exit != "01:30" || spec.StopPips != 20 || spec.Quantity != 1 {
		t.Fatalf("decoded spec = %+v", spec)
	}
}

func TestScheduledTradeRejectsDuplicatesAndUnsupportedAuthorship(t *testing.T) {
	for _, tc := range []struct{ directive, want string }{
		{"scheduled side long", "duplicate scheduled directive side"},
		{"scheduled stop 25 pips", "duplicate scheduled directive stop"},
		{"stop 20 pips", "scheduled trade does not support authored directive setup|stop"},
		{"scheduled unknown value", "unknown scheduled directive unknown"},
		{"scheduled stop 0 pips", "stop must be positive"},
		{"scheduled timezone America/Toronto", "duplicate scheduled directive timezone"},
		{"scheduled quantity 2 units", "quantity must be exactly 1 unit"},
	} {
		source := withScheduledDirective(tc.directive)
		got, err := Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if !diagnosticsContain(got.Errors, tc.want) {
			t.Fatalf("diagnostics %v do not contain %q for source: %s", got.Errors, tc.want, source)
		}
	}
	badZone := strings.Replace(validScheduledTradeSource, "scheduled timezone UTC", "scheduled timezone America/Toronto", 1)
	parsed, err := Parse(badZone)
	if err != nil || !diagnosticsContain(parsed.Errors, "UTC and japan-settlement calendar") {
		t.Fatalf("unsupported timezone diagnostics=%v err=%v", parsed.Errors, err)
	}
}

func withScheduledDirective(directive string) string {
	end := strings.LastIndex(validScheduledTradeSource, "\n}")
	if end < 0 {
		panic("scheduled source missing setup close")
	}
	return validScheduledTradeSource[:end] + "\n  " + directive + validScheduledTradeSource[end:]
}

func diagnosticsContain(diagnostics []string, want string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, want) {
			return true
		}
	}
	return false
}

func TestScheduledTradeRequiresAllDirectives(t *testing.T) {
	source := strings.Replace(validScheduledTradeSource, " scheduled quantity 1 unit", "", 1)
	got, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if !diagnosticsContain(got.Errors, "quantity 1 unit") {
		t.Fatalf("missing quantity diagnostics=%v", got.Errors)
	}
}

func TestScheduledTradeRouteIsNarrow(t *testing.T) {
	for _, route := range []string{"USDJPY 15m", "XAUUSD 5m", "EURUSD 1h"} {
		source := validScheduledTradeSource
		source = replaceScheduledRoute(source, route)
		got, _ := Parse(source)
		if len(got.Errors) == 0 {
			t.Fatalf("accepted route %s", route)
		}
	}
	misaligned := strings.Replace(validScheduledTradeSource, "scheduled entry 00:55", "scheduled entry 00:56", 1)
	got, _ := Parse(misaligned)
	if len(got.Errors) == 0 {
		t.Fatal("accepted entry clock outside 5m grid")
	}
}

func replaceScheduledRoute(source, route string) string {
	return strings.Replace(source, "USDJPY 5m", route, 1)
}
