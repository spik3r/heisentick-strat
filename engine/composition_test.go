package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
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

func TestInteractiveCompositionMatchesCostBearingJSOracle(t *testing.T) {
	root := os.Getenv("HEISENTICK_APP_SOURCE_ROOT")
	if root == "" {
		t.Skip("set HEISENTICK_APP_SOURCE_ROOT for pinned JS wrapper oracle")
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
	rawOracle, err := os.ReadFile(filepath.Join("testdata", "composition", "js-cost-oracle-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Schema         string `json:"schema"`
		SourceRevision string `json:"sourceRevision"`
		Cases          []struct {
			Fixture     string          `json:"fixture"`
			StrategyID  string          `json:"strategyId"`
			TradeCount  int             `json:"tradeCount"`
			Trades      json.RawMessage `json:"trades"`
			EquityCurve []float64       `json:"equityCurve"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(rawOracle, &oracle); err != nil {
		t.Fatal(err)
	}
	if oracle.Schema != "composition-js-cost-oracle-v1" || oracle.SourceRevision != manifest.SourceRevision || len(oracle.Cases) != 2 {
		t.Fatal("invalid cost oracle source/version/cases")
	}
	for _, tc := range oracle.Cases {
		t.Run(filepath.Base(tc.Fixture), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", tc.Fixture))
			if err != nil {
				t.Fatal(err)
			}
			fixture, err := DecodeRunFixture(raw)
			if err != nil {
				t.Fatal(err)
			}
			result, err := RunInteractiveCompositionFixture(raw, sources)
			if err != nil {
				t.Fatal(err)
			}
			wantChildren := map[string]bool{}
			for _, composite := range manifest.Composites {
				if composite.ID == tc.StrategyID {
					for _, route := range composite.Routes {
						for _, id := range route.Children {
							wantChildren[id] = true
						}
					}
				}
			}
			if result.Schema != InteractiveCompositionSchema || result.Run.TradeCount != tc.TradeCount || tc.TradeCount == 0 || len(result.Provenance.Children) != len(wantChildren) {
				t.Fatalf("incomplete interactive result: schema=%q trades=%d children=%d", result.Schema, result.Run.TradeCount, len(result.Provenance.Children))
			}
			if got, want := canonicalJSON(ConformanceProjection(result.Run).Trades), canonicalRawJSON(t, tc.Trades); got != want {
				t.Fatalf("cost-bearing wrapper trades differ: %s", firstDiff(got, want))
			}
			if len(result.EquityCurve) != len(tc.EquityCurve) || len(result.ClosedEquity) != len(tc.EquityCurve) || len(result.EquityCurve) != len(fixture.Bars) {
				t.Fatal("missing per-bar chart marks")
			}
			for i, want := range tc.EquityCurve {
				if math.Abs(result.EquityCurve[i]-want) > 1e-9 {
					t.Fatalf("equity[%d] = %.15g, JS = %.15g", i, result.EquityCurve[i], want)
				}
			}
			tradeNet := 0.0
			for i, net := range result.TradeNetPnL {
				if net >= result.Run.Trades[i].PnL {
					t.Fatalf("trade %d does not include entry fee", i)
				}
				tradeNet += net
			}
			if math.Abs(result.CashEndEquity-(fixture.Costs.StartEquity+tradeNet)) > 1e-8 ||
				math.Abs(result.Stats.Net-tradeNet) > 1e-8 || result.Stats.Trades != tc.TradeCount ||
				result.SkipDiagnostics != "measured" || result.SkipReasonSchema != InteractiveSkipReasonSchema || result.Skips == nil {
				t.Fatalf("fee-inclusive result is inconsistent: cash=%g net=%g tradeNet=%g", result.CashEndEquity, result.Stats.Net, tradeNet)
			}
		})
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
	manifest.Children = append(manifest.Children, compositionTestChild("other-route", source))
	manifest.Children[1].Params["unsupportedKnob"] = 1
	manifest.Composites[0].Routes = append(manifest.Composites[0].Routes, CompositionRoute{Symbol: "GBPUSD", Timeframe: "15m", Children: []string{"other-route"}})
	sources["other-route"] = source
	for _, symbol := range []string{"XAUUSD", "EURUSD"} {
		request.Symbol = symbol
		if _, err := RunComposition(manifest, request, sources); err == nil || !strings.Contains(err.Error(), "unsupported param") {
			t.Fatalf("unsupported dependency on another route returned success for %s: %v", symbol, err)
		}
	}
	delete(manifest.Children[1].Params, "unsupportedKnob")
	manifest.Children[1].Params["targetR"] = 9
	if _, err := RunComposition(manifest, request, sources); err == nil || !strings.Contains(err.Error(), "disagree with pinned DSL source") {
		t.Fatalf("mismatched child params on another route returned success: %v", err)
	}
	unsupported := "dsl v7\nsetup {\n  type: break retest\n  ema confluence within 0.25 ATR\n}"
	manifest.Children[1] = compositionTestChild("other-route", unsupported)
	sources["other-route"] = unsupported
	if _, err := RunComposition(manifest, request, sources); err == nil || !strings.Contains(err.Error(), "EMA confluence is not implemented") {
		t.Fatalf("unsupported child DSL on another route returned success: %v", err)
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
				raw, err := os.ReadFile(filepath.Join(runFixtureDir(), fixtureByTimeframe[route.Timeframe]+".fixture.json"))
				if err != nil {
					t.Fatal(err)
				}
				var input map[string]any
				if err := json.Unmarshal(raw, &input); err != nil {
					t.Fatal(err)
				}
				input["strategyId"], input["symbol"] = composite.ID, route.Symbol
				delete(input, "contextOptions") // The pinned child DSL owns its EMA context.
				raw, err = json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				interactive, err := RunInteractiveCompositionFixture(raw, sources)
				if err != nil {
					t.Fatal(err)
				}
				if interactive.Run.TradeCount != result.TradeCount || len(interactive.EquityCurve) != request.Series.Len() || len(interactive.ClosedEquity) != request.Series.Len() {
					t.Fatalf("interactive route lost broker data: trades=%d, equity=%d, closed=%d", interactive.Run.TradeCount, len(interactive.EquityCurve), len(interactive.ClosedEquity))
				}
				t.Logf("%s %s: %d Go trades", request.Symbol, request.Timeframe, result.TradeCount)
			})
		}
	}
}

// This oracle was generated by the pinned app's actual JS wrapper registry
// and runBacktest, including context construction and child onBar calls.
// App sources remain external, as required by repository ownership rules.
func TestBuiltInCompositionMatchesFrozenJSWrapperOracle(t *testing.T) {
	root := os.Getenv("HEISENTICK_APP_SOURCE_ROOT")
	if root == "" {
		t.Skip("set HEISENTICK_APP_SOURCE_ROOT for pinned JS wrapper oracle")
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
	raw, err := os.ReadFile(filepath.Join("testdata", "composition", "js-oracle-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		SourceRevision string `json:"sourceRevision"`
		Cases          []struct {
			StrategyID string          `json:"strategyId"`
			Fixture    string          `json:"fixture"`
			TradeCount int             `json:"tradeCount"`
			Trades     json.RawMessage `json:"trades"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	if oracle.SourceRevision != manifest.SourceRevision || len(oracle.Cases) != 5 {
		t.Fatal("oracle source revision or case set does not match composition manifest")
	}
	for _, tc := range oracle.Cases {
		t.Run(tc.StrategyID, func(t *testing.T) {
			fixture, err := LoadRunFixture(filepath.Join("..", tc.Fixture))
			if err != nil {
				t.Fatal(err)
			}
			request := RunRequest{
				Series: marketdata.SeriesFromBars(fixture.Bars), HTFSeries: marketdata.SeriesFromBars(fixture.HTFBars),
				StrategyID: tc.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
				HigherTimeframe: fixture.HigherTimeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs,
			}
			result, err := RunBuiltInComposition(request, sources)
			if err != nil {
				t.Fatal(err)
			}
			if result.TradeCount != tc.TradeCount || result.TradeCount == 0 {
				t.Fatalf("Go trades = %d, JS oracle = %d", result.TradeCount, tc.TradeCount)
			}
			if got, want := canonicalJSON(ConformanceProjection(result).Trades), canonicalRawJSON(t, tc.Trades); got != want {
				t.Fatalf("whole-wrapper JS oracle mismatch: %s", firstDiff(got, want))
			}
		})
	}
}
