package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func compositionTestRequest(t *testing.T, fixtureName, strategyID string) RunRequest {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), fixtureName+".fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	return RunRequest{
		Series: marketdata.SeriesFromBars(fixture.Bars), HTFSeries: marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID: strategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs,
	}
}

func compositionTestChild(id, source string) CompositionChild {
	digest := sha256.Sum256([]byte(source))
	return CompositionChild{ID: id, Path: "test fixture", SHA256: hex.EncodeToString(digest[:]), Params: map[string]float64{}}
}

func TestBuiltInCompositionManifestHasExactFiveRoutesAndDependencies(t *testing.T) {
	manifest, err := BuiltInCompositionManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != CompositionSchema || len(manifest.Children) != 10 || len(manifest.Composites) != 5 {
		t.Fatalf("unexpected manifest shape: schema=%q children=%d composites=%d", manifest.Schema, len(manifest.Children), len(manifest.Composites))
	}
	want := map[string]map[string][]string{
		"dslSessionBiasSeasonalConvergenceBalancedFifteen": {"XAUUSD 15m": {"dslSessionBiasSeasonalConvergenceQualityFifteen", "dslSessionBiasSeasonalConvergenceLlShortFifteen"}},
		"dslLondonBreakRetestPlaybook":                     {"XAUUSD 1h": {"dslLondonBreakRetestQuality"}, "XAUUSD 4h": {"dslLondonBreakRetestXauusd4h"}, "USDJPY 1h": {"dslLondonBreakRetestUsdjpy1h"}, "GBPUSD 4h": {"dslLondonBreakRetestGbpusd4h"}},
		"continuationReviewPlaybook":                       {"XAUUSD 15m": {"dslSessionBreakHoldNyFocusBalancedFifteen"}, "XAUUSD 1h": {"dslBreakRetestAsiaLondonOneFourHourReview"}, "XAUUSD 4h": {"dslBreakRetestAsiaLondonOneFourHourReview"}},
		"continuationReviewPlaybookHighSample":             {"XAUUSD 15m": {"dslSessionBreakHoldNyFocusBalancedFifteen"}, "XAUUSD 1h": {"dslBreakRetestAsiaLondonOneFourHourReview"}, "XAUUSD 4h": {"dslBreakRetestOneFourHourReview"}},
		"continuationReviewPlaybookTrendPullback":          {"XAUUSD 15m": {"dslSessionBreakHoldNyFocusBalancedFifteen"}, "XAUUSD 1h": {"dslBreakRetestAsiaLondonOneFourHourReview"}, "XAUUSD 4h": {"dslTrendPullbackXauusdFourHourCloseResume"}},
	}
	got := make(map[string]map[string][]string)
	for _, composite := range manifest.Composites {
		got[composite.ID] = make(map[string][]string)
		for _, route := range composite.Routes {
			got[composite.ID][route.Symbol+" "+route.Timeframe] = route.Children
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("composition routes differ\ngot: %#v\nwant: %#v", got, want)
	}
	manifest.Composites[0].Routes[0].Children[0] = "mutated"
	again, err := BuiltInCompositionManifest()
	if err != nil || again.Composites[0].Routes[0].Children[0] == "mutated" {
		t.Fatal("built-in manifest can be mutated by a caller")
	}
}

func TestCompositionExecutesFullFixtureAndFailsClosed(t *testing.T) {
	sourceRaw, err := os.ReadFile(filepath.Join(runFixtureDir(), "research-dsl-session-break-hold-fifteen.strat"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceRaw)
	request := compositionTestRequest(t, "research-dsl-session-break-hold-fifteen", "testComposition")
	manifest := CompositionManifest{Schema: CompositionSchema,
		Children:   []CompositionChild{compositionTestChild("session", source)},
		Composites: []Composite{{ID: request.StrategyID, Routes: []CompositionRoute{{Symbol: request.Symbol, Timeframe: request.Timeframe, Children: []string{"session"}}}}},
	}
	sources := map[string]string{"session": source}
	got, err := RunComposition(manifest, request, sources)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "research-dsl-session-break-hold-fifteen.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatal(err)
	}
	if got.TradeCount != 6 || canonicalJSON(got.Trades) != canonicalJSON(want.Trades) {
		t.Fatalf("whole-strategy run differs from child fixture: got %d trades, want %d", got.TradeCount, want.TradeCount)
	}
	if got.StrategyID != request.StrategyID {
		t.Fatalf("lost composite identity: %q", got.StrategyID)
	}
	request.Symbol = "EURUSD"
	offRoute, err := RunComposition(manifest, request, sources)
	if err != nil || offRoute.TradeCount != 0 || offRoute.Trades == nil {
		t.Fatalf("off route: result=%+v error=%v", offRoute, err)
	}
	if _, err := RunComposition(manifest, request, nil); err == nil || !strings.Contains(err.Error(), "missing source") {
		t.Fatalf("missing child source did not fail closed: %v", err)
	}
	if _, err := RunComposition(manifest, request, map[string]string{"session": source + "\n"}); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("stale child source did not fail closed: %v", err)
	}
}

