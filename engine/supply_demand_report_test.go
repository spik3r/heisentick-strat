package engine

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func sdReportRequest(t *testing.T, fixture RunFixture) []byte {
	t.Helper()
	rows := func(bars []marketdata.Bar) [][]float64 {
		out := make([][]float64, len(bars))
		for i, b := range bars {
			out[i] = []float64{b.T, b.O, b.H, b.L, b.C, b.V}
		}
		return out
	}
	request := map[string]any{
		"schema": SupplyDemandRetestRequestSchema, "case": fixture.Case,
		"strategyId": fixture.StrategyID, "symbol": fixture.Symbol, "timeframe": fixture.Timeframe,
		"sourceTimeframe": fixture.SourceTimeframe, "higherTimeframe": fixture.HigherTimeframe,
		"rangeMethod": fixture.RangeMethod, "bars": rows(fixture.Bars), "sourceBars": rows(fixture.SourceBars), "sourceHtfBars": rows(fixture.SourceHTFBars),
		"costs": map[string]any{"feePerUnit": fixture.Costs.FeePerUnit, "slippage": fixture.Costs.Slippage, "slippageBps": fixture.Costs.SlippageBps, "startEquity": fixture.Costs.StartEquity, "fillOn": fixture.Costs.FillOn},
	}
	b, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sdReportFixture() RunFixture {
	chart, source := entryRetestBars(-1)
	return RunFixture{Schema: runFixtureSchema, Case: "synthetic-sd-report", StrategyID: "synthetic-sd-report", Symbol: "XAUUSD", Timeframe: "1h", SourceTimeframe: "4h", RangeMethod: "zone", Costs: Costs{StartEquity: 1000, FillOn: "close"}, Bars: chart, SourceBars: source}
}

func sdReportMirror(f *RunFixture) {
	for _, bars := range [][]marketdata.Bar{f.Bars, f.SourceBars} {
		for i, b := range bars {
			bars[i].O, bars[i].H, bars[i].L, bars[i].C = 200-b.O, 200-b.L, 200-b.H, 200-b.C
		}
	}
}

func sdReportRun(t *testing.T, fixture RunFixture, source string) SupplyDemandRetestExecution {
	t.Helper()
	out, err := RunSupplyDemandRetestWithAccounting(sdReportRequest(t, fixture), source)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sdReportSameRun(t *testing.T, a, b RunResult) {
	t.Helper()
	left, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatalf("retained run differs:\n%s\n%s", left, right)
	}
}

func TestSupplyDemandReportCaptureParityRoutesAndRepeat(t *testing.T) {
	for _, tf := range []string{"1h", "30m", "15m"} {
		for _, short := range []bool{false, true} {
			for _, fill := range []string{"close", "open", "nextOpen"} {
				t.Run(tf+"/"+map[bool]string{false: "long", true: "short"}[short]+"/"+fill, func(t *testing.T) {
					f := sdReportFixture()
					if short {
						sdReportMirror(&f)
					}
					dslSource := strings.ReplaceAll(entryRetestSource, "1h", tf)
					parts := map[string]int{"1h": 1, "30m": 2, "15m": 4}[tf]
					if parts > 1 {
						expanded := make([]marketdata.Bar, 0, len(f.Bars)*parts)
						for _, b := range f.Bars {
							for j := 0; j < parts; j++ {
								x := b
								x.T += float64(j) * hourMS / float64(parts)
								expanded = append(expanded, x)
							}
						}
						f.Bars = expanded
					}
					f.Timeframe = tf
					f.Costs = Costs{StartEquity: 1000, FeePerUnit: .03125, Slippage: .0625, SlippageBps: 1.25, FillOn: fill}
					legacy, err := RunFixtureCase(f, dslSource)
					if err != nil {
						t.Fatal(err)
					}
					on := sdReportRun(t, f, dslSource)
					if on.Run.TradeCount != 1 {
						t.Fatalf("expected a real trade: %+v", on.Run.Trades)
					}
					sdReportSameRun(t, legacy, on.Run)
					admitted, err := prepareSupplyDemandRetest(sdReportRequest(t, f), dslSource)
					if err != nil {
						t.Fatal(err)
					}
					chart, src, daily := marketdata.SeriesFromBars(f.Bars), marketdata.SeriesFromBars(f.SourceBars), marketdata.SeriesFromBars(f.SourceHTFBars)
					offTrades := runSourceRetestSeriesWithOptions(f, admitted.cfg, paramsFromConfig(admitted.cfg), chart, src, daily, ExecutionBounds{TradeEnd: len(f.Bars) - 1}, false, &supplyDemandRetestOptions{})
					off, err := checkedResultEnvelope(f, offTrades)
					if err != nil {
						t.Fatal(err)
					}
					sdReportSameRun(t, off, on.Run)
					if len(on.Accounting.Marks) != len(f.Bars) || len(on.Accounting.Trades) != 1 || len(on.Accounting.Events) != 2 {
						t.Fatalf("capture counts: %+v", on.Accounting)
					}
					for i, m := range on.Accounting.Marks {
						if m.Index != i || m.T != f.Bars[i].T {
							t.Fatalf("original chart mark lost: %+v", m)
						}
					}
					last := on.Accounting.Marks[len(f.Bars)-1]
					if last.Unrealized != 0 || last.Equity != on.Accounting.EndEquity || on.Terminal.Position != "flat" {
						t.Fatal("terminal is not flat")
					}
					if on.Accounting.Profile != SupplyDemandRetestProfile || on.Accounting.Policy != "off" {
						t.Fatalf("wrong qualification: %+v", on.Accounting)
					}
					for repeat := 0; repeat < 2; repeat++ {
						again := sdReportRun(t, f, dslSource)
						if !reflect.DeepEqual(on, again) {
							t.Fatal("capture leaked between repeated calls")
						}
					}
				})
			}
		}
	}
}

func TestSupplyDemandReportNoTradesAndLastBarPending(t *testing.T) {
	f := sdReportFixture()
	f.Bars = f.Bars[:retestBar+1]
	f.Costs.FillOn = "nextOpen"
	pending := sdReportRun(t, f, entryRetestSource)
	if pending.Run.TradeCount != 0 || pending.Terminal.PendingMarketOrders != 1 || pending.Terminal.Liquidated || pending.Terminal.EndOfTestTrades != 0 || len(pending.Accounting.Events) != 0 {
		t.Fatalf("last-bar order must remain unfilled: %+v", pending)
	}
	f.Bars = f.Bars[:retestBar]
	none := sdReportRun(t, f, entryRetestSource)
	if none.Run.TradeCount != 0 || none.Terminal.PendingMarketOrders != 0 || len(none.Accounting.Events) != 0 {
		t.Fatalf("no signal must be distinguishable from pending: %+v", none)
	}
	for _, m := range none.Accounting.Marks {
		if m.Realized != 0 || m.Unrealized != 0 || m.Equity != 1000 {
			t.Fatalf("no-trade mark: %+v", m)
		}
	}
}

func TestSupplyDemandReportFinalLiquidationAndNextSuppliedOpen(t *testing.T) {
	for _, fill := range []string{"close", "open", "nextOpen"} {
		t.Run(fill, func(t *testing.T) {
			f := sdReportFixture()
			f.Costs.FillOn = fill
			f.Bars = f.Bars[:retestBar+1]
			if fill != "close" {
				b := f.Bars[len(f.Bars)-1]
				b.T += 3 * hourMS
				b.O, b.H, b.L, b.C = 101.5, 101.6, 101.4, 101.5
				f.Bars = append(f.Bars, b)
			}
			out := sdReportRun(t, f, entryRetestSource)
			if out.Run.TradeCount != 1 || !out.Terminal.Liquidated || out.Terminal.EndOfTestTrades != 1 || out.Run.Trades[0].Reason != ReasonEndOfTest {
				t.Fatalf("terminal policy: %+v %+v", out.Terminal, out.Run.Trades)
			}
			tr := out.Run.Trades[0]
			last := len(f.Bars) - 1
			if tr.ExitIndex != last || tr.ExitT != f.Bars[last].T || tr.EntryIndex != last || tr.Exit != f.Bars[last].C {
				t.Fatalf("next supplied row/final close semantics: %+v", tr)
			}
			if len(out.Accounting.Marks) != len(f.Bars) || out.Accounting.Marks[last].Unrealized != 0 {
				t.Fatal("final mark was not replaced")
			}
		})
	}
}

func TestSupplyDemandReportHandDerivedLongShortCashMarks(t *testing.T) {
	for _, direction := range []side{sideLong, sideShort} {
		t.Run(direction.String(), func(t *testing.T) {
			bars := make([]marketdata.Bar, 4)
			for i, delta := range []float64{0, 2, -1, 3} {
				c := 100 + float64(direction)*delta
				bars[i] = marketdata.Bar{T: float64(i) * hourMS, O: c, H: c + .1, L: c - .1, C: c, V: 1}
			}
			b := new(broker)
			b.reset(marketdata.SeriesFromBars(bars), contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: Costs{StartEquity: 100, FeePerUnit: .25, Slippage: .25, FillOn: "close"}}, nil)
			capture := newSequentialAccountingCollector(100)
			b.sequentialAccounting = capture
			b.pendingOrders = []order{{Side: direction, Size: 2, HasSize: true, SL: 100 - float64(direction)*20, TP: 100 + float64(direction)*20, Index: 0}}
			terminal := SupplyDemandRetestTerminal{}
			r := sdEntryRetestRun{src: &broker{}, idx: []int{-1, -1, -1, -1}, done: -1, terminal: &terminal}
			trades := r.run(b)
			got, err := capture.finish(4, 1)
			if err != nil {
				t.Fatal(err)
			}
			// Entry 100 +/- .25, size 2: debit .50. Close marks: -.50,
			// +3.50, -2.50 unrealized. Final liquidation at 103 - .25 (long)
			// or 97 + .25 (short) credits 4.50; net after both fees is 4.00.
			want := []SequentialEquityMark{{0, 0, -.5, -.5, 99}, {1, hourMS, -.5, 3.5, 103}, {2, 2 * hourMS, -.5, -2.5, 97}, {3, 3 * hourMS, 4, 0, 104}}
			if !reflect.DeepEqual(got.Marks, want) || got.FinalRealized != 4 || got.EndEquity != 104 {
				t.Fatalf("hand-derived marks: got %+v want %+v", got, want)
			}
			if len(trades) != 1 || trades[0].PnL != 4.5 || got.Trades[0].NetPnL != 4 || got.Trades[0].EntryFee != .5 || got.Trades[0].ExitFee != .5 {
				t.Fatalf("exit credit is not all-fee net: %+v %+v", trades, got.Trades)
			}
			if !terminal.Liquidated || terminal.EndOfTestTrades != 1 || terminal.PendingMarketOrders != 0 {
				t.Fatalf("endpoint: %+v", terminal)
			}
		})
	}
}

