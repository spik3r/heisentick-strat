package adaptiveflagunit

import (
	"bytes"
	"encoding/json"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/engine"
)

func handOwned(t *testing.T, name string) ownedRaw {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testsupport", "testdata", "adaptive-flag-unit", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var run engine.AdaptiveFlagResult
	if err = json.Unmarshal(body, &run); err != nil {
		t.Fatal(err)
	}
	result := ownedRaw{run: run, dslSHA: strings.Repeat("5", 64), compiledConfigSHA: strings.Repeat("6", 64), btb1SHA: strings.Repeat("7", 64)}
	result.bytes = append([]byte(`{"run":`), body...)
	result.bytes = append(result.bytes, []byte(`,"dslSha256":"`+result.dslSHA+`","configSha256":"`+result.compiledConfigSHA+`","btb1Sha256":"`+result.btb1SHA+`"}`+"\n")...)
	return result
}
func rebindHand(t *testing.T, raw ownedRaw) ownedRaw {
	t.Helper()
	body, err := json.Marshal(raw.run)
	if err != nil {
		t.Fatal(err)
	}
	raw.bytes = append([]byte(`{"run":`), body...)
	raw.bytes = append(raw.bytes, []byte(`,"dslSha256":"`+raw.dslSHA+`","configSha256":"`+raw.compiledConfigSHA+`","btb1Sha256":"`+raw.btb1SHA+`"}`+"\n")...)
	return raw
}
func unitRequest(policy string) Request {
	return Request{RequestSchema, "UNIT_POINT_VALUE_1", "BINARY64_ORDERED_V1", policy, DataSource{"HAND_DERIVED_ADAPTER_FIXTURE_V1", strings.Repeat("4", 64)}}
}
func assertBits(t *testing.T, got, want float64) {
	t.Helper()
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("got%016x want%016x (%g,%g)", math.Float64bits(got), math.Float64bits(want), got, want)
	}
}
func coreCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected refusal")
	}
	var out ErrorEnvelope
	if e := json.Unmarshal([]byte(errorJSON(err)), &out); e != nil {
		t.Fatal(e)
	}
	return out.Error.Code
}

// Independently rounded53-bit arithmetic supplies hand numeric expectations;
// this is not the retained projector, a strategy model or an economic engine.
func handOp(x, y float64, op byte) float64 {
	a := new(big.Float).SetPrec(53).SetMode(big.ToNearestEven).SetFloat64(x)
	b := new(big.Float).SetPrec(53).SetMode(big.ToNearestEven).SetFloat64(y)
	switch op {
	case '+':
		a.Add(a, b)
	case '-':
		a.Sub(a, b)
	case '*':
		a.Mul(a, b)
	case '/':
		a.Quo(a, b)
	}
	v, _ := a.Float64()
	return v
}
func TestProjectionHandBothSidesAllFixedCosts(t *testing.T) {
	for _, side := range []string{"long", "short"} {
		for _, policy := range []struct {
			name string
			cost float64
		}{{"RAW", 0}, {"RAZOR_PROXY_BASIC", .095}, {"RAZOR_PROXY_HARSH_AGGREGATE", .155}} {
			t.Run(side+"/"+policy.name, func(t *testing.T) {
				raw := handOwned(t, "hand-"+side+"-closed")
				before, _ := json.Marshal(raw.run)
				saved := append([]byte(nil), raw.bytes...)
				p, err := projectCore(raw, unitRequest(policy.name))
				if err != nil {
					t.Fatal(err)
				}
				q := handOp(1, 10, '/')
				charge := handOp(q, policy.cost, '*')
				gross := handOp(12, q, '*')
				net := handOp(handOp(gross, charge, '-'), charge, '-')
				assertBits(t, *p.Orders[0].Quantity, q)
				assertBits(t, *p.Orders[0].FillRiskRawU, handOp(13, q, '*'))
				assertBits(t, p.ClosedTrades[0].GrossU, gross)
				assertBits(t, p.ClosedTrades[0].NetR, net)
				if policy.name == "RAZOR_PROXY_BASIC" {
					assertBits(t, net, 0x1.2e5604189374cp+0)
				}
				if len(p.CostEvents) != 2 || len(p.ClosedTrades) != 1 || p.Summary.Wins != 1 {
					t.Fatal("actual fill accounting counts")
				}
				assertBits(t, p.CostEvents[0].TotalU, charge)
				assertBits(t, p.CostEvents[1].TotalU, charge)
				assertBits(t, p.Terminal.MarkedPnLR, p.Marks[2].MarkedPnLR)
				assertBits(t, p.Terminal.HypotheticalLiquidationPnLR, p.Terminal.MarkedPnLR)
				if p.Manifest.FundingStatus != "unknown_unmodeled" || p.Manifest.FundingCashU != nil || p.Manifest.RawEnvelopeSHA256 != rawBytesSHA(saved) {
					t.Fatal("funding/raw identity contract")
				}
				if policy.name == "RAZOR_PROXY_HARSH_AGGREGATE" && (p.CostEvents[0].SpreadProxyU != nil || p.CostEvents[0].CommissionProxyU != nil || p.Orders[0].DisplayProxyAdjustedEntry == nil || *p.Orders[0].DisplayProxyAdjustedEntry != nil) {
					t.Fatal("unknown harsh decomposition")
				}
				after, _ := json.Marshal(raw.run)
				if !bytes.Equal(before, after) || !bytes.Equal(raw.bytes, saved) {
					t.Fatal("raw input mutated")
				}
			})
		}
	}
}

