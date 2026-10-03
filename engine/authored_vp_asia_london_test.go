package engine

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

type authoredVPWholeFixture struct {
	Schema     string `json:"schema"`
	Provenance struct {
		StrategyID          string `json:"strategyId"`
		StrategySHA256      string `json:"strategySha256"`
		InheritedBaseSHA256 string `json:"inheritedBaseSha256"`
		EngineSHA256        string `json:"engineSha256"`
		Generator           string `json:"generator"`
	} `json:"provenance"`
	Bars              []marketdata.Bar          `json:"bars"`
	Expected          []authoredVPExpectedTrade `json:"expected"`
	ExpectedRealistic []authoredVPExpectedTrade `json:"expectedRealistic"`
}

type authoredVPExpectedTrade struct {
	Side       string    `json:"side"`
	Entry      float64   `json:"entry"`
	Exit       float64   `json:"exit"`
	SL         float64   `json:"sl"`
	TP         float64   `json:"tp"`
	Size       float64   `json:"size"`
	EntryIndex int       `json:"entryIndex"`
	ExitIndex  int       `json:"exitIndex"`
	EntryT     float64   `json:"entryT"`
	ExitT      float64   `json:"exitT"`
	Reason     string    `json:"reason"`
	Tag        string    `json:"tag"`
	PnL        float64   `json:"pnl"`
	Meta       TradeMeta `json:"meta"`
}

func readAuthoredVPWholeFixture(t *testing.T) authoredVPWholeFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/asia-london-wide-whole-strategy.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture authoredVPWholeFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Schema != "heisentick-strat/authored-js-vp-asia-london-wide/v1" || fixture.Provenance.StrategyID != authoredVPAsiaLondonWideID || fixture.Provenance.StrategySHA256 != "cf34cc972b04781df3eca374417eed0bc71a1af2aabecd31f265a3e2d58ff478" || fixture.Provenance.InheritedBaseSHA256 != "adb1b4ec86a32c9114c449c0f53a10139382fc93c7e48f573e647548778eaddb" || fixture.Provenance.EngineSHA256 != "d7f756252762bfce4866f2481c4e93c079097dee7599079ab3d5e082bd2e1204" || fixture.Provenance.Generator != "engine/testdata/asia-london-wide-oracle.mjs" {
		t.Fatal("missing authored JS oracle provenance")
	}
	return fixture
}

func TestAuthoredVPAsiaLondonWideMatchesWholeJavaScriptStrategy(t *testing.T) {
	fixture := readAuthoredVPWholeFixture(t)
	for _, testCase := range []struct {
		name     string
		costs    Costs
		expected []authoredVPExpectedTrade
	}{
		{"raw", Costs{}, fixture.Expected}, {"realistic", Costs{Slippage: 0.06}, fixture.ExpectedRealistic},
	} {
		t.Run(testCase.name, func(t *testing.T) { compareAuthoredVPWholeStrategy(t, fixture.Bars, testCase.costs, testCase.expected) })
	}
}

func compareAuthoredVPWholeStrategy(t *testing.T, bars []marketdata.Bar, costs Costs, expected []authoredVPExpectedTrade) {
	t.Helper()
	result, err := RunAuthoredVPAsiaLondonWide(bars, "XAUUSD", "5m", costs)
	if err != nil {
		t.Fatal(err)
	}
	if result.StrategyID != authoredVPAsiaLondonWideID || len(result.Trades) != len(expected) || len(result.Trades) != 3 {
		t.Fatalf("result identity/trade count = %s/%d, want %s/3", result.StrategyID, len(result.Trades), authoredVPAsiaLondonWideID)
	}
	for i, want := range expected {
		got := result.Trades[i]
		reason := got.Reason
		if reason == ReasonRule {
			reason = got.Rule
		}
		if got.Side != want.Side || got.EntryIndex != want.EntryIndex || got.ExitIndex != want.ExitIndex || reason != want.Reason || got.Tag != want.Tag {
			t.Errorf("trade %d identity = %+v, want %+v", i, got, want)
		}
		for name, pair := range map[string][2]float64{
			"entry": {got.Entry, want.Entry}, "exit": {got.Exit, want.Exit}, "sl": {got.SL, want.SL},
			"tp": {got.TP, want.TP}, "size": {got.Size, want.Size}, "pnl": {got.PnL, want.PnL},
			"entry timestamp": {got.EntryT, want.EntryT}, "exit timestamp": {got.ExitT, want.ExitT},
		} {
			if math.Abs(pair[0]-pair[1]) > 1e-9 {
				t.Errorf("trade %d %s = %.12g, want %.12g", i, name, pair[0], pair[1])
			}
		}
		for key, value := range want.Meta {
			actual, present := got.Meta[key]
			if !present {
				t.Errorf("trade %d missing meta %s", i, key)
				continue
			}
			if number, ok := value.(float64); ok {
				actualNumber, ok := actual.(float64)
				if !ok {
					if n, intOK := actual.(int); intOK {
						actualNumber, ok = float64(n), true
					}
				}
				if !ok || math.Abs(actualNumber-number) > 1e-9 {
					t.Errorf("trade %d meta %s = %v, want %v", i, key, actual, value)
				}
			} else if actual != value {
				t.Errorf("trade %d meta %s = %v, want %v", i, key, actual, value)
			}
		}
	}
}

func TestAuthoredVPAsiaLondonWideFailsClosed(t *testing.T) {
	fixture := readAuthoredVPWholeFixture(t)
	for _, tc := range []struct {
		name              string
		bars              []marketdata.Bar
		symbol, timeframe string
		costs             Costs
		message           string
	}{
		{"other symbol", fixture.Bars, "XAGUSD", "5m", Costs{}, "XAUUSD 5m only"},
		{"other timeframe", fixture.Bars, "XAUUSD", "1m", Costs{}, "XAUUSD 5m only"},
		{"unsupported fill", fixture.Bars, "XAUUSD", "5m", Costs{FillOn: "open"}, "requires close fills"},
		{"empty", nil, "XAUUSD", "5m", Costs{}, "requires bars"},
		{"insufficient warmup", fixture.Bars[:4], "XAUUSD", "5m", Costs{}, "produced no trades"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := RunAuthoredVPAsiaLondonWide(tc.bars, tc.symbol, tc.timeframe, tc.costs)
			if err == nil || !strings.Contains(err.Error(), tc.message) || result.TradeCount != 0 {
				t.Fatalf("result = %+v, err = %v, want %q", result, err, tc.message)
			}
		})
	}
}
