package engine

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func ordinaryInspectionFault(t *testing.T, err error, code, stage string) {
	t.Helper()
	ordinaryTestFault(t, err, code)
	var fault *OrdinaryBacktestFault
	if !errors.As(err, &fault) || fault.Stage != stage {
		t.Fatalf("error stage=%v, want %s", err, stage)
	}
}

func TestOrdinaryInspectionNeverExecutesAndRunReprepares(t *testing.T) {
	fixture, source := ordinaryExecutionFixture(t, "sar-named-level-allow")
	for _, data := range []bool{false, true} {
		t.Run(fmt.Sprintf("data=%t", data), func(t *testing.T) {
			capture := ordinaryExecutionCapture(t, "inspect", source, fixture, func(m map[string]any) {
				if !data {
					m["data"] = nil
				}
			})
			plan, err := PrepareOrdinaryBacktest(capture)
			if err != nil || plan == nil {
				t.Fatal("inspect:", err)
			}
			if plan.DataValidated() != data || !ordinaryHex(plan.ConfigIdentity(), 64) || plan.SourceName() != "Named Level Sweep Synthetic" {
				t.Fatal("inspection lost independent parse/data/source facts")
			}
			if !data {
				if plan.Preflight() != "not-evaluated" || plan.prepared != nil {
					t.Fatal("text-only inspection invented execution admission or context")
				}
			} else {
				if plan.Preflight() != "admitted" || plan.prepared == nil {
					t.Fatal("accepted dataful inspection did not finish shared admission")
				}
				b := &plan.prepared.broker
				if b.paperAccounting != nil || b.sequentialAccounting != nil || b.hasPosition || b.realized != 0 || len(b.trades) != 0 || len(plan.prepared.trades) != 0 {
					t.Fatal("inspection executed or attached an observer")
				}
			}
			run, refusedPlan, err := RunOrdinaryBacktest(capture)
			ordinaryInspectionFault(t, err, "ARGUMENTS", "transport")
			if run != nil || refusedPlan != nil {
				t.Fatal("inspect capture became a run grant")
			}
		})
	}
	inspect := ordinaryExecutionCapture(t, "inspect", source, fixture, nil)
	plan, err := PrepareOrdinaryBacktest(inspect)
	if err != nil {
		t.Fatal(err)
	}
	// Poison the old nonexecuting plan. The run must prepare its own capture,
	// not execute a mutable prepared runner from a prior inspection.
	plan.prepared.offRoute = true
	plan.prepared.params.SetupType = string(dsl.FamilyDailyFlushFailure)
	plan.prepared.fixture.Costs.StartEquity = -1
	plan.admission.config["riskUsd"] = -1.0
	changed := strings.Replace(source, "risk 200 USD", "risk 137 USD", 1)
	runCapture := ordinaryExecutionCapture(t, "run", changed, fixture, nil)
	run, runPlan := ordinaryExecutionParity(t, runCapture)
	if runPlan == plan || runPlan.prepared == plan.prepared || runPlan.ConfigIdentity() == plan.ConfigIdentity() || run.NativeResult().TradeCount == 0 {
		t.Fatal("run reused stale inspection identity or execution state")
	}
	if runCapture.SourceIdentity() == inspect.SourceIdentity() || runCapture.RequestIdentity() == inspect.RequestIdentity() || *runCapture.DataIdentity() != *inspect.DataIdentity() {
		t.Fatal("independent inspect/run capture bindings changed")
	}
}

