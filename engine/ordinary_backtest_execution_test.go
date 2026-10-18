package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// These are new ordinary requests containing only the original primary rows.
// The full conformance fixtures (including their HTF companions) remain intact;
// neither their golden results nor a full-fixture equivalence is asserted here.
func ordinaryExecutionFixture(t *testing.T, name string) (RunFixture, string) {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), name+".fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), name+".strat"))
	if err != nil {
		t.Fatal(err)
	}
	return fixture, string(source)
}

func ordinaryExecutionCapture(t *testing.T, operation, source string, fixture RunFixture, mutate func(map[string]any)) *OrdinaryBacktestCapture {
	t.Helper()
	raw := ordinaryTestRequest(t, operation, func(m map[string]any) {
		m["requestId"] = "ordinary.primary-series.1"
		m["strategyId"] = "ordinary.primary-series"
		m["route"] = map[string]any{"symbol": fixture.Symbol, "timeframe": fixture.Timeframe}
		costs := fixture.Costs.normalized()
		m["execution"].(map[string]any)["costs"] = map[string]any{
			"feePerUnit": costs.FeePerUnit, "fillOn": costs.FillOn,
			"slippage": costs.Slippage, "slippageBps": costs.SlippageBps, "startEquity": costs.StartEquity,
		}
		rows := make([]any, len(fixture.Bars))
		for i, b := range fixture.Bars {
			rows[i] = []any{b.T, b.O, b.H, b.L, b.C, b.V}
		}
		if len(rows) == 0 {
			m["data"] = nil
		} else {
			duration, ok := dsl.SequentialFullTimeframeMS(fixture.Timeframe)
			if !ok {
				t.Fatalf("unsupported test timeframe %q", fixture.Timeframe)
			}
			d := m["data"].(map[string]any)
			d["bars"], d["firstT"], d["lastT"] = rows, fixture.Bars[0].T, fixture.Bars[len(fixture.Bars)-1].T
			d["closedThroughT"] = fixture.Bars[len(fixture.Bars)-1].T + float64(duration)
			d["origin"].(map[string]any)["datasetId"] = "new-ordinary-primary-only"
		}
		if mutate != nil {
			mutate(m)
		}
	})
	capture, err := DecodeOrdinaryBacktestRequest(operation, raw, source)
	if err != nil {
		t.Fatal(err)
	}
	return capture
}

// Independently construct the native request from the captured public fields.
// Do not reuse ordinaryRunRequest, which is one of the functions under test.
func ordinaryExecutionNativeRequest(t *testing.T, c *OrdinaryBacktestCapture) RunRequest {
	t.Helper()
	parsed, err := dsl.Parse(c.Source())
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse native control: %v %v", err, parsed.Errors)
	}
	request := c.Request()
	var bars []marketdata.Bar
	if request.Data != nil {
		bars = make([]marketdata.Bar, len(request.Data.Bars))
		for i, row := range request.Data.Bars {
			bars[i] = marketdata.Bar{T: row[0], O: row[1], H: row[2], L: row[3], C: row[4], V: row[5]}
		}
	}
	costs := request.Execution.Costs
	return RunRequest{Config: parsed.Config, Series: marketdata.SeriesFromBars(bars),
		StrategyID: request.StrategyID, Symbol: request.Route.Symbol, Timeframe: request.Route.Timeframe,
		RangeMethod: request.Execution.RangeMethod,
		Costs:       Costs{FeePerUnit: costs.FeePerUnit, FillOn: costs.FillOn, Slippage: costs.Slippage, SlippageBps: costs.SlippageBps, StartEquity: costs.StartEquity}}
}

