package engine

import "github.com/spik3r/heisentick-strat/marketdata"

// SupplyDemandRetestTerminalPolicy names the unchanged native end-of-data rule.
const SupplyDemandRetestTerminalPolicy = "last-close-liquidation"

// SupplyDemandRetestExecution is the admitted execution and its raw accounting.
// The reporting package validates and reduces this into the dedicated public
// envelope. SequentialAccounting is reused as an internal accounting shape;
// its use does not assert Sequential strategy qualification.
type SupplyDemandRetestExecution struct {
	Run        RunResult                  `json:"run"`
	Accounting SequentialAccounting       `json:"accounting"`
	Identity   SupplyDemandRetestIdentity `json:"identity"`
	Terminal   SupplyDemandRetestTerminal `json:"terminal"`
}

// SupplyDemandRetestTerminal separates actual pending orders observed after the
// final supplied chart row from the final-close liquidation of a held position.
// Pending orders are unfilled observations, not synthetic economic events.
type SupplyDemandRetestTerminal struct {
	Policy               string `json:"policy"`
	Position             string `json:"position"`
	Liquidated           bool   `json:"liquidated"`
	EndOfTestTrades      int    `json:"endOfTestTrades"`
	PendingMarketOrders  int    `json:"pendingMarketOrders"`
	PendingLimitOrders   int    `json:"pendingLimitOrders"`
	PendingDeferredExits int    `json:"pendingDeferredExits"`
}

// RunSupplyDemandRetestWithAccounting admits exact request and DSL source bytes,
// then executes the existing source-entry S&D detector, scheduler and broker
// once. Only this companion selects the decision-time daily HTF policy. Any
// admission, output or accounting failure returns no partial execution.
func RunSupplyDemandRetestWithAccounting(requestJSON []byte, source string) (SupplyDemandRetestExecution, error) {
	admitted, err := prepareSupplyDemandRetest(requestJSON, source)
	if err != nil {
		return SupplyDemandRetestExecution{}, err
	}
	fixture := admitted.fixture
	chart := marketdata.SeriesFromBars(fixture.Bars)
	sourceSeries := marketdata.SeriesFromBars(fixture.SourceBars)
	daily := marketdata.SeriesFromBars(fixture.SourceHTFBars)
	capture := newSequentialAccountingCollector(fixture.Costs.StartEquity)
	capture.data.Profile = SupplyDemandRetestProfile
	capture.data.Policy = admitted.identity.HTFPolicy
	var terminal SupplyDemandRetestTerminal
	var trend []int8
	if admitted.identity.HTFMode == "notAgainst" {
		trend = supplyDemandRetestCompletedDailyTrend(sourceSeries, daily)
	}
	trades := runSourceRetestSeriesWithOptions(fixture, admitted.cfg, paramsFromConfig(admitted.cfg), chart, sourceSeries, daily,
		ExecutionBounds{TradeEnd: chart.Len() - 1}, false, &supplyDemandRetestOptions{
			sourceHTFTrend: trend, accounting: capture, terminal: &terminal,
		})
	if capture.err != nil {
		return SupplyDemandRetestExecution{}, supplyDemandReportError("invalid-derived-value", "accounting", -1)
	}
	run, err := checkedResultEnvelope(fixture, trades)
	if err != nil {
		return SupplyDemandRetestExecution{}, supplyDemandReportError("invalid-derived-value", "run", -1)
	}
	accounting, err := capture.finish(len(fixture.Bars), run.TradeCount)
	if err != nil {
		return SupplyDemandRetestExecution{}, supplyDemandReportError("invalid-derived-value", "accounting", -1)
	}
	return SupplyDemandRetestExecution{Run: run, Accounting: accounting, Identity: admitted.identity, Terminal: terminal}, nil
}

// supplyDemandRetestCompletedDailyTrend is the new companion's versioned daily
// O/C policy. Admission establishes fixed, UTC-grid-aligned four-hour source and
// daily durations, so neither duration nor an earlier eligibility decision can
// depend on a future row. Generic HTF helpers deliberately remain unchanged.
//
// Stale reuse keeps the inherited any-Saturday/at-least-seven-day predicate,
// evaluated only through the current completed four-hour decision. It has no
// maximum stale age and is not an exchange holiday calendar.
func supplyDemandRetestCompletedDailyTrend(source, daily marketdata.Series) []int8 {
	out := make([]int8, source.Len()) // htfUnavailable until a completed daily row
	p := -1
	for i, open := range source.T {
		decision := open + 4*60*60*1000
		for p+1 < daily.Len() && daily.T[p+1]+dayMS <= decision {
			p++
		}
		if p < 0 {
			continue
		}
		close := daily.T[p] + dayMS
		if decision-close >= dayMS && !spansWeekendClosure(close, decision) {
			continue
		}
		switch {
		case daily.C[p] > daily.O[p]:
			out[i] = trendUp
		case daily.C[p] < daily.O[p]:
			out[i] = trendDown
		default:
			out[i] = trendFlat
		}
	}
	return out
}
