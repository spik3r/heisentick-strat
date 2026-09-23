package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestCheckPassesOnTheCommittedCorpus(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"check", "--dir=" + testsupport.StratConformanceRoot()}, &out); err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(out.String(), "conformance OK: 43 parse goldens, 34 run goldens, 0 TODO") {
		t.Fatalf("unexpected check summary: %s", out.String())
	}
}

func TestRegenIsIdempotentAndWritesTheScoreboard(t *testing.T) {
	dir := copyCorpus(t)
	before := snapshot(t, dir)

	var out bytes.Buffer
	if err := run([]string{"regen", "--dir=" + dir}, &out); err != nil {
		t.Fatalf("regen: %v", err)
	}
	if !strings.Contains(out.String(), "wrote 43 parse goldens, 34 run goldens (0 TODO)") {
		t.Fatalf("unexpected regen summary: %s", out.String())
	}
	after := snapshot(t, dir)
	for path, content := range before {
		if !bytes.Equal(content, after[path]) {
			t.Fatalf("regen changed %s", path)
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, metadataFile))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		GoRun struct {
			Implemented []string          `json:"implemented"`
			TODO        map[string]string `json:"todo"`
		} `json:"goRun"`
		RunCases int `json:"runCases"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.GoRun.Implemented) != 34 || metadata.RunCases != 34 || len(metadata.GoRun.TODO) != 0 {
		t.Fatalf("scoreboard = %+v", metadata)
	}
	if !bytes.Contains(raw, []byte(`"todo": {}`)) {
		t.Fatalf("empty TODO map must serialize as {}: %s", raw)
	}
}

func TestCheckReportsADriftedGolden(t *testing.T) {
	dir := copyCorpus(t)
	path := filepath.Join(dir, "run", "money-stop-distance-gate.trades.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Replace(raw, []byte(`"tradeCount": 0`), []byte(`"tradeCount": 1`), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	err = run([]string{"check", "--dir=" + dir}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "money-stop-distance-gate.trades.json differs") {
		t.Fatalf("check error = %v, want the drifted golden named", err)
	}
	if !strings.Contains(err.Error(), "reviewed PR") {
		t.Fatalf("check error does not point at the regeneration rule: %v", err)
	}
}

func TestUnclassifiedFixtureIsRefused(t *testing.T) {
	dir := copyCorpus(t)
	for _, suffix := range []string{".fixture.json", ".strat"} {
		src := filepath.Join(dir, "run", "money-risk-sizing"+suffix)
		raw, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "run", "money-unclassified"+suffix), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := run([]string{"regen", "--dir=" + dir}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "money-unclassified is neither") {
		t.Fatalf("regen error = %v, want the unclassified case refused", err)
	}
}

func TestStableJSONMatchesTheCorpusStyle(t *testing.T) {
	got, err := stableJSON(map[string]any{
		"b":     []string{},
		"a":     map[string]any{"y": 1651230000000.0, "x": "<&>"},
		"empty": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": {\n    \"x\": \"<&>\",\n    \"y\": 1651230000000\n  },\n  \"b\": [],\n  \"empty\": {}\n}\n"
	if string(got) != want {
		t.Fatalf("stableJSON = %q, want %q", got, want)
	}
}

func copyCorpus(t *testing.T) string {
	t.Helper()
	src := testsupport.StratConformanceRoot()
	dst := t.TempDir()
	for _, sub := range []string{"parse", "run"} {
		if err := os.MkdirAll(filepath.Join(dst, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(filepath.Join(src, sub))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			raw, err := os.ReadFile(filepath.Join(src, sub, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dst, sub, entry.Name()), raw, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	raw, err := os.ReadFile(filepath.Join(src, metadataFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, metadataFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

func snapshot(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = raw
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
