package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSMAGoldenCrossMatchesCommittedConformanceFixture(t *testing.T) {
	const caseName = "family-sma-golden-cross"
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), caseName+".fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".strat"))
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	result, err := RunFixtureCase(fixture, string(source))
	if err != nil {
		t.Fatalf("run fixture: %v", err)
	}
	if result.TradeCount != 13 {
		t.Fatalf("trade count = %d, want 13", result.TradeCount)
	}
	expected, err := os.ReadFile(filepath.Join(runFixtureDir(), caseName+".trades.json"))
	if err != nil {
		t.Fatalf("read expected trades: %v", err)
	}
	if got, want := canonicalJSON(ConformanceProjection(result)), canonicalRawJSON(t, expected); got != want {
		t.Fatalf("trade mismatch\nfirst diff: %s\n got: %s\nwant: %s", firstDiff(got, want), got, want)
	}
}

func TestSMAGoldenCrossCrossesAreEqualityAware(t *testing.T) {
	if !smaBullishCross(10, 10, 11, 10) {
		t.Fatal("bullish cross should accept an equal previous fast/slow pair")
	}
	if !smaBearishCross(10, 10, 9, 10) {
		t.Fatal("bearish cross should accept an equal previous fast/slow pair")
	}
}

func TestSMAGoldenCrossRejectsInvalidProtectedDistances(t *testing.T) {
	tests := []struct {
		name                          string
		signalATR, stopATR, targetATR float64
	}{
		{name: "zero atr", signalATR: 0, stopATR: 2, targetATR: 3},
		{name: "negative atr", signalATR: -1, stopATR: 2, targetATR: 3},
		{name: "infinite stop multiplier", signalATR: 1, stopATR: math.Inf(1), targetATR: 3},
		{name: "stop multiplication overflow", signalATR: 2, stopATR: math.MaxFloat64, targetATR: 3},
		{name: "target multiplication overflow", signalATR: 2, stopATR: 2, targetATR: math.MaxFloat64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if stopDistance, targetDistance, ok := smaProtectedDistances(tt.signalATR, tt.stopATR, tt.targetATR); ok {
				t.Fatalf("distances = %v/%v, want invalid", stopDistance, targetDistance)
			}
		})
	}
}

