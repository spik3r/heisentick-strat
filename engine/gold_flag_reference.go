package engine

import (
	"encoding/json"
	"fmt"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
	"math"
)

func goldFlagPoint(t int64, kind string) GoldFlagEventTime {
	return GoldFlagEventTime{LowerMS: t, UpperMS: t, LowerInclusive: true, UpperInclusive: true, Kind: kind}
}
func goldFlagIntrabar(b GoldFlagBar) GoldFlagEventTime {
	return GoldFlagEventTime{LowerMS: b.FirstObservedT, UpperMS: b.LastObservedCloseT, Kind: "nonopening-crossing-unknown-model-time"}
}
func goldFlagAddEvent(o *GoldFlagOrder, state string, b GoldFlagBar, when GoldFlagEventTime, basis string) {
	var after *int
	if len(o.Events) > 0 {
		after = goldFlagPointer(len(o.Events) - 1)
	}
	o.Events = append(o.Events, GoldFlagEvent{State: state, RowIdx: b.Index, Time: when, AfterEventID: after, Basis: basis})
}
func goldFlagOriented(b GoldFlagBar, d int) (float64, float64, float64) {
	if d == 1 {
		return b.Open, b.High, b.Low
	}
	return -b.Open, -b.Low, -b.High
}

func goldFlagBroker(bars []GoldFlagBar) broker {
	raw := make([]marketdata.Bar, len(bars))
	for k, b := range bars {
		raw[k] = marketdata.Bar{T: float64(b.OpenT), O: b.Open, H: b.High, L: b.Low, C: b.Close, V: b.Volume}
	}
	return broker{series: marketdata.SeriesFromBars(raw), params: flagParams{MaxHoldBars: 24}, trades: []Trade{}}
}
func goldFlagOrder(bars []GoldFlagBar, sig GoldFlagSignal, cost float64) GoldFlagOrder {
	b := goldFlagBroker(bars)
	return goldFlagOrderWithBroker(bars, sig, cost, &b)
}
func goldFlagOrderWithBroker(bars []GoldFlagBar, sig GoldFlagSignal, cost float64, b *broker) GoldFlagOrder {
	i, d := sig.SignalIdx, sig.Side
	buf := .1 * sig.ATR
	entry, stop := sig.EdgeHi+buf, sig.EdgeLo-buf
	if d == -1 {
		entry, stop = sig.EdgeLo-buf, sig.EdgeHi+buf
	}
	o := GoldFlagOrder{GoldFlagSignal: sig, EntryPx: entry, Stop: stop, PlanRisk: math.Abs(entry - stop), ResolutionIdx: i, Ambiguities: []GoldFlagAmbiguity{}, Events: []GoldFlagEvent{}}
	if o.PlanRisk < .4*sig.ATR {
		o.Status = "invalid-risk"
		goldFlagAddEvent(&o, "invalid-risk", bars[i], goldFlagPoint(bars[i].CloseT, "nominal-close-point"), "signal consumes cooldown before minimum-risk rejection")
		return o
	}
	goldFlagAddEvent(&o, "pending", bars[i], goldFlagPoint(bars[i].CloseT, "nominal-close-point"), "nominal signal availability; first eligible observation is next row")
	end := min(i+4, len(bars)-1)
	fill := -1
	px := 0.
	entryAtOpen := false
	for j := i + 1; j <= end; j++ {
		p, h, l := goldFlagOriented(bars[j], d)
		e, s := float64(d)*entry, float64(d)*stop
		if p <= s {
			o.Status = "cancelled-open-opposite"
			o.ResolutionIdx = j
			goldFlagAddEvent(&o, "cancelled", bars[j], goldFlagPoint(bars[j].FirstObservedT, "observed-open-point"), "known opposite opening precedes later extremes")
			return o
		}
		if p >= e {
			fill = j
			px = bars[j].Open
			entryAtOpen = true
			break
		}
		if h >= e {
			fill = j
			px = entry
			o.EntryDual = l <= s
			break
		}
		if l <= s {
			o.Status = "cancelled-opposite"
			o.ResolutionIdx = j
			goldFlagAddEvent(&o, "cancelled", bars[j], goldFlagIntrabar(bars[j]), "opposite touched without entry")
			return o
		}
	}
	if fill < 0 {
		o.Status = "expired"
		o.ResolutionIdx = end
		if end < i+4 {
			o.Status = "working-at-end"
		} else {
			goldFlagAddEvent(&o, "expired", bars[end], goldFlagPoint(bars[end].CloseT, "nominal-close-point"), "four observed eligible rows exhausted")
		}
		return o
	}
	risk := math.Abs(px - stop)
	tp := px + float64(d)*2*risk
	o.FillIdx = goldFlagPointer(fill)
	o.Entry = goldFlagPointer(px)
	o.Risk = goldFlagPointer(risk)
	o.TP = goldFlagPointer(tp)
	o.EntryGap = goldFlagPointer(math.Abs(px - entry))
	when := goldFlagIntrabar(bars[fill])
	if entryAtOpen {
		when = goldFlagPoint(bars[fill].FirstObservedT, "observed-open-point")
	}
	basis := "fixed stress scenario; entry boundary touched after observed open"
	if entryAtOpen {
		basis = "fixed reference fill at observed opening price; exact event under model timestamp convention"
	}
	goldFlagAddEvent(&o, "filled", bars[fill], when, basis)
	// Reuse canonical Go broker position creation, bracket resolution, time exit
	// and close accounting. Only the pending stop order and evidence projection
	// are family-specific. Costs remain a separate diagnostic deduction.
	b.trades = b.trades[:0]
	b.openPosition(side(d), px, order{SL: stop, TP: tp, Size: 1, HasSize: true, NoSlip: true, Tag: "GOLD-FLAG-REFERENCE", Meta: TradeMeta{}}, fill)
	b.position.OpeningTargetPrecedence = true
	for j := fill; j < len(bars); j++ {
		p, h, l := goldFlagOriented(bars[j], d)
		sd, td := float64(d)*stop, float64(d)*tp
		sl, th := l <= sd, h >= td
		openStop, openTarget := j > fill && p <= sd, j > fill && p >= td
		if j == fill && o.EntryDual {
			alts := []string{"cancel", "stop"}
			if th {
				alts = append(alts, "target")
			}
			o.Ambiguities = append(o.Ambiguities, GoldFlagAmbiguity{Kind: "entry-before-cancel", BarIdx: j, Alternatives: alts, Time: goldFlagIntrabar(bars[j])})
		} else if !openStop && !openTarget && sl && th {
			kind := "later-bar-bracket"
			if j == fill {
				kind = "fill-bar-bracket"
			}
			o.Ambiguities = append(o.Ambiguities, GoldFlagAmbiguity{Kind: kind, BarIdx: j, Alternatives: []string{"stop", "target"}, Time: goldFlagIntrabar(bars[j])})
		}
		b.resolveIntrabarExit(j)
		b.exitAfterBars(j)
		if b.hasPosition {
			continue
		}
		t := b.trades[len(b.trades)-1]
		reason := t.Reason
		state := map[string]string{"sl": "stop", "tp": "target", "time": "time-exit"}[reason]
		et := goldFlagIntrabar(bars[j])
		basis := "fixed stop-first unordered-extreme reference scenario"
		if j == fill {
			reason += "-same-bar"
		} else if openStop {
			reason = "sl-open"
			et = goldFlagPoint(bars[j].FirstObservedT, "observed-open-point")
			basis = "known opening stop precedes later extremes"
		} else if openTarget {
			reason = "tp-open-capped"
			et = goldFlagPoint(bars[j].FirstObservedT, "observed-open-point")
			basis = "known opening target precedes later extremes; no target improvement"
		} else if t.Reason == "time" {
			et = goldFlagPoint(bars[j].CloseT, "nominal-close-point")
			basis = "fill plus 24 observed rows after bracket checks"
		}
		o.Status = "closed"
		o.ExitIdx = goldFlagPointer(j)
		o.Exit = goldFlagPointer(t.Exit)
		o.Reason = reason
		o.ResolutionIdx = j
		gross := float64(d) * (t.Exit - px) / risk
		o.GrossR = goldFlagPointer(gross)
		o.NetR = goldFlagPointer((float64(d)*(t.Exit-px) - 2*cost) / risk)
		goldFlagAddEvent(&o, state, bars[j], et, basis)
		return o
	}
	o.Status = "open-at-end"
	o.ResolutionIdx = len(bars) - 1
	o.Mark = goldFlagPointer(bars[len(bars)-1].Close)
	return o
}

