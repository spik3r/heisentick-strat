package dsl

import (
	"reflect"
	"strings"
	"testing"
)

func TestFrozenSelectionReservesMalformedSelectorsBeforeRelabel(t *testing.T) {
	for _, value := range []string{`"frozen level breakout"`, `'frozen level breakout'`, "`frozen level breakout`", `frozenLevelBreakout`, `FROZENLEVELBREAKOUT`, `"\u0066rozen level breakout"`, `frozen`, `frozen-level-breakout`, "\nfrozen level breakout", `/*invalid*/ frozen level breakout`, `"\ud800frozen level breakout"`, `unknown frozenLevelBreakout`} {
		for _, suffix := range []string{"", " type: flag continuation"} {
			source := "dsl v7\nstrategy \"x\"\nsetup { type: " + value + suffix + " }"
			r, err := Parse(source)
			if err != nil || len(r.Errors) == 0 || len(r.Config) != 0 {
				t.Errorf("selector %q became %v: %v %v", value+suffix, r.Config["setupType"], err, r.Errors)
			}
		}
	}
	for _, quote := range []string{`"`, "'", "`"} {
		for _, head := range []string{"description ", "name ", "strategy "} {
			for _, newline := range []string{"\n", "\r\n"} {
				source := "dsl v7\nstrategy \"x\"\n" + head + quote + "missing end" + newline + "setup { type: frozen level breakout type: flag continuation }"
				r, err := Parse(source)
				if err != nil || len(r.Errors) == 0 || len(r.Config) != 0 {
					t.Errorf("damaged %q metadata hid selector: %v %v", head+quote, err, r.Errors)
				}
			}
		}
		// A missing closing quote on the same physical line must also not hide
		// a selector that the permissive inline parser might otherwise see.
		source := "dsl v7\nstrategy \"x\"\nsetup { description " + quote + "missing type: frozen level breakout type: flag continuation }"
		r, err := Parse(source)
		if err != nil || len(r.Errors) == 0 || len(r.Config) != 0 {
			t.Fatalf("damaged inline quote hid selector: %v %v", err, r.Errors)
		}
	}
}

func TestFrozenSelectionPreservesLegacyValuesAndMetadata(t *testing.T) {
	clauses := []string{
		"symbols FROZEN", "symbols FROZEN CONTROL", "symbols(FROZEN)", "symbols(FROZEN, BTCUSDT)",
		"slices(FROZEN 1h, BTCUSDT 1m)", "description frozen lock pivot", "name frozen control",
	}
	for _, q := range []string{`"`, "'", "`"} {
		clauses = append(clauses, "symbols "+q+"FROZEN"+q, "symbols("+q+"FROZEN"+q+", BTCUSDT)", "description "+q+"type: frozen level breakout # { frozen lock pivot }"+q, "name "+q+"frozenLevelBreakout"+q)
	}
	for _, clause := range clauses {
		t.Run(clause, func(t *testing.T) {
			source := "dsl v7\nstrategy \"Legacy\"\n" + clause + "\nsetup { type: flag continuation }"
			old := newParser(source)
			old.parse()
			want := old.result()
			if len(want.Errors) > 0 {
				t.Fatalf("invalid old-family positive control: %v", want.Errors)
			}
			got, err := Parse(source)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("valid old value changed: %v %v", err, got.Errors)
			}
		})
	}
	// A complete type-looking phrase remains inert in every metadata quote
	// style; only its grammatical use as a selector reserves this family.
	for _, q := range []string{`"`, "'", "`"} {
		source := "dsl v7\nstrategy " + q + "type: frozen level breakout" + q + "\nsetup { type: flag continuation }"
		if frozenSourceCandidate(source) {
			t.Fatal("quoted strategy metadata selected family")
		}
		old := newParser(source)
		old.parse()
		want := old.result()
		got, _ := Parse(source)
		if !reflect.DeepEqual(got, want) {
			t.Fatal("quoted strategy metadata changed")
		}
	}
	for _, source := range []string{"dsl v7\nsetup { frozen control stored-m30-lock-isolation-v1 }", "dsl v7\nrisk { frozen grid .01 price units }"} {
		r, _ := Parse(source)
		if len(r.Errors) == 0 || len(r.Config) != 0 || !strings.Contains(r.Errors[0], "frozen level breakout") {
			t.Fatal("authored frozen directive not reserved")
		}
	}
}