func ordinaryExecutionParity(t *testing.T, c *OrdinaryBacktestCapture) (*OrdinaryBacktestRun, *OrdinaryBacktestPlan) {
	t.Helper()
	request := ordinaryExecutionNativeRequest(t, c)
	native, err := PrepareRun(request)
	if err != nil {
		t.Fatal("native control preparation:", err)
	}
	// RunRequest has no case field. Bind the control envelope to the same case
	// without changing the request, source, input series or execution settings.
	native.fixture.Case = c.Request().RequestID
	want, err := native.RunChecked(request.Costs)
	if err != nil {
		t.Fatal("native control execution:", err)
	}
	if native.broker.paperAccounting != nil {
		t.Fatal("native control unexpectedly attached an ordinary observer")
	}
	got, plan, err := RunOrdinaryBacktest(c)
	if err != nil || got == nil || plan == nil {
		t.Fatalf("ordinary execution: run=%v plan=%v err=%v", got, plan, err)
	}
	if !ordinaryStateEqual(got.NativeResult(), want) {
		t.Fatalf("same-input native result changed: got trades=%d want=%d", got.NativeResult().TradeCount, want.TradeCount)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(got.NativeResult())
	if err != nil || !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("same-input native serialization changed: %v", err)
	}
	if plan.Preflight() != "admitted" || !plan.DataValidated() || plan.Classification().Kind != "ordinary" || !ordinaryHex(plan.ConfigIdentity(), 64) {
		t.Fatal("execution did not retain admitted source/data facts")
	}
	if plan.prepared.broker.paperAccounting == nil || plan.prepared.broker.paperAccounting.ordinaryState == nil {
		t.Fatal("execution did not attach its witness to the actual broker")
	}
	if len(plan.prepared.broker.trades) != len(native.broker.trades) {
		t.Fatal("observer changed the raw native exit count")
	}
	for i, trade := range native.broker.trades {
		if !ordinaryStateEqual(plan.prepared.broker.trades[i], trade) {
			t.Fatalf("observer changed unrounded native exit %d", i)
		}
	}
	if !paperSame(got.Financials().TerminalRealized, native.broker.realized) {
		t.Fatal("observer changed raw native realized arithmetic")
	}
	ordinaryExecutionWitness(t, c, got)
	return got, plan
}

func ordinaryExecutionWitness(t *testing.T, c *OrdinaryBacktestCapture, run *OrdinaryBacktestRun) {
	t.Helper()
	bars, financials, states := c.Request().Data.Bars, run.Financials(), run.states
	if len(financials.Marks) != len(bars) || len(states) < 2 || states[0].Boundary != "initial" || states[len(states)-1].Boundary != "terminal" {
		t.Fatal("missing complete retained witness")
	}
	stages := []string{"before-pending-exits", "after-pending-exits", "before-pending-entries", "after-pending-entries", "before-limit-orders", "after-limit-orders", "before-on-bar", "after-on-bar", "original-close"}
	counts := make([]map[string]int, len(bars))
	for i := range counts {
		counts[i] = make(map[string]int)
	}
	financialAfter := 0
	for stateIndex, state := range states[1 : len(states)-1] {
		if state.Index < 0 || state.Index >= len(bars) || !paperSame(state.T, bars[state.Index][0]) {
			t.Fatal("state did not refer to an original captured row")
		}
		counts[state.Index][state.Boundary]++
		if state.Boundary == "after-entry" || state.Boundary == "after-partial" || state.Boundary == "after-exit" {
			if financialAfter >= len(financials.Events) {
				t.Fatal("witness invented a financial mutation")
			}
			event := financials.Events[financialAfter]
			before := states[stateIndex]
			if before.Boundary != "before-"+event.Kind || before.Index != event.Index || !paperSame(before.Realized, event.RealizedBefore) {
				t.Fatal("financial event missing its immediate contemporaneous before state")
			}
			if state.Boundary != "after-"+event.Kind || state.Index != event.Index || !paperSame(state.Realized, event.RealizedAfter) {
				t.Fatal("financial event detached from contemporaneous state")
			}
			financialAfter++
		}
	}
	if financialAfter != len(financials.Events) {
		t.Fatal("financial events missing contemporaneous after states")
	}
	for i, mark := range financials.Marks {
		if mark.Index != i || !paperSame(mark.T, bars[i][0]) {
			t.Fatal("marks sorted, dropped or invented original rows")
		}
		for _, boundary := range stages {
			if counts[i][boundary] != 1 {
				t.Fatalf("bar %d: %s count = %d, want 1", i, boundary, counts[i][boundary])
			}
		}
	}
	last := states[len(states)-1]
	if last.HasPosition || last.Index != len(bars)-1 || last.TradeCount != run.NativeResult().TradeCount ||
		!paperSame(last.Realized, financials.TerminalRealized) || !paperSame(financials.LastFlatRealized, financials.TerminalRealized) ||
		financials.TerminalUnrealized != 0 || financials.OpenEntryFee != 0 || financials.Marks[len(bars)-1].Unrealized != 0 {
		t.Fatal("ordinary final liquidation or terminal binding changed")
	}
}

