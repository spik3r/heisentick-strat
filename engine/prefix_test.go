package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func moneyRiskSizingPrefixRequest(t *testing.T, barCount int) RunRequest {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "money-risk-sizing.fixture.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	if barCount > len(fixture.Bars) {
		t.Fatalf("fixture bars = %d, need %d", len(fixture.Bars), barCount)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "money-risk-sizing.strat"))
	if err != nil {
		t.Fatalf("read DSL source: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse DSL: %v / %v", err, parsed.Errors)
	}
	return RunRequest{
		Config: parsed.Config, Series: marketdata.SeriesFromBars(fixture.Bars[:barCount]),
		StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs,
	}
}

func TestRunPrefixPreservesOpenPositionAndLaterClosesOnce(t *testing.T) {
	const openBarCount = 679
	open, err := RunPrefix(moneyRiskSizingPrefixRequest(t, openBarCount))
	if err != nil {
		t.Fatalf("run open prefix: %v", err)
	}
	if len(open.Trades) != 0 || len(open.OpenPositions) != 1 {
		t.Fatalf("open prefix = trades %+v positions %+v, want one preserved position and no synthetic close", open.Trades, open.OpenPositions)
	}
	position := open.OpenPositions[0]
	if position.EntryT != 1736335800000 || position.EntryIndex != 677 || position.Side != "long" {
		t.Fatalf("open position = %+v, want reviewed fixture entry", position)
	}

	closed, err := RunPrefix(moneyRiskSizingPrefixRequest(t, 700))
	if err != nil {
		t.Fatalf("run extended prefix: %v", err)
	}
	if len(closed.OpenPositions) != 0 {
		t.Fatalf("extended prefix positions = %+v, want closed", closed.OpenPositions)
	}
	matches := 0
	for _, trade := range closed.Trades {
		if trade.EntryT == position.EntryT && trade.EntryIndex == position.EntryIndex && trade.Side == position.Side {
			matches++
			if trade.Reason == ReasonEndOfTest {
				t.Fatalf("extended prefix synthesized end-of-test close: %+v", trade)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("extended prefix matching closes = %d in %+v, want exactly one", matches, closed.Trades)
	}
}

func TestScheduledPrefixPreservesOpenPosition(t *testing.T) {
	series := marketdata.SeriesFromBars([]marketdata.Bar{{T: 1000, O: 100, H: 101, L: 99, C: 100, V: 1}})
	var b broker
	b.reset(series, contextcols.Build(series, contextcols.Options{}), nil, nil, nil, flagParams{RiskUSD: 100}, RunFixture{}, nil)
	orders := []order{{Side: sideLong, SL: 90, TP: 120, RiskUSD: 100, Tag: "scheduled", Index: 0}}
	trades := b.runScheduledWithFinalization([]ScheduledEntry{{ChartIndex: 0}}, orders, false)
	if len(trades) != 0 || len(b.openPositionSnapshot()) != 1 {
		t.Fatalf("scheduled prefix trades=%+v positions=%+v, want preserved open position", trades, b.openPositionSnapshot())
	}
}

func TestRunPrefixRejectsUnsupportedEnginePaths(t *testing.T) {
	special := moneyRiskSizingPrefixRequest(t, 679)
	special.Config["setupType"] = string(dsl.FamilyDailyFlushFailure)
	_, err := RunPrefix(special)
	var unsupported *PrefixUnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Path != "special family dailyFlushFailure" {
		t.Fatalf("special prefix error = %#v, want typed unsupported path", err)
	}

	c5 := moneyRiskSizingPrefixRequest(t, 679)
	c5.Config["sourceTimeframe"] = "15m"
	c5.Config["entryTf"] = "1m"
	c5.Timeframe = "1m"
	c5.SourceTimeframe = "15m"
	c5.SourceSeries = c5.Series
	_, err = RunPrefix(c5)
	if !errors.As(err, &unsupported) || unsupported.Path != "source-entry C5" {
		t.Fatalf("C5 prefix error = %#v, want typed unsupported path", err)
	}
}
