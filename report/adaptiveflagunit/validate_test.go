package adaptiveflagunit

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/engine"
)

func validationHandRun(t *testing.T, name string) *engine.AdaptiveFlagResult {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testsupport", "testdata", "adaptive-flag-unit", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var run engine.AdaptiveFlagResult
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}
	return &run
}

func validationError(t *testing.T, err error, code, operation string) {
	t.Helper()
	var core *coreError
	if !errors.As(err, &core) || core.code != code || core.operation != operation {
		t.Fatalf("want %s/%s, got %#v", code, operation, err)
	}
}

func validationFlatRun(t *testing.T, opens []int64) *engine.AdaptiveFlagResult {
	t.Helper()
	run := validationHandRun(t, "hand-long-pending")
	row := run.Snapshots[0]
	run.Orders, run.Events = nil, nil
	run.Snapshots = make([]engine.AdaptiveFlagSnapshot, len(opens))
	run.States = make([]engine.AdaptiveFlagState, len(opens))
	for i, open := range opens {
		run.Snapshots[i] = row
		run.Snapshots[i].Index, run.Snapshots[i].OpenT, run.Snapshots[i].CloseT = i, open, open+run.TimeframeMS
		run.States[i] = engine.AdaptiveFlagState{RowIdx: i, CloseMS: open + run.TimeframeMS, Status: "flat"}
	}
	run.Terminal = run.States[len(opens)-1]
	run.ExecutionWindow = nil
	run.PreTradeRows, run.EligibleTradeRows = 0, len(opens)
	return run
}

func TestValidateRunFrozenPrivateHandInputs(t *testing.T) {
	for _, name := range []string{"hand-long-pending", "hand-long-open", "hand-long-closed", "hand-short-closed", "hand-invalid-risk-pending", "hand-invalid-risk-filled"} {
		t.Run(name, func(t *testing.T) {
			run := validationHandRun(t, name)
			before, err := json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := validateRun(run)
			if err != nil {
				t.Fatalf("private adapter fixture refused: %s", errorJSON(err))
			}
			if ctx.Pre != 0 || ctx.Days != 1 || ctx.Midnights != 0 || ctx.Gaps != 0 || len(ctx.EventsByRow) != len(run.Snapshots) {
				t.Fatalf("incorrect fixture context: %+v", ctx)
			}
			after, _ := json.Marshal(run)
			if string(before) != string(after) {
				t.Fatal("validation mutated the native ledger")
			}
		})
	}
}

