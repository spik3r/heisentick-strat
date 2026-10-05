package frozenlevels

import (
	"fmt"
)

type LockMode string

const (
	NoLock    LockMode = "none"
	ScaleLock LockMode = "prior-open-scale"
	PivotLock LockMode = "frozen-pivot"
)

// LockPolicy has no implicit trading defaults. Reference experiments may
// explicitly select +1R and zero extra latency; neither is recovered behavior.
type LockPolicy struct {
	Mode          LockMode `json:"mode"`
	ActivationR   float64  `json:"activationR"`
	Grid          float64  `json:"grid"`
	LatencyMillis int64    `json:"latencyMillis"`
	AssumptionID  string   `json:"assumptionId"`
}
type Quote struct {
	Event             EventKey `json:"event"`
	AvailableAtMillis int64    `json:"availableAtMillis"`
	Bid               float64  `json:"bid"`
	Ask               float64  `json:"ask"`
}

func (q Quote) validate() error {
	if !q.Event.valid() || !validMillis(q.AvailableAtMillis) || q.AvailableAtMillis < q.Event.AtMillis || !finitePositive(q.Bid) || !finitePositive(q.Ask) || q.Ask < q.Bid {
		return fmt.Errorf("invalid-quote")
	}
	return nil
}
func (q Quote) liquidation(side Direction) float64 {
	if side == Long {
		return q.Bid
	}
	return q.Ask
}

type LockLevels struct {
	ID             string    `json:"id"`
	SourceWindowID string    `json:"sourceWindowId"`
	KnownAtMillis  int64     `json:"knownAtMillis"`
	FrozenAtMillis int64     `json:"frozenAtMillis"`
	Prices         []float64 `json:"prices"`
}

// PositionSeed is a previously established position, not an entry instruction.
// The surrounding engine must determine executable entry and structural risk.
// EntryQuote.Event is the actual establishment key, not notification time.
// Context has millisecond precision without a source-sequence key, so it must
// be frozen strictly before that millisecond; equal-time context fails closed.
type PositionSeed struct {
	Side        Direction `json:"side"`
	Entry       float64   `json:"entry"`
	InitialStop float64   `json:"initialStop"`
	EntryQuote  Quote     `json:"entryQuote"`
}
type LockEvent struct {
	Kind              string   `json:"kind"`
	Reason            string   `json:"reason,omitempty"`
	Event             EventKey `json:"event"`
	AvailableAtMillis int64    `json:"availableAtMillis"`
	ActiveStop        float64  `json:"activeStop"`
	ProposedStop      float64  `json:"proposedStop,omitempty"`
}
type LockCounters struct {
	Activations int `json:"activations"`
	Eligible    int `json:"eligible"`
	Submitted   int `json:"submitted"`
	Accepted    int `json:"accepted"`
	Rejected    int `json:"rejected"`
	Canceled    int `json:"canceled"`
}

// LockTracker is the shared Go request/acceptance primitive. It emits due-order
// events, never fills, trade P&L, account returns or quote-completeness claims.
// Old barriers have precedence over acceptance. Once activated, there is one
// attempt only. The initial stop remains active while the attempt is pending.
type LockTracker struct {
	entry      price
	grid       price
	activation price
	seed       PositionSeed
	policy     LockPolicy
	target     price
	activeStop price
	scaleStop  price
	levels     []price
	last       Quote
	requested  *Quote
	proposed   price
	attempted  bool
	terminal   bool
	counters   LockCounters
}

