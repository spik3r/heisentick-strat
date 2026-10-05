package dsl

import (
	"os"
	"strings"
	"testing"
)

func TestFailedBreakoutNumericTargetLowersToFallback(t *testing.T) {
	raw, err := os.ReadFile("../engine/testdata/failed-breakout-target.strat")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, target string
		want         float64
	}{
		{"numeric", "target 2R", 2},
		{"decimal", "target 1.5 R", 1.5},
		{"explicit fallback", "fallback 3R", 3},
		{"later fallback wins", "target 2R\n fallback 3R", 3},
		{"later numeric wins", "fallback 3R\n target 2R", 2},
		{"default", "take profit at opposite range edge", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.Replace(string(raw), "target 2R", tc.target, 1)
			parsed, err := Parse(source)
			if err != nil || len(parsed.Errors) != 0 {
				t.Fatalf("parse: %v %v", err, parsed.Errors)
			}
			target := parsed.Config["target"].(map[string]any)
			if got := target["fallbackR"]; !numEquals(got, tc.want) {
				t.Fatalf("fallbackR = %v, want %v", got, tc.want)
			}
			if _, ok := target["r"]; ok {
				t.Fatalf("numeric target must not be stored in unused target.r: %v", target)
			}
			if target["edge"] != "range" || target["minR"] != .6 {
				t.Fatalf("edge/minimum changed: %v", target)
			}
		})
	}
}
