package adaptiveflagunit

import (
	"math"
	"reflect"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

const unitDayMS int64 = 86400000

// validationContext contains only bounded indexing and count information. The
// economic projection is allocated only after the exposure count pass succeeds.
type validationContext struct {
	Start, End      int64
	Pre, Days       int
	Midnights, Gaps int
	EventsByRow     [][]int
}

func eventPhase(event engine.AdaptiveFlagEvent) string {
	switch event.Time.Kind {
	case "observed-open-point":
		return "open"
	case "nominal-close-point":
		return "close"
	case "nonopening-crossing-unknown-model-time":
		return "intrabar"
	case "observed-bar-opportunity":
		return "bar-interval"
	default:
		return "unknown"
	}
}

func validateRun(run *engine.AdaptiveFlagResult) (validationContext, error) {
	var ctx validationContext
	bad := func(op string, row, order, event int, message string) (validationContext, error) {
		return validationContext{}, newCoreError("invalid_native_ledger", op, row, order, event, message)
	}
	resource := func(op, message string) (validationContext, error) {
		return validationContext{}, newCoreError("resource_limit", op, -1, -1, -1, message)
	}
	domain := func(op string, row int, message string) (validationContext, error) {
		return validationContext{}, newCoreError("projection_domain_rejected", op, row, -1, -1, message)
	}
	if run == nil {
		return bad("raw_decode", -1, -1, -1, "native ledger is required")
	}
	n := len(run.Snapshots)
	if n == 0 {
		return bad("lifecycle", -1, -1, -1, "nonempty native snapshots are required")
	}
	if n > MaxRetainedRows || len(run.States) > MaxRetainedRows || len(run.Orders) > n || len(run.Events) > 4*n || len(run.Gaps) > n || len(run.Assumptions) > 13 {
		return resource("output_bound", "native ledger exceeds a collection bound")
	}
	for i := range run.Orders {
		if len(run.Orders[i].EventIDs) > 26 {
			return resource("output_bound", "native order exceeds its event-link bound")
		}
	}
	if len(run.States) != n {
		return bad("lifecycle", -1, -1, -1, "native snapshots and states must have equal length")
	}
	if run.ResearchPolicy != nil || run.ResearchProducer != nil || run.ResearchPolicySHA256 != "" {
		return bad("raw_decode", -1, -1, -1, "research policy overlays are not admitted")
	}
	if run.Schema != "strat-adaptive-volume-flag-reference-v1" || run.Policy != dsl.AdaptiveFlagPolicy || run.ExecutionSemantics != engine.AdaptiveFlagExecutionSemantics || run.NumericalPolicy != dsl.AdaptiveFlagNumericalPolicy {
		return bad("raw_decode", -1, -1, -1, "unsupported native schema or execution policy")
	}
	// Inspect every retained numeric field, including optional indicators and
	// diagnostic prices. The preceding cardinality checks bound this typed walk.
	if !nativeFiniteFields(reflect.ValueOf(run)) {
		return bad("raw_decode", -1, -1, -1, "native ledger contains a nonfinite numeric field")
	}
	if !nativeBoundedStrings(reflect.ValueOf(run), "") {
		return resource("output_bound", "native ledger contains an invalid or overlong string")
	}
	step := run.TimeframeMS
	if step != 1800000 && step != 3600000 {
		return domain("time_domain", -1, "native timeframe must be M30 or H1")
	}
	timeframe := "M30"
	if step == 3600000 {
		timeframe = "H1"
	}
	rules, err := dsl.AdaptiveFlagPreset(run.EffectiveConfig.Bundle)
	if err != nil || run.EffectiveConfig != (dsl.AdaptiveFlagSpec{Policy: dsl.AdaptiveFlagPolicy, NumericalPolicy: dsl.AdaptiveFlagNumericalPolicy, Timeframe: timeframe, Bundle: run.EffectiveConfig.Bundle, Rules: rules}) {
		return bad("identity", -1, -1, -1, "native effective configuration must exactly match a named A/B/C preset")
	}
	for _, digest := range []string{run.InputSHA256, run.ConfigSHA256, run.SourcePineSHA256} {
		if !nativeDigest(digest) {
			return bad("identity", -1, -1, -1, "native ledger requires lowercase SHA256 provenance")
		}
	}
	prior := int64(-1)
	for i, row := range run.Snapshots {
		if row.OpenT < 0 || row.OpenT >= MaxCalendarEndpointMS || row.OpenT%step != 0 || row.OpenT <= prior || row.OpenT > MaxCalendarEndpointMS-step {
			return domain("time_domain", i, "native bar time is outside the ordered calendar grid")
		}
		if row.Index != i || row.CloseT != row.OpenT+step {
			return bad("lifecycle", i, -1, -1, "native bar index or nominal close is inconsistent")
		}
		prior = row.OpenT
		if !(0 < row.Low && row.Low <= math.Min(row.Open, row.Close) && math.Max(row.Open, row.Close) <= row.High && row.Volume >= 0) || math.Abs(row.Open) > 1e100 || math.Abs(row.High) > 1e100 || math.Abs(row.Low) > 1e100 || math.Abs(row.Close) > 1e100 || math.Abs(row.Volume) > 1e100 {
			return bad("lifecycle", i, -1, -1, "native retained OHLCV is outside the admitted domain")
		}
		state := run.States[i]
		if state.RowIdx != i || state.CloseMS != row.CloseT {
			return bad("lifecycle", i, -1, -1, "native state does not match its observed close")
		}
		if state.PendingAge < 0 || state.PendingAge > MaxRetainedRows {
			return bad("lifecycle", i, -1, -1, "native state pending age is outside the admitted row bound")
		}
		for _, id := range []*int{state.PendingOrderID, state.PositionOrderID} {
			if id != nil && (*id < 0 || *id >= len(run.Orders)) {
				return bad("lifecycle", i, -1, -1, "native state has an invalid order reference")
			}
		}
		for _, pivot := range []*engine.AdaptiveFlagPivot{row.ConfirmedHigh, row.ConfirmedLow, row.LastHigh, row.LastLow} {
			if pivot != nil && (pivot.Index < 0 || pivot.Index > pivot.ConfirmationIdx || pivot.ConfirmationIdx > i) {
				return bad("identity", i, -1, -1, "native pivot indices must be causal")
			}
		}
	}
	ctx.Start, ctx.End = run.Snapshots[0].OpenT, run.Snapshots[n-1].CloseT
	if run.ExecutionWindow != nil {
		ctx.Start, ctx.End = run.ExecutionWindow.TradeFromMS, run.ExecutionWindow.TradeToMS
	}
	if ctx.Start < 0 || ctx.Start >= ctx.End || ctx.End > MaxCalendarEndpointMS || ctx.Start%step != 0 || ctx.End%step != 0 {
		return domain("time_domain", -1, "execution window must be a positive calendar-bounded grid interval")
	}
	ctx.Days = int((ctx.End-1)/unitDayMS - ctx.Start/unitDayMS + 1)
	if ctx.Days > MaxCalendarDays {
		return resource("calendar_count", "execution window exceeds the calendar-day bound")
	}
	for i, row := range run.Snapshots {
		if row.OpenT >= ctx.End {
			return bad("lifecycle", i, -1, -1, "retained native snapshots must stop before the execution window end")
		}
		if row.OpenT < ctx.Start {
			ctx.Pre++
			state := run.States[i]
			if state.Status != "flat" || state.PendingOrderID != nil || state.PositionOrderID != nil || state.QueuedExit != nil {
				return bad("lifecycle", i, -1, -1, "warmup states must remain flat")
			}
		}
	}
	if run.PreTradeRows != ctx.Pre || run.EligibleTradeRows != n-ctx.Pre {
		return bad("lifecycle", -1, -1, -1, "native execution-window counts are inconsistent")
	}
	eligible := func(i int) bool { return i >= ctx.Pre && i < n }
	for i, order := range run.Orders {
		if order.ID != i || !eligible(order.SignalIdx) || (order.Side != "long" && order.Side != "short") {
			return bad("lifecycle", order.SignalIdx, i, -1, "native order identity, side, or creation row is invalid")
		}
		if order.PendingAge < 0 || order.PendingAge > MaxRetainedRows || order.FillOpportunities < 0 || order.FillOpportunities > MaxRetainedRows {
			return bad("lifecycle", order.SignalIdx, i, -1, "native order counters are outside the admitted row bound")
		}
		// The explicit float64 conversion is the declared binary64 rounding
		// barrier, including when finite operands have an overflowing difference.
		distance := math.Abs(float64(order.Trigger - order.Stop))
		if math.IsInf(distance, 0) || order.PlannedTriggerToStopDistance != distance {
			return bad("distance", order.SignalIdx, i, -1, "native declared planned distance is inconsistent")
		}
		for _, idx := range []*int{order.FillIdx, order.ExitIdx, order.BracketCreationIdx} {
			if idx != nil && !eligible(*idx) {
				return bad("lifecycle", *idx, i, -1, "native fill or bracket row is outside the execution window")
			}
		}
		if (order.FillIdx == nil) != (order.BracketCreationIdx == nil) || (order.FillIdx != nil && *order.BracketCreationIdx != *order.FillIdx) {
			return bad("lifecycle", order.SignalIdx, i, -1, "native bracket creation metadata must match the fill row")
		}
	}
	// Order creation indices and state references are now bounded, so these
	// differences cannot overflow even for a malformed typed input.
	for i, state := range run.States {
		age, oid := 0, -1
		if state.PendingOrderID != nil {
			oid = *state.PendingOrderID
			age = i - run.Orders[oid].SignalIdx
		}
		if state.PendingAge != age {
			return bad("lifecycle", i, oid, -1, "native state pending age must match its pending order and observed row")
		}
	}
	if !reflect.DeepEqual(run.Terminal, run.States[n-1]) {
		return bad("lifecycle", n-1, -1, -1, "terminal state must equal the final observed native state")
	}
	ctx.EventsByRow = make([][]int, n)
	seenLinks := make([]int, len(run.Orders))
	entries, exits := make([]int, len(run.Orders)), make([]int, len(run.Orders))
	expiries, brackets := make([]int, len(run.Orders)), make([]int, len(run.Orders))
	for i := range entries {
		entries[i], exits[i] = -1, -1
		expiries[i], brackets[i] = -1, -1
	}
	priorRow := -1
	for i, event := range run.Events {
		idx, oid := event.RowIdx, event.OrderID
		if event.ID != i || !eligible(idx) || idx < priorRow || oid < 0 || oid >= len(run.Orders) {
			return bad("lifecycle", idx, oid, i, "native event identity, ordering, or order link is invalid")
		}
		priorRow = idx
		order := run.Orders[oid]
		link := seenLinks[oid]
		if link >= len(order.EventIDs) || order.EventIDs[link] != i {
			return bad("lifecycle", idx, oid, i, "native order event links must exactly match event order")
		}
		seenLinks[oid]++
		row, phase, when := run.Snapshots[idx], eventPhase(event), event.Time
		allowed := false
		switch event.State {
		case "filled", "closed":
			allowed = phase == "open" || phase == "intrabar"
		case "pending", "expired", "bracket-created", "exit-queued":
			allowed = phase == "close"
		case "entry-opportunity":
			allowed = phase == "bar-interval"
		}
		if !allowed {
			return bad("lifecycle", idx, oid, i, "native event state has an invalid phase")
		}
		if phase == "open" || phase == "close" {
			exact := row.OpenT
			if phase == "close" {
				exact = row.CloseT
			}
			if when.LowerMS != exact || when.UpperMS != exact || !when.LowerInclusive || !when.UpperInclusive {
				return bad("lifecycle", idx, oid, i, "native point event time must equal its observed endpoint")
			}
		} else if when.LowerMS != row.OpenT || when.UpperMS != row.CloseT || when.UpperInclusive || when.LowerInclusive != (phase == "bar-interval") {
			return bad("lifecycle", idx, oid, i, "native intrabar event must preserve its declared uncertainty interval")
		}
		switch event.State {
		case "filled":
			if entries[oid] >= 0 || expiries[oid] >= 0 || order.FillIdx == nil || *order.FillIdx != idx || order.Entry == nil || event.Price == nil || *event.Price <= 0 || *order.Entry != *event.Price || order.EntryAtOpen != (phase == "open") {
				return bad("lifecycle", idx, oid, i, "native entry references or unique fill are inconsistent")
			}
			if order.ActualFillToStopDistance == nil || *order.ActualFillToStopDistance != math.Abs(float64(*event.Price-order.Stop)) {
				return bad("distance", idx, oid, i, "native declared actual fill distance is inconsistent")
			}
			entries[oid] = i
		case "closed":
			if entries[oid] < 0 || exits[oid] >= 0 || order.ExitIdx == nil || *order.ExitIdx != idx || order.Exit == nil || event.Price == nil || *event.Price <= 0 || *order.Exit != *event.Price {
				return bad("lifecycle", idx, oid, i, "native exit references or unique close are inconsistent")
			}
			exits[oid] = i
		case "expired":
			if expiries[oid] >= 0 || entries[oid] >= 0 {
				return bad("lifecycle", idx, oid, i, "native expiry must be unique and unfilled")
			}
			expiries[oid] = i
		case "bracket-created":
			if entries[oid] < 0 || brackets[oid] >= 0 || order.FillIdx == nil || *order.FillIdx != idx {
				return bad("lifecycle", idx, oid, i, "native bracket creation must occur once after its fill at that row's close")
			}
			brackets[oid] = i
		}
		ctx.EventsByRow[idx] = append(ctx.EventsByRow[idx], i)
	}
	for i, order := range run.Orders {
		if seenLinks[i] != len(order.EventIDs) || (order.FillIdx != nil) != (entries[i] >= 0) || (order.ExitIdx != nil) != (exits[i] >= 0) {
			return bad("lifecycle", -1, i, -1, "native order links or fill/close references are inconsistent")
		}
		if (entries[i] >= 0) != (brackets[i] >= 0) {
			return bad("lifecycle", order.SignalIdx, i, -1, "native filled orders require exactly one bracket creation event")
		}
		endpoint := n - 1
		if entries[i] >= 0 {
			endpoint = run.Events[entries[i]].RowIdx
		} else if expiries[i] >= 0 {
			endpoint = run.Events[expiries[i]].RowIdx
		}
		// Opportunity events are deliberately absent from the private hand
		// adapters. Reconcile native counters from observed lifecycle rows.
		opportunities := endpoint - order.SignalIdx
		age := opportunities
		if entries[i] >= 0 {
			age--
		}
		if order.FillOpportunities != opportunities || order.PendingAge != age {
			return bad("lifecycle", endpoint, i, -1, "native order counters must match observed lifecycle opportunities")
		}
	}
	// Only count exposure elements here: no projection lists or economic
	// intermediates are constructed. Every unique fill contributes one closed
	// exposure or a possible terminal exposure. Sequential lifecycle validation
	// subsequently rejects any unmatched exposure that is not actually terminal.
	for oid, entryID := range entries {
		if entryID < 0 {
			continue
		}
		entry := run.Events[entryID]
		last := run.Snapshots[n-1]
		endpoint := engine.AdaptiveFlagEvent{ID: -1, OrderID: oid, RowIdx: n - 1, Time: engine.AdaptiveFlagEventTime{LowerMS: last.CloseT, UpperMS: last.CloseT, LowerInclusive: true, UpperInclusive: true, Kind: "nominal-close-point"}}
		if exits[oid] >= 0 {
			endpoint = run.Events[exits[oid]]
		}
		if err := countNativeExposure(&ctx, entry, endpoint, run.Snapshots); err != nil {
			return validationContext{}, err
		}
	}
	return ctx, nil
}

func countNativeExposure(ctx *validationContext, entry, endpoint engine.AdaptiveFlagEvent, rows []engine.AdaptiveFlagSnapshot) error {
	a, z := entry.Time, endpoint.Time
	for boundary := (a.LowerMS/unitDayMS + 1) * unitDayMS; boundary <= z.UpperMS; boundary += unitDayMS {
		if a.LowerMS < boundary && (z.UpperMS > boundary || z.UpperInclusive) {
			ctx.Midnights++
		}
		if (a.UpperMS < boundary || (a.UpperMS == boundary && !a.UpperInclusive)) && z.LowerMS >= boundary {
			ctx.Midnights++
		}
		if ctx.Midnights > MaxExposureMidnights {
			return newCoreError("resource_limit", "exposure_count", endpoint.RowIdx, entry.OrderID, endpoint.ID, "aggregate exposure midnight elements exceed the bound")
		}
	}
	for i := entry.RowIdx + 1; i <= endpoint.RowIdx; i++ {
		if rows[i].OpenT > rows[i-1].CloseT {
			ctx.Gaps++
			if ctx.Gaps > MaxExposureGaps {
				return newCoreError("resource_limit", "exposure_count", endpoint.RowIdx, entry.OrderID, endpoint.ID, "aggregate copied exposure gaps exceed the bound")
			}
		}
	}
	return nil
}

func nativeDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func nativeFiniteFields(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer:
		return v.IsNil() || nativeFiniteFields(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !nativeFiniteFields(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !nativeFiniteFields(v.Index(i)) {
				return false
			}
		}
	case reflect.Float64:
		return !math.IsNaN(v.Float()) && !math.IsInf(v.Float(), 0)
	}
	return true
}

// These are the existing raw object's string allowances. Exact enums/config
// are additionally checked above; private invented fixtures may use bounded
// descriptive reasons such as synthetic-exit without claiming public admission.
func nativeBoundedStrings(v reflect.Value, name string) bool {
	switch v.Kind() {
	case reflect.Pointer:
		return v.IsNil() || nativeBoundedStrings(v.Elem(), name)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !nativeBoundedStrings(v.Field(i), v.Type().Field(i).Name) {
				return false
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !nativeBoundedStrings(v.Index(i), name) {
				return false
			}
		}
	case reflect.String:
		limit := 1024
		switch name {
		case "Side":
			limit = 8
		case "Status":
			limit = 16
		case "Reason", "State":
			limit = 32
		case "Kind":
			limit = 64
		case "Basis":
			limit = 256
		}
		return len(v.String()) <= limit && utf8.ValidString(v.String())
	}
	return true
}
