package master

import (
	"fmt"
	"github.com/spik3r/heisentick-strat/engine/regime"
	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
	"math"
	"sort"
	"time"
)

func execute(r Request, mode string, rows []IndicatorRow) (Result, error) {
	return executeArithmetic(r, mode, rows, false)
}

func executeArithmetic(r Request, mode string, rows []IndicatorRow, portable bool) (Result, error) {
	out := Result{Symbol: "XAUUSD", SourceTimeframe: "5m", NativeTimeframe: "30m", Schema: "strat-master-structural-report-v1", Mode: mode, ExecutionModel: "native-m30-ohlc-path-next-observed-open", QuantityModel: "floor-0.1-step-epsilon1e-12-10pct-equity-notional-pointvalue1", PriceModel: "continuous-reference-no-tick-rounding", WarmupFromT: r.WarmupFromT, TradeFromT: r.TradeFromT, TradeToT: r.TradeToT, Costs: r.Costs, Indicators: rows, Signals: []Signal{}, Trades: []Trade{}, Edits: []OrderEdit{}, Assumptions: []string{
		"Owned reference OHLCV; provider, executable quote side and volume units unverified.",
		"Spread scenarios conditionally treat reference bars as bid-style; this is not verified feed identity.",
		"Pointvalue1 and one chart unit=one ounce are illustrative; quantity floor((raw+1e-12)/0.1)*0.1; continuous prices without tick rounding.",
		"Fee0.50 per unit per side; financing, swaps, slippage and broker contract costs unmodeled.",
		"Native OHLC path: high first only if strictly nearer open, ties low first; hit times interval-censored to close.",
		"HMA odd halves floor; no Pine compilation or TradingView parity established.",
		"Source-like mode retains non-latched newly marketable target behavior from the prior offline model.",
		"Pending entries never canceled using future bar completeness; partial fills use actual observed opening timestamp.",
		"H4 phase0 UTC nominal close; SOURCE uses decision close, PROTECTED uses native bar open horizon.",
		"H4 default bearish direction before ST initialization is retained from Python; no native broker clock certificate.",
		"Protected mode is the declared frozen/sticky AUDIT_SAFE control; no source/Pine equivalence implied.",
		"Closed drawdown excludes unrealized path drawdown; final exposure stays open and is marked separately.",
	}}
	equity := r.Costs.InitialEquity
	var p *Position
	var pending *Signal
	closePosition := func(price float64, when int64, reason string) error {
		if p == nil {
			return fmt.Errorf("master close without position")
		}
		var gross, net, total, exitFee float64
		if portable {
			gross = fp.Mul(float64(p.Signal.Direction), fp.Sub(price, p.Entry))
			net = fp.Sub(gross, fp.Mul(2, r.Costs.FeePerUnitSide))
			equity = fp.Add(equity, fp.Mul(p.Quantity, fp.Sub(gross, r.Costs.FeePerUnitSide)))
			total = fp.Mul(net, p.Quantity)
			exitFee = fp.Mul(p.Quantity, r.Costs.FeePerUnitSide)
		} else {
			gross = float64(p.Signal.Direction) * (price - p.Entry)
			net = gross - 2*r.Costs.FeePerUnitSide
			equity += p.Quantity * (gross - r.Costs.FeePerUnitSide)
			if !finite(price) || !finite(equity) || !finite(net*p.Quantity) {
				return fmt.Errorf("master exit overflow")
			}
			total = net * p.Quantity
			exitFee = p.Quantity * r.Costs.FeePerUnitSide
		}
		out.Trades = append(out.Trades, Trade{Position: *p, ExitT: when, Exit: price, Reason: reason, GrossPerUnit: gross, NetPerUnit: net, Net: total, ExitFee: exitFee, EquityAfter: equity})
		p = nil
		return nil
	}
	for i, row := range rows {
		candidate, h4 := row.SourceCandidate, row.HistoricalH4
		if mode == ProtectedMode {
			candidate, h4 = row.ProtectedCandidate, row.StableH4
		}
		if row.BucketT < r.TradeFromT {
			continue
		}
		if row.BucketT >= r.TradeToT {
			break
		}
		if pending != nil {
			var err error
			p, equity, err = openPositionArithmetic(*pending, row.NativeBar, mode, r.Costs, equity, portable)
			if err != nil {
				return Result{}, err
			}
			pending = nil
			if !row.Complete {
				out.Summary.PartialEntryFills++
			}
		}
		if p != nil {
			if !row.Complete {
				p.PartialBars++
				out.Summary.PartialPositionBars++
			}
			if p.ForcedReason != "" {
				q := row.Open
				if p.Signal.Direction == -1 {
					if portable {
						q = fp.Add(q, r.Costs.Spread)
					} else {
						q += r.Costs.Spread
					}
				}
				if err := closePosition(q, row.FirstObservedT, p.ForcedReason); err != nil {
					return Result{}, err
				}
			} else {
				if p.SL != nil {
					var price float64
					var reason string
					var when int64
					if portable {
						var err error
						price, reason, when, err = regime.ReferenceBarrierPortableV1(row.NativeBar, *p.SL, *p.TP, p.Signal.Direction, r.Costs.Spread)
						if err != nil {
							return Result{}, err
						}
					} else {
						price, reason, when = regime.ReferenceBarrier(row.NativeBar, *p.SL, *p.TP, p.Signal.Direction, r.Costs.Spread)
					}
					if reason != "" {
						if err := closePosition(price, when, reason); err != nil {
							return Result{}, err
						}
					}
				}
			}
		}
		// The only source of signal/management data is this completed native close.
		// Start-flat and end-exclusion deliberately match the frozen reference.
		if candidate != 0 && row.CloseT < r.TradeToT {
			if row.ATR14 == nil || row.Supertrend == nil || row.VolumeMean == nil || *row.VolumeMean <= 0 {
				return Result{}, fmt.Errorf("invalid master candidate indicators")
			}
			if h4 == nil || row.ATRPercent == nil {
				return Result{}, fmt.Errorf("invalid master H4 candidate")
			}
			var ratio float64
			if portable {
				ratio = fp.Div(row.Volume, *row.VolumeMean)
			} else {
				ratio = row.Volume / *row.VolumeMean
			}
			s := Signal{Signal: regime.Signal{Time: row.CloseT, Direction: candidate, Anchor: row.Close, ATR: *row.ATR14, Supertrend: *row.Supertrend, Regime: row.Regime, Volume: row.Volume, VolumeRatio: ratio, Accepted: p == nil}, High: row.High, Low: row.Low, H4: *h4, ATRPercent: *row.ATRPercent}
			out.Signals = append(out.Signals, s)
			if s.Accepted {
				pending = &s
			}
		}
		if p != nil {
			prev := row.Regime
			if i > 0 {
				prev = rows[i-1].Regime
			}
			edit, err := manageArithmetic(p, row, prev, mode, r.Costs.Spread, portable)
			if err != nil {
				return Result{}, err
			}
			out.Edits = append(out.Edits, edit)
			if edit.TargetMarketable {
				out.Summary.MarketableTargetEdits++
			}
		}
	}
	out.OpenPosition, out.PendingSignal = p, pending
	summarizeArithmetic(&out, equity, portable)
	out.Monthly = monthlyArithmetic(out.Trades, portable)
	return out, nil
}