func TestOrdinaryExecutionEditedSourceSameInputNativeParity(t *testing.T) {
	cases := []struct{ fixture, old, replacement string }{
		{"sar-named-level-allow", "risk 200 USD", "risk 137 USD"},
		{"family-price-momentum-atr-stop", "stop 2 ATR", "stop 1.7 ATR"},
		{"family-donchian-breakout", "channel 55 candles", "channel 41 candles"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			fixture, original := ordinaryExecutionFixture(t, tc.fixture)
			if !strings.Contains(original, tc.old) {
				t.Fatal("edited-source test operand missing")
			}
			for _, edited := range []bool{false, true} {
				t.Run(fmt.Sprintf("edited=%t", edited), func(t *testing.T) {
					source := original
					if edited {
						source = strings.Replace(original, tc.old, tc.replacement, 1)
						source = strings.Replace(source, "strategy \"", "strategy \"Unregistered ordinary edit: ", 1)
					}
					capture := ordinaryExecutionCapture(t, "run", source, fixture, nil)
					run, _ := ordinaryExecutionParity(t, capture)
					if run.NativeResult().TradeCount == 0 {
						t.Fatal("actual trade-producing ordinary primary-series case became a zero-trade shortcut")
					}
					t.Logf("primary-only rows=%d trades=%d states=%d events=%d", len(fixture.Bars), run.NativeResult().TradeCount, len(run.states), len(run.Financials().Events))
				})
			}
		})
	}
}

func TestOrdinaryExecutionFillCostsAndZeroTradeParity(t *testing.T) {
	fixture, source := ordinaryExecutionFixture(t, "sar-named-level-allow")
	for _, fill := range []string{"close", "open", "nextOpen"} {
		t.Run(fill, func(t *testing.T) {
			fixture.Costs = Costs{StartEquity: 7137.25, FeePerUnit: .13, FillOn: fill, Slippage: .02, SlippageBps: .7}
			capture := ordinaryExecutionCapture(t, "run", strings.Replace(source, "risk 200 USD", "risk 137 USD", 1), fixture, nil)
			run, _ := ordinaryExecutionParity(t, capture)
			if run.NativeResult().TradeCount == 0 || run.Financials().EntryFees <= 0 || run.Financials().ExitFees <= 0 {
				t.Fatal("cost-bearing native trade coverage missing")
			}
			if run.NativeResult().Costs.FillOn != fill || !paperSame(run.Financials().StartEquity, 7137.25) {
				t.Fatal("explicit cost spelling or equity was defaulted")
			}
		})
	}
	for _, name := range []string{"sar-named-level-allow", "family-price-momentum-atr-stop", "family-donchian-breakout"} {
		t.Run("warmup/"+name, func(t *testing.T) {
			fixture, source := ordinaryExecutionFixture(t, name)
			fixture.Bars = fixture.Bars[:1]
			capture := ordinaryExecutionCapture(t, "run", source, fixture, nil)
			run, _ := ordinaryExecutionParity(t, capture)
			financials := run.Financials()
			if run.NativeResult().TradeCount != 0 || len(financials.Events) != 0 || len(financials.Marks) != 1 || len(run.states) != 11 ||
				financials.TerminalRealized != 0 || !paperSame(financials.Marks[0].Equity, financials.StartEquity) {
				t.Fatal("warmup-only run skipped evidence or invented financial activity")
			}
		})
	}
}