func goldFlagClock(bars []GoldFlagBar, o GoldFlagOrder) GoldFlagClock {
	i := o.SignalIdx
	c := GoldFlagClock{SignalOpenMS: bars[i].OpenT, SignalAvailableMS: bars[i].CloseT, ExpiryRow: i + 4, ElapsedTwoHourMS: bars[i].CloseT + 4*goldFlagM30, SetupGapRows: []int{}, ActiveGapRows: []int{}, PartialRows: []int{}}
	if i+1 < len(bars) {
		c.NextObservedOpenMS = goldFlagPointer(bars[i+1].FirstObservedT)
	}
	if i+4 < len(bars) {
		c.ExpiryCloseMS = goldFlagPointer(bars[i+4].CloseT)
	}
	if o.FillIdx != nil {
		k := *o.FillIdx + 24
		c.TimeExitRow = goldFlagPointer(k)
		if k < len(bars) {
			c.TimeExitCloseMS = goldFlagPointer(bars[k].CloseT)
		}
	}
	start := max(0, i-11)
	for j := start; j <= o.ResolutionIdx; j++ {
		if !bars[j].Complete {
			c.PartialRows = append(c.PartialRows, j)
		}
		if j > start && bars[j].OpenT > bars[j-1].CloseT {
			if j <= i {
				c.SetupGapRows = append(c.SetupGapRows, j)
			} else {
				c.ActiveGapRows = append(c.ActiveGapRows, j)
			}
		}
	}
	return c
}

