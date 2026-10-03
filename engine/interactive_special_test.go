package engine

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const activeSourceFVG = `dsl v7
strategy "ICT 2TF FVG XAUUSD 1h Source 5m Entry 0.618" {
  description "Completed 1h FVG zone tap, sweep-first displacement gate, causal 5m execution, and a 12-entry-bar 0.618 limit."
}
market conditions {
  slices(XAUUSD 5m)
  sessions(asia, london, ny)
}
setup {
  type: fair value gap
  source timeframe 1h
  gap minimum 0.08 ATR
  displacement minimum 0.8 ATR
  retest within 18 candles
  entry at proximal edge
}
filters { side both }
risk { stop size min 0.3 max 2 }
target { target 2R }
management {
  move stop to breakeven after 1 R plus 0.05 ATR
  wait 6 candles after trade
}
execution { risk 200 USD }
entry { entryTf 5m }
`

func readInteractiveFixturePair(t *testing.T, stem string) ([]byte, string) {
	t.Helper()
	base := filepath.Join("..", "conformance", "run", stem)
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	return raw, string(source)
}

func assertInteractiveTradeParity(t *testing.T, raw []byte, source string, wantSchema string) InteractiveRunResult {
	t.Helper()
	result, err := RunInteractiveFixture(raw, source)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := RunFixtureCase(fixture, source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Run, legacy) || result.Run.TradeCount == 0 {
		t.Fatalf("interactive trade envelope differs or is empty: got %d, legacy %d", result.Run.TradeCount, legacy.TradeCount)
	}
	if len(result.EquityCurve) != len(fixture.Bars) || len(result.ClosedEquity) != len(fixture.Bars) ||
		result.Skips == nil || result.SkipReasonSchema != wantSchema || result.Stats.EndEquity != result.CashEndEquity ||
		len(result.TradeNetPnL) != result.Run.TradeCount {
		t.Fatalf("incomplete interactive result: marks %d/%d, skips %v, schema %q", len(result.EquityCurve),
			len(result.ClosedEquity), result.Skips, result.SkipReasonSchema)
	}
	return result
}

func TestInteractiveSpecialProfilesWholeStrategyParity(t *testing.T) {
	for _, stem := range []string{
		"family-daily-flush-failure",
		"deployed-dsl-weekend-extreme-fade-btcusdt-four-hour",
	} {
		t.Run(stem, func(t *testing.T) {
			raw, source := readInteractiveFixturePair(t, stem)
			var fixture RunFixture
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			fixture.Costs.FeePerUnit = 0.02
			raw, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			result := assertInteractiveTradeParity(t, raw, source, InteractiveSkipReasonSchema)
			if result.SkipDiagnostics != "measured" {
				t.Fatalf("skip diagnostics = %q", result.SkipDiagnostics)
			}
			cash := fixture.Costs.StartEquity
			for _, trade := range result.Run.Trades {
				cash += trade.PnL - fixture.Costs.FeePerUnit*trade.Size
			}
			if math.Abs(cash-result.CashEndEquity) > 1e-8 {
				t.Fatalf("cash end = %v, fee inclusive trades = %v", result.CashEndEquity, cash)
			}
			if stem == "family-daily-flush-failure" {
				for i := 1; i < len(fixture.RawBars); i++ {
					day := time.UnixMilli(int64(fixture.RawBars[i][0])).UTC().Weekday()
					if day == time.Saturday || day == time.Sunday {
						if result.EquityCurve[i] != result.EquityCurve[i-1] || result.ClosedEquity[i] != result.ClosedEquity[i-1] {
							t.Fatalf("weekend bar %d changed retained equity", i)
						}
					}
				}
			}
		})
	}
}

func TestInteractiveWeekendExtremeUnreachableFailsClosed(t *testing.T) {
	raw, source := readInteractiveFixturePair(t, "deployed-dsl-weekend-extreme-fade-btcusdt-four-hour")
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	weekdays := fixture.RawBars[:0]
	for _, bar := range fixture.RawBars {
		day := time.UnixMilli(int64(bar[0])).UTC().Weekday()
		if day != time.Saturday && day != time.Sunday {
			weekdays = append(weekdays, bar)
		}
	}
	fixture.RawBars = weekdays
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunInteractiveFixture(raw, source); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("unreachable weekend setup must fail closed: %v", err)
	}
}

func sourceFVGInteractiveFixture(t *testing.T) ([]byte, string) {
	t.Helper()
	baseRaw, _ := readInteractiveFixturePair(t, "family-fair-value-gap")
	fixture, err := DecodeRunFixture(baseRaw)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Case = "interactive-source-fvg-1h-5m"
	fixture.StrategyID = "ict2tfFvgXauusd1h5m618"
	fixture.Symbol = "XAUUSD"
	fixture.Timeframe = "5m"
	fixture.SourceTimeframe = "1h"
	fixture.RawSourceBars = fixture.RawBars
	fixture.RawSourceBars[23][3] = 103 // narrow the completed FVG to fit the active stop bound
	fixture.RawBars = make([][]float64, 0, len(fixture.RawSourceBars)*12)
	for _, bar := range fixture.RawSourceBars {
		for step := 0; step < 12; step++ {
			fixture.RawBars = append(fixture.RawBars, []float64{bar[0] + float64(step)*300_000, bar[4], bar[2], bar[3], bar[4], bar[5]})
		}
	}
	fixture.Costs.FeePerUnit = 0.01
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return raw, activeSourceFVG
}

func TestInteractiveSourceFVGWholeStrategyParityAndCompletedSourcePrefix(t *testing.T) {
	raw, source := sourceFVGInteractiveFixture(t)
	full := assertInteractiveTradeParity(t, raw, source, InteractiveSourceSkipReasonSchema)
	if full.SkipDiagnostics != "measured-source" {
		t.Fatalf("skip diagnostics = %q", full.SkipDiagnostics)
	}
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.RawBars = fixture.RawBars[:305]
	fixture.RawSourceBars = fixture.RawSourceBars[:25]
	prefixRaw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := RunInteractiveFixture(prefixRaw, source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prefix.EquityCurve, full.EquityCurve[:305]) ||
		!reflect.DeepEqual(prefix.ClosedEquity, full.ClosedEquity[:305]) {
		t.Fatal("chart/source prefix changed earlier marks")
	}
	fixture.RawSourceBars[24][1], fixture.RawSourceBars[24][2], fixture.RawSourceBars[24][3], fixture.RawSourceBars[24][4] = 107, 108, 106, 107
	mutatedRaw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	mutated, err := RunInteractiveFixture(mutatedRaw, source)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(mutated.EquityCurve, prefix.EquityCurve) && reflect.DeepEqual(mutated.Run.Trades, prefix.Run.Trades) {
		t.Fatal("completed source-bar mutation did not affect execution")
	}
	fixture.RawSourceHTFBars = [][]float64{fixture.RawSourceBars[0], fixture.RawSourceBars[1]}
	unsupportedRaw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunInteractiveFixture(unsupportedRaw, source); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("undeclared source HTF must fail closed: %v", err)
	}
	fixture.RawSourceHTFBars = nil
	fixture.RawSourceBars[1][0] += 1 // duration inference must not accept an off-grid source bar
	misalignedRaw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunInteractiveFixture(misalignedRaw, source); !errors.Is(err, ErrInteractiveUnsupported) {
		t.Fatalf("misaligned source series must fail closed: %v", err)
	}
}
