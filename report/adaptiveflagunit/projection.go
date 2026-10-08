package adaptiveflagunit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"time"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

// ownedRaw is deliberately private and has no production constructor in B1.
// A future reviewed builder must construct its run and identities from the same
// single fresh Stage A report. B1 tests construct it only from frozen invented
// wrappers. It is not a user-supplied ledger API or provenance authenticator.
type ownedRaw struct {
	run                                engine.AdaptiveFlagResult
	bytes                              []byte
	dslSHA, compiledConfigSHA, btb1SHA string
}

type arithmetic struct {
	err               error
	row, order, event int
}

func (a *arithmetic) check(v float64, op string) float64 {
	if a.err == nil && (math.IsNaN(v) || math.IsInf(v, 0)) {
		a.err = newCoreError("nonfinite_projection_arithmetic", op, a.row, a.order, a.event, "nonfinite ordered binary64 result")
	}
	return v
}
func (a *arithmetic) add(x, y float64, op string) float64 { return a.check(float64(x+y), op) }
func (a *arithmetic) sub(x, y float64, op string) float64 { return a.check(float64(x-y), op) }
func (a *arithmetic) mul(x, y float64, op string) float64 { return a.check(float64(x*y), op) }
func (a *arithmetic) div(x, y float64, op string) float64 { return a.check(float64(x/y), op) }
func unitPointer[T any](v T) *T                           { return &v }
func copyPointer[T any](v *T) *T {
	if v == nil {
		return nil
	}
	return unitPointer(*v)
}
func copyNullableFloat(v **float64) **float64 {
	if v == nil {
		return nil
	}
	p := copyPointer(*v)
	return &p
}
func copyOrder(v ProjectedOrder) ProjectedOrder {
	v.Quantity = copyPointer(v.Quantity)
	v.PlannedRiskU = copyPointer(v.PlannedRiskU)
	v.RejectionReason = copyPointer(v.RejectionReason)
	v.RawEntry = copyPointer(v.RawEntry)
	v.FillRiskRawU = copyPointer(v.FillRiskRawU)
	v.DisplayProxyAdjustedEntry = copyNullableFloat(v.DisplayProxyAdjustedEntry)
	v.FillRiskEffectiveU = copyNullableFloat(v.FillRiskEffectiveU)
	return v
}
func copyState(v engine.AdaptiveFlagState) engine.AdaptiveFlagState {
	v.PendingOrderID = copyPointer(v.PendingOrderID)
	v.PositionOrderID = copyPointer(v.PositionOrderID)
	v.QueuedExit = copyPointer(v.QueuedExit)
	return v
}
func orderIndex(v *int) int {
	if v == nil {
		return -1
	}
	return *v
}
func indexPointer(i int) *int {
	if i < 0 {
		return nil
	}
	return unitPointer(i)
}
func orderedMax(x, y float64) float64 {
	if y > x {
		return y
	}
	return x
}
func unitIsClose(x, y, delta float64) bool {
	return x == y || math.Abs(delta) <= math.Abs(float64(1e-12*y)) || math.Abs(delta) <= math.Abs(float64(1e-12*x)) || math.Abs(delta) <= 1e-12
}
func rawBytesSHA(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func projectCore(raw ownedRaw, request Request) (*Projection, error) {
	if request.Schema != RequestSchema || request.Scenario != "UNIT_POINT_VALUE_1" || request.NumericalPolicy != "BINARY64_ORDERED_V1" || !validIdentitySourceID(request.DataSource.ID) || !lowercaseHexOfLength(request.DataSource.SourceSHA256, 64) {
		return nil, newCoreError("projection_request_rejected", "projection_request", -1, -1, -1, "invalid closed unit request values")
	}
	policy, err := policyFor(request.CostPolicy)
	if err != nil {
		return nil, err
	}
	run := &raw.run
	ctx, err := validateRun(run)
	if err != nil {
		return nil, err
	}
	rawLimit, err := adaptiveflag.RuntimeOutputBound(uint64(len(run.Snapshots)))
	if err != nil {
		return nil, newCoreError("resource_limit", "output_bound", -1, -1, -1, "invalid raw bound")
	}
	if len(raw.bytes) == 0 {
		return nil, newCoreError("invalid_native_ledger", "identity", -1, -1, -1, "owned raw bytes are required")
	}
	if uint64(len(raw.bytes)) > rawLimit {
		return nil, newCoreError("resource_limit", "identity", -1, -1, -1, "owned raw bytes exceed the bounded profile")
	}
	for _, s := range []string{raw.dslSHA, raw.compiledConfigSHA, raw.btb1SHA} {
		if !lowercaseHexOfLength(s, 64) {
			return nil, newCoreError("invalid_native_ledger", "identity", -1, -1, -1, "missing owned raw file identity")
		}
	}
	limit, err := GeneratedOutputBound(uint64(len(run.Snapshots)), uint64(ctx.Days), uint64(ctx.Midnights), uint64(ctx.Gaps))
	if err != nil || limit > MaxOutputBytes {
		return nil, newCoreError("resource_limit", "output_bound", -1, -1, -1, "unit output exceeds its generated bound")
	}
	p := &Projection{Schema: ProjectionSchema, SourceAuthority: "native Go raw strategy ledger", Orders: make([]ProjectedOrder, 0, len(run.Orders)), InvalidPlannedRiskRejections: []InvalidRiskRejection{}, CostEvents: []CostEvent{}, ClosedTrades: []ClosedTrade{}, Marks: []Mark{}, Daily: []Daily{}, ByEntryYearCohort: map[string]Summary{}, ByCalendarYearMarkedChange: map[string]CalendarYear{}}
	a := &arithmetic{row: -1, order: -1, event: -1}
	for _, order := range run.Orders {
		a.row, a.order, a.event = order.SignalIdx, order.ID, -1
		d := math.Abs(a.sub(order.Trigger, order.Stop, "distance"))
		if a.err != nil {
			return nil, a.err
		}
		var q *float64
		if d > 0 {
			v := float64(1.0 / d)
			if !math.IsInf(v, 0) && !math.IsNaN(v) && v > 0 {
				q = unitPointer(v)
			}
		}
		episode, candidate, e := originIdentity(run, order.Side, order.SignalIdx, request.DataSource.ID)
		if e != nil {
			return nil, e
		}
		item := ProjectedOrder{OrderID: order.ID, EpisodeID: episode, CandidateID: candidate, Side: order.Side, CreationRowIndex: order.SignalIdx, RawTrigger: order.Trigger, FrozenStop: order.Stop, FrozenTarget: order.Target, PlannedDistance: d, Quantity: q}
		if q == nil {
			item.RejectionReason = unitPointer("invalid_planned_risk")
			p.InvalidPlannedRiskRejections = append(p.InvalidPlannedRiskRejections, InvalidRiskRejection{order.ID, "invalid_planned_risk"})
		} else {
			item.PlannedRiskU = unitPointer(1.0)
		}
		p.Orders = append(p.Orders, item)
	}
	n := len(run.Orders)
	created, filled, closed, expired := make([]bool, n), make([]bool, n), make([]bool, n), make([]bool, n)
	entryEvents := make([]engine.AdaptiveFlagEvent, n)
	entryCosts := make([]float64, n)
	active, pending := -1, -1
	var queue *engine.AdaptiveFlagQueuedExit
	grossRealized, paidCost, closedNet, peak, maxDD, closedPeak, closedDD := 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0
	fail := func(message string) error {
		return newCoreError("invalid_native_ledger", "lifecycle", a.row, a.order, a.event, message)
	}
	for i := ctx.Pre; i < len(run.Snapshots); i++ {
		row := run.Snapshots[i]
		events := ctx.EventsByRow[i]
		a.row, a.order, a.event = i, -1, -1
		if queue != nil {
			if active < 0 || queue.NextOpenIdx != i || len(events) == 0 {
				return nil, fail("queued exit missing next observed open")
			}
			first := run.Events[events[0]]
			if first.State != "closed" || first.OrderID != active || eventPhase(first) != "open" || first.Price == nil || *first.Price != row.Open || run.Orders[active].Reason != queue.Reason {
				return nil, fail("queued exit does not match first next-open event")
			}
		}
		for _, ei := range events {
			event := run.Events[ei]
			oid := event.OrderID
			order := run.Orders[oid]
			item := &p.Orders[oid]
			a.order, a.event = oid, event.ID
			sign := 1.0
			if order.Side == "short" {
				sign = -1
			}
			switch event.State {
			case "pending":
				if created[oid] || active >= 0 || pending >= 0 || order.SignalIdx != i || eventPhase(event) != "close" {
					return nil, fail("invalid pending creation")
				}
				created[oid] = true
				pending = oid
			case "expired":
				if pending != oid || filled[oid] {
					return nil, fail("invalid pending expiry")
				}
				expired[oid] = true
				pending = -1
			case "filled":
				if pending != oid || active >= 0 || filled[oid] || i <= order.SignalIdx {
					return nil, fail("invalid fill lifecycle")
				}
				if item.Quantity == nil {
					return nil, newCoreError("invalid_planned_risk", "quantity", i, oid, event.ID, "filled order has invalid planned risk")
				}
				if event.Price == nil || *event.Price <= 0 || order.Entry == nil || *order.Entry != *event.Price || orderIndex(order.FillIdx) != i || order.EntryAtOpen != (eventPhase(event) == "open") {
					return nil, fail("raw entry event mismatch")
				}
				price, q := *event.Price, *item.Quantity
				distance := math.Abs(a.sub(price, order.Stop, "fill_risk_raw"))
				if order.ActualFillToStopDistance == nil || *order.ActualFillToStopDistance != distance {
					return nil, fail("raw entry distance mismatch")
				}
				charge := a.mul(q, policy.PerFill, "entry_charge")
				paidCost = a.add(paidCost, charge, "paid_cost")
				p.CostEvents = append(p.CostEvents, unitCostEvent(a, policy, q, charge, event, "entry"))
				entryCosts[oid], entryEvents[oid] = charge, event
				item.RawEntry = unitPointer(price)
				item.FillRiskRawU = unitPointer(a.mul(distance, q, "fill_risk_raw"))
				var adjusted, effective *float64
				if policy.Name == "RAZOR_PROXY_BASIC" {
					adjusted = unitPointer(a.add(price, a.mul(sign, 0.06, "display_adjustment"), "display_adjustment"))
					effective = unitPointer(a.mul(math.Abs(a.sub(*adjusted, order.Stop, "fill_risk_effective")), q, "fill_risk_effective"))
				}
				item.DisplayProxyAdjustedEntry = &adjusted
				item.FillRiskEffectiveU = &effective
				filled[oid] = true
				active = oid
				pending = -1
			case "closed":
				if active != oid || !filled[oid] || closed[oid] || item.RawEntry == nil || item.Quantity == nil {
					return nil, fail("invalid close lifecycle")
				}
				if event.Price == nil || *event.Price <= 0 || order.Exit == nil || *order.Exit != *event.Price || orderIndex(order.ExitIdx) != i {
					return nil, fail("raw exit event mismatch")
				}
				price, q := *event.Price, *item.Quantity
				gross := a.mul(a.mul(a.sub(price, *item.RawEntry, "gross"), sign, "gross"), q, "gross")
				charge := a.mul(q, policy.PerFill, "exit_charge")
				grossRealized = a.add(grossRealized, gross, "gross_realized")
				paidCost = a.add(paidCost, charge, "paid_cost")
				net := a.sub(a.sub(gross, entryCosts[oid], "net"), charge, "net")
				closedNet = a.add(closedNet, net, "closed_net")
				closedPeak = orderedMax(closedPeak, closedNet)
				closedDD = orderedMax(closedDD, a.sub(closedPeak, closedNet, "closed_drawdown"))
				p.CostEvents = append(p.CostEvents, unitCostEvent(a, policy, q, charge, event, "exit"))
				var adjusted *float64
				if policy.Name == "RAZOR_PROXY_BASIC" {
					adjusted = unitPointer(a.sub(price, a.mul(sign, 0.06, "display_adjustment"), "display_adjustment"))
				}
				entry := entryEvents[oid]
				p.ClosedTrades = append(p.ClosedTrades, ClosedTrade{ProjectedOrder: copyOrder(*item), RawExit: price, Reason: order.Reason, GrossU: gross, EntryModelCostU: entryCosts[oid], ExitModelCostU: charge, NetU: net, NetR: net, DisplayProxyAdjustedExit: adjusted, EntryEventID: entry.ID, ExitEventID: event.ID, EntryEventTime: entry.Time, ExitEventTime: event.Time, EntryPhase: eventPhase(entry), ExitPhase: eventPhase(event), EntryYearCohort: time.UnixMilli(run.Snapshots[entry.RowIdx].OpenT).UTC().Format("2006"), Exposure: unitExposure(entry, event, run.Snapshots, false)})
				closed[oid] = true
				active = -1
				queue = nil
			case "exit-queued":
				candidate := run.States[i].QueuedExit
				if active != oid || candidate == nil || queue != nil || candidate.CreationIdx != i || candidate.CreationCloseMS != row.CloseT || candidate.NextOpenIdx != i+1 || (candidate.Reason != "stop-activated-at-close" && candidate.Reason != "max-hold") {
					return nil, fail("invalid queued exit")
				}
				queue = copyPointer(candidate)
			case "entry-opportunity", "bracket-created":
				if !created[oid] {
					return nil, fail("event before order creation")
				}
			default:
				return nil, fail("unknown native lifecycle event")
			}
			if a.err != nil {
				return nil, a.err
			}
		}
		a.order, a.event = active, -1
		state := run.States[i]
		status := "flat"
		if queue != nil {
			status = "queued-exit"
		} else if active >= 0 {
			status = "open"
		} else if pending >= 0 {
			status = "pending"
		}
		if orderIndex(state.PositionOrderID) != active || orderIndex(state.PendingOrderID) != pending || state.Status != status || !reflect.DeepEqual(state.QueuedExit, queue) {
			return nil, fail("native state and events disagree")
		}
		openGross, openFee := 0.0, 0.0
		if active >= 0 {
			item := p.Orders[active]
			sign := 1.0
			if item.Side == "short" {
				sign = -1
			}
			openGross = a.mul(a.mul(a.sub(row.Close, *item.RawEntry, "open_gross"), sign, "open_gross"), *item.Quantity, "open_gross")
			openFee = entryCosts[active]
		}
		marked := a.add(a.sub(grossRealized, paidCost, "marked"), openGross, "marked")
		alternate := a.sub(a.add(closedNet, openGross, "alternate"), openFee, "alternate")
		delta := a.sub(marked, alternate, "reconciliation_delta")
		if a.err != nil {
			return nil, a.err
		}
		if !unitIsClose(marked, alternate, delta) {
			return nil, newCoreError("ledger_reconciliation_failed", "reconciliation_delta", i, active, -1, "independent ledger paths do not reconcile")
		}
		peak = orderedMax(peak, marked)
		dd := a.sub(peak, marked, "close_drawdown")
		maxDD = orderedMax(maxDD, dd)
		if a.err != nil {
			return nil, a.err
		}
		p.Marks = append(p.Marks, Mark{RowIndex: i, BarOpenMS: row.OpenT, NominalCloseMS: row.CloseT, RawClose: row.Close, GrossRealizedClosedU: grossRealized, ModeledExecutionCostsPaidU: paidCost, OpenRawGrossU: openGross, OpenEntryModelCostU: openFee, ClosedTradeNetU: closedNet, MarkedPnLU: marked, MarkedPnLR: marked, ReconciliationDeltaU: delta, CloseMarkedDrawdownR: dd, OpenOrderID: indexPointer(active), PendingOrderID: indexPointer(pending), QueuedExit: copyPointer(state.QueuedExit)})
	}
	for oid, order := range run.Orders {
		a.row, a.order, a.event = order.SignalIdx, oid, -1
		if !created[oid] {
			return nil, fail("native order has no creation event")
		}
		status := ""
		if closed[oid] {
			status = "closed"
		} else if expired[oid] {
			status = "expired"
		} else if oid == active && queue != nil {
			status = "queued-exit"
		} else if oid == active {
			status = "open"
		} else if oid == pending {
			status = "pending"
		}
		if order.Status != status || (order.FillIdx != nil) != filled[oid] || (order.ExitIdx != nil) != closed[oid] {
			return nil, fail("native order final status mismatch")
		}
	}
	a.row, a.order, a.event = -1, -1, -1
	p.CloseMarkedMaxDrawdownR, p.ClosedTradeMaxDrawdownR = maxDD, closedDD
	p.Summary = unitTradeSummary(a, p.ClosedTrades)
	cohorts := map[string][]ClosedTrade{}
	for _, trade := range p.ClosedTrades {
		cohorts[trade.EntryYearCohort] = append(cohorts[trade.EntryYearCohort], trade)
	}
	years := make([]string, 0, len(cohorts))
	for year := range cohorts {
		years = append(years, year)
	}
	sort.Strings(years)
	for _, year := range years {
		p.ByEntryYearCohort[year] = unitTradeSummary(a, cohorts[year])
	}
	pointer, lastMark := 0, -1
	priorLevel := 0.0
	for day := ctx.Start / unitDayMS * unitDayMS; day < ctx.End; day += unitDayMS {
		asOf := day + unitDayMS
		if asOf > ctx.End {
			asOf = ctx.End
		}
		hadQuote := false
		for pointer < len(p.Marks) && p.Marks[pointer].NominalCloseMS <= asOf {
			lastMark = pointer
			pointer++
			hadQuote = true
		}
		level := 0.0
		var lastClose *int64
		gapFrom := ctx.Start
		if lastMark >= 0 {
			level = p.Marks[lastMark].MarkedPnLR
			lastClose = unitPointer(p.Marks[lastMark].NominalCloseMS)
			gapFrom = *lastClose
		}
		change := a.sub(level, priorLevel, "daily_change")
		date := time.UnixMilli(day).UTC().Format("2006-01-02")
		p.Daily = append(p.Daily, Daily{date, asOf, level, change, !hadQuote, lastClose, asOf - gapFrom})
		priorLevel = level
		year := date[:4]
		entry := p.ByCalendarYearMarkedChange[year]
		entry.CalendarYearMarkedChangeR = a.add(entry.CalendarYearMarkedChangeR, change, "calendar_year_sum")
		p.ByCalendarYearMarkedChange[year] = entry
	}
	level := 0.0
	if len(p.Marks) > 0 {
		level = p.Marks[len(p.Marks)-1].MarkedPnLR
	}
	remaining := 0.0
	var item *ProjectedOrder
	var exposure *Exposure
	if active >= 0 {
		v := copyOrder(p.Orders[active])
		item = &v
		remaining = a.mul(*v.Quantity, policy.PerFill, "hypothetical_exit_cost")
		last := run.Snapshots[len(run.Snapshots)-1]
		endpoint := engine.AdaptiveFlagEvent{RowIdx: last.Index, Time: engine.AdaptiveFlagEventTime{LowerMS: last.CloseT, UpperMS: last.CloseT, LowerInclusive: true, UpperInclusive: true, Kind: "nominal-close-point"}}
		exp := unitExposure(entryEvents[active], endpoint, run.Snapshots, true)
		exposure = &exp
	}
	p.Terminal = Terminal{copyState(run.Terminal), level, a.sub(level, remaining, "hypothetical_liquidation"), remaining, item, exposure, len(p.ClosedTrades)}
	dailySum := 0.0
	for _, d := range p.Daily {
		dailySum = a.add(dailySum, d.DailyMarkedChangeR, "daily_reconciliation")
	}
	dailyDelta := a.sub(dailySum, level, "daily_reconciliation")
	if a.err != nil {
		return nil, a.err
	}
	if !unitIsClose(dailySum, level, dailyDelta) {
		return nil, newCoreError("daily_reconciliation_failed", "daily_reconciliation", -1, -1, -1, "daily changes and terminal mark do not reconcile")
	}
	p.Manifest, err = unitManifest(raw, request, policy, ctx)
	if err != nil {
		return nil, err
	}
	if err := validateProjectedCardinalities(p, ctx, len(run.Snapshots), len(run.Orders)); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return nil, newCoreError("internal_error", "serialize", -1, -1, -1, "unit projection serialization failed")
	}
	if uint64(len(encoded)+len(raw.bytes)+1) > limit {
		return nil, newCoreError("resource_limit", "output_bound", -1, -1, -1, "unit output violated generated bound")
	}
	return p, nil
}

