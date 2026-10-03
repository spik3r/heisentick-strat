package engine

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Snapshot sources under testdata mirror the app's active source catalog at
// 2026-10-03. The editor source is DEFAULT_SPEC_TEXT in specStrategy.js.
// Fixture sources are byte-for-byte copies of their named app strategies.
func TestInteractiveOrdinaryWholeStrategies(t *testing.T) {
	cases := []struct {
		id, fixture, source, symbol, timeframe string
		wantTrades                             bool
	}{
		{"dslSmaGoldenCrossXauusdOneMinuteCanary", "deployed-dsl-sma-golden-cross-xauusd-one-minute-canary", "deployed-dsl-sma-golden-cross-xauusd-one-minute-canary", "XAUUSD", "1m", true},
		{"dslCloseVwapExtremeMagnetDefensive", "deployed-dsl-close-vwap-extreme-magnet-defensive", "deployed-dsl-close-vwap-extreme-magnet-defensive", "XAUUSD", "15m", true},
		{"dslGoldNamedLevelFlagBodyHalfAtr", "family-named-level-flag", "dslGoldNamedLevelFlagBodyHalfAtr", "XAUUSD", "5m", true},
		{"dslGoldNamedLevelFlagBodyHalfAtrRiskFloor12", "family-named-level-flag", "dslGoldNamedLevelFlagBodyHalfAtrRiskFloor12", "XAUUSD", "5m", true},
		{"dslForwardTesterCanaryOrbXauusdFiveMinute", "research-dsl-failed-breakout-five-minute-early-breakeven", "dslForwardTesterCanaryOrbXauusdFiveMinute", "XAUUSD", "5m", true},
		{"dslForwardTesterCanaryOrbBtcusdFiveMinute", "research-dsl-failed-breakout-five-minute-early-breakeven", "dslForwardTesterCanaryOrbBtcusdFiveMinute", "BTCUSD", "5m", true},
		{"dslEditorStrategy", "research-dsl-failed-breakout-five-minute-early-breakeven", "dslEditorStrategy", "XAUUSD", "5m", true},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			fixturePath := filepath.Join("..", "conformance", "run", tc.fixture+".fixture.json")
			raw, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			var base RunFixture
			if err := json.Unmarshal(raw, &base); err != nil {
				t.Fatal(err)
			}
			base.StrategyID = tc.id
			base.Symbol = tc.symbol
			if tc.fixture == "family-named-level-flag" {
				// The deployed known-level target needs room above the PDH.
				// Supply one completed prior-week high while retaining the
				// conformance fixture's PDH impulse and flag geometry.
				priorWeek := float64(time.Date(2023, time.December, 25, 0, 0, 0, 0, time.UTC).UnixMilli())
				base.RawBars = append([][]float64{{priorWeek, 100, 110, 95, 100, 1}}, base.RawBars...)
			}
			if tc.id == "dslForwardTesterCanaryOrbBtcusdFiveMinute" {
				// Reuse frozen five-minute prices as synthetic BTCUSD bars and
				// shift the start from Monday to Saturday to exercise weekend
				// UTC-slot decisions without claiming historical BTC prices.
				for _, bar := range base.RawBars {
					bar[0] += float64(5 * 24 * time.Hour.Milliseconds())
				}
			}
			raw, err = json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join("..", "conformance", "run", tc.source+".strat")
			if tc.source != tc.fixture {
				sourcePath = filepath.Join("testdata", "interactive", tc.source+".strat")
			}
			source, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			fixture, err := DecodeRunFixture(raw)
			if err != nil {
				t.Fatal(err)
			}
			if fixture.Symbol != tc.symbol || fixture.Timeframe != tc.timeframe {
				t.Fatalf("fixture route %s %s", fixture.Symbol, fixture.Timeframe)
			}
			if fixture.StrategyID != tc.id {
				t.Fatalf("fixture ID %q, want %q", fixture.StrategyID, tc.id)
			}
			result, err := RunInteractiveFixture(raw, string(source))
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := RunFixtureCase(fixture, string(source))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Run, legacy) {
				t.Fatal("interactive trade envelope differs from ordinary fixture run")
			}
			if tc.wantTrades && result.Run.TradeCount == 0 {
				t.Fatal("whole-strategy fixture produced no trades")
			}
			t.Logf("route=%s/%s trades=%d skips=%v", fixture.Symbol, fixture.Timeframe, result.Run.TradeCount, result.Skips)
			if tc.id == "dslForwardTesterCanaryOrbBtcusdFiveMinute" {
				weekendEntry := false
				for _, trade := range result.Run.Trades {
					day := time.UnixMilli(int64(trade.EntryT)).UTC().Weekday()
					weekendEntry = weekendEntry || day == time.Saturday || day == time.Sunday
				}
				if !weekendEntry {
					t.Fatal("BTCUSD UTC-slot canary did not exercise a weekend entry")
				}
			}
			if result.Schema != InteractiveRunSchema || result.SkipDiagnostics != "measured" || result.SkipReasonSchema != InteractiveSkipReasonSchema || result.Skips == nil {
				t.Fatalf("incomplete interactive envelope: %+v", result)
			}
			if len(result.EquityCurve) != len(fixture.Bars) || len(result.ClosedEquity) != len(fixture.Bars) || len(result.TradeNetPnL) != result.Run.TradeCount {
				t.Fatal("incomplete per-bar or trade accounting")
			}
			if !isFiniteDerivedOutput(result.CashEndEquity) || math.Abs(result.Stats.EndEquity-result.CashEndEquity) > 1e-9 {
				t.Fatal("invalid terminal cash")
			}
			for i := range result.EquityCurve {
				if !isFiniteDerivedOutput(result.EquityCurve[i]) || !isFiniteDerivedOutput(result.ClosedEquity[i]) {
					t.Fatalf("nonfinite mark at bar %d", i)
				}
			}
			// A parsed source with an out-of-route timeframe must fail, never look like a
			// successful zero-trade result. Preserve every other fixture input.
			var changed RunFixture
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			changed.Timeframe = "1d"
			badRaw, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = RunInteractiveFixture(badRaw, string(source)); !errors.Is(err, ErrInteractiveUnsupported) {
				t.Fatalf("off-route error = %v", err)
			}
			if tc.id != "dslEditorStrategy" {
				changed.Timeframe = fixture.Timeframe
				changed.Symbol = "OUTOFROUTE"
				badRaw, err = json.Marshal(changed)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = RunInteractiveFixture(badRaw, string(source)); !errors.Is(err, ErrInteractiveUnsupported) {
					t.Fatalf("wrong-symbol route error = %v", err)
				}
			}
			if tc.id == "dslEditorStrategy" {
				mutated := strings.Replace(string(source), "by 0.1 ATR", "by 0.2 ATR", 1)
				if mutated == string(source) {
					t.Fatal("editor mutation did not change source")
				}
				if _, err := RunInteractiveFixture(raw, mutated); !errors.Is(err, ErrInteractiveUnsupported) {
					t.Fatalf("mutated editor source inherited default capability: %v", err)
				}
			}
		})
	}
}

