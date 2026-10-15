package engine

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func sarTestBroker() broker {
	return broker{
		series: marketdata.Series{T: []float64{0, 60000, 120000}, O: []float64{10, 12, 9}, H: []float64{11, 13, 10}, L: []float64{9, 11, 8}, C: []float64{10, 12, 9}, V: []float64{1, 1, 1}},
		cols:   contextcols.Columns{ER: []float64{0, 0, 0}}, params: flagParams{RiskUSD: 200, TradeWindowUnrestricted: true}, costs: Costs{FillOn: "close", StartEquity: 10000},
		parabolicSAR: &parabolicSARRuntime{spec: dsl.ParabolicSARSpec{Predicate: "bullish"}, columns: contextcols.ParabolicSARColumns{SAR: []float64{math.NaN(), 10, 12}, Trend: []int8{0, 1, -1}, Flip: []int8{0, 0, -1}}},
	}
}

func TestParabolicSARGatePredicates(t *testing.T) {
	for _, tc := range []struct {
		predicate string
		i         int
		side      side
		want      bool
	}{
		{"bullish", 1, sideShort, true}, {"bullish", 2, sideLong, false},
		{"bearish", 2, sideLong, true}, {"bearish", 1, sideShort, false},
		{"closeAbove", 1, sideLong, true}, {"closeAbove", 2, sideLong, false},
		{"closeBelow", 2, sideLong, true}, {"closeBelow", 1, sideLong, false},
		{"flipBullish", 1, sideLong, false}, {"flipBearish", 2, sideLong, true},
		{"supportsEntry", 1, sideLong, true}, {"supportsEntry", 1, sideShort, false},
		{"supportsEntry", 2, sideShort, true}, {"supportsEntry", 2, sideLong, false},
		{"supportsEntry", 1, side(0), false}, {"unknown", 1, sideLong, false},
	} {
		b := sarTestBroker()
		b.parabolicSAR.spec.Predicate = tc.predicate
		if got := b.parabolicSARGateOK(tc.i, tc.side); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
		for _, i := range []int{-1, 0, 3} {
			if b.parabolicSARGateOK(i, tc.side) {
				t.Fatalf("unavailable index %d passed", i)
			}
		}
	}
	for _, predicate := range []string{"closeAbove", "closeBelow"} {
		b := sarTestBroker()
		b.parabolicSAR.spec.Predicate = predicate
		b.series.C[1] = 10
		if b.parabolicSARGateOK(1, sideLong) {
			t.Fatal("equality passed", predicate)
		}
	}
	b := sarTestBroker()
	b.parabolicSAR.columns.Flip[1] = 1
	b.parabolicSAR.spec.Predicate = "flipBullish"
	if !b.parabolicSARGateOK(1, sideLong) {
		t.Fatal("bullish flip rejected")
	}
	b.parabolicSAR.spec.Predicate = ""
	if !b.parabolicSARGateOK(-1, sideLong) {
		t.Fatal("parameter-only series changed admission")
	}
}

func TestParabolicSARAllAdmissionPathsAndFillClock(t *testing.T) {
	for _, path := range []string{"flag", "setup", "limit"} {
		for _, capture := range []bool{false, true} {
			for _, fill := range []string{"close", "nextOpen"} {
				for _, allow := range []bool{false, true} {
					b := sarTestBroker()
					b.captureEntries = capture
					b.costs.FillOn = fill
					if !allow {
						b.parabolicSAR.spec.Predicate = "bearish"
					}
					setup := setupPlan{Side: sideLong, Stop: 7, Target: 20}
					switch path {
					case "flag":
						b.enter(1, sideLong, flagSetup{Stop: 7, Target: 20})
					case "setup":
						b.enterSetup(1, setup)
					case "limit":
						b.enterLimit(1, 11, setup, 3)
					}
					admitted := b.hasPosition || len(b.pendingOrders) > 0 || len(b.limitOrders) > 0 || len(b.capturedEntries) > 0
					if admitted != allow {
						t.Fatalf("%s capture%v fill%s allow%v admitted%v", path, capture, fill, allow, admitted)
					}
					if !allow {
						continue
					}
					// The next bar has reversed bearish. Accepted signal-bar orders must
					// still dispatch/fill without consulting that future closed-bar SAR.
					if capture {
						b.captureEntries = false
						b.dispatchCaptured(1, b.capturedEntries[0])
					}
					b.fillPending(2)
					b.fillLimits(2)
					if !b.hasPosition {
						t.Fatalf("%s accepted signal was re-gated on fill", path)
					}
				}
			}
		}
	}
}