func TestValidateRunMalformedStructure(t *testing.T) {
	tests := []struct {
		name, code, operation string
		mutate                func(*engine.AdaptiveFlagResult)
	}{
		{"schema", "invalid_native_ledger", "raw_decode", func(r *engine.AdaptiveFlagResult) { r.Schema = "other" }},
		{"policy", "invalid_native_ledger", "raw_decode", func(r *engine.AdaptiveFlagResult) { r.Policy = "G" }},
		{"semantics", "invalid_native_ledger", "raw_decode", func(r *engine.AdaptiveFlagResult) { r.ExecutionSemantics = "other" }},
		{"numerical", "invalid_native_ledger", "raw_decode", func(r *engine.AdaptiveFlagResult) { r.NumericalPolicy = "other" }},
		{"research", "invalid_native_ledger", "raw_decode", func(r *engine.AdaptiveFlagResult) { r.ResearchPolicySHA256 = strings.Repeat("a", 64) }},
		{"custom", "invalid_native_ledger", "identity", func(r *engine.AdaptiveFlagResult) { r.EffectiveConfig.Bundle = "CUSTOM" }},
		{"config-rule", "invalid_native_ledger", "identity", func(r *engine.AdaptiveFlagResult) { r.EffectiveConfig.Rules.ATRLen++ }},
		{"config-timeframe", "invalid_native_ledger", "identity", func(r *engine.AdaptiveFlagResult) { r.EffectiveConfig.Timeframe = "H1" }},
		{"digest", "invalid_native_ledger", "identity", func(r *engine.AdaptiveFlagResult) { r.InputSHA256 = strings.Repeat("A", 64) }},
		{"states", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.States = r.States[:2] }},
		{"row-index", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Snapshots[1].Index = 0 }},
		{"close-time", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Snapshots[1].CloseT++ }},
		{"state-time", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.States[1].CloseMS++ }},
		{"state-order", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.States[1].PositionOrderID = validationInt(2) }},
		{"negative-volume", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Snapshots[0].Volume = -1 }},
		{"zero-low", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Snapshots[0].Low = 0 }},
		{"broken-envelope", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Snapshots[0].High = r.Snapshots[0].Low }},
		{"ohlcv-bound", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Snapshots[0].Volume = math.Nextafter(1e100, math.Inf(1)) }},
		{"future-pivot", "invalid_native_ledger", "identity", func(r *engine.AdaptiveFlagResult) { r.Snapshots[0].LastHigh.ConfirmationIdx = 1 }},
		{"negative-pivot", "invalid_native_ledger", "identity", func(r *engine.AdaptiveFlagResult) { r.Snapshots[0].LastHigh.Index = -1 }},
		{"order-id", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].ID = 1 }},
		{"order-side", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].Side = "flat" }},
		{"signal-row", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].SignalIdx = 3 }},
		{"distance", "invalid_native_ledger", "distance", func(r *engine.AdaptiveFlagResult) { r.Orders[0].PlannedTriggerToStopDistance++ }},
		{"overflow-distance", "invalid_native_ledger", "distance", func(r *engine.AdaptiveFlagResult) {
			r.Orders[0].Trigger, r.Orders[0].Stop = math.MaxFloat64, -math.MaxFloat64
		}},
		{"actual-distance", "invalid_native_ledger", "distance", func(r *engine.AdaptiveFlagResult) { *r.Orders[0].ActualFillToStopDistance++ }},
		{"fill-row", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].FillIdx = validationInt(0) }},
		{"missing-entry", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].Entry = nil }},
		{"entry-phase", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].EntryAtOpen = false }},
		{"exit-reference", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].ExitIdx = validationInt(1) }},
		{"missing-exit", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].Exit = nil }},
		{"event-id", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Events[1].ID = 0 }},
		{"event-order", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Events[3].RowIdx = 0 }},
		{"event-order-link", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Events[1].OrderID = 1 }},
		{"event-links", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].EventIDs[1] = 2 }},
		{"extra-event-link", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Orders[0].EventIDs = append(r.Orders[0].EventIDs, 4) }},
		{"negative-entry", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Events[1].Price = validationFloat(-1) }},
		{"exit-price", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Events[3].Price = validationFloat(114) }},
		{"terminal", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.Terminal.PendingAge++ }},
		{"window-count", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.EligibleTradeRows++ }},
		{"warmup-occupancy", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.ExecutionWindow.TradeFromMS += r.TimeframeMS }},
		{"post-window-row", "invalid_native_ledger", "lifecycle", func(r *engine.AdaptiveFlagResult) { r.ExecutionWindow.TradeToMS -= r.TimeframeMS }},
		{"basis-bound", "resource_limit", "output_bound", func(r *engine.AdaptiveFlagResult) { r.Events[1].Basis = strings.Repeat("a", 257) }},
		{"reason-bound", "resource_limit", "output_bound", func(r *engine.AdaptiveFlagResult) { r.Orders[0].Reason = strings.Repeat("a", 33) }},
		{"invalid-utf8", "resource_limit", "output_bound", func(r *engine.AdaptiveFlagResult) { r.Assumptions = []string{"\xff"} }},
		{"assumption-bound", "resource_limit", "output_bound", func(r *engine.AdaptiveFlagResult) { r.Assumptions = make([]string, 14) }},
		{"gap-bound", "resource_limit", "output_bound", func(r *engine.AdaptiveFlagResult) { r.Gaps = make([]engine.AdaptiveFlagGap, 4) }},
		{"order-bound", "resource_limit", "output_bound", func(r *engine.AdaptiveFlagResult) { r.Orders = make([]engine.AdaptiveFlagOrder, 4) }},
		{"event-bound", "resource_limit", "output_bound", func(r *engine.AdaptiveFlagResult) { r.Events = make([]engine.AdaptiveFlagEvent, 13) }},
		{"link-bound", "resource_limit", "output_bound", func(r *engine.AdaptiveFlagResult) { r.Orders[0].EventIDs = make([]int, 27) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := validationHandRun(t, "hand-long-closed")
			tc.mutate(run)
			_, err := validateRun(run)
			validationError(t, err, tc.code, tc.operation)
		})
	}
	if _, err := validateRun(nil); err == nil {
		t.Fatal("nil ledger accepted")
	}
}

