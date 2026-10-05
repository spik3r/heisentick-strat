package marketdata

import (
	"fmt"
	"math"
	"sort"
)

// ResearchQuote distinguishes the source event time from the time the quote
// became available. Both are UTC Unix milliseconds. Availability may be a
// separately declared latency model; these fields do not establish provenance.
type ResearchQuote struct {
	EventMS     int64
	AvailableMS int64
	Bid         float64
	Ask         float64
}

// ResearchQuotes is an immutable, strictly ordered bid/ask sample. It is not a
// market-data decoder, coverage certificate, fill model, or trading interface.
type ResearchQuotes struct{ rows []ResearchQuote }

const maxResearchMS int64 = 253402300799999 // 9999-12-31T23:59:59.999Z

func researchTime(t int64) bool { return t > 0 && t <= maxResearchMS }

// NewResearchQuotes copies input without sorting, repairing or deduplicating.
// Same-millisecond events require a separately reviewed sequence contract and
// are refused here, even if an upstream source legitimately emits them.
func NewResearchQuotes(input []ResearchQuote) (ResearchQuotes, error) {
	if len(input) == 0 {
		return ResearchQuotes{}, fmt.Errorf("empty quote sample")
	}
	for i, q := range input {
		if !researchTime(q.EventMS) || !researchTime(q.AvailableMS) || q.AvailableMS < q.EventMS {
			return ResearchQuotes{}, fmt.Errorf("quote %d: invalid event/availability time", i)
		}
		if math.IsNaN(q.Bid) || math.IsInf(q.Bid, 0) || math.IsNaN(q.Ask) || math.IsInf(q.Ask, 0) || q.Bid <= 0 || q.Ask < q.Bid {
			return ResearchQuotes{}, fmt.Errorf("quote %d: invalid bid/ask", i)
		}
		if i > 0 && (q.EventMS <= input[i-1].EventMS || q.AvailableMS <= input[i-1].AvailableMS) {
			return ResearchQuotes{}, fmt.Errorf("quote %d: unordered or duplicate event/availability time", i)
		}
	}
	return ResearchQuotes{rows: append([]ResearchQuote(nil), input...)}, nil
}

// Before returns the newest quote available strictly before decisionMS. Quote
// age is measured from event time, not receipt time. No stale fallback occurs.
func (s ResearchQuotes) Before(decisionMS, maxAgeMS int64) (ResearchQuote, bool) {
	if !researchTime(decisionMS) || maxAgeMS <= 0 {
		return ResearchQuote{}, false
	}
	i := sort.Search(len(s.rows), func(i int) bool { return s.rows[i].AvailableMS >= decisionMS }) - 1
	if i < 0 || decisionMS-s.rows[i].EventMS > maxAgeMS {
		return ResearchQuote{}, false
	}
	return s.rows[i], true
}

// FirstAfter returns the first fresh quote with event time at/after submission
// and availability at/before the inclusive deadline. Earlier market events
// arriving late cannot be used as executable post-submission observations.
// This selects an observation only; actual fillability remains unverified.
func (s ResearchQuotes) FirstAfter(submitMS, deadlineMS, maxAgeMS int64) (ResearchQuote, bool) {
	if !researchTime(submitMS) || !researchTime(deadlineMS) || deadlineMS < submitMS || maxAgeMS <= 0 {
		return ResearchQuote{}, false
	}
	i := sort.Search(len(s.rows), func(i int) bool { return s.rows[i].EventMS >= submitMS })
	for ; i < len(s.rows) && s.rows[i].AvailableMS <= deadlineMS; i++ {
		q := s.rows[i]
		if q.AvailableMS-q.EventMS <= maxAgeMS {
			return q, true
		}
	}
	return ResearchQuote{}, false
}

// ResearchSession is one explicit contiguous UTC quote/trading interval.
// Calendar eligibility and historically applicable schedules must be verified
// externally. Open is inclusive, Close is exclusive. Weekdays are not inferred.
type ResearchSession struct {
	OpenMS  int64
	CloseMS int64
}

// ResearchWindow is a prespecified synthetic/preflight observation contract.
// Sessions are the intersection of separately supplied source and execution
// venue intervals. No clock, financing cutoff, or numeric default is inferred.
type ResearchWindow struct {
	Source, Execution              ResearchSession
	DecisionMS                     int64
	EntrySubmitMS, EntryDeadlineMS int64
	ExitSubmitMS, ExitDeadlineMS   int64
	MaxSignalAgeMS, MaxQuoteAgeMS  int64
}

func (w ResearchWindow) validate() error {
	for _, v := range []int64{w.Source.OpenMS, w.Source.CloseMS, w.Execution.OpenMS, w.Execution.CloseMS,
		w.DecisionMS, w.EntrySubmitMS, w.EntryDeadlineMS, w.ExitSubmitMS, w.ExitDeadlineMS} {
		if !researchTime(v) {
			return fmt.Errorf("invalid window/session timestamp")
		}
	}
	if w.MaxSignalAgeMS <= 0 || w.MaxQuoteAgeMS <= 0 ||
		w.Source.OpenMS >= w.Source.CloseMS || w.Execution.OpenMS >= w.Execution.CloseMS {
		return fmt.Errorf("invalid session or finite age limit")
	}
	if w.DecisionMS >= w.EntrySubmitMS || w.EntrySubmitMS > w.EntryDeadlineMS ||
		w.EntryDeadlineMS >= w.ExitSubmitMS || w.ExitSubmitMS > w.ExitDeadlineMS {
		return fmt.Errorf("window must order decision < entry <= deadline < exit <= deadline")
	}
	for _, session := range []ResearchSession{w.Source, w.Execution} {
		if w.DecisionMS <= session.OpenMS || w.ExitDeadlineMS >= session.CloseMS {
			return fmt.Errorf("decision and exit deadline must lie strictly inside both sessions")
		}
	}
	return nil
}

// ResearchWindowEvidence preserves an entered window with a missing exit.
// Status is one of signal_missing, entry_missing, unresolved_exit, available.
// Available means only that these observations satisfy the supplied contract.
// Nil observations are never zero returns. There is no readiness or P&L field.
type ResearchWindowEvidence struct {
	Status string
	Signal *ResearchQuote
	Entry  *ResearchQuote
	Exit   *ResearchQuote
}

// InspectResearchWindow checks synthetic/preflight observation causality. It
// neither computes strategy directions/returns nor infers market coverage.
// The exit query is forward from a prescheduled submission, never the last
// quote selected retrospectively before a break.
func InspectResearchWindow(s ResearchQuotes, w ResearchWindow) (ResearchWindowEvidence, error) {
	if err := w.validate(); err != nil {
		return ResearchWindowEvidence{}, err
	}
	result := ResearchWindowEvidence{Status: "signal_missing"}
	signal, ok := s.Before(w.DecisionMS, w.MaxSignalAgeMS)
	if !ok || signal.EventMS < w.Source.OpenMS || signal.EventMS < w.Execution.OpenMS {
		return result, nil
	}
	result.Signal = &signal
	result.Status = "entry_missing"
	entry, ok := s.FirstAfter(w.EntrySubmitMS, w.EntryDeadlineMS, w.MaxQuoteAgeMS)
	if !ok {
		return result, nil
	}
	result.Entry = &entry
	result.Status = "unresolved_exit"
	exit, ok := s.FirstAfter(w.ExitSubmitMS, w.ExitDeadlineMS, w.MaxQuoteAgeMS)
	if !ok {
		return result, nil
	}
	result.Exit = &exit
	result.Status = "available"
	return result, nil
}
