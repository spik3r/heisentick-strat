package dsl

import "testing"

func TestHigherTimeframeStrictAgreementMode(t *testing.T) {
	for _, tc := range []struct {
		line string
		want string
	}{
		{"higher timeframe must be directional and agree", "strictAgree"},
		{"higher timeframe legacy bias must be directional and agree", "strictLegacyAgree"},
		{"higher timeframe must agree", "notAgainst"},
		{"higher timeframe must not oppose entry", "notAgainst"},
		{"higher timeframe off", "off"},
	} {
		t.Run(tc.line, func(t *testing.T) {
			result, err := Parse("dsl v7\nstrategy \"HTF mode\"\nsetup { type: named level sweep }\nfilters { " + tc.line + " }\n")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(result.Errors) != 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			htf, ok := result.Config["htf"].(map[string]any)
			if !ok || htf["mode"] != tc.want {
				t.Fatalf("htf config = %#v, want mode %q", result.Config["htf"], tc.want)
			}
		})
	}
}

func TestHigherTimeframeStrictAgreementIsScopedToNamedLevelSweep(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "Strict HTF scope"
setup { type: failed breakout }
filters { higher timeframe must be directional and agree }
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 1 || result.Errors[0] != `strict directional higher timeframe agreement is currently supported only by "named level sweep" setups` {
		t.Fatalf("errors = %#v, want scoped-use diagnostic", result.Errors)
	}
}

func TestNamedLevelSweepRepeatedLevelPhraseDisablesDailyDedup(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "Repeated sweep"
setup {
  type: named level sweep
  allow repeated level sweeps in one local day
}
`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	nls := result.Config["namedLevelSweep"].(map[string]any)
	if nls["dedupDaily"] != false {
		t.Fatalf("namedLevelSweep config = %#v, want dedupDaily false", nls)
	}
}
