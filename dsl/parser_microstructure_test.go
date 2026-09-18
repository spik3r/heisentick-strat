package dsl

import "testing"

func TestMicrostructureFilters(t *testing.T) {
	result, err := Parse(`dsl v7
strategy "Micro" { description "Causal event filters." }
filters {
  micro spread ticks at most 3
  micro liquidity imbalance above 0.2
  micro quote flow imbalance below -10 over 5s
}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	filters, ok := result.Config["microstructureFilters"].([]any)
	if !ok || len(filters) != 3 {
		t.Fatalf("microstructure filters = %#v", result.Config["microstructureFilters"])
	}
	quoteFlow := filters[2].(map[string]any)
	if quoteFlow["windowMs"] != float64(5000) || quoteFlow["capability"] != "l1Size" {
		t.Fatalf("quote-flow filter = %#v", quoteFlow)
	}
}

func TestMicrostructureFiltersRejectMissingWindow(t *testing.T) {
	result, err := Parse("dsl v7\nfilters { micro quote rate above 4 }")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected an explicit-window error")
	}
}
