package engine

import (
	"math"
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestBrokerMarkPrecedesFinalLiquidationAndResetClearsCapture(t *testing.T) {
	series := marketdata.Series{
		T: []float64{0, 1, 2, 3, 4, 5},
		O: []float64{3, 2, 1, 4, 5, 3},
		H: []float64{3, 2, 1, 4, 5, 3},
		L: []float64{3, 2, 1, 4, 3, 0},
		C: []float64{3, 2, 1, 4, 3, 0},
		V: []float64{1, 1, 1, 1, 1, 1},
	}
	params := flagParams{SetupType: "smaGoldenCross", SMAGoldenCross: smaGoldenCrossParams{
		FastLen: 2, SlowLen: 3, AllowLong: true,
	}}
	fixture := RunFixture{Costs: Costs{FillOn: "close", StartEquity: 10000, FeePerUnit: 0.1}}
	var b broker
	b.reset(series, contextcols.Columns{}, nil, nil, nil, params, fixture, nil)
	b.equityCurve = make([]float64, series.Len())
	trades := b.run()
	if len(trades) != 1 || trades[0].Reason != ReasonEndOfTest || trades[0].ExitIndex != 5 {
		t.Fatalf("final liquidation trades = %+v", trades)
	}
	if math.IsNaN(b.equityCurve[5]) || math.IsInf(b.equityCurve[5], 0) ||
		math.IsNaN(b.realized) || math.IsInf(b.realized, 0) ||
		math.Abs(b.equityCurve[5]-9994.9) > 1e-9 || math.Abs(10000+b.realized-9994.8) > 1e-9 {
		t.Fatalf("last mark %.12f, post-liquidation equity %.12f", b.equityCurve[5], 10000+b.realized)
	}
	b.reset(series, contextcols.Columns{}, nil, nil, nil, params, fixture, nil)
	if b.equityCurve != nil || b.cashCurve != nil || b.realized != 0 || len(b.trades) != 0 {
		t.Fatalf("reset retained marked-run state: curve %v, realized %v, trades %+v", b.equityCurve, b.realized, b.trades)
	}
}
