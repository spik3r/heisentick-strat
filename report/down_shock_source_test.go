package report

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/testsupport"
)

func TestDownShockReportUsesAdmittedSource(t *testing.T) {
	chart, source := testsupport.DownShockBars()
	parsed, err := dsl.Parse(testsupport.DownShockSource)
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse: %v %v", err, parsed.Errors)
	}
	zero := 0.0
	for _, slippage := range []*float64{&zero, nil} {
		document, err := Build(context.Background(), Request{
			Config: parsed.Config, StrategyID: "inventedDownShock", Slippage: slippage,
			IncludeTrades: true, GeneratedAt: time.Unix(0, 0),
			Route: Route{Symbol: "XAUUSD", TF: "1m", SourceTimeframe: "15m", Range: "zone",
				Series: marketdata.SeriesFromBars(chart), SourceSeries: marketdata.SeriesFromBars(source)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(document.Warnings) != 0 || len(document.Slices) != 1 {
			t.Fatalf("report envelope: %+v", document)
		}
		slice := document.Slices[0]
		if slice.SourceBars == nil || *slice.SourceBars != 461 || slice.SourceTimeframe == nil || *slice.SourceTimeframe != "15m" ||
			slice.Trades == nil || len(*slice.Trades) != 1 {
			t.Fatalf("report slice: %+v", slice)
		}
		for i, row := range document.Costs {
			fixture, err := engine.RunFixtureCase(engine.RunFixture{
				StrategyID: "inventedDownShock", Symbol: "XAUUSD", Timeframe: "1m", SourceTimeframe: "15m", RangeMethod: "zone",
				Costs: engine.Costs{FillOn: "close", StartEquity: 10000, Slippage: row.Slippage, SlippageBps: row.SlippageBps},
				Bars:  chart, SourceBars: source,
			}, testsupport.DownShockSource)
			if err != nil || fixture.TradeCount != 1 {
				t.Fatalf("fixture: %+v, %v", fixture, err)
			}
			if row.Trades != 1 || row.Net != fixture.Trades[0].PnL {
				t.Fatalf("cost row %+v differs from fixture %+v", row, fixture)
			}
			if i == document.PrimaryCost.Index && !reflect.DeepEqual((*slice.Trades)[0].Trade, fixture.Trades[0]) {
				t.Fatalf("report trade %+v differs from fixture %+v", (*slice.Trades)[0].Trade, fixture.Trades[0])
			}
		}
	}
}
