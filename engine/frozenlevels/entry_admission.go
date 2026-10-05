package frozenlevels

import "fmt"

func (m *LevelLifecycle) observePending(q Quote, busy bool) ([]LevelEvent, error) {
	if !q.Event.After(m.signal.Event) {
		return m.invalidate(q, "non-subsequent-pending-observation")
	}
	if q.Event.AtMillis < m.submitNotBefore {
		return nil, nil
	}
	if busy {
		m.state = "active"
		m.signal = nil
		m.armed = m.inside(q.Bid)
		return []LevelEvent{m.event("skipped", "position-busy-at-admission", q.Event, q.AvailableAtMillis)}, nil
	}
	ready := q
	m.ready = &ready
	m.state = "ready"
	return []LevelEvent{m.event("admission-ready", "strictly-subsequent-source-observation", q.Event, q.AvailableAtMillis)}, nil
}

// ResolveAdmission must follow the exact admission-ready event before another
// observation. Consumed means an external caller attests a successful execution;
// this primitive has not priced or executed it. Rejection consumes the attempt
// without retry. A busy skip preserves one-use eligibility but requires a later
// fresh crossing; no additions or OCO relationship are inferred.
func (m *LevelLifecycle) ResolveAdmission(at EventKey, outcome string) (LevelEvent, error) {
	if m.Terminal() {
		return LevelEvent{}, fmt.Errorf("level-lifecycle-terminal")
	}
	if m.state != "ready" || m.ready == nil {
		return LevelEvent{}, fmt.Errorf("no-ready-admission")
	}
	if at != m.ready.Event {
		return LevelEvent{}, fmt.Errorf("admission-key-mismatch")
	}
	q := *m.ready
	switch outcome {
	case "consumed":
		m.state = "consumed"
	case "rejected":
		m.state = "invalidated"
	case "busy":
		m.state = "active"
		m.armed = m.inside(q.Bid)
	default:
		return LevelEvent{}, fmt.Errorf("unsupported-admission-outcome")
	}
	m.ready = nil
	m.signal = nil
	kind, reason := "consumed", "external-successful-execution"
	if outcome == "rejected" {
		kind, reason = "invalidated", "external-execution-rejected-no-retry"
	}
	if outcome == "busy" {
		kind, reason = "skipped", "position-busy-at-resolution"
	}
	return m.event(kind, reason, q.Event, q.AvailableAtMillis), nil
}