func TestOrdinaryInspectionAdmissionRefusesBeforeContext(t *testing.T) {
	fixture, source := ordinaryExecutionFixture(t, "sar-named-level-allow")
	cases := []struct {
		name, code string
		mutate     func(map[string]any)
	}{
		{"off-route", "ROUTE_UNSUPPORTED", func(m map[string]any) { m["route"].(map[string]any)["symbol"] = "NOT-DECLARED" }},
		{"unsupported-timeframe", "ROUTE_UNSUPPORTED", func(m map[string]any) { m["route"].(map[string]any)["timeframe"] = "calendar-month" }},
		{"invalid-original-OHLC", "DATA_INVALID", func(m map[string]any) { m["data"].(map[string]any)["bars"].([]any)[0].([]any)[2] = -100 }},
		{"unsorted-original-bars", "DATA_INVALID", func(m map[string]any) {
			rows := m["data"].(map[string]any)["bars"].([]any)
			rows[1].([]any)[0] = rows[0].([]any)[0]
		}},
		{"forming-original-bar", "FORMING_BAR", func(m map[string]any) { d := m["data"].(map[string]any); d["closedThroughT"] = d["lastT"] }},
	}
	for _, tc := range cases {
		for _, operation := range []string{"inspect", "run"} {
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				capture := ordinaryExecutionCapture(t, operation, source, fixture, tc.mutate)
				plan, err := PrepareOrdinaryBacktest(capture)
				ordinaryInspectionFault(t, err, tc.code, "admission")
				if plan == nil || plan.Preflight() != "refused" || plan.prepared != nil || !ordinaryHex(plan.ConfigIdentity(), 64) {
					t.Fatal("admission refusal allocated context or lost parsed identity")
				}
				if operation == "run" {
					run, runPlan, err := RunOrdinaryBacktest(capture)
					ordinaryInspectionFault(t, err, tc.code, "admission")
					if run != nil || runPlan == nil || runPlan.prepared != nil || runPlan.ConfigIdentity() != plan.ConfigIdentity() {
						t.Fatal("run bypassed deterministic inspection refusal")
					}
				}
			})
		}
	}
	for _, data := range []bool{false, true} {
		capture := ordinaryExecutionCapture(t, "inspect", source, fixture, func(m map[string]any) {
			m["route"].(map[string]any)["symbol"] = "NOT-DECLARED"
			if !data {
				m["data"] = nil
			}
		})
		plan, err := PrepareOrdinaryBacktest(capture)
		ordinaryInspectionFault(t, err, "ROUTE_UNSUPPORTED", "admission")
		if plan == nil || plan.prepared != nil {
			t.Fatal("off-route zero-trade/text-only shortcut escaped admission")
		}
	}
}

func TestOrdinaryInspectionRefusesAllUninstrumentedLoops(t *testing.T) {
	families := []string{string(dsl.FamilyClockRangeBreakout), string(dsl.FamilyDownShockRebound), string(dsl.FamilyDailyFlushFailure), string(dsl.FamilyWeekendExtremeFade), string(dsl.FamilyIntraHourRunExhaustion)}
	for _, family := range families {
		t.Run(family, func(t *testing.T) {
			ordinaryInspectionFault(t, ordinaryLoopCapability(family), "ACCOUNTING_UNAVAILABLE", "admission")
			// A malformed/empty series is deliberate: the capability boundary
			// must precede context, source fallback and zero-trade dispatch.
			request := RunRequest{Config: dsl.Config{"setupType": family}, Series: marketdata.Series{T: []float64{0}}, Symbol: "OFF-ROUTE", Timeframe: "unsupported"}
			code := "ACCOUNTING_UNAVAILABLE"
			if family == string(dsl.FamilyClockRangeBreakout) {
				code = "DEDICATED_REQUIRED"
			}
			ordinaryInspectionFault(t, ordinaryRunCapability(request), code, "admission")
			ordinaryInspectionFault(t, ordinaryPreparedCapability(&PreparedRun{params: flagParams{SetupType: family}}), "ACCOUNTING_UNAVAILABLE", "admission")
		})
	}
	fixture, _ := ordinaryExecutionFixture(t, "sar-named-level-allow")
	fixture.Bars = fixture.Bars[:1]
	for _, name := range []string{"family-clock-range-breakout-ordinary-long", "family-down-shock-rebound", "family-daily-flush-failure", "family-weekend-extreme-fade", "family-intra-hour-run-exhaustion"} {
		source, err := os.ReadFile(filepath.Join(runFixtureDir(), name+".strat"))
		if err != nil {
			t.Fatal(err)
		}
		for _, operation := range []string{"inspect", "run"} {
			t.Run(name+"/"+operation, func(t *testing.T) {
				capture := ordinaryExecutionCapture(t, operation, string(source), fixture, func(m map[string]any) {
					m["route"].(map[string]any)["symbol"] = "OFF-ROUTE"
					if operation == "inspect" {
						m["data"] = nil
					}
				})
				code := "ACCOUNTING_UNAVAILABLE"
				if strings.Contains(name, "clock-range") {
					code = "DEDICATED_REQUIRED"
				}
				plan, err := PrepareOrdinaryBacktest(capture)
				ordinaryInspectionFault(t, err, code, "admission")
				if plan == nil || plan.prepared != nil || plan.Preflight() != "refused" {
					t.Fatal("unsupported loop reached context or empty success")
				}
				if operation == "run" {
					run, _, err := RunOrdinaryBacktest(capture)
					ordinaryInspectionFault(t, err, code, "admission")
					if run != nil {
						t.Fatal("unsupported loop published an ordinary result")
					}
				}
			})
		}
	}
}

