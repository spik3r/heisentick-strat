package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Generated from the app's sessionBreakHoldOnBar with a shared api.state.
// The first scenario alternates sides inside and outside the 12-bar cooldown;
// the second proves a rejected entry leaves seen/cooldown available for a
// different fresh level on the next bar.
func TestSessionBreakHoldSharedStateMatchesJSOracle(t *testing.T) {
	root := filepath.Join("testdata", "composition")
	rawFixture, err := os.ReadFile(filepath.Join(root, "sbh-state-fixture-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Bars [][]float64 `json:"bars"`
	}
	if err := json.Unmarshal(rawFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	rows := make([]marketdata.Bar, len(fixture.Bars))
	for i, row := range fixture.Bars {
		if len(row) != 6 {
			t.Fatalf("bar %d has %d columns", i, len(row))
		}
		rows[i] = marketdata.Bar{T: row[0], O: row[1], H: row[2], L: row[3], C: row[4], V: row[5]}
	}
	rawOracle, err := os.ReadFile(filepath.Join(root, "sbh-state-js-oracle-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Scenarios []struct {
			Name      string   `json:"name"`
			Indices   []int    `json:"indices"`
			LastEntry int      `json:"lastEntry"`
			Seen      []string `json:"seen"`
			Events    []struct {
				Index    int    `json:"index"`
				Side     string `json:"side"`
				LevelKey string `json:"levelKey"`
				Accepted bool   `json:"accepted"`
			} `json:"events"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(rawOracle, &oracle); err != nil {
		t.Fatal(err)
	}
	if len(oracle.Scenarios) != 2 {
		t.Fatal("expected both JS state scenarios")
	}
	for _, scenario := range oracle.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			n := len(rows)
			levels := make([]float64, n)
			for i := range levels {
				levels[i] = math.NaN()
			}
			copyLevels := func(value float64) []float64 {
				out := make([]float64, n)
				for i := range out {
					out[i] = value
				}
				return out
			}
			cols := contextcols.Columns{
				ATR: copyLevels(1), ER: copyLevels(0),
				PriorSession: contextcols.PriorSessionColumns{
					Asia:   contextcols.OHLCColumns{H: copyLevels(100), L: append([]float64(nil), levels...)},
					London: contextcols.OHLCColumns{H: copyLevels(101), L: copyLevels(99)},
				},
			}
			long := flagParams{SetupType: "sessionBreakHold", SBHHoldCandles: 3, SBHStopInsideATR: 0.5,
				SBHMinStopATR: 0.5, SBHMaxStopATR: 3, SBHTargetR: 1.2,
				CooldownBars: 12, AllowLong: true, SBHAllowedLevels: []string{"AH", "LH"},
				UseLondonWindow: true, UseNYWindow: true, MaxMovementER: 1, RiskUSD: 200,
			}
			short := long
			short.AllowLong, short.AllowShort = false, true
			short.SBHAllowedLevels = []string{"LL"}
			var b broker
			b.reset(marketdata.SeriesFromBars(rows), cols, nil, nil, nil, long,
				RunFixture{Symbol: "XAUUSD", Timeframe: "15m", RangeMethod: "zone", Costs: Costs{FillOn: "close", StartEquity: 10000}}, nil)
			var got []struct {
				Index    int
				Side     string
				LevelKey string
			}
			for _, i := range scenario.Indices {
				b.hasPosition = false // oracle closes positions before each decision
				b.params = long
				if scenario.Name == "rejected-retry" && i == 5 {
					b.params.MaxEntryDistanceATR = 0.01 // JS enter returned false
				}
				b.onSessionBreakHoldBar(i)
				if !b.hasPosition {
					b.params = short
					b.onSessionBreakHoldBar(i)
				}
				if b.hasPosition {
					got = append(got, struct {
						Index    int
						Side     string
						LevelKey string
					}{i, b.position.Side.String(), b.position.Meta["levelKey"].(string)})
				}
				if scenario.Name == "rejected-retry" && i == 5 && (b.hasSBHEntry || len(b.seen.sbh.keys) != 0) {
					t.Fatal("rejected entry consumed shared cooldown or seen state")
				}
			}
			var want []struct {
				Index    int
				Side     string
				LevelKey string
			}
			for _, event := range scenario.Events {
				if event.Accepted {
					want = append(want, struct {
						Index    int
						Side     string
						LevelKey string
					}{event.Index, event.Side, event.LevelKey})
				}
			}
			if !reflect.DeepEqual(got, want) || b.sbhLastEntry != scenario.LastEntry || !reflect.DeepEqual(b.seen.sbh.keys, scenario.Seen) {
				t.Fatalf("Go state differs from JS oracle: events=%v last=%d seen=%v; want events=%v last=%d seen=%v",
					got, b.sbhLastEntry, b.seen.sbh.keys, want, scenario.LastEntry, scenario.Seen)
			}
		})
	}
}
