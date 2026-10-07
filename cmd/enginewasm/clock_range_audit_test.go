package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	native "github.com/spik3r/heisentick-strat/engine"
)

func TestClockRangeFixtureAudit(t *testing.T) {
	cases := []struct{ name, outcome, reason string }{
		{"ordinary-long", "entered", ""},
		{"ordinary-short", "entered", ""},
		{"coverage-35-missing-final-slot", "rejected", "missing-final-slot"},
		{"expiry-bar-excluded", "expired", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := "../../conformance/run/family-clock-range-breakout-" + c.name
			raw, err := os.ReadFile(base + ".fixture.json")
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(base + ".strat")
			if err != nil {
				t.Fatal(err)
			}
			ordinary, err := runFixture(string(raw), string(source))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(ordinary), "rangeStartT") {
				t.Fatal("audit leaked into golden envelope")
			}
			audited, err := runClockRangeFixture(string(raw), string(source))
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Schema string
				Run    json.RawMessage
				Days   []native.ClockRangeDay
			}
			if err := json.Unmarshal(audited, &result); err != nil {
				t.Fatal(err)
			}
			if result.Schema != "clock-range-fixture-audit-v1" || string(result.Run) != string(ordinary) {
				t.Fatal("ordinary run changed")
			}
			if len(result.Days) != 1 || result.Days[0].Outcome != c.outcome || result.Days[0].Reason != c.reason {
				t.Fatalf("unexpected days: %+v", result.Days)
			}
		})
	}
	if _, err := runClockRangeFixture(`{}`, "dsl v7\nsetup { type: failed breakout }\n"); err == nil {
		t.Fatal("other family accepted")
	}
}