func TestSupplyDemandReportObservesPendingBeforeLiquidation(t *testing.T) {
	b, c := accountingBroker(Costs{StartEquity: 100}, 100, 100)
	b.openPosition(sideLong, 100, order{Size: 1, HasSize: true, SL: 90, TP: 110}, 0)
	b.pendingOrders = []order{{Index: 5}}
	b.limitOrders = []order{{Side: sideLong, HasLimit: true, Limit: 50, ExpireAt: 5}}
	b.pendingExits = []pendingExit{{PositionEntryIndex: 0, Index: 5, Rule: "synthetic"}}
	terminal := SupplyDemandRetestTerminal{}
	r := sdEntryRetestRun{src: &broker{}, idx: []int{-1, -1}, done: -1, terminal: &terminal}
	r.run(b)
	if terminal.PendingMarketOrders != 1 || terminal.PendingLimitOrders != 1 || terminal.PendingDeferredExits != 1 || !terminal.Liquidated || len(b.pendingExits) != 0 {
		t.Fatalf("pending endpoint lost: %+v", terminal)
	}
	if _, err := c.finish(2, 1); err != nil {
		t.Fatal(err)
	}
}

func sdReportDate(s string) float64 {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return float64(t.UnixMilli())
}
func sdReportDaily(date string, direction int) marketdata.Bar {
	return marketdata.Bar{T: sdReportDate(date), O: 100, H: 102, L: 98, C: 100 + float64(direction), V: 1}
}
func sdReportShift(f *RunFixture, date string) {
	offset := sdReportDate(date)
	for _, bars := range [][]marketdata.Bar{f.Bars, f.SourceBars} {
		for i := range bars {
			bars[i].T += offset
		}
	}
}

