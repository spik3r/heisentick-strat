package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestExecutionWindowKeepsIndicatorContextButStartsTradingAtBoundary(t *testing.T) {
	series := marketdata.Series{
		T: []float64{0, 1, 2, 3, 4, 5, 6},
		O: []float64{3, 2, 1, 4, 5, 3, 6},
		H: []float64{3, 2, 1, 4, 5, 3, 6},
		L: []float64{3, 2, 1, 4, 3, 0, 2},
		C: []float64{3, 2, 1, 4, 3, 0, 2},
		V: []float64{1, 1, 1, 1, 1, 1, 1},
	}
	cfg := dsl.Config{
		"setupType": "smaGoldenCross",
		"allowLong": 1,
		"smaGoldenCross": dsl.Config{
			"fastSmaLen": 2,
			"slowSmaLen": 3,
		},
	}
	from, to := int64(3), int64(7)
	request := RunRequest{
		Config: cfg, Series: series, StrategyID: "window-test", Symbol: "TEST", Timeframe: "4h",
		ExecutionWindow: &ExecutionWindow{TradeFromT: &from, TradeToT: &to},
		Costs:           Costs{FillOn: "close", SlippageBps: 10},
	}
	result, err := Run(request)
	if err != nil {
		t.Fatalf("windowed run: %v", err)
	}
	if len(result.Trades) != 1 {
		t.Fatalf("trades = %+v, want one context-dependent trade", result.Trades)
	}
	trade := result.Trades[0]
	if trade.EntryIndex < 3 || trade.EntryT < float64(from) {
		t.Fatalf("trade entered before boundary: %+v", trade)
	}
	if trade.EntryIndex != 4 || trade.ExitIndex != 6 {
		t.Fatalf("windowed execution = %+v, want entry 4 and exit 6", trade)
	}
	if trade.PnL == 0 {
		t.Fatalf("windowed trade has no realized PnL: %+v", trade)
	}
}

func TestResolveExecutionWindowRejectsNonExactOrReversedBounds(t *testing.T) {
	series := marketdata.Series{T: []float64{10, 20, 30}, O: []float64{1, 1, 1}, H: []float64{2, 2, 2}, L: []float64{0, 0, 0}, C: []float64{1, 1, 1}}
	missing := int64(15)
	if _, err := ResolveExecutionWindow(series, &ExecutionWindow{TradeFromT: &missing}); err == nil {
		t.Fatal("missing trade boundary was accepted")
	}
	from := int64(10)
	badEnd := int64(25)
	if _, err := ResolveExecutionWindow(series, &ExecutionWindow{TradeFromT: &from, TradeToT: &badEnd}); err == nil {
		t.Fatal("non-boundary exclusive endpoint was accepted")
	}
	reversedFrom, reversedTo := int64(30), int64(20)
	if _, err := ResolveExecutionWindow(series, &ExecutionWindow{TradeFromT: &reversedFrom, TradeToT: &reversedTo}); err == nil {
		t.Fatal("reversed trade window was accepted")
	}
	lastEndpoint := int64(40)
	bounds, err := ResolveExecutionWindow(series, &ExecutionWindow{TradeFromT: &from, TradeToT: &lastEndpoint})
	if err != nil || bounds.TradeEnd != 2 {
		t.Fatalf("next-bar exclusive endpoint = %+v, err=%v; want final supplied bar", bounds, err)
	}
	context := int64(20)
	if _, err := ResolveExecutionWindow(series, &ExecutionWindow{ContextFromT: &context}); err == nil {
		t.Fatal("non-first context boundary was accepted")
	}
}

func TestResolveExecutionWindowRequiresOptInForGapfulExclusiveEnd(t *testing.T) {
	series := marketdata.Series{
		T: []float64{10, 20, 30},
		O: []float64{1, 1, 1}, H: []float64{2, 2, 2},
		L: []float64{0, 0, 0}, C: []float64{1, 1, 1},
	}
	from, gapfulEnd := int64(10), int64(50)
	strict := &ExecutionWindow{TradeFromT: &from, TradeToT: &gapfulEnd}
	if _, err := ResolveExecutionWindow(series, strict); err == nil {
		t.Fatal("gapful exclusive endpoint was accepted without explicit source proof")
	}

	verified := &ExecutionWindow{TradeFromT: &from, TradeToT: &gapfulEnd, AllowGapfulTradeToT: true}
	bounds, err := ResolveExecutionWindow(series, verified)
	if err != nil {
		t.Fatalf("verified gapful exclusive endpoint: %v", err)
	}
	if bounds.TradeStart != 0 || bounds.TradeEnd != 2 {
		t.Fatalf("bounds = %+v, want all supplied bars inside [10,50)", bounds)
	}

	missingInside := int64(25)
	verified.TradeToT = &missingInside
	if _, err := ResolveExecutionWindow(series, verified); err == nil {
		t.Fatal("missing endpoint inside supplied series was accepted")
	}
}

func TestSharedContextKeySeparatesExecutionWindows(t *testing.T) {
	series := marketdata.Series{
		T: []float64{0, 1, 2, 3},
		O: []float64{1, 1, 1, 1}, H: []float64{2, 2, 2, 2},
		L: []float64{0, 0, 0, 0}, C: []float64{1, 1, 1, 1},
	}
	cfg := dsl.Config{"setupType": "smaGoldenCross", "smaGoldenCross": dsl.Config{"fastSmaLen": 2, "slowSmaLen": 3}}
	fromA, fromB := int64(1), int64(2)
	base := RunRequest{Config: cfg, Series: series, StrategyID: "key-test", Symbol: "TEST", Timeframe: "4h"}
	requestA, requestB := base, base
	requestA.ExecutionWindow = &ExecutionWindow{TradeFromT: &fromA}
	requestB.ExecutionWindow = &ExecutionWindow{TradeFromT: &fromB}
	keyA, err := SharedContextKey(requestA)
	if err != nil {
		t.Fatalf("key A: %v", err)
	}
	keyB, err := SharedContextKey(requestB)
	if err != nil {
		t.Fatalf("key B: %v", err)
	}
	if keyA == keyB {
		t.Fatal("shared context keys reused across different execution windows")
	}
}

func TestDailyFlushFailureDropsPreWindowPendingSignal(t *testing.T) {
	series := dailyFlushFailureSeries()
	start := int64(series.T[27])
	b := broker{series: series, costs: Costs{FillOn: "close", StartEquity: 10_000}, params: dailyFlushFailureTestParams(3)}
	b.setExecutionWindow(ExecutionBounds{TradeStart: 27, TradeEnd: series.Len() - 1})
	for _, trade := range b.run() {
		if trade.EntryT == float64(start) {
			t.Fatalf("pre-window pending signal entered at first trade bar: %+v", trade)
		}
	}
}
