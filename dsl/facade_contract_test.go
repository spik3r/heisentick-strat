package dsl_test

import (
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func TestCompatibilityFacadePreservesPublicParserContract(t *testing.T) {
	result, err := dsl.Parse("dsl v7\nstrategy \"Facade contract\"\nsetup { type: flag continuation }\n")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Config["setupType"] != string(dsl.FamilyFlagContinuation) {
		t.Fatalf("setupType = %#v, want %q", result.Config["setupType"], dsl.FamilyFlagContinuation)
	}

	var config dsl.Config = result.Config
	var diagnostic dsl.Diagnostic
	var severity dsl.DiagnosticSeverity = dsl.DiagnosticError
	if config == nil || diagnostic.Severity != "" || severity != "error" {
		t.Fatal("public parser types must remain usable through backtester/go/dsl")
	}

	if got := dsl.HigherTimeframe("15m"); got != "1h" {
		t.Fatalf("HigherTimeframe(15m) = %q, want 1h", got)
	}
	if got := dsl.ResolveHigherTimeframe("15m", "auto"); got != "1h" {
		t.Fatalf("ResolveHigherTimeframe(15m, auto) = %q, want 1h", got)
	}
}
