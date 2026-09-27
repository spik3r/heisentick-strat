package report

import (
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func TestResolveHigherTimeframeForStrictAgreement(t *testing.T) {
	for _, mode := range []string{"notAgainst", "strictAgree"} {
		cfg := dsl.Config{"htf": map[string]any{"mode": mode, "timeframe": "auto"}}
		if got := ResolveHigherTimeframe("1h", cfg); got != "4h" {
			t.Errorf("ResolveHigherTimeframe(%q) = %q, want 4h", mode, got)
		}
	}
	if got := ResolveHigherTimeframe("1h", dsl.Config{"htf": map[string]any{"mode": "off", "timeframe": "auto"}}); got != "" {
		t.Fatalf("off mode resolved HTF %q, want empty", got)
	}
}
