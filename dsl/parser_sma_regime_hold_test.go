package dsl

import "testing"

func TestSMARegimeHoldParsesOneLongOnlyPeriod(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "SMA Regime Hold" { description "Long above the SMA, otherwise cash" }
market conditions { slices(SPYCFD 1d) }
setup {
 type: sma regime hold
 sma period 200
}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %v", result.Errors)
	}
	if result.Config["setupType"] != string(FamilySMARegimeHold) {
		t.Fatalf("setupType = %v", result.Config["setupType"])
	}
	if got := result.Config["smaRegimeHold"].(map[string]any)["period"]; got != 200.0 {
		t.Fatalf("period = %#v, want 200", got)
	}
}

func TestSMARegimeHoldRejectsInvalidPeriodAndExtraRuleDirectives(t *testing.T) {
	for _, setup := range []string{
		"sma period 0",
		"sma period 20.5",
		"sma period Infinity",
		"sma period 2147483648",
	} {
		result, err := Parse("dsl v7\nsetup {\n type: sma regime hold\n " + setup + "\n}")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Errors) == 0 {
			t.Fatalf("setup %q unexpectedly parsed without errors", setup)
		}
	}
	reset, err := Parse("dsl v7\nsetup {\n type: sma regime hold\n sma period 2147483648\n sma period 100\n}")
	if err != nil {
		t.Fatal(err)
	}
	if len(reset.Errors) == 0 || reset.Config["smaRegimeHold"].(map[string]any)["period"] != 100.0 {
		t.Fatalf("invalid period followed by valid value: config=%#v errors=%v", reset.Config["smaRegimeHold"], reset.Errors)
	}
	result, err := Parse("dsl v7\nsetup { type: sma regime hold }\nfilters { side short only }")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("unexpected side directive must be rejected")
	}
	resetFamily, err := Parse("dsl v7\nsetup {\n type: sma regime hold\n type: failed breakout\n}")
	if err != nil {
		t.Fatal(err)
	}
	if len(resetFamily.Errors) == 0 {
		t.Fatal("resetting the family after selecting sma regime hold must be rejected")
	}
}
