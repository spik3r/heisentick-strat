package engine

import (
	"errors"
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// AuthoredVPAsiaLondonWideVersion identifies the fixed active JavaScript
// strategy defaults ported here. Changes to its trading semantics require a
// new version and a new whole-strategy oracle fixture.
const AuthoredVPAsiaLondonWideVersion = "vpAsiaLondonSweepContinuationFiveMinuteWideAsia/go-v1"

const authoredVPAsiaLondonWideID = "vpAsiaLondonSweepContinuationFiveMinuteWideAsia"

// RunAuthoredVPAsiaLondonWide executes the fixed XAUUSD 5m authored strategy.
// It intentionally does not accept a DSL config or silently dispatch another
// setup. The JavaScript base strategy owns the session, sweep, bracket, and
// London-close rules; the active variant owns the percentile and edge-distance
// filters. The frozen oracle in testdata records both source hashes.
func RunAuthoredVPAsiaLondonWide(bars []marketdata.Bar, symbol, timeframe string, costs Costs) (RunResult, error) {
	b, fixture, err := runAuthoredVPAsiaLondonWide(bars, symbol, timeframe, costs)
	if err != nil {
		return RunResult{}, err
	}
	return checkedResultEnvelope(fixture, b.trades)
}

func runAuthoredVPAsiaLondonWide(bars []marketdata.Bar, symbol, timeframe string, costs Costs) (*broker, RunFixture, error) {
	if symbol != "XAUUSD" || timeframe != "5m" {
		return nil, RunFixture{}, fmt.Errorf("%s supports XAUUSD 5m only, got %s %s", AuthoredVPAsiaLondonWideVersion, symbol, timeframe)
	}
	if costs.FillOn != "close" {
		return nil, RunFixture{}, fmt.Errorf("%s requires close fills", AuthoredVPAsiaLondonWideVersion)
	}
	if len(bars) == 0 {
		return nil, RunFixture{}, errors.New("VP Asia-London wide strategy requires bars")
	}
	if !isFinite(costs.FeePerUnit) || !isFinite(costs.Slippage) || !isFinite(costs.SlippageBps) || !isFinite(costs.StartEquity) || costs.FeePerUnit < 0 || costs.Slippage < 0 || costs.SlippageBps < 0 || costs.StartEquity <= 0 {
		return nil, RunFixture{}, errors.New("VP Asia-London wide strategy requires finite non-negative execution costs and positive start equity")
	}
	series := marketdata.SeriesFromBars(bars)
	if err := validateSeriesValues("VP Asia-London wide", series); err != nil {
		return nil, RunFixture{}, err
	}
	for i, bar := range bars {
		if !isFinite(bar.V) || bar.V < 0 || bar.H < math.Max(bar.O, bar.C) || bar.L > math.Min(bar.O, bar.C) || bar.H < bar.L {
			return nil, RunFixture{}, fmt.Errorf("VP Asia-London wide invalid OHLCV at bar %d", i)
		}
		if i > 0 && !(bar.T > bars[i-1].T) {
			return nil, RunFixture{}, fmt.Errorf("VP Asia-London wide timestamps must increase at bar %d", i)
		}
	}
	fixture := RunFixture{Case: AuthoredVPAsiaLondonWideVersion, StrategyID: authoredVPAsiaLondonWideID, Symbol: symbol, Timeframe: timeframe, RangeMethod: "zone", Costs: costs.normalized()}
	b := &broker{}
	b.reset(series, contextcols.Columns{}, nil, nil, nil, flagParams{}, fixture, nil)
	b.equityCurve = make([]float64, series.Len())
	b.cashCurve = make([]float64, series.Len())
	var state AsiaLondonSweepState
	params := AsiaLondonSweepParams{
		TickSize: 0.1, RowsLayout: "number_of_rows", SessionRows: 24, ValueAreaPercent: 70,
		SourceStartHour: 0, AsiaEndHour: 7, LondonStartHour: 7, LondonEndHour: 16,
		MinAsiaRangePercentile: 0.67, AsiaRangeLookback: 120, MinAsiaRangeHistory: 30,
		MaxTargetDistanceRange: 0.25, StopBufferRange: 0.35, RMultiple: 0.5,
		RequireOpenOutsideCompletedValue: true,
	}
	for i, bar := range bars {
		b.resolveIntrabarExit(i)
		hour := utcHourOfDay(bar.T)
		if b.hasPosition && hour >= params.LondonEndHour {
			b.closePosition(bar.C, i, ReasonRule, "london-close")
			b.markToMarket(i)
			continue
		}
		if b.hasPosition && hour >= params.LondonStartHour && hour < params.LondonEndHour {
			b.markToMarket(i)
			continue
		}
		signal := state.Process(bar, params)
		if signal == nil {
			b.markToMarket(i)
			continue
		}
		s := sideLong
		if signal.Side == "short" {
			s = sideShort
		}
		b.openPosition(s, signal.Entry, order{
			SL: signal.Stop, TP: signal.Target, RiskUSD: 200, HasRisk: true,
			Tag: "vpAsiaLondonSweepContinuation", Index: i,
			Meta: TradeMeta{
				"setup": "vpAsiaLondonSweepContinuation", "predictedSweep": signal.PredictedSweep,
				"openOutsideCompletedVA": signal.OpenOutsideCompletedVA,
				"expectedFirstRaidSide":  signal.PredictedSweep, "firstRaidTargetR": signal.FirstRaidTargetR,
				"sourceRange": signal.SourceRange, "sweepAlignmentBlocked": false,
				"asiaHigh": signal.AsiaHigh, "asiaLow": signal.AsiaLow,
				"asiaPoc": signal.AsiaPOC, "asiaVah": signal.AsiaVAH, "asiaVal": signal.AsiaVAL,
				"asiaRange": signal.SourceRange, "sourceStartHour": 0, "sourceEndHour": 7,
				"tradeStartHour": 7, "tradeEndHour": 16, "rMultiple": 0.5, "stopBufferRange": 0.35,
			},
		}, i)
		b.markToMarket(i)
	}
	if b.hasPosition {
		b.closePosition(series.C[series.Len()-1], series.Len()-1, ReasonEndOfTest, "")
	}
	if len(b.trades) == 0 {
		return nil, RunFixture{}, fmt.Errorf("%s produced no trades; parity is unproven for this input", AuthoredVPAsiaLondonWideVersion)
	}
	return b, fixture, nil
}
