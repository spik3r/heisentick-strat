package regime

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/dsl"
)

func validateRequest(r Request) error {
	for _, t := range []int64{r.WarmupFromT, r.TradeFromT, r.TradeToT} {
		if t < 0 || t > 9007199254740991 || t%M30MS != 0 {
			return fmt.Errorf("regime endpoints must be exact nonnegative UTC M30 boundaries")
		}
	}
	if r.WarmupFromT >= r.TradeFromT || r.TradeFromT >= r.TradeToT {
		return fmt.Errorf("regime requires warmup < trade from < trade to")
	}
	if r.Costs.InitialEquity != 10000 || r.Costs.FeePerUnitSide != .5 || (r.Costs.Spread != 0 && r.Costs.Spread != 1) {
		return fmt.Errorf("regime fixed study requires equity10000, fee0.5/unit/side and spread0 or1")
	}
	return nil
}

// Run executes a closed, fixed historical study through the dedicated Go API.
// It neither places orders nor routes into the ordinary generic engine.
func Run(r Request) (Result, error) {
	spec, err := dsl.DecodeRegimeEngine(r.Config)
	if err != nil {
		return Result{}, err
	}
	if err = validateRequest(r); err != nil {
		return Result{}, err
	}
	bars, used, err := aggregate(r.M5, r.WarmupFromT, r.TradeToT)
	if err != nil {
		return Result{}, err
	}
	indicators, err := calculate(bars)
	if err != nil {
		return Result{}, err
	}
	out, err := execute(r, spec.Mode, indicators)
	if err != nil {
		return Result{}, err
	}
	out.UsedM5Rows = used
	// This also rejects every accidental NaN/Inf in audit or derived cash.
	if _, err = json.Marshal(out); err != nil {
		return Result{}, fmt.Errorf("regime nonfinite output: %w", err)
	}
	return out, nil
}

func literalOrders(anchor, atr, st float64, d int) (float64, float64) {
	if d == 1 {
		return math.Max(anchor-2*atr, st), anchor + 3.5*atr
	}
	return math.Min(anchor+2*atr, st), anchor - 3.5*atr
}

func openPosition(signal Signal, b NativeBar, mode string, cost Costs, equity float64) (*Position, float64, error) {
	d := signal.Direction
	entry := b.Open
	if d == 1 {
		entry += cost.Spread
	}
	if !finite(equity) || equity <= 0 || !finite(entry) || entry <= 0 {
		return nil, equity, fmt.Errorf("regime entry requires positive finite equity and price")
	}
	qty := equity * .1 / entry
	fee := qty * cost.FeePerUnitSide
	if !finite(qty) || qty <= 0 || !finite(fee) {
		return nil, equity, fmt.Errorf("regime quantity/fee overflow")
	}
	p := &Position{Signal: signal, EntryBucketT: b.BucketT, EntryT: b.FirstObservedT, Entry: entry, Quantity: qty, EntryFee: fee, AnchorError: entry - signal.Anchor, NakedEntryBar: mode == SourceMode}
	if mode == AuditMode {
		distance := 2 * signal.ATR
		structural := float64(d) * (signal.Anchor - signal.Supertrend)
		if structural > 0 {
			distance = math.Min(distance, structural)
		}
		p.InitialDistance = pointer(distance)
		p.SL = pointer(entry - float64(d)*distance)
		p.TP = pointer(entry + float64(d)*3.5*signal.ATR)
		p.FirstLiveSL = pointer(*p.SL)
		p.FirstLiveTP = pointer(*p.TP)
		p.FirstLiveT = p.EntryT
	}
	return p, equity - fee, nil
}

// barrier resolves orders already active at the opening. Wrong-side stop
// activation is handled first by the caller, independently of opening gaps.
func barrier(b NativeBar, sl, tp float64, d int, spread float64) (price float64, reason string, when int64) {
	o, h, l := b.Open, b.High, b.Low
	if d == -1 {
		o += spread
		h += spread
		l += spread
	}
	if float64(d)*(o-sl) <= 0 {
		return o, "stop_gap", b.FirstObservedT
	}
	if float64(d)*(o-tp) >= 0 {
		return o, "target_gap", b.FirstObservedT
	}
	stop, target := l <= sl, h >= tp
	if d == -1 {
		stop, target = h >= sl, l <= tp
	}
	if stop && target {
		firstHigh := math.Abs(o-h) < math.Abs(o-l)
		targetFirst := firstHigh
		if d == -1 {
			targetFirst = !firstHigh
		}
		if targetFirst {
			return tp, "target_ambiguous", b.CloseT
		}
		return sl, "stop_ambiguous", b.CloseT
	}
	if stop {
		return sl, "stop", b.CloseT
	}
	if target {
		return tp, "target", b.CloseT
	}
	return 0, "", 0
}

