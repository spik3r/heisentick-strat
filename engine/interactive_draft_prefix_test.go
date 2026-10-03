package engine

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func prefixDraftFixture(t *testing.T, raw []byte, count int) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["bars"] = fields["bars"].([]any)[:count]
	fields["costs"].(map[string]any)["fillOn"] = "nextOpen"
	out, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInteractiveDraftPrefixPendingOpenCloseAndIdentity(t *testing.T) {
	raw := draftFixture(t)
	run := func(n int, source string) InteractiveDraftPrefixResult {
		t.Helper()
		out, err := RunInteractiveDraftPrefixFixture(prefixDraftFixture(t, raw, n), source)
		if err != nil {
			t.Fatal(err)
		}
		if out.Schema != InteractiveDraftPrefixSchema || out.CalculationSource != "raw" {
			t.Fatal("wrong prefix contract")
		}
		return out
	}
	pending, open := run(8, draftSMASource), run(9, draftSMASource)
	if len(pending.Result.Trades) != 0 || len(pending.Result.OpenPositions) != 0 || len(pending.TradeNetPnL) != 0 {
		t.Fatal("unfilled final next-open entry was synthesized")
	}
	if len(open.Result.Trades) != 0 || len(open.Result.OpenPositions) != 1 || len(open.TradeNetPnL) != 0 {
		t.Fatal("open exposure was liquidated or given a closed net summary")
	}
	position := open.Result.OpenPositions[0]
	if position.EntryIndex != 8 || position.Entry != 11.06 || position.Size != 1 || !strings.HasPrefix(position.PositionID, "fp_") {
		t.Fatalf("raw next-open witness: %#v", position)
	}
	if !reflect.DeepEqual(open, run(9, draftSMASource)) {
		t.Fatal("same-input prefix changed")
	}
	extended, closed := run(11, draftSMASource), run(14, draftSMASource)
	if len(extended.Result.OpenPositions) != 1 || extended.Result.OpenPositions[0].PositionID != position.PositionID ||
		len(closed.Result.OpenPositions) != 0 || len(closed.Result.Trades) != 1 || closed.Result.Trades[0].PositionID != position.PositionID {
		t.Fatal("append-only open-to-closed identity changed")
	}
	trade := closed.Result.Trades[0].Trade
	if trade.Reason == ReasonEndOfTest || closed.TradeNetPnL[0] != trade.PnL-0.1*trade.Size {
		t.Fatal("prefix introduced final liquidation or lost entry fee")
	}
	if open.StrategyVersion != closed.StrategyVersion || open.Result.CheckpointDigest == closed.Result.CheckpointDigest || open.Provenance.FixtureSHA256 == closed.Provenance.FixtureSHA256 {
		t.Fatal("identity scope or full input binding is wrong")
	}
	changed := run(9, strings.Replace(draftSMASource, "Mutable SMA", "Another exact source", 1))
	if changed.StrategyVersion == open.StrategyVersion || changed.Result.OpenPositions[0].PositionID == position.PositionID {
		t.Fatal("different mutable source collided")
	}
	var fields map[string]any
	_ = json.Unmarshal(prefixDraftFixture(t, raw, 9), &fields)
	fields["costs"].(map[string]any)["feePerUnit"] = 0.2
	costRaw, _ := json.Marshal(fields)
	costChanged, err := RunInteractiveDraftPrefixFixture(costRaw, draftSMASource)
	if err != nil || costChanged.StrategyVersion == open.StrategyVersion || costChanged.Result.OpenPositions[0].PositionID == position.PositionID {
		t.Fatal("different costs collided")
	}
	fields["costs"].(map[string]any)["feePerUnit"] = 0.1
	fields["bars"].([]any)[0].([]any)[5] = 2.0
	originRaw, _ := json.Marshal(fields)
	originChanged, err := RunInteractiveDraftPrefixFixture(originRaw, draftSMASource)
	if err != nil || originChanged.StrategyVersion == open.StrategyVersion {
		t.Fatal("different history origin collided")
	}
	finalized, err := RunInteractiveDraftFixture(prefixDraftFixture(t, raw, 9), draftSMASource)
	if err != nil || len(finalized.Run.Trades) != 1 || finalized.Run.Trades[0].Reason != ReasonEndOfTest || finalized.TradeNetPnL[0] != finalized.Run.Trades[0].PnL-0.1 {
		t.Fatal("finalized draft output/last-bar entry fee changed")
	}
}

func TestInteractiveDraftPrefixFixedFailedBreakout(t *testing.T) {
	raw := failedBreakoutDraftFixture(t, 1600)
	open, err := RunInteractiveDraftPrefixFixture(prefixDraftFixture(t, raw, 298), draftFailedBreakoutSource)
	if err != nil || len(open.Result.OpenPositions) != 1 || len(open.Result.Trades) != 0 {
		t.Fatalf("FB open: %v", err)
	}
	closed, err := RunInteractiveDraftPrefixFixture(prefixDraftFixture(t, raw, 304), draftFailedBreakoutSource)
	if err != nil || len(closed.Result.Trades) != 1 || len(closed.Result.OpenPositions) != 0 ||
		closed.Result.Trades[0].PositionID != open.Result.OpenPositions[0].PositionID {
		t.Fatalf("FB close: %v", err)
	}
}