func sarNamedLevel(t *testing.T) (RunFixture, string) {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "family-named-level-sweep.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "family-named-level-sweep.strat"))
	if err != nil {
		t.Fatal(err)
	}
	return fixture, string(source)
}
func sarParsed(t *testing.T, source string) dsl.Config {
	t.Helper()
	p, e := dsl.Parse(source)
	if e != nil || len(p.Errors) > 0 {
		t.Fatal(e, p.Errors)
	}
	return p.Config
}

func TestParabolicSARFixtureHandDerivedGate(t *testing.T) {
	fixture, source := sarNamedLevel(t)
	baseline, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.TradeCount != 1 || baseline.Trades[0].EntryIndex != 10 {
		t.Fatal("baseline witness changed")
	}
	// The index5 low95 seeds a rising segment. Index10 low94 reverses it
	// to falling SAR101; index10 close101 is strict equality.
	for _, tc := range []struct {
		directive string
		count     int
	}{
		{"sar step 0.01", 1}, {"sar trend bearish", 1}, {"sar trend bullish", 0},
		{"sar close above", 0}, {"sar close below", 0}, {"sar reverses bearish", 1},
		{"sar reverses bullish", 0}, {"sar supports entry", 0},
	} {
		got, err := RunFixtureCase(fixture, source+"\nfilters { "+tc.directive+" }\n")
		if err != nil {
			t.Fatal(err)
		}
		if got.TradeCount != tc.count {
			t.Fatalf("%s: %d", tc.directive, got.TradeCount)
		}
		if tc.count == 1 && !reflect.DeepEqual(got.Trades, baseline.Trades) {
			t.Fatal("filter changed admitted trade", tc.directive)
		}
	}
}

func TestParabolicSARSharedContextIdentity(t *testing.T) {
	fixture, source := sarNamedLevel(t)
	cfg := sarParsed(t, source+"\nfilters { sar trend bearish }\n")
	req := RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
	key, err := SharedContextKey(req)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := PrepareSharedRunContext(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, directive := range []string{"sar start 0.01", "sar step 0.01", "sar max 0.3"} {
		variant := sarParsed(t, source+"\nfilters { "+directive+" }\n")
		req.Config = variant
		different, err := SharedContextKey(req)
		if err != nil {
			t.Fatal(err)
		}
		if key == different {
			t.Fatal("parameter collision", directive)
		}
		if _, err := shared.PrepareVariant(variant); err == nil {
			t.Fatal("reused incompatible SAR context", directive)
		}
	}
	variant := sarParsed(t, source+"\nfilters { sar trend bullish }\n")
	req.Config = variant
	same, err := SharedContextKey(req)
	if err != nil || same != key {
		t.Fatal("predicate unnecessarily changes numeric context", err)
	}
	if _, err := shared.PrepareVariant(variant); err != nil {
		t.Fatal(err)
	}
}

func TestParabolicSARPublicRunsRejectInvalidInput(t *testing.T) {
	fixture, source := sarNamedLevel(t)
	source += "\nfilters { sar trend bearish }\n"
	cfg := sarParsed(t, source)
	req := RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe}
	for _, mutate := range []func(*RunRequest){
		func(r *RunRequest) { r.Series.C[2] = math.NaN() },
		func(r *RunRequest) { r.Series.O[2] = r.Series.H[2] + 1 },
		func(r *RunRequest) { r.Series.T[2] = r.Series.T[1] },
		func(r *RunRequest) { r.Series.T[2] = r.Series.T[1] - 1 },
		func(r *RunRequest) { r.Series.H = r.Series.H[:2] },
	} {
		bad := req
		bad.Series = marketdata.SeriesFromBars(fixture.Bars)
		mutate(&bad)
		if _, err := Run(bad); err == nil {
			t.Fatal("Run accepted invalid SAR input")
		}
		if _, err := PrepareRun(bad); err == nil {
			t.Fatal("PrepareRun accepted invalid SAR input")
		}
		if _, err := PrepareSharedRunContext(bad); err == nil {
			t.Fatal("shared accepted invalid SAR input")
		}
		if _, err := SharedContextKey(bad); err == nil {
			t.Fatal("key accepted invalid SAR input")
		}
	}
	for _, family := range []string{"timedReturn", "legacySetup9", "clockRangeBreakout", "dailyFlushFailure", "downShockRebound", "weekendExtremeFade"} {
		bad := req
		bad.Config = dsl.Config{"setupType": family, "parabolicSAR": map[string]any{}}
		if _, err := Run(bad); err == nil || !strings.Contains(err.Error(), "SAR") {
			t.Fatalf("Run silently ignored %s: %v", family, err)
		}
		if _, err := PrepareRun(bad); err == nil || !strings.Contains(err.Error(), "SAR") {
			t.Fatalf("PrepareRun silently ignored %s: %v", family, err)
		}
	}
	fixture.RawBars = append(fixture.RawBars, []float64{1, 2, 3})
	if _, err := RunFixtureCase(fixture, source); err == nil {
		t.Fatal("fixture accepted a dropped malformed row")
	}
}