func TestSupplyDemandReportCompletedDailyCausality(t *testing.T) {
	for _, direction := range []int{-1, 0, 1} {
		daily := marketdata.SeriesFromBars([]marketdata.Bar{sdReportDaily("2026-01-05", direction), sdReportDaily("2026-01-06", direction)})
		decisionTimes := []float64{sdReportDate("2026-01-05"), sdReportDate("2026-01-07"), sdReportDate("2026-01-08") + 4*hourMS, sdReportDate("2026-01-11"), sdReportDate("2026-02-01")}
		bars := make([]marketdata.Bar, len(decisionTimes))
		for i, d := range decisionTimes {
			bars[i].T = d - 4*hourMS
		}
		source := marketdata.SeriesFromBars(bars)
		got := supplyDemandRetestCompletedDailyTrend(source, daily)
		trend := map[int]int8{-1: trendDown, 0: trendFlat, 1: trendUp}[direction]
		want := []int8{htfUnavailable, trend, htfUnavailable, trend, trend}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("direction %d: got%v want%v", direction, got, want)
		}
		for _, date := range []string{"2026-01-12", "2026-01-13", "2026-02-02"} {
			changed := marketdata.SeriesFromBars([]marketdata.Bar{sdReportDaily("2026-01-05", direction), sdReportDaily("2026-01-06", direction), sdReportDaily(date, -direction)})
			prefix := marketdata.SeriesFromBars(bars[:3])
			if have := supplyDemandRetestCompletedDailyTrend(prefix, changed); !reflect.DeepEqual(have, want[:3]) {
				t.Fatalf("unclosed future timestamp/value changed earlier direction: %v", have)
			}
		}
	}
}