func summarize(out *Result, cash float64) { summarizeArithmetic(out, cash, false) }

func summarizeArithmetic(out *Result, cash float64, portable bool) {
	s := &out.Summary
	s.Trades = len(out.Trades)
	equity, peak := out.Costs.InitialEquity, out.Costs.InitialEquity
	unitProfit, unitLoss := 0., 0.
	wins := 0
	positive := []float64{}
	for _, t := range out.Trades {
		if portable {
			s.Net = fp.Add(s.Net, t.Net)
			equity = fp.Add(equity, t.Net)
		} else {
			s.Net += t.Net
			equity += t.Net
		}
		peak = math.Max(peak, equity)
		s.ClosedEquityDD = math.Max(s.ClosedEquityDD, plainDifference(peak, equity, portable))
		if t.Net > 0 {
			if portable {
				s.GrossProfit = fp.Add(s.GrossProfit, t.Net)
			} else {
				s.GrossProfit += t.Net
			}
			wins++
			positive = append(positive, t.Net)
		} else if t.Net < 0 {
			if portable {
				s.GrossLoss = fp.Sub(s.GrossLoss, t.Net)
			} else {
				s.GrossLoss -= t.Net
			}
		}
		if t.NetPerUnit > 0 {
			if portable {
				unitProfit = fp.Add(unitProfit, t.NetPerUnit)
			} else {
				unitProfit += t.NetPerUnit
			}
		} else if t.NetPerUnit < 0 {
			if portable {
				unitLoss = fp.Sub(unitLoss, t.NetPerUnit)
			} else {
				unitLoss -= t.NetPerUnit
			}
		}
	}
	if s.GrossLoss > 0 {
		if portable {
			s.PF = pointer(fp.Div(s.GrossProfit, s.GrossLoss))
		} else {
			s.PF = pointer(s.GrossProfit / s.GrossLoss)
		}
	}
	if unitLoss > 0 {
		if portable {
			s.EqualUnitPF = pointer(fp.Div(unitProfit, unitLoss))
		} else {
			s.EqualUnitPF = pointer(unitProfit / unitLoss)
		}
	}
	if len(out.Trades) > 0 {
		if portable {
			s.WinRate = pointer(fp.Div(float64(wins), float64(len(out.Trades))))
		} else {
			s.WinRate = pointer(float64(wins) / float64(len(out.Trades)))
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(positive)))
	s.NetWithoutBest5 = s.Net
	for i := 0; i < min(5, len(positive)); i++ {
		if portable {
			s.NetWithoutBest5 = fp.Sub(s.NetWithoutBest5, positive[i])
		} else {
			s.NetWithoutBest5 -= positive[i]
		}
	}
	if portable {
		s.ClosedEquity = fp.Add(out.Costs.InitialEquity, s.Net)
	} else {
		s.ClosedEquity = out.Costs.InitialEquity + s.Net
	}
	s.CashEquity = cash
	if len(out.Indicators) > 0 {
		last := out.Indicators[len(out.Indicators)-1]
		s.FinalMarkT = last.CloseT
		if p := out.OpenPosition; p != nil {
			q := last.Close
			if p.Signal.Direction == -1 {
				if portable {
					q = fp.Add(q, out.Costs.Spread)
				} else {
					q += out.Costs.Spread
				}
			}
			s.FinalLiquidationReference = pointer(q)
			s.OpenEntryFee = p.EntryFee
			if portable {
				s.UnrealizedGross = fp.Mul(fp.Mul(float64(p.Signal.Direction), fp.Sub(q, p.Entry)), p.Quantity)
				s.HypotheticalClosingFee = fp.Mul(p.Quantity, out.Costs.FeePerUnitSide)
			} else {
				s.UnrealizedGross = float64(p.Signal.Direction) * (q - p.Entry) * p.Quantity
				s.HypotheticalClosingFee = p.Quantity * out.Costs.FeePerUnitSide
			}
		}
	}
	if portable {
		s.MarkedEquity = fp.Add(cash, s.UnrealizedGross)
		s.MarkedEquityAfterClosingFee = fp.Sub(s.MarkedEquity, s.HypotheticalClosingFee)
	} else {
		s.MarkedEquity = cash + s.UnrealizedGross
		s.MarkedEquityAfterClosingFee = s.MarkedEquity - s.HypotheticalClosingFee
	}
}