// RunGoldFlagReference executes only the fixed native-offline stress policy.
// Every reported fill is a model scenario, not an assertion of observed fills.
func RunGoldFlagReference(r GoldFlagReferenceRequest) (GoldFlagReferenceResult, error) {
	if _, err := dsl.DecodeGoldFlagReference(r.Config); err != nil {
		return GoldFlagReferenceResult{}, err
	}
	if r.FromT < 0 || r.ToT > 9007199254740991 || r.FromT >= r.ToT || r.FromT%goldFlagM30 != 0 || r.ToT%goldFlagM30 != 0 {
		return GoldFlagReferenceResult{}, fmt.Errorf("gold flag requires ordered nonnegative exact UTC M30 selection boundaries")
	}
	switch r.CostPerFill {
	case 0, .06, .15, .25, .50:
	default:
		return GoldFlagReferenceResult{}, fmt.Errorf("gold flag diagnostic cost must be 0, 0.06, 0.15, 0.25 or 0.50 price points per fill")
	}
	bars, used, err := goldFlagSource(r)
	if err != nil {
		return GoldFlagReferenceResult{}, err
	}
	rows := goldFlagSnapshots(bars)
	out := GoldFlagReferenceResult{Schema: "strat-gold-flag-reference-v1", Policy: dsl.GoldFlagReferencePolicy, EvidenceStatus: "deterministic-coarse-OHLC-stress-scenario-not-observed-execution", FromT: r.FromT, ToT: r.ToT, CostPerFill: r.CostPerFill, UsedSourceRows: used, Snapshots: rows, Orders: []GoldFlagOrder{}, Gaps: []GoldFlagGap{}, Assumptions: []string{
		"UTC source opens and nominal bar-close availability are model assumptions; provider latency and executable quote side are unverified.",
		"Partial observed buckets and gaps remain; no interpolation or calendar classification. Missing intervals are not proof of no price events.",
		"Same-bar entry-first and stop-first select a fixed stress scenario; local alternatives are not propagated portfolio bounds or actual fills.",
		"Only source M15 constructs M30. No finer-feed witness is promoted and no intrabar tick path is invented.",
		"Costs are fixed per-fill price-point deductions after reference outcomes, not measured spread, financing, fees, sizing or broker execution.",
		"Last observed row handles existing exposure but creates no new order; terminal pending/open state is retained without liquidation.",
	}}
	for j := 1; j < len(bars); j++ {
		if bars[j].OpenT > bars[j-1].CloseT {
			out.Gaps = append(out.Gaps, GoldFlagGap{RowIdx: j, FromMS: bars[j-1].CloseT, ToMS: bars[j].OpenT, MissingM30Slots: (bars[j].OpenT - bars[j-1].CloseT) / goldFlagM30})
		}
	}
	out.Orders, out.TerminalOrderID = goldFlagPortfolio(bars, rows, r.CostPerFill)
	if _, err = json.Marshal(out); err != nil {
		return GoldFlagReferenceResult{}, fmt.Errorf("gold flag nonfinite output: %w", err)
	}
	return out, nil
}

func goldFlagPortfolio(bars []GoldFlagBar, rows []GoldFlagSnapshot, cost float64) (orders []GoldFlagOrder, terminal *int) {
	orders = []GoldFlagOrder{}
	b := goldFlagBroker(bars)
	lastSignal := -7
	for i := 1; i < len(rows)-1; {
		if i-lastSignal <= 6 || rows[i].Candidate == nil {
			i++
			continue
		}
		lastSignal = i
		o := goldFlagOrderWithBroker(bars, *rows[i].Candidate, cost, &b)
		o.Clock = goldFlagClock(bars, o)
		orders = append(orders, o)
		if o.Status == "open-at-end" || o.Status == "working-at-end" {
			terminal = goldFlagPointer(i)
		}
		if o.Status == "open-at-end" {
			break
		}
		if o.Status == "closed" {
			i = *o.ExitIdx + 1
		} else {
			i++
		}
	}
	return orders, terminal
}
