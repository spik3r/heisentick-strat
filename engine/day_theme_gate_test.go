package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestRunFixtureCaseHonorsTypedDayTheme(t *testing.T) {
	const caseName = "family-break-retest"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	baseline, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run baseline fixture: %v", err)
	}
	if baseline.TradeCount != 13 || len(baseline.Trades) != 13 {
		t.Fatalf("baseline trades = %d/%d, want reviewed fixture count 13", baseline.TradeCount, len(baseline.Trades))
	}

	mutated := strings.Replace(string(source), "  day type", "  day theme in (stand_aside)\n  day type", 1)
	if mutated == string(source) {
		t.Fatal("fixture source did not contain the expected day-type directive")
	}
	parsed, err := dsl.Parse(mutated)
	if err != nil {
		t.Fatalf("parse mutated fixture: %v", err)
	}
	if got := stringSliceValue(parsed.Config["dayThemes"]); len(got) != 1 || got[0] != "stand_aside" {
		t.Fatalf("parsed day themes = %v, want [stand_aside]", got)
	}
	result, err := RunFixtureCase(fixture, mutated)
	if err != nil {
		t.Fatalf("run fixture with stand-aside theme: %v", err)
	}
	if result.TradeCount != 0 || len(result.Trades) != 0 {
		t.Fatalf("stand-aside theme produced %d trades: %+v; want none", result.TradeCount, result.Trades)
	}

	runner := newPreparedRunner(fixture, parsed.Config)
	if trades := runner.RunPrepared(); len(trades) != 0 {
		t.Fatalf("theme-rejected prepared run produced %d trades, want 0", len(trades))
	}
	if !runner.broker.hasBREntry || runner.broker.brLastEntry <= 0 {
		t.Fatalf("final theme rejection did not preserve typed candidate consumption: cooldown=%v last=%d",
			runner.broker.hasBREntry, runner.broker.brLastEntry)
	}
}

func TestDayThemeInferencePriorityAndSideBoundaries(t *testing.T) {
	newBroker := func(close float64, regime, trend, location int8, priorHigh, priorLow float64) broker {
		return broker{
			series: marketdata.SeriesFromBars([]marketdata.Bar{{C: close}}),
			cols: contextcols.Columns{
				Regime:       []int8{regime},
				TrendDir:     []int8{trend},
				OpenLocation: []int8{location},
				PriorDayH:    []float64{priorHigh},
				PriorDayL:    []float64{priorLow},
			},
		}
	}

	for _, tt := range []struct {
		name                    string
		close                   float64
		regime, trend, location int8
		priorHigh, priorLow     float64
		want                    string
	}{
		{name: "trending wins over high location", close: 50, regime: regimeTrending, trend: trendUp, location: 1, priorHigh: 100, priorLow: 0, want: "join_momentum"},
		{name: "near prior high", close: 50, location: 1, priorHigh: 100, priorLow: 0, want: "sell_highs"},
		{name: "near Asia low", close: 50, location: 8, priorHigh: 100, priorLow: 0, want: "buy_lows"},
		{name: "exact lower boundary", close: 38, priorHigh: 100, priorLow: 0, want: "buy_lows"},
		{name: "exact upper boundary", close: 62, priorHigh: 100, priorLow: 0, want: "sell_highs"},
		{name: "middle", close: 50, priorHigh: 100, priorLow: 0, want: "stand_aside"},
		{name: "invalid prior range", close: 50, priorHigh: math.NaN(), priorLow: 0, want: "stand_aside"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newBroker(tt.close, tt.regime, tt.trend, tt.location, tt.priorHigh, tt.priorLow)
			if got := b.inferDayTheme(0); got != tt.want {
				t.Fatalf("inferDayTheme = %q, want %q", got, tt.want)
			}
		})
	}
	for _, tt := range []struct {
		name      string
		locations []int8
		want      string
	}{
		{name: "all high aliases", locations: []int8{1, 7, 9, 11}, want: "sell_highs"},
		{name: "all low aliases", locations: []int8{2, 8, 10, 12}, want: "buy_lows"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, location := range tt.locations {
				b := newBroker(50, 0, 0, location, 100, 0)
				if got := b.inferDayTheme(0); got != tt.want {
					t.Fatalf("location code %d theme = %q, want %q", location, got, tt.want)
				}
			}
		})
	}

	for _, tt := range []struct {
		name      string
		theme     string
		regime    int8
		trend     int8
		location  int8
		side      side
		wantAllow bool
	}{
		{name: "join up allows long", theme: "join_momentum", regime: regimeTrending, trend: trendUp, side: sideLong, wantAllow: true},
		{name: "join up rejects short", theme: "join_momentum", regime: regimeTrending, trend: trendUp, side: sideShort},
		{name: "join down allows short", theme: "join_momentum", regime: regimeTrending, trend: trendDown, side: sideShort, wantAllow: true},
		{name: "join down rejects long", theme: "join_momentum", regime: regimeTrending, trend: trendDown, side: sideLong},
		{name: "buy lows allows long", theme: "buy_lows", location: 2, side: sideLong, wantAllow: true},
		{name: "buy lows rejects short", theme: "buy_lows", location: 2, side: sideShort},
		{name: "sell highs allows short", theme: "sell_highs", location: 1, side: sideShort, wantAllow: true},
		{name: "sell highs rejects long", theme: "sell_highs", location: 1, side: sideLong},
		{name: "stand aside rejects long", theme: "stand_aside", side: sideLong},
		{name: "stand aside rejects short", theme: "stand_aside", side: sideShort},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := newBroker(50, tt.regime, tt.trend, tt.location, 100, 0)
			b.params.DayThemes = []string{tt.theme}
			gotTheme, gotAllow := b.dayThemeAdmission(0, tt.side)
			if gotTheme != tt.theme || gotAllow != tt.wantAllow {
				t.Fatalf("dayThemeAdmission = (%q, %v), want (%q, %v)", gotTheme, gotAllow, tt.theme, tt.wantAllow)
			}
		})
	}
}