func TestSupplyDemandReportDailyFutureRowsActualSignalWitness(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(map[bool]string{false: "long", true: "short"}[short], func(t *testing.T) {
			f := sdReportFixture()
			if short {
				sdReportMirror(&f)
			}
			sdReportShift(&f, "2026-01-02") // retest Thursday 8 January 13:00
			direction := 1
			if short {
				direction = -1
			}
			f.HigherTimeframe = "1d"
			f.SourceHTFBars = []marketdata.Bar{sdReportDaily("2026-01-05", direction), sdReportDaily("2026-01-06", direction)}
			source := strings.Replace(entryRetestSource, "higher timeframe off", "higher timeframe must agree", 1)
			prefix := sdReportRun(t, f, source)
			if prefix.Run.TradeCount != 0 {
				t.Fatalf("stale Thursday daily direction must block: %+v", prefix.Run.Trades)
			}
			legacyPrefix, err := RunFixtureCase(f, source)
			if err != nil {
				t.Fatal(err)
			}
			sdReportSameRun(t, legacyPrefix, prefix.Run)
			for _, date := range []string{"2026-01-12", "2026-01-13"} {
				for _, futureDirection := range []int{-1, 0, 1} {
					changed := f
					changed.SourceHTFBars = append(append([]marketdata.Bar{}, f.SourceHTFBars...), sdReportDaily(date, futureDirection))
					got := sdReportRun(t, changed, source)
					sdReportSameRun(t, prefix.Run, got.Run)
					legacy, err := RunFixtureCase(changed, source)
					if err != nil {
						t.Fatal(err)
					}
					if legacy.TradeCount != 1 || legacy.Trades[0].EntryIndex != retestBar {
						t.Fatalf("fixture must reproduce actual generic future-dependent signal: %+v", legacy.Trades)
					}
					// Same-policy capture-off remains equal while the deliberately fixed
					// legacy future-timestamp witness is different.
					admitted, err := prepareSupplyDemandRetest(sdReportRequest(t, changed), source)
					if err != nil {
						t.Fatal(err)
					}
					chart, src, daily := marketdata.SeriesFromBars(changed.Bars), marketdata.SeriesFromBars(changed.SourceBars), marketdata.SeriesFromBars(changed.SourceHTFBars)
					trades := runSourceRetestSeriesWithOptions(changed, admitted.cfg, paramsFromConfig(admitted.cfg), chart, src, daily, ExecutionBounds{TradeEnd: chart.Len() - 1}, false, &supplyDemandRetestOptions{sourceHTFTrend: supplyDemandRetestCompletedDailyTrend(src, daily)})
					off, err := checkedResultEnvelope(changed, trades)
					if err != nil {
						t.Fatal(err)
					}
					sdReportSameRun(t, off, got.Run)
				}
			}
		})
	}
}

