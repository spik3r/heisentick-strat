package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRefusalWritesNoPartialSuccess(t *testing.T) {
	p := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(p, []byte(`{"mode":"execute"}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {p}, {p, p}} {
		var out bytes.Buffer
		if err := run(args, &out); err == nil || out.Len() != 0 {
			t.Fatal("refusal must produce no success JSON")
		}
	}
}