func TestOrdinaryInspectionRejectsNativeAlternateRoutes(t *testing.T) {
	fixture, source := ordinaryExecutionFixture(t, "sar-named-level-allow")
	capture := ordinaryExecutionCapture(t, "run", source, fixture, nil)
	cases := []struct {
		name   string
		mutate func(*RunRequest)
	}{
		{"window", func(r *RunRequest) { r.ExecutionWindow = &ExecutionWindow{} }},
		{"timed-calendar", func(r *RunRequest) { r.TimedCalendar = &TimedReturnCalendar{} }},
		{"force-route", func(r *RunRequest) { r.ForceRoute = true }},
		{"source-series", func(r *RunRequest) { r.SourceSeries = r.Series }},
		{"HTF-series", func(r *RunRequest) { r.HTFSeries = r.Series }},
		{"source-HTF-series", func(r *RunRequest) { r.SourceHTFSeries = r.Series }},
		{"higher-label", func(r *RunRequest) { r.HigherTimeframe = "1d" }},
		{"source-label", func(r *RunRequest) { r.SourceTimeframe = "1d" }},
		{"source-config", func(r *RunRequest) { r.Config["sourceTimeframe"] = "1d" }},
		{"HTF-config", func(r *RunRequest) { r.Config["htf"] = map[string]any{"mode": "notAgainst", "timeframe": "1d"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := ordinaryExecutionNativeRequest(t, capture)
			tc.mutate(&request)
			ordinaryInspectionFault(t, ordinaryRunCapability(request), "ACCOUNTING_UNAVAILABLE", "admission")
		})
	}
	request := ordinaryExecutionNativeRequest(t, capture)
	request.SourceTimeframe = request.Timeframe
	if err := ordinaryRunCapability(request); err != nil {
		t.Fatal("same-chart source label without companion data changed profile:", err)
	}
	for _, tc := range []struct {
		name string
		run  *PreparedRun
		code string
	}{
		{"nil", nil, "ACCOUNTING_UNAVAILABLE"},
		{"source-dispatch", &PreparedRun{c5: true}, "ACCOUNTING_UNAVAILABLE"},
		{"sequential-dispatch", &PreparedRun{sequentialFull: &sequentialPrepared{}}, "ACCOUNTING_UNAVAILABLE"},
		{"windowed", &PreparedRun{windowed: true}, "ACCOUNTING_UNAVAILABLE"},
		{"partial-start", &PreparedRun{execution: ExecutionBounds{TradeStart: 1}}, "ACCOUNTING_UNAVAILABLE"},
		{"off-route", &PreparedRun{offRoute: true}, "ROUTE_UNSUPPORTED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ordinaryInspectionFault(t, ordinaryPreparedCapability(tc.run), tc.code, "admission")
		})
	}
}

