package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine/master"
)

func TestMasterPortableCLIExplicitContractAndRawSpread(t *testing.T) {
	args := append(masterCLIInputs(t), "--arithmetic-contract="+master.PortableArithmeticContract)
	var out bytes.Buffer
	if err := run(append([]string{"master-portable-report"}, args...), &out); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["schema"] != "strat-master-structural-portable-cli-v1" {
		t.Fatal("portable schema")
	}
	for _, token := range []string{"-0", "0.000", "1.000"} {
		copy := append([]string(nil), args...)
		copy[5] = "--spread=" + token
		out.Reset()
		if err := runMasterPortableReport(copy, &out); err != nil {
			t.Fatalf("%s:%v", token, err)
		}
		if token == "-0" && !strings.Contains(out.String(), `"spread": -0`) {
			t.Fatal("negative zero not preserved")
		}
	}
	for _, token := range []string{"1.00000000000000001", "0.99999999999999999", "1e-999", "1e0", "+0", "NaN", " 0"} {
		copy := append([]string(nil), args...)
		copy[5] = "--spread=" + token
		out.Reset()
		if err := runMasterPortableReport(copy, &out); err == nil || out.Len() != 0 {
			t.Fatalf("admitted %q", token)
		}
	}
	for _, copy := range [][]string{args[:6], append(append([]string(nil), args...), "--arithmetic-contract="+master.PortableArithmeticContract), append(append([]string(nil), args[:6]...), "--arithmetic-contract=future-v2"), append(append([]string(nil), args...), "--unknown=1")} {
		out.Reset()
		if err := runMasterPortableReport(copy, &out); err == nil || out.Len() != 0 {
			t.Fatal("contract flags accepted", copy)
		}
	}
	// Original native command still rejects the opt-in flag.
	out.Reset()
	if err := runMasterReport(args, &out); err == nil || out.Len() != 0 {
		t.Fatal("portable flag leaked into legacy command")
	}
}

type portableCountingReader struct{ consumed int }

func (r *portableCountingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.consumed += len(p)
	return len(p), nil
}
func TestMasterPortableBoundedStreamRead(t *testing.T) {
	r := &portableCountingReader{}
	if raw, err := readPortableBounded(r, 64); err == nil || raw != nil || r.consumed != 65 {
		t.Fatalf("unbounded consumption %d %v", r.consumed, err)
	}
	raw, err := readPortableBounded(strings.NewReader(strings.Repeat("x", 64)), 64)
	if err != nil || len(raw) != 64 {
		t.Fatal("exact boundary", err)
	}
	raw, err = readPortableBounded(io.LimitReader(strings.NewReader("abc"), 2), 64)
	if err != nil || string(raw) != "ab" {
		t.Fatal("short stream", err)
	}
}