func unitCostEvent(a *arithmetic, policy CostPolicy, q, charge float64, event engine.AdaptiveFlagEvent, leg string) CostEvent {
	product := func(v *float64) *float64 {
		if v == nil {
			return nil
		}
		return unitPointer(a.mul(q, *v, "cost_component"))
	}
	return CostEvent{event.OrderID, event.ID, leg, event.RowIdx, event.Time, charge, product(policy.Spread), product(policy.Commission), product(policy.Aggregate)}
}

func unitTradeSummary(a *arithmetic, trades []ClosedTrade) Summary {
	s := Summary{ClosedTrades: len(trades), ProfitFactorStatus: "undefined_no_nonzero_trades"}
	for _, t := range trades {
		if t.NetR > 0 {
			s.PositiveNetR = a.add(s.PositiveNetR, t.NetR, "summary_positive")
			s.Wins++
		} else if t.NetR < 0 {
			s.Losses++
		} else {
			s.ZeroNetTrades++
		}
	}
	for _, t := range trades {
		if t.NetR < 0 {
			s.AbsoluteNegativeNetR = a.add(s.AbsoluteNegativeNetR, -t.NetR, "summary_negative")
		}
	}
	for _, t := range trades {
		s.NetR = a.add(s.NetR, t.NetR, "summary_net")
	}
	if len(trades) > 0 {
		s.MeanNetR = unitPointer(a.div(s.NetR, float64(len(trades)), "summary_mean"))
	}
	if s.AbsoluteNegativeNetR > 0 {
		s.ProfitFactor = unitPointer(a.div(s.PositiveNetR, s.AbsoluteNegativeNetR, "summary_pf"))
		s.ProfitFactorStatus = "finite"
	} else if s.PositiveNetR > 0 {
		s.ProfitFactorStatus = "infinite_no_losses"
	}
	return s
}