func TestProjectionOpenPendingAndInvalidRisk(t *testing.T) {
	for _, policy := range []string{"RAW", "RAZOR_PROXY_BASIC", "RAZOR_PROXY_HARSH_AGGREGATE"} {
		opened := handOwned(t, "hand-long-open")
		p, err := projectCore(opened, unitRequest(policy))
		if err != nil {
			t.Fatal(err)
		}
		if len(p.CostEvents) != 1 || p.Summary.ClosedTrades != 0 || p.Terminal.OpenPosition == nil {
			t.Fatal("open position counted as closed")
		}
		expected := handOp(p.Terminal.MarkedPnLR, p.CostEvents[0].TotalU, '-')
		assertBits(t, p.Terminal.HypotheticalLiquidationPnLR, expected)
		p.Terminal.NativeState.PositionOrderID = unitPointer(99)
		p.Terminal.OpenPosition.RawEntry = unitPointer(99.0)
		if *opened.run.Terminal.PositionOrderID != 0 || *opened.run.Orders[0].Entry != 103 {
			t.Fatal("result aliases raw state")
		}
		for _, name := range []string{"hand-long-pending", "hand-invalid-risk-pending"} {
			pending := handOwned(t, name)
			p, err = projectCore(pending, unitRequest(policy))
			if err != nil {
				t.Fatal(err)
			}
			if len(p.CostEvents) != 0 || p.Terminal.OpenPosition != nil || p.Terminal.MarkedPnLR != 0 {
				t.Fatal("pending charged")
			}
			if name == "hand-invalid-risk-pending" && (p.Orders[0].Quantity != nil || len(p.InvalidPlannedRiskRejections) != 1) {
				t.Fatal("invalid risk not preserved")
			}
		}
		if p, err := projectCore(handOwned(t, "hand-invalid-risk-filled"), unitRequest(policy)); p != nil || coreCode(t, err) != "invalid_planned_risk" {
			t.Fatal("invalid fill must refuse entire projection")
		}
	}
}

func TestProjectionGapLossBeyondOneR(t *testing.T) {
	raw := handOwned(t, "hand-long-closed")
	raw.run.Orders[0].Exit = unitPointer(87.0)
	raw.run.Events[3].Price = unitPointer(87.0)
	r := &raw.run.Snapshots[2]
	r.Open, r.High, r.Low, r.Close = 87, 88, 86, 87
	raw = rebindHand(t, raw)
	p, err := projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	q := handOp(1, 10, '/')
	charge := handOp(q, .095, '*')
	want := handOp(handOp(handOp(-16, q, '*'), charge, '-'), charge, '-')
	assertBits(t, p.ClosedTrades[0].NetR, want)
	if p.ClosedTrades[0].NetR >= -1 || p.Summary.Losses != 1 || *p.Orders[0].Quantity != q {
		t.Fatal("gap resized/capped fixed risk")
	}
}