func TestFlagDayThemeRequestsAndBuildsNamedLocationContext(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "deployed-dsl-flag-continuation-one-four-hour-review")
	withoutTheme := contextOptions(fixture, cfg)
	if !withoutTheme.Selective || withoutTheme.NeedOpenLocation {
		t.Fatalf("no-theme flag options = selective %v openLocation %v, want true/false",
			withoutTheme.Selective, withoutTheme.NeedOpenLocation)
	}
	withoutThemeRunner := newPreparedRunner(fixture, cfg)
	if len(withoutThemeRunner.cols.OpenLocation) != 0 {
		t.Fatalf("no-theme selective flag built %d open-location rows, want none", len(withoutThemeRunner.cols.OpenLocation))
	}

	cfg["dayThemes"] = []any{"buy_lows", "sell_highs"}
	withTheme := contextOptions(fixture, cfg)
	if !withTheme.Selective || !withTheme.NeedOpenLocation || withTheme.NeedReportTradeContext {
		t.Fatalf("themed flag options = selective %v openLocation %v reportContext %v, want true/true/false",
			withTheme.Selective, withTheme.NeedOpenLocation, withTheme.NeedReportTradeContext)
	}
	runner := newPreparedRunner(fixture, cfg)
	if len(runner.cols.OpenLocation) != runner.series.Len() {
		t.Fatalf("themed selective flag open-location rows = %d, want %d", len(runner.cols.OpenLocation), runner.series.Len())
	}
	shared, err := PrepareSharedRunContext(RunRequest{
		Config:          cfg,
		Series:          runner.series,
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	})
	if err != nil {
		t.Fatalf("prepare themed shared context: %v", err)
	}
	if !shared.options.NeedOpenLocation || len(shared.cols.OpenLocation) != runner.series.Len() {
		t.Fatalf("shared themed context = option %v rows %d, want true/%d",
			shared.options.NeedOpenLocation, len(shared.cols.OpenLocation), runner.series.Len())
	}
	if _, err := shared.PrepareVariant(cfg); err != nil {
		t.Fatalf("prepare themed shared variant: %v", err)
	}

	// This committed fixture row is near the Asia high (code 7), while its
	// close is in the bottom 38% of the prior-day range. JS gives the named
	// location priority, so it must infer sell_highs rather than buy_lows.
	const index = 40
	if got := runner.cols.OpenLocation[index]; got != 7 {
		t.Fatalf("fixture open-location code at %d = %d, want nearAH code 7", index, got)
	}
	hi := runner.cols.PriorDayH[index]
	lo := runner.cols.PriorDayL[index]
	position := (runner.series.C[index] - lo) / (hi - lo)
	if !isFinite(hi) || !isFinite(lo) || hi <= lo || position > 0.38 {
		t.Fatalf("fixture prior-range fallback at %d = hi %.6f lo %.6f position %.6f, want valid bottom 38%%",
			index, hi, lo, position)
	}
	b := broker{series: runner.series, cols: runner.cols, params: runner.params}
	if got := b.inferDayTheme(index); got != "sell_highs" {
		t.Fatalf("fixture inferred theme at %d = %q, want named-location sell_highs", index, got)
	}
}

