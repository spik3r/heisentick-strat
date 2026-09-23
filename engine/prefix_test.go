package engine

import (
	"errors"
	"math"
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
	if open.Trades == nil || len(open.Trades) != 0 || len(open.OpenPositions) != 1 {
		t.Fatalf("open prefix = trades %+v positions %+v, want one preserved position and no synthetic close", open.Trades, open.OpenPositions)
	}
	position := open.OpenPositions[0]
	if position.PositionID == "" || position.EntryT != 1736335800000 || position.EntryIndex != 677 || position.Side != "long" {
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
	for _, closedTrade := range closed.Trades {
		trade := closedTrade.Trade
		if trade.EntryT == position.EntryT && trade.EntryIndex == position.EntryIndex && trade.Side == position.Side {
			matches++
			if closedTrade.PositionID != position.PositionID {
				t.Fatalf("closed position id = %q, want open id %q", closedTrade.PositionID, position.PositionID)
			}
			if trade.Reason == ReasonEndOfTest {
				t.Fatalf("extended prefix synthesized end-of-test close: %+v", trade)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("extended prefix matching closes = %d in %+v, want exactly one", matches, closed.Trades)
	}
}

func TestPrefixIdentityAndInputBindingDomains(t *testing.T) {
	base := moneyRiskSizingPrefixRequest(t, 679)
	result, err := RunPrefix(base)
	if err != nil {
		t.Fatalf("base prefix: %v", err)
	}
	positionID := result.OpenPositions[0].PositionID
	otherID, err := prefixPositionID(base.StrategyID, base.Symbol, base.Timeframe, result.OpenPositions[0].EntryT+1, result.OpenPositions[0].EntryIndex+1, result.OpenPositions[0].Side)
	if err != nil || otherID == positionID {
		t.Fatalf("new entry id = %q / %v, want different from %q", otherID, err, positionID)
	}

	variants := []RunRequest{base, base, base}
	variants[0].Costs.Slippage += 0.01
	variants[1].Series.C = append([]float64(nil), base.Series.C...)
	variants[1].Series.C[0] += 0.01
	variants[2].Config = dsl.Config{}
	for key, value := range base.Config {
		variants[2].Config[key] = value
	}
	variants[2].Config["description"] = "digest-only-change"
	for i, variant := range variants {
		digest, err := prefixCheckpointDigest(variant)
		if err != nil {
			t.Fatalf("variant %d digest: %v", i, err)
		}
		if digest == result.CheckpointDigest {
			t.Fatalf("variant %d digest did not change", i)
		}
		gotID, err := prefixPositionID(variant.StrategyID, variant.Symbol, variant.Timeframe, result.OpenPositions[0].EntryT, result.OpenPositions[0].EntryIndex, result.OpenPositions[0].Side)
		if err != nil || gotID != positionID {
			t.Fatalf("variant %d position id = %q / %v, want %q", i, gotID, err, positionID)
		}
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

func TestPrefixOutputRejectsNonFiniteClosedAndOpenState(t *testing.T) {
	finiteTrade := Trade{
		Entry: 1, EntryT: 2, Exit: 3, ExitT: 4, InitialSL: 0, InitialTP: 0,
		PnL: 5, Points: 6, Size: 7, SL: 0, TP: 0, NoStop: true, NoTarget: true,
	}
	if err := validatePrefixOutput(Costs{}, []Trade{finiteTrade}, []OpenPositionSnapshot{{
		Entry: 1, EntryT: 2, InitialSL: 0, InitialTP: 0, SL: 0, TP: 0, Size: 1,
		NoStop: true, NoTarget: true,
	}}); err != nil {
		t.Fatalf("absent brackets use finite zero values plus flags: %v", err)
	}

	badTrade := finiteTrade
	badTrade.Meta = TradeMeta{"metric": math.Inf(1)}
	if err := validatePrefixOutput(Costs{}, []Trade{badTrade}, nil); err == nil {
		t.Fatal("non-finite closed trade metadata was accepted")
	}
	badPosition := OpenPositionSnapshot{Entry: 1, EntryT: 2, InitialSL: 0, InitialTP: 0, SL: 0, TP: math.NaN(), Size: 1}
	if err := validatePrefixOutput(Costs{}, nil, []OpenPositionSnapshot{badPosition}); err == nil {
		t.Fatal("non-finite open position field was accepted")
	}
}

func TestRunPrefixValidatesCostsBeforeOffRouteReturn(t *testing.T) {
	request := moneyRiskSizingPrefixRequest(t, 679)
	request.Timeframe = "4h"
	request.Costs.FeePerUnit = math.NaN()
	if _, err := RunPrefix(request); err == nil || err.Error() != "run result costs.feePerUnit contains non-finite value" {
		t.Fatalf("off-route prefix cost error = %v, want checked non-finite rejection", err)
	}

	request.Costs.FeePerUnit = 0
	result, err := RunPrefix(request)
	if err != nil {
		t.Fatalf("valid off-route prefix: %v", err)
	}
	if result.Trades == nil || result.OpenPositions == nil || len(result.Trades) != 0 || len(result.OpenPositions) != 0 {
		t.Fatalf("off-route prefix = %#v, want non-nil empty arrays", result)
	}
}
