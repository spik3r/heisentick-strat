package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

type d20SemanticTrade struct {
	entryIndex int
	exitIndex  int
	entry      float64
	exit       float64
	reason     string
	rule       string
}

func TestD20SemanticFixturesPreserveBoundaryEconomics(t *testing.T) {
	tests := []struct {
		name   string
		trades []d20SemanticTrade
	}{
		{
			name: "sma-cross-next-open-entry-exit",
			trades: []d20SemanticTrade{
				{entryIndex: 6, exitIndex: 10, entry: 2020, exit: 2020, reason: ReasonRule, rule: "sma-bearish-cross"},
			},
		},
		{
			name: "sma-end-of-test-open-position",
			trades: []d20SemanticTrade{
				{entryIndex: 6, exitIndex: 8, entry: 2020, exit: 2030, reason: ReasonEndOfTest},
			},
		},
		{
			name: "sma-prefix-extension-a",
			trades: []d20SemanticTrade{
				{entryIndex: 6, exitIndex: 10, entry: 2020, exit: 2020, reason: ReasonRule, rule: "sma-bearish-cross"},
			},
		},
		{
			name: "sma-prefix-extension-b",
			trades: []d20SemanticTrade{
				{entryIndex: 6, exitIndex: 10, entry: 2020, exit: 2020, reason: ReasonRule, rule: "sma-bearish-cross"},
				{entryIndex: 14, exitIndex: 16, entry: 2020, exit: 2050, reason: ReasonEndOfTest},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixturePath := filepath.Join(testsupport.StratConformanceRoot(), "semantic", tt.name, "fixture.json")
			fixture, err := LoadRunFixture(fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(filepath.Join(filepath.Dir(fixturePath), "strategy.strat"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := RunFixtureCase(fixture, string(source))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Trades) != len(tt.trades) {
				t.Fatalf("trade count = %d, want %d", len(result.Trades), len(tt.trades))
			}
			for index, want := range tt.trades {
				got := result.Trades[index]
				if got.EntryIndex != want.entryIndex || got.ExitIndex != want.exitIndex || got.Entry != want.entry || got.Exit != want.exit || got.Reason != want.reason || got.Rule != want.rule {
					t.Fatalf("trade %d = %+v, want entry=%v/%d exit=%v/%d reason=%q rule=%q", index, got, want.entry, want.entryIndex, want.exit, want.exitIndex, want.reason, want.rule)
				}
			}
		})
	}
}