func unitExposure(entry, endpoint engine.AdaptiveFlagEvent, rows []engine.AdaptiveFlagSnapshot, open bool) Exposure {
	a, z := entry.Time, endpoint.Time
	low, high := z.LowerMS-a.UpperMS, z.UpperMS-a.LowerMS
	if low < 0 {
		low = 0
	}
	if high < 0 {
		high = 0
	}
	x := Exposure{EntryTimeBounds: a, EndpointTimeBounds: z, EntryBarOpenMS: rows[entry.RowIdx].OpenT, EndpointBarOpenMS: rows[endpoint.RowIdx].OpenT, ElapsedMSLower: low, ElapsedMSUpper: high, ObservedBarsWithExposure: endpoint.RowIdx - entry.RowIdx + 1, QuoteGapsBridged: []ExposureGap{}, UTCMidnightsDefinitelyCrossed: []int64{}, UTCMidnightsPossiblyCrossed: []int64{}, RolloverLabel: "UTC exposure proxies, not broker charge counts", IsOpenPosition: open}
	if !open && eventPhase(endpoint) == "open" {
		x.ObservedBarsWithExposure--
	}
	for boundary := (a.LowerMS/unitDayMS + 1) * unitDayMS; boundary <= z.UpperMS; boundary += unitDayMS {
		if a.LowerMS < boundary && (z.UpperMS > boundary || z.UpperInclusive) {
			x.UTCMidnightsPossiblyCrossed = append(x.UTCMidnightsPossiblyCrossed, boundary)
		}
		if (a.UpperMS < boundary || (a.UpperMS == boundary && !a.UpperInclusive)) && z.LowerMS >= boundary {
			x.UTCMidnightsDefinitelyCrossed = append(x.UTCMidnightsDefinitelyCrossed, boundary)
		}
	}
	for i := entry.RowIdx + 1; i <= endpoint.RowIdx; i++ {
		if rows[i].OpenT > rows[i-1].CloseT {
			x.QuoteGapsBridged = append(x.QuoteGapsBridged, ExposureGap{rows[i-1].CloseT, rows[i].OpenT, rows[i].OpenT - rows[i-1].CloseT})
		}
	}
	return x
}

