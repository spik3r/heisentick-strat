package engine

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestHeikinAshiCausalSeedAndIndependentHTF(t *testing.T) {
	raw := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 10, H: 15, L: 8, C: 12, V: 3},
		{T: 3600000, O: 12, H: 16, L: 9, C: 14, V: 5},
		// A missing time period does not reset the recursive open.
		{T: 10800000, O: 14, H: 20, L: 13, C: 18, V: 7},
	})
	got, err := heikinAshiSeries(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.T, raw.T) || !reflect.DeepEqual(got.V, raw.V) {
		t.Fatalf("provider timestamp/volume changed: %+v", got)
	}
	for i, want := range []marketdata.Bar{
		{T: 0, O: 11, H: 15, L: 8, C: 11.25, V: 3},
		{T: 3600000, O: 11.125, H: 16, L: 9, C: 12.75, V: 5},
		{T: 10800000, O: 11.9375, H: 20, L: 11.9375, C: 16.25, V: 7},
	} {
		if got.Bar(i) != want {
			t.Fatalf("HA bar %d = %+v, want %+v", i, got.Bar(i), want)
		}
	}
	prefix, err := heikinAshiSeries(marketdata.SeriesFromBars(raw.Bars()[:2]))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bar(0) != prefix.Bar(0) || got.Bar(1) != prefix.Bar(1) {
		t.Fatal("future append changed prior calculation bars")
	}
	htf, err := heikinAshiSeries(marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 100, H: 120, L: 90, C: 110, V: 20},
		{T: 14400000, O: 110, H: 125, L: 100, C: 115, V: 30},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if htf.O[0] != 105 || htf.C[0] != 105 || htf.O[1] != 105 {
		t.Fatalf("HTF was not seeded from its own first provider bar: %+v", htf)
	}
}

func TestInteractiveHeikinAshiChangesSMASignalButFillsAndMarksRaw(t *testing.T) {
	// This prefix has an HA bullish cross at decision index 3, while raw
	// closes first cross later. The HA trade fills on the next raw open.
	bars := [][]float64{
		{0, 97.4337173887123, 98.46782213880319, 88.22426462960587, 91.06750421981548, 1},
		{14400000, 89.39901707172075, 91.88724639028254, 87.19833420751091, 88.87218184923867, 1},
		{28800000, 93.2288105408643, 96.93891087894082, 91.06814226564106, 95.93838905659194, 1},
		{43200000, 91.88464648478794, 92.67506510922438, 82.90019574732415, 85.46735762364025, 1},
		{57600000, 85.42731292212541, 86.22482144948039, 84.50580757129003, 84.51574751423136, 1},
		{72000000, 80.0080394367005, 86.90536849001896, 78.5360852839294, 84.87224646011977, 1},
		{86400000, 87.92616788662865, 91.88972964337306, 86.94150465202323, 91.35604667299349, 1},
		{100800000, 91.68421156457433, 93.63455911926134, 83.3611838521956, 84.63033062749777, 1},
	}
	fixture := RunFixture{Schema: runFixtureSchema, Case: "ha-sma", StrategyID: "sma",
		Symbol: "XAUUSD", Timeframe: "4h", RangeMethod: "zone",
		Costs: Costs{FillOn: "nextOpen", Slippage: 0.2, FeePerUnit: 0.1, StartEquity: 10000}, RawBars: bars}
	run := func(f RunFixture) InteractiveRunResult {
		t.Helper()
		encoded, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		result, err := RunInteractiveFixture(encoded, interactiveSMASource)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	standard := run(fixture)
	fixture.CalculationSource = "raw"
	explicitStandard := run(fixture)
	if !reflect.DeepEqual(standard.Run, explicitStandard.Run) ||
		!reflect.DeepEqual(standard.EquityCurve, explicitStandard.EquityCurve) {
		t.Fatal("explicit raw changed the standard baseline")
	}
	fixture.CalculationSource = "heikinAshi"
	ha := run(fixture)
	decoded := fixture
	decoded.Bars, _ = decodeFixtureBars("bars", fixture.RawBars)
	if _, err := RunFixtureCase(decoded, interactiveSMASource); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("legacy fixture interpreter silently ignored HA: %v", err)
	}
	if standard.Run.Trades[0].EntryIndex != 7 || ha.Run.Trades[0].EntryIndex != 4 ||
		math.Abs(ha.Run.Trades[0].Entry-(bars[4][1]+0.2)) > 1e-9 || ha.CalculationSource != "heikinAshi" ||
		ha.CalculationSourceSchema != InteractiveCalculationSourceSchema {
		t.Fatalf("calculation did not alter signal or preserve raw next-open fill: standard=%+v HA=%+v", standard.Run.Trades, ha.Run.Trades)
	}
	entry := ha.Run.Trades[0]
	wantMark := 10000 - 0.1 + (bars[4][4]-entry.Entry)*entry.Size
	if math.Abs(ha.EquityCurve[4]-wantMark) > 1e-9 {
		t.Fatalf("entry mark %.12f, want raw-close mark %.12f", ha.EquityCurve[4], wantMark)
	}
	fixture.RawBars = bars[:6]
	prefix := run(fixture)
	if prefix.Run.Trades[0].EntryIndex != 4 ||
		!reflect.DeepEqual(prefix.EquityCurve, ha.EquityCurve[:6]) ||
		!reflect.DeepEqual(prefix.ClosedEquity, ha.ClosedEquity[:6]) {
		t.Fatal("future bars changed prior HA signal or market marks")
	}
}

