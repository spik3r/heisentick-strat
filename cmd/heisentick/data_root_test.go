package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRepoRootUsesRepositoryGoMod(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "go", "cmd", "heisentick")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module backtester\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	got, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot() error = %v", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != resolvedRoot {
		t.Fatalf("findRepoRoot() = %q, want %q", got, resolvedRoot)
	}
}