func TestValidateRunFiniteFields(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, mutate := range []func(*engine.AdaptiveFlagResult, float64){
			func(r *engine.AdaptiveFlagResult, f float64) { r.Snapshots[0].ATR = &f },
			func(r *engine.AdaptiveFlagResult, f float64) { r.Snapshots[0].FastEMA = f },
			func(r *engine.AdaptiveFlagResult, f float64) {
				r.Snapshots[0].Candidate = &engine.AdaptiveFlagSignal{Target: f}
			},
			func(r *engine.AdaptiveFlagResult, f float64) { r.Snapshots[0].LastHigh.Price = f },
			func(r *engine.AdaptiveFlagResult, f float64) { r.Orders[0].Target = f },
			func(r *engine.AdaptiveFlagResult, f float64) { r.Orders[0].Entry = &f },
			func(r *engine.AdaptiveFlagResult, f float64) { r.Events[0].Price = &f },
			func(r *engine.AdaptiveFlagResult, f float64) { r.EffectiveConfig.Rules.TargetR = f },
		} {
			run := validationHandRun(t, "hand-long-pending")
			mutate(run, bad)
			_, err := validateRun(run)
			validationError(t, err, "invalid_native_ledger", "raw_decode")
		}
	}
}

func TestValidateRunCalendarAndWarmupBoundaries(t *testing.T) {
	const step int64 = 1800000
	for _, bundle := range []string{"INITIAL", "TWEAKED", "SNAPSHOT_C"} {
		for _, frame := range []string{"M30", "H1"} {
			run := validationFlatRun(t, []int64{0})
			run.EffectiveConfig.Bundle = bundle
			run.EffectiveConfig.Rules, _ = dsl.AdaptiveFlagPreset(bundle)
			run.EffectiveConfig.Timeframe = frame
			if frame == "H1" {
				run.TimeframeMS *= 2
				run.Snapshots[0].CloseT = run.TimeframeMS
				run.States[0].CloseMS = run.TimeframeMS
				run.Terminal = run.States[0]
			}
			if _, err := validateRun(run); err != nil {
				t.Fatalf("named configuration %s/%s: %s", bundle, frame, errorJSON(err))
			}
		}
	}
	for _, size := range []int{1024, 1025} {
		opens := make([]int64, size)
		for i := range opens {
			opens[i] = int64(i) * step
		}
		run := validationFlatRun(t, opens)
		run.ExecutionWindow = &engine.AdaptiveFlagExecutionWindow{TradeFromMS: int64(size) * step, TradeToMS: int64(size+1) * step}
		run.PreTradeRows, run.EligibleTradeRows = size, 0
		ctx, err := validateRun(run)
		if size == 1025 {
			validationError(t, err, "resource_limit", "output_bound")
		} else if err != nil || ctx.Pre != 1024 || ctx.Days != 1 {
			t.Fatalf("all-warmup 1024-row boundary failed: %+v %v", ctx, err)
		}
	}
	run := validationFlatRun(t, []int64{0})
	run.ExecutionWindow = &engine.AdaptiveFlagExecutionWindow{TradeFromMS: 0, TradeToMS: 366 * unitDayMS}
	if ctx, err := validateRun(run); err != nil || ctx.Days != 366 {
		t.Fatalf("366-day boundary: %+v %v", ctx, err)
	}
	run.ExecutionWindow.TradeToMS += step
	_, err := validateRun(run)
	validationError(t, err, "resource_limit", "calendar_count")
	// Less than 366 elapsed days can still touch 367 calendar days.
	run = validationFlatRun(t, []int64{unitDayMS - step})
	run.ExecutionWindow = &engine.AdaptiveFlagExecutionWindow{TradeFromMS: unitDayMS - step, TradeToMS: 366*unitDayMS + step}
	_, err = validateRun(run)
	validationError(t, err, "resource_limit", "calendar_count")
	run = validationFlatRun(t, []int64{MaxCalendarEndpointMS - step})
	if ctx, err := validateRun(run); err != nil || ctx.End != MaxCalendarEndpointMS {
		t.Fatalf("last admissible UTC bar: %+v %v", ctx, err)
	}
	for _, open := range []int64{-step, 1, MaxCalendarEndpointMS, math.MaxInt64 - step} {
		run = validationFlatRun(t, []int64{open})
		_, err = validateRun(run)
		validationError(t, err, "projection_domain_rejected", "time_domain")
	}
	run = validationFlatRun(t, []int64{0, 0})
	_, err = validateRun(run)
	validationError(t, err, "projection_domain_rejected", "time_domain")
}

