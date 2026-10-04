package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestJSONBridgeRejectsCalendarOnOtherFamilyBeforeExecution(t *testing.T) {
	raw, e := os.ReadFile("../../conformance/run/family-timed-return.fixture.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixture map[string]any
	if e = json.Unmarshal(raw, &fixture); e != nil {
		t.Fatal(e)
	}
	source := `dsl v7
strategy "Synthetic generic admission"
market conditions {
 slices(SYNTHUSD 1m)
}
setup {
 type: sma golden cross
}
`
	for _, calendar := range []any{fixture["timedCalendar"], map[string]any{}} {
		fixture["timedCalendar"] = calendar
		for _, symbol := range []string{"SYNTHUSD", "OTHERUSD"} {
			fixture["symbol"] = symbol
			b, _ := json.Marshal(fixture)
			out, e := runFixture(string(b), source)
			if e == nil || !strings.Contains(e.Error(), "timed calendar requires the timed-return family") || len(out) != 0 {
				t.Fatalf("%v %s", e, out)
			}
		}
	}
}
