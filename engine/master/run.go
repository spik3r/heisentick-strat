package master

import (
	"encoding/json"
	"fmt"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine/regime"
	"math"
)

// QuantityBelowStepError terminates a run before a position or fee is created.
// It deliberately refuses the Python reference's zero-quantity fake position.
type QuantityBelowStepError struct{ Equity, Entry float64 }

func (e *QuantityBelowStepError) Error() string {
	return "quantity_below_step: master rounded quantity is nonpositive"
}

func validateRequest(r Request) error {
	for _, t := range []int64{r.WarmupFromT, r.TradeFromT, r.TradeToT} {
		if t < 0 || t > 9007199254740991 || t%M30MS != 0 {
			return fmt.Errorf("master endpoints must be exact nonnegative UTC M30 boundaries")
		}
	}
	if r.WarmupFromT >= r.TradeFromT || r.TradeFromT >= r.TradeToT {
		return fmt.Errorf("master requires warmup < trade from < trade to")
	}
	if r.Costs.InitialEquity != 10000 || r.Costs.FeePerUnitSide != .5 || (r.Costs.Spread != 0 && r.Costs.Spread != 1) {
		return fmt.Errorf("master fixed study requires equity10000, fee0.5/unit/side and spread0 or1")
	}
	return nil
}

// Run executes exactly one fixed native M30 reference policy. It performs no
// external orders, registration, release or generic-broker execution.
func Run(r Request) (Result, error) {
	spec, err := dsl.DecodeMasterStructural(r.Config)
	if err != nil {
		return Result{}, err
	}
	if err = validateRequest(r); err != nil {
		return Result{}, err
	}
	rows, h4, used, err := buildIndicators(r)
	if err != nil {
		return Result{}, err
	}
	out, err := execute(r, spec.Mode, rows)
	if err != nil {
		return Result{}, err
	}
	out.H4 = h4
	out.UsedM5Rows = used
	if _, err = json.Marshal(out); err != nil {
		return Result{}, fmt.Errorf("master nonfinite output: %w", err)
	}
	return out, nil
}

func literalOrders(anchor, atr, st float64, d int, locked bool) (sl, tp, base float64) {
	base = math.Max(anchor-2*atr, st)
	if d == -1 {
		base = math.Min(anchor+2*atr, st)
	}
	sl = base
	if locked {
		sl = st
	}
	return sl, anchor + float64(d)*4*atr, base
}

func openPosition(s Signal, b NativeBar, mode string, cost Costs, equity float64) (*Position, float64, error) {
	entry := b.Open
	if s.Direction == 1 {
		entry += cost.Spread
	}
	if !finite(equity) || equity <= 0 || !finite(entry) || entry <= 0 {
		return nil, equity, fmt.Errorf("master entry requires positive finite equity and price")
	}
	raw := equity * .1 / entry
	qty := math.Floor((raw+1e-12)/.1) * .1
	if !finite(qty) {
		return nil, equity, fmt.Errorf("master quantity overflow")
	}
	if qty <= 0 {
		return nil, equity, &QuantityBelowStepError{Equity: equity, Entry: entry}
	}
	fee := qty * cost.FeePerUnitSide
	if !finite(fee) || !finite(equity-fee) {
		return nil, equity, fmt.Errorf("master entry fee overflow")
	}
	p := &Position{Signal: s, EntryBucketT: b.BucketT, EntryT: b.FirstObservedT, Entry: entry, Quantity: qty, EntryFee: fee, AnchorError: entry - s.Anchor, NakedEntryBar: mode == SourceMode, MaxHigh: s.High, MinLow: s.Low}
	if mode == ProtectedMode {
		distance := 2 * s.ATR
		structural := float64(s.Direction) * (s.Anchor - s.Supertrend)
		if structural > 0 {
			distance = math.Min(distance, structural)
		}
		if !finite(distance) || distance <= 0 {
			return nil, equity, fmt.Errorf("master invalid initial protection distance")
		}
		p.InitialDistance = pointer(distance)
		p.SL = pointer(entry - float64(s.Direction)*distance)
		p.TP = pointer(entry + float64(s.Direction)*4*s.ATR)
		if !finite(*p.SL) || !finite(*p.TP) {
			return nil, equity, fmt.Errorf("master initial bracket overflow")
		}
		p.FirstLiveSL = pointer(*p.SL)
		p.FirstLiveTP = pointer(*p.TP)
		p.FirstLiveT = p.EntryT
		// Before a completed post-fill bar the only observed position price is fill.
		// First management replaces these with the completed post-fill extrema.
		p.MaxHigh = entry
		p.MinLow = entry
	}
	return p, equity - fee, nil
}

