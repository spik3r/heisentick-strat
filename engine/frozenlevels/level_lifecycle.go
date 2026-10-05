package frozenlevels

import "fmt"

const LifecycleVersion = "frozen-level-lifecycle-v1"

// LevelPolicy records a research hypothesis, never a recovered trading rule.
// Only one-use/no-retry is implemented. Busy skips do not consume a level;
// another signal still requires a fresh inside-to-outside crossing.
type LevelPolicy struct {
	AssumptionID            string `json:"assumptionId"`
	MaxPredecessorAgeMillis int64  `json:"maxPredecessorAgeMillis"`
	MaxObservationAgeMillis int64  `json:"maxObservationAgeMillis"`
	SubmissionLatencyMillis int64  `json:"submissionLatencyMillis"`
	UsePolicy               string `json:"usePolicy"`
}

// LevelSpec binds a directional level to its source window and decision-event
// order. KnownAt and CreatedAt belong to the explicitly qualified OrderingProof
// domain. The proof must place these context events and quotes in one stable
// chronological order. The package cannot infer that ordering from timestamps.
type LevelSpec struct {
	Version         string      `json:"version"`
	ProviderID      string      `json:"providerId"`
	SourceWindowID  string      `json:"sourceWindowId"`
	ContextSource   Source      `json:"contextSource"`
	QuoteSource     Source      `json:"quoteSource"`
	OrderingProof   string      `json:"orderingProof"`
	KnownAt         EventKey    `json:"knownAt"`
	CreatedAt       EventKey    `json:"createdAt"`
	ExpiresAtMillis int64       `json:"expiresAtMillis"`
	Side            Direction   `json:"side"`
	Price           float64     `json:"price"`
	Policy          LevelPolicy `json:"policy"`
}
type LevelQuote struct {
	Quote  Quote  `json:"quote"`
	Source Source `json:"source"`
}
type LevelEvent struct {
	Version           string   `json:"version"`
	LevelID           string   `json:"levelId"`
	Kind              string   `json:"kind"`
	Reason            string   `json:"reason,omitempty"`
	At                EventKey `json:"at"`
	AvailableAtMillis int64    `json:"availableAtMillis"`
	SignalID          string   `json:"signalId,omitempty"`
	ExpiresAtMillis   int64    `json:"expiresAtMillis"`
}

// LevelLifecycle records context/admission events only. It neither submits an
// order nor produces fills, positions, costs, P&L, OCO behavior or account state.
type LevelLifecycle struct {
	spec            LevelSpec
	id              string
	state           string
	threshold       price
	previous        *Quote
	armed           bool
	signalID        string
	signal          *Quote
	ready           *Quote
	submitNotBefore int64
}

func NewLevelLifecycle(spec LevelSpec, previous *LevelQuote) (*LevelLifecycle, LevelEvent, error) {
	p := spec.Policy
	if spec.Version != LifecycleVersion || spec.ProviderID == "" || spec.SourceWindowID == "" || spec.OrderingProof == "" || !spec.KnownAt.valid() || !spec.CreatedAt.valid() || spec.KnownAt.After(spec.CreatedAt) || !validMillis(spec.ExpiresAtMillis) || spec.ExpiresAtMillis <= spec.CreatedAt.AtMillis || !spec.Side.valid() || !finitePositive(spec.Price) || p.AssumptionID == "" || p.UsePolicy != "one-success-no-retry" || p.MaxPredecessorAgeMillis <= 0 || !validMillis(p.MaxPredecessorAgeMillis) || p.MaxObservationAgeMillis <= 0 || !validMillis(p.MaxObservationAgeMillis) || !validMillis(p.SubmissionLatencyMillis) {
		return nil, LevelEvent{}, fmt.Errorf("invalid-level-contract")
	}
	if err := spec.ContextSource.validate(); err != nil {
		return nil, LevelEvent{}, err
	}
	if err := spec.QuoteSource.validate(); err != nil {
		return nil, LevelEvent{}, err
	}
	if spec.ContextSource.Side != Bid || spec.QuoteSource.Side != Bid {
		return nil, LevelEvent{}, fmt.Errorf("unsupported-level-signal-side")
	}
	m := &LevelLifecycle{spec: spec, id: identity(spec), state: "active", threshold: inputPrice(spec.Price)}
	if previous != nil {
		q := previous.Quote
		if err := m.validateQuote(*previous); err != nil {
			return nil, LevelEvent{}, err
		}
		if !spec.CreatedAt.After(q.Event) || q.AvailableAtMillis > spec.CreatedAt.AtMillis {
			return nil, LevelEvent{}, fmt.Errorf("unproved-creation-predecessor")
		}
		if spec.CreatedAt.AtMillis-q.Event.AtMillis > p.MaxPredecessorAgeMillis {
			return nil, LevelEvent{}, fmt.Errorf("stale-creation-predecessor")
		}
		copy := q
		m.previous = &copy
		m.armed = m.inside(q.Bid)
	}
	return m, m.event("created", "", spec.CreatedAt, spec.CreatedAt.AtMillis), nil
}
func (m *LevelLifecycle) ID() string    { return m.id }
func (m *LevelLifecycle) State() string { return m.state }
func (m *LevelLifecycle) Terminal() bool {
	return m.state == "expired" || m.state == "invalidated" || m.state == "consumed"
}
func (m *LevelLifecycle) event(kind, reason string, key EventKey, available int64) LevelEvent {
	return LevelEvent{Version: LifecycleVersion, LevelID: m.id, Kind: kind, Reason: reason, At: key, AvailableAtMillis: available, SignalID: m.signalID, ExpiresAtMillis: m.spec.ExpiresAtMillis}
}
func (m *LevelLifecycle) inside(bid float64) bool {
	return directedDifference(inputPrice(bid), m.threshold, m.spec.Side).sign() < 0
}
func (m *LevelLifecycle) validateQuote(q LevelQuote) error {
	if q.Source != m.spec.QuoteSource {
		return fmt.Errorf("level-quote-source-mismatch")
	}
	if err := q.Quote.validate(); err != nil {
		return err
	}
	if q.Quote.AvailableAtMillis-q.Quote.Event.AtMillis > m.spec.Policy.MaxObservationAgeMillis {
		return fmt.Errorf("stale-level-observation")
	}
	return nil
}
func (m *LevelLifecycle) invalidate(q Quote, reason string) ([]LevelEvent, error) {
	m.state = "invalidated"
	return []LevelEvent{m.event("invalidated", reason, q.Event, q.AvailableAtMillis)}, fmt.Errorf("%s", reason)
}

