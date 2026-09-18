package testsupport

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRepoRootAndConformanceRootIgnoreWorkingDirectory(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository-root test")
	}
	wantRoot, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(source), ".."))
	if err != nil {
		t.Fatalf("resolve expected repository root: %v", err)
	}

	gotRoot, err := RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot() error: %v", err)
	}
	gotRoot, err = filepath.EvalSymlinks(gotRoot)
	if err != nil {
		t.Fatalf("resolve RepoRoot() result: %v", err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("RepoRoot() = %q, want %q", gotRoot, wantRoot)
	}

	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalCWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(filepath.Join(wantRoot, "cmd")); err != nil {
		t.Fatalf("change to alternate go working directory: %v", err)
	}

	gotRoot, err = RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot() from go working directory: %v", err)
	}
	gotRoot, err = filepath.EvalSymlinks(gotRoot)
	if err != nil {
		t.Fatalf("resolve alternate-cwd RepoRoot() result: %v", err)
	}
	if gotRoot != wantRoot {
		t.Fatalf("RepoRoot() from go working directory = %q, want %q", gotRoot, wantRoot)
	}

	conformanceRoot := StratConformanceRoot()
	if conformanceRoot != filepath.Join(wantRoot, "conformance") {
		t.Fatalf("StratConformanceRoot() = %q, want %q", conformanceRoot, filepath.Join(wantRoot, "conformance"))
	}
	fixture := filepath.Join(conformanceRoot, "parse", "setup-channel-break-hold.strat")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("conformance fixture from alternate go working directory: %v", err)
	}
}
