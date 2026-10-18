package main

import (
	"encoding/json"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/testsupport"
	"os"
	"path/filepath"
	"testing"
)

func TestSequentialInspectionBridgeMatchesNative(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(testsupport.StratConformanceRoot(), "parse", "*.strat"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 161 {
		t.Fatalf("corpus count: %d", len(paths))
	}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want, err := json.Marshal(engine.InspectSequentialSource(string(source)))
		if err != nil {
			t.Fatal(err)
		}
		if got := inspectSequentialSource(string(source)); got != string(want) {
			t.Fatalf("bridge differs: %s", path)
		}
	}
}

func TestSequentialInspectionSerializationFailureIsClosed(t *testing.T) {
	got := engine.SequentialInspectionFailure("SEQUENTIAL_INSPECTION_SOURCE")
	if inspectSequentialSource("") != sequentialInspectionJSON(got) {
		t.Fatal("native transport failure shape")
	}
}