func TestSupplyDemandReportDailyGateAndWeekendSignal(t *testing.T) {
	for _, tc := range []struct {
		name, start, first, second string
		direction                  int
		short                      bool
		trades                     int
	}{
		{"fresh-up-long", "2026-01-02", "2026-01-06", "2026-01-07", 1, false, 1},
		{"fresh-up-short-blocked", "2026-01-02", "2026-01-06", "2026-01-07", 1, true, 0},
		{"fresh-down-short", "2026-01-02", "2026-01-06", "2026-01-07", -1, true, 1},
		{"fresh-down-long-blocked", "2026-01-02", "2026-01-06", "2026-01-07", -1, false, 0},
		{"flat-long", "2026-01-02", "2026-01-06", "2026-01-07", 0, false, 1},
		{"flat-short", "2026-01-02", "2026-01-06", "2026-01-07", 0, true, 1},
		{"ordinary-weekend", "2026-01-05", "2026-01-08", "2026-01-09", 1, false, 1},
		{"ordinary-weekend-short", "2026-01-05", "2026-01-08", "2026-01-09", -1, true, 1},
		{"retained-indefinite-staleness", "2026-02-02", "2026-01-05", "2026-01-06", 1, false, 1},
		{"holiday-missing-weekday", "2026-01-02", "2026-01-05", "2026-01-06", 1, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := sdReportFixture()
			if tc.short {
				sdReportMirror(&f)
			}
			sdReportShift(&f, tc.start)
			f.HigherTimeframe = "1d"
			f.SourceHTFBars = []marketdata.Bar{sdReportDaily(tc.first, tc.direction), sdReportDaily(tc.second, tc.direction)}
			source := strings.Replace(entryRetestSource, "higher timeframe off", "higher timeframe must agree", 1)
			out := sdReportRun(t, f, source)
			if out.Run.TradeCount != tc.trades {
				t.Fatalf("got %+v, want %d", out.Run.Trades, tc.trades)
			}
			legacy, err := RunFixtureCase(f, source)
			if err != nil {
				t.Fatal(err)
			}
			sdReportSameRun(t, legacy, out.Run)
			admitted, err := prepareSupplyDemandRetest(sdReportRequest(t, f), source)
			if err != nil {
				t.Fatal(err)
			}
			chart, src, daily := marketdata.SeriesFromBars(f.Bars), marketdata.SeriesFromBars(f.SourceBars), marketdata.SeriesFromBars(f.SourceHTFBars)
			offTrades := runSourceRetestSeriesWithOptions(f, admitted.cfg, paramsFromConfig(admitted.cfg), chart, src, daily, ExecutionBounds{TradeEnd: chart.Len() - 1}, false, &supplyDemandRetestOptions{sourceHTFTrend: supplyDemandRetestCompletedDailyTrend(src, daily)})
			off, err := checkedResultEnvelope(f, offTrades)
			if err != nil {
				t.Fatal(err)
			}
			sdReportSameRun(t, off, out.Run)
			if out.Identity.HTFPolicy != SupplyDemandRetestHTFPolicy || out.Accounting.Policy != SupplyDemandRetestHTFPolicy {
				t.Fatal("corrected policy not identified")
			}
		})
	}
}

func TestSupplyDemandReportFiniteCostOverflowReturnsNoPartialExecution(t *testing.T) {
	f := sdReportFixture()
	f.Costs.FeePerUnit = math.MaxFloat64
	got, err := RunSupplyDemandRetestWithAccounting(sdReportRequest(t, f), entryRetestSource)
	if err == nil || !reflect.DeepEqual(got, SupplyDemandRetestExecution{}) {
		t.Fatalf("overflow leaked success: %+v %v", got, err)
	}
}