func TestValidateRunEventPhaseIntervals(t *testing.T) {
	for _, phase := range []string{"open", "intrabar"} {
		run := validationHandRun(t, "hand-long-closed")
		if phase == "intrabar" {
			for _, i := range []int{1, 3} {
				row := run.Snapshots[run.Events[i].RowIdx]
				run.Events[i].Time = engine.AdaptiveFlagEventTime{LowerMS: row.OpenT, UpperMS: row.CloseT, Kind: "nonopening-crossing-unknown-model-time"}
			}
			run.Orders[0].EntryAtOpen = false
		}
		if _, err := validateRun(run); err != nil {
			t.Fatalf("valid %s phase: %s", phase, errorJSON(err))
		}
		for _, mutate := range []func(*engine.AdaptiveFlagEventTime){
			func(v *engine.AdaptiveFlagEventTime) { v.LowerMS++ },
			func(v *engine.AdaptiveFlagEventTime) { v.UpperMS++ },
			func(v *engine.AdaptiveFlagEventTime) { v.LowerInclusive = !v.LowerInclusive },
			func(v *engine.AdaptiveFlagEventTime) { v.UpperInclusive = !v.UpperInclusive },
			func(v *engine.AdaptiveFlagEventTime) { v.Kind = "unknown" },
		} {
			saved := run.Events[1].Time
			mutate(&run.Events[1].Time)
			_, err := validateRun(run)
			validationError(t, err, "invalid_native_ledger", "lifecycle")
			run.Events[1].Time = saved
		}
	}
	// Opportunity intervals are inclusive only at their opening endpoint.
	run := validationHandRun(t, "hand-long-open")
	row := run.Snapshots[1]
	opportunity := engine.AdaptiveFlagEvent{ID: 3, OrderID: 0, RowIdx: 1, State: "entry-opportunity", Time: engine.AdaptiveFlagEventTime{LowerMS: row.OpenT, UpperMS: row.CloseT, LowerInclusive: true, Kind: "observed-bar-opportunity"}}
	run.Events = append(run.Events, opportunity)
	run.Orders[0].EventIDs = append(run.Orders[0].EventIDs, 3)
	if _, err := validateRun(run); err != nil {
		t.Fatal(errorJSON(err))
	}
	run.Events[3].Time.UpperInclusive = true
	_, err := validateRun(run)
	validationError(t, err, "invalid_native_ledger", "lifecycle")
	for _, state := range []string{"unknown", "filled", "closed", "entry-opportunity"} {
		run = validationHandRun(t, "hand-long-pending")
		run.Events[0].State = state
		_, err := validateRun(run)
		validationError(t, err, "invalid_native_ledger", "lifecycle")
	}
}

func TestValidateRunDuplicateFillAndClose(t *testing.T) {
	for _, index := range []int{1, 3} {
		run := validationHandRun(t, "hand-long-closed")
		duplicate := run.Events[index]
		run.Events = append(run.Events[:index+1], append([]engine.AdaptiveFlagEvent{duplicate}, run.Events[index+1:]...)...)
		run.Orders[0].EventIDs = nil
		for i := range run.Events {
			run.Events[i].ID = i
			run.Orders[0].EventIDs = append(run.Orders[0].EventIDs, i)
		}
		_, err := validateRun(run)
		validationError(t, err, "invalid_native_ledger", "lifecycle")
	}
}

