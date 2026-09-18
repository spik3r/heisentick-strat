package engine

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func dailyFlushFailureSeries() marketdata.Series {
	const day = 24 * time.Hour
	bars := make([]marketdata.Bar, 0, 45)
	timestamp := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for len(bars) < 25 {
		if timestamp.Weekday() >= time.Monday && timestamp.Weekday() <= time.Friday {
			bars = append(bars, marketdata.Bar{T: float64(timestamp.UnixMilli()), O: 100, H: 101, L: 99, C: 100, V: 1})
		}
		timestamp = timestamp.Add(day)
	}
	bars[23].O, bars[23].H, bars[23].L, bars[23].C = 100, 101, 95, 96
	bars[24].O, bars[24].H, bars[24].L, bars[24].C = 96, 99, 94.5, 98
	retained := 0
	for retained < 11 {
		price := 98 + float64(retained)
		bars = append(bars, marketdata.Bar{T: float64(timestamp.UnixMilli()), O: price, H: price + 1.5, L: price - 1, C: price + 1, V: 1})
		if timestamp.Weekday() >= time.Monday && timestamp.Weekday() <= time.Friday {
			retained++
		}
		timestamp = timestamp.Add(day)
	}
	return marketdata.SeriesFromBars(bars)
}

func dailyFlushFailureTestParams(maxStop float64) flagParams {
	return flagParams{
		SetupType: string(dsl.FamilyDailyFlushFailure), AllowLong: true,
		MinStopATR: 0.5, MaxStopATR: maxStop, StopBufferATR: 0.1,
		MaxHoldBars: 10, RiskUSD: 200,
		DailyFlushFailure: dailyFlushFailureParams{
			ATRLength: 20, MinFlushRangeATR: 1.25, MaxFlushCloseLocation: 0.25,
		},
	}
}

func dailyFlushFailureSignalRisk(series marketdata.Series) (float64, float64) {
	retained := make([]int, 0, series.Len())
	atr := make([]float64, 0, series.Len())
	for i, timestamp := range series.T {
		if !isWeekdayTimestamp(timestamp) {
			continue
		}
		previousClose := series.C[i]
		if len(retained) > 0 {
			previousClose = series.C[retained[len(retained)-1]]
		}
		trueRange := math.Max(series.H[i]-series.L[i], math.Max(math.Abs(series.H[i]-previousClose), math.Abs(series.L[i]-previousClose)))
		value := trueRange
		if len(atr) > 0 {
			value = (atr[len(atr)-1]*19 + trueRange) / 20
		}
		retained = append(retained, i)
		atr = append(atr, value)
	}
	trigger := retained[24]
	stop := series.L[trigger] - 0.1*atr[24]
	return stop, atr[24]
}

func TestDailyFlushFailureRuntimeSkipsWeekendsAndUsesRetainedHold(t *testing.T) {
	b := broker{
		series: dailyFlushFailureSeries(),
		costs:  Costs{FillOn: "close", StartEquity: 10_000},
		params: dailyFlushFailureTestParams(3),
	}
	trades := b.run()
	if len(trades) != 1 {
		t.Fatalf("trades = %d, want 1", len(trades))
	}
	trade := trades[0]
	if trade.Entry != 98 || trade.Reason != "time" || trade.ExitIndex-trade.EntryIndex != 14 {
		t.Fatalf("trade = %+v", trade)
	}
	if time.UnixMilli(int64(trade.EntryT)).UTC().Weekday() != time.Monday {
		t.Fatalf("entry weekday = %s", time.UnixMilli(int64(trade.EntryT)).UTC().Weekday())
	}
	encoded, err := json.Marshal(RunResult{Trades: []Trade{trade}})
	if err != nil {
		t.Fatalf("Marshal trade: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal trade: %v", err)
	}
	serializedTrade := decoded["trades"].([]any)[0].(map[string]any)
	if serializedTrade["tp"] != nil || serializedTrade["initialTp"] != nil {
		t.Fatalf("serialized no-target fields = tp %#v initialTp %#v", serializedTrade["tp"], serializedTrade["initialTp"])
	}
}