func TestSupplyDemandReportDailyCloseAlignment(t *testing.T) {
	daily := marketdata.SeriesFromBars([]marketdata.Bar{sdReportDaily("2026-01-06", 1), sdReportDaily("2026-01-07", -1), sdReportDaily("2026-01-08", 1)})
	start := sdReportDate("2026-01-07") + 16*hourMS
	src := marketdata.SeriesFromBars([]marketdata.Bar{{T: start}, {T: start + 4*hourMS}, {T: start + 8*hourMS}})
	trend := supplyDemandRetestCompletedDailyTrend(src, daily)
	if !reflect.DeepEqual(trend, []int8{trendUp, trendDown, trendDown}) {
		t.Fatalf("daily close must align exactly at completed 4h decision: %v", trend)
	}
	for _, step := range []float64{hourMS, hourMS / 2, hourMS / 4} {
		close := sdReportDate("2026-01-08")
		chart := marketdata.SeriesFromBars([]marketdata.Bar{{T: close - 2*step}, {T: close - step}, {T: close}})
		got := projectSourceInt8(chart, src, trend)
		if !reflect.DeepEqual(got, []int8{trendUp, trendDown, trendDown}) {
			t.Fatalf("entry step %v: full daily-to-4h-to-entry alignment %v", step, got)
		}
	}
}

func TestSupplyDemandReportFutureSourceAndGapSignalInvariance(t *testing.T) {
	for _, short := range []bool{false, true} {
		f := sdReportFixture()
		if short {
			sdReportMirror(&f)
		}
		f.Bars = f.Bars[:retestBar+1]
		base := sdReportRun(t, f, entryRetestSource)
		if base.Run.TradeCount != 1 {
			t.Fatal("control must enter")
		}
		for _, mutate := range []string{"ohlc", "timestamps", "append"} {
			changed := f
			changed.SourceBars = append([]marketdata.Bar{}, f.SourceBars...)
			for i := 39; i < len(changed.SourceBars); i++ {
				if mutate == "ohlc" {
					b := &changed.SourceBars[i]
					b.O, b.H, b.L, b.C = 100, 200, 0, 150
				}
				if mutate == "timestamps" {
					changed.SourceBars[i].T += 4 * hourMS
				}
			}
			if mutate == "append" {
				last := changed.SourceBars[len(changed.SourceBars)-1]
				last.T += 4 * hourMS
				last.O, last.H, last.L, last.C = 100, 200, 0, 150
				changed.SourceBars = append(changed.SourceBars, last)
			}
			out := sdReportRun(t, changed, entryRetestSource)
			sdReportSameRun(t, base.Run, out.Run)
			if !reflect.DeepEqual(base.Accounting, out.Accounting) {
				t.Fatalf("future source %s changed accounting", mutate)
			}
		}
		gap := f
		gap.SourceBars = append(append([]marketdata.Bar{}, f.SourceBars[:38]...), f.SourceBars[39:]...)
		blocked := sdReportRun(t, gap, entryRetestSource)
		if blocked.Run.TradeCount != 0 {
			t.Fatalf("missing source interval must block new signal: %+v", blocked.Run.Trades)
		}
	}
}

func TestSupplyDemandReportPendingAndManagementContinueAcrossSourceGap(t *testing.T) {
	for _, fill := range []string{"close", "open", "nextOpen"} {
		f := sdReportFixture()
		f.Costs.FillOn = fill
		f.Bars = f.Bars[:retestBar+1]
		b := f.Bars[len(f.Bars)-1]
		b.T += 3 * hourMS
		b.O, b.H, b.L, b.C = 101.5, 101.6, 101.4, 101.5
		f.Bars = append(f.Bars, b)
		f.SourceBars = append(append([]marketdata.Bar{}, f.SourceBars[:39]...), f.SourceBars[41:]...)
		source := strings.Replace(entryRetestSource, "breakeven off", "breakeven off\n  maxHoldCandles 1", 1)
		out := sdReportRun(t, f, source)
		if out.Run.TradeCount != 1 {
			t.Fatalf("source gap must not stop broker execution: %+v", out.Run.Trades)
		}
		tr := out.Run.Trades[0]
		wantReason := ReasonEndOfTest
		if fill == "close" {
			wantReason = "time"
		}
		if tr.ExitIndex != len(f.Bars)-1 || tr.Reason != wantReason {
			t.Fatalf("management/fills stopped in source gap: %+v", tr)
		}
	}
}