func TestValidateRunCountsTerminalExposure(t *testing.T) {
	run := validationHandRun(t, "hand-long-open")
	start := run.Snapshots[0].OpenT
	last := run.Snapshots[1]
	last.Index, last.OpenT, last.CloseT = 2, start+366*unitDayMS-run.TimeframeMS, start+366*unitDayMS
	run.Snapshots = append(run.Snapshots, last)
	state := run.States[1]
	state.RowIdx, state.CloseMS = 2, last.CloseT
	run.States = append(run.States, state)
	run.Terminal, run.EligibleTradeRows = state, 3
	run.ExecutionWindow.TradeToMS = last.CloseT
	ctx, err := validateRun(run)
	if err != nil || ctx.Midnights != 732 || ctx.Gaps != 1 {
		t.Fatalf("terminal exposure bound not included: %+v %v", ctx, err)
	}
	// Requested trailing daily horizon does not extend the terminal exposure.
	run.Snapshots[2].OpenT -= unitDayMS
	run.Snapshots[2].CloseT -= unitDayMS
	run.States[2].CloseMS -= unitDayMS
	run.Terminal = run.States[2]
	ctx, err = validateRun(run)
	if err != nil || ctx.Midnights != 730 || ctx.Days != 366 {
		t.Fatalf("daily horizon leaked into exposure: %+v %v", ctx, err)
	}
}

func TestValidateRunAggregatesClosedAndTerminalExposure(t *testing.T) {
	run := validationHandRun(t, "hand-long-closed")
	open := validationHandRun(t, "hand-long-open")
	start := run.Snapshots[0].OpenT
	opens := []int64{start, start + run.TimeframeMS, start + unitDayMS, start + 2*unitDayMS, start + 3*unitDayMS}
	run.Snapshots = append(run.Snapshots, open.Snapshots[1], open.Snapshots[1])
	run.States = append(run.States, open.States[1], open.States[1])
	second := open.Orders[0]
	second.ID, second.SignalIdx = 1, 2
	second.FillIdx, second.BracketCreationIdx = validationInt(3), validationInt(3)
	second.EventIDs = []int{4, 5, 6}
	run.Orders = append(run.Orders, second)
	for i, event := range open.Events {
		event.ID, event.OrderID, event.RowIdx = i+4, 1, event.RowIdx+2
		run.Events = append(run.Events, event)
	}
	run.States[2] = engine.AdaptiveFlagState{Status: "pending", PendingOrderID: validationInt(1)}
	for _, i := range []int{3, 4} {
		run.States[i].PositionOrderID = validationInt(1)
	}
	for i, ms := range opens {
		run.Snapshots[i].Index, run.Snapshots[i].OpenT, run.Snapshots[i].CloseT = i, ms, ms+run.TimeframeMS
		run.States[i].RowIdx, run.States[i].CloseMS = i, ms+run.TimeframeMS
	}
	for i := range run.Events {
		event := &run.Events[i]
		row := run.Snapshots[event.RowIdx]
		when := row.OpenT
		if eventPhase(*event) == "close" {
			when = row.CloseT
		}
		event.Time.LowerMS, event.Time.UpperMS = when, when
	}
	run.Terminal, run.EligibleTradeRows = run.States[4], 5
	run.ExecutionWindow.TradeToMS = run.Snapshots[4].CloseT
	ctx, err := validateRun(run)
	if err != nil || ctx.Midnights != 4 || ctx.Gaps != 2 {
		t.Fatalf("closed and terminal exposure elements must be counted together: %+v %v", ctx, err)
	}
}

