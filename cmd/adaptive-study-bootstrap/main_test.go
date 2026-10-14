package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidInputNeverCreatesOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "outputs")
	if run(nil) == nil {
		t.Fatal("usage accepted")
	}
	if run([]string{"missing", out}) == nil {
		t.Fatal("missing input accepted")
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("outputs on refusal")
	}
}