func queuedHand(t *testing.T, closed bool) ownedRaw {
	raw := handOwned(t, "hand-long-open")
	r := &raw.run.Snapshots[1]
	r.Low = 84
	r.Close = 85
	queue := &engine.AdaptiveFlagQueuedExit{Reason: "stop-activated-at-close", CreationIdx: 1, CreationCloseMS: r.CloseT, NextOpenIdx: 2}
	raw.run.States[1].Status = "queued-exit"
	raw.run.States[1].QueuedExit = queue
	raw.run.Terminal = copyState(raw.run.States[1])
	raw.run.Orders[0].Status = "queued-exit"
	raw.run.Events = append(raw.run.Events, engine.AdaptiveFlagEvent{ID: 3, OrderID: 0, RowIdx: 1, State: "exit-queued", Time: engine.AdaptiveFlagEventTime{LowerMS: r.CloseT, UpperMS: r.CloseT, LowerInclusive: true, UpperInclusive: true, Kind: "nominal-close-point"}})
	raw.run.Orders[0].EventIDs = append(raw.run.Orders[0].EventIDs, 3)
	if closed {
		last := raw.run.Snapshots[1]
		last.Index = 2
		last.OpenT = last.CloseT
		last.CloseT += raw.run.TimeframeMS
		last.Open, last.High, last.Low, last.Close = 100, 101, 99, 100
		raw.run.Snapshots = append(raw.run.Snapshots, last)
		state := engine.AdaptiveFlagState{RowIdx: 2, CloseMS: last.CloseT, Status: "flat"}
		raw.run.States = append(raw.run.States, state)
		raw.run.Terminal = state
		raw.run.Events = append(raw.run.Events, engine.AdaptiveFlagEvent{ID: 4, OrderID: 0, RowIdx: 2, State: "closed", Price: unitPointer(100.0), Time: engine.AdaptiveFlagEventTime{LowerMS: last.OpenT, UpperMS: last.OpenT, LowerInclusive: true, UpperInclusive: true, Kind: "observed-open-point"}})
		order := &raw.run.Orders[0]
		order.Status = "closed"
		order.ExitIdx = unitPointer(2)
		order.Exit = unitPointer(100.0)
		order.Reason = "stop-activated-at-close"
		order.EventIDs = append(order.EventIDs, 4)
		raw.run.EligibleTradeRows = 3
		raw.run.ExecutionWindow.TradeToMS = last.CloseT
	}
	return rebindHand(t, raw)
}
func TestProjectionQueuedTerminalAndNextOpenPriority(t *testing.T) {
	raw := queuedHand(t, false)
	p, err := projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.CostEvents) != 1 || len(p.ClosedTrades) != 0 || p.Terminal.NativeState.Status != "queued-exit" || p.Terminal.Exposure == nil {
		t.Fatal("terminal queue liquidated")
	}
	if !reflect.DeepEqual(p.Terminal.NativeState, raw.run.Terminal) {
		t.Fatal("native terminal changed")
	}
	*p.Terminal.NativeState.PositionOrderID = 99
	p.Terminal.NativeState.QueuedExit.Reason = "mutated"
	if *raw.run.Terminal.PositionOrderID != 0 || raw.run.Terminal.QueuedExit.Reason != "stop-activated-at-close" {
		t.Fatal("terminal pointer aliases raw")
	}
	raw = queuedHand(t, true)
	p, err = projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.ClosedTrades) != 1 || p.ClosedTrades[0].RawExit != 100 || p.ClosedTrades[0].ExitPhase != "open" {
		t.Fatal("queued exit recovery changed")
	}
	raw.run.Events[4].Price = unitPointer(100.5)
	raw.run.Orders[0].Exit = unitPointer(100.5)
	raw = rebindHand(t, raw)
	if p, err = projectCore(raw, unitRequest("RAW")); p != nil || coreCode(t, err) != "invalid_native_ledger" {
		t.Fatal("queued wrong-open accepted")
	}
}

func TestProjectionTinyQuantityAndNonfiniteRefusal(t *testing.T) {
	for _, v := range []struct {
		d     float64
		valid bool
	}{{0, false}, {0x1p-1023, true}, {0x1p-1024, false}, {math.SmallestNonzeroFloat64, false}} {
		raw := handOwned(t, "hand-long-pending")
		raw.run.Orders[0].Trigger = v.d
		raw.run.Orders[0].Stop = 0
		raw.run.Orders[0].PlannedTriggerToStopDistance = v.d
		raw = rebindHand(t, raw)
		p, err := projectCore(raw, unitRequest("RAW"))
		if err != nil {
			t.Fatal(err)
		}
		if (p.Orders[0].Quantity != nil) != v.valid {
			t.Fatal("tiny distance epsilon/reciprocal contract")
		}
	}
	raw := handOwned(t, "hand-long-open")
	order := &raw.run.Orders[0]
	order.Trigger = 0x1p-1023
	order.Stop = 0
	order.PlannedTriggerToStopDistance = 0x1p-1023
	order.ActualFillToStopDistance = unitPointer(103.0)
	raw = rebindHand(t, raw)
	if p, err := projectCore(raw, unitRequest("RAW")); p != nil || coreCode(t, err) != "nonfinite_projection_arithmetic" {
		t.Fatal("overflowing fill risk must refuse")
	}
	a := &arithmetic{row: -1, order: -1, event: -1}
	s := unitTradeSummary(a, []ClosedTrade{{NetR: 1}, {NetR: -math.SmallestNonzeroFloat64}})
	if a.err == nil || coreCode(t, a.err) != "nonfinite_projection_arithmetic" || s.ProfitFactorStatus != "finite" {
		t.Fatal("PF overflow disguised as no losses")
	}
}

