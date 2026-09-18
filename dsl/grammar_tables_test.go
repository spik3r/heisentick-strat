package dsl

import (
	"regexp"
	"strings"
	"testing"
)

func TestGeneratedGrammarProvenance(t *testing.T) {
	if GrammarGeneratorVersion != "dsl-grammar-generator/v1" {
		t.Fatalf("GrammarGeneratorVersion = %q", GrammarGeneratorVersion)
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(GrammarManifestSHA256) {
		t.Fatalf("GrammarManifestSHA256 = %q, want 64 lowercase hex characters", GrammarManifestSHA256)
	}
}

func TestGeneratedDeprecationsKeepATRCaseSensitive(t *testing.T) {
	result, err := Parse("dsl v7\nstrategy \"tables\"\nsetup {\n type: opening range breakout\n stop beyond last 3 candle extreme by 0.2 ATR\n}")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, `"ATR" is deprecated`) {
			t.Fatalf("ordinary ATR unit was reported as deprecated: %s", warning)
		}
	}
}
