package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestPreparedRunRunOwnsResultsAcrossCostReruns(t *testing.T) {
	fixture, cfg := loadEntryAttemptCase(t, "family-break-retest")
	prepared, err := PrepareRun(RunRequest{
		Config:          cfg,
		Series:          marketdata.SeriesFromBars(fixture.Bars),
		HTFSeries:       marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Costs:           fixture.Costs,
	})
	if err != nil {
		t.Fatalf("prepare public run: %v", err)
	}

	lowCosts := Costs{FillOn: "close", StartEquity: 10_000}
	highCosts := Costs{FeePerUnit: 0.25, Slippage: 0.1, FillOn: "close", StartEquity: 20_000}
	low := prepared.Run(lowCosts)
	if low.TradeCount != 13 || len(low.Trades) != 13 {
		t.Fatalf("low-cost trades = %d/%d, want reviewed fixture count 13", low.TradeCount, len(low.Trades))
	}
	if low.Costs != lowCosts.normalized() {
		t.Fatalf("low-cost envelope = %+v, want %+v", low.Costs, lowCosts.normalized())
	}
	lowSnapshot := canonicalJSON(low)

	high := prepared.Run(highCosts)
	if got := canonicalJSON(low); got != lowSnapshot {
		t.Fatalf("later run mutated earlier result\n got: %s\nwant: %s", got, lowSnapshot)
	}
	if high.TradeCount != low.TradeCount || len(high.Trades) != len(low.Trades) {
		t.Fatalf("high-cost trades = %d/%d, want %d/%d", high.TradeCount, len(high.Trades), low.TradeCount, len(low.Trades))
	}
	if high.Costs != highCosts.normalized() {
		t.Fatalf("high-cost envelope = %+v, want %+v", high.Costs, highCosts.normalized())
	}
	entryChanged := false
	exitChanged := false
	pnlChanged := false
	for i := range low.Trades {
		lowTrade := low.Trades[i]
		highTrade := high.Trades[i]
		if lowTrade.EntryIndex != highTrade.EntryIndex || lowTrade.ExitIndex != highTrade.ExitIndex ||
			lowTrade.EntryT != highTrade.EntryT || lowTrade.ExitT != highTrade.ExitT ||
			lowTrade.Side != highTrade.Side || lowTrade.Reason != highTrade.Reason ||
			lowTrade.Tag != highTrade.Tag || lowTrade.Partial != highTrade.Partial {
			t.Fatalf("trade %d identity changed across costs\n low: %+v\nhigh: %+v", i, lowTrade, highTrade)
		}
		if got, want := canonicalJSON(highTrade.Meta), canonicalJSON(lowTrade.Meta); got != want {
			t.Fatalf("trade %d metadata changed across costs\n got: %s\nwant: %s", i, got, want)
		}
		entryChanged = entryChanged || lowTrade.Entry != highTrade.Entry
		exitChanged = exitChanged || lowTrade.Exit != highTrade.Exit
		pnlChanged = pnlChanged || lowTrade.PnL != highTrade.PnL
	}
	if !entryChanged || !exitChanged || !pnlChanged {
		t.Fatalf("changed costs did not affect all execution dimensions: entry=%v exit=%v pnl=%v",
			entryChanged, exitChanged, pnlChanged)
	}
	highSnapshot := canonicalJSON(high)

	low.Trades[0].PnL = -999_999
	low.Trades[0].Meta["setup"] = "caller-mutated"
	highAgain := prepared.Run(highCosts)
	if got := canonicalJSON(highAgain); got != highSnapshot {
		t.Fatalf("caller mutation poisoned later high-cost run\n got: %s\nwant: %s", got, highSnapshot)
	}

	lowAgain := prepared.Run(lowCosts)
	if got := canonicalJSON(lowAgain); got != lowSnapshot {
		t.Fatalf("alternating costs did not fully reset prepared-run state\n got: %s\nwant: %s", got, lowSnapshot)
	}
}
