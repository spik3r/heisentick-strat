package engine

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const rrTestSource = `dsl v7
strategy "Synthetic range reversion" { description "Order timing fixture" }
market {
 rangereversion timeframe M30
 rangereversion source H4 availability next-native-row
}
setup {
 type: range reversion
 rangereversion policy DELAYED_PINE_OHLC_V1
 rangereversion bounds chart 2
 rangereversion candle-color true
 rangereversion htf-ema false 200
 rangereversion vector-gates false 14 30 48
 rangereversion range-expansion false 1.1
 rangereversion atr 2
 rangereversion stop-atr 0.6
 rangereversion target-r 2
 rangereversion cooldown 2
 rangereversion breakeven false 1 10
 rangereversion tick-size 0.01
}
`

func rrFixtureSeries(bars []marketdata.Bar) marketdata.Series { return marketdata.SeriesFromBars(bars) }
func rrFixtureConfig(t *testing.T, policy string) dsl.Config {
	t.Helper()
	source := strings.Replace(rrTestSource, dsl.RangeReversionDelayedPinePolicy, policy, 1)
	if policy == dsl.RangeReversionImmediatePolicy {
		source = strings.Replace(source, "rangereversion bounds chart 2", "rangereversion bounds source 1", 1)
		source = strings.Replace(source, "rangereversion candle-color true", "rangereversion candle-color false", 1)
	}
	parsed, err := dsl.Parse(source)
	if err != nil || len(parsed.Errors) > 0 {
		t.Fatalf("parse config: %v %v", err, parsed.Errors)
	}
	return parsed.Config
}
func rrBars() []marketdata.Bar {
	return []marketdata.Bar{
		{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1},
		{T: 1800000, O: 100, H: 101, L: 99, C: 100, V: 1},
		{T: 3600000, O: 101, H: 103, L: 98, C: 100, V: 1},
		{T: 5400000, O: 100, H: 109, L: 99, C: 106, V: 1},
		{T: 7200000, O: 100.2, H: 101, L: 99, C: 100, V: 1},
	}
}
func rrSourceBars() []marketdata.Bar {
	return []marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}, {T: 14400000, O: 100, H: 102, L: 98, C: 100, V: 1}, {T: 43200000, O: 100, H: 103, L: 97, C: 100, V: 1}}
}

func rrImmediateGapBars() ([]marketdata.Bar, []marketdata.Bar) {
	entry := make([]marketdata.Bar, 40)
	for i := range entry {
		entry[i] = marketdata.Bar{T: float64(i) * 1800000, O: 9, H: 10, L: 8, C: 9, V: 1}
	}
	entry[16] = marketdata.Bar{T: 16 * 1800000, O: 9.8, H: 10.5, L: 9.3, C: 9.5, V: 1}
	entry[17] = marketdata.Bar{T: 17 * 1800000, O: 12, H: 12.2, L: 11.9, C: 12, V: 1}
	source := []marketdata.Bar{
		{T: 0, O: 9, H: 10.1, L: 8, C: 9, V: 1},
		{T: 14400000, O: 9, H: 10, L: 8, C: 9, V: 1},
		{T: 28800000, O: 9, H: 10.5, L: 8, C: 9, V: 1},
		{T: 43200000, O: 9, H: 10.5, L: 8, C: 9, V: 1},
	}
	return entry, source
}