func TestOrdinaryInspectionOptionsAndNativeGuards(t *testing.T) {
	for _, tc := range []struct {
		name, fixture string
		mutate        func(*RunRequest)
	}{
		{"SAR", "sar-named-level-allow", func(r *RunRequest) { mapValue(r.Config, "parabolicSAR")["start"] = -1.0 }},
		{"PM-ATR", "family-price-momentum-atr-stop", func(r *RunRequest) { mapValue(r.Config, "stop")["atr"] = 0.0 }},
		{"Donchian", "family-donchian-breakout", func(r *RunRequest) { mapValue(r.Config, "donchianBreakout")["channelBars"] = 0.0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, source := ordinaryExecutionFixture(t, tc.fixture)
			fixture.Bars = fixture.Bars[:1]
			capture := ordinaryExecutionCapture(t, "run", source, fixture, nil)
			request := ordinaryExecutionNativeRequest(t, capture)
			tc.mutate(&request)
			ordinaryInspectionFault(t, ordinaryRunOptions(request), "OPTIONS_UNSUPPORTED", "admission")
			if _, err := PrepareRun(request); err == nil {
				t.Fatal("native invalid-config guard weakened")
			}
			if _, err := Run(request); err == nil {
				t.Fatal("native direct-run invalid-config guard weakened")
			}
		})
	}
	fixture, source := ordinaryExecutionFixture(t, "family-legacy-setup9")
	fixture.Bars = fixture.Bars[:1]
	for _, fill := range []string{"open", "nextOpen"} {
		for _, operation := range []string{"inspect", "run"} {
			t.Run("legacy-"+fill+"/"+operation, func(t *testing.T) {
				fixture.Costs.FillOn = fill
				capture := ordinaryExecutionCapture(t, operation, source, fixture, nil)
				plan, err := PrepareOrdinaryBacktest(capture)
				ordinaryInspectionFault(t, err, "OPTIONS_UNSUPPORTED", "admission")
				if plan == nil || plan.prepared != nil {
					t.Fatal("legacy cost guard ran after context allocation")
				}
				if _, err := PrepareRun(ordinaryExecutionNativeRequest(t, capture)); err == nil {
					t.Fatal("native legacy cost guard weakened")
				}
				if operation == "run" {
					run, _, err := RunOrdinaryBacktest(capture)
					ordinaryInspectionFault(t, err, "OPTIONS_UNSUPPORTED", "admission")
					if run != nil {
						t.Fatal("invalid legacy costs executed")
					}
				}
			})
		}
	}
}

func TestOrdinaryInspectionDedicatedPrecedenceAndNativeRangeGuard(t *testing.T) {
	fixture, _ := ordinaryExecutionFixture(t, "sar-named-level-allow")
	for _, name := range []string{"family-sequential-full-e1-long", "family-clock-range-breakout-ordinary-long", "family-timed-return"} {
		source, err := os.ReadFile(filepath.Join(runFixtureDir(), name+".strat"))
		if err != nil {
			t.Fatal(err)
		}
		for _, operation := range []string{"inspect", "run"} {
			t.Run(name+"/"+operation, func(t *testing.T) {
				capture := ordinaryExecutionCapture(t, operation, string(source), fixture, func(m map[string]any) {
					m["route"] = map[string]any{"symbol": "OFF-ROUTE", "timeframe": "unsupported"}
					if operation == "inspect" {
						m["data"] = nil
					} else {
						m["data"].(map[string]any)["bars"].([]any)[0].([]any)[2] = -100
					}
				})
				plan, err := PrepareOrdinaryBacktest(capture)
				ordinaryInspectionFault(t, err, "DEDICATED_REQUIRED", "admission")
				if plan == nil || plan.Classification().Kind != "dedicated" || plan.DataValidated() || plan.prepared != nil {
					t.Fatal("dedicated source reached data/route/context shortcuts")
				}
				if operation == "run" {
					run, _, err := RunOrdinaryBacktest(capture)
					ordinaryInspectionFault(t, err, "DEDICATED_REQUIRED", "admission")
					if run != nil {
						t.Fatal("dedicated source published an ordinary result")
					}
				}
			})
		}
	}
	for _, cfg := range []dsl.Config{
		{"setupType": string(dsl.FamilyRangeReversion)},
		{"setupType": "namedLevelSweep", "hidden": map[string]any{"Range-Reversion": false}},
		{"SETUPTYPE": "Range-Reversion"},
	} {
		request := RunRequest{Config: cfg, Symbol: "OFF-ROUTE", Timeframe: "unsupported"}
		ordinaryInspectionFault(t, ordinaryRunCapability(request), "DEDICATED_REQUIRED", "admission")
		if _, err := Run(request); err == nil || !strings.Contains(err.Error(), dsl.RangeReversionDedicatedRunnerRequired) {
			t.Fatalf("native range run guard lost: %v", err)
		}
		if _, err := PrepareRun(request); err == nil || !strings.Contains(err.Error(), dsl.RangeReversionDedicatedRunnerRequired) {
			t.Fatalf("native range preparation guard lost: %v", err)
		}
		if _, err := PrepareSharedRunContext(request); err == nil || !strings.Contains(err.Error(), dsl.RangeReversionDedicatedRunnerRequired) {
			t.Fatalf("native range shared-context guard lost: %v", err)
		}
		if _, err := SharedContextKey(request); err == nil || !strings.Contains(err.Error(), dsl.RangeReversionDedicatedRunnerRequired) {
			t.Fatalf("native range context-key guard lost: %v", err)
		}
	}
	// Preserve runner.go's source-level range reservation before legacy parsing
	// and before empty/off-route fixtures can be treated as a no-trade result.
	for _, selector := range []string{"type range-reversion", "type=range-reversion", "type range_reversion"} {
		source := strings.Replace(r11ReservationLegacySource, "setup {", "setup {\n"+selector, 1)
		if result, err := RunFixtureCase(RunFixture{}, source); err == nil || !strings.Contains(err.Error(), "range reversion") || len(result.Trades) != 0 {
			t.Fatalf("runner reserved selector %q escaped: %v", selector, err)
		}
	}
}

