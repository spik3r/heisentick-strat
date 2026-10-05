package marketdata

import (
	"math"
	"strings"
	"testing"
)

func validResearchRows() []ResearchQuote {
	return []ResearchQuote{{1000, 1001, 100, 101}, {2000, 2001, 102, 103}, {3000, 3001, 104, 105}}
}

func mustResearchQuotes(t *testing.T, rows []ResearchQuote) ResearchQuotes {
	t.Helper()
	s, err := NewResearchQuotes(rows)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResearchQuoteValidation(t *testing.T) {
	tests := []struct {
		name string
		edit func([]ResearchQuote) []ResearchQuote
	}{
		{"empty", func(_ []ResearchQuote) []ResearchQuote { return nil }},
		{"zero event", func(q []ResearchQuote) []ResearchQuote { q[0].EventMS = 0; return q }},
		{"negative event", func(q []ResearchQuote) []ResearchQuote { q[0].EventMS = -1; return q }},
		{"time overflow", func(q []ResearchQuote) []ResearchQuote { q[2].AvailableMS = maxResearchMS + 1; return q }},
		{"available before event", func(q []ResearchQuote) []ResearchQuote { q[0].AvailableMS = 999; return q }},
		{"duplicate event", func(q []ResearchQuote) []ResearchQuote { q[1].EventMS = 1000; return q }},
		{"duplicate availability", func(q []ResearchQuote) []ResearchQuote { q[1].AvailableMS = 3001; return q }},
		{"unordered", func(q []ResearchQuote) []ResearchQuote { q[0], q[1] = q[1], q[0]; return q }},
		{"zero bid", func(q []ResearchQuote) []ResearchQuote { q[1].Bid = 0; return q }},
		{"crossed quote", func(q []ResearchQuote) []ResearchQuote { q[1].Ask = 101; return q }},
		{"NaN bid", func(q []ResearchQuote) []ResearchQuote { q[1].Bid = math.NaN(); return q }},
		{"NaN ask", func(q []ResearchQuote) []ResearchQuote { q[1].Ask = math.NaN(); return q }},
		{"infinite ask", func(q []ResearchQuote) []ResearchQuote { q[1].Ask = math.Inf(1); return q }},
		{"infinite bid", func(q []ResearchQuote) []ResearchQuote { q[1].Bid = math.Inf(-1); return q }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewResearchQuotes(tc.edit(validResearchRows())); err == nil {
				t.Fatal("invalid sample accepted")
			}
		})
	}
	rows := validResearchRows()
	s := mustResearchQuotes(t, rows)
	rows[0].Bid = math.NaN()
	q, ok := s.Before(2001, 2000)
	if !ok || q.Bid != 100 || q.EventMS != 1000 {
		t.Fatal("strict decision boundary or immutable snapshot violated")
	}
	q.Bid = 999
	if again, _ := s.Before(2001, 2000); again.Bid != 100 {
		t.Fatal("returned row mutates snapshot")
	}
}

func TestResearchQuoteSelection(t *testing.T) {
	s := mustResearchQuotes(t, validResearchRows())
	for _, tc := range []struct{ decision, age int64 }{{1001, 10}, {2002, 1}, {2002, 0}, {0, 10}} {
		if _, ok := s.Before(tc.decision, tc.age); ok {
			t.Errorf("invalid/stale decision accepted: %+v", tc)
		}
	}
	if q, ok := s.Before(2002, 2); !ok || q.EventMS != 2000 {
		t.Fatal("inclusive age bound failed")
	}
	for _, tc := range []struct{ submit, deadline, age int64 }{{2000, 2000, 100}, {2002, 2999, 100}, {2000, 2001, 0}, {2000, 1999, 100}} {
		if _, ok := s.FirstAfter(tc.submit, tc.deadline, tc.age); ok {
			t.Errorf("invalid/missing observation accepted: %+v", tc)
		}
	}
	if q, ok := s.FirstAfter(2000, 2001, 1); !ok || q.EventMS != 2000 {
		t.Fatal("inclusive submission/deadline failed")
	}
	late := mustResearchQuotes(t, []ResearchQuote{{1000, 5000, 10, 11}, {6000, 6100, 12, 13}})
	if q, ok := late.FirstAfter(4000, 6200, 200); !ok || q.EventMS != 6000 {
		t.Fatal("late pre-submission event used as an executable observation")
	}
	stale := mustResearchQuotes(t, []ResearchQuote{{4000, 5500, 10, 11}, {6000, 6100, 12, 13}})
	if q, ok := stale.FirstAfter(4000, 6200, 200); !ok || q.EventMS != 6000 {
		t.Fatal("stale quote not skipped inside bounded window")
	}
	var empty ResearchQuotes
	if _, ok := empty.Before(2000, 1000); ok {
		t.Fatal("zero-value sample accepted")
	}
}