func manage(p *Position, row IndicatorRow, previousRegime int, mode string, spread float64) (OrderEdit, error) {
	if row.ATR14 == nil || row.Supertrend == nil {
		return OrderEdit{}, fmt.Errorf("regime position lacks completed indicators")
	}
	d := p.Signal.Direction
	oldSL, oldTP := p.SL, p.TP
	newSL, newTP := 0., 0.
	qclose := row.Close
	if d == -1 {
		qclose += spread
	}
	if mode == SourceMode {
		newSL, newTP = literalOrders(p.Signal.Anchor, *row.ATR14, *row.Supertrend, d)
		if float64(d)*(qclose-newSL) <= 0 {
			p.ForcedReason = "wrong_side_stop"
		}
	} else {
		newSL, newTP = *p.SL, *p.TP
		if row.Regime != previousRegime && d != -row.Regime {
			p.ForcedReason = "explicit_regime_flip"
		} else if d == -row.Regime && float64(d)*(row.Close-*row.Supertrend) > 0 {
			if d == 1 {
				newSL = math.Max(newSL, *row.Supertrend)
			} else {
				newSL = math.Min(newSL, *row.Supertrend)
			}
			if float64(d)*(qclose-newSL) <= 0 {
				p.ForcedReason = "tightened_stop_at_market"
			}
		}
	}
	if !finite(newSL) || !finite(newTP) {
		return OrderEdit{}, fmt.Errorf("regime nonfinite order edit")
	}
	widened := oldSL != nil && float64(d)*(newSL-*oldSL) < -1e-9
	changed := oldTP != nil && math.Abs(newTP-*oldTP) > 1e-9
	marketable := float64(d)*(qclose-newTP) >= 0
	if widened {
		p.StopWidenings++
	}
	if changed {
		p.TargetChanges++
	}
	if marketable {
		p.MarketableTargetEdits++
	}
	p.SL, p.TP = pointer(newSL), pointer(newTP)
	if p.FirstLiveSL == nil {
		p.FirstLiveSL = pointer(newSL)
		p.FirstLiveTP = pointer(newTP)
		p.FirstLiveT = row.CloseT
	}
	return OrderEdit{Time: row.CloseT, EntryT: p.EntryT, Direction: d, Close: row.Close, ATR: *row.ATR14, Supertrend: *row.Supertrend, OldSL: oldSL, OldTP: oldTP, NewSL: newSL, NewTP: newTP, StopWidened: widened, TargetChanged: changed, TargetMarketable: marketable, ForcedReason: p.ForcedReason}, nil
}