func TestCountNativeExposureGlobalBudgetsAndInclusivity(t *testing.T) {
	point := func(ms int64) engine.AdaptiveFlagEventTime {
		return engine.AdaptiveFlagEventTime{LowerMS: ms, UpperMS: ms, LowerInclusive: true, UpperInclusive: true, Kind: "observed-open-point"}
	}
	rows := []engine.AdaptiveFlagSnapshot{{AdaptiveFlagBar: engine.AdaptiveFlagBar{OpenT: 0, CloseT: 1800000}}, {AdaptiveFlagBar: engine.AdaptiveFlagBar{OpenT: unitDayMS, CloseT: unitDayMS + 1800000}}}
	entry := engine.AdaptiveFlagEvent{RowIdx: 0, Time: point(0)}
	endpoint := engine.AdaptiveFlagEvent{ID: 1, RowIdx: 1, Time: point(unitDayMS)}
	ctx := validationContext{Midnights: 730, Gaps: 1023}
	if err := countNativeExposure(&ctx, entry, endpoint, rows); err != nil || ctx.Midnights != 732 || ctx.Gaps != 1024 {
		t.Fatalf("inclusive exposure budget: %+v %v", ctx, err)
	}
	err := countNativeExposure(&ctx, entry, endpoint, rows)
	validationError(t, err, "resource_limit", "exposure_count")
	ctx = validationContext{Gaps: 1024}
	err = countNativeExposure(&ctx, entry, endpoint, rows)
	validationError(t, err, "resource_limit", "exposure_count")
	for _, tc := range []struct {
		name            string
		entry, endpoint engine.AdaptiveFlagEventTime
		count           int
	}{
		{"exit-exclusive-midnight", point(0), engine.AdaptiveFlagEventTime{LowerMS: unitDayMS - 1800000, UpperMS: unitDayMS}, 0},
		{"entry-exclusive-midnight", engine.AdaptiveFlagEventTime{LowerMS: unitDayMS - 1800000, UpperMS: unitDayMS}, point(unitDayMS), 2},
		{"entry-inclusive-midnight", point(unitDayMS), point(unitDayMS + 1800000), 0},
		{"possible-only", engine.AdaptiveFlagEventTime{LowerMS: unitDayMS - 1800000, UpperMS: unitDayMS, UpperInclusive: true}, point(unitDayMS), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := validationContext{}
			entry.Time, endpoint.Time = tc.entry, tc.endpoint
			if err := countNativeExposure(&ctx, entry, endpoint, rows); err != nil || ctx.Midnights != tc.count {
				t.Fatalf("midnight inclusivity: %+v %v", ctx, err)
			}
		})
	}
	if eventPhase(engine.AdaptiveFlagEvent{}) != "unknown" {
		t.Fatal("unknown event phase changed")
	}
}

// The private hand ledgers deliberately omit opportunity events. These small
// extensions exercise the row-derived counter rules without adding such events.
func validationAgedPending(t *testing.T, expired bool) *engine.AdaptiveFlagResult {
	t.Helper()
	run := validationHandRun(t, "hand-long-pending")
	first := run.Snapshots[0]
	for i := 1; i < 4; i++ {
		row := first
		row.Index = i
		row.OpenT += int64(i) * run.TimeframeMS
		row.CloseT += int64(i) * run.TimeframeMS
		run.Snapshots = append(run.Snapshots, row)
		state := engine.AdaptiveFlagState{RowIdx: i, CloseMS: row.CloseT, Status: "pending", PendingOrderID: validationInt(0), PendingAge: i}
		if expired && i >= 2 {
			state.Status, state.PendingOrderID, state.PendingAge = "flat", nil, 0
		}
		run.States = append(run.States, state)
	}
	run.Orders[0].FillOpportunities, run.Orders[0].PendingAge = 3, 3
	if expired {
		run.Orders[0].Status = "expired"
		run.Orders[0].FillOpportunities, run.Orders[0].PendingAge = 2, 2
		when := run.Snapshots[2].CloseT
		run.Events = append(run.Events, engine.AdaptiveFlagEvent{ID: 1, OrderID: 0, RowIdx: 2, State: "expired", Time: engine.AdaptiveFlagEventTime{LowerMS: when, UpperMS: when, LowerInclusive: true, UpperInclusive: true, Kind: "nominal-close-point"}})
		run.Orders[0].EventIDs = append(run.Orders[0].EventIDs, 1)
	}
	run.Terminal = run.States[3]
	run.EligibleTradeRows = 4
	run.ExecutionWindow.TradeToMS = run.Terminal.CloseMS
	return run
}

func validationRelinkEvents(run *engine.AdaptiveFlagResult) {
	for i := range run.Orders {
		run.Orders[i].EventIDs = nil
	}
	for i := range run.Events {
		run.Events[i].ID = i
		oid := run.Events[i].OrderID
		run.Orders[oid].EventIDs = append(run.Orders[oid].EventIDs, i)
	}
}