func TestOrdinaryExecutionCopiesRetainedNativeAndFinancials(t *testing.T) {
	fixture, source := ordinaryExecutionFixture(t, "sar-named-level-allow")
	capture := ordinaryExecutionCapture(t, "run", source, fixture, nil)
	run, plan := ordinaryExecutionParity(t, capture)
	wantNative, wantFinancials := run.NativeResult(), run.Financials()
	exposed, financials := run.NativeResult(), run.Financials()
	exposed.Case, exposed.Costs.StartEquity, exposed.Trades[0].Size = "changed", -1, -1
	if exposed.Trades[0].Meta != nil {
		exposed.Trades[0].Meta["caller-mutation"] = true
	}
	financials.Events[0].Size, financials.Marks[0].Equity, financials.StartEquity = -1, -1, -1
	if !ordinaryStateEqual(run.NativeResult(), wantNative) || !ordinaryStateEqual(run.Financials(), wantFinancials) {
		t.Fatal("public accessors expose retained native or financial state")
	}
	// The prepared runner and its collector are not the retained immutable run.
	plan.prepared.trades[0].Size = -2
	plan.prepared.broker.realized = -3
	plan.prepared.broker.paperAccounting.ordinaryState.states[0].Realized = -4
	if !ordinaryStateEqual(run.NativeResult(), wantNative) || !ordinaryStateEqual(run.Financials(), wantFinancials) || run.states[0].Realized != 0 {
		t.Fatal("retained result aliases mutable broker or collector memory")
	}
	// Exercise nested typed metadata, signed zero, and nil/empty distinctions
	// separately from the genuine strategy path above.
	synthetic := &OrdinaryBacktestRun{native: RunResult{Trades: []Trade{{NoStop: true, NoTarget: true,
		Meta: TradeMeta{"nested": map[string]any{"values": []float64{math.Copysign(0, -1), 2}}, "nil": []string(nil), "empty": []string{}}}}}}
	copy := synthetic.NativeResult()
	copy.Trades[0].Meta["nested"].(map[string]any)["values"].([]float64)[1] = 9
	again := synthetic.NativeResult().Trades[0]
	values := again.Meta["nested"].(map[string]any)["values"].([]float64)
	if values[1] != 2 || math.Float64bits(values[0]) != 1<<63 || again.Meta["nil"].([]string) != nil || again.Meta["empty"].([]string) == nil || !again.NoStop || !again.NoTarget {
		t.Fatal("native accessor changed typed metadata, float bits or absent bracket flags")
	}
}

func TestOrdinaryExecutionRejectsAlternateObserverLifecycles(t *testing.T) {
	for _, mode := range []string{"scheduled", "source-capture", "partial-start", "nonliquidating", "windowed"} {
		t.Run(mode, func(t *testing.T) {
			p, capture := ordinaryStatePair(t, []float64{100, 101, 102}, "close")
			b := &p.observed
			switch mode {
			case "scheduled":
				b.runScheduled(nil, nil)
			case "source-capture":
				b.runCapturedSource()
			case "partial-start":
				b.runRangeWithFinalization(1, true)
			case "nonliquidating":
				b.openPosition(sideLong, 100, order{Size: 1, HasSize: true, NoStop: true, NoTarget: true}, 0)
				b.runRangeWithFinalization(0, false)
			case "windowed":
				b.setExecutionWindow(ExecutionBounds{TradeStart: 1, TradeEnd: 2})
				b.run()
			}
			ordinaryStateRefused(t, capture)
		})
	}
}