func TestInteractiveHeikinAshiBrokerTouchesRawBarsAndGaps(t *testing.T) {
	calc := marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 12, H: 16, L: 8, C: 12, V: 1}, {T: 1, O: 12, H: 16, L: 7, C: 12, V: 1}})
	raw := marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 12, H: 15, L: 10, C: 13, V: 1}, {T: 1, O: 8, H: 12, L: 7, C: 10, V: 1}})
	var b broker
	b.reset(calc, contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{Costs: Costs{StartEquity: 10000, FeePerUnit: 0.25}}, nil)
	b.rawSeries = raw
	b.hasPosition = true
	b.position = position{Side: sideLong, Entry: 12, Size: 1, SL: 9, NoTarget: true, EntryIndex: 0, EntryT: 0}
	b.equityCurve = make([]float64, 2)
	b.cashCurve = make([]float64, 2)
	b.resolveIntrabarExit(0)
	if !b.hasPosition {
		t.Fatal("synthetic low triggered a raw-market stop")
	}
	b.markToMarket(0)
	if b.equityCurve[0] != 10001 || b.cashCurve[0] != 10000 {
		t.Fatalf("market marks = %v/%v", b.equityCurve[0], b.cashCurve[0])
	}
	b.resolveIntrabarExit(1)
	if len(b.trades) != 1 || b.trades[0].Exit != 8 || b.trades[0].Reason != "sl" ||
		b.trades[0].PnL != -4.25 {
		t.Fatalf("raw gap/fee exit = %+v", b.trades)
	}
}

func TestInteractiveHeikinAshiDerivedLevelUsesRawImmediateFill(t *testing.T) {
	base := filepath.Join("..", "conformance", "run", "research-dsl-daily-snd-retest-xauusd-4h")
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	standard, err := RunInteractiveFixture(raw, string(source))
	if err != nil {
		t.Fatal(err)
	}
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.CalculationSource = "heikinAshi"
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	ha, err := RunInteractiveFixture(encoded, string(source))
	if err != nil {
		t.Fatal(err)
	}
	if standard.Run.TradeCount != 43 || ha.Run.TradeCount != 11 ||
		ha.Run.Trades[0].EntryIndex != 278 || ha.Run.Trades[0].Entry != fixture.RawBars[278][4] ||
		standard.Run.Trades[1].EntryIndex != 278 ||
		ha.Run.Trades[0].InitialSL == standard.Run.Trades[1].InitialSL {
		t.Fatalf("HA level/entry distinction failed: standard=%+v HA=%+v", standard.Run.Trades[:2], ha.Run.Trades[:1])
	}
}

func TestInteractiveHeikinAshiHTFAndUnsupportedRoutes(t *testing.T) {
	base := filepath.Join("..", "conformance", "run", "research-dsl-session-bias-seasonal-convergence-quality-fifteen")
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.CalculationSource = "heikinAshi"
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunInteractiveFixture(encoded, string(source))
	if err != nil {
		t.Fatal(err)
	}
	if result.CalculationSource != "heikinAshi" || len(result.EquityCurve) != len(fixture.RawBars) {
		t.Fatalf("HTF HA result = %+v", result)
	}
	fixture.RawSourceBars = [][]float64{{0, 1, 1, 1, 1, 1}}
	encoded, _ = json.Marshal(fixture)
	if _, err := RunInteractiveFixture(encoded, string(source)); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("source route silently accepted HA: %v", err)
	}
	for _, value := range []string{`null`, `"renko"`, `42`, `""`} {
		request := []byte(`{"schema":"dsl-conformance-run-fixture-v1","case":"bad-ha","strategyId":"sma","symbol":"XAUUSD","timeframe":"4h","costs":{},"bars":[[0,1,1,1,1,1]],"calculationSource":` + value + `}`)
		if _, err := RunInteractiveFixture(request, interactiveSMASource); err == nil {
			t.Errorf("accepted calculationSource %s", value)
		}
	}
}