func execute(r Request, mode string, rows []IndicatorRow) (Result, error) {
	out := Result{Symbol: "XAUUSD", SourceTimeframe: "5m", NativeTimeframe: "30m", Schema: "strat-regime-engine-report-v1", Mode: mode, ExecutionModel: "native-m30-ohlc-path-next-observed-open", QuantityModel: "continuous-10pct-equity-notional-pointvalue1", WarmupFromT: r.WarmupFromT, TradeFromT: r.TradeFromT, TradeToT: r.TradeToT, Costs: r.Costs, Indicators: rows, Signals: []Signal{}, Trades: []Trade{}, Edits: []OrderEdit{}, Assumptions: []string{
		"Owned reference OHLCV; provider, executable quote side and volume units unverified.",
		"Spread scenarios conditionally treat reference bars as bid-style; this is not verified feed identity.",
		"Pointvalue1 and one chart unit=one ounce are illustrative; continuous quantity, no lot/tick rounding.",
		"Fee0.50 per unit per side; financing, swaps, slippage and broker contract costs unmodeled.",
		"Native OHLC path: high first only if strictly nearer open, ties low first; hit times interval-censored to close.",
		"HMA odd halves floor; no Pine compilation or TradingView parity established.",
		"Source-like mode retains non-latched newly marketable target behavior from the prior offline model.",
		"Pending entries never canceled using future bar completeness; partial fills use actual observed opening timestamp.",
		"Closed drawdown excludes unrealized path drawdown; final exposure stays open and is marked separately.",
	}}
	equity := r.Costs.InitialEquity
	var p *Position
	var pending *Signal
	closePosition := func(price float64, when int64, reason string) error {
		if p == nil {
			return fmt.Errorf("regime close without position")
		}
		gross := float64(p.Signal.Direction) * (price - p.Entry)
		net := gross - 2*r.Costs.FeePerUnitSide
		equity += p.Quantity * (gross - r.Costs.FeePerUnitSide)
		if !finite(price) || !finite(equity) || !finite(net*p.Quantity) {
			return fmt.Errorf("regime exit overflow")
		}
		out.Trades = append(out.Trades, Trade{Position: *p, ExitT: when, Exit: price, Reason: reason, GrossPerUnit: gross, NetPerUnit: net, Net: net * p.Quantity, ExitFee: p.Quantity * r.Costs.FeePerUnitSide, EquityAfter: equity})
		p = nil
		return nil
	}
	for i, row := range rows {
		if row.BucketT < r.TradeFromT {
			continue
		}
		if row.BucketT >= r.TradeToT {
			break
		}
		if pending != nil {
			var err error
			p, equity, err = openPosition(*pending, row.NativeBar, mode, r.Costs, equity)
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
					q += r.Costs.Spread
				}
				if err := closePosition(q, row.FirstObservedT, p.ForcedReason); err != nil {
					return Result{}, err
				}
			} else {
				if p.NakedEntryBar && row.BucketT == p.EntryBucketT {
					shadow, _ := literalOrders(p.Signal.Anchor, p.Signal.ATR, p.Signal.Supertrend, p.Signal.Direction)
					if p.Signal.Direction == 1 {
						p.ShadowStopHit = row.Low <= shadow
					} else {
						p.ShadowStopHit = row.High+r.Costs.Spread >= shadow
					}
				}
				if p.SL != nil {
					price, reason, when := barrier(row.NativeBar, *p.SL, *p.TP, p.Signal.Direction, r.Costs.Spread)
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
		if row.Candidate != 0 && row.CloseT < r.TradeToT {
			if row.ATR14 == nil || row.Supertrend == nil || row.VolumeMean == nil || *row.VolumeMean <= 0 {
				return Result{}, fmt.Errorf("invalid regime candidate indicators")
			}
			s := Signal{Time: row.CloseT, Direction: row.Candidate, Anchor: row.Close, ATR: *row.ATR14, Supertrend: *row.Supertrend, Regime: row.Regime, Volume: row.Volume, VolumeRatio: row.Volume / *row.VolumeMean, Accepted: p == nil}
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
			edit, err := manage(p, row, prev, mode, r.Costs.Spread)
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
	summarize(&out, equity)
	return out, nil
}

func summarize(out *Result, cash float64) {
	s := &out.Summary
	s.Trades = len(out.Trades)
	equity, peak := out.Costs.InitialEquity, out.Costs.InitialEquity
	unitProfit, unitLoss := 0., 0.
	wins := 0
	positive := []float64{}
	for _, t := range out.Trades {
		s.Net += t.Net
		equity += t.Net
		peak = math.Max(peak, equity)
		s.ClosedEquityDD = math.Max(s.ClosedEquityDD, peak-equity)
		if t.Net > 0 {
			s.GrossProfit += t.Net
			wins++
			positive = append(positive, t.Net)
		} else if t.Net < 0 {
			s.GrossLoss -= t.Net
		}
		if t.NetPerUnit > 0 {
			unitProfit += t.NetPerUnit
		} else if t.NetPerUnit < 0 {
			unitLoss -= t.NetPerUnit
		}
	}
	if s.GrossLoss > 0 {
		s.PF = pointer(s.GrossProfit / s.GrossLoss)
	}
	if unitLoss > 0 {
		s.EqualUnitPF = pointer(unitProfit / unitLoss)
	}
	if len(out.Trades) > 0 {
		s.WinRate = pointer(float64(wins) / float64(len(out.Trades)))
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(positive)))
	s.NetWithoutBest5 = s.Net
	for i := 0; i < min(5, len(positive)); i++ {
		s.NetWithoutBest5 -= positive[i]
	}
	s.ClosedEquity = out.Costs.InitialEquity + s.Net
	s.CashEquity = cash
	if len(out.Indicators) > 0 {
		last := out.Indicators[len(out.Indicators)-1]
		s.FinalMarkT = last.CloseT
		if p := out.OpenPosition; p != nil {
			q := last.Close
			if p.Signal.Direction == -1 {
				q += out.Costs.Spread
			}
			s.FinalLiquidationReference = pointer(q)
			s.OpenEntryFee = p.EntryFee
			s.UnrealizedGross = float64(p.Signal.Direction) * (q - p.Entry) * p.Quantity
			s.HypotheticalClosingFee = p.Quantity * out.Costs.FeePerUnitSide
		}
	}
	s.MarkedEquity = cash + s.UnrealizedGross
	s.MarkedEquityAfterClosingFee = s.MarkedEquity - s.HypotheticalClosingFee
}