func TestParabolicSARRouteLabelsAndTypedFamily(t *testing.T) {
	fixture, source := sarNamedLevel(t)
	cfg := sarParsed(t, source+"\nfilters { sar trend bearish }\n")
	cfg["setupType"] = dsl.FamilyNamedLevelSweep
	req := RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, Costs: fixture.Costs}
	for _, label := range []string{"", fixture.Timeframe} {
		req.SourceTimeframe = label
		if _, err := Run(req); err != nil {
			t.Fatal("typed family/chart label rejected", err)
		}
	}
	req.SourceTimeframe = "1h"
	if _, err := Run(req); err == nil {
		t.Fatal("misleading source label accepted")
	}
	fixture.SourceTimeframe = "1h"
	if _, err := RunFixtureCase(fixture, source+"\nfilters { sar trend bearish }\n"); err == nil {
		t.Fatal("fixture source label accepted")
	}
}

func TestParabolicSARIndependentQueueFamilies(t *testing.T) {
	for _, name := range []string{"family-sma-golden-cross", "deployed-dsl-dual-ema-resumption-xauusd-four-hour"} {
		fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), name+".fixture.json"))
		if err != nil {
			t.Fatal(err)
		}
		source, err := os.ReadFile(filepath.Join(runFixtureDir(), name+".strat"))
		if err != nil {
			t.Fatal(err)
		}
		cfg := sarParsed(t, string(source))
		runner := newPreparedRunner(fixture, cfg)
		baseline := append([]Trade(nil), runner.RunPrepared()...)
		if len(baseline) == 0 {
			t.Fatal("missing queue witness", name)
		}
		cols := contextcols.ParabolicSARColumns{SAR: make([]float64, len(fixture.Bars)), Trend: make([]int8, len(fixture.Bars)), Flip: make([]int8, len(fixture.Bars))}
		for i := range cols.Trend {
			cols.Trend[i] = 1
		}
		for _, predicate := range []string{"bullish", "bearish"} {
			runner.parabolicSAR = &parabolicSARRuntime{spec: dsl.ParabolicSARSpec{Predicate: predicate}, columns: cols}
			got := runner.RunPrepared()
			if predicate == "bullish" && !reflect.DeepEqual(got, baseline) {
				t.Fatal("passing queue filter changed trades", name)
			}
			if predicate == "bearish" && len(got) != 0 {
				t.Fatal("direct queue bypassed SAR", name)
			}
		}
		text := strings.Replace(string(source), "dsl v6", "dsl v7", 1) + "\nfilters { sar step 0.01 }\n"
		parsed := sarParsed(t, text)
		if _, _, err := dsl.DecodeParabolicSAR(parsed); err != nil {
			t.Fatal(err)
		}
		got, err := RunFixtureCase(fixture, text)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Trades) != len(baseline) {
			t.Fatal("parameter-only changed dedicated queue family", name)
		}
	}
}

