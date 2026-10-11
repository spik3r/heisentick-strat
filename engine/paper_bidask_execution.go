package engine

import (
	"math/big"
	"strconv"
)

type bidAskOpen struct {
	view                 BidAskPosition
	entry, qty, entryFee *big.Rat
	fullFrom, entryT     int64
}
type bidAskDecision struct {
	signalCommitT  string
	episodeCommitT string
	view           BidAskPending
	order          order
	signal         BidAskBar
	index          int
	episode        *BidAskEpisode
}
type bidAskRunner struct {
	request                                         PaperBidAskRequest
	prepared                                        *PreparedRun
	binding                                         BidAskBinding
	identity                                        string
	collector                                       *bidAskCollector
	events                                          []BidAskEvent
	position                                        *bidAskOpen
	pending                                         *bidAskDecision
	coverage                                        string
	duration, activation, maxQuoteAge, maxSignalAge int64
	lastBarOpen, lastBarClose, lastQuoteT           int64
	quoteIdentities                                 map[int64]string
	barIndex                                        int
	err                                             error
}

func newBidAskRunner(r PaperBidAskRequest, p *PreparedRun, b BidAskBinding) *bidAskRunner {
	activation, _ := bidAskTime(r.ActivationFromT)
	quoteAge, _ := strconv.ParseInt(r.Policy.MaxQuoteAgeMS, 10, 64)
	signalAge, _ := strconv.ParseInt(r.Policy.MaxSignalAgeMS, 10, 64)
	x := &bidAskRunner{request: r, prepared: p, binding: b, identity: bidAskHash(b), collector: newBidAskCollector(r.Economics.StartEquity), events: []BidAskEvent{}, coverage: "sampled-observations-only", duration: paperTimeframeMS(r.Route.Timeframe) * 1000000, activation: activation, maxQuoteAge: quoteAge * 1000000, maxSignalAge: signalAge * 1000000, lastBarOpen: -1, lastBarClose: -1, lastQuoteT: -1, quoteIdentities: make(map[int64]string), barIndex: -1}
	if p != nil {
		p.broker.reset(p.series, p.cols, p.htfTrend, p.ema, p.emaSlope, p.params, p.fixture, nil)
	}
	return x
}
func (x *bidAskRunner) emit(rec BidAskRecord, e BidAskEvent) {
	if len(x.events) >= PaperBidAskMaxEvents {
		x.err = bidAskError("EVENT_LIMIT")
		return
	}
	e.RecordSequence = rec.Sequence
	e.Clock.CommitT = rec.CommitT
	e.ID = bidAskHash(struct {
		Binding, Sequence string
		Ordinal           int
		Kind              string
	}{x.identity, rec.Sequence, len(x.events), e.Kind})
	x.events = append(x.events, e)
}
func bidAskQuoteClock(q *BidAskQuote) BidAskClock {
	return BidAskClock{ProviderT: q.ProviderT, RequestStartT: q.RequestStartT, ReceivedT: q.ReceivedT, QualifiedT: q.QualifiedT, ModelEffectiveT: q.ProviderT}
}
func bidAskBarClock(b *BidAskBar) BidAskClock {
	return BidAskClock{ReceivedT: b.ReceivedT, IntervalOpenT: b.OpenT, IntervalCloseT: b.CloseT, ModelEffectiveT: b.CloseT}
}
func (x *bidAskRunner) unresolved(rec BidAskRecord, why string) {
	x.coverage = "unresolved-exposure:" + why
	x.collector.account.MarkStatus = "stale-unresolved"
	x.emit(rec, BidAskEvent{Kind: "coverage", Outcome: why, Basis: "no-fabricated-liquidation"})
}
func (x *bidAskRunner) noFill(rec BidAskRecord, why string, clock BidAskClock) {
	d := x.pending
	if d == nil {
		x.err = bidAskError("EPISODE_STATE")
		return
	}
	clock.SignalOpenT = d.signal.OpenT
	clock.SignalCloseT = d.signal.CloseT
	x.emit(rec, BidAskEvent{Kind: "decision-outcome", Outcome: why, DecisionID: d.view.DecisionID, Basis: "original-episode", Clock: clock})
	x.pending = nil
}
func (x *bidAskRunner) mark(rec BidAskRecord, price *big.Rat, basis string, clock BidAskClock) {
	clock.CommitT = rec.CommitT
	e := BidAskEvent{Kind: "mark", Basis: basis, Clock: clock}
	if x.position != nil {
		p := x.position
		b := &x.prepared.broker
		x.collector.mark(p.entry, price, p.qty, b.position.Side, basis, clock)
		e.PositionID = p.view.ID
		e.Price = bidAskCash(price)
		e.Quantity = p.view.Quantity
		e.Side = p.view.Side
	} else {
		x.collector.mark(nil, nil, nil, sideLong, basis, clock)
	}
	x.emit(rec, e)
}
func (x *bidAskRunner) close(rec BidAskRecord, price *big.Rat, why, basis string, clock BidAskClock) {
	p := x.position
	if p == nil {
		return
	}
	b := &x.prepared.broker
	s := b.position.Side
	fill := bidAskAdverse(price, s, false, x.request.Economics)
	if fill.Sign() <= 0 {
		x.unresolved(rec, "invalid-modeled-exit")
		return
	}
	fee := newRatMul(p.qty, bidAskRat(x.request.Economics.ExitFee))
	net := x.collector.exit(p.entry, fill, p.qty, p.entryFee, fee, s)
	x.emit(rec, BidAskEvent{Kind: "exit", Outcome: why, DecisionID: p.view.DecisionID, PositionID: p.view.ID, Side: s.String(), Price: bidAskCash(fill), Quantity: p.view.Quantity, Fee: bidAskCash(fee), NetPnL: bidAskCash(net), ProtectionVersion: p.view.ProtectionVersion, Basis: basis, Clock: clock})
	b.hasPosition = false
	b.lastExitIndex = x.barIndex
	x.position = nil
	x.mark(rec, nil, basis, clock)
}
func (x *bidAskRunner) fill(rec BidAskRecord, q *BidAskQuote) {
	d := x.pending
	b := &x.prepared.broker
	s := d.order.Side
	ec := x.request.Economics
	entryTier, liquidTier := q.Asks[0], q.Bids[0]
	if s == sideShort {
		entryTier, liquidTier = q.Bids[0], q.Asks[0]
	}
	entry := bidAskAdverse(bidAskRat(entryTier.Price), s, true, ec)
	stop, e := bidAskFamily(d.order.SL)
	if e != nil {
		x.err = e
		return
	}
	target, e := bidAskFamily(d.order.TP)
	if e != nil {
		x.err = e
		return
	}
	// Preserve the ordinary Go entry-fill-distance interpretation through the
	// same binary64 protection position. Monetary entry/fees remain exact.
	entryOperand, _ := entry.Float64()
	if d.order.HasStopDistance {
		stop, e = bidAskFamily(entryOperand - float64(s)*d.order.StopDistance)
		if e != nil {
			x.err = e
			return
		}
	}
	if d.order.HasTargetDistance {
		target, e = bidAskFamily(entryOperand + float64(s)*d.order.TargetDistance)
		if e != nil {
			x.err = e
			return
		}
	}
	clock := bidAskQuoteClock(q)
	clock.SignalOpenT = d.signal.OpenT
	clock.SignalCloseT = d.signal.CloseT
	if entry.Sign() <= 0 || stop.Sign() <= 0 || target.Sign() <= 0 || bidAskSigned(newRatSub(entry, stop), s).Sign() <= 0 || bidAskSigned(newRatSub(target, entry), s).Sign() <= 0 || bidAskSigned(newRatSub(bidAskRat(liquidTier.Price), stop), s).Sign() <= 0 {
		x.noFill(rec, "invalid-or-breached-protection", clock)
		return
	}
	if d.order.HasLimit || d.order.HasSize || d.order.NoStop || d.order.NoTarget || !d.order.HasRisk || d.order.NoSlip {
		x.err = bidAskError("ORDER_UNQUALIFIED")
		return
	}
	risk, e := bidAskFamily(d.order.RiskUSD)
	if e != nil || risk.Sign() <= 0 {
		x.err = bidAskError("RISK")
		return
	}
	stopFill := bidAskAdverse(stop, s, false, ec)
	if stopFill.Sign() <= 0 {
		x.noFill(rec, "invalid-stop-budget", clock)
		return
	}
	loss := newRatAdd(bidAskSigned(newRatSub(entry, stopFill), s), newRatAdd(bidAskRat(ec.EntryFee), bidAskRat(ec.ExitFee)))
	if loss.Sign() <= 0 {
		x.noFill(rec, "invalid-stop-budget", clock)
		return
	}
	quantity := bidAskFloorQuantity(risk, loss, bidAskRat(ec.QuantityQuantum))
	if quantity.Cmp(bidAskRat(ec.QuantityMinimum)) < 0 {
		x.noFill(rec, "below-minimum-quantity", clock)
		return
	}
	if quantity.Cmp(bidAskRat(ec.QuantityMaximum)) > 0 {
		x.noFill(rec, "above-maximum-quantity", clock)
		return
	}
	if ec.NotionalPolicy == "cap" && newRatMul(quantity, entry).Cmp(bidAskRat(ec.NotionalMaximum)) > 0 {
		x.noFill(rec, "notional-cap", clock)
		return
	}
	if quantity.Cmp(bidAskRat(entryTier.Liquidity)) > 0 {
		x.noFill(rec, "insufficient-entry-liquidity", clock)
		return
	}
	entryT, _ := bidAskTime(q.ProviderT)
	full := entryT
	if mod := entryT % x.duration; mod != 0 {
		full = entryT + x.duration - mod
	}
	entryFloat, _ := entry.Float64()
	qtyFloat, _ := quantity.Float64()
	stopFloat, _ := stop.Float64()
	targetFloat, _ := target.Float64()
	if !isFinite(entryFloat) || !isFinite(qtyFloat) || !isFinite(stopFloat) || !isFinite(targetFloat) {
		x.err = bidAskError("NONFINITE")
		return
	}
	// This is the only shared protection position. Legacy midpoint economics and
	// collectors are never called by this execution model.
	b.position = position{Side: s, Entry: entryFloat, Size: qtyFloat, SL: stopFloat, TP: targetFloat, InitialSL: stopFloat, InitialTP: targetFloat, Meta: d.order.Meta, EntryIndex: d.index, EntryT: float64(entryT) / 1000000, Tag: d.order.Tag, GapAwareStop: true}
	b.hasPosition = true
	fee := newRatMul(quantity, bidAskRat(ec.EntryFee))
	x.collector.entry(fee)
	p := &bidAskOpen{entry: entry, qty: quantity, entryFee: fee, entryT: entryT, fullFrom: full, view: BidAskPosition{ID: bidAskHash(struct{ Decision string }{d.view.DecisionID}), DecisionID: d.view.DecisionID, Side: s.String(), Entry: bidAskCash(entry), Quantity: bidAskCash(quantity), Stop: bidAskCash(stop), Target: bidAskCash(target), RiskBudget: bidAskCash(risk), StopBudgetLoss: bidAskCash(newRatMul(quantity, loss)), EntryFee: bidAskCash(fee), ProtectionVersion: 1, SignalIndex: d.index, EntryProviderT: q.ProviderT, FullPhaseFromT: bidAskStamp(full)}}
	p.view.Phase = "sampled-straddle"
	if entryT == full {
		p.view.Phase = "full-bar"
	}
	p.view.ExpectedFullIntervalOpenT = bidAskStamp(full)
	x.position = p
	x.pending = nil
	x.emit(rec, BidAskEvent{Kind: "entry", Outcome: "filled", DecisionID: p.view.DecisionID, PositionID: p.view.ID, Side: p.view.Side, Price: p.view.Entry, Quantity: p.view.Quantity, Fee: p.view.EntryFee, Stop: p.view.Stop, Target: p.view.Target, ProtectionVersion: 1, Basis: "observed-entry-side-with-modeled-costs", Clock: clock})
	x.mark(rec, bidAskRat(liquidTier.Price), "entry-liquidation-side-quote", clock)
}
func bidAskDueSlot(b *broker, i int) bool {
	if !b.hasPosition {
		return false
	}
	slotEnd, ok := numericMetaValue(b.position.Meta["slotEnd"])
	return ok && b.series.T[i] >= slotEnd
}
func bidAskDueHold(b *broker, i int) bool {
	return b.hasPosition && b.params.MaxHoldBars != 0 && !(float64(i-b.position.EntryIndex) < b.params.MaxHoldBars)
}
func (x *bidAskRunner) onBar(rec BidAskRecord) {
	x.barIndex++
	i := x.barIndex
	bar := rec.Bar
	open, _ := bidAskTime(bar.OpenT)
	close, _ := bidAskTime(bar.CloseT)
	previousOpen := x.lastBarOpen
	x.lastBarOpen = open
	x.lastBarClose = close
	if x.pending != nil {
		x.err = bidAskError("UNRESOLVED_EPISODE")
		return
	}
	if open < x.activation {
		return
	}
	if x.prepared == nil {
		x.err = bidAskError("INTERNAL")
		return
	}
	b := &x.prepared.broker
	if x.position != nil && previousOpen >= 0 && open != previousOpen+x.duration {
		x.unresolved(rec, "missing-completed-interval")
	}
	if x.coverage != "sampled-observations-only" {
		return
	}
	clock := bidAskBarClock(bar)
	wasHeld := b.hasPosition
	if wasHeld {
		p := x.position
		if close >= p.fullFrom {
			p.view.Phase = "full-bar"
		}
		if open >= p.fullFrom {
			p.view.ExpectedFullIntervalOpenT = bar.CloseT
		}
		sideBar := bar.Bid
		if b.position.Side == sideShort {
			sideBar = bar.Ask
		}
		if open >= p.fullFrom {
			sl, e := bidAskFamily(b.position.SL)
			if e != nil {
				x.err = e
				return
			}
			tp, e := bidAskFamily(b.position.TP)
			if e != nil {
				x.err = e
				return
			}
			o, h, l := bidAskRat(sideBar.Open), bidAskRat(sideBar.High), bidAskRat(sideBar.Low)
			s := b.position.Side
			if bidAskDueSlot(b, i) {
				clock.ModelEffectiveT = bar.OpenT
				x.close(rec, o, "window-close", "modeled-side-open", clock)
			} else if (s == sideLong && o.Cmp(sl) <= 0) || (s == sideShort && o.Cmp(sl) >= 0) {
				clock.ModelEffectiveT = bar.OpenT
				x.close(rec, o, "stop-gap", "modeled-side-open", clock)
			} else if (s == sideLong && l.Cmp(sl) <= 0) || (s == sideShort && h.Cmp(sl) >= 0) {
				x.close(rec, sl, "stop", "modeled-side-bar-stop-first", clock)
			} else if (s == sideLong && h.Cmp(tp) >= 0) || (s == sideShort && l.Cmp(tp) <= 0) {
				x.close(rec, tp, "target", "modeled-side-bar-stop-first", clock)
			}
		}
		if x.coverage != "sampled-observations-only" {
			return
		}
		if b.hasPosition {
			old := b.position.SL
			b.moveStopToBreakeven(i)
			if old != b.position.SL {
				stop, e := bidAskFamily(b.position.SL)
				if e != nil {
					x.err = e
					return
				}
				p.view.Stop = bidAskCash(stop)
				p.view.ProtectionVersion++
				clock.ModelEffectiveT = bar.CloseT
				x.emit(rec, BidAskEvent{Kind: "protection", Outcome: "breakeven", PositionID: p.view.ID, DecisionID: p.view.DecisionID, Stop: p.view.Stop, Target: p.view.Target, ProtectionVersion: p.view.ProtectionVersion, PriorProtectionVersion: p.view.ProtectionVersion - 1, Basis: "future-only-midpoint-close-management", Clock: clock})
			}
			if bidAskDueHold(b, i) {
				clock.ModelEffectiveT = bar.CloseT
				x.close(rec, bidAskRat(sideBar.Close), "time", "modeled-side-close", clock)
			} else if open >= p.fullFrom {
				x.mark(rec, bidAskRat(sideBar.Close), "completed-full-side-bar", clock)
			}
			return // Existing held ORB handler does not retry entry after a time exit.
		}
	}
	// A bracket/slot exit precedes ordinary onBar; its real attempt state and
	// source gates decide whether a new decision exists. Warmup never gets here.
	before := len(b.capturedEntries)
	b.captureEntries = true
	b.onBar(i)
	b.captureEntries = false
	if len(b.capturedEntries)-before > 1 {
		x.err = bidAskError("MULTIPLE_DECISIONS")
		return
	}
	if len(b.capturedEntries) > before {
		ord := b.capturedEntries[before]
		id := bidAskHash(struct {
			Binding, Signal string
			Ordinal         int
		}{x.identity, rec.Sequence, 0})
		x.pending = &bidAskDecision{view: BidAskPending{DecisionID: id, SignalSequence: rec.Sequence, State: "awaiting-original-episode"}, order: ord, signal: *bar, index: i, signalCommitT: rec.CommitT}
		x.emit(rec, BidAskEvent{Kind: "decision", Outcome: "awaiting-original-episode", DecisionID: id, Side: ord.Side.String(), Basis: "go-family-capture", Clock: BidAskClock{SignalOpenT: bar.OpenT, SignalCloseT: bar.CloseT, ReceivedT: bar.ReceivedT, ModelEffectiveT: bar.CloseT}})
	}
}
func bidAskQuoteFingerprint(q *BidAskQuote) string {
	type tier struct{ Price, Liquidity string }
	bids, asks := []tier{}, []tier{}
	for _, v := range q.Bids {
		bids = append(bids, tier{bidAskCash(bidAskRat(v.Price)), bidAskCash(bidAskRat(v.Liquidity))})
	}
	for _, v := range q.Asks {
		asks = append(asks, tier{bidAskCash(bidAskRat(v.Price)), bidAskCash(bidAskRat(v.Liquidity))})
	}
	return bidAskHash(struct {
		Account, Instrument string
		Tradeable           bool
		Bids, Asks          []tier
	}{q.AccountDigest, q.Instrument, q.Tradeable, bids, asks})
}
func (x *bidAskRunner) onQuote(rec BidAskRecord) {
	q := rec.Quote
	at, _ := bidAskTime(q.ProviderT)
	qualified, _ := bidAskTime(q.QualifiedT)
	commit, _ := bidAskTime(rec.CommitT)
	fingerprint := bidAskQuoteFingerprint(q)
	original := q.EpisodeSignalSequence != ""
	// Identity is prefix-wide, independent of arrival adjacency, position state,
	// HTTP cursor and receipt time. Conflicting historical content is always an
	// atomic refusal; genuinely unseen late observations retain the late policy.
	knownFingerprint, known := x.quoteIdentities[at]
	if known && fingerprint != knownFingerprint {
		x.err = bidAskError("QUOTE_CONFLICT")
		return
	}
	if original && (x.pending == nil || x.pending.episode == nil || x.pending.view.SignalSequence != q.EpisodeSignalSequence) {
		x.err = bidAskError("EPISODE_STATE")
		return
	}
	if known {
		if original {
			x.noFill(rec, "duplicate-original-observation", bidAskQuoteClock(q))
		} else {
			x.emit(rec, BidAskEvent{Kind: "observation", Outcome: "duplicate-provider-observation", Basis: "no-new-tick", Clock: bidAskQuoteClock(q)})
		}
		return
	}
	if len(x.quoteIdentities) >= PaperBidAskMaxQuoteIdentities {
		x.err = bidAskError("QUOTE_IDENTITY_LIMIT")
		return
	}
	x.quoteIdentities[at] = fingerprint
	if at < x.lastQuoteT {
		if original {
			x.noFill(rec, "out-of-order-original-observation", bidAskQuoteClock(q))
		} else {
			x.emit(rec, BidAskEvent{Kind: "observation", Outcome: "late-observation", Basis: "no-retrospective-reordering", Clock: bidAskQuoteClock(q)})
			if x.position != nil && at >= x.position.entryT && at < x.position.fullFrom {
				x.unresolved(rec, "late-straddle-observation")
			}
		}
		return
	}
	x.lastQuoteT = at
	if original {
		d := x.pending
		admit, _ := bidAskTime(d.episode.AdmittedT)
		deadline, _ := bidAskTime(d.episode.DeadlineT)
		close, _ := bidAskTime(d.signal.CloseT)
		start, _ := bidAskTime(q.RequestStartT)
		if x.coverage != "sampled-observations-only" {
			x.noFill(rec, "coverage-unhealthy", bidAskQuoteClock(q))
			return
		}
		if !q.Tradeable {
			x.noFill(rec, "not-tradeable", bidAskQuoteClock(q))
			return
		}
		episodeCommit, _ := bidAskTime(d.episodeCommitT)
		if at < close || start < admit || start < episodeCommit || qualified-at > x.maxQuoteAge || commit > deadline || at >= close+x.maxSignalAge {
			x.noFill(rec, "original-quote-expired", bidAskQuoteClock(q))
			return
		}
		if slotEnd, ok := numericMetaValue(d.order.Meta["slotEnd"]); ok && at >= int64(slotEnd)*1000000 {
			x.noFill(rec, "source-slot-expired", bidAskQuoteClock(q))
			return
		}
		x.fill(rec, q)
		return
	}
	if x.position == nil || x.coverage != "sampled-observations-only" {
		return
	}
	p := x.position
	// At and after the first grid boundary there is one authority: completed side
	// bars. A delayed bar does not let these observations compete with its extrema.
	if at >= p.fullFrom {
		// An observation in a completed full interval cannot reopen its pending
		// resolution or downgrade the authoritative completed-bar account mark.
		if at < x.lastBarClose {
			return
		}
		p.view.Phase = "full-bar"
		x.collector.account.MarkStatus = "awaiting-completed-interval"
		return
	}
	if at <= p.entryT {
		return
	}
	if at < x.lastBarClose {
		x.emit(rec, BidAskEvent{Kind: "observation", Outcome: "late-observation", Basis: "finalized-straddle-interval", Clock: bidAskQuoteClock(q)})
		x.unresolved(rec, "late-straddle-observation")
		return
	}
	if !q.Tradeable || qualified-at > x.maxQuoteAge {
		x.unresolved(rec, "unqualified-straddle-observation")
		return
	}
	b := &x.prepared.broker
	tier := q.Bids[0]
	if b.position.Side == sideShort {
		tier = q.Asks[0]
	}
	price := bidAskRat(tier.Price)
	sl, e := bidAskFamily(b.position.SL)
	if e != nil {
		x.err = e
		return
	}
	tp, e := bidAskFamily(b.position.TP)
	if e != nil {
		x.err = e
		return
	}
	s := b.position.Side
	stop := (s == sideLong && price.Cmp(sl) <= 0) || (s == sideShort && price.Cmp(sl) >= 0)
	target := (s == sideLong && price.Cmp(tp) >= 0) || (s == sideShort && price.Cmp(tp) <= 0)
	if stop || target {
		if p.qty.Cmp(bidAskRat(tier.Liquidity)) > 0 {
			x.unresolved(rec, "insufficient-sampled-exit-liquidity")
			return
		}
		why := "target"
		if stop {
			why = "stop"
		}
		x.close(rec, price, why, "observed-liquidation-side-with-modeled-costs", bidAskQuoteClock(q))
		return
	}
	x.mark(rec, price, "sampled-straddle-liquidation-side-quote", bidAskQuoteClock(q))
}
func (x *bidAskRunner) onEpisode(rec BidAskRecord) {
	d := x.pending
	e := rec.Episode
	if d == nil || d.episode != nil || d.view.SignalSequence != e.SignalSequence {
		x.err = bidAskError("EPISODE_STATE")
		return
	}
	close, _ := bidAskTime(d.signal.CloseT)
	admit, _ := bidAskTime(e.AdmittedT)
	deadline, _ := bidAskTime(e.DeadlineT)
	limit := close + x.maxSignalAge
	if limit > close+x.duration {
		limit = close + x.duration
	}
	if slotEnd, ok := numericMetaValue(d.order.Meta["slotEnd"]); ok && int64(slotEnd)*1000000 < limit {
		limit = int64(slotEnd) * 1000000
	}
	signalCommit, _ := bidAskTime(d.signalCommitT)
	if admit < close || admit < signalCommit || deadline > limit || deadline <= admit {
		x.err = bidAskError("EPISODE_DEADLINE")
		return
	}
	copy := *e
	d.episode = &copy
	d.episodeCommitT = rec.CommitT
	d.view.State = "awaiting-original-commit-or-seal"
	d.view.DeadlineT = e.DeadlineT
}
func (x *bidAskRunner) onSeal(rec BidAskRecord) {
	d := x.pending
	s := rec.Seal
	if d == nil || d.episode == nil || d.view.SignalSequence != s.SignalSequence {
		x.err = bidAskError("EPISODE_STATE")
		return
	}
	at, _ := bidAskTime(s.EffectiveT)
	admit, _ := bidAskTime(d.episode.AdmittedT)
	deadline, _ := bidAskTime(d.episode.DeadlineT)
	if at < admit || (s.Reason == "deadline" && at < deadline) {
		x.err = bidAskError("SEAL_CLOCK")
		return
	}
	x.noFill(rec, "expired-"+s.Reason, BidAskClock{ModelEffectiveT: s.EffectiveT})
}
func executeBidAsk(r PaperBidAskRequest, p *PreparedRun, b BidAskBinding) (PaperBidAskResult, error) {
	x := newBidAskRunner(r, p, b)
	for _, rec := range r.Records {
		switch rec.Kind {
		case "bar":
			x.onBar(rec)
		case "episode":
			x.onEpisode(rec)
		case "quote":
			x.onQuote(rec)
		case "seal":
			x.onSeal(rec)
		case "coverage-gap":
			if x.position != nil {
				x.unresolved(rec, "recorded-coverage-gap")
			} else {
				x.coverage = "unhealthy-recorded-gap"
				x.collector.account.MarkStatus = "stale-coverage-gap"
				x.emit(rec, BidAskEvent{Kind: "coverage", Outcome: "recorded-coverage-gap", Basis: "entries-refused"})
			}
		}
		if x.err != nil {
			return PaperBidAskResult{}, x.err
		}
	}
	a, s := x.collector.snapshot()
	result := PaperBidAskResult{Schema: PaperBidAskSchema, Mode: "replay-verified-candidate", Binding: b, BindingDigest: x.identity, InputDigest: bidAskHash(r.Records), ProcessedRecords: len(r.Records), ProcessedBars: x.barIndex + 1, EventRange: PaperEventRange{From: 0, To: len(x.events)}, Events: x.events, Account: a, CompletedTrades: s, Coverage: x.coverage}
	if x.position != nil {
		v := x.position.view
		result.Position = &v
	}
	if x.pending != nil {
		v := x.pending.view
		result.Pending = &v
	}
	var state any
	if p != nil {
		state = struct {
			Last        int
			HasLast     bool
			SeenDay     int64
			SeenKeys    []string
			Protection  position
			HasPosition bool
			Pending     *bidAskDecision
		}{p.broker.orbLastEntry, p.broker.hasORBEntry, p.broker.seen.orb.day, append([]string{}, p.broker.seen.orb.keys...), p.broker.position, p.broker.hasPosition, x.pending}
	}
	// Private pending order data is included explicitly because its fields are not
	// exported; no mutable broker/account object is ever restored from the host.
	var pendingOrder any
	if x.pending != nil {
		pendingOrder = struct {
			Order                         order
			Signal                        BidAskBar
			Index                         int
			Episode                       *BidAskEpisode
			SignalCommitT, EpisodeCommitT string
		}{x.pending.order, x.pending.signal, x.pending.index, x.pending.episode, x.pending.signalCommitT, x.pending.episodeCommitT}
	}
	result.StateDigest = bidAskHash(struct {
		Account                               BidAskAccount
		Statistics                            BidAskStatistics
		Position                              *BidAskPosition
		Pending                               *BidAskPending
		Family                                any
		PendingOrder                          any
		Coverage                              string
		LastBarOpen, LastBarClose, LastQuoteT int64
		QuoteIdentities                       map[int64]string
	}{a, s, result.Position, result.Pending, state, pendingOrder, x.coverage, x.lastBarOpen, x.lastBarClose, x.lastQuoteT, x.quoteIdentities})
	if result.StateDigest == "" {
		return PaperBidAskResult{}, bidAskError("NONFINITE")
	}
	cp := bidAskCheckpointFor(result)
	result.Checkpoint, _ = jsonMarshalBidAsk(cp)
	return result, nil
}
