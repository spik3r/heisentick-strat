package engine

import (
	"encoding/json"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// OrdinaryBacktestPlan contains nonexecuting admission facts. Its prepared
// runner is private and never an execution grant: RunOrdinaryBacktest always
// prepares the current immutable capture again.
type OrdinaryBacktestPlan struct {
	admission *OrdinaryBacktestPreparation
	prepared  *PreparedRun
	preflight string
}

func (p *OrdinaryBacktestPlan) ConfigIdentity() string { return p.admission.ConfigIdentity() }
func (p *OrdinaryBacktestPlan) Classification() OrdinaryBacktestClassification {
	return p.admission.Classification()
}
func (p *OrdinaryBacktestPlan) DataValidated() bool { return p.admission.DataValidated() }
func (p *OrdinaryBacktestPlan) Preflight() string   { return p.preflight }
func (p *OrdinaryBacktestPlan) SourceName() string {
	return stringValue(p.admission.config, "name", "")
}

// PrepareOrdinaryBacktest shares deterministic admission with execution. It
// never runs the broker or attaches an observer, including for dataful inspect.
func PrepareOrdinaryBacktest(c *OrdinaryBacktestCapture) (plan *OrdinaryBacktestPlan, err error) {
	stage := "parse"
	defer func() {
		if recover() != nil {
			err = ordinaryFault("INTERNAL", stage, "")
		}
	}()
	if c == nil {
		return nil, ordinaryFault("INTERNAL", "transport", "")
	}
	// Keep the parse/hash boundary here so even a later admission panic retains
	// the actual digest. P1's classification/data helpers remain authoritative.
	parsed, parseErr := dsl.Parse(c.source)
	if parseErr != nil || len(parsed.Errors) != 0 || dsl.SequentialFullParseError(parsed) != nil {
		return nil, ordinaryFault("PARSE_REJECTED", "parse", "")
	}
	rawConfig, hashErr := json.Marshal(parsed.Config)
	if hashErr != nil {
		return nil, ordinaryFault("INTERNAL", "parse", "")
	}
	admission := &OrdinaryBacktestPreparation{config: parsed.Config, configSHA256: ordinaryIdentity(rawConfig).SHA256}
	plan = &OrdinaryBacktestPlan{admission: admission, preflight: "refused"}
	stage = "admission"
	admission.classification = ordinaryClassifyConfig(parsed.Config)
	if admission.classification.Kind == "dedicated" {
		return plan, ordinaryFault("DEDICATED_REQUIRED", stage, "")
	}
	if admission.classification.Kind != "ordinary" {
		return plan, ordinaryFault("ACCOUNTING_UNAVAILABLE", stage, "")
	}
	if c.request.Data != nil {
		if err = ordinaryValidateData(c.request.Route.Timeframe, c.request.Data); err != nil {
			return plan, err
		}
		admission.dataValidated = true
	}
	request := ordinaryRunRequest(c, admission.config)
	// Route capability precedes context allocation and all zero-trade shortcuts.
	if err = ordinaryRunCapability(request); err != nil {
		return plan, err
	}
	if err = ordinaryRunOptions(request); err != nil {
		return plan, err
	}
	if c.request.Data == nil {
		plan.preflight = "not-evaluated"
		return plan, nil
	}
	if err = validateRunRequest(request); err != nil {
		return plan, ordinaryFault("OPTIONS_UNSUPPORTED", "admission", "")
	}
	plan.prepared, err = PrepareRun(request)
	if err != nil {
		return plan, ordinaryFault("OPTIONS_UNSUPPORTED", "admission", "")
	}
	if err = ordinaryPreparedCapability(plan.prepared); err != nil {
		return plan, err
	}
	plan.prepared.fixture.Case = c.request.RequestID
	plan.preflight = "admitted"
	return plan, nil
}

func ordinaryRunRequest(c *OrdinaryBacktestCapture, cfg dsl.Config) RunRequest {
	r := c.request
	costs := r.Execution.Costs
	request := RunRequest{Config: cfg, StrategyID: r.StrategyID, Symbol: r.Route.Symbol,
		Timeframe: r.Route.Timeframe, RangeMethod: r.Execution.RangeMethod,
		Costs: Costs{FeePerUnit: costs.FeePerUnit, FillOn: costs.FillOn, Slippage: costs.Slippage,
			SlippageBps: costs.SlippageBps, StartEquity: costs.StartEquity}}
	if r.Data != nil {
		bars := make([]marketdata.Bar, len(r.Data.Bars))
		for i, row := range r.Data.Bars {
			bars[i] = marketdata.Bar{T: row[0], O: row[1], H: row[2], L: row[3], C: row[4], V: row[5]}
		}
		request.Series = marketdata.SeriesFromBars(bars)
	}
	return request
}

// This is the actual dispatch boundary in broker.runWithFinalization and
// runSpecialSetup, not a source/catalogue allowlist. Any new separate dispatcher
// requires a renewed capability review and corresponding rejection here.
func ordinaryLoopCapability(family string) error {
	switch family {
	case string(dsl.FamilyClockRangeBreakout), string(dsl.FamilyDownShockRebound),
		string(dsl.FamilyDailyFlushFailure), string(dsl.FamilyWeekendExtremeFade), string(dsl.FamilyIntraHourRunExhaustion):
		return ordinaryFault("ACCOUNTING_UNAVAILABLE", "admission", "")
	}
	return nil
}

func ordinaryRunCapability(r RunRequest) error {
	classification := ordinaryClassifyConfig(r.Config)
	if classification.Kind == "dedicated" {
		return ordinaryFault("DEDICATED_REQUIRED", "admission", "")
	}
	if classification.Kind != "ordinary" {
		return ordinaryFault("ACCOUNTING_UNAVAILABLE", "admission", "")
	}
	if err := ordinaryLoopCapability(classification.Family); err != nil {
		return err
	}
	if r.ExecutionWindow != nil || r.TimedCalendar != nil || r.ForceRoute ||
		r.SourceSeries.Len() != 0 || r.HTFSeries.Len() != 0 || r.SourceHTFSeries.Len() != 0 ||
		r.HigherTimeframe != "" || r.SourceTimeframe != "" && r.SourceTimeframe != r.Timeframe || sourceEntryRequest(r) {
		return ordinaryFault("ACCOUNTING_UNAVAILABLE", "admission", "")
	}
	return nil
}

func ordinaryRunOptions(r RunRequest) error {
	if _, ok := dsl.SequentialFullTimeframeMS(r.Timeframe); !ok {
		return ordinaryFault("ROUTE_UNSUPPORTED", "admission", "route.timeframe")
	}
	if !RouteAllowed(r.Config, r.Symbol, r.Timeframe, r.Series) {
		return ordinaryFault("ROUTE_UNSUPPORTED", "admission", "route")
	}
	// These are also exercised by PrepareRun with data. Text-only inspection
	// checks deterministic source/options without inventing indicator input.
	checks := []func() error{
		func() error { return validateLegacySetup9Execution(r.Config, r.Costs) },
		func() error { return validateSupplyDemandEntryRetest(r.Config, r.Timeframe) },
		func() error { return validateParabolicSARConfig(r.Config) },
		func() error { return validatePriceMomentumStopConfig(r.Config) },
		func() error { return validateDonchianBreakoutConfig(r.Config) },
		func() error { return validateParabolicSARRoute(r.Config, r.Timeframe, r.SourceTimeframe) },
		func() error { return validateRMVConfig(r.Config) },
		func() error { return validateRMVSourceEntryRoute(r.Symbol, r.Timeframe, r.Config) },
		func() error { return validateDoubleTopBottomPivotWindow(r.Config) },
		func() error { return validateSharedAdmissionLists(r.Config) },
		func() error { return validateSessionBreakHoldLevelPriority(r.Config) },
	}
	for _, check := range checks {
		if check() != nil {
			return ordinaryFault("OPTIONS_UNSUPPORTED", "admission", "")
		}
	}
	return nil
}

func ordinaryPreparedCapability(r *PreparedRun) error {
	if r == nil || r.sequentialFull != nil || r.c5 || r.windowed || r.execution.TradeStart != 0 {
		return ordinaryFault("ACCOUNTING_UNAVAILABLE", "admission", "")
	}
	if err := ordinaryLoopCapability(r.params.SetupType); err != nil {
		return err
	}
	if r.offRoute {
		return ordinaryFault("ROUTE_UNSUPPORTED", "admission", "route")
	}
	return nil
}
