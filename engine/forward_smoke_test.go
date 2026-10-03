package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

type smokeOracle struct {
	StrategyID      string           `json:"strategyId"`
	StrategyVersion string           `json:"strategyVersion"`
	Symbol          string           `json:"symbol"`
	Timeframe       string           `json:"timeframe"`
	Costs           Costs            `json:"costs"`
	Bars            []marketdata.Bar `json:"bars"`
	WholeTrades     []map[string]any `json:"wholeTrades"`
	Prefix          []struct {
		Length        int              `json:"length"`
		Trades        []map[string]any `json:"trades"`
		OpenPositions []map[string]any `json:"openPositions"`
	} `json:"prefix"`
}

func smokeVersion(id string) string {
	if id == ForwardSmokeBTCID {
		return ForwardSmokeBTCVersion
	}
	return ForwardSmokeAnyVersion
}

func TestForwardSmokeFrozenJSWholeTradesAndPrefixes(t *testing.T) {
	files, err := filepath.Glob("testdata/forward-smoke/*.json")
	if err != nil || len(files) != 8 {
		t.Fatalf("oracle files = %v, %v", files, err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var oracle smokeOracle
			if err := json.Unmarshal(raw, &oracle); err != nil {
				t.Fatal(err)
			}
			if oracle.StrategyVersion != smokeVersion(oracle.StrategyID) {
				t.Fatalf("frozen source version %q does not match Go port", oracle.StrategyVersion)
			}
			request := ForwardSmokeRequest{StrategyID: oracle.StrategyID, StrategyVersion: smokeVersion(oracle.StrategyID), Symbol: oracle.Symbol, Timeframe: oracle.Timeframe, Series: marketdata.SeriesFromBars(oracle.Bars), Costs: oracle.Costs}
			result, err := RunForwardSmoke(request)
			if err != nil {
				t.Fatal(err)
			}
			if result.TradeCount == 0 {
				t.Fatal("oracle route silently returned zero trades")
			}
			if filepath.Base(path) == "btc-open-cost.json" {
				wantEntries := []int{1, 2, 6, 8}
				if len(result.Trades) != len(wantEntries) {
					t.Fatalf("open-mode trade count = %d", len(result.Trades))
				}
				for i, entry := range wantEntries {
					if result.Trades[i].EntryIndex != entry {
						t.Fatalf("open-mode trade %d entry index = %d, want %d", i, result.Trades[i].EntryIndex, entry)
					}
				}
			}
			assertSmokeTrades(t, result.Trades, oracle.WholeTrades)
			positionIDs := map[int]string{}
			for _, prefix := range oracle.Prefix {
				prefixRequest := request
				prefixRequest.Series = marketdata.SeriesFromBars(oracle.Bars[:prefix.Length])
				got, err := RunForwardSmokePrefix(prefixRequest)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(got.CheckpointDigest, "sha256:") {
					t.Fatalf("missing prefix digest: %q", got.CheckpointDigest)
				}
				closed := make([]Trade, len(got.Trades))
				for i := range got.Trades {
					closed[i] = got.Trades[i].Trade
					key := closed[i].EntryIndex
					if previous, exists := positionIDs[key]; exists && previous != got.Trades[i].PositionID {
						t.Fatalf("closed position %d changed ID after prefix restart", key)
					}
					positionIDs[key] = got.Trades[i].PositionID
				}
				assertSmokeTrades(t, closed, prefix.Trades)
				if len(got.OpenPositions) != len(prefix.OpenPositions) {
					t.Fatalf("prefix %d open count = %d, want %d", prefix.Length, len(got.OpenPositions), len(prefix.OpenPositions))
				}
				for i, open := range got.OpenPositions {
					want := prefix.OpenPositions[i]
					assertSmokeNumbers(t, "prefix open", map[string]float64{"entry": open.Entry, "sl": open.SL, "tp": open.TP, "size": open.Size, "entryT": open.EntryT, "entryIndex": float64(open.EntryIndex)}, want)
					if open.Side != want["side"] || open.Tag != want["tag"] {
						t.Fatalf("prefix %d open identity = %+v, want %v", prefix.Length, open, want)
					}
					if previous, exists := positionIDs[open.EntryIndex]; exists && previous != open.PositionID {
						t.Fatalf("open position %d changed ID after prefix restart", open.EntryIndex)
					}
					positionIDs[open.EntryIndex] = open.PositionID
				}
			}
		})
	}
}

func assertSmokeTrades(t *testing.T, got []Trade, want []map[string]any) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("trades = %d, want %d", len(got), len(want))
	}
	for i, trade := range got {
		oracle := want[i]
		assertSmokeNumbers(t, "trade", map[string]float64{"entry": trade.Entry, "exit": trade.Exit, "sl": trade.SL, "tp": trade.TP, "initialSl": trade.InitialSL, "initialTp": trade.InitialTP, "size": trade.Size, "points": trade.Points, "pnl": trade.PnL, "entryT": trade.EntryT, "exitT": trade.ExitT, "entryIndex": float64(trade.EntryIndex), "exitIndex": float64(trade.ExitIndex)}, oracle)
		reason := trade.Reason
		if reason == ReasonEndOfTest {
			reason = "eod"
		}
		if trade.Side != oracle["side"] || trade.Tag != oracle["tag"] || reason != oracle["reason"] {
			t.Fatalf("trade %d labels %+v != %v", i, trade, oracle)
		}
		if trade.Meta != nil || oracle["meta"] != nil {
			t.Fatalf("trade %d meta %+v != %v", i, trade.Meta, oracle["meta"])
		}
	}
}

func assertSmokeNumbers(t *testing.T, context string, got map[string]float64, want map[string]any) {
	t.Helper()
	for key, value := range got {
		expected, ok := want[key].(float64)
		if !ok || math.Abs(value-expected) > 1e-8*math.Max(1, math.Abs(expected)) {
			t.Fatalf("%s %s = %.15g, want %v", context, key, value, want[key])
		}
	}
}

func TestForwardSmokeRejectsUnknownVersionAndRoute(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 1, H: 2, L: 1, C: 2}})
	r := ForwardSmokeRequest{StrategyID: ForwardSmokeBTCID, StrategyVersion: ForwardSmokeBTCVersion, Symbol: "BTCUSD", Timeframe: "1m", Series: series}
	for _, mutation := range []func(*ForwardSmokeRequest){
		func(r *ForwardSmokeRequest) { r.StrategyID = "unknown" },
		func(r *ForwardSmokeRequest) { r.StrategyVersion = "sha256:old" },
		func(r *ForwardSmokeRequest) { r.Symbol = "EURUSD" },
		func(r *ForwardSmokeRequest) { r.Timeframe = "5m" },
	} {
		candidate := r
		mutation(&candidate)
		if _, err := RunForwardSmoke(candidate); err == nil {
			t.Fatalf("unsupported route/version succeeded: %+v", candidate)
		}
		if _, err := RunForwardSmokePrefix(candidate); err == nil {
			t.Fatalf("unsupported prefix route/version succeeded: %+v", candidate)
		}
	}
}
