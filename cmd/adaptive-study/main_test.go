package main

import (
	"bytes"
	"testing"
)

func TestRefuseWithoutPinnedData(t *testing.T) {
	var b bytes.Buffer
	for _, args := range [][]string{nil, {"missing", "unknown", "B0", "BASIC", "none"}, {"missing", "development", "B0", "BASIC", "none"}} {
		if run(args, &b) == nil || b.Len() != 0 {
			t.Fatal("invalid CLI emitted outcome")
		}
	}
}
