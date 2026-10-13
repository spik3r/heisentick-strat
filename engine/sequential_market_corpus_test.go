package engine_test

// V2 market route against the accepted independent E1/E2 execution corpus.
//
// Two checks, neither of which reads an expected value from the engine:
//
//  1. Replay: every binding corpus case runs through RunSequentialMarketFixture
//     with an authored SYN/5m source and the corpus comparator used for the
//     other entry points. Marked equity, drawdown, per-trade fees and the
//     headline statistics are then recomputed here from the corpus's own
//     expected trades (fill, size, exit, gross P&L) and compared with the
//     metrics projection.
//  2. Route and unit matrix: the same cases are re-timed and re-priced onto
//     other symbols and timeframes. Multiplying every price, the slippage and
//     the per-unit fee by k, and dividing quantity by k, leaves USD P&L, fees,
//     equity, drawdown and bar indices unchanged. The expected scaled values
//     are derived from the corpus numbers by that identity.
//
// Tolerance: relative 1e-9 with no absolute floor, the corpus tolerance
// (seqAcceptanceRelative). Indices, counts, sides, strings and null reasons
// are compared exactly.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
	"github.com/spik3r/heisentick-strat/report"
)

const seqMarketStartEquity = 10000.0

func seqMarketSource(symbol, timeframe, policy string, risk, cap float64) string {
	return fmt.Sprintf("dsl v7\nstrategy \"Invented market E1/E2\" { description \"Invented valid OHLC\" }\nmarket { sequential symbol %s sequential timeframe %s }\nsetup { type: sequential full sequential profile %s sequential policy %s }\nrisk { sequential riskUsd %.17g sequential maxNotionalUsd %.17g }\n", symbol, timeframe, seqAcceptanceProfile, policy, risk, cap)
}

// seqMarketFixture builds a fixture through the production loader so that raw
// row scanning is exercised exactly as for a caller-supplied JSON file.
func seqMarketFixture(t *testing.T, id, symbol, timeframe string, bars []marketdata.Bar, costs engine.Costs) engine.RunFixture {
	t.Helper()
	rows := make([][]float64, len(bars))
	for i, b := range bars {
		rows[i] = []float64{b.T, b.O, b.H, b.L, b.C, b.V}
	}
	raw, err := json.Marshal(engine.RunFixture{Schema: "dsl-conformance-run-fixture-v1", Case: id, StrategyID: id, Symbol: symbol, Timeframe: timeframe, RangeMethod: "zone", Costs: costs, RawBars: rows})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	fixture, err := engine.LoadRunFixture(path)
	if err != nil {
		t.Fatalf("fixture transport: %v", err)
	}
	return fixture
}

// seqMarketExpected is what the corpus says should happen, restated in the
// units of a (possibly re-priced) route.
type seqMarketExpected struct {
	side                       float64
	entryIndex, exitIndex      int
	fill, exit, size, grossPnL float64
}

func seqMarketExpectedTrades(want seqAcceptanceOK, scale float64) []seqMarketExpected {
	out := make([]seqMarketExpected, len(want.Trades))
	for i, w := range want.Trades {
		side := 1.0
		if w.Side == "short" {
			side = -1
		}
		out[i] = seqMarketExpected{side, w.EntryIndex, w.Exit.Index, w.EntryFill * scale, w.Exit.Price * scale, w.Size / scale, w.GrossPnL}
	}
	return out
}

// seqMarketOracle recomputes the accounting from expected trades alone. A bar
// holds an open position when entry <= bar < exit; an exit at a bar's open is
// complete before that bar's close mark. The fee is charged per unit on entry
// and on exit. No hypothetical exit cost is charged to an open mark.
type seqMarketOracle struct {
	equity          []float64
	net             []float64
	entryFee        []float64
	exitFee         []float64
	maxDD, maxDDPct float64
	finalRealized   float64
}

func seqMarketCompute(bars []marketdata.Bar, trades []seqMarketExpected, fee float64) seqMarketOracle {
	var o seqMarketOracle
	for _, tr := range trades {
		o.entryFee = append(o.entryFee, fee*tr.size)
		o.exitFee = append(o.exitFee, fee*tr.size)
		o.net = append(o.net, tr.grossPnL-2*fee*tr.size)
	}
	peak := seqMarketStartEquity
	for i, bar := range bars {
		realized, unrealized := 0.0, 0.0
		for k, tr := range trades {
			if tr.entryIndex <= i {
				realized -= o.entryFee[k]
			}
			if tr.exitIndex <= i {
				realized += tr.grossPnL - o.exitFee[k]
			}
			if tr.entryIndex <= i && i < tr.exitIndex {
				unrealized += tr.side * (bar.C - tr.fill) * tr.size
			}
		}
		equity := seqMarketStartEquity + realized + unrealized
		o.equity = append(o.equity, equity)
		o.finalRealized = realized
		if equity > peak {
			peak = equity
		}
		if dd := peak - equity; dd > o.maxDD {
			o.maxDD = dd
		}
		if pct := (peak - equity) / peak * 100; pct > o.maxDDPct {
			o.maxDDPct = pct
		}
	}
	return o
}

