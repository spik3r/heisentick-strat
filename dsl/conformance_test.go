package dsl

import (
	"encoding/json"
	"testing"
)

func TestParseConformance(t *testing.T) {
	fixtures := loadParseFixtures(t)

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Case, func(t *testing.T) {
			result, err := Parse(fixture.DSL)
			if err != nil {
				t.Fatalf("parse %s: %v", fixture.Case, err)
			}
			assertParseFixtureMatch(t, fixture, result)
		})
	}
}

func assertParseFixtureMatch(t *testing.T, fixture parseFixture, result ParseResult) {
	t.Helper()

	var expectedConfig any
	if err := json.Unmarshal(fixture.Config, &expectedConfig); err != nil {
		t.Fatalf("decode expected cfg: %v", err)
	}
	if got, want := canonicalJSON(result.Config), canonicalJSON(expectedConfig); got != want {
		t.Fatalf("cfg mismatch\n got: %s\nwant: %s", got, want)
	}
	if got, want := canonicalJSON(result.Errors), canonicalJSON(fixture.Errors); got != want {
		t.Fatalf("errors mismatch\n got: %s\nwant: %s", got, want)
	}
	if got, want := canonicalJSON(result.Warnings), canonicalJSON(fixture.Warnings); got != want {
		t.Fatalf("warnings mismatch\n got: %s\nwant: %s", got, want)
	}

	var expectedDiagnostics any
	if err := json.Unmarshal(mustJSON(fixture.Diagnostics), &expectedDiagnostics); err != nil {
		t.Fatalf("decode expected diagnostics: %v", err)
	}
	var gotDiagnostics any
	if err := json.Unmarshal(mustJSON(result.Diagnostics), &gotDiagnostics); err != nil {
		t.Fatalf("decode got diagnostics: %v", err)
	}
	if got, want := canonicalJSON(gotDiagnostics), canonicalJSON(expectedDiagnostics); got != want {
		t.Fatalf("diagnostics mismatch\n got: %s\nwant: %s", got, want)
	}
}

func mustJSON(value any) []byte {
	bytes, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return bytes
}
