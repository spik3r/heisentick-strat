package dsl

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestReviewFrozenExactInlineReproductions(t *testing.T) {
	b, err := os.ReadFile("testdata/frozen_level/same-line-reproductions.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases map[string]string
	if err = json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 15 {
		t.Fatal("original reproduction count changed")
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := Parse(source)
			if err != nil || len(r.Errors) == 0 || len(r.Config) != 0 {
				t.Fatalf("inline frozen selection escaped: %v family=%v errors=%v", err, r.Config["setupType"], r.Errors)
			}
		})
	}
}

func TestReviewFrozenInlineClauseCrossProduct(t *testing.T) {
	prefixes := []string{
		"", "symbols XAUUSD", "symbols FROZEN", "symbols(FROZEN, XAUUSD)",
		"description free", `description "type: frozen level breakout"`, "name x", `name 'frozen'`,
		"sessions london", "sessions(london)", "timeframes 1m", "timeframes(1m)",
		"slices XAUUSD 1m", "slices(XAUUSD 1m)", "side both", "stop 1 ATR",
	}
	selectors := []string{`frozen level breakout`, `"frozen level breakout"`, `'frozen level breakout'`, "`frozen level breakout`", `frozenLevelBreakout`}
	groups := []struct{ open, close string }{{"", ""}, {"(", ")"}, {"((", "))"}, {"(", ""}, {"{", "}"}}
	separators := []string{" ", "\n"}
	n := 0
	for _, prefix := range prefixes {
		for _, selector := range selectors {
			for _, group := range groups {
				for _, sep := range separators {
					for _, prior := range []string{"", "type: flag continuation "} {
						source := fmt.Sprintf("dsl v7\nstrategy \"x\"\nsetup { %s%s%s%stype: %s%s type: flag continuation }", prefix, sep, prior, group.open, selector, group.close)
						r, err := Parse(source)
						if err != nil || len(r.Errors) == 0 || len(r.Config) != 0 {
							t.Fatalf("prefix=%q selector=%q group=%q/%q separator=%q escaped: %v %v", prefix, selector, group.open, group.close, sep, err, r.Errors)
						}
						n++
					}
				}
			}
		}
	}
	if n != 1600 {
		t.Fatal(n)
	}
	t.Logf("reserved inline clause combinations: %d", n)
}

func TestReviewFrozenInlineContextCompatibility(t *testing.T) {
	// Compare with the unchanged legacy parser, including names/values which
	// resemble selectors inside genuine lists or metadata. A selector's role
	// depends on the section and the legacy clause boundaries, not its spelling
	// alone or a blanket parenthesis-depth rule.
	clauses := []string{"symbols FROZEN", "symbols(FROZEN, XAUUSD)", "symbols(TYPE, FROZEN)", "symbols(TYPE: FROZEN)", "symbols XAUUSD", "description free", `description "frozen level breakout"`, "name frozen", `name "type: frozen level breakout"`}
	for _, quote := range []string{`"`, "'", "`"} {
		clauses = append(clauses, "description "+quote+"symbols FROZEN { type: frozen level breakout } # words"+quote, "name "+quote+"type: frozenLevelBreakout"+quote, "symbols("+quote+"type: frozen level breakout"+quote+", FROZEN)")
	}
	for _, clause := range clauses {
		for _, context := range []string{"setup { %s type: flag continuation }", "%s\nsetup { type: flag continuation }"} {
			source := "dsl v7\nstrategy \"Legacy\"\n" + fmt.Sprintf(context, clause)
			old := newParser(source)
			old.parse()
			want := old.result()
			if len(want.Errors) > 0 {
				t.Fatalf("invalid positive control %q: %v", source, want.Errors)
			}
			got, err := Parse(source)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("legacy control changed %q: %v %v", source, err, got.Errors)
			}
		}
	}
	for _, clause := range []string{"description free type: frozen level breakout", "name x type: frozenLevelBreakout"} {
		source := "dsl v7\nstrategy \"Legacy\" { " + clause + " }\nsetup { type: flag continuation }"
		old := newParser(source)
		old.parse()
		want := old.result()
		if len(want.Errors) > 0 {
			t.Fatal(want.Errors)
		}
		got, err := Parse(source)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("strategy metadata acquired a setup clause: %v %v", err, got.Errors)
		}
	}
}

func TestReviewFrozenMetadataVersusSelectorPairs(t *testing.T) {
	for _, words := range []string{"frozen level breakout", "frozenLevelBreakout"} {
		for _, quote := range []string{`"`, "'", "`"} {
			metadata := "dsl v7\nstrategy \"Legacy\"\nsetup { description " + quote + words + quote + " type: flag continuation }"
			old := newParser(metadata)
			old.parse()
			want := old.result()
			got, err := Parse(metadata)
			if err != nil || len(got.Errors) != 0 || !reflect.DeepEqual(got, want) {
				t.Fatalf("metadata pair changed: %q %v %v", metadata, err, got.Errors)
			}
			selector := "dsl v7\nstrategy \"Legacy\"\nsetup { type: " + quote + words + quote + " type: flag continuation }"
			got, err = Parse(selector)
			if err != nil || len(got.Errors) == 0 || len(got.Config) != 0 {
				t.Fatalf("selector pair escaped: %q %v %v", selector, err, got.Errors)
			}
		}
	}
}
