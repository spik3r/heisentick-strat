package dsl

import "testing"

func TestRelativeMeasuredVolatilityFilter(t *testing.T) {
	result, err := Parse(`dsl v7
filters {
  rmv atr period 10
  rmv lookback 80
  rmv at least 65
}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	rmv, ok := result.Config["relativeMeasuredVolatility"].(map[string]any)
	if !ok {
		t.Fatalf("RMV config = %#v", result.Config["relativeMeasuredVolatility"])
	}
	if rmv["op"] != "atLeast" || rmv["value"] != float64(65) || rmv["atrPeriod"] != float64(10) || rmv["lookback"] != float64(80) {
		t.Fatalf("RMV config = %#v", rmv)
	}
}

func TestRelativeMeasuredVolatilityDefaultsAndParameterOnly(t *testing.T) {
	result, err := Parse("dsl v7\nfilters { rmv lookback 40 }")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	rmv := result.Config["relativeMeasuredVolatility"].(map[string]any)
	if rmv["atrPeriod"] != float64(14) || rmv["lookback"] != float64(40) {
		t.Fatalf("RMV defaults = %#v", rmv)
	}
}

func TestRelativeMeasuredVolatilityRejectsInvalidThresholdAndLength(t *testing.T) {
	for _, source := range []string{
		"dsl v7\nfilters { rmv above 101 }",
		"dsl v7\nfilters { rmv atr period 1.5 }",
		"dsl v7\nfilters { rmv lookback 0 }",
	} {
		result, err := Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Errors) == 0 {
			t.Errorf("Parse(%q) accepted invalid RMV directive", source)
		}
	}
}

func TestRelativeMeasuredVolatilityRejectsDuplicateFields(t *testing.T) {
	for _, source := range []string{
		"dsl v7\nfilters { rmv above 70\nrmv below 30 }",
		"dsl v7\nfilters { rmv atr period 10\nrmv atr period 20 }",
		"dsl v7\nfilters { rmv lookback 50\nrmv lookback 100 }",
	} {
		result, err := Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Errors) == 0 {
			t.Errorf("Parse(%q) accepted a duplicate RMV field", source)
		}
	}
}

func TestRelativeMeasuredVolatilitySourceTimeframeNeedsEntryRoute(t *testing.T) {
	unsupported, err := Parse("dsl v7\nfilters {\nsource timeframe 4h\nrmv above 50\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(unsupported.Errors, "RMV with a source timeframe requires a supported source-entry route") {
		t.Fatalf("source-only RMV errors = %v", unsupported.Errors)
	}

	supported, err := Parse(`dsl v7
market conditions {
  slices(XAUUSD 15m)
}
filters {
  source timeframe 4h
  rmv above 50
}
entryTf 15m`)
	if err != nil {
		t.Fatal(err)
	}
	if len(supported.Errors) != 0 {
		t.Fatalf("supported source-entry RMV errors = %v", supported.Errors)
	}

	nonGoldSymbol, err := Parse(`dsl v7
market conditions {
  slices(EURUSD 15m)
}
filters {
  source timeframe 4h
  rmv above 50
}
entryTf 15m`)
	if err != nil {
		t.Fatal(err)
	}
	if !containsError(nonGoldSymbol.Errors, "RMV with a source timeframe requires a supported source-entry route") {
		t.Fatalf("non-XAUUSD source-entry RMV errors = %v", nonGoldSymbol.Errors)
	}
}