func TestInteractiveOrdinaryHTFPrefixesAndFees(t *testing.T) {
	for _, name := range []string{"deployed-dsl-close-vwap-extreme-magnet-defensive", "family-opening-range-breakout"} {
		t.Run(name, func(t *testing.T) {
			base := filepath.Join("..", "conformance", "run", name)
			raw, err := os.ReadFile(base + ".fixture.json")
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(base + ".strat")
			if err != nil {
				t.Fatal(err)
			}
			full, err := RunInteractiveFixture(raw, string(source))
			if err != nil {
				t.Fatal(err)
			}
			var fixture RunFixture
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			fixture.RawBars = fixture.RawBars[:len(fixture.RawBars)/2]
			lastChartOpen := fixture.RawBars[len(fixture.RawBars)-1][0]
			htfCount := 0
			for htfCount < len(fixture.RawHTFBars) && fixture.RawHTFBars[htfCount][0] <= lastChartOpen {
				htfCount++
			}
			fixture.RawHTFBars = fixture.RawHTFBars[:htfCount]
			prefixRaw, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			prefix, err := RunInteractiveFixture(prefixRaw, string(source))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(prefix.EquityCurve, full.EquityCurve[:len(prefix.EquityCurve)]) ||
				!reflect.DeepEqual(prefix.ClosedEquity, full.ClosedEquity[:len(prefix.ClosedEquity)]) ||
				!reflect.DeepEqual(prefix.Skips, full.Skips) && name == "deployed-dsl-close-vwap-extreme-magnet-defensive" {
				t.Fatal("future chart/HTF suffix changed prior marks or VWAP skip counts")
			}
			// A supplied but mislabeled HTF series is never silently projected.
			fixture.HigherTimeframe = "4h"
			bad, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = RunInteractiveFixture(bad, string(source)); !errors.Is(err, ErrInteractiveUnsupported) {
				t.Fatalf("mislabeled HTF error = %v", err)
			}
		})
	}
	base := filepath.Join("..", "conformance", "run", "deployed-dsl-close-vwap-extreme-magnet-defensive")
	raw, err := os.ReadFile(base + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(base + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	withoutFee, err := RunInteractiveFixture(raw, string(source))
	if err != nil {
		t.Fatal(err)
	}
	var fixture RunFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Costs.FeePerUnit = .1
	withFeeRaw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	withFee, err := RunInteractiveFixture(withFeeRaw, string(source))
	if err != nil {
		t.Fatal(err)
	}
	if withFee.Run.TradeCount != withoutFee.Run.TradeCount || withFee.CashEndEquity >= withoutFee.CashEndEquity || withFee.TradeNetPnL[0] >= withoutFee.TradeNetPnL[0] {
		t.Fatalf("fee-inclusive VWAP outcomes did not decrease: cash %.8f -> %.8f, first net %.8f -> %.8f", withoutFee.CashEndEquity, withFee.CashEndEquity, withoutFee.TradeNetPnL[0], withFee.TradeNetPnL[0])
	}
}