func TestOrdinaryExecutionPartialFinalizationAndPendingLastBar(t *testing.T) {
	fixture, source := ordinaryExecutionFixture(t, "sar-named-level-allow")
	source = strings.Replace(source, "target 0.5R", "target 100R", 1)
	source = strings.Replace(source, "risk 200 USD", "risk 137 USD", 1)
	source += "\nmanagement { partial 70% at 0.25R move stop to breakeven }\n"
	fixture.Costs.FeePerUnit = .13
	capture := ordinaryExecutionCapture(t, "run", source, fixture, nil)
	run, _ := ordinaryExecutionParity(t, capture)
	financials := run.Financials()
	if len(financials.Events) != 3 || financials.Events[0].Kind != "entry" || financials.Events[1].Kind != "partial" || financials.Events[2].Kind != "exit" {
		t.Fatalf("actual edited-source partial lifecycle missing: %+v", financials.Events)
	}
	partial := ordinaryStateFind(t, run.states, "after-partial", financials.Events[1].Index)
	if !partial.HasPosition || !partial.Position.PartialTaken || !paperSame(partial.Position.Size, financials.Events[2].Size) {
		t.Fatal("final exit size was not the contemporaneous actual post-partial remainder")
	}
	lastIndex := len(fixture.Bars) - 1
	originalClose := ordinaryStateFind(t, run.states, "original-close", lastIndex)
	terminal := run.states[len(run.states)-1]
	trades := run.NativeResult().Trades
	if !originalClose.HasPosition || terminal.HasPosition || trades[len(trades)-1].Reason != ReasonEndOfTest ||
		financials.Marks[lastIndex].Unrealized != 0 || !paperSame(financials.Marks[lastIndex].Realized, financials.TerminalRealized) {
		t.Fatal("provisional original close was not replaced by actual final liquidation")
	}
	// End the newly constructed request on the known signal bar. A next-open
	// order must stay pending; ordinary finalization must not invent a fill.
	fixture, source = ordinaryExecutionFixture(t, "sar-named-level-allow")
	fixture.Bars = fixture.Bars[:11]
	fixture.Costs.FillOn = "nextOpen"
	capture = ordinaryExecutionCapture(t, "run", source, fixture, nil)
	run, _ = ordinaryExecutionParity(t, capture)
	terminal = run.states[len(run.states)-1]
	if run.NativeResult().TradeCount != 0 || len(run.Financials().Events) != 0 || len(terminal.PendingOrders) != 1 || terminal.HasPosition {
		t.Fatal("pending last-bar signal lost its real queue or invented a financial fill")
	}
}

func TestOrdinaryExecutionExactMaximumOriginalBars(t *testing.T) {
	fixture, source := ordinaryExecutionFixture(t, "sar-named-level-allow")
	// Invented flat prices exercise the exact structural ceiling without
	// claiming that this ceiling is safe on every device or target.
	bars := make([]marketdata.Bar, OrdinaryBacktestMaxBars)
	for i := range bars {
		bars[i] = marketdata.Bar{T: fixture.Bars[0].T + float64(i)*14400000, O: 100, H: 101, L: 99, C: 100, V: 1}
	}
	fixture.Bars = bars
	inspect := ordinaryExecutionCapture(t, "inspect", source, fixture, nil)
	plan, err := PrepareOrdinaryBacktest(inspect)
	if err != nil || plan == nil || plan.Preflight() != "admitted" || !plan.DataValidated() || plan.prepared.broker.paperAccounting != nil {
		t.Fatalf("exact maximum inspection: plan=%v err=%v", plan, err)
	}
	capture := ordinaryExecutionCapture(t, "run", source, fixture, nil)
	run, _ := ordinaryExecutionParity(t, capture)
	if run.NativeResult().TradeCount != 0 || len(run.Financials().Marks) != OrdinaryBacktestMaxBars || len(run.states) != 9*OrdinaryBacktestMaxBars+2 {
		t.Fatal("exact maximum zero-trade request truncated input or skipped lifecycle evidence")
	}
	t.Logf("exact maximum original rows=%d retained states=%d", len(bars), len(run.states))
}
