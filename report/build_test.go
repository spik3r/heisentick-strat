package report

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/engine"
)

func TestBuildProducesStatisticsConsistentWithThePrimaryRow(t *testing.T) {
	cfg, route, strategy := loadConformanceCase(t, "money-risk-sizing")
	generatedAt := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	holdoutFrom := int64(route.Series.T[route.Series.Len()/2])
	document, err := Build(context.Background(), Request{
		Config:        cfg,
		Route:         route,
		StrategyID:    strategy,
		IncludeTrades: true,
		HoldoutFromT:  &holdoutFrom,
		GeneratedAt:   generatedAt,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if document.GeneratedAt != "2026-09-19T12:00:00Z" || document.EvidenceEnvelope.GeneratedAt != document.GeneratedAt {
		t.Fatalf("generatedAt = %q envelope %q", document.GeneratedAt, document.EvidenceEnvelope.GeneratedAt)
	}
	primary := document.Costs[document.PrimaryCost.Index]
	if primary.Label != "realistic" || primary.Trades == 0 {
		t.Fatalf("primary row = %+v, want realistic with trades", primary)
	}
	if document.Headline == nil || document.Headline.Trades != primary.Trades || document.Headline.Net != primary.Net || document.Headline.MaxDrawdown != primary.DD {
		t.Fatalf("headline %+v does not mirror primary row %+v", document.Headline, primary)
	}
	if *document.Headline.WinRate != primary.WinRate/100 {
		t.Fatalf("headline winRate %v is not the row's fraction %v", *document.Headline.WinRate, primary.WinRate/100)
	}
	trades := *document.Slices[0].Trades
	if len(trades) != primary.Trades {
		t.Fatalf("emitted %d trades, primary row counts %d", len(trades), primary.Trades)
	}
	if document.Groupings == nil {
		t.Fatal("groupings missing")
	}
	for name, groups := range map[string][]Group{"session": document.Groupings.Session, "side": document.Groupings.Side, "exitReason": document.Groupings.ExitReason, "year": document.Groupings.Year} {
		total := 0
		for _, group := range groups {
			total += group.Trades
		}
		if total != primary.Trades {
			t.Fatalf("%s groups count %d trades, want %d", name, total, primary.Trades)
		}
	}
	for _, trade := range trades {
		if trade.ReportSessionPhase != sessionPhase(trade.EntryT) {
			t.Fatalf("trade session phase %q differs from grouping key %q", trade.ReportSessionPhase, sessionPhase(trade.EntryT))
		}
	}
	if document.DateBounds == nil || document.DateBounds.Bars != route.Series.Len() || document.DateBounds.FirstT != int64(route.Series.T[0]) {
		t.Fatalf("dateBounds = %+v", document.DateBounds)
	}
	if document.Holdout == nil || document.Holdout.FromT != holdoutFrom || document.Holdout.InSample.Trades+document.Holdout.Holdout.Trades != primary.Trades {
		t.Fatalf("holdout = %+v", document.Holdout)
	}

	first, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("document JSON is not deterministic")
	}
	for _, key := range []string{`"headline":`, `"groupings":`, `"dateBounds":`, `"holdout":`} {
		if !strings.Contains(string(first), key) {
			t.Fatalf("document JSON lacks %s", key)
		}
	}
}

func TestBuildOmitsHoldoutAndTradesUnlessRequested(t *testing.T) {
	cfg, route, strategy := loadConformanceCase(t, "money-risk-sizing")
	document, err := Build(context.Background(), Request{Config: cfg, Route: route, StrategyID: strategy})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if document.Holdout != nil || document.TradeSchema != "" || document.Slices[0].Trades != nil {
		t.Fatalf("default document carried opt-in fields: %+v", document)
	}
	if document.Headline == nil || document.Headline.FirstTradeT == nil {
		t.Fatalf("headline trade times need the primary trades even without IncludeTrades: %+v", document.Headline)
	}
}

func TestBuildExecutionWindowReportsTradableBoundsAfterContextWarmup(t *testing.T) {
	cfg, route, strategy := loadConformanceCase(t, "money-risk-sizing")
	from := int64(route.Series.T[route.Series.Len()/2])
	step := int64(route.Series.T[1] - route.Series.T[0])
	to := int64(route.Series.T[route.Series.Len()-1]) + step
	document, err := Build(context.Background(), Request{
		Config: cfg, Route: route, StrategyID: strategy,
		ExecutionWindow: &engine.ExecutionWindow{TradeFromT: &from, TradeToT: &to},
	})
	if err != nil {
		t.Fatalf("Build window: %v", err)
	}
	wantBars := route.Series.Len() - route.Series.Len()/2
	if document.DateBounds == nil || document.DateBounds.FirstT != from || document.DateBounds.LastT != to-step || document.DateBounds.Bars != wantBars {
		t.Fatalf("date bounds = %+v, want %d..%d/%d bars", document.DateBounds, from, to-step, wantBars)
	}
	if document.Bars != wantBars || document.Slices[0].Bars != wantBars {
		t.Fatalf("document bars = %d/%d, want %d", document.Bars, document.Slices[0].Bars, wantBars)
	}
	for _, row := range document.Costs {
		if row.SlippageBps == 0 {
			continue
		}
		if row.Net == 0 && row.Trades > 0 {
			t.Fatalf("cost row lost PnL in window: %+v", row)
		}
	}
}

func TestBuildRejectsBadRequests(t *testing.T) {
	cfg, route, strategy := loadConformanceCase(t, "money-risk-sizing")
	cases := map[string]Request{
		"missing config":   {Route: route, StrategyID: strategy},
		"missing strategy": {Config: cfg, Route: route},
		"bad range": {Config: cfg, Route: func() Route {
			bad := route
			bad.Range = "wide"
			return bad
		}(), StrategyID: strategy},
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(context.Background(), request); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(cancelled, Request{Config: cfg, Route: route, StrategyID: strategy}); err != context.Canceled {
		t.Fatalf("cancelled context error = %v, want context.Canceled", err)
	}
}
