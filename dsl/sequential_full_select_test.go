package dsl

import (
	"reflect"
	"testing"
)

func TestSequentialFullReservedSourceNeverFallsBack(t *testing.T) {
	for _, clause := range []string{
		"type: sequential full", "type: sequentialFull", "type: SEQUENTIAL_FULL", "type: sequential", "type: \"sequential full\"", "type==sequential-full",
		"type: sequential full type: flag continuation", "type: flag continuation type: sequential full",
		"unknown sequentialFull null", "type: flag continuation sequential policy E1",
		"type: legacy setup 9 sequential profile seq.full.public_approx.v1",
		"type: flag continuation unknown seq.full.public_approx.v1",
		"type: flag continuation sequential\npolicy E2",
		"type\u2003:\u2003sequential\u2003full",
		"type: sequential : full", "type: sequential/full", "type: sequential.full",
		"(type: sequential full)", "{ type: sequential full }",
		`type: flag continuation type: "seq\u0075ential full\uD800"`,
		`type: flag continuation sequential "profile" "seq.full.public_approx.v1"`,
		`type: flag continuation sequential "policy" E1`,
		`type: flag continuation "sequential" policy E1`,
		`type: flag continuation "sequential" "profile" "seq.full.public_approx.v1"`,
		`"type": sequential full`,
	} {
		sequentialFullReject(t, "dsl v7\nstrategy \"Adversarial\"\nsetup { "+clause+" }")
	}
	for _, clause := range []string{"sequential policy E1", "sequential profile seq.full.public_approx.v1", "sequential riskUsd 25", "sequential maxNotionalUsd 1000", "sequentialFull null"} {
		sequentialFullReject(t, "dsl v7\nstrategy \"Adversarial\"\nsetup { type: flag continuation }\nrisk { stop 2 ATR "+clause+" }")
	}
}

func TestSequentialFullInvalidUnicodeQuotedSelectorIsTyped(t *testing.T) {
	sequentialFullReject(t, "dsl v7\nstrategy \"probe\" {}\nsetup {\n type: flag continuation\n type: \"seq\\u0075ential full\\uD800\"\n}")
	for _, clause := range []string{`sequential "profile" "seq.full.public_approx.v1"`, `sequential "policy" E1`} {
		sequentialFullReject(t, "dsl v7\nstrategy \"probe\" {}\nsetup {\n type: flag continuation\n "+clause+"\n}")
	}
	for _, suffix := range []string{`\q`, `\uZZZZ`, `\u`, `\uD800`, `\u0000\q`} {
		sequentialFullReject(t, `dsl v7
strategy "probe" {}
setup { type: flag continuation type: "seq\u0075ential full`+suffix+`" }`)
	}
	for _, clause := range []string{`"sequential" policy E1`, `"sequential" profile seq.full.public_approx.v1`, `"type" sequential full`, `'sequential' policy E1`, `"seq\u0075ential" policy E1`} {
		sequentialFullReject(t, "dsl v7\nstrategy \"probe\" {}\nsetup {\n type: flag continuation\n "+clause+"\n}")
	}
}

func TestSequentialFullSourceMetadataAndLegacyProfilesStayOpaque(t *testing.T) {
	for _, source := range []string{
		"dsl v7\nstrategy \"sequentialFull\"\nsetup { type: flag continuation }",
		"dsl v7\nstrategy \"Legacy\"\n description \"type: sequential full sequential policy E1\"\nsetup { type: flag continuation }",
		"dsl v7\nstrategy \"Legacy\"\nmarket { symbols (sequentialFull) }\nsetup { type: flag continuation }",
		"dsl v7\nstrategy \"Legacy\"\nsetup { type: flag continuation } # sequential profile seq.full.public_approx.v1",
		`dsl v7
strategy "sequential" { description "type: sequential full" }
market { symbols ("sequential" "policy" "E1") }
setup { type: flag continuation }`,
		`dsl v7
strategy "Legacy" { description "sequential" policy E1 }
setup { type: flag continuation }`,
	} {
		if sequentialFullSourceCandidate(source) {
			t.Fatalf("metadata manufactured full intent: %s", source)
		}
		legacy := newParser(source)
		legacy.parse()
		got, err := Parse(source)
		if err != nil || !reflect.DeepEqual(got, legacy.result()) {
			t.Fatalf("opaque metadata changed legacy parsing: %v %+v", err, got)
		}
	}
	for _, profile := range LegacySetup9Profiles {
		source := legacySetup9Source(" sequential profile "+profile, "")
		compact := "dsl v7\nstrategy \"c\" { description \"c\" }\nmarket conditions { slices(XAUUSD 1h) }\nsetup { type: legacy setup 9 sequential profile " + profile + " }\n"
		for _, candidate := range []string{source, compact} {
			if sequentialFullSourceCandidate(candidate) {
				t.Fatalf("captured legacy profile: %s", candidate)
			}
			result, err := Parse(candidate)
			if err != nil || len(result.Errors) > 0 || result.Config["setupType"] != string(FamilyLegacySetup9) {
				t.Fatalf("legacy parse changed: %v %+v", err, result)
			}
		}
	}
}

func TestSequentialFullDamagedQuoteCannotHideLaterSelector(t *testing.T) {
	for _, quote := range []string{"\"", "'", "`"} {
		source := "dsl v7\nstrategy " + quote + "unfinished\nsetup { type: sequential full }"
		sequentialFullReject(t, source)
	}
}