func TestOrdinaryInspectionNilAndParseFailure(t *testing.T) {
	plan, err := PrepareOrdinaryBacktest(nil)
	ordinaryInspectionFault(t, err, "INTERNAL", "transport")
	if plan != nil {
		t.Fatal("nil capture invented a plan")
	}
	run, plan, err := RunOrdinaryBacktest(nil)
	ordinaryInspectionFault(t, err, "ARGUMENTS", "transport")
	if run != nil || plan != nil {
		t.Fatal("nil run capture invented a result")
	}
	fixture, _ := ordinaryExecutionFixture(t, "sar-named-level-allow")
	for _, operation := range []string{"inspect", "run"} {
		capture := ordinaryExecutionCapture(t, operation, "dsl v999\n", fixture, nil)
		plan, err := PrepareOrdinaryBacktest(capture)
		ordinaryInspectionFault(t, err, "PARSE_REJECTED", "parse")
		if plan != nil {
			t.Fatal("parse failure invented config identity")
		}
		if operation == "run" {
			run, plan, err := RunOrdinaryBacktest(capture)
			ordinaryInspectionFault(t, err, "PARSE_REJECTED", "parse")
			if run != nil || plan != nil {
				t.Fatal("parse failure invented a run or plan")
			}
		}
	}
}

// A new separate dispatcher cannot silently acquire ordinary witness coverage.
// This pins engine routing structure, never an authored strategy/catalogue hash.
func TestOrdinaryInspectionDispatcherCoverageCannotSilentlyExpand(t *testing.T) {
	cases := []struct {
		file, name      string
		families, calls []string
	}{
		{"broker.go", "runWithFinalization", []string{"FamilyClockRangeBreakout"}, []string{"runClockRangeBreakout", "runRangeWithFinalization", "runSpecialSetup"}},
		{"broker_special_setups.go", "runSpecialSetup", []string{"FamilyDailyFlushFailure", "FamilyDownShockRebound", "FamilyIntraHourRunExhaustion", "FamilyWeekendExtremeFade"}, []string{"runDailyFlushFailure", "runDownShockRebound", "runIntraHourRunExhaustion", "runWeekendExtremeFade"}},
	}
	for _, tc := range cases {
		file, err := parser.ParseFile(token.NewFileSet(), tc.file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		var body *ast.BlockStmt
		for _, declaration := range file.Decls {
			if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == tc.name {
				body = fn.Body
			}
		}
		if body == nil {
			t.Fatal("missing broker dispatcher", tc.name)
		}
		var families, calls []string
		ast.Inspect(body, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok {
				if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == "dsl" && strings.HasPrefix(selector.Sel.Name, "Family") {
					families = append(families, selector.Sel.Name)
				}
			}
			if call, ok := node.(*ast.CallExpr); ok {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
					if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == "b" {
						calls = append(calls, selector.Sel.Name)
					}
				}
			}
			return true
		})
		sort.Strings(families)
		sort.Strings(calls)
		if !reflect.DeepEqual(families, tc.families) || !reflect.DeepEqual(calls, tc.calls) {
			t.Fatalf("dispatcher %s changed: families=%v calls=%v; renew ordinary witness admission review", tc.name, families, calls)
		}
	}
}
