package dsl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

const parseFixtureSchema = "dsl-conformance-parse-v1"

type parseFixture struct {
	Schema      string            `json:"schema"`
	Case        string            `json:"case"`
	Category    string            `json:"category"`
	Source      string            `json:"source"`
	Config      json.RawMessage   `json:"cfg"`
	Errors      []string          `json:"errors"`
	Warnings    []string          `json:"warnings"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	DSL         string
}

func loadParseFixtures(t *testing.T) []parseFixture {
	t.Helper()

	dir := parseFixtureDir()
	cfgPaths, err := filepath.Glob(filepath.Join(dir, "*.cfg.json"))
	if err != nil {
		t.Fatalf("glob parse fixture configs: %v", err)
	}
	if len(cfgPaths) == 0 {
		t.Fatalf("no parse fixture configs found in %s", dir)
	}
	sort.Strings(cfgPaths)

	dslPaths, err := filepath.Glob(filepath.Join(dir, "*.strat"))
	if err != nil {
		t.Fatalf("glob parse fixture Strat files: %v", err)
	}
	if len(dslPaths) != len(cfgPaths) {
		t.Fatalf("parse fixture count mismatch: %d .strat files, %d .cfg.json files", len(dslPaths), len(cfgPaths))
	}

	dslByCase := map[string]string{}
	for _, dslPath := range dslPaths {
		caseName := strings.TrimSuffix(filepath.Base(dslPath), ".strat")
		dslByCase[caseName] = dslPath
	}

	fixtures := make([]parseFixture, 0, len(cfgPaths))
	for _, cfgPath := range cfgPaths {
		caseName := strings.TrimSuffix(filepath.Base(cfgPath), ".cfg.json")
		dslPath, ok := dslByCase[caseName]
		if !ok {
			t.Fatalf("%s has no matching .strat fixture", filepath.Base(cfgPath))
		}

		cfgBytes, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("read %s: %v", cfgPath, err)
		}
		var fixture parseFixture
		if err := json.Unmarshal(cfgBytes, &fixture); err != nil {
			t.Fatalf("decode %s: %v", cfgPath, err)
		}

		dslBytes, err := os.ReadFile(dslPath)
		if err != nil {
			t.Fatalf("read %s: %v", dslPath, err)
		}
		fixture.DSL = string(dslBytes)

		validateParseFixtureShape(t, caseName, fixture)
		fixtures = append(fixtures, fixture)
	}

	return fixtures
}

func parseFixtureDir() string {
	return filepath.Join(testsupport.StratConformanceRoot(), "parse")
}

func validateParseFixtureShape(t *testing.T, caseName string, fixture parseFixture) {
	t.Helper()

	if fixture.Schema != parseFixtureSchema {
		t.Fatalf("%s: schema = %q, want %q", caseName, fixture.Schema, parseFixtureSchema)
	}
	if fixture.Case != caseName {
		t.Fatalf("%s: case field = %q", caseName, fixture.Case)
	}
	if fixture.Category == "" {
		t.Fatalf("%s: category is required", caseName)
	}
	if fixture.Source == "" {
		t.Fatalf("%s: source is required", caseName)
	}
	if len(fixture.Config) == 0 || !isJSONObject(fixture.Config) {
		t.Fatalf("%s: cfg must be a JSON object", caseName)
	}
	if fixture.Errors == nil {
		t.Fatalf("%s: errors must be an array", caseName)
	}
	if fixture.Warnings == nil {
		t.Fatalf("%s: warnings must be an array", caseName)
	}
	if fixture.Diagnostics == nil {
		t.Fatalf("%s: diagnostics must be an array", caseName)
	}
	if fixture.DSL == "" {
		t.Fatalf("%s: DSL fixture is empty", caseName)
	}

	for i, raw := range fixture.Diagnostics {
		if err := validateDiagnosticShape(raw); err != nil {
			t.Fatalf("%s: diagnostics[%d]: %v", caseName, i, err)
		}
	}
}

func isJSONObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil
}

func validateDiagnosticShape(raw json.RawMessage) error {
	var diagnostic struct {
		Severity   string  `json:"severity"`
		Message    string  `json:"message"`
		Line       *int    `json:"line"`
		Column     *int    `json:"col"`
		Suggestion *string `json:"suggestion"`
	}
	if err := json.Unmarshal(raw, &diagnostic); err != nil {
		return err
	}
	switch diagnostic.Severity {
	case string(DiagnosticError), string(DiagnosticWarning):
	default:
		return fmt.Errorf("severity = %q", diagnostic.Severity)
	}
	if diagnostic.Message == "" {
		return fmt.Errorf("message is required")
	}
	if diagnostic.Line != nil && *diagnostic.Line < 1 {
		return fmt.Errorf("line must be null or >= 1")
	}
	if diagnostic.Column != nil && *diagnostic.Column < 1 {
		return fmt.Errorf("col must be null or >= 1")
	}
	return nil
}
