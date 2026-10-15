package dsl

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestParabolicSARParseParametersPredicatesAndCompact(t *testing.T) {
	for phrase, predicate := range map[string]string{"trend bullish": "bullish", "trend bearish": "bearish", "close above": "closeAbove", "close below": "closeBelow", "reverses bullish": "flipBullish", "reverses bearish": "flipBearish", "supports entry": "supportsEntry"} {
		for _, body := range []string{"sar start 0.01\nsar step 0.03\nsar max 0.3\nsar " + phrase, "sar start 0.01 sar step 0.03 sar max 0.3 sar " + phrase} {
			source := "dsl v7\nfilters {\n" + body + "\n}"
			if !strings.Contains(body, "\n") {
				source = "dsl v7\nfilters { " + body + " }"
			}
			parsed, err := Parse(source)
			if err != nil || len(parsed.Errors) != 0 {
				t.Fatal(err, parsed.Errors)
			}
			spec, present, err := DecodeParabolicSAR(parsed.Config)
			if err != nil || !present || spec != (ParabolicSARSpec{.01, .03, .3, predicate}) {
				t.Fatal(spec, present, err)
			}
		}
	}
	p, _ := Parse("dsl v7\nfilters { sar step 0.01 }")
	if len(p.Errors) != 0 {
		t.Fatal(p.Errors)
	}
	want := map[string]any{"start": .02, "step": .01, "max": .2}
	if !reflect.DeepEqual(p.Config["parabolicSAR"], want) {
		t.Fatal(p.Config["parabolicSAR"])
	}
}

func TestParabolicSARParseRejections(t *testing.T) {
	for _, source := range []string{
		"dsl v6\nfilters { sar trend bullish }", "dsl v7\nsetup { sar trend bullish }",
		"dsl v7\nfilters { sar start 0 }", "dsl v7\nfilters { sar step -0.02 }", "dsl v7\nfilters { sar max 1.1 }",
		"dsl v7\nfilters { sar start NaN }", "dsl v7\nfilters { sar step Inf }", "dsl v7\nfilters { sar max 1e999 }",
		"dsl v7\nfilters { sar start 0.3 }", "dsl v7\nfilters { sar max 0.01 }", "dsl v7\nfilters { sar step 0.3 }",
		"dsl v7\nfilters { sar trend bullish\nsar trend bullish }", "dsl v7\nfilters { sar start 0.01\nsar start 0.02 }",
		"dsl v7\nfilters { sar close above extra }", "dsl v7\nfilters { sar trend sideways }", "dsl v7\nfilters { sar }",
		"dsl v7\nfilters { source timeframe 4h\nsar supports entry }", "dsl v7\nfilters { sar supports entry }\nentryTf 15m",
		"dsl v7\nsetup { type: daily flush failure }\nfilters { sar trend bullish }",
	} {
		p, err := Parse(source)
		if err != nil || len(p.Errors) == 0 {
			t.Errorf("accepted %q: %v %v", source, err, p.Errors)
		}
	}
	// Relational validation happens after all directives, so order is free.
	p, _ := Parse("dsl v7\nfilters {\nsar start 0.3\nsar max 0.4\n}")
	if len(p.Errors) != 0 {
		t.Fatal(p.Errors)
	}
}

func TestParabolicSARDirectConfigRejections(t *testing.T) {
	for _, raw := range []any{nil, "bad", map[string]any{"unknown": 1}, map[string]any{"start": "0.02"}, map[string]any{"start": nil}, map[string]any{"step": math.NaN()}, map[string]any{"max": math.Inf(1)}, map[string]any{"start": -.01}, map[string]any{"predicate": ""}, map[string]any{"predicate": "unknown"}, map[string]any{"predicate": false}} {
		_, present, err := DecodeParabolicSAR(Config{"setupType": string(FamilyFlagContinuation), "parabolicSAR": raw})
		if !present || err == nil {
			t.Fatalf("accepted %#v", raw)
		}
	}
	for _, family := range []FamilyID{FamilyTimedReturn, FamilyDownShockRebound, FamilyDailyFlushFailure, FamilyWeekendExtremeFade, FamilyIntraHourRunExhaustion, FamilyLegacySetup9, FamilyClockRangeBreakout, FamilySequentialFull, FamilyRegimeEngine, FamilyMasterStructural} {
		if _, _, err := DecodeParabolicSAR(Config{"setupType": string(family), "parabolicSAR": map[string]any{}}); err == nil {
			t.Fatalf("unsupported family %s", family)
		}
	}
}

func TestParabolicSARFieldSchemaAndFamilyID(t *testing.T) {
	schema, path := loadContractSchema(t, "parabolic-sar-config-v1.schema.json")
	for _, value := range []any{map[string]any{}, map[string]any{"start": .01, "step": .03, "max": .3, "predicate": "supportsEntry"}} {
		if err := validateContract(value, schema, path, schema, "sar"); err != nil {
			t.Fatal(err)
		}
		spec, present, err := DecodeParabolicSAR(Config{"setupType": FamilyFlagContinuation, "parabolicSAR": value})
		if err != nil || !present || spec.Start <= 0 {
			t.Fatal("FamilyID rejected", err)
		}
	}
	for _, value := range []any{nil, "bad", map[string]any{"start": 0.0}, map[string]any{"step": 1.1}, map[string]any{"max": ".2"}, map[string]any{"predicate": "unknown"}, map[string]any{"unknown": 1.0}} {
		if err := validateContract(value, schema, path, schema, "sar"); err == nil {
			t.Fatalf("schema accepted %#v", value)
		}
	}
	// JSON schema cannot express cross-field numeric comparisons: the Go
	// decoder remains authoritative for start/step<=max and enclosing routes.
	if _, _, err := DecodeParabolicSAR(Config{"setupType": FamilyFlagContinuation, "parabolicSAR": map[string]any{"start": .3, "max": .2}}); err == nil {
		t.Fatal("invalid relationship accepted")
	}
}
