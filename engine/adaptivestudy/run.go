package adaptivestudy

import (
	"fmt"
	"math"
	"time"

	"github.com/spik3r/heisentick-strat/engine/regimeadaptive"
	fp "github.com/spik3r/heisentick-strat/internal/float64contract"
)

func feeTerms(cost string) (float64, float64) {
	switch cost {
	case "BASIC":
		return .095, 0
	case "HARSH":
		return .155, 0
	case "SOURCE_FEE":
		return 0, .25
	}
	return 0, 0
}
func fee(cost string, qty float64) float64 { a, b := feeTerms(cost); return fp.Add(fp.Mul(a, qty), b) }
func ceilDistance(atr, multiple float64) float64 {
	steps := math.Max(1, math.Ceil(fp.Sub(fp.Div(fp.Mul(multiple, atr), .01), 1e-9)))
	return fp.Mul(.01, steps)
}

func signalQty(equity, price, dStop float64, cost string) float64 {
	a, b := feeTerms(cost)
	if equity <= fp.Mul(2, b) {
		return 0
	}
	risk := fp.Div(fp.Mul(equity, .005), dStop)
	cap := fp.Div(fp.Sub(equity, fp.Mul(2, b)), fp.Add(price, fp.Mul(2, a)))
	return fp.Mul(.001, math.Floor(fp.Add(fp.Div(math.Min(risk, cap), .001), 1e-9)))
}

// Tick indices are exact integers in the admitted research price domain. Convert
// once by division by100 so decimal-grid OHLC touches compare to the same binary64
// value. Raw fill-distance subtraction can land one ULP outside the intended grid.
func bracketLevels(fill, stopDistance, trendDistance float64, side int) (float64, float64) {
	ticks := math.Round(fp.Mul(fill, 100))
	stopTicks := math.Round(fp.Mul(stopDistance, 100))
	trendTicks := math.Round(fp.Mul(trendDistance, 100))
	if ticks > 9007199254740991 || stopTicks > 9007199254740991 || trendTicks > 9007199254740991 {
		panic(&fp.Error{Operation: "research tick index outside exact integer domain"})
	}
	f, s, t := int64(ticks), int64(stopTicks), int64(trendTicks)
	stop, target := f-int64(side)*s, f+int64(side)*t
	if stop < -9007199254740991 || target < -9007199254740991 || stop > 9007199254740991 || target > 9007199254740991 {
		panic(&fp.Error{Operation: "research bracket tick index outside exact integer domain"})
	}
	return fp.Div(float64(stop), 100), fp.Div(float64(target), 100)
}

func adverse(price float64, side int, stress string, marketOrStop bool) float64 {
	if marketOrStop && stress == "one-tick" {
		price = fp.Add(price, fp.Mul(float64(side), .01))
	}
	if side > 0 {
		return fp.Div(math.Ceil(fp.Sub(fp.Mul(price, 100), 1e-9)), 100)
	}
	return fp.Div(math.Floor(fp.Add(fp.Mul(price, 100), 1e-9)), 100)
}
func toward(price float64, side int) float64 {
	if side > 0 {
		return fp.Div(math.Floor(fp.Add(fp.Mul(price, 100), 1e-9)), 100)
	}
	return fp.Div(math.Ceil(fp.Sub(fp.Mul(price, 100), 1e-9)), 100)
}
func validConfig(c Config) error {
	if c.Candidate != "B0" && c.Candidate != "B1" && c.Candidate != "B2" {
		return fmt.Errorf("candidate must be B0/B1/B2")
	}
	if c.Cost != "GROSS" && c.Cost != "BASIC" && c.Cost != "HARSH" && c.Cost != "SOURCE_FEE" {
		return fmt.Errorf("unknown cost")
	}
	if c.Stress != "none" && c.Stress != "one-tick" && c.Stress != "target-first" && c.Stress != "skip-entry-gap" {
		return fmt.Errorf("unknown stress")
	}
	if c.StartT < 0 || c.EndT <= c.StartT || c.EndT > 9007199254740991 {
		return fmt.Errorf("invalid study window")
	}
	return nil
}