func seqMarketCheckMetrics(t *testing.T, run engine.SequentialMarketRun, bars []marketdata.Bar, trades []seqMarketExpected, fee float64) {
	t.Helper()
	metrics, err := report.ProjectSequentialMetrics(run.Run, run.Accounting)
	if err != nil {
		t.Fatalf("metrics projection: %v", err)
	}
	want := seqMarketCompute(bars, trades, fee)
	if len(metrics.Equity) != len(bars) {
		t.Fatalf("equity points %d, want one per bar (%d)", len(metrics.Equity), len(bars))
	}
	for i, p := range metrics.Equity {
		seqAcceptanceExact(t, fmt.Sprintf("equity[%d].index", i), p.Index, i)
		seqAcceptanceExact(t, fmt.Sprintf("equity[%d].t", i), p.T, bars[i].T)
		seqAcceptanceNear(t, fmt.Sprintf("equity[%d]", i), p.Equity, want.equity[i])
	}
	h := metrics.Headline
	seqAcceptanceNear(t, "startEquity", h.StartEquity, seqMarketStartEquity)
	sum := 0.0
	for _, n := range want.net {
		sum += n
	}
	seqAcceptanceNear(t, "net", h.Net, want.finalRealized)
	seqAcceptanceNear(t, "net vs sum of trade nets", h.Net, sum)
	seqAcceptanceNear(t, "endEquity", h.EndEquity, seqMarketStartEquity+want.finalRealized)
	seqAcceptanceNear(t, "returnPct", h.ReturnPct, want.finalRealized/seqMarketStartEquity*100)
	seqAcceptanceNear(t, "maxDD", h.MaxDD, want.maxDD)
	seqAcceptanceNear(t, "maxDDpct", h.MaxDDpct, want.maxDDPct)
	seqAcceptanceExact(t, "trades", h.Trades, len(trades))
	seqAcceptanceExact(t, "tradeAccounting length", len(metrics.TradeAccounting), len(trades))
	wins, grossWin, grossLoss, worst, hold := 0, 0.0, 0.0, 0.0, 0.0
	winStreak, lossStreak, maxWin, maxLoss := 0, 0, 0, 0
	for i, n := range want.net {
		ta := metrics.TradeAccounting[i]
		seqAcceptanceNear(t, fmt.Sprintf("trade[%d].entryFee", i), ta.EntryFee, want.entryFee[i])
		seqAcceptanceNear(t, fmt.Sprintf("trade[%d].exitFee", i), ta.ExitFee, want.exitFee[i])
		seqAcceptanceNear(t, fmt.Sprintf("trade[%d].netPnl", i), ta.NetPnL, n)
		hold += float64(trades[i].exitIndex - trades[i].entryIndex)
		if n > 0 {
			wins++
			grossWin += n
			winStreak++
			lossStreak = 0
		} else {
			grossLoss -= n
			lossStreak++
			winStreak = 0
			if n < worst {
				worst = n
			}
		}
		maxWin, maxLoss = max(maxWin, winStreak), max(maxLoss, lossStreak)
	}
	seqAcceptanceExact(t, "maxWinStreak", h.MaxWinStreak, maxWin)
	seqAcceptanceExact(t, "maxLossStreak", h.MaxLossStreak, maxLoss)
	seqAcceptanceNear(t, "worstLoss", h.WorstLoss, worst)
	n := len(trades)
	nonWinners := n - wins
	checkOptional := func(name string, got *float64, reason string, wantValue float64, wantReason string) {
		t.Helper()
		if wantReason != "" {
			if got != nil || reason != wantReason {
				t.Errorf("%s = %v reason %q, want null with reason %q", name, got, reason, wantReason)
			}
			return
		}
		if got == nil || reason != "" {
			t.Errorf("%s = %v reason %q, want a number", name, got, reason)
			return
		}
		seqAcceptanceNear(t, name, *got, wantValue)
	}
	if n == 0 {
		checkOptional("winRate", h.WinRate, h.WinRateReason, 0, "no-trades")
		checkOptional("profitFactor", h.ProfitFactor, h.ProfitFactorReason, 0, "no-trades")
		checkOptional("expectancy", h.Expectancy, h.ExpectancyReason, 0, "no-trades")
		checkOptional("avgHoldBars", h.AvgHoldBars, h.AvgHoldBarsReason, 0, "no-trades")
		checkOptional("avgWin", h.AvgWin, h.AvgWinReason, 0, "no-winning-trades")
		checkOptional("avgLoss", h.AvgLoss, h.AvgLossReason, 0, "no-nonwinning-trades")
		return
	}
	checkOptional("winRate", h.WinRate, h.WinRateReason, float64(wins)/float64(n)*100, "")
	checkOptional("expectancy", h.Expectancy, h.ExpectancyReason, sum/float64(n), "")
	checkOptional("avgHoldBars", h.AvgHoldBars, h.AvgHoldBarsReason, hold/float64(n), "")
	switch {
	case grossLoss > 0:
		checkOptional("profitFactor", h.ProfitFactor, h.ProfitFactorReason, grossWin/grossLoss, "")
	case grossWin > 0:
		checkOptional("profitFactor", h.ProfitFactor, h.ProfitFactorReason, 0, "no-losses")
	default:
		checkOptional("profitFactor", h.ProfitFactor, h.ProfitFactorReason, 0, "")
	}
	if wins > 0 {
		checkOptional("avgWin", h.AvgWin, h.AvgWinReason, grossWin/float64(wins), "")
	} else {
		checkOptional("avgWin", h.AvgWin, h.AvgWinReason, 0, "no-winning-trades")
	}
	if nonWinners > 0 {
		checkOptional("avgLoss", h.AvgLoss, h.AvgLossReason, -grossLoss/float64(nonWinners), "")
	} else {
		checkOptional("avgLoss", h.AvgLoss, h.AvgLossReason, 0, "no-nonwinning-trades")
	}
}