func TestProjectionSummaryDefinitionsAndSignedZero(t *testing.T) {
	for _, v := range []struct {
		trades              []ClosedTrade
		status              string
		wins, losses, zeros int
	}{{nil, "undefined_no_nonzero_trades", 0, 0, 0}, {[]ClosedTrade{{NetR: 0}, {NetR: math.Copysign(0, -1)}}, "undefined_no_nonzero_trades", 0, 0, 2}, {[]ClosedTrade{{NetR: 1}}, "infinite_no_losses", 1, 0, 0}, {[]ClosedTrade{{NetR: -1}}, "finite", 0, 1, 0}} {
		a := &arithmetic{}
		s := unitTradeSummary(a, v.trades)
		if a.err != nil || s.ProfitFactorStatus != v.status || s.Wins != v.wins || s.Losses != v.losses || s.ZeroNetTrades != v.zeros {
			t.Fatal("summary definition")
		}
	}
	a := &arithmetic{}
	s := unitTradeSummary(a, []ClosedTrade{{NetR: 1e16}, {NetR: 1}, {NetR: -1e16}})
	assertBits(t, s.NetR, 0)
	if a.err != nil {
		t.Fatal(a.err)
	}
	raw := handOwned(t, "hand-long-closed")
	raw.run.Orders[0].Exit = unitPointer(103.0)
	raw.run.Events[3].Price = unitPointer(103.0)
	r := &raw.run.Snapshots[2]
	r.Open, r.High, r.Low, r.Close = 103, 104, 102, 103
	raw = rebindHand(t, raw)
	p, err := projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Summary.Losses != 1 {
		t.Fatal("raw zero with costs must lose")
	}
}

func shiftHandTimes(raw *ownedRaw, firstRow int, delta int64) {
	for i := firstRow; i < len(raw.run.Snapshots); i++ {
		raw.run.Snapshots[i].OpenT += delta
		raw.run.Snapshots[i].CloseT += delta
		raw.run.States[i].CloseMS += delta
	}
	for i := range raw.run.Events {
		if raw.run.Events[i].RowIdx >= firstRow {
			raw.run.Events[i].Time.LowerMS += delta
			raw.run.Events[i].Time.UpperMS += delta
		}
	}
	if firstRow == 0 {
		raw.run.ExecutionWindow.TradeFromMS += delta
	}
	raw.run.ExecutionWindow.TradeToMS += delta
	raw.run.Terminal = copyState(raw.run.States[len(raw.run.States)-1])
}
func TestProjectionDailyGapsYearsAndUncertainExposure(t *testing.T) {
	raw := handOwned(t, "hand-long-closed")
	shiftHandTimes(&raw, 2, 3*unitDayMS)
	raw = rebindHand(t, raw)
	p, err := projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Daily) != 4 || len(p.ClosedTrades[0].Exposure.QuoteGapsBridged) != 1 || p.ClosedTrades[0].Exposure.QuoteGapsBridged[0].DurationMS != 3*unitDayMS {
		t.Fatal("observed gap/daily horizon")
	}
	for _, d := range p.Daily[1:3] {
		if !d.CarriedNoQuote || d.DailyMarkedChangeR != 0 {
			t.Fatal("missing-quote day used a future price")
		}
		assertBits(t, d.MarkedPnLR, p.Marks[1].MarkedPnLR)
	}
	assertBits(t, p.ClosedTrades[0].NetR, 0x1.2e5604189374cp+0)
	assertBits(t, p.Marks[2].MarkedPnLR, 0x1.2e5604189374dp+0)
	assertBits(t, p.Marks[2].ReconciliationDeltaU, 0x1p-52)
	// Entry's observed bar starts in2025; the actual exit is at2026 midnight.
	raw = handOwned(t, "hand-long-closed")
	shiftHandTimes(&raw, 0, -3600000)
	raw.run.Events[1].Time = engine.AdaptiveFlagEventTime{LowerMS: raw.run.Snapshots[1].OpenT, UpperMS: raw.run.Snapshots[1].CloseT, Kind: "nonopening-crossing-unknown-model-time"}
	raw.run.Orders[0].EntryAtOpen = false
	raw = rebindHand(t, raw)
	p, err = projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.ByEntryYearCohort) != 1 || p.ByEntryYearCohort["2025"].ClosedTrades != 1 || len(p.ByCalendarYearMarkedChange) != 2 {
		t.Fatal("entry cohort confused with calendar attribution")
	}
	ex := p.ClosedTrades[0].Exposure
	if ex.ElapsedMSLower != 0 || ex.ElapsedMSUpper != 1800000 || len(ex.UTCMidnightsDefinitelyCrossed) != 1 || len(ex.UTCMidnightsPossiblyCrossed) != 1 || ex.ObservedBarsWithExposure != 1 {
		t.Fatal("intrabar/midnight exposure bounds")
	}
	if p.Daily[0].DateUTC != "2025-12-31" || p.Daily[1].DateUTC != "2026-01-01" {
		t.Fatal("UTC calendar day labels")
	}
	if p.ByCalendarYearMarkedChange["2025"].CalendarYearMarkedChangeR == p.ByEntryYearCohort["2025"].NetR {
		t.Fatal("different year attribution collapsed")
	}
}

