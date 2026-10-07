package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func adaptiveFlagPoint(timestamp int64, kind string) AdaptiveFlagEventTime {
	return AdaptiveFlagEventTime{LowerMS: timestamp, UpperMS: timestamp, LowerInclusive: true, UpperInclusive: true, Kind: kind}
}
func adaptiveFlagIntrabar(bar AdaptiveFlagBar) AdaptiveFlagEventTime {
	return AdaptiveFlagEventTime{LowerMS: bar.OpenT, UpperMS: bar.CloseT, Kind: "nonopening-crossing-unknown-model-time"}
}

func adaptiveFlagEntry(sig AdaptiveFlagSignal, b AdaptiveFlagBar) (price float64, atOpen, filled bool) {
	if sig.Side == "long" {
		if b.Open >= sig.Trigger {
			return b.Open, true, true
		}
		if b.High >= sig.Trigger {
			return sig.Trigger, false, true
		}
	} else {
		if b.Open <= sig.Trigger {
			return b.Open, true, true
		}
		if b.Low <= sig.Trigger {
			return sig.Trigger, false, true
		}
	}
	return 0, false, false
}

// Only established brackets use this path. Equal nearer-extreme distances pick
// high first, and favorable target gaps fill at the open. Neither convention is
// a claim about observed ticks or independently verified TradingView parity.
func adaptiveFlagBracket(sig AdaptiveFlagSignal, b AdaptiveFlagBar) (reason string, price float64, atOpen bool) {
	if sig.Side == "long" {
		if b.Open < sig.Stop {
			return "stop-gap", b.Open, true
		}
		if b.Open > sig.Target {
			return "target-gap", b.Open, true
		}
	} else {
		if b.Open > sig.Stop {
			return "stop-gap", b.Open, true
		}
		if b.Open < sig.Target {
			return "target-gap", b.Open, true
		}
	}
	path := [4]float64{b.Open, b.High, b.Low, b.Close}
	if math.Abs(float64(b.Open-b.High)) > math.Abs(float64(b.Open-b.Low)) {
		path = [4]float64{b.Open, b.Low, b.High, b.Close}
	}
	for i := 0; i < 3; i++ {
		a, z := path[i], path[i+1]
		found := false
		distance := 0.
		for _, barrier := range []struct {
			reason string
			price  float64
		}{{"stop", sig.Stop}, {"target", sig.Target}} {
			if math.Min(a, z) <= barrier.price && barrier.price <= math.Max(a, z) {
				d := math.Abs(float64(barrier.price - a))
				// Equal distances retain stop, matching the reference tuple's
				// lexical stop-before-target tie ordering.
				if !found || d < distance {
					found = true
					distance = d
					reason = barrier.reason
					price = barrier.price
				}
			}
		}
		if found {
			return reason, price, i == 0 && price == b.Open
		}
	}
	return "", 0, false
}

func adaptiveFlagPortfolio(series marketdata.Series, bars []AdaptiveFlagBar, rows []AdaptiveFlagSnapshot, r dsl.AdaptiveFlagRules) ([]AdaptiveFlagOrder, []AdaptiveFlagEvent, []AdaptiveFlagState, AdaptiveFlagState) {
	return adaptiveFlagPortfolioFrom(series, bars, rows, r, 0)
}

