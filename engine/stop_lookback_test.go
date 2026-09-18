package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
)

func TestParamsFromConfigPreservesRangeBreakFakeStopLookbackPresence(t *testing.T) {
	tests := []struct {
		name string
		cfg  dsl.Config
		want int
	}{
		{
			name: "range break fake explicit zero",
			cfg: dsl.Config{
				"setupType": string(dsl.FamilyRangeBreakFake),
				"stop":      map[string]any{"extremeCandles": 0},
			},
			want: 0,
		},
		{
			name: "range break fake missing value",
			cfg:  dsl.Config{"setupType": string(dsl.FamilyRangeBreakFake)},
			want: 3,
		},
		{
			name: "range break fake positive value",
			cfg: dsl.Config{
				"setupType": string(dsl.FamilyRangeBreakFake),
				"stop":      map[string]any{"extremeCandles": 7},
			},
			want: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := paramsFromConfig(tt.cfg).StopLookbackCandles; got != tt.want {
				t.Fatalf("stop lookback = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRangeBreakFakeAuthoredZeroStopLookbackReachesExecution(t *testing.T) {
	fixture, source := loadStopLookbackCase(t, "family-range-break-fake")
	const authored = "stop beyond last 3 candle extreme by 0.25 ATR"
	const explicitZero = "stop beyond last 0 candle extreme by 0.25 ATR"
	zeroSource := strings.Replace(source, authored, explicitZero, 1)
	if zeroSource == source {
		t.Fatalf("fixture source does not contain %q", authored)
	}

	baseline, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	if baseline.TradeCount == 0 {
		t.Fatal("baseline fixture must trade so the zero-lookback regression is discriminating")
	}

	result, err := RunFixtureCase(fixture, zeroSource)
	if err != nil {
		t.Fatalf("run explicit-zero fixture: %v", err)
	}
	if result.TradeCount != 0 {
		t.Fatalf("explicit-zero stop lookback produced %d trades, want 0", result.TradeCount)
	}
}

func loadStopLookbackCase(t *testing.T, caseName string) (RunFixture, string) {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL: %v", err)
	}
	return fixture, string(source)
}
