package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduledTradeFixtureCommand(t *testing.T) {
	root := "../.."
	fixture := filepath.Join(root, "conformance/run/family-scheduled-trade.fixture.json")
	source := filepath.Join(root, "conformance/run/family-scheduled-trade.strat")
	var out bytes.Buffer
	if err := run([]string{"scheduled-trade-fixture", "--fixture-path=" + fixture, "--strategy-path=" + source}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"tradeCount": 1`) || !strings.Contains(out.String(), `"scheduledTrade"`) {
		t.Fatalf("output=%s", out.String())
	}
	pivot := filepath.Join(t.TempDir(), "pivot.json")
	if err := os.WriteFile(pivot, []byte(strings.Replace(string(rawFixture(t, fixture)), `"rangeMethod": "zone"`, `"rangeMethod": "pivot"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"scheduled-trade-fixture", "--fixture-path=" + pivot, "--strategy-path=" + source}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"rangeMethod": "pivot"`) {
		t.Fatalf("result lost declared range method: %s", out.String())
	}
	wrongFamily := filepath.Join(root, "conformance/run/family-timed-return.strat")
	if err := run([]string{"scheduled-trade-fixture", "--fixture-path=" + fixture, "--strategy-path=" + wrongFamily}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "requires the scheduled trade family") {
		t.Fatalf("wrong-family error=%v", err)
	}
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	malformed := filepath.Join(t.TempDir(), "malformed.json")
	sourceText := strings.Replace(string(raw), "[1609807800000,110.00,110.01,109.99,110.00,1]", "[1609807800000,110.00,110.01,109.99,110.00]", 1)
	if err = os.WriteFile(malformed, []byte(sourceText), 0600); err != nil {
		t.Fatal(err)
	}
	if err = run([]string{"scheduled-trade-fixture", "--fixture-path=" + malformed, "--strategy-path=" + source}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "six-value") {
		t.Fatalf("malformed column error=%v", err)
	}
	nullValue := filepath.Join(t.TempDir(), "null-value.json")
	nullJSON := strings.Replace(string(raw), "110.00,110.01", "null,110.01", 1)
	if err = os.WriteFile(nullValue, []byte(nullJSON), 0600); err != nil {
		t.Fatal(err)
	}
	if err = run([]string{"scheduled-trade-fixture", "--fixture-path=" + nullValue, "--strategy-path=" + source}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "not a number") {
		t.Fatalf("null raw column error=%v", err)
	}
}

func rawFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