func adaptiveFlagPortfolioFrom(series marketdata.Series, bars []AdaptiveFlagBar, rows []AdaptiveFlagSnapshot, r dsl.AdaptiveFlagRules, tradeFromMS int64) ([]AdaptiveFlagOrder, []AdaptiveFlagEvent, []AdaptiveFlagState, AdaptiveFlagState) {
	orders := []AdaptiveFlagOrder{}
	events := []AdaptiveFlagEvent{}
	states := make([]AdaptiveFlagState, 0, len(bars))
	pending, positionID := -1, -1
	var queued *AdaptiveFlagQueuedExit
	// Reuse canonical raw position creation/closure, with zero costs and an
	// explicitly zero-size internal placeholder. No sizing or accounting result
	// escapes this adapter. Family-specific timing/path never calls generic exits.
	b := broker{series: series, trades: []Trade{}}
	addEvent := func(orderID int, row AdaptiveFlagBar, state string, when AdaptiveFlagEventTime, price *float64, basis string) {
		id := len(events)
		events = append(events, AdaptiveFlagEvent{ID: id, OrderID: orderID, RowIdx: row.Index, State: state, Time: when, Price: price, Basis: basis})
		orders[orderID].EventIDs = append(orders[orderID].EventIDs, id)
	}
	closePosition := func(i int, price float64, reason string, atOpen bool) {
		o := &orders[positionID]
		b.closePosition(price, i, reason, "")
		t := b.trades[len(b.trades)-1]
		o.Status = "closed"
		o.ExitIdx = adaptiveFlagPointer(i)
		o.Exit = adaptiveFlagPointer(t.Exit)
		o.Reason = reason
		when := adaptiveFlagIntrabar(bars[i])
		basis := "nearer-extreme-first assumed OHLC path; high-first ties"
		if atOpen {
			when = adaptiveFlagPoint(bars[i].OpenT, "observed-open-point")
			basis = "next observed opening price under the raw reference policy"
		}
		addEvent(positionID, bars[i], "closed", when, o.Exit, reason+": "+basis)
		positionID = -1
		queued = nil
		// Canonical economic records remain internal and are not accumulated.
		b.trades = b.trades[:0]
	}
	terminal := AdaptiveFlagState{RowIdx: -1, Status: "flat"}
	for i, bar := range bars {
		if positionID >= 0 && queued != nil && queued.NextOpenIdx == i {
			closePosition(i, bar.Open, queued.Reason, true)
		}
		if positionID >= 0 && i > *orders[positionID].FillIdx {
			reason, price, atOpen := adaptiveFlagBracket(orders[positionID].AdaptiveFlagSignal, bar)
			if reason != "" {
				closePosition(i, price, reason, atOpen)
			}
		}
		if positionID < 0 && pending >= 0 && i > orders[pending].SignalIdx {
			o := &orders[pending]
			o.FillOpportunities++
			addEvent(pending, bar, "entry-opportunity", AdaptiveFlagEventTime{LowerMS: bar.OpenT, UpperMS: bar.CloseT, LowerInclusive: true, Kind: "observed-bar-opportunity"}, nil, "pending stop remains active before close-phase expiry; no filter recheck or invalidation")
			price, atOpen, filled := adaptiveFlagEntry(o.AdaptiveFlagSignal, bar)
			if filled {
				d := sideLong
				if o.Side == "short" {
					d = sideShort
				}
				b.openPosition(d, price, order{SL: o.Stop, TP: o.Target, HasSize: true, Size: 0, NoSlip: true, Tag: "ADAPTIVE-VOLUME-FLAG"}, i)
				o.Status = "open"
				o.FillIdx = adaptiveFlagPointer(i)
				o.Entry = adaptiveFlagPointer(b.position.Entry)
				o.EntryAtOpen = atOpen
				o.ActualFillToStopDistance = adaptiveFlagPointer(math.Abs(float64(b.position.Entry - o.Stop)))
				when := adaptiveFlagIntrabar(bar)
				if atOpen {
					when = adaptiveFlagPoint(bar.OpenT, "observed-open-point")
				}
				addEvent(pending, bar, "filled", when, o.Entry, "stop entry; frozen trigger, stop and trigger-anchored target; no entry-bar bracket")
				positionID = pending
				pending = -1
			}
		}
		if positionID >= 0 {
			o := &orders[positionID]
			if i == *o.FillIdx {
				o.BracketCreationIdx = adaptiveFlagPointer(i)
				addEvent(positionID, bar, "bracket-created", adaptiveFlagPoint(bar.CloseT, "nominal-close-point"), nil, "first delayed bracket created only after the entry bar path")
				if (o.Side == "long" && bar.Close <= o.Stop) || (o.Side == "short" && bar.Close >= o.Stop) {
					queued = &AdaptiveFlagQueuedExit{Reason: "stop-activated-at-close", CreationIdx: i, CreationCloseMS: bar.CloseT, NextOpenIdx: i + 1}
				}
			}
			if queued == nil && i-*o.FillIdx >= r.MaxHold {
				queued = &AdaptiveFlagQueuedExit{Reason: "max-hold", CreationIdx: i, CreationCloseMS: bar.CloseT, NextOpenIdx: i + 1}
			}
			if queued != nil && queued.CreationIdx == i {
				o.Status = "queued-exit"
				addEvent(positionID, bar, "exit-queued", adaptiveFlagPoint(bar.CloseT, "nominal-close-point"), nil, queued.Reason+": market exit precedes next observed bar brackets, regardless of recovery")
			}
		} else {
			if pending >= 0 {
				o := &orders[pending]
				o.PendingAge++
				if o.PendingAge > r.ValidBars {
					o.Status = "expired"
					addEvent(pending, bar, "expired", adaptiveFlagPoint(bar.CloseT, "nominal-close-point"), nil, "age exceeds validBars after this row's final fill opportunity; same-close rearm allowed")
					pending = -1
				}
			}
			if pending < 0 && bar.OpenT >= tradeFromMS && rows[i].Candidate != nil {
				pending = len(orders)
				orders = append(orders, AdaptiveFlagOrder{AdaptiveFlagSignal: *rows[i].Candidate, ID: pending, Status: "pending", EventIDs: []int{}})
				addEvent(pending, bar, "pending", adaptiveFlagPoint(bar.CloseT, "nominal-close-point"), nil, "creation-time source filters; first stop-entry opportunity is next observed row")
			}
		}
		state := AdaptiveFlagState{RowIdx: i, CloseMS: bar.CloseT, Status: "flat"}
		if pending >= 0 {
			state.Status = "pending"
			state.PendingOrderID = adaptiveFlagPointer(pending)
			state.PendingAge = orders[pending].PendingAge
		}
		if positionID >= 0 {
			state.Status = "open"
			state.PositionOrderID = adaptiveFlagPointer(positionID)
		}
		if queued != nil {
			state.Status = "queued-exit"
			state.QueuedExit = adaptiveFlagPointer(*queued)
		}
		states = append(states, state)
		terminal = state
	}
	return orders, events, states, terminal
}

func adaptiveFlagHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// RunAdaptiveVolumeFlag implements the isolated native delayed-bracket raw
// reference. It provides model prices and state only, with no account inference.
func RunAdaptiveVolumeFlag(request AdaptiveFlagRequest) (AdaptiveFlagResult, error) {
	spec, err := dsl.DecodeAdaptiveVolumeFlag(request.Config)
	if err != nil {
		return AdaptiveFlagResult{}, err
	}
	research, err := adaptiveFlagResearchPolicy(request.ResearchAblation, spec)
	if err != nil {
		return AdaptiveFlagResult{}, err
	}
	series, window, err := adaptiveFlagWindowPrefix(request.Series, spec.Timeframe, request.Window)
	if err != nil {
		return AdaptiveFlagResult{}, err
	}
	bars, step, err := adaptiveFlagSource(series, spec.Timeframe)
	if err != nil {
		return AdaptiveFlagResult{}, err
	}
	rows := adaptiveFlagSnapshots(bars, spec.Rules)
	if research != nil {
		adaptiveFlagResearchOverlay(rows, spec.Rules, *research)
	}
	if _, err = json.Marshal(rows); err != nil {
		return AdaptiveFlagResult{}, fmt.Errorf("adaptive volume flag nonfinite derived source state: %w", err)
	}
	configHash, err := adaptiveFlagHash(spec)
	if err != nil {
		return AdaptiveFlagResult{}, fmt.Errorf("adaptive volume flag config identity: %w", err)
	}
	inputRows := make([][6]float64, len(bars))
	for i, b := range bars {
		inputRows[i] = [6]float64{float64(b.OpenT), b.Open, b.High, b.Low, b.Close, b.Volume}
	}
	inputHash, err := adaptiveFlagHash(inputRows)
	if err != nil {
		return AdaptiveFlagResult{}, fmt.Errorf("adaptive volume flag input identity: %w", err)
	}
	out := AdaptiveFlagResult{
		Schema: "strat-adaptive-volume-flag-reference-v1", Policy: spec.Policy, ExecutionSemantics: AdaptiveFlagExecutionSemantics,
		NumericalPolicy:         dsl.AdaptiveFlagNumericalPolicy,
		EvidenceStatus:          "deterministic-OHLC-reference-assumptions-not-TradingView-parity-or-observed-execution",
		ArithmeticQualification: "explicit-float64-rounding-barriers; cross-architecture-not-qualified",
		SourcePineSHA256:        AdaptiveFlagSourcePineSHA256, ReferenceSHA256: AdaptiveFlagReferenceSHA256,
		ConfigSHA256: configHash, InputSHA256: inputHash,
		IdentityEncoding: "sha256 of Go encoding/json compact effectiveConfig and ordered [t,o,h,l,c,v] arrays respectively; not original config or BTB1 file bytes",
		EffectiveConfig:  spec, ExecutionWindow: window, ProvidedSourceRows: request.Series.Len(), IgnoredSuffixRows: request.Series.Len() - len(bars), FirstRetainedOpenMS: bars[0].OpenT, LastRetainedOpenMS: bars[len(bars)-1].OpenT, LastRetainedCloseMS: bars[len(bars)-1].CloseT, UsedSourceRows: len(bars), TimeframeMS: step, Snapshots: rows, Gaps: []AdaptiveFlagGap{},
		Assumptions: []string{
			"Retained timestamps are exact UTC bar opens. Close-phase availability is open plus declared timeframe, independent of the next observed timestamp; provider latency and executable quotes are unverified.",
			"Observed gaps remain without interpolation, aggregation, calendar inference or invented bars. Hold and validity count observed rows.",
			"An optional evaluation window uses row-open [tradeFromMs,tradeToMs). Earlier retained rows warm indicators/pivots with a flat broker and no orders; rows at/after the end never enter indicators or execution. A final included close may leave pending exposure, with no post-end fill. Only the retained prefix is validated; no suffix-coverage claim is made.",
			"Strict symmetric pivot ties are rejected as a reference assumption; confirmation preserves occurrence indexes. Pole endpoints move with high[sensitivity]/low[sensitivity]. Both flag windows use last pivot HIGH age and include the current row.",
			"BINARY64_ORDERED_V1 is fixed numerical provenance, not a configurable strategy input. ATR includes initial high-minus-low; its first configured-length true ranges are summed sequentially, then divided once. Wilder recurrence follows the written operation order with separate binary64 rounding. Recursive EMA seeds at first close. Volume SMA includes current volume and requires a full window.",
			"Comparisons use direct binary64 values without blanket decimal rounding or epsilon. CPython-version-specific compensated sum is not this numerical contract. Exact Pine numerical behavior and arbitrary-input Python equivalence are unqualified; passing Python trace comparisons apply only to their named invented fixtures.",
			"Source EMA OR gates and volume filter apply at creation only. Pending levels remain frozen. Long has precedence, with trigger-anchored target even after an opening entry gap.",
			"Pending stops first activate on the next observed row and have validBars+1 fill opportunities before flat-close expiry. Expiry or exit permits same-close rearming, including the final observed close.",
			"The first bracket is created at entry-bar close. A stop at/beyond close activation queues an unconditional next-open market exit; take-profit limits do not latch. This is a reference inference, not independently verified TradingView execution.",
			"Established brackets follow open, nearer extreme, other extreme, close, with high-first ties. Stop and favorable target gaps fill at open. Nonopening event times are intervals rather than fabricated ticks.",
			"Entry-close hold count is zero. Max hold H queues at E+H close and fills E+H+1 observed open. Queued market exits precede brackets. Terminal pending/open/queued states are retained without liquidation.",
			"Raw prices and distances only: no costs, sizing, quantity, point value, cash PnL, return, equity, risk budget, margin, account properties or performance qualification.",
			"Source snapshots compute causal predicates at each close for inspection; a candidate creates an order only when order state admits it. Explicit float64 rounding barriers do not establish cross-architecture qualification.",
		},
	}
	if research != nil {
		out.Schema = AdaptiveFlagResearchSchema
		out.ResearchPolicy = research
		out.ResearchPolicySHA256, err = adaptiveFlagHash(*research)
		if err != nil {
			return AdaptiveFlagResult{}, err
		}
		out.ResearchProducer = adaptiveFlagResearchBuild()
	}
	for i := 1; i < len(bars); i++ {
		if bars[i].OpenT > bars[i-1].CloseT {
			out.Gaps = append(out.Gaps, AdaptiveFlagGap{RowIdx: i, FromMS: bars[i-1].CloseT, ToMS: bars[i].OpenT, MissingSlots: (bars[i].OpenT - bars[i-1].CloseT) / step})
		}
	}
	tradeFromMS := int64(0)
	if window != nil {
		tradeFromMS = window.TradeFromMS
	}
	for _, bar := range bars {
		if bar.OpenT < tradeFromMS {
			out.PreTradeRows++
		} else {
			out.EligibleTradeRows++
		}
	}
	out.Orders, out.Events, out.States, out.Terminal = adaptiveFlagPortfolioFrom(series, bars, rows, spec.Rules, tradeFromMS)
	if _, err = json.Marshal(out); err != nil {
		return AdaptiveFlagResult{}, fmt.Errorf("adaptive volume flag nonfinite raw result: %w", err)
	}
	return out, nil
}