func TestValidateRunCounterEndpointsWithoutOpportunityEvents(t *testing.T) {
	for _, expired := range []bool{false, true} {
		run := validationAgedPending(t, expired)
		if _, err := validateRun(run); err != nil {
			t.Fatalf("row-derived pending/expiry counters: %s", errorJSON(err))
		}
		raw := handOwned(t, "hand-long-pending")
		raw.run = *run
		if _, err := projectCore(rebindHand(t, raw), unitRequest("RAW")); err != nil {
			t.Fatalf("pending/expiry lifecycle: %s", errorJSON(err))
		}
		// The expiry at row 2 must not accumulate the final row 3 opportunity.
		if expired {
			run.Orders[0].FillOpportunities, run.Orders[0].PendingAge = 3, 3
			_, err := validateRun(run)
			validationError(t, err, "invalid_native_ledger", "lifecycle")
		}
	}
	run := validationAgedPending(t, false)
	opened := validationHandRun(t, "hand-long-open")
	order := opened.Orders[0]
	order.FillIdx, order.BracketCreationIdx = validationInt(3), validationInt(3)
	order.FillOpportunities, order.PendingAge = 3, 2
	run.Orders[0] = order
	for _, event := range opened.Events[1:] {
		event.RowIdx = 3
		when := run.Snapshots[3].OpenT
		if eventPhase(event) == "close" {
			when = run.Snapshots[3].CloseT
		}
		event.Time.LowerMS, event.Time.UpperMS = when, when
		run.Events = append(run.Events, event)
	}
	validationRelinkEvents(run)
	run.States[3].Status, run.States[3].PendingOrderID, run.States[3].PendingAge = "open", nil, 0
	run.States[3].PositionOrderID = validationInt(0)
	run.Terminal = run.States[3]
	if _, err := validateRun(run); err != nil {
		t.Fatalf("filled counter freezes before fill-row close: %s", errorJSON(err))
	}
	raw := handOwned(t, "hand-long-open")
	raw.run = *run
	if _, err := projectCore(rebindHand(t, raw), unitRequest("RAW")); err != nil {
		t.Fatalf("delayed-fill lifecycle: %s", errorJSON(err))
	}
}