func TestDayThemeFinalAdmissionPathsAnnotateMetadata(t *testing.T) {
	newBroker := func() broker {
		return broker{
			series: marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 7, H: 9, L: 6, C: 8}}),
			cols: contextcols.Columns{
				ATR:          []float64{1},
				ER:           []float64{0},
				Regime:       []int8{regimeTrending},
				TrendDir:     []int8{trendUp},
				SessionPhase: []int8{0},
				PriorDayType: []int8{0},
			},
			params: flagParams{
				UseAsiaWindow:   true,
				AdmitAsiaWindow: true,
				MaxMovementER:   1,
				DayThemes:       []string{"join_momentum"},
				RiskUSD:         1,
			},
			costs: Costs{StartEquity: 10000},
		}
	}

	t.Run("typed immediate", func(t *testing.T) {
		b := newBroker()
		meta := TradeMeta{"source": "caller"}
		b.enterSetup(0, setupPlan{Side: sideLong, Stop: 6, Target: 10, Meta: meta})
		if !b.hasPosition || b.position.Meta["dayTheme"] != "join_momentum" {
			t.Fatalf("typed position = active %v meta %+v", b.hasPosition, b.position.Meta)
		}
		if _, exists := meta["dayTheme"]; exists {
			t.Fatalf("day-theme annotation mutated caller metadata: %+v", meta)
		}
	})

	t.Run("flag immediate and side rejection", func(t *testing.T) {
		b := newBroker()
		b.enter(0, sideLong, flagSetup{Stop: 6, Target: 10, Meta: TradeMeta{}})
		if !b.hasPosition || b.position.Meta["dayTheme"] != "join_momentum" {
			t.Fatalf("flag position = active %v meta %+v", b.hasPosition, b.position.Meta)
		}
		b = newBroker()
		b.enter(0, sideShort, flagSetup{Stop: 10, Target: 6, Meta: TradeMeta{}})
		if b.hasPosition {
			t.Fatal("theme-rejected flag side entered")
		}
	})

	t.Run("limit", func(t *testing.T) {
		b := newBroker()
		b.enterLimit(0, 7, setupPlan{Side: sideLong, Stop: 6, Target: 10, Meta: TradeMeta{}}, 5)
		if len(b.limitOrders) != 1 || b.limitOrders[0].Meta["dayTheme"] != "join_momentum" {
			t.Fatalf("limit orders = %+v", b.limitOrders)
		}
		b = newBroker()
		b.enterLimit(0, 7, setupPlan{Side: sideShort, Stop: 10, Target: 6, Meta: TradeMeta{}}, 5)
		if len(b.limitOrders) != 0 {
			t.Fatalf("theme-rejected limit orders = %d, want 0", len(b.limitOrders))
		}
	})

	t.Run("absent list is no-op", func(t *testing.T) {
		b := newBroker()
		b.params.DayThemes = nil
		b.cols.Regime = nil
		b.cols.TrendDir = nil
		b.enterSetup(0, setupPlan{Side: sideLong, Stop: 6, Target: 10, Meta: TradeMeta{}})
		if !b.hasPosition {
			t.Fatal("absent day-theme list blocked entry")
		}
		if _, exists := b.position.Meta["dayTheme"]; exists {
			t.Fatalf("absent day-theme list annotated metadata: %+v", b.position.Meta)
		}
	})
}

func TestFailedBreakoutDayThemeRejectsBeforeSeenState(t *testing.T) {
	fixture, source := loadLevelSweepLimitCase(t)
	parsed := parseEngineTestDSL(t, source)
	baseline := newPreparedRunner(fixture, parsed.Config)
	if trades := baseline.RunPrepared(); len(trades) != 1 || !baseline.broker.hasLSEntry {
		t.Fatalf("baseline failed breakout = trades %d cooldown %v, want 1/true", len(trades), baseline.broker.hasLSEntry)
	}
	parsed.Config["dayThemes"] = []any{"stand_aside"}
	runner := newPreparedRunner(fixture, parsed.Config)
	if trades := runner.RunPrepared(); len(trades) != 0 {
		t.Fatalf("theme-rejected failed breakout produced %d trades, want 0", len(trades))
	}
	if runner.broker.hasLSEntry || runner.broker.lsLastEntry != 0 || len(runner.broker.seen.ls.keys) != 0 {
		t.Fatalf("theme rejection mutated generic state: cooldown=%v last=%d seen=%v",
			runner.broker.hasLSEntry, runner.broker.lsLastEntry, runner.broker.seen.ls.keys)
	}
}