func unitManifest(raw ownedRaw, request Request, policy CostPolicy, ctx validationContext) (Manifest, error) {
	build := buildIdentity()
	if err := validateBuildIdentity(build); err != nil {
		return Manifest{}, err
	}
	economicHash, err := economicPolicySHA256(policy)
	if err != nil {
		return Manifest{}, err
	}
	numericHash, err := numericalPolicySHA256()
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{ContractID: ProjectionSchema, ReferenceSourceSHA256: ReferenceSourceSHA256, ReferenceCommonSHA256: ReferenceCommonSHA256, Scenario: "UNIT_POINT_VALUE_1", Units: "U research PnL and R; 1U planned risk; not account dollars", PointValue: 1, PlannedRiskU: 1, StartingCumulativePnLU: 0, NumericalPolicy: "BINARY64_ORDERED_V1", CostPolicy: policy, EconomicPolicy: economicPolicy(policy), GoEconomicPolicySHA256: economicHash, NumericalDefinition: numericalDefinition(), GoNumericalPolicySHA256: numericHash, FundingStatus: "unknown_unmodeled", FundingCashU: nil, EconomicsQualification: "after modeled execution costs, before unmodeled funding; basic also before additional unmodeled slippage", RawEnvelopeSHA256: rawBytesSHA(raw.bytes), RawInputSHA256: raw.run.InputSHA256, RawConfigSHA256: raw.run.ConfigSHA256, SourcePineSHA256: raw.run.SourcePineSHA256, DSLSHA256: raw.dslSHA, CompiledConfigSHA256: raw.compiledConfigSHA, BTB1SHA256: raw.btb1SHA, EffectiveConfig: raw.run.EffectiveConfig, ExecutionSemantics: raw.run.ExecutionSemantics, ExecutionWindow: EffectiveWindow{ctx.Start, ctx.End, "bar-open timestamp in [start,end); nominal close may equal end"}, WarmupRowsExcluded: ctx.Pre, DataSource: request.DataSource, DataSourceStatus: "declared_only", BuildIdentity: build, RegistryEnabled: false, CostsNeverModifyRawPhysics: true, LiquidationIsHypothetical: true}, nil
}