// Observe expires before any signal/admission action and refuses malformed,
// duplicate, reordered or stale observations. Such input invalidates the level:
// callers cannot discard bad rows and continue the same level. A stale crossing
// PREDECESSOR instead resets the comparison to the fresh current observation.
// continuityVerified is an externally qualified assertion, not inferred here.
// busy means the caller's one-position admission guard currently blocks entry.
func (m *LevelLifecycle) Observe(observation LevelQuote, continuityVerified, busy bool) ([]LevelEvent, error) {
	if m.Terminal() {
		return nil, fmt.Errorf("level-lifecycle-terminal")
	}
	q := observation.Quote
	if err := m.validateQuote(observation); err != nil {
		return m.invalidate(q, err.Error())
	}
	if !q.Event.After(m.spec.CreatedAt) || q.AvailableAtMillis < m.spec.CreatedAt.AtMillis {
		return m.invalidate(q, "observation-before-level-creation")
	}
	if m.previous != nil && (!q.Event.After(m.previous.Event) || q.AvailableAtMillis < m.previous.AvailableAtMillis) {
		return m.invalidate(q, "non-subsequent-level-observation")
	}
	if !continuityVerified {
		return m.invalidate(q, "unverified-level-quote-gap")
	}
	if m.state == "ready" {
		return m.invalidate(q, "unresolved-admission-result")
	}
	if q.Event.AtMillis >= m.spec.ExpiresAtMillis || q.AvailableAtMillis >= m.spec.ExpiresAtMillis {
		m.state = "expired"
		return []LevelEvent{m.event("expired", "expiry-before-admission", q.Event, q.AvailableAtMillis)}, nil
	}
	previous := m.previous
	copy := q
	m.previous = &copy
	if m.state == "pending" {
		return m.observePending(q, busy)
	}
	currentInside := m.inside(q.Bid)
	if previous == nil {
		m.armed = currentInside
		reason := "already-outside"
		if currentInside {
			reason = "inside-observation"
		}
		return []LevelEvent{m.event("reset", reason, q.Event, q.AvailableAtMillis)}, nil
	}
	if q.Event.AtMillis-previous.Event.AtMillis > m.spec.Policy.MaxPredecessorAgeMillis {
		m.armed = currentInside
		return []LevelEvent{m.event("reset", "stale-predecessor", q.Event, q.AvailableAtMillis)}, nil
	}
	if currentInside {
		reset := !m.armed
		m.armed = true
		if reset {
			return []LevelEvent{m.event("reset", "inside-observation", q.Event, q.AvailableAtMillis)}, nil
		}
		return nil, nil
	}
	crossed := m.armed && m.inside(previous.Bid)
	m.armed = false
	if !crossed {
		return nil, nil
	}
	m.signalID = identity(struct {
		Version, LevelID string
		At               EventKey
	}{LifecycleVersion, m.id, q.Event})
	events := []LevelEvent{m.event("crossed", "bid-crossing", q.Event, q.AvailableAtMillis)}
	if busy {
		return append(events, m.event("skipped", "position-busy", q.Event, q.AvailableAtMillis)), nil
	}
	// Submission begins when the signal is available. Source events predating
	// submission cannot later become executable merely by arriving late.
	// This subtraction is positive for every admitted unexpired quote. Test
	// expiry before adding latency: even an otherwise overflowing submission
	// has the same crossed-then-expired audit, and addition is then bounded.
	if m.spec.Policy.SubmissionLatencyMillis >= m.spec.ExpiresAtMillis-q.AvailableAtMillis {
		m.state = "expired"
		return append(events, m.event("expired", "submission-not-before-expiry", q.Event, q.AvailableAtMillis)), nil
	}
	m.submitNotBefore = q.AvailableAtMillis + m.spec.Policy.SubmissionLatencyMillis
	signal := q
	m.signal = &signal
	m.state = "pending"
	return append(events, m.event("submitted", "admission-pending", q.Event, q.AvailableAtMillis)), nil
}
