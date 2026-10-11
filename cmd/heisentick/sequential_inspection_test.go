package main

import (
	"bytes"
	"encoding/json"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/testsupport"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSequentialInspectionCLI(t *testing.T) {
	path := filepath.Join(testsupport.StratConformanceRoot(), "run", "family-sequential-full-e1-long.strat")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"inspect-sequential-source", "--dsl-file=" + path}, &out); err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(engine.InspectSequentialSource(string(source)))
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != string(want)+"\n" {
		t.Fatal("CLI changed inspection")
	}
	if !strings.Contains(usageText(), "inspect-sequential-source") {
		t.Fatal("missing help")
	}
}

func TestSequentialInspectionCLIRejectsArgumentsAndInput(t *testing.T) {
	for _, args := range [][]string{nil, {"--dsl-file=a", "--dsl-file=b"}, {"--dsl-file=a", "--ready=true"}, {"--source=a"}, {"a"}, {"--dsl-file"}, {"--dsl-file", "1"}, {"--dsl-file="}} {
		var out bytes.Buffer
		if err := runSequentialInspection(args, &out); err == nil || out.Len() != 0 {
			t.Fatalf("unsafe arguments: %v", args)
		}
	}
	path := filepath.Join(t.TempDir(), "invalid.strat")
	if err := os.WriteFile(path, []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, filepath.Join(t.TempDir(), "sensitive-missing-path")} {
		var out bytes.Buffer
		err := runSequentialInspection([]string{"--dsl-file=" + p}, &out)
		if err == nil || strings.Contains(err.Error(), p) || strings.Contains(out.String(), p) {
			t.Fatal("input error leaked path or succeeded")
		}
		var raw map[string]any
		if e := json.Unmarshal(out.Bytes(), &raw); e != nil || raw["ok"] != false || raw["result"] != nil || raw["capabilities"] != nil {
			t.Fatal("failure envelope not closed")
		}
	}
}

func TestSequentialInspectionCLIRequiresExplicitPath(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(testsupport.StratConformanceRoot(), "run", "family-sequential-full-e1-long.strat"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1"), source, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
	var out bytes.Buffer
	if err := run([]string{"inspect-sequential-source", "--dsl-file"}, &out); err == nil || out.Len() != 0 || !strings.HasPrefix(err.Error(), "SEQUENTIAL_INSPECTION_ARGUMENTS:") {
		t.Fatal("missing value inspected implicit cwd/1")
	}
	if err := run([]string{"inspect-sequential-source", "--dsl-file=1"}, &out); err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(engine.InspectSequentialSource(string(source)))
	if err != nil || out.String() != string(want)+"\n" {
		t.Fatal("explicit literal filename 1 was not inspected")
	}
}