// Run requires500 prior bars and only trades bars fully closed within the window.
// It truncates future bars before indicator computation; every invocation starts flat.
func Run(data VerifiedM30, c Config) (Result, error) {
	if data.hash != M30Hash {
		return Result{}, fmt.Errorf("verified M30 data required")
	}
	frozen := false
	for _, name := range []string{"development", "2024", "2025", "2026", "train2024", "train2025", "train2026"} {
		a, z, _ := StudyWindow(name)
		if c.StartT == a && c.EndT == z {
			frozen = true
		}
	}
	if !frozen {
		return Result{}, fmt.Errorf("only frozen market windows admitted")
	}
	bars := data.bars
	if err := validConfig(c); err != nil {
		return Result{}, err
	}
	start, end := 0, 0
	for start < len(bars) && bars[start].T < c.StartT {
		start++
	}
	end = start
	for end < len(bars) && bars[end].CloseT <= c.EndT {
		end++
	}
	if start < 500 || end == start {
		return Result{}, fmt.Errorf("need500 prior M30 warmup bars and nonempty evaluation")
	}
	selected := bars[start-500 : end]
	rows, err := regimeadaptive.ResearchRows(selected, c.Candidate == "B2")
	if err != nil {
		return Result{}, err
	}
	result, err := runRows(selected, rows, 500, c)
	if err != nil {
		return Result{}, err
	}
	result.DataHash = data.hash
	result.Status = "retrospective-assumed-model-not-Pine-parity"
	return result, nil
}

