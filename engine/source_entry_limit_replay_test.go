package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestCapturedSourceLimitReplaysOnLowerTimeframe(t *testing.T) {
	source := marketdata.SeriesFromBars([]marketdata.Bar{{T: 0, O: 100, H: 101, L: 99, C: 100, V: 1}})
	var sourceBroker broker
	sourceBroker.reset(source, contextcols.Build(source, contextcols.Options{}), nil, nil, nil, flagParams{RiskUSD: 100, TradeWindowUnrestricted: true}, RunFixture{}, nil)
	sourceBroker.captureEntries = true
	if !sourceBroker.enterLimit(0, 99, setupPlan{Side: sideLong, Stop: 98, Target: 101, Tag: "source-limit"}, 2) {
		t.Fatal("source limit should be captured")
	}
	if len(sourceBroker.capturedEntries) != 1 || !sourceBroker.capturedEntries[0].HasLimit {
		t.Fatalf("captured source orders = %+v, want one limit", sourceBroker.capturedEntries)
	}

	chart := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 100, H: 100, L: 100, C: 100, V: 1},
		{T: 60_000, O: 100, H: 100, L: 99, C: 99, V: 1},
		{T: 120_000, O: 99, H: 101, L: 99, C: 100, V: 1},
	})
	var chartBroker broker
	chartBroker.reset(chart, contextcols.Build(chart, contextcols.Options{}), nil, nil, nil, flagParams{RiskUSD: 100}, RunFixture{}, nil)
	trades := chartBroker.runScheduled([]ScheduledEntry{{ChartIndex: 0}}, sourceBroker.capturedEntries)
	if len(trades) != 1 {
		t.Fatalf("lower-timeframe replay trades = %+v, want one", trades)
	}
	if trades[0].EntryIndex != 1 || trades[0].ExitIndex != 2 || trades[0].Reason != "tp" {
		t.Fatalf("lower-timeframe replay trade = %+v, want fill at 1 and target at 2", trades[0])
	}
}

func TestScheduledSourceEventsKeepTheirCapturedOrderAfterCoverageFiltering(t *testing.T) {
	chart := marketdata.SeriesFromBars([]marketdata.Bar{
		{T: 0, O: 100, H: 100, L: 100, C: 100, V: 1},
		{T: 60_000, O: 100, H: 102, L: 100, C: 101, V: 1},
		{T: 120_000, O: 101, H: 102, L: 96, C: 99, V: 1},
	})
	var chartBroker broker
	chartBroker.reset(chart, contextcols.Build(chart, contextcols.Options{}), nil, nil, nil, flagParams{RiskUSD: 100}, RunFixture{}, nil)
	orders := []order{
		{Side: sideLong, SL: 98, TP: 101, Limit: 99, HasLimit: true, Index: 0, ExpireBars: 2, RiskUSD: 100, HasRisk: true},
		{Side: sideShort, SL: 103, TP: 99, Limit: 101, HasLimit: true, Index: 1, ExpireBars: 2, RiskUSD: 100, HasRisk: true},
		{Side: sideLong, SL: 98, TP: 103, Limit: 99, HasLimit: true, Index: 1, ExpireBars: 2, RiskUSD: 100, HasRisk: true},
	}
	trades := chartBroker.runScheduled([]ScheduledEntry{{SourceIndex: 1, ChartIndex: 0}}, orders)
	if len(trades) != 1 {
		t.Fatalf("scheduled replay trades = %+v, want one trade from source event 1", trades)
	}
	trade := trades[0]
	if trade.Side != "short" || trade.Entry != 101 || trade.InitialSL != 103 || trade.InitialTP != 99 || trade.Reason != "tp" {
		t.Fatalf("scheduled replay trade = %+v, want first source-index-1 short limit and exits", trade)
	}
}