func seqMarketBinding(t *testing.T) []seqAcceptanceCase {
	t.Helper()
	var out []seqAcceptanceCase
	for _, c := range seqAcceptanceLoad(t, filepath.Join("testdata", "sequential_execution_e1_e2_v1")) {
		if c.Status == "binding" && (c.Policy == "E1" || c.Policy == "E2") {
			out = append(out, c)
		}
	}
	return out
}

func TestSequentialMarketAcceptanceReplay(t *testing.T) {
	cases := seqMarketBinding(t)
	if len(cases) == 0 {
		t.Fatal("no binding cases loaded")
	}
	for _, c := range cases {
		for _, side := range []string{"long", "short"} {
			c, side, v := c, side, c.Variants[side]
			t.Run(c.ID+"/"+side, func(t *testing.T) {
				bars := seqAcceptanceBars(t, c, v)
				var cfg seqAcceptanceConfig
				seqAcceptanceReadJSON(t, c.Config, &cfg)
				costs, err := seqAcceptanceCosts(c.Policy, cfg)
				if err != nil {
					t.Fatal(err)
				}
				fixture := seqMarketFixture(t, c.ID+"."+side, c.Series.Symbol, c.Series.Timeframe, bars, costs)
				run, runErr := engine.RunSequentialMarketFixture(fixture, seqMarketSource(c.Series.Symbol, c.Series.Timeframe, c.Policy, cfg.Risk, cfg.MaxNotional))
				envelope := seqAcceptanceEnvelope{Schema: "dsl-conformance-trades-v1", Case: fixture.Case, StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs}
				seqAcceptanceResult(t, c, v, cfg, bars, envelope, run.Run, runErr)
				if runErr != nil {
					if run.Run.Trades != nil || len(run.Accounting.Marks) != 0 {
						t.Fatal("refusal leaked a partial run or accounting")
					}
					return
				}
				var want seqAcceptanceOK
				seqAcceptanceReadJSON(t, v.Expected, &want)
				seqMarketCheckMetrics(t, run, bars, seqMarketExpectedTrades(want, 1), 0)
			})
		}
	}
}

// seqMarketRoutes are invented venues. Start times are Monday 00:00 UTC
// 2024-03-04. Scales only change the price level; no calendar is implied.
var seqMarketRoutes = []struct {
	symbol, timeframe string
	timeframeMS       int64
	scale             float64
}{
	{"EURUSD", "1m", 60_000, 0.011},
	{"GBPUSD", "5m", 300_000, 0.0127},
	{"USDJPY", "15m", 900_000, 1.5},
	{"AUDUSD", "30m", 1_800_000, 0.0066},
	{"XAUUSD", "1h", 3_600_000, 21.5},
	{"GER40", "4h", 14_400_000, 180},
	{"SPX500", "1d", 86_400_000, 52},
}