func researchWindowFixture() (ResearchWindow, []ResearchQuote) {
	w := ResearchWindow{Source: ResearchSession{1000, 10000}, Execution: ResearchSession{1200, 9000},
		DecisionMS: 3000, EntrySubmitMS: 4000, EntryDeadlineMS: 5000, ExitSubmitMS: 7000,
		ExitDeadlineMS: 8000, MaxSignalAgeMS: 1000, MaxQuoteAgeMS: 100}
	q := []ResearchQuote{{2500, 2501, 100, 101}, {3000, 3001, 200, 201}, {4000, 4001, 102, 103},
		{7000, 7001, 104, 105}, {7900, 7901, 106, 107}, {8999, 8999, 999, 1000}}
	return w, q
}

func TestResearchWindowCausalityAndUnresolvedExit(t *testing.T) {
	w, rows := researchWindowFixture()
	result, err := InspectResearchWindow(mustResearchQuotes(t, rows), w)
	if err != nil || result.Status != "available" || result.Signal.EventMS != 2500 || result.Entry.EventMS != 4000 || result.Exit.EventMS != 7000 {
		t.Fatalf("causal scheduled observations failed: %+v %v", result, err)
	}
	// The post-submission first exit must not be replaced by a favorable later
	// quote, including the last tick before the venue break.
	rows[4].Bid, rows[4].Ask = 1e6, 1e6+1
	result, _ = InspectResearchWindow(mustResearchQuotes(t, rows), w)
	if result.Exit.EventMS != 7000 {
		t.Fatal("future arrival altered selected exit")
	}
	result, err = InspectResearchWindow(mustResearchQuotes(t, rows[:3]), w)
	if err != nil || result.Status != "unresolved_exit" || result.Entry == nil || result.Exit != nil {
		t.Fatalf("missing exit discarded entry or fabricated exit: %+v %v", result, err)
	}
	result, _ = InspectResearchWindow(mustResearchQuotes(t, rows[:2]), w)
	if result.Status != "entry_missing" || result.Entry != nil {
		t.Fatal("missing entry admitted")
	}
	result, _ = InspectResearchWindow(mustResearchQuotes(t, rows[1:]), w)
	if result.Status != "signal_missing" || result.Signal != nil {
		t.Fatal("same-decision quote leaked into predictor")
	}
	rows[0].EventMS, rows[0].AvailableMS = 1100, 2501
	w.MaxSignalAgeMS = 5000
	result, _ = InspectResearchWindow(mustResearchQuotes(t, rows), w)
	if result.Status != "signal_missing" {
		t.Fatal("signal from outside execution session admitted")
	}
}

func TestResearchWindowContractRefusesUnsafeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ResearchWindow)
	}{
		{"missing calendar", func(w *ResearchWindow) { w.Source = ResearchSession{} }},
		{"reversed session", func(w *ResearchWindow) { w.Source.OpenMS = w.Source.CloseMS }},
		{"entry at decision", func(w *ResearchWindow) { w.EntrySubmitMS = w.DecisionMS }},
		{"entry deadline before submit", func(w *ResearchWindow) { w.EntryDeadlineMS = 3999 }},
		{"exit during entry window", func(w *ResearchWindow) { w.ExitSubmitMS = w.EntryDeadlineMS }},
		{"exit deadline before submit", func(w *ResearchWindow) { w.ExitDeadlineMS = 6999 }},
		{"exit at execution close", func(w *ResearchWindow) { w.ExitDeadlineMS = w.Execution.CloseMS }},
		{"exit at source close", func(w *ResearchWindow) { w.Source.CloseMS = w.ExitDeadlineMS }},
		{"decision at session open", func(w *ResearchWindow) { w.Source.OpenMS = w.DecisionMS }},
		{"zero signal age", func(w *ResearchWindow) { w.MaxSignalAgeMS = 0 }},
		{"negative quote age", func(w *ResearchWindow) { w.MaxQuoteAgeMS = -1 }},
		{"invalid timestamp", func(w *ResearchWindow) { w.ExitSubmitMS = maxResearchMS + 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, rows := researchWindowFixture()
			tc.edit(&w)
			if _, err := InspectResearchWindow(mustResearchQuotes(t, rows), w); err == nil {
				t.Fatal("unsafe window accepted")
			}
		})
	}
}

func TestResearchValidationErrorsIdentifyIndexNotPrices(t *testing.T) {
	rows := validResearchRows()
	rows[1].Bid = math.NaN()
	_, err := NewResearchQuotes(rows)
	if err == nil || !strings.Contains(err.Error(), "quote 1:") || strings.Contains(err.Error(), "NaN") {
		t.Fatalf("error should locate bad row without printing data: %v", err)
	}
}
