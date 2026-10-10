package dsl

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const sequentialFullTestSource = `dsl v7
strategy "Sequential synthetic E1" {
  description "Producer regression, not a historical study."
}
market {
  sequential symbol SYNTH
  sequential timeframe 1m
}
setup {
  type: sequential full
  sequential profile seq.full.public_approx.v1
  sequential policy E1
}
risk {
  sequential riskUsd 25
  sequential maxNotionalUsd 10000
}`

func sequentialFullParseOK(t *testing.T, source string) Config {
	t.Helper()
	result, err := Parse(source)
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("Parse: %v, %v", err, result.Errors)
	}
	if len(result.Warnings) != 0 || len(result.Diagnostics) != 0 || SequentialFullParseError(result) != nil {
		t.Fatalf("unexpected diagnostic: %+v", result)
	}
	if _, err := DecodeSequentialFullConfig(result.Config); err != nil {
		t.Fatal(err)
	}
	return result.Config
}

func sequentialFullReject(t *testing.T, source string) {
	t.Helper()
	result, err := Parse(source)
	if err != nil || len(result.Errors) == 0 || len(result.Config) != 0 || len(result.Diagnostics) == 0 {
		t.Fatalf("unsupported source escaped strict refusal: err=%v result=%+v source=%q", err, result, source)
	}
	var typed *SequentialFullConfigError
	if !errors.As(SequentialFullParseError(result), &typed) || typed.Code != "unsupported_config" {
		t.Fatalf("missing typed parse error: %+v", result)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code != "unsupported_config" || diagnostic.Severity != DiagnosticError {
			t.Fatalf("incorrect refusal diagnostic: %+v", diagnostic)
		}
	}
}

func TestSequentialFullParsePoliciesAndCanonicalShape(t *testing.T) {
	for _, policy := range []string{"E1", "E2"} {
		cfg := sequentialFullParseOK(t, strings.Replace(sequentialFullTestSource, "policy E1", "policy "+policy, 1))
		spec, _ := DecodeSequentialFullConfig(cfg)
		want := SequentialFullSpec{Profile: SequentialFullProfileID, Policy: policy, Symbol: "SYNTH", Timeframe: "1m", RiskUSD: 25, MaxNotionalUSD: 10000}
		if spec != want || cfg["setupType"] != "sequentialFull" || len(cfg) != 5 {
			t.Fatalf("got %+v, config=%+v", spec, cfg)
		}
	}
}

func TestSequentialFullEquivalentSyntax(t *testing.T) {
	want := sequentialFullParseOK(t, sequentialFullTestSource)
	for _, source := range []string{
		strings.ReplaceAll(sequentialFullTestSource, "\n", " "),
		"# type: flag continuation\n" + sequentialFullTestSource,
		strings.ReplaceAll(sequentialFullTestSource, "sequential ", "SEQUENTIAL "),
		strings.Replace(sequentialFullTestSource, "riskUsd 25", "riskUsd 25.0", 1),
	} {
		if got := sequentialFullParseOK(t, source); !reflect.DeepEqual(got, want) {
			t.Fatalf("equivalent source changed projection: %+v", got)
		}
	}
}

func TestSequentialFullStrictSourceRefusals(t *testing.T) {
	replacements := [][2]string{
		{"dsl v7", "dsl v6"}, {"dsl v7", ""},
		{"type: sequential full", "type: sequential full type: legacy setup 9"},
		{"type: sequential full", "type: flag continuation type: sequential full"},
		{"type: sequential full", ""},
		{"type: sequential full", "sequential policy E1 type: sequential full"},
		{"sequential policy E1", "sequential policy P1"},
		{"sequential policy E1", "sequential policy e1"},
		{"sequential policy E1", "sequential policy E1 sequential policy E2"},
		{"sequential profile seq.full.public_approx.v1", "sequential profile seq.legacy.setup9.v1"},
		{"sequential timeframe 1m", "sequential timeframe M1"},
		{"sequential timeframe 1m", "sequential timeframe 2m"},
		{"sequential symbol SYNTH", "sequential symbol synth"},
		{"sequential symbol SYNTH", "sequential symbol SYNTH,OTHER"},
		{"sequential riskUsd 25", "sequential riskUsd 0"},
		{"sequential riskUsd 25", "sequential riskUsd -25"},
		{"sequential riskUsd 25", "sequential riskUsd NaN"},
		{"sequential riskUsd 25", "sequential riskUsd 1e3"},
		{"sequential maxNotionalUsd 10000", "sequential maxNotionalUsd Infinity"},
		{"sequential maxNotionalUsd 10000", "sequential maxNotionalUsd \"10000\""},
		{"sequential riskUsd 25", "riskUsd 25"},
		{"sequential policy E1", "sequential policy E1 stop beyond last 3 candle extreme by 0.2 ATR"},
		{"sequential policy E1", "sequential policy E1 sequential setupCount 9"},
		{"sequential policy E1", "sequential policy E1 sequential intersection false"},
	}
	for _, replacement := range replacements {
		sequentialFullReject(t, strings.Replace(sequentialFullTestSource, replacement[0], replacement[1], 1))
	}
	for _, directive := range []string{"type: sequential full", "sequential profile seq.full.public_approx.v1", "sequential policy E1", "sequential symbol SYNTH", "sequential timeframe 1m", "sequential riskUsd 25", "sequential maxNotionalUsd 10000"} {
		sequentialFullReject(t, strings.Replace(sequentialFullTestSource, directive, "", 1))
	}
	for _, tail := range []string{" ignored", "}", " setup {}", " execution { fill nextOpen }", " market { calendar synthetic }"} {
		sequentialFullReject(t, sequentialFullTestSource+tail)
	}
	sequentialFullReject(t, strings.Replace(sequentialFullTestSource, "sequential policy E1", "sequential symbol SYNTH", 1))
}

func TestSequentialFullDiagnosticCodeDoesNotChangeLegacySerialization(t *testing.T) {
	old := Diagnostic{Severity: DiagnosticError, Message: "existing"}
	raw, err := json.Marshal(old)
	if err != nil || strings.Contains(string(raw), `"code"`) {
		t.Fatalf("legacy diagnostic shape changed: %s %v", raw, err)
	}
	if SequentialFullParseError(ParseResult{Diagnostics: []Diagnostic{old}}) != nil {
		t.Fatal("ordinary diagnostics were relabeled sequential")
	}
}
