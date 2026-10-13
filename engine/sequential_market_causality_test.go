package engine

import (
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Prefix causality for the Full E1/E2 scheduler on invented random-walk bars.
// The finalizing run refuses a held terminal position, so this uses the
// internal non-finalizing mode (as the existing causality tests do) and
// compares complete marks, trades and audit rows. Equality is bit-exact: the
// same code sees the same inputs, so no tolerance is involved.

func causalityBars(seed int64, n int, stepMS float64, level float64) []marketdata.Bar {
	rng := rand.New(rand.NewSource(seed))
	bars := make([]marketdata.Bar, n)
	price := level
	for i := range bars {
		open := price
		drift := (rng.Float64() - 0.5) * 0.02 * level
		closePrice := open + drift
		high := math.Max(open, closePrice) + rng.Float64()*0.006*level
		low := math.Min(open, closePrice) - rng.Float64()*0.006*level
		bars[i] = marketdata.Bar{T: 1709510400000 + float64(i)*stepMS, O: open, H: high, L: low, C: closePrice, V: 1}
		price = closePrice
	}
	return bars
}

func causalityRun(t *testing.T, policy, timeframe string, step int64, bars []marketdata.Bar) ([]Trade, SequentialFullAudit, []SequentialEquityMark) {
	t.Helper()
	spec := dsl.SequentialFullSpec{Profile: dsl.SequentialFullProfileID, Policy: policy, Symbol: "EURUSD", Timeframe: timeframe, RiskUSD: 50, MaxNotionalUSD: 100000}
	costs := Costs{FillOn: "open", Slippage: 0.0001, FeePerUnit: 0.000002, StartEquity: 10000}.normalized()
	capture := newSequentialAccountingCollector(costs.StartEquity)
	trades, audit, err := runSequentialFullWithCapture(spec, marketdata.SeriesFromBars(bars), RunFixture{Costs: costs}, false, capture)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if capture.err != nil {
		t.Fatalf("capture: %v", capture.err)
	}
	return trades, audit, capture.data.Marks
}

// Exercises the shared scheduler directly, not the v2 entry point.
func TestSequentialMarketPrefixCausality(t *testing.T) {
	for _, policy := range []string{"E1", "E2"} {
		for seed := int64(1); seed <= 4; seed++ {
			step := int64(900_000)
			full := causalityBars(seed, 600, float64(step), 1.1)
			fullTrades, fullAudit, fullMarks := causalityRun(t, policy, "15m", step, full)
			if policy == "E1" && len(fullTrades) < 2 {
				t.Fatalf("seed %d produced %d E1 trades; the series does not exercise the scheduler", seed, len(fullTrades))
			}
			for cut := 20; cut < len(full); cut += 13 {
				// Changing only bars after the cut must not change anything known by the cut.
				changed := append([]marketdata.Bar{}, full[:cut+1]...)
				changed = append(changed, causalityBars(seed+1000, len(full)-cut-1, float64(step), 1.1)...)
				for i := cut + 1; i < len(changed); i++ {
					changed[i].T = full[i].T
				}
				prefixTrades, prefixAudit, prefixMarks := causalityRun(t, policy, "15m", step, full[:cut+1])
				otherTrades, otherAudit, otherMarks := causalityRun(t, policy, "15m", step, changed)
				if !reflect.DeepEqual(prefixMarks, fullMarks[:cut+1]) || !reflect.DeepEqual(otherMarks[:cut+1], fullMarks[:cut+1]) {
					t.Fatalf("%s seed %d cut %d: marks through the cut changed", policy, seed, cut)
				}
				closed := func(trades []Trade) []Trade {
					out := []Trade{}
					for _, tr := range trades {
						if tr.ExitIndex <= cut {
							out = append(out, tr)
						}
					}
					return out
				}
				if !reflect.DeepEqual(closed(fullTrades), prefixTrades) || !reflect.DeepEqual(closed(fullTrades), closed(otherTrades)) {
					t.Fatalf("%s seed %d cut %d: completed trades changed", policy, seed, cut)
				}
				// A decision before the cut has its fill on or before the cut.
				settled := func(a SequentialFullAudit) []SequentialFullOpportunity {
					out := []SequentialFullOpportunity{}
					for _, o := range a.Opportunities {
						if o.DecisionIndex < cut {
							out = append(out, o)
						}
					}
					return out
				}
				if !reflect.DeepEqual(settled(fullAudit), settled(prefixAudit)) || !reflect.DeepEqual(settled(fullAudit), settled(otherAudit)) {
					t.Fatalf("%s seed %d cut %d: settled decisions changed", policy, seed, cut)
				}
			}
		}
	}
}