const seqMarketMonday = 1709510400000.0

var seqMarketMatrixCases = map[string]bool{
	"e1.target_basic": true, "e1.stop_basic": true, "e1.stop_target_same_bar": true, "e1.entry_bar_both": true,
	"e1.time_exit_open_e4": true, "e1.time_exit_gap_below_stop": true, "e1.stop_gap_open_through": true,
	"e1.gap_up_entry": true, "e1.slip_large": true, "e1.cap_binds_reduces_size": true, "e1.wrong_side_gap_through": true,
	"e1.no_next_bar": true, "e1.delayed_age1": true, "e1.delayed_age4_fill_outside_window": true,
	"e2.target_basic": true, "e2.time_exit_before_stop": true, "e2.stop_gap_open_through": true, "e2.slip_target_from_fill": true,
}

func TestSequentialMarketRouteAndUnitMatrix(t *testing.T) {
	ran := 0
	for _, c := range seqMarketBinding(t) {
		if !seqMarketMatrixCases[c.ID] {
			continue
		}
		for _, side := range []string{"long", "short"} {
			for _, route := range seqMarketRoutes {
				for _, feePerUnit := range []float64{0, 0.03} {
					c, side, route, feePerUnit, v := c, side, route, feePerUnit, c.Variants[side]
					t.Run(fmt.Sprintf("%s/%s/%s_%s/fee%g", c.ID, side, route.symbol, route.timeframe, feePerUnit), func(t *testing.T) {
						base := seqAcceptanceBars(t, c, v)
						var want seqAcceptanceOK
						seqAcceptanceReadJSON(t, v.Expected, &want)
						if c.AllowNonpositivePrices != nil && *c.AllowNonpositivePrices {
							t.Skip("nonpositive prices are not re-priced")
						}
						bars := make([]marketdata.Bar, len(base))
						for i, b := range base {
							bars[i] = marketdata.Bar{T: seqMarketMonday + float64(i)*float64(route.timeframeMS), O: b.O * route.scale, H: b.H * route.scale, L: b.L * route.scale, C: b.C * route.scale, V: 1}
						}
						var cfg seqAcceptanceConfig
						seqAcceptanceReadJSON(t, c.Config, &cfg)
						costs, err := seqAcceptanceCosts(c.Policy, cfg)
						if err != nil {
							t.Fatal(err)
						}
						costs.Slippage *= route.scale
						costs.FeePerUnit = feePerUnit * route.scale
						fixture := seqMarketFixture(t, c.ID, route.symbol, route.timeframe, bars, costs)
						run, err := engine.RunSequentialMarketFixture(fixture, seqMarketSource(route.symbol, route.timeframe, c.Policy, cfg.Risk, cfg.MaxNotional))
						if err != nil {
							var refused seqAcceptanceRefused
							if json.Unmarshal(v.Expected, &refused); refused.Outcome == "refused" {
								return
							}
							t.Fatalf("unexpected refusal: %v", err)
						}
						if run.Route.Symbol != route.symbol || run.Route.Timeframe != route.timeframe || run.Route.TimeframeMS != route.timeframeMS {
							t.Fatalf("route %+v", run.Route)
						}
						seqAcceptanceExact(t, "audit.symbol", run.Run.SequentialFull.Symbol, route.symbol)
						seqAcceptanceExact(t, "audit.timeframe", run.Run.SequentialFull.Timeframe, route.timeframe)
						expected := seqMarketExpectedTrades(want, route.scale)
						seqAcceptanceExact(t, "tradeCount", run.Run.TradeCount, len(expected))
						for i, g := range run.Run.Trades {
							w := expected[i]
							seqAcceptanceExact(t, "entryIndex", g.EntryIndex, w.entryIndex)
							seqAcceptanceExact(t, "exitIndex", g.ExitIndex, w.exitIndex)
							seqAcceptanceNear(t, "entry", g.Entry, w.fill)
							seqAcceptanceNear(t, "exit", g.Exit, w.exit)
							seqAcceptanceNear(t, "size", g.Size, w.size)
							seqAcceptanceNear(t, "exit credit", g.PnL, w.grossPnL-costs.FeePerUnit*w.size)
						}
						seqMarketCheckMetrics(t, run, bars, expected, costs.FeePerUnit)
						ran++
					})
				}
			}
		}
	}
	if ran == 0 && !t.Failed() {
		t.Fatal("matrix executed no case")
	}
}