func NewLockTracker(seed PositionSeed, policy LockPolicy, opening *FrozenOpening, levels *LockLevels) (*LockTracker, error) {
	if err := seed.EntryQuote.validate(); err != nil {
		return nil, err
	}
	if !seed.Side.valid() || !finitePositive(seed.Entry) || !finitePositive(seed.InitialStop) || !finitePositive(policy.Grid) || !finitePositive(policy.ActivationR) || !validMillis(policy.LatencyMillis) || policy.AssumptionID == "" {
		return nil, fmt.Errorf("invalid-lock-contract")
	}
	if policy.Mode != NoLock && policy.Mode != ScaleLock && policy.Mode != PivotLock {
		return nil, fmt.Errorf("unsupported-lock-mode")
	}
	entry, stop, grid := inputPrice(seed.Entry), inputPrice(seed.InitialStop), inputPrice(policy.Grid)
	risk := directedDifference(entry, stop, seed.Side)
	target := entry.add(risk.mul(inputPrice(float64(seed.Side) * 2)))
	activation := risk.mul(inputPrice(policy.ActivationR))
	initialLiquidation := inputPrice(seed.EntryQuote.liquidation(seed.Side))
	if risk.sign() <= 0 || !finitePositive(target.project()) || !finitePositive(activation.project()) || directedDifference(initialLiquidation, stop, seed.Side).cmp(grid) < 0 {
		return nil, fmt.Errorf("invalid-initial-risk-geometry")
	}
	if directedDifference(initialLiquidation, target, seed.Side).sign() >= 0 {
		return nil, fmt.Errorf("seed-target-already-due")
	}
	t := &LockTracker{seed: seed, policy: policy, entry: entry, grid: grid, activation: activation, target: target, activeStop: stop, last: seed.EntryQuote, proposed: inputPrice(0)}
	if policy.Mode == ScaleLock {
		if opening == nil || opening.FrozenAtMillis >= seed.EntryQuote.Event.AtMillis {
			return nil, fmt.Errorf("missing-or-future-scale-context")
		}
		_, location, err := priorOpenScaleLocation(*opening, seed.Entry, seed.Side, policy.Grid)
		if err != nil {
			return nil, err
		}
		t.scaleStop = location
	} else if opening != nil {
		return nil, fmt.Errorf("unused-scale-context")
	}
	if policy.Mode == PivotLock {
		if levels == nil || levels.ID == "" || levels.SourceWindowID == "" || !validMillis(levels.KnownAtMillis) || !validMillis(levels.FrozenAtMillis) || levels.KnownAtMillis > levels.FrozenAtMillis || levels.FrozenAtMillis >= seed.EntryQuote.Event.AtMillis {
			return nil, fmt.Errorf("missing-or-future-pivot-context")
		}
		for _, p := range levels.Prices {
			if !finitePositive(p) {
				return nil, fmt.Errorf("invalid-pivot-price")
			}
		}
		for _, p := range levels.Prices {
			t.levels = append(t.levels, inputPrice(p))
		}
	} else if levels != nil {
		return nil, fmt.Errorf("unused-pivot-context")
	}
	return t, nil
}
func (t *LockTracker) ActiveStop() float64    { return t.activeStop.project() }
func (t *LockTracker) CommonTarget() float64  { return t.target.project() }
func (t *LockTracker) Counters() LockCounters { return t.counters }
func (t *LockTracker) Terminal() bool         { return t.terminal }
func (t *LockTracker) event(kind, reason string, q Quote) LockEvent {
	return LockEvent{Kind: kind, Reason: reason, Event: q.Event, AvailableAtMillis: q.AvailableAtMillis, ActiveStop: t.activeStop.project(), ProposedStop: t.proposed.project()}
}
func (t *LockTracker) admissible(stop, liq price) bool {
	return stop.sign() > 0 && finitePositive(stop.project()) && directedDifference(stop, t.entry, t.seed.Side).sign() > 0 && directedDifference(stop, t.activeStop, t.seed.Side).sign() > 0 && directedDifference(liq, stop, t.seed.Side).cmp(t.grid) >= 0
}

// Observe requires the caller to have verified continuity since the preceding
// quote. False makes the position unresolved and terminal; later prices cannot
// silently repair the missing path. No stop-trigger reaction delay is modeled.
func (t *LockTracker) Observe(q Quote, continuityVerified bool) ([]LockEvent, error) {
	if t.terminal {
		return nil, fmt.Errorf("lock-tracker-terminal")
	}
	if err := q.validate(); err != nil {
		return nil, err
	}
	if !q.Event.After(t.last.Event) || q.AvailableAtMillis < t.last.AvailableAtMillis {
		return nil, fmt.Errorf("non-subsequent-observation")
	}
	if !continuityVerified {
		t.terminal = true
		return []LockEvent{t.event("unresolved", "unverified-quote-gap", q)}, nil
	}
	t.last = q
	liq := inputPrice(q.liquidation(t.seed.Side))
	var events []LockEvent
	if directedDifference(liq, t.activeStop, t.seed.Side).sign() <= 0 || directedDifference(liq, t.target, t.seed.Side).sign() >= 0 {
		reason := "stop"
		if directedDifference(liq, t.activeStop, t.seed.Side).sign() > 0 {
			reason = "target"
		}
		if t.requested != nil {
			t.counters.Canceled++
			events = append(events, t.event("canceled", "old-order-before-amendment", q))
			t.requested = nil
		}
		t.terminal = true
		return append(events, t.event("exit-due", reason, q)), nil
	}
	if t.requested != nil {
		// Subtract only after monotonic availability validation to avoid overflow.
		if q.AvailableAtMillis-t.requested.AvailableAtMillis < t.policy.LatencyMillis {
			return nil, nil
		}
		if t.admissible(t.proposed, liq) {
			t.activeStop = t.proposed
			t.counters.Accepted++
			events = append(events, t.event("accepted", "", q))
		} else {
			t.counters.Rejected++
			events = append(events, t.event("rejected", "invalid-at-acceptance", q))
		}
		t.requested = nil
		return events, nil
	}
	if t.attempted || t.policy.Mode == NoLock {
		return nil, nil
	}
	if directedDifference(liq, t.entry, t.seed.Side).cmp(t.activation) < 0 {
		return nil, nil
	}
	t.attempted = true
	t.counters.Activations++
	events = append(events, t.event("activated", "", q))
	if t.policy.Mode == ScaleLock {
		t.proposed = t.scaleStop
	} else {
		best := inputPrice(0)
		for _, raw := range t.levels {
			if directedDifference(raw, t.entry, t.seed.Side).sign() <= 0 || directedDifference(liq, raw, t.seed.Side).cmp(t.grid) < 0 {
				continue
			}
			stop := raw.snap(t.grid, t.seed.Side == Short)
			if t.admissible(stop, liq) && (best.sign() == 0 || directedDifference(stop, best, t.seed.Side).sign() > 0) {
				best = stop
			}
		}
		t.proposed = best
	}
	if !t.admissible(t.proposed, liq) {
		t.counters.Rejected++
		return append(events, t.event("rejected", "no-eligible-lock", q)), nil
	}
	t.counters.Eligible++
	t.counters.Submitted++
	copy := q
	t.requested = &copy
	return append(events, t.event("submitted", "", q)), nil
}
