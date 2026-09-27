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
