package dsl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestAdaptiveFlagRuntimeStrictWrapperNamedParseParity(t *testing.T) {
	for _, bundle := range []string{"initial", "tweaked", "snapshot_c"} {
		source, err := os.ReadFile(filepath.Join(testsupport.StratConformanceRoot(), "parse", "family-adaptive-volume-flag-"+bundle+".strat"))
		if err != nil {
			t.Fatal(err)
		}
		for _, tf := range []string{"M30", "H1"} {
			t.Run(bundle+"/"+tf, func(t *testing.T) {
				input := strings.Replace(string(source), "timeframe M30", "timeframe "+tf, 1)
				legacy, err := Parse(input)
				if err != nil || len(legacy.Errors) != 0 {
					t.Fatalf("legacy source: %v, %v", err, legacy.Errors)
				}
				strict, err := ParseAdaptiveVolumeFlagRuntimeSource(input)
				if err != nil || !reflect.DeepEqual(strict, legacy) {
					t.Fatalf("ParseResult changed: %v\nstrict: %#v\nlegacy: %#v", err, strict, legacy)
				}
				for _, values := range [][2]any{{strict, legacy}, {strict.Config, legacy.Config}} {
					a, ea := json.Marshal(values[0])
					b, eb := json.Marshal(values[1])
					if ea != nil || eb != nil || !bytes.Equal(a, b) {
						t.Fatalf("exact encoding changed: %v, %v", ea, eb)
					}
				}
				if len(strict.Warnings) != 1 || strict.Warnings[0] != AdaptiveFlagDedicatedRunnerRequired {
					t.Fatal("dedicated-runner warning was removed")
				}
			})
		}
	}
}

func TestAdaptiveFlagRuntimeStrictWrapperByteLimit(t *testing.T) {
	for _, padding := range []string{" ", "#" + strings.Repeat("x", 10) + "\n", "# λ\n"} {
		source := adaptiveFlagTestSource
		for len(source)+len(padding) <= AdaptiveFlagRuntimeMaxSourceBytes {
			source += padding
		}
		source += strings.Repeat(" ", AdaptiveFlagRuntimeMaxSourceBytes-len(source))
		got, err := ParseAdaptiveVolumeFlagRuntimeSource(source)
		if err != nil || len(got.Errors) != 0 || len(source) != 4096 {
			t.Fatalf("4096-byte valid source rejected: %v, %v", err, got.Errors)
		}
		for _, oversized := range []string{source + " ", source + "λ", strings.Repeat("{", 4097)} {
			got, err = ParseAdaptiveVolumeFlagRuntimeSource(oversized)
			if err == nil || !reflect.DeepEqual(got, ParseResult{}) {
				t.Fatal("oversized source reached strict parser", len(oversized), got, err)
			}
		}
		// The additive wrapper must not impose its source cap on the legacy path.
		legacy, err := Parse(source + " ")
		if err != nil || len(legacy.Errors) != 0 {
			t.Fatalf("legacy source admission changed: %v, %v", err, legacy.Errors)
		}
	}
}

func TestAdaptiveFlagRuntimeStrictWrapperPreservesDiagnostics(t *testing.T) {
	// This exact 800-byte wrong-family pattern caused 32,768 diagnostics via
	// generic dispatch in the admission review. Never call generic Parse here.
	clock := "dsl v7\nstrategy \"Wrong-family probe\" {\n description \"invented\"\n}\nmarket conditions {\n symbols(" + strings.Repeat("x ", 128) + ")\n timeframes(" + strings.Repeat("x ", 128) + ")\n clock UTC+10\n}\nsetup {\n type: clock range breakout\n range 11:05 to 14:05\n orders expire 03:00\n buffer 0 pips\n}\nrisk {\n stop 1 percent\n}\nmanagement {\n close positions at 03:00\n}\n"
	malformed := []string{
		"", "dsl v7", clock,
		strings.Replace(adaptiveFlagTestSource, "type: adaptive volume flag", "type: adaptive volume flag type: clock range breakout", 1),
		strings.Repeat("setup { setup {", 4096/len("setup { setup {")),
		strings.Replace(adaptiveFlagTestSource, "adaptiveflag targetR 2.5", "adaptiveflag targetR NaN", 1),
		strings.Replace(adaptiveFlagTestSource, "adaptiveflag bundle INITIAL", "adaptiveflag bundle initial", 1),
		strings.Replace(adaptiveFlagTestSource, "adaptiveflag timeframe M30", "adaptiveflag timeframe Ｍ30", 1),
		adaptiveFlagTestSource + " risk { risk 0 USD }",
		adaptiveFlagTestSource + string([]byte{0xff}),
	}
	for i, source := range malformed {
		got, err := ParseAdaptiveVolumeFlagRuntimeSource(source)
		want := parseAdaptiveVolumeFlagSource(source)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d: strict ParseResult lost: %v", i, err)
		}
		if len(got.Config) != 0 || len(got.Errors) != 1 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Severity != DiagnosticError {
			t.Fatalf("case %d: wrong-family/malformed source escaped bounded strict refusal: %#v", i, got)
		}
	}
	// The wrapper owns neither new grammar nor the report's named-only profile.
	for _, source := range []string{
		strings.Replace(adaptiveFlagTestSource, "bundle INITIAL", "bundle CUSTOM", 1),
		strings.ReplaceAll(adaptiveFlagTestSource, "adaptiveflag ", "ADAPTIVEFLAG "),
		strings.Replace(adaptiveFlagTestSource, "Input identity only; dedicated raw reference runner", "clock range breakout λ; adaptiveflag bundle CUSTOM", 1),
	} {
		got, err := ParseAdaptiveVolumeFlagRuntimeSource(source)
		if err != nil || len(got.Errors) != 0 || !reflect.DeepEqual(got, parseAdaptiveVolumeFlagSource(source)) {
			t.Fatalf("strict grammar changed: %v, %v", err, got.Errors)
		}
	}
}