func monthly(trades []Trade) []MonthlySummary { return monthlyArithmetic(trades, false) }

func monthlyArithmetic(trades []Trade, portable bool) []MonthlySummary {
	groups := map[string][]Trade{}
	for _, t := range trades {
		m := time.UnixMilli(t.EntryT).UTC().Format("2006-01")
		groups[m] = append(groups[m], t)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []MonthlySummary{}
	for _, k := range keys {
		s := MonthlySummary{EntryMonth: k, Trades: len(groups[k])}
		up, ul := 0., 0.
		for _, t := range groups[k] {
			if portable {
				s.Net = fp.Add(s.Net, t.Net)
			} else {
				s.Net += t.Net
			}
			if t.Net > 0 {
				if portable {
					s.GrossProfit = fp.Add(s.GrossProfit, t.Net)
				} else {
					s.GrossProfit += t.Net
				}
			} else if t.Net < 0 {
				if portable {
					s.GrossLoss = fp.Sub(s.GrossLoss, t.Net)
				} else {
					s.GrossLoss -= t.Net
				}
			}
			if t.NetPerUnit > 0 {
				if portable {
					up = fp.Add(up, t.NetPerUnit)
				} else {
					up += t.NetPerUnit
				}
			} else if t.NetPerUnit < 0 {
				if portable {
					ul = fp.Sub(ul, t.NetPerUnit)
				} else {
					ul -= t.NetPerUnit
				}
			}
		}
		if s.GrossLoss > 0 {
			if portable {
				s.PF = pointer(fp.Div(s.GrossProfit, s.GrossLoss))
			} else {
				s.PF = pointer(s.GrossProfit / s.GrossLoss)
			}
		}
		if ul > 0 {
			if portable {
				s.EqualUnitPF = pointer(fp.Div(up, ul))
			} else {
				s.EqualUnitPF = pointer(up / ul)
			}
		}
		out = append(out, s)
	}
	return out
}
