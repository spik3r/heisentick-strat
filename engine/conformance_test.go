package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestRunConformance(t *testing.T) {
	fixturePaths, err := filepath.Glob(filepath.Join(runFixtureDir(), "*.fixture.json"))
	if err != nil {
		t.Fatalf("glob run fixtures: %v", err)
	}
	if len(fixturePaths) == 0 {
		t.Fatalf("no run fixtures found")
	}
	sort.Strings(fixturePaths)

	seen := make(map[string]bool, len(fixturePaths))
	passed := 0
	skipped := 0

	for _, fixturePath := range fixturePaths {
		caseName := strings.TrimSuffix(filepath.Base(fixturePath), ".fixture.json")
		seen[caseName] = true
		if reason, ok := runConformanceTODO[caseName]; ok {
			t.Run(caseName, func(t *testing.T) {
				t.Skip(reason)
			})
			skipped++
			continue
		}
		if !runCaseImplemented(caseName) {
			t.Fatalf("%s is neither implemented nor listed in runConformanceTODO", caseName)
		}
		t.Run(caseName, func(t *testing.T) {
			fixture, err := LoadRunFixture(fixturePath)
			if err != nil {
				t.Fatalf("load fixture: %v", err)
			}
			dslSource, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
			if err != nil {
				t.Fatalf("read DSL: %v", err)
			}
			result, err := RunFixtureCase(fixture, string(dslSource))
			if err != nil {
				t.Fatalf("run fixture: %v", err)
			}
			expected, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".trades.json"))
			if err != nil {
				t.Fatalf("read expected trades: %v", err)
			}
			if got, want := canonicalJSON(conformanceResult(result)), canonicalRawJSON(t, expected); got != want {
				t.Fatalf("trade mismatch\nfirst diff: %s\n got: %s\nwant: %s", firstDiff(got, want), got, want)
			}
			passed++
		})
	}

	for caseName := range runConformanceTODO {
		if !seen[caseName] {
			t.Fatalf("runConformanceTODO lists missing fixture %q", caseName)
		}
	}
	if passed != len(implementedRunCases) || skipped != len(runConformanceTODO) {
		t.Fatalf("run-conformance scoreboard pass=%d skip=%d todo=%d", passed, skipped, len(runConformanceTODO))
	}
	t.Logf("run-conformance scoreboard: pass=%d (%s), skipped=%d explicit TODO", passed, strings.Join(implementedRunCases, ", "), skipped)
}

func conformanceResult(result RunResult) RunResult {
	if result.Trades == nil {
		return result
	}
	trades := make([]Trade, len(result.Trades))
	copy(trades, result.Trades)
	result.Trades = trades
	for i := range result.Trades {
		result.Trades[i].Partial = false
	}
	return result
}

func TestConformanceResultPreservesTradeSliceShapeAndOnlyClearsPartial(t *testing.T) {
	nilResult := conformanceResult(RunResult{})
	if nilResult.Trades != nil {
		t.Fatalf("nil trades projected as %#v, want nil", nilResult.Trades)
	}

	emptyResult := conformanceResult(RunResult{Trades: []Trade{}})
	if emptyResult.Trades == nil || len(emptyResult.Trades) != 0 {
		t.Fatalf("empty trades projected as %#v, want nonnil empty slice", emptyResult.Trades)
	}

	trade := Trade{
		Entry:      100,
		EntryIndex: 1,
		EntryT:     1000,
		Exit:       102,
		ExitIndex:  2,
		ExitT:      2000,
		InitialSL:  99,
		InitialTP:  104,
		Meta:       TradeMeta{"setup": "flag", "gradeScore": 4.5},
		Partial:    true,
		PnL:        10,
		Points:     2,
		Reason:     "partial",
		Side:       "long",
		Size:       5,
		SL:         100,
		Tag:        "DSL-FLAG",
		TP:         104,
	}
	source := RunResult{Case: "partial", Trades: []Trade{trade}}
	projected := conformanceResult(source)
	if !reflect.DeepEqual(source.Trades[0], trade) {
		t.Fatalf("conformance projection mutated source trade: got %#v, want %#v", source.Trades[0], trade)
	}
	if &source.Trades[0] == &projected.Trades[0] {
		t.Fatal("conformance projection reused the source trade slice")
	}
	want := trade
	want.Partial = false
	if !reflect.DeepEqual(projected.Trades[0], want) {
		t.Fatalf("projected trade = %#v, want only Partial cleared from %#v", projected.Trades[0], trade)
	}
}

func TestRunResultSerializesAbsentHigherTimeframeAsNull(t *testing.T) {
	raw, err := json.Marshal(RunResult{Trades: []Trade{}})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload["higherTimeframe"] != nil {
		t.Fatalf("higherTimeframe = %#v, want null", payload["higherTimeframe"])
	}
}

var implementedRunCases = []string{
	stage1RunCase,
	"family-range-break-fake",
	"family-opening-range-breakout",
	"family-inside-day-expansion",
	"family-day-open-reclaim",
	"family-daily-flush-failure",
	"deployed-dsl-session-expansion-ny",
	"deployed-dsl-trend-pullback-xauusd-four-hour-close-resume",
	"family-break-retest",
	"family-supply-demand",
	"family-double-top-bottom",
	"deployed-dsl-dual-ema-resumption-xauusd-four-hour",
	"family-fib-continuation",
	"family-channel-break-hold",
	"family-level-sweep",
	"family-triple-push-exhaustion",
	"family-vwap-extension-fade",
	"family-volume-anomaly-exhaustion",
	"family-elder-triple-screen",
	"family-elder-triple-screen-trail-override",
	"family-price-momentum",
	"family-price-momentum-guarded",
	"family-fair-value-gap",
	"family-weekend-extreme-fade",
	"family-intra-hour-run-exhaustion",
	"family-sma-golden-cross",
	"family-sma-golden-cross-protected",
	"family-keltner-reversion",
	"family-keltner-expansion",
	"money-risk-sizing",
	"money-partial-exit",
	"money-stop-distance-gate",
}

func runCaseImplemented(caseName string) bool {
	for _, implemented := range implementedRunCases {
		if caseName == implemented {
			return true
		}
	}
	return false
}

func runFixtureDir() string {
	return filepath.Join(testsupport.StratConformanceRoot(), "run")
}

func canonicalRawJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return canonicalJSON(value)
}

func canonicalJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		panic(err)
	}
	out, err := json.Marshal(decoded)
	if err != nil {
		panic(err)
	}
	return string(out)
}

func firstDiff(got string, want string) string {
	limit := len(got)
	if len(want) < limit {
		limit = len(want)
	}
	for i := 0; i < limit; i++ {
		if got[i] != want[i] {
			start := i - 80
			if start < 0 {
				start = 0
			}
			endGot := i + 160
			if endGot > len(got) {
				endGot = len(got)
			}
			endWant := i + 160
			if endWant > len(want) {
				endWant = len(want)
			}
			return fmt.Sprintf("byte %d got[%q] want[%q]", i, got[start:endGot], want[start:endWant])
		}
	}
	if len(got) != len(want) {
		return fmt.Sprintf("length got=%d want=%d", len(got), len(want))
	}
	return "none"
}