func manage(p *Position, row IndicatorRow, previousRegime int, mode string, spread float64) (OrderEdit, error) {
	if row.ATR14 == nil || row.Supertrend == nil {
		return OrderEdit{}, fmt.Errorf("master position lacks completed indicators")
	}
	d := p.Signal.Direction
	first := p.PostHigh == nil
	hi, lo := row.High, row.Low
	if !first {
		hi = math.Max(hi, *p.PostHigh)
		lo = math.Min(lo, *p.PostLow)
	}
	p.PostHigh = pointer(hi)
	p.PostLow = pointer(lo)
	if mode == ProtectedMode {
		p.MaxHigh = hi
		p.MinLow = lo
	} else {
		p.MaxHigh = math.Max(p.MaxHigh, row.High)
		p.MinLow = math.Min(p.MinLow, row.Low)
	}
	oldSL, oldTP := p.SL, p.TP
	qclose := row.Close
	if d == -1 {
		qclose += spread
	}
	previousLocked := p.Locked
	locked := p.MaxHigh >= p.Signal.Anchor+2**row.ATR14
	postOnly := hi >= p.Signal.Anchor+2**row.ATR14
	if d == -1 {
		locked = p.MinLow <= p.Signal.Anchor-2**row.ATR14
		postOnly = lo <= p.Signal.Anchor-2**row.ATR14
	}
	newSL, newTP, base := 0., 0., 0.
	rejected := false
	if mode == SourceMode {
		newSL, newTP, base = literalOrders(p.Signal.Anchor, *row.ATR14, *row.Supertrend, d, locked)
		if float64(d)*(qclose-newSL) <= 0 {
			p.ForcedReason = "wrong_side_stop"
		}
	} else {
		if oldSL == nil || oldTP == nil {
			return OrderEdit{}, fmt.Errorf("master protected position missing attached bracket")
		}
		hit := hi >= p.Entry+2*p.Signal.ATR
		if d == -1 {
			hit = lo <= p.Entry-2*p.Signal.ATR
		}
		locked = previousLocked || hit
		newSL, newTP, base = *oldSL, *oldTP, *oldSL
		if locked && d == -row.Regime && float64(d)*(row.Close-*row.Supertrend) > 0 {
			rejected = float64(d)*(*row.Supertrend-*oldSL) < -1e-9
			if d == 1 {
				newSL = math.Max(newSL, *row.Supertrend)
			} else {
				newSL = math.Min(newSL, *row.Supertrend)
			}
		}
		if row.Regime != previousRegime && d != -row.Regime {
			p.ForcedReason = "regime_flip_market_exit"
		}
	}
	if !finite(newSL) || !finite(newTP) {
		return OrderEdit{}, fmt.Errorf("master nonfinite order edit")
	}
	widened := oldSL != nil && float64(d)*(newSL-*oldSL) < -1e-9
	changed := oldTP != nil && math.Abs(newTP-*oldTP) > 1e-9
	became := locked && !previousLocked
	deactivated := previousLocked && !locked
	relaxation := mode == SourceMode && became && float64(d)*(*row.Supertrend-base) < -1e-9
	preentry := mode == SourceMode && became && !postOnly
	marketable := float64(d)*(qclose-newTP) >= 0
	if widened {
		p.StopWidenings++
	}
	if changed {
		p.TargetChanges++
	}
	if became {
		p.LockTransitions++
	}
	if deactivated {
		p.LockDeactivations++
	}
	if relaxation {
		p.HypotheticalLockRelaxations++
	}
	if preentry {
		p.PreentryOnlyActivations++
	}
	if rejected {
		p.RejectedWorseST++
	}
	if locked && float64(d)*(newSL-p.Entry) <= 0 {
		p.NonprofitLockEdits++
	}
	if marketable {
		p.MarketableTargetEdits++
	}
	p.Locked = locked
	p.SL = pointer(newSL)
	p.TP = pointer(newTP)
	if p.FirstLiveSL == nil {
		p.FirstLiveSL = pointer(newSL)
		p.FirstLiveTP = pointer(newTP)
		p.FirstLiveT = row.CloseT
	}
	return OrderEdit{OrderEdit: regime.OrderEdit{Time: row.CloseT, EntryT: p.EntryT, Direction: d, Close: row.Close, ATR: *row.ATR14, Supertrend: *row.Supertrend, OldSL: oldSL, OldTP: oldTP, NewSL: newSL, NewTP: newTP, StopWidened: widened, TargetChanged: changed, TargetMarketable: marketable, ForcedReason: p.ForcedReason}, Entry: p.Entry, Anchor: p.Signal.Anchor, UnlockedCandidate: base, PreviousLocked: previousLocked, Locked: locked, BecameLocked: became, Deactivated: deactivated, HypotheticalLockRelaxation: relaxation, RejectedWorseST: rejected, PreentryOnlyActivation: preentry, PostHigh: hi, PostLow: lo}, nil
}