func TestRangeReversionImmediateEntryThroughStopFillsAndChargesBothSides(t *testing.T) {
	bars, source := rrImmediateGapBars()
	request := RangeReversionRequest{
		Config:       rrFixtureConfig(t, dsl.RangeReversionImmediatePolicy),
		EntrySeries:  rrFixtureSeries(bars),
		SourceSeries: rrFixtureSeries(source),
		Window:       RangeReversionWindow{TradeFromMS: 0, TradeToMS: 72000000},
		Execution:    RangeReversionExecution{Units: 1},
	}
	result, err := RunRangeReversion(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Signals) != 1 || len(result.Trades) != 1 || result.CanceledAtEntryRisk != 0 {
		t.Fatalf("signals=%d trades=%d canceled=%d", len(result.Signals), len(result.Trades), result.CanceledAtEntryRisk)
	}
	trade := result.Trades[0]
	if trade.Side != "short" || trade.ExitReason != "stop-gap" || trade.EntryIndex != 17 || trade.ExitIndex != 17 || trade.EntryMS != trade.ExitMS {
		t.Fatalf("unexpected immediate through-stop trade: %+v", trade)
	}
	if trade.Entry <= trade.Stop || math.Abs(trade.Entry-trade.Stop) <= 1e-12 || trade.Entry != 12 || trade.Exit != 12 || trade.GrossPnL != 0 || trade.NetPnL != 0 {
		t.Fatalf("raw through-stop fill should flatten at the same open with zero P&L: %+v", trade)
	}

	request.Execution = RangeReversionExecution{SlippagePerFill: .06, CommissionPerUnitSide: .5, Units: 1}
	withCosts, err := RunRangeReversion(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(withCosts.Trades) != 1 || withCosts.CanceledAtEntryRisk != 0 {
		t.Fatalf("costed signals=%d trades=%d canceled=%d", len(withCosts.Signals), len(withCosts.Trades), withCosts.CanceledAtEntryRisk)
	}
	costed := withCosts.Trades[0]
	if math.Abs(costed.Entry-11.94) > 1e-12 || math.Abs(costed.Exit-12.06) > 1e-12 || math.Abs(costed.GrossPnL-(-.12)) > 1e-12 || math.Abs(costed.CommissionPrice-1) > 1e-12 || math.Abs(costed.NetPnL-(-1.12)) > 1e-12 {
		t.Fatalf("through-stop costs must hit both fills and commission sides: %+v", costed)
	}
}

func TestRangeReversionImmediateCancelsOnlyZeroOrNearZeroFillRisk(t *testing.T) {
	config := rrFixtureConfig(t, dsl.RangeReversionImmediatePolicy)
	baseBars, source := rrImmediateGapBars()
	base, err := RunRangeReversion(RangeReversionRequest{Config: config, EntrySeries: rrFixtureSeries(baseBars), SourceSeries: rrFixtureSeries(source), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 72000000}, Execution: RangeReversionExecution{Units: 1}})
	if err != nil || len(base.Signals) != 1 {
		t.Fatalf("baseline signal setup: signals=%d err=%v", len(base.Signals), err)
	}
	stop := base.Signals[0].Stop
	for _, delta := range []float64{0, 5e-13} {
		t.Run(fmt.Sprintf("risk_%g", delta), func(t *testing.T) {
			bars := append([]marketdata.Bar(nil), baseBars...)
			open := stop - delta
			bars[17] = marketdata.Bar{T: 17 * 1800000, O: open, H: open + .2, L: open - .2, C: open, V: 1}
			result, err := RunRangeReversion(RangeReversionRequest{Config: config, EntrySeries: rrFixtureSeries(bars), SourceSeries: rrFixtureSeries(source), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 72000000}, Execution: RangeReversionExecution{Units: 1}})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Signals) != 1 || len(result.Trades) != 0 || result.CanceledAtEntryRisk != 1 {
				t.Fatalf("risk=%g signals=%d trades=%d canceled=%d", delta, len(result.Signals), len(result.Trades), result.CanceledAtEntryRisk)
			}
		})
	}

	// A distance greater than the documented tolerance remains a valid fill,
	// even when the entry candle then touches the stop.
	bars := append([]marketdata.Bar(nil), baseBars...)
	open := stop - 2e-12
	bars[17] = marketdata.Bar{T: 17 * 1800000, O: open, H: open + .2, L: open - .2, C: open, V: 1}
	result, err := RunRangeReversion(RangeReversionRequest{Config: config, EntrySeries: rrFixtureSeries(bars), SourceSeries: rrFixtureSeries(source), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 72000000}, Execution: RangeReversionExecution{Units: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 1 || result.CanceledAtEntryRisk != 0 || result.Trades[0].ExitReason != "stop" {
		t.Fatalf("positive-risk stop touch should execute, not cancel: trades=%+v canceled=%d", result.Trades, result.CanceledAtEntryRisk)
	}
}

func TestRangeReversionDelayedEntryBarStopLatch(t *testing.T) {
	result, err := RunRangeReversion(RangeReversionRequest{Config: rrFixtureConfig(t, dsl.RangeReversionDelayedPinePolicy), EntrySeries: rrFixtureSeries(rrBars()), SourceSeries: rrFixtureSeries(rrSourceBars()), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 18000000}, Execution: RangeReversionExecution{Units: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Signals) != 1 || len(result.Trades) != 1 {
		t.Fatalf("signals=%d trades=%d", len(result.Signals), len(result.Trades))
	}
	trade := result.Trades[0]
	if trade.EntryIndex != 3 || trade.ExitIndex != 4 || trade.ExitReason != "close-latched-stop" || trade.Exit != 100.2 {
		t.Fatalf("unexpected delayed trade: %+v", trade)
	}
	if result.ConfigSHA256 == "" || result.EntrySHA256 == "" || result.SourceSHA256 == "" {
		t.Fatal("missing frozen identities")
	}
}

func TestRangeReversionSourceAvailabilityUsesNextNativeRow(t *testing.T) {
	bars := []rrBar{{t: 0}, {t: 4 * 3600000}, {t: 12 * 3600000}}
	got := rrSourceAvailability(bars)
	if got[0] != 4*3600000 || got[1] != 12*3600000 || got[2] != int64(^uint64(0)>>1) {
		t.Fatalf("availability=%v", got)
	}
}

func TestRangeReversionSourceBoundsIncludeLatestCompletedRow(t *testing.T) {
	bars := []rrBar{{h: 10, l: 5}, {h: 99, l: 4}, {h: 11, l: 6}}
	hi, lo, ok := rrBounds(bars, 2, 1)
	if !ok || hi != 99 || lo != 4 {
		t.Fatalf("latest completed source bound = (%v,%v,%v)", hi, lo, ok)
	}
}

func TestRangeReversionBreakevenOffsetUsesTenTicksAndFillSlippage(t *testing.T) {
	sourceText := strings.Replace(rrTestSource, "rangereversion breakeven false 1 10", "rangereversion breakeven true 1 10", 1)
	parsed, err := dsl.Parse(sourceText)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse config: %v %v", err, parsed.Errors)
	}
	bars := rrBars()
	bars[3] = marketdata.Bar{T: 5400000, O: 100, H: 101, L: 93, C: 94, V: 1}
	bars[4] = marketdata.Bar{T: 7200000, O: 99.8, H: 100, L: 99, C: 100, V: 1}
	result, err := RunRangeReversion(RangeReversionRequest{Config: parsed.Config, EntrySeries: rrFixtureSeries(bars), SourceSeries: rrFixtureSeries(rrSourceBars()), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 18000000}, Execution: RangeReversionExecution{Units: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 1 {
		t.Fatalf("trades=%d", len(result.Trades))
	}
	trade := result.Trades[0]
	if trade.ExitReason != "stop" || math.Abs(trade.RawExit-99.9) > 1e-9 {
		t.Fatalf("expected stop at entry minus .10, got %+v", trade)
	}
	withSlip, err := RunRangeReversion(RangeReversionRequest{Config: parsed.Config, EntrySeries: rrFixtureSeries(bars), SourceSeries: rrFixtureSeries(rrSourceBars()), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 18000000}, Execution: RangeReversionExecution{SlippagePerFill: .06, Units: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(withSlip.Trades) != 1 || math.Abs(withSlip.Trades[0].Entry-99.94) > 1e-9 {
		t.Fatalf("slippage did not change fill-relative state: %+v", withSlip.Trades)
	}
}

func TestRangeReversionInputHashesIncludeEveryObservedColumn(t *testing.T) {
	cfg := rrFixtureConfig(t, dsl.RangeReversionDelayedPinePolicy)
	window := RangeReversionWindow{TradeFromMS: 0, TradeToMS: 18000000}
	execution := RangeReversionExecution{Units: 1}
	base, err := RunRangeReversion(RangeReversionRequest{Config: cfg, EntrySeries: rrFixtureSeries(rrBars()), SourceSeries: rrFixtureSeries(rrSourceBars()), Window: window, Execution: execution})
	if err != nil {
		t.Fatal(err)
	}
	entry := rrBars()
	entry[0].V = 2
	changedEntry, err := RunRangeReversion(RangeReversionRequest{Config: cfg, EntrySeries: rrFixtureSeries(entry), SourceSeries: rrFixtureSeries(rrSourceBars()), Window: window, Execution: execution})
	if err != nil {
		t.Fatal(err)
	}
	source := rrSourceBars()
	source[0].H += .25
	changedSource, err := RunRangeReversion(RangeReversionRequest{Config: cfg, EntrySeries: rrFixtureSeries(rrBars()), SourceSeries: rrFixtureSeries(source), Window: window, Execution: execution})
	if err != nil {
		t.Fatal(err)
	}
	if base.EntrySHA256 == changedEntry.EntrySHA256 || base.SourceSHA256 == changedSource.SourceSHA256 {
		t.Fatal("input hashes omitted an observed value")
	}
}

func TestRangeReversionImmediateDiscardsDualSweepBeforeGates(t *testing.T) {
	// A high ADX gate would suppress the short side. The raw candle swept both
	// source bounds, so the conceptual policy must discard it before that gate.
	sourceText := strings.Replace(rrTestSource, "rangereversion vector-gates false 14 30 48", "rangereversion vector-gates true 14 0.1 48", 1)
	sourceText = strings.Replace(sourceText, dsl.RangeReversionDelayedPinePolicy, dsl.RangeReversionImmediatePolicy, 1)
	sourceText = strings.Replace(sourceText, "rangereversion bounds chart 2", "rangereversion bounds source 1", 1)
	sourceText = strings.Replace(sourceText, "rangereversion candle-color true", "rangereversion candle-color false", 1)
	parsed, err := dsl.Parse(sourceText)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse config: %v %v", err, parsed.Errors)
	}
	bars := make([]marketdata.Bar, 29)
	for i := range bars {
		bars[i] = marketdata.Bar{T: float64(i) * 1800000, O: 100, H: 101, L: 99, C: 100, V: 1}
	}
	bars[28] = marketdata.Bar{T: 28 * 1800000, O: 100, H: 103, L: 97, C: 100, V: 1}
	result, err := RunRangeReversion(RangeReversionRequest{Config: parsed.Config, EntrySeries: rrFixtureSeries(bars), SourceSeries: rrFixtureSeries(rrSourceBars()), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 29 * 1800000}, Execution: RangeReversionExecution{Units: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Signals) != 0 {
		t.Fatalf("dual sweep created gated one-sided signal: %+v", result.Signals)
	}
}

func TestRangeReversionImmediateCancelsZeroEntryRisk(t *testing.T) {
	cfg := rrFixtureConfig(t, dsl.RangeReversionImmediatePolicy)
	bars, source := rrImmediateGapBars()
	baseline, err := RunRangeReversion(RangeReversionRequest{Config: cfg, EntrySeries: rrFixtureSeries(bars), SourceSeries: rrFixtureSeries(source), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 72000000}, Execution: RangeReversionExecution{Units: 1}})
	if err != nil || len(baseline.Signals) != 1 {
		t.Fatalf("baseline signals=%d err=%v", len(baseline.Signals), err)
	}
	stop := baseline.Signals[0].Stop
	open := stop + .06 // Short-entry slippage brings the fill exactly to the stop.
	bars[17] = marketdata.Bar{T: 17 * 1800000, O: open, H: open + .2, L: open - .2, C: open, V: 1}
	result, err := RunRangeReversion(RangeReversionRequest{Config: cfg, EntrySeries: rrFixtureSeries(bars), SourceSeries: rrFixtureSeries(source), Window: RangeReversionWindow{TradeFromMS: 0, TradeToMS: 72000000}, Execution: RangeReversionExecution{SlippagePerFill: .06, Units: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Signals) != 1 || len(result.Trades) != 0 || result.CanceledAtEntryRisk != 1 {
		t.Fatalf("signals=%d trades=%d canceled=%d", len(result.Signals), len(result.Trades), result.CanceledAtEntryRisk)
	}
}

func TestRangeReversionRejectsGenericEngineExecution(t *testing.T) {
	cfg := rrFixtureConfig(t, dsl.RangeReversionDelayedPinePolicy)
	if _, err := Run(RunRequest{Config: cfg}); err == nil || !strings.Contains(err.Error(), dsl.RangeReversionDedicatedRunnerRequired) {
		t.Fatalf("generic run error=%v", err)
	}
}