// Check the actual repeated collections against the count pass before marshaling.
// This guards future drift between exposure counting and construction as well as
// the global caps; unused allowance in another term cannot hide an oversized list.
func validateProjectedCardinalities(p *Projection, ctx validationContext, n, orders int) error {
	bad := func() error {
		return newCoreError("resource_limit", "output_bound", -1, -1, -1, "projection collections disagree with their admitted counts")
	}
	if len(p.Orders) != orders || len(p.Orders) > n || len(p.InvalidPlannedRiskRejections) > orders || len(p.CostEvents) > 2*orders || len(p.ClosedTrades) > orders || len(p.Marks) != n-ctx.Pre || len(p.Daily) != ctx.Days || len(p.ByEntryYearCohort) > ctx.Days || len(p.ByCalendarYearMarkedChange) > ctx.Days {
		return bad()
	}
	e, g := 0, 0
	count := func(x *Exposure) {
		if x != nil {
			e += len(x.UTCMidnightsDefinitelyCrossed) + len(x.UTCMidnightsPossiblyCrossed)
			g += len(x.QuoteGapsBridged)
		}
	}
	for i := range p.ClosedTrades {
		count(&p.ClosedTrades[i].Exposure)
	}
	count(p.Terminal.Exposure)
	if e != ctx.Midnights || g != ctx.Gaps || e > MaxExposureMidnights || g > MaxExposureGaps {
		return bad()
	}
	return nil
}