func TestInteractiveDraftPrefixDualEMATrailingOpenToClose(t *testing.T) {
	source := `dsl v7
strategy "Mutable raw Dual prefix"
market conditions { slices(XAUUSD 5m) }
setup {
 type: dual ema resumption
 ema fast 3
 ema slow 8
 ema slow rise 1
 ema wilder atr 3
 stop initial 2.5 ATR
 trail close 3 ATR
 fallback below slow ema
}
filters { side long only }
execution { risk 200 USD }`
	var fields map[string]any
	_ = json.Unmarshal(draftFixture(t), &fields)
	rows := make([][]float64, 1600)
	for i := range rows {
		c := 1900 + float64(i)*0.2
		if i%12 == 9 {
			c -= 2.4
		}
		rows[i] = []float64{float64(i) * 300000, c - 0.1, c + 0.3, c - 0.3, c, 1}
	}
	fields["bars"] = rows
	fields["costs"].(map[string]any)["fillOn"] = "nextOpen"
	raw, _ := json.Marshal(fields)
	full, err := RunInteractiveDraftPrefixFixture(raw, source)
	if err != nil {
		t.Fatal(err)
	}
	var witness *PrefixClosedTrade
	for i := range full.Result.Trades {
		closed := &full.Result.Trades[i]
		if closed.Trade.ExitIndex > closed.Trade.EntryIndex+2 && closed.Trade.SL > closed.Trade.InitialSL {
			witness = closed
			break
		}
	}
	if witness == nil {
		t.Fatal("Dual synthetic fixture needs a trailing-stop witness")
	}
	entry, exit := witness.Trade.EntryIndex, witness.Trade.ExitIndex
	open, err := RunInteractiveDraftPrefixFixture(prefixDraftFixture(t, raw, entry+1), source)
	if err != nil || len(open.Result.OpenPositions) != 1 {
		t.Fatalf("Dual open: %v", err)
	}
	extended, err := RunInteractiveDraftPrefixFixture(prefixDraftFixture(t, raw, exit), source)
	if err != nil || len(extended.Result.OpenPositions) != 1 {
		t.Fatalf("Dual extended: %v", err)
	}
	closed, err := RunInteractiveDraftPrefixFixture(prefixDraftFixture(t, raw, exit+1), source)
	if err != nil || len(closed.Result.Trades) == 0 {
		t.Fatalf("Dual closed: %v", err)
	}
	a, b := open.Result.OpenPositions[0], extended.Result.OpenPositions[0]
	last := closed.Result.Trades[len(closed.Result.Trades)-1]
	if a.PositionID != b.PositionID || a.PositionID != last.PositionID || a.PositionID != witness.PositionID ||
		b.SL <= a.SL || math.Abs(last.Trade.SL-b.SL) > 1e-9 || !reflect.DeepEqual(last, *witness) {
		t.Fatal("Dual append changed identity or lost current trailing stop/close")
	}
}

func TestInteractiveDraftPrefixStrictCostsAndRefusals(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"switch":        func(f map[string]any) { f["closeAtEnd"] = false },
		"HA":            func(f map[string]any) { f["calculationSource"] = "heikinAshi" },
		"start zero":    func(f map[string]any) { f["costs"].(map[string]any)["startEquity"] = 0 },
		"fee negative":  func(f map[string]any) { f["costs"].(map[string]any)["feePerUnit"] = -1 },
		"slip negative": func(f map[string]any) { f["costs"].(map[string]any)["slippage"] = -1 },
		"bps negative":  func(f map[string]any) { f["costs"].(map[string]any)["slippageBps"] = -1 },
		"fill unknown":  func(f map[string]any) { f["costs"].(map[string]any)["fillOn"] = "open" },
		"cost alias":    func(f map[string]any) { f["costs"].(map[string]any)["FeePerUnit"] = 0 },
		"cost missing":  func(f map[string]any) { delete(f["costs"].(map[string]any), "slippageBps") },
		"params":        func(f map[string]any) { f["params"] = map[string]any{} },
		"htf":           func(f map[string]any) { f["htfBars"] = []any{} },
		"window":        func(f map[string]any) { f["executionWindow"] = map[string]any{} },
		"route":         func(f map[string]any) { f["symbol"] = "BTCUSDT" },
		"range":         func(f map[string]any) { f["rangeMethod"] = "zone" },
		"grid":          func(f map[string]any) { f["bars"].([]any)[1].([]any)[0] = 300001.0 },
	} {
		t.Run(name, func(t *testing.T) {
			var fields map[string]any
			_ = json.Unmarshal(draftFixture(t), &fields)
			mutate(fields)
			raw, _ := json.Marshal(fields)
			if _, err := RunInteractiveDraftPrefixFixture(raw, draftSMASource); err == nil {
				t.Fatal("unsupported prefix executed")
			}
		})
	}
	if _, err := RunInteractiveDraftPrefixFixture(draftFixture(t), strings.Replace(draftSMASource, "side long only", "side long only\n candle body fraction >= 0.5", 1)); err == nil {
		t.Fatal("source audit bypassed")
	}
}