func TestValidateRunCounterBracketMutationsAndRecovery(t *testing.T) {
	good := handOwned(t, "hand-long-closed")
	baseline, err := projectCore(good, unitRequest("RAZOR_PROXY_BASIC"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*engine.AdaptiveFlagResult)
	}{
		{"negative-order-age", func(r *engine.AdaptiveFlagResult) { r.Orders[0].PendingAge = -1 }},
		{"oversized-order-age", func(r *engine.AdaptiveFlagResult) { r.Orders[0].PendingAge = MaxRetainedRows + 1 }},
		{"wrong-positive-order-age", func(r *engine.AdaptiveFlagResult) { r.Orders[0].PendingAge = 1 }},
		{"negative-opportunities", func(r *engine.AdaptiveFlagResult) { r.Orders[0].FillOpportunities = -1 }},
		{"oversized-opportunities", func(r *engine.AdaptiveFlagResult) { r.Orders[0].FillOpportunities = MaxRetainedRows + 1 }},
		{"wrong-positive-opportunities", func(r *engine.AdaptiveFlagResult) { r.Orders[0].FillOpportunities = 2 }},
		{"both-counters-999", func(r *engine.AdaptiveFlagResult) { r.Orders[0].FillOpportunities, r.Orders[0].PendingAge = 999, 999 }},
		{"negative-state-and-terminal", func(r *engine.AdaptiveFlagResult) { r.States[2].PendingAge = -1; r.Terminal = r.States[2] }},
		{"oversized-state-and-terminal", func(r *engine.AdaptiveFlagResult) {
			r.States[2].PendingAge = MaxRetainedRows + 1
			r.Terminal = r.States[2]
		}},
		{"positive-state-and-terminal", func(r *engine.AdaptiveFlagResult) { r.States[2].PendingAge = 999; r.Terminal = r.States[2] }},
		{"missing-bracket-metadata", func(r *engine.AdaptiveFlagResult) { r.Orders[0].BracketCreationIdx = nil }},
		{"bracket-metadata-before-fill", func(r *engine.AdaptiveFlagResult) { r.Orders[0].BracketCreationIdx = validationInt(0) }},
		{"bracket-metadata-after-fill", func(r *engine.AdaptiveFlagResult) { r.Orders[0].BracketCreationIdx = validationInt(2) }},
		{"missing-bracket-event", func(r *engine.AdaptiveFlagResult) {
			r.Events = append(r.Events[:2], r.Events[3:]...)
			validationRelinkEvents(r)
		}},
		{"duplicate-bracket-event", func(r *engine.AdaptiveFlagResult) {
			tail := append([]engine.AdaptiveFlagEvent{r.Events[2]}, r.Events[3:]...)
			r.Events = append(r.Events[:3], tail...)
			validationRelinkEvents(r)
		}},
		{"bracket-event-before-fill", func(r *engine.AdaptiveFlagResult) {
			r.Events[1], r.Events[2] = r.Events[2], r.Events[1]
			validationRelinkEvents(r)
		}},
		{"bracket-event-wrong-row", func(r *engine.AdaptiveFlagResult) {
			r.Events[2].RowIdx = 2
			r.Events[2].Time.LowerMS, r.Events[2].Time.UpperMS = r.Snapshots[2].CloseT, r.Snapshots[2].CloseT
		}},
		{"bracket-event-not-close", func(r *engine.AdaptiveFlagResult) { r.Events[2].Time = r.Events[1].Time }},
		{"bracket-event-wrong-order", func(r *engine.AdaptiveFlagResult) {
			second := validationHandRun(t, "hand-long-pending").Orders[0]
			second.ID = 1
			r.Orders = append(r.Orders, second)
			r.Events[2].OrderID = 1
			validationRelinkEvents(r)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := handOwned(t, "hand-long-closed")
			tc.mutate(&raw.run)
			_, err := validateRun(&raw.run)
			validationError(t, err, "invalid_native_ledger", "lifecycle")
			p, err := projectCore(rebindHand(t, raw), unitRequest("RAZOR_PROXY_BASIC"))
			if p != nil {
				t.Fatal("corrupt native ledger produced a partial projection")
			}
			validationError(t, err, "invalid_native_ledger", "lifecycle")
			// A refusal must not contaminate the next valid call or its bytes.
			recovered, err := projectCore(good, unitRequest("RAZOR_PROXY_BASIC"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(recovered)
			if err != nil || string(got) != string(want) {
				t.Fatalf("post-refusal valid projection changed: %v", err)
			}
		})
	}
}

func TestValidateRunUnfilledBracketsAndPendingStateCounters(t *testing.T) {
	for _, kind := range []string{"metadata", "event"} {
		run := validationHandRun(t, "hand-long-pending")
		if kind == "metadata" {
			run.Orders[0].BracketCreationIdx = validationInt(0)
		} else {
			event := run.Events[0]
			event.State = "bracket-created"
			run.Events = append(run.Events, event)
			validationRelinkEvents(run)
		}
		_, err := validateRun(run)
		validationError(t, err, "invalid_native_ledger", "lifecycle")
	}
	for _, age := range []int{-1, 1, 999, MaxRetainedRows + 1} {
		run := validationAgedPending(t, false)
		run.States[3].PendingAge = age
		run.Terminal = run.States[3]
		_, err := validateRun(run)
		validationError(t, err, "invalid_native_ledger", "lifecycle")
	}
	run := validationFlatRun(t, []int64{0})
	run.ExecutionWindow = &engine.AdaptiveFlagExecutionWindow{TradeFromMS: run.TimeframeMS, TradeToMS: 2 * run.TimeframeMS}
	run.PreTradeRows, run.EligibleTradeRows = 1, 0
	run.States[0].PendingAge = 1
	run.Terminal = run.States[0]
	_, err := validateRun(run)
	validationError(t, err, "invalid_native_ledger", "lifecycle")
}

func validationInt(i int) *int           { return &i }
func validationFloat(f float64) *float64 { return &f }
