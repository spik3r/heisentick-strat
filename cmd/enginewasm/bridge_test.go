package main

import (
	"encoding/json"
	native "github.com/spik3r/heisentick-strat/engine"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The string bridge must preserve the existing file-loader semantics.
func TestBridgeMatchesFixtureLoader(t *testing.T) {
	paths, err := filepath.Glob("../../conformance/run/deployed-*.fixture.json")
	if err != nil || len(paths) != 4 {
		t.Fatalf("fixtures: %v (%d)", err, len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			fixture, err := native.LoadRunFixture(path)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(strings.TrimSuffix(path, ".fixture.json") + ".strat")
			if err != nil {
				t.Fatal(err)
			}
			result, err := native.RunFixtureCase(fixture, string(source))
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			got, err := runFixture(string(raw), string(source))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatal("string bridge differs from existing file loader")
			}
		})
	}
}