func TestParabolicSARPreparedResetClearsAndReattaches(t *testing.T) {
	fixture, source := sarNamedLevel(t)
	cfg := sarParsed(t, source+"\nfilters { sar trend bearish }\n")
	p, err := PrepareRun(RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), Symbol: fixture.Symbol, Timeframe: fixture.Timeframe})
	if err != nil {
		t.Fatal(err)
	}
	p.resetBroker(nil)
	if p.broker.parabolicSAR == nil || p.broker.parabolicSAR != p.parabolicSAR {
		t.Fatal("prepared SAR not attached")
	}
	p.broker.reset(p.series, p.cols, p.htfTrend, p.ema, p.emaSlope, p.params, p.fixture, nil)
	if p.broker.parabolicSAR != nil {
		t.Fatal("bare reset leaked previous SAR")
	}
	p.resetBroker(nil)
	if p.broker.parabolicSAR != p.parabolicSAR {
		t.Fatal("repeat reset lost SAR")
	}
}

func TestParabolicSARPrefixPaperAndResumption(t *testing.T) {
	paper := paperORBRequest(t, "close")
	paper.Source = strings.Replace(paper.Source, "dsl v6", "dsl v7", 1) + "\nfilters { sar trend bearish }\n"
	bars := make([]marketdata.Bar, len(paper.Bars))
	for i, r := range paper.Bars {
		bars[i] = marketdata.Bar{T: r[0], O: r[1], H: r[2], L: r[3], C: r[4], V: r[5]}
	}
	cfg := sarParsed(t, paper.Source)
	req := RunRequest{Config: cfg, Series: marketdata.SeriesFromBars(bars), Symbol: "XAUUSD", Timeframe: "5m", Costs: Costs{FillOn: "close", StartEquity: 5000}}
	// The ORB signal at index3 is bullish SAR; the bearish predicate blocks it.
	prefix, err := RunPrefix(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(prefix.Trades) != 0 || len(prefix.OpenPositions) != 0 {
		t.Fatal("prefix bypassed SAR")
	}
	var checkpoint []byte
	for n := 1; n <= len(bars); n++ {
		part := req
		part.Series = marketdata.SeriesFromBars(bars[:n])
		result, next, err := RunPrefixResumable(part, checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Trades) != 0 || len(result.OpenPositions) != 0 {
			t.Fatal("resumable prefix bypassed SAR")
		}
		checkpoint = next
	}
	// Existing paper source profiles are closed contracts: retain their
	// explicit SAR rejection rather than broadening a separately reviewed API.
	if _, err := RunPaperPrefix(paper); err == nil {
		t.Fatal("paper profile unexpectedly widened")
	}
	base := paperORBRequest(t, "close")
	p, binding, err := preparePaperPrefix(base)
	if err != nil {
		t.Fatal(err)
	}
	p.parabolicSAR = prepareParabolicSAR(cfg, p.series)
	result, _, err := executePaperPrefix(base, p, binding)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.OpenPositions) != 0 || result.CompletedTrades.Count != 0 || result.Pending.Market != 0 {
		t.Fatal("internal paper reset lost SAR")
	}
	bid := bidAskTestRequest(t)
	denied := bid
	denied.Source = strings.Replace(bid.Source, "dsl v6", "dsl v7", 1) + "\nfilters { sar trend bearish }\n"
	if _, err := RunPaperBidAsk(denied); err == nil {
		t.Fatal("bid/ask profile unexpectedly widened")
	}
	p, bidBinding, err := prepareBidAsk(bid, true)
	if err != nil {
		t.Fatal(err)
	}
	p.parabolicSAR = prepareParabolicSAR(cfg, p.series)
	x := newBidAskRunner(bid, p, bidBinding)
	if x.prepared.broker.parabolicSAR == nil || x.prepared.broker.parabolicSARGateOK(3, sideLong) {
		t.Fatal("bid/ask reset lost SAR")
	}

}
