package engine

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"reflect"
)

// OrdinaryBacktestRun retains one checked native result, its contemporaneous
// state witness and raw finances from the same finalized broker lifecycle.
// Accessors return independent copies; no caller can replace the retained link.
type OrdinaryBacktestRun struct {
	native     RunResult
	financials OrdinaryBacktestFinancials
	states     []ordinaryBacktestState
}

func (r *OrdinaryBacktestRun) NativeResult() RunResult {
	// The exact same bounded typed copier succeeded before publication. A fresh
	// copy retains nil/empty distinctions, metadata types and no-stop/target flags.
	copier := &ordinaryBacktestStateCapture{}
	value, err := copier.copyValue(reflect.ValueOf(r.native), 0)
	if err != nil {
		panic("invalid retained ordinary result")
	}
	return value.Interface().(RunResult)
}
func (r *OrdinaryBacktestRun) Financials() OrdinaryBacktestFinancials {
	f := r.financials
	f.Events = append([]OrdinaryBacktestFinancialEvent(nil), f.Events...)
	f.Marks = append([]OrdinaryBacktestFinancialMark(nil), f.Marks...)
	return f
}

// RunOrdinaryBacktest does not consume an earlier inspection or mutable native
// RunRequest. It independently parses and admits this exact current capture.
func RunOrdinaryBacktest(c *OrdinaryBacktestCapture) (result *OrdinaryBacktestRun, plan *OrdinaryBacktestPlan, err error) {
	stage := "transport"
	defer func() {
		if recover() != nil {
			result = nil
			err = ordinaryFault("INTERNAL", stage, "")
		}
	}()
	if c == nil || c.request.Operation != "run" {
		return nil, nil, ordinaryFault("ARGUMENTS", "transport", "")
	}
	plan, err = PrepareOrdinaryBacktest(c)
	if err != nil {
		return nil, plan, err
	}
	stage = "admission"
	if plan.preflight != "admitted" || plan.prepared == nil {
		return nil, plan, ordinaryFault("ACCOUNTING_UNAVAILABLE", stage, "")
	}
	r := plan.prepared
	if err = ordinaryPreparedCapability(r); err != nil {
		return nil, plan, err
	}
	costs := ordinaryRunRequest(c, plan.admission.config).Costs
	// Keep RunChecked's legacy cost guard and runRaw's normalized fixture setup.
	if r.params.SetupType == string(dsl.FamilyLegacySetup9) {
		if err = r.legacySetup9CostsError(costs); err != nil {
			return nil, plan, ordinaryFault("OPTIONS_UNSUPPORTED", stage, "execution.costs")
		}
	}
	r.fixture.Costs = costs.normalized()
	stage = "execution"
	r.resetBroker(nil)
	capture, err := attachOrdinaryBacktestStateCapture(&r.broker)
	if err != nil {
		return nil, plan, ordinaryFault("INTERNAL", stage, "")
	}
	// No reset, RunChecked, alternate dispatch, partial start or non-liquidating
	// execution may occur after attachment. The supported broker loop runs once.
	r.trades = r.broker.runRangeWithFinalization(0, true)
	financials, states, err := capture.finish()
	if err != nil {
		return nil, plan, ordinaryFault("INTERNAL", stage, "")
	}
	stage = "report"
	native, err := checkedResultEnvelope(r.fixture, r.trades)
	if err != nil {
		return nil, plan, ordinaryFault("NONFINITE_OUTPUT", stage, "")
	}
	copier := &ordinaryBacktestStateCapture{}
	retained, err := copier.copyValue(reflect.ValueOf(native), 0)
	if err != nil {
		return nil, plan, ordinaryFault("NONFINITE_OUTPUT", stage, "")
	}
	result = &OrdinaryBacktestRun{native: retained.Interface().(RunResult), financials: financials, states: states}
	return result, plan, nil
}
