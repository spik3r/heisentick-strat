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

func TestSMARegimeHoldAuditsAllAuthoredSelectors(t *testing.T) {
	for _, version := range []string{"6", "7"} {
		for _, selector := range []string{"sma regime hold", "smaRegimeHold"} {
			for _, source := range []string{
				"type: " + selector + "\nfilters { side short only }\nexecution { risk: 25 USD }",
				"filters {\n type: " + selector + "\n}",
				"type: " + selector + "\nsetup { type: failed breakout }",
				"setup { type: " + selector + " }\ntype: failed breakout",
			} {
				result, err := Parse("dsl v" + version + "\n" + source)
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Errors) == 0 {
					t.Errorf("v%s accepted %q", version, source)
				}
			}
		}
	}
	for _, source := range []string{
		"setup { leverage 5 type: sma regime hold }",
		"setup { type: sma regime hold } leverage 5",
	} {
		result, err := Parse("dsl v7\n" + source)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Errors) == 0 {
			t.Errorf("accepted uncovered source %q", source)
		}
	}
	result, err := Parse("dsl v7\nstrategy \"type: smaRegimeHold { leverage 5 }\" {\n description \"leverage 5 } {\"\n}\n# leverage 5\nsetup { type: sma regime hold } # ignored\n")
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("quoted metadata/comment control: %v %v", err, result.Errors)
	}
}
