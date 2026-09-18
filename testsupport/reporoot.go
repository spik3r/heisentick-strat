package testsupport

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// RepoRoot finds the repository from this helper's source location, not from
// the process working directory. Go tests can therefore run from any package
// depth while continuing to load the shared Strat fixtures.
func RepoRoot() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("locate repository-root helper")
	}
	for dir := filepath.Dir(source); ; dir = filepath.Dir(dir) {
		if isFile(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found from %s", source)
		}
	}
}

// MustRepoRoot is convenient for package tests whose setup helpers cannot
// return an error. It panics only when the checkout is missing its module
// markers, which is a test-environment failure rather than a fixture result.
func MustRepoRoot() string {
	root, err := RepoRoot()
	if err != nil {
		panic(err)
	}
	return root
}

// StratConformanceRoot returns the canonical location of the shared parse and
// run corpus used by the JavaScript and Go implementations.
func StratConformanceRoot() string {
	return filepath.Join(MustRepoRoot(), "conformance")
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
