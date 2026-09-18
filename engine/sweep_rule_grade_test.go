package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func TestLevelSweepRuleGradesFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   dsl.Config
		wantHigh string
		wantLow  string
	}{
		{
			name: "independent high grade",
			config: dsl.Config{"sweepRules": map[string]any{
				"high": map[string]any{"grade": "B"},
			}},
			wantHigh: "B",
			wantLow:  "A",
		},
		{
			name: "independent low grade",
			config: dsl.Config{"sweepRules": map[string]any{
				"low": map[string]any{"grade": "B"},
			}},
			wantHigh: "A",
			wantLow:  "B",
		},
		{name: "missing rules default to A", config: dsl.Config{}, wantHigh: "A", wantLow: "A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := paramsFromConfig(tt.config)
			if params.LevelSweepHighGrade != tt.wantHigh || params.LevelSweepLowGrade != tt.wantLow {
				t.Fatalf("level-sweep grades = high %q low %q, want high %q low %q", params.LevelSweepHighGrade, params.LevelSweepLowGrade, tt.wantHigh, tt.wantLow)
			}
		})
	}
}

func TestArchivedRoundNumberConfluenceGradesReachEngineParams(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "strategies", "source", "dslRoundNumberConfluence.strat"))
	if err != nil {
		t.Fatalf("read archived round-number strategy: %v", err)
	}
	parsed := parseEngineTestDSL(t, string(source))
	params := paramsFromConfig(parsed.Config)
	if params.LevelSweepHighGrade != "B" || params.LevelSweepLowGrade != "B" {
		t.Fatalf("archived level-sweep grades = high %q low %q, want B/B", params.LevelSweepHighGrade, params.LevelSweepLowGrade)
	}
}

func TestFailedBreakoutBGradePreservesMarketExecution(t *testing.T) {
	fixture, source := loadLevelSweepLimitCase(t)
	if got := strings.Count(source, "grade A"); got != 2 {
		t.Fatalf("fixture grade A occurrences = %d, want 2", got)
	}
	bSource := strings.ReplaceAll(source, "grade A", "grade B")

	baseline, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	graded, err := RunFixtureCase(fixture, bSource)
	if err != nil {
		t.Fatalf("run B-grade fixture: %v", err)
	}
	if len(baseline.Trades) != 1 || len(graded.Trades) != 1 {
		t.Fatalf("trade counts = baseline %d B-grade %d, want 1/1", len(baseline.Trades), len(graded.Trades))
	}
	if baseline.Trades[0].Meta["grade"] != "A" || !strings.HasSuffix(baseline.Trades[0].Tag, ":A") {
		t.Fatalf("baseline identity = tag %q meta.grade %#v, want :A/A", baseline.Trades[0].Tag, baseline.Trades[0].Meta["grade"])
	}

	want := baseline
	want.Trades = append([]Trade(nil), baseline.Trades...)
	want.Trades[0].Meta = cloneTradeMeta(baseline.Trades[0].Meta)
	want.Trades[0].Meta["grade"] = "B"
	want.Trades[0].Tag = strings.TrimSuffix(baseline.Trades[0].Tag, ":A") + ":B"
	if graded.Trades[0].Meta["grade"] != "B" || !strings.HasSuffix(graded.Trades[0].Tag, ":B") {
		t.Fatalf("B-grade identity = tag %q meta.grade %#v, want :B/B", graded.Trades[0].Tag, graded.Trades[0].Meta["grade"])
	}
	if !reflect.DeepEqual(graded, want) {
		t.Fatalf("B-grade run changed more than tag/meta identity\n got: %#v\nwant: %#v", graded, want)
	}
}

func cloneTradeMeta(meta TradeMeta) TradeMeta {
	cloned := make(TradeMeta, len(meta))
	for key, value := range meta {
		cloned[key] = value
	}
	return cloned
}