func TestSMAGoldenCrossQueuesFixedSizeNextOpenTradesWithoutBrackets(t *testing.T) {
	b := broker{
		series: marketdata.Series{
			T: []float64{0, 1, 2, 3, 4, 5, 6},
			O: []float64{3, 2, 1, 4, 5, 3, 6},
			H: []float64{3, 2, 1, 4, 5, 3, 6},
			L: []float64{3, 2, 1, 4, 3, 0, 2},
			C: []float64{3, 2, 1, 4, 3, 0, 2},
			V: []float64{1, 1, 1, 1, 1, 1, 1},
		},
		params: flagParams{
			SetupType: "smaGoldenCross",
			SMAGoldenCross: smaGoldenCrossParams{
				FastLen: 2, SlowLen: 3, AllowLong: true,
			},
		},
		costs: Costs{FillOn: "close"},
	}

	trades := b.run()
	if len(trades) != 1 {
		t.Fatalf("trades = %+v, want one trade", trades)
	}
	trade := trades[0]
	if trade.Side != "long" || trade.Size != 1 || trade.EntryIndex != 4 || trade.Entry != 5 {
		t.Fatalf("entry = %+v, want long size 1 at next-open index 4 price 5", trade)
	}
	if trade.ExitIndex != 6 || trade.Exit != 6 || trade.Reason != "sma-bearish-cross" {
		t.Fatalf("exit = %+v, want bearish-cross next-open exit at index 6 price 6", trade)
	}
	if !trade.NoStop || !trade.NoTarget {
		t.Fatalf("bracket flags = stop %v target %v, want both absent", trade.NoStop, trade.NoTarget)
	}
	if trade.Tag != "DSL-SMA-GC" || trade.Meta["setup"] != "smaGoldenCross" || trade.Meta["signalIndex"] != float64(3) {
		t.Fatalf("trade metadata = %+v, want SMA signal metadata", trade.Meta)
	}

	raw, err := json.Marshal(RunResult{Trades: trades})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var envelope struct {
		Trades []map[string]any `json:"trades"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	for _, key := range []string{"initialSl", "initialTp", "sl", "tp"} {
		if value := envelope.Trades[0][key]; value != nil {
			t.Fatalf("serialized %s = %#v, want null", key, value)
		}
	}
}

func TestSMAGoldenCrossProtectedVariantUsesCurrentBarWilderATR(t *testing.T) {
	series := marketdata.Series{
		T: []float64{0, 1, 2, 3, 4, 5, 6},
		O: []float64{3, 2, 1, 4, 5, 3, 6},
		H: []float64{3.5, 2.5, 1.5, 4.5, 5.5, 3.2, 6.5},
		L: []float64{2.5, 1.5, 0.5, 3.5, 4.5, 2.5, 1.5},
		C: []float64{3, 2, 1, 4, 3, 0, 2},
		V: []float64{1, 1, 1, 1, 1, 1, 1},
	}
	b := broker{
		series: series,
		params: flagParams{SetupType: "smaGoldenCross", SMAGoldenCross: protectedSMAGoldenCrossParams()},
		costs:  Costs{FillOn: "close"},
	}

	for i := 0; i <= 3; i++ {
		b.onSMAGoldenCrossBar(i)
	}
	if len(b.pendingOrders) != 1 {
		t.Fatalf("pending orders = %+v, want one protected next-open order", b.pendingOrders)
	}
	protected := b.pendingOrders[0]
	wantATR := 37.0 / 18.0
	if !protected.HasStopDistance || !protected.HasTargetDistance || protected.NoStop || protected.NoTarget || !protected.GapAwareStop {
		t.Fatalf("protected order flags = %+v, want distances, gap-aware stop, and brackets", protected)
	}
	if math.Abs(protected.StopDistance-2*wantATR) > 1e-12 || math.Abs(protected.TargetDistance-3*wantATR) > 1e-12 {
		t.Fatalf("protected distances = %.15g/%.15g, want %.15g/%.15g", protected.StopDistance, protected.TargetDistance, 2*wantATR, 3*wantATR)
	}
	if protected.Meta["forwardBracketAnchor"] != "entry-fill" || protected.Meta["signalAtr"] != wantATR ||
		protected.Meta["forwardStopDistance"] != protected.StopDistance || protected.Meta["forwardTargetDistance"] != protected.TargetDistance {
		t.Fatalf("protected metadata = %+v, want entry-fill ATR and distances", protected.Meta)
	}

	b = broker{series: series, params: flagParams{SetupType: "smaGoldenCross", SMAGoldenCross: protectedSMAGoldenCrossParams()}, costs: Costs{FillOn: "close"}}
	trades := b.run()
	if len(trades) != 1 {
		t.Fatalf("protected trades = %+v, want one bearish-cross trade", trades)
	}
	trade := trades[0]
	if trade.Entry != 5 || trade.EntryIndex != 4 || trade.Exit != 6 || trade.ExitIndex != 6 || trade.Reason != "sma-bearish-cross" {
		t.Fatalf("protected execution = %+v, want next-open entry/exit", trade)
	}
	if math.Abs(trade.InitialSL-8.0/9.0) > 1e-12 || math.Abs(trade.InitialTP-67.0/6.0) > 1e-12 ||
		math.Abs(trade.SL-8.0/9.0) > 1e-12 || math.Abs(trade.TP-67.0/6.0) > 1e-12 {
		t.Fatalf("protected brackets = sl %.15g/%.15g tp %.15g/%.15g, want entry-fill distances", trade.InitialSL, trade.SL, trade.InitialTP, trade.TP)
	}
}

func protectedSMAGoldenCrossParams() smaGoldenCrossParams {
	return smaGoldenCrossParamsFromConfig(dsl.Config{
		"allowLong": 1,
		"smaGoldenCross": dsl.Config{
			"fastSmaLen": 2, "slowSmaLen": 3,
			"atrLen": 3, "stopAtr": 2, "targetAtr": 3,
		},
	})
}
