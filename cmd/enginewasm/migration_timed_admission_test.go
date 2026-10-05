package main

import (
	"strings"
	"testing"
)

func TestCompositionBridgeDoesNotDiscardTimedCalendar(t *testing.T) {
	raw := `{"schema":"dsl-conformance-run-fixture-v1","timedCalendar":{}}`
	out, err := runCompositionFixture(raw, `{}`)
	if err == nil || !strings.Contains(err.Error(), "timed calendar") || len(out) != 0 {
		t.Fatalf("calendar was discarded before composition admission: %v %s", err, out)
	}
}
