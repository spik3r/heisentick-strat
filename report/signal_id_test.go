package report

import (
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestComputeSignalIDStableAcrossRuns(t *testing.T) {
	a := ComputeSignalID("dslDualEmaResumptionXauusdFourHour", "sha-1", "XAUUSD", "4h", 1700000000000, "long")
	b := ComputeSignalID("dslDualEmaResumptionXauusdFourHour", "sha-1", "XAUUSD", "4h", 1700000000000, "long")
	if a != b {
		t.Fatalf("same inputs produced different ids: %q vs %q", a, b)
	}
	if len(a) != SignalIDLength {
		t.Fatalf("signal id length = %d, want %d", len(a), SignalIDLength)
	}
}

func TestComputeSignalIDChangesWithEachInput(t *testing.T) {
	base := ComputeSignalID("strat-a", "v1", "XAUUSD", "4h", 1700000000000, "long")
	variants := []string{
		ComputeSignalID("strat-b", "v1", "XAUUSD", "4h", 1700000000000, "long"),  // strategy id
		ComputeSignalID("strat-a", "v2", "XAUUSD", "4h", 1700000000000, "long"),  // strategy version
		ComputeSignalID("strat-a", "v1", "EURUSD", "4h", 1700000000000, "long"),  // symbol
		ComputeSignalID("strat-a", "v1", "XAUUSD", "15m", 1700000000000, "long"), // timeframe
		ComputeSignalID("strat-a", "v1", "XAUUSD", "4h", 1700000000001, "long"),  // signal bar timestamp
		ComputeSignalID("strat-a", "v1", "XAUUSD", "4h", 1700000000000, "short"), // side
	}
	seen := map[string]bool{base: true}
	for i, v := range variants {
		if seen[v] {
			t.Fatalf("variant %d collided with a prior id: %q", i, v)
		}
		seen[v] = true
	}
}

func TestAnnotateReportTradesSetsStableSignalID(t *testing.T) {
	trade := engine.Trade{
		Entry: 100, Exit: 101, InitialSL: 98, SL: 98,
		EntryIndex: 0, ExitIndex: 1, EntryT: 1700000000000, ExitT: 1700003600000,
		Side: "long", Size: 1,
	}
	bars := []marketdata.Bar{{H: 102, L: 99}, {H: 105, L: 98}}
	series := marketdata.SeriesFromBars(bars)
	route := reportTradeRoute{Symbol: "XAUUSD", TF: "4h", StrategyID: "strat-a", StrategyVersion: "v1"}
	first, err := annotateReportTrades([]engine.Trade{trade}, series, nil, route)
	if err != nil {
		t.Fatal(err)
	}
	second, err := annotateReportTrades([]engine.Trade{trade}, series, nil, route)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].SignalID == "" {
		t.Fatal("expected a non-empty signalId")
	}
	if first[0].SignalID != second[0].SignalID {
		t.Fatalf("signalId not stable across identical runs: %q vs %q", first[0].SignalID, second[0].SignalID)
	}
	differentRoute := route
	differentRoute.Symbol = "EURUSD"
	third, err := annotateReportTrades([]engine.Trade{trade}, series, nil, differentRoute)
	if err != nil {
		t.Fatal(err)
	}
	if third[0].SignalID == first[0].SignalID {
		t.Fatal("signalId did not change when the route symbol changed")
	}
}