func TestDailyFlushFailureSlippedFillUsesInclusiveActualRiskGate(t *testing.T) {
	series := dailyFlushFailureSeries()
	stop, atr := dailyFlushFailureSignalRisk(series)
	entryIndex := 27 // 25 weekdays followed by Saturday/Sunday, then Monday.
	rawBoundary := (series.O[entryIndex] - stop) / atr

	admitted := broker{series: series, costs: Costs{StartEquity: 10_000}, params: dailyFlushFailureTestParams(rawBoundary)}
	if trades := admitted.run(); len(trades) != 1 {
		t.Fatalf("inclusive boundary trades = %d, want 1", len(trades))
	}
	rejected := broker{series: series, costs: Costs{StartEquity: 10_000, Slippage: 0.01}, params: dailyFlushFailureTestParams(rawBoundary)}
	if trades := rejected.run(); len(trades) != 0 || rejected.realized != 0 {
		t.Fatalf("slipped boundary trades=%d realized=%v, want 0/0", len(trades), rejected.realized)
	}
}

func TestDailyFlushFailureEntryBarStopAndAdverseGapOrdering(t *testing.T) {
	base := dailyFlushFailureSeries()
	stop, _ := dailyFlushFailureSignalRisk(base)
	const entryIndex = 27

	entryBar := base
	entryBar.L = append([]float64(nil), base.L...)
	entryBar.L[entryIndex] = stop - 0.5
	entryStopped := broker{series: entryBar, costs: Costs{StartEquity: 10_000, Slippage: 0.1}, params: dailyFlushFailureTestParams(3)}
	entryTrade := entryStopped.run()[0]
	if entryTrade.ExitIndex != entryTrade.EntryIndex || math.Abs(entryTrade.Exit-(stop-0.1)) > 1e-12 {
		t.Fatalf("entry-bar stop trade = %+v", entryTrade)
	}

	gap := base
	gap.O = append([]float64(nil), base.O...)
	gap.H = append([]float64(nil), base.H...)
	gap.L = append([]float64(nil), base.L...)
	gap.C = append([]float64(nil), base.C...)
	gap.O[entryIndex+1] = stop - 2
	gap.H[entryIndex+1] = stop - 1
	gap.L[entryIndex+1] = stop - 3
	gap.C[entryIndex+1] = stop - 2
	gapStopped := broker{series: gap, costs: Costs{StartEquity: 10_000, Slippage: 0.1}, params: dailyFlushFailureTestParams(3)}
	gapTrade := gapStopped.run()[0]
	wantExit := gap.O[entryIndex+1] - 0.1
	if gapTrade.ExitIndex != entryIndex+1 || math.Abs(gapTrade.Exit-wantExit) > 1e-12 {
		t.Fatalf("gap stop trade = %+v, want exit %v", gapTrade, wantExit)
	}
}

func TestDailyFlushFailureTimeExitPrecedesStopAtEPlusTenOpen(t *testing.T) {
	series := dailyFlushFailureSeries()
	baseline := broker{series: series, costs: Costs{StartEquity: 10_000}, params: dailyFlushFailureTestParams(3)}
	baselineTrade := baseline.run()[0]
	exitIndex := baselineTrade.ExitIndex
	series.O = append([]float64(nil), series.O...)
	series.H = append([]float64(nil), series.H...)
	series.L = append([]float64(nil), series.L...)
	series.C = append([]float64(nil), series.C...)
	series.O[exitIndex] = baselineTrade.InitialSL - 2
	series.H[exitIndex] = baselineTrade.InitialSL - 1
	series.L[exitIndex] = baselineTrade.InitialSL - 3
	series.C[exitIndex] = baselineTrade.InitialSL - 2

	runner := broker{series: series, costs: Costs{StartEquity: 10_000}, params: dailyFlushFailureTestParams(3)}
	trade := runner.run()[0]
	if trade.Reason != "time" || trade.Exit != series.O[exitIndex] {
		t.Fatalf("E+10 priority trade = %+v", trade)
	}
}

func TestDailyFlushFailureOpenAtEndUsesGenericFinalRawClose(t *testing.T) {
	series := dailyFlushFailureSeries()
	series.T = series.T[:34]
	series.O = series.O[:34]
	series.H = series.H[:34]
	series.L = series.L[:34]
	series.C = series.C[:34]
	series.V = series.V[:34]
	if time.UnixMilli(int64(series.T[len(series.T)-1])).UTC().Weekday() != time.Sunday {
		t.Fatal("fixture must end on Sunday")
	}
	runner := broker{series: series, costs: Costs{StartEquity: 10_000}, params: dailyFlushFailureTestParams(3)}
	trade := runner.run()[0]
	if trade.Reason != "eod" || trade.ExitIndex != series.Len()-1 || trade.Exit != series.C[series.Len()-1] {
		t.Fatalf("open-at-end trade = %+v", trade)
	}
}