func TestOrderedCompositionSharesPositionAndKeepsBranchState(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(runFixtureDir(), "research-dsl-session-break-hold-fifteen.strat"))
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Replace(string(raw), "priority(AH, AL, LH, LL)", "priority(AH, LH)", 1)
	long = strings.Replace(long, "  higher timeframe must agree", "  higher timeframe must agree\n  side long only", 1)
	short := strings.Replace(string(raw), "priority(AH, AL, LH, LL)", "priority(AL, LL)", 1)
	short = strings.Replace(short, "  higher timeframe must agree", "  higher timeframe must agree\n  side short only", 1)
	request := compositionTestRequest(t, "research-dsl-session-break-hold-fifteen", "orderedSession")
	manifest := CompositionManifest{Schema: CompositionSchema,
		Children:   []CompositionChild{compositionTestChild("long", long), compositionTestChild("short", short)},
		Composites: []Composite{{ID: request.StrategyID, Routes: []CompositionRoute{{Symbol: request.Symbol, Timeframe: request.Timeframe, Children: []string{"long", "short"}}}}},
	}
	result, err := RunComposition(manifest, request, map[string]string{"long": long, "short": short})
	if err != nil {
		t.Fatal(err)
	}
	var sides []string
	for _, trade := range result.Trades {
		sides = append(sides, trade.Side)
	}
	if result.TradeCount != 6 || !reflect.DeepEqual(sides, []string{"short", "long", "long", "long", "long", "long"}) {
		t.Fatalf("ordered branches produced %d trades with sides %v", result.TradeCount, sides)
	}
	for i := 1; i < len(result.Trades); i++ {
		if result.Trades[i].EntryIndex <= result.Trades[i-1].ExitIndex {
			t.Fatalf("shared broker allowed overlapping positions: trades %d and %d", i-1, i)
		}
	}
}

// The app owns .strat sources. Set this to its root for a source-identity and
// end-to-end run against every real child without copying strategies here.
func TestBuiltInCompositionWithAppSources(t *testing.T) {
	root := os.Getenv("HEISENTICK_APP_SOURCE_ROOT")
	if root == "" {
		t.Skip("set HEISENTICK_APP_SOURCE_ROOT for pinned app-source integration")
	}
	manifest, err := BuiltInCompositionManifest()
	if err != nil {
		t.Fatal(err)
	}
	sources := make(map[string]string)
	for _, child := range manifest.Children {
		raw, err := os.ReadFile(filepath.Join(root, child.Path))
		if err != nil {
			t.Fatal(err)
		}
		sources[child.ID] = string(raw)
	}
	fixtureByTimeframe := map[string]string{
		"15m": "research-dsl-session-break-hold-fifteen",
		"1h":  "deployed-dsl-break-retest-asia-london-one-four-hour-review-xauusd-one-hour",
		"4h":  "deployed-dsl-trend-pullback-xauusd-four-hour-close-resume",
	}
	wantPositive := map[string]int{
		"continuationReviewPlaybook XAUUSD 1h":              16,
		"continuationReviewPlaybookHighSample XAUUSD 4h":    8,
		"continuationReviewPlaybookTrendPullback XAUUSD 4h": 21,
	}
	for _, composite := range manifest.Composites {
		for _, route := range composite.Routes {
			t.Run(composite.ID+"/"+route.Symbol+"/"+route.Timeframe, func(t *testing.T) {
				request := compositionTestRequest(t, fixtureByTimeframe[route.Timeframe], composite.ID)
				request.Symbol = route.Symbol
				result, err := RunBuiltInComposition(request, sources)
				if err != nil {
					t.Fatal(err)
				}
				if want, ok := wantPositive[composite.ID+" "+route.Symbol+" "+route.Timeframe]; ok && result.TradeCount != want {
					t.Fatalf("whole-strategy fixture trades = %d, want %d", result.TradeCount, want)
				}
				t.Logf("%s %s: %d Go trades", request.Symbol, request.Timeframe, result.TradeCount)
			})
		}
	}
}