func TestProjectionAllWarmupAndTrailingCarry(t *testing.T) {
	raw := handOwned(t, "hand-long-closed")
	raw.run.Orders = []engine.AdaptiveFlagOrder{}
	raw.run.Events = []engine.AdaptiveFlagEvent{}
	for i := range raw.run.States {
		raw.run.States[i].Status = "flat"
		raw.run.States[i].PendingOrderID = nil
		raw.run.States[i].PositionOrderID = nil
		raw.run.States[i].QueuedExit = nil
	}
	raw.run.Terminal = copyState(raw.run.States[len(raw.run.States)-1])
	raw.run.ExecutionWindow.TradeFromMS = raw.run.Terminal.CloseMS
	raw.run.ExecutionWindow.TradeToMS = raw.run.Terminal.CloseMS + unitDayMS
	raw.run.PreTradeRows = 3
	raw.run.EligibleTradeRows = 0
	raw = rebindHand(t, raw)
	p, err := projectCore(raw, unitRequest("RAW"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Marks) != 0 || len(p.Daily) != 2 || p.Manifest.WarmupRowsExcluded != 3 || p.Terminal.MarkedPnLR != 0 || p.Summary.MeanNetR != nil {
		t.Fatal("all-warmup accounting")
	}
	for _, d := range p.Daily {
		if !d.CarriedNoQuote || d.LastQuoteCloseMS != nil || d.MarkedPnLR != 0 {
			t.Fatal("invented warmup mark")
		}
	}
	raw = handOwned(t, "hand-long-open")
	lastClose := raw.run.Terminal.CloseMS
	raw.run.ExecutionWindow.TradeToMS += 3 * unitDayMS
	raw = rebindHand(t, raw)
	p, err = projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Daily) != 4 || p.Terminal.Exposure.EndpointTimeBounds.UpperMS != lastClose || p.Daily[3].QuoteGapDurationMS != 3*unitDayMS {
		t.Fatal("terminal exposure extended to unobserved horizon")
	}
	if len(p.CostEvents) != 1 || len(p.ClosedTrades) != 0 {
		t.Fatal("trailing horizon charged/closed position")
	}
}

func TestProjectionPoliciesIsolatedAndOutputDetached(t *testing.T) {
	raw := handOwned(t, "hand-long-open")
	before, _ := json.Marshal(raw.run)
	first, err := projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	for _, name := range []string{"RAW", "RAZOR_PROXY_HARSH_AGGREGATE", "RAZOR_PROXY_BASIC"} {
		if _, err := projectCore(raw, unitRequest(name)); err != nil {
			t.Fatal(err)
		}
	}
	second, err := projectCore(raw, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(second)
	if !bytes.Equal(a, b) {
		t.Fatal("policy calls contaminate later runs")
	}
	*second.Terminal.NativeState.PositionOrderID = 99
	*second.Orders[0].RawEntry = 0
	*second.Terminal.OpenPosition.RawEntry = 1
	after, _ := json.Marshal(raw.run)
	if !bytes.Equal(before, after) {
		t.Fatal("output mutation reached raw inputs")
	}
	for _, request := range []Request{unitRequest("CUSTOM_COST"), {Schema: RequestSchema, Scenario: "ACCOUNT", NumericalPolicy: "BINARY64_ORDERED_V1", CostPolicy: "RAW", DataSource: unitRequest("RAW").DataSource}} {
		if p, err := projectCore(raw, request); p != nil || coreCode(t, err) != "projection_request_rejected" {
			t.Fatal("unsupported economics admitted")
		}
	}
}