// runRows is package-private for manual supplied-signal mechanics fixtures. The CLI
// cannot bypass causal Go indicators or admit synthetic rows as market outcomes.
func runRows(bars []Bar, rows []regimeadaptive.Row, start int, c Config) (result Result, err error) {
	defer func() {
		if err != nil {
			result = Result{}
		}
	}()
	defer fp.Recover(&err)
	if e := validConfig(c); e != nil {
		return result, e
	}
	if len(rows) != len(bars) || start < 0 || start >= len(bars) {
		return result, fmt.Errorf("invalid synthetic mechanics shape")
	}
	result = Result{Schema: "adaptive-study-result-v1", Profile: Profile, Status: "synthetic-mechanics-only-not-Pine-parity", Config: c, SourceHash: regimeadaptive.SourceHash, Warmup: start, Bars: len(bars) - start, Trades: []Trade{}, Events: []Event{}, Equity: []EquityPoint{}, Diagnostics: Diagnostics{BranchSignals: map[string]int{}, BranchFills: map[string]int{}, BranchNet: map[string]float64{}}, Limitations: []string{"normalized research units/pointValue1/tick.01/quantityStep.001; not verified gold lots/dollars", "unknown provider/quote side/volume; all dates previously researched; no untouched holdout", "OHLC stage ordering assumed; intrabarT=null; no tick/Pine parity", "fixed/proxy cash costs modeled, not measured; pre-financing and funding unknown", "no additions/reversal/bracket movement; indicator state retains observed gaps without interpolation"}}
	d := &result.Diagnostics
	balance, grossRealized, fees := 10000., 0., 0.
	var pos *Position
	var pending *Order
	orderCounter := 0
	emit := func(b Bar, stage, id, reason string, qty, price, cost float64) {
		result.Events = append(result.Events, Event{BarT: b.T, Stage: stage, OrderID: id, Reason: reason, Qty: qty, Price: price, Fee: cost})
	}
	closePosition := func(b Bar, raw float64, stage, reason string) {
		p := *pos
		exit := raw
		if reason == "stop" {
			exit = adverse(raw, -p.Order.Side, c.Stress, true)
		}
		exitFee := fee(c.Cost, p.Order.Qty)
		gross := fp.Mul(fp.Mul(fp.Sub(exit, p.Entry), float64(p.Order.Side)), p.Order.Qty)
		net := fp.Sub(fp.Sub(gross, p.EntryFee), exitFee)
		balance = fp.Add(balance, fp.Sub(gross, exitFee))
		grossRealized = fp.Add(grossRealized, gross)
		fees = fp.Add(fees, exitFee)
		t := Trade{Position: p, ExitBarT: b.T, ExitStage: stage, Exit: exit, Reason: reason, Gross: gross, ExitFee: exitFee, Fees: fp.Add(p.EntryFee, exitFee), Net: net, R: fp.Div(net, p.Order.PlannedRisk)}
		result.Trades = append(result.Trades, t)
		d.BranchNet[p.Order.Branch] = fp.Add(d.BranchNet[p.Order.Branch], net)
		if p.EntryBarT == b.T {
			d.EntryBarExits++
		}
		emit(b, stage, p.Order.ID, reason, p.Order.Qty, exit, exitFee)
		pos = nil
	}
	for n := start; n < len(bars); n++ {
		b, row := bars[n], rows[n]
		gap := false
		if n > start && b.T > bars[n-1].CloseT {
			gap = true
			ms := b.T - bars[n-1].CloseT
			d.GapCount++
			d.MissingIntervals += ms / Duration
			if pos != nil {
				d.HeldGaps++
				d.HeldGapMS += ms
			}
		}
		if pos != nil && n > start && time.UnixMilli(b.T).UTC().Format("2006-01-02") != time.UnixMilli(bars[n-1].T).UTC().Format("2006-01-02") {
			d.OvernightBoundaries++
		}
		// Carried opening outcomes precede unordered later extremes. An opening target
		// fills only at the target, never the favorable gap-open price.
		if pos != nil {
			p := pos
			if (p.Order.Side > 0 && b.Open <= p.Stop) || (p.Order.Side < 0 && b.Open >= p.Stop) {
				d.GapStops++
				closePosition(b, b.Open, "open-gap-exit", "stop")
			} else if (p.Order.Side > 0 && b.Open >= p.Target) || (p.Order.Side < 0 && b.Open <= p.Target) {
				closePosition(b, p.Target, "open-gap-exit", "target")
			}
		}
		if pending != nil {
			o := *pending
			pending = nil
			interval := b.T - o.SignalCloseT
			if interval > 0 {
				d.PendingEntryGaps++
			}
			if c.Stress == "skip-entry-gap" && interval > Duration {
				d.SkippedEntryGaps++
				emit(b, "open-admission", o.ID, "prospective-entry-gap", o.Qty, 0, 0)
			} else {
				entry := adverse(b.Open, o.Side, c.Stress, true)
				entryFee := fee(c.Cost, o.Qty)
				exitReservation := fee(c.Cost, o.Qty)
				stop, target := bracketLevels(entry, o.StopDistance, o.TrendDistance, o.Side)
				if o.Branch == "range-long" || o.Branch == "range-short" {
					target = toward(o.Basis, o.Side)
				}
				if fp.Mul(float64(o.Side), fp.Sub(entry, stop)) <= 0 || stop <= 0 || fp.Mul(float64(o.Side), fp.Sub(target, entry)) <= 0 {
					d.RejectedTarget++
					emit(b, "open-admission", o.ID, "wrong-side-or-zero-bracket", o.Qty, entry, 0)
				} else if fp.Mul(o.Qty, entry) > fp.Add(balance, 1e-9) {
					d.RejectedCap++
					emit(b, "open-admission", o.ID, "opening-notional-cap", o.Qty, entry, 0)
				} else if fp.Add(fp.Add(fp.Mul(o.Qty, entry), entryFee), exitReservation) > fp.Add(balance, 1e-9) {
					d.RejectedCost++
					emit(b, "open-admission", o.ID, "cost-reservation", o.Qty, entry, 0)
				} else {
					pos = &Position{Order: o, EntryBarT: b.T, Entry: entry, Stop: stop, Target: target, EntryFee: entryFee}
					balance = fp.Sub(balance, entryFee)
					fees = fp.Add(fees, entryFee)
					d.BranchFills[o.Branch]++
					emit(b, "open-fill", o.ID, o.Branch, o.Qty, entry, entryFee)
				}
			}
		}
		if pos != nil {
			p := pos
			hitStop := (p.Order.Side > 0 && b.Low <= p.Stop) || (p.Order.Side < 0 && b.High >= p.Stop)
			hitTarget := (p.Order.Side > 0 && b.High >= p.Target) || (p.Order.Side < 0 && b.Low <= p.Target)
			if hitStop && hitTarget {
				d.Collisions++
			}
			if hitTarget && (!hitStop || c.Stress == "target-first") {
				closePosition(b, p.Target, "intrabar-exit", "target")
			} else if hitStop {
				closePosition(b, p.Stop, "intrabar-exit", "stop")
			}
		}
		unreal := 0.
		if pos != nil {
			unreal = fp.Mul(fp.Mul(fp.Sub(b.Close, pos.Entry), float64(pos.Order.Side)), pos.Order.Qty)
		}
		equity := fp.Add(balance, unreal)
		fp.Check(equity, "study equity")
		fp.Check(balance, "study balance")
		fp.Check(unreal, "study mark")
		result.Equity = append(result.Equity, EquityPoint{T: b.CloseT, Balance: balance, Unrealized: unreal, Equity: equity, Open: pos != nil, Gap: gap})
		s := row.Signals
		hasSignal := s.InitialLong || s.InitialShort || s.RangeLong || s.RangeShort
		if !hasSignal {
			continue
		}
		d.RawSignals++
		if c.Candidate == "B1" && !s.InitialLong && !s.InitialShort {
			continue
		}
		if s.RangeLong && s.RangeShort {
			d.BothBand++
			continue
		}
		branch, side := "", 0
		switch {
		case s.InitialLong:
			branch, side = "trend-long", 1
		case s.InitialShort:
			branch, side = "trend-short", -1
		case s.RangeLong && c.Candidate != "B1":
			branch, side = "range-long", 1
		case s.RangeShort && c.Candidate != "B1":
			branch, side = "range-short", -1
		}
		if side == 0 {
			continue
		}
		d.BranchSignals[branch]++
		if pos != nil || pending != nil {
			d.Occupied++
			continue
		}
		if row.ATR == nil || row.BBBasis == nil || !s.FlatEligible || *row.ATR <= 0 || equity <= 0 {
			d.RejectedQty++
			continue
		}
		sizingPrice := adverse(b.Close, side, c.Stress, true)
		dStop, dTrend := ceilDistance(*row.ATR, 1.8), ceilDistance(*row.ATR, 3.2)
		qty := signalQty(equity, sizingPrice, dStop, c.Cost)
		if qty < .001 {
			d.RejectedQty++
			continue
		}
		orderCounter++
		pending = &Order{ID: fmt.Sprintf("o%d", orderCounter), Side: side, Branch: branch, SignalBarT: b.T, SignalCloseT: b.CloseT, SignalClose: b.Close, SignalEquity: equity, SizingPrice: sizingPrice, StopDistance: dStop, TrendDistance: dTrend, ATR: *row.ATR, Basis: *row.BBBasis, Qty: qty, PlannedRisk: fp.Mul(qty, dStop)}
		emit(b, "closed-bar-signal", pending.ID, branch, qty, b.Close, 0)
	}
	last := bars[len(bars)-1]
	unreal := 0.
	exitCost := 0.
	if pos != nil {
		unreal = fp.Mul(fp.Mul(fp.Sub(last.Close, pos.Entry), float64(pos.Order.Side)), pos.Order.Qty)
		exitCost = fee(c.Cost, pos.Order.Qty)
	}
	result.Terminal = Terminal{T: c.EndT, LastClosedBarT: last.T, Balance: balance, GrossRealized: grossRealized, Fees: fees, Unrealized: unreal, Equity: fp.Add(balance, unreal), LiquidationEquivalentEquity: fp.Sub(fp.Add(balance, unreal), exitCost), ModeledExitCost: exitCost, Open: pos, Pending: pending, Mark: last.Close}
	result.Metrics = metrics(result)
	return result, nil
}
