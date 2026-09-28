package dsl

import (
	"reflect"
	"testing"
)

func TestParseLocalWeekdayHourAndOpenLocationGates(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "Market context gates"
market conditions {
  local weekday in (Mon, Tue)
  weekday not in (Fri)
  local hour in (9, 10:00)
  hour not in (23)
  open location in (nearAH, near PDH, nearDO, other)
}
setup { type: failed breakout }
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	for key, want := range map[string]any{
		"localWeekdays":        []any{"Mon", "Tue"},
		"blockedLocalWeekdays": []any{"Fri"},
		"localHours":           []any{9, 10},
		"blockedLocalHours":    []any{23},
		"openLocations":        []any{"nearAH", "nearPDH", "nearDayOpen", "other"},
	} {
		if got := result.Config[key]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

func TestParseLocalMarketContextGatesRejectInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "missing list", line: "local weekday in ()"},
		{name: "unknown weekday", line: "local weekday in (Funday)"},
		{name: "hour out of range", line: "local hour in (24)"},
		{name: "unknown open location", line: "open location in (nearMars)"},
		{name: "unsupported negated open location", line: "open location not in (nearPDH)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Parse("dsl v7\nmarket conditions {\n  " + test.line + "\n}\nsetup { type: failed breakout }\n")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(result.Errors) == 0 {
				t.Fatal("invalid market gate was accepted")
			}
		})
	}
}

func TestCanonicalStrategyAndSetupBlocksRetainNameDescriptionAndDefaults(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "Named gate fixture" {
  description "A canonical parser fixture."
}
setup {
  type: failed breakout
}
management {
  move stop to breakeven after 0.5R plus 0.05 ATR
  wait 12 candles after trade
}
`)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("parse errors=%v err=%v", result.Errors, err)
	}
	if result.Config["name"] != "Named gate fixture" || result.Config["description"] != "A canonical parser fixture." {
		t.Fatalf("identity = name %q description %q", result.Config["name"], result.Config["description"])
	}
	if result.Config["setupType"] != string(FamilyFailedBreakout) {
		t.Fatalf("setupType = %v, want %s", result.Config["setupType"], FamilyFailedBreakout)
	}
	if got := result.Config["breakeven"].(map[string]any); got["atR"] != 0.5 || got["offsetAtr"] != 0.05 {
		t.Fatalf("failed-breakout breakeven = %#v", got)
	}
	if got := result.Config["cooldownCandles"]; got != 12 && got != float64(12) {
		t.Fatalf("failed-breakout cooldown = %v, want 12", result.Config["cooldownCandles"])
	}
}

func TestFailedBreakoutSetupKeepsBaseManagementDefaults(t *testing.T) {
	result, err := Parse(`dsl v7
setup { type: failed breakout }
`)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("parse errors=%v err=%v", result.Errors, err)
	}
	breakeven := result.Config["breakeven"].(map[string]any)
	if breakeven["atR"] != 0.75 || breakeven["offsetAtr"] != 0.02 {
		t.Fatalf("failed-breakout breakeven = %#v, want base defaults {atR: 0.75, offsetAtr: 0.02}", breakeven)
	}
	if got := result.Config["cooldownCandles"]; got != 3 && got != float64(3) {
		t.Fatalf("failed-breakout cooldown = %v, want base default 3", got)
	}
}

func TestStandaloneStrategyDirectiveAndInlineSetupAreNotTreatedAsSections(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "Standalone title"
setup { type: failed breakout }
`)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("parse errors=%v err=%v", result.Errors, err)
	}
	if result.Config["name"] != "Standalone title" {
		t.Fatalf("name = %q, want standalone title", result.Config["name"])
	}
	if result.Config["setupType"] != string(FamilyFailedBreakout) {
		t.Fatalf("setupType = %v, want %s", result.Config["setupType"], FamilyFailedBreakout)
	}
}