func TestSupplyDemandReportManagementRetainsBrokerOrdering(t *testing.T) {
	for _, short := range []bool{false, true} {
		for _, tc := range []struct {
			name, management string
			sl               bool
			reason           string
		}{
			{"max-hold", "breakeven off\n  maxHoldCandles 1", false, "time"},
			{"breakeven", "move stop to breakeven after 0.1R plus 0 ATR", true, "sl"},
			{"trail", "breakeven off\n  trail 0.1 ATR after 0.1R", true, "sl"},
		} {
			t.Run(tc.name+map[bool]string{false: "-long", true: "-short"}[short], func(t *testing.T) {
				f := sdReportFixture()
				f.Bars = f.Bars[:retestBar+4]
				for i := retestBar + 1; i < len(f.Bars); i++ {
					f.Bars[i].O, f.Bars[i].H, f.Bars[i].L, f.Bars[i].C = 103, 103.2, 102.9, 103
				}
				if tc.sl {
					b := &f.Bars[retestBar+2]
					b.O, b.H, b.L, b.C = 103, 103.2, 100.5, 101
				}
				if short {
					sdReportMirror(&f)
				}
				source := strings.Replace(entryRetestSource, "target 2R", "target 100R", 1)
				source = strings.Replace(source, "breakeven off", tc.management, 1)
				out := sdReportRun(t, f, source)
				legacy, err := RunFixtureCase(f, source)
				if err != nil {
					t.Fatal(err)
				}
				sdReportSameRun(t, legacy, out.Run)
				if out.Run.TradeCount != 1 || out.Run.Trades[0].Reason != tc.reason {
					t.Fatalf("management not exercised: %+v", out.Run.Trades)
				}
				if tc.sl && out.Run.Trades[0].SL == out.Run.Trades[0].InitialSL {
					t.Fatalf("stop did not move: %+v", out.Run.Trades[0])
				}
			})
		}
	}
}

func TestSupplyDemandReportFormationFlipWholeEntryInterval(t *testing.T) {
	for _, flip := range []bool{false, true} {
		for _, short := range []bool{false, true} {
			for _, later := range []bool{false, true} {
				f := sdReportFixture()
				for i := range f.Bars {
					f.Bars[i].T += 12 * hourMS
				}
				boundary := 4*zoneSource + 3
				f.Bars[boundary].H, f.Bars[boundary].C = 116.1, 116
				if flip {
					boundary += 4
					f.Bars[boundary].L, f.Bars[boundary].C = 94.9, 95
					if later {
						b := &f.Bars[boundary+1]
						b.O, b.H, b.L, b.C = 95, 100.1, 94.9, 99
					}
				} else {
					f.Bars[boundary].L = 100.1
					if later {
						b := &f.Bars[boundary+1]
						b.O, b.H, b.L, b.C = 108, 108.2, 100.1, 101
					}
				}
				f.SourceBars = retestBoundarySource(f.Bars)
				if short {
					sdReportMirror(&f)
				}
				source := strings.Replace(entryRetestSource, "min 0.4 ATR max 3 ATR", "min 0.4 ATR max 30 ATR", 1)
				source = strings.Replace(source, "impulse at least 2.5 ATR", "impulse at least 6 ATR", 1)
				if flip {
					source = strings.Replace(source, "  wait 0 candles after zone", "  zone flip\n  wait 0 candles after zone", 1)
				}
				out := sdReportRun(t, f, source)
				legacy, err := RunFixtureCase(f, source)
				if err != nil {
					t.Fatal(err)
				}
				sdReportSameRun(t, legacy, out.Run)
				found := false
				for _, tr := range out.Run.Trades {
					if tr.EntryIndex == boundary {
						t.Fatalf("formation/flip used preceding entry range: %+v", tr)
					}
					if tr.EntryIndex == boundary+1 {
						found = true
					}
				}
				if later && !found {
					t.Fatalf("entry opening at formation/flip close should enter: %+v", out.Run.Trades)
				}
			}
		}
	}
}
