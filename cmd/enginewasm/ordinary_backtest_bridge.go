package main

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report"
)

// These values are supplied by the artifact build with -ldflags -X. Empty
// development builds fail closed. A stamp is a claim by the build, not release
// authority: consumers must independently authenticate the artifact and receipt.
// No request, environment variable, source hash, or runtime setter supplies it.
var (
	ordinaryBacktestBuildTag             string
	ordinaryBacktestBuildSourceCommit    string
	ordinaryBacktestBuildSourceTree      string
	ordinaryBacktestBuildSchemaSetSHA256 string
)

func ordinaryBacktestProducer() engine.OrdinaryBacktestProducer {
	return engine.OrdinaryBacktestProducer{
		Repo: "spik3r/heisentick-strat", Tag: ordinaryBacktestBuildTag,
		SourceCommit: ordinaryBacktestBuildSourceCommit, SourceTree: ordinaryBacktestBuildSourceTree,
		Target: "js/wasm", GoVersion: runtime.Version(), SchemaSetSHA256: ordinaryBacktestBuildSchemaSetSHA256,
	}
}

type ordinaryBacktestReadiness struct {
	Parse     string `json:"parse"`
	Data      string `json:"data"`
	Preflight string `json:"preflight"`
	Execution string `json:"execution"`
}

type ordinaryBacktestSeries struct {
	Role      string `json:"role"`
	Timeframe string `json:"timeframe"`
	BarModel  string `json:"barModel"`
}

type ordinaryBacktestCapabilities struct {
	Classification string                   `json:"classification"`
	ContractID     *string                  `json:"contractId"`
	InputProfile   string                   `json:"inputProfile"`
	RequiredSeries []ordinaryBacktestSeries `json:"requiredSeries"`
	Accounting     string                   `json:"accounting"`
	Reason         *string                  `json:"reason"`
}

type ordinaryBacktestSourceFacts struct {
	Name      string   `json:"name"`
	SetupType string   `json:"setupType"`
	Features  []string `json:"features"`
}

type ordinaryBacktestInspection struct {
	Schema       string                              `json:"schema"`
	OK           bool                                `json:"ok"`
	RequestID    string                              `json:"requestId"`
	Operation    string                              `json:"operation"`
	Binding      engine.OrdinaryBacktestBinding      `json:"binding"`
	Readiness    ordinaryBacktestReadiness           `json:"readiness"`
	Capabilities ordinaryBacktestCapabilities        `json:"capabilities"`
	Diagnostics  []engine.OrdinaryBacktestDiagnostic `json:"diagnostics"`
	SourceFacts  *ordinaryBacktestSourceFacts        `json:"sourceFacts"`
}

type ordinaryBacktestExecutionResult struct {
	Status             string  `json:"status"`
	Profile            string  `json:"profile"`
	TerminalPolicy     string  `json:"terminalPolicy"`
	FinalizationReason string  `json:"finalizationReason"`
	BarCount           int     `json:"barCount"`
	FirstT             float64 `json:"firstT"`
	LastT              float64 `json:"lastT"`
}

type ordinaryBacktestReport struct {
	Schema      string                               `json:"schema"`
	OK          bool                                 `json:"ok"`
	RequestID   string                               `json:"requestId"`
	Operation   string                               `json:"operation"`
	Binding     engine.OrdinaryBacktestBinding       `json:"binding"`
	Execution   ordinaryBacktestExecutionResult      `json:"execution"`
	NativeRun   engine.RunResult                     `json:"nativeRun"`
	Account     report.OrdinaryBacktestAccount       `json:"account"`
	Completed   report.OrdinaryBacktestCompleted     `json:"completedPositions"`
	Drawdown    report.OrdinaryBacktestDrawdown      `json:"drawdown"`
	Equity      []report.OrdinaryBacktestEquityPoint `json:"equity"`
	Analyses    report.OrdinaryBacktestAnalyses      `json:"analyses"`
	Diagnostics []engine.OrdinaryBacktestDiagnostic  `json:"diagnostics"`
}

// Dependencies are per invocation, never caller-controlled globals. Tests can
// prove the non-executing inspection and stage-correct failure boundaries.
type ordinaryBacktestDependencies struct {
	prepare func(*engine.OrdinaryBacktestCapture) (*engine.OrdinaryBacktestPlan, error)
	run     func(*engine.OrdinaryBacktestCapture) (*engine.OrdinaryBacktestRun, *engine.OrdinaryBacktestPlan, error)
	project func(engine.OrdinaryBacktestFinancials) (report.OrdinaryBacktestProjection, error)
}

func ordinaryBacktestDefaultDependencies() ordinaryBacktestDependencies {
	return ordinaryBacktestDependencies{engine.PrepareOrdinaryBacktest, engine.RunOrdinaryBacktest, report.ProjectOrdinaryBacktestFinancials}
}

func inspectOrdinaryBacktestJSON(requestJSON, source string) string {
	return ordinaryBacktestJSON("inspect", requestJSON, source, ordinaryBacktestDefaultDependencies())
}

func runOrdinaryBacktestReportJSON(requestJSON, source string) string {
	return ordinaryBacktestJSON("run", requestJSON, source, ordinaryBacktestDefaultDependencies())
}

func ordinaryBacktestFault(code, stage string) engine.OrdinaryBacktestFault {
	return engine.OrdinaryBacktestFault{Code: "BACKTEST_" + code, Stage: stage, Message: "Ordinary backtest input or operation refused."}
}

func ordinaryBacktestFailure(operation string, capture *engine.OrdinaryBacktestCapture, producer engine.OrdinaryBacktestProducer, config *string, fault engine.OrdinaryBacktestFault) string {
	envelope, err := capture.ErrorEnvelope(operation, producer, config, fault)
	if err != nil {
		// Sanitize an invalid internal fault without dropping an existing binding
		// or computed digest. No parser message or panic value crosses the API.
		stage := "transport"
		if capture != nil {
			stage = "parse"
			if config != nil {
				stage = "admission"
				if operation == "run" {
					stage = "execution"
				}
			}
		}
		envelope, _ = capture.ErrorEnvelope(operation, producer, config, ordinaryBacktestFault("INTERNAL", stage))
	}
	raw, _ := json.Marshal(envelope) // All strings/fields above are bounded and finite.
	return string(raw)
}

func ordinaryBacktestTransportFailure(operation, code string) string {
	return ordinaryBacktestFailure(operation, nil, engine.OrdinaryBacktestProducer{}, nil, ordinaryBacktestFault(code, "transport"))
}

func ordinaryBacktestJSON(operation, requestJSON, source string, deps ordinaryBacktestDependencies) (response string) {
	stage := "transport"
	var capture *engine.OrdinaryBacktestCapture
	var config *string
	producer := ordinaryBacktestProducer()
	fail := func(err error) string {
		fault := ordinaryBacktestFault("INTERNAL", stage)
		var typed *engine.OrdinaryBacktestFault
		if errors.As(err, &typed) && typed != nil {
			fault = *typed
			fault.Message = "Ordinary backtest input or operation refused."
		}
		return ordinaryBacktestFailure(operation, capture, producer, config, fault)
	}
	defer func() {
		if recover() != nil {
			response = fail(nil)
		}
	}()
	c, err := engine.DecodeOrdinaryBacktestRequest(operation, requestJSON, source)
	if err != nil {
		return fail(err)
	}
	// ErrorEnvelope validates the whole producer shape. Validate before retaining
	// this capture: an incomplete build has no complete producer binding to emit.
	if _, err := c.ErrorEnvelope(operation, producer, nil, ordinaryBacktestFault("INTERNAL", "parse")); err != nil {
		return fail(nil)
	}
	capture, stage = c, "parse"
	var plan *engine.OrdinaryBacktestPlan
	var run *engine.OrdinaryBacktestRun
	if operation == "inspect" {
		plan, err = deps.prepare(capture)
	} else {
		// Run reparses and revalidates independently. No inspection receipt or
		// caller-supplied plan authorizes this exactly-once execution.
		run, plan, err = deps.run(capture)
	}
	if plan != nil && plan.ConfigIdentity() != "" {
		digest := plan.ConfigIdentity()
		config, stage = &digest, "admission"
	}
	if err != nil {
		return fail(err)
	}
	if plan == nil || config == nil {
		return fail(nil)
	}
	// Obtain the binding through the same capture validator as typed errors.
	bound, err := capture.ErrorEnvelope(operation, producer, config, ordinaryBacktestFault("INTERNAL", "admission"))
	if err != nil {
		return fail(err)
	}
	request := capture.Request()
	stage = "report"
	var document any
	if operation == "inspect" {
		class := plan.Classification()
		if class.Kind != "ordinary" || !ordinaryBacktestText(plan.SourceName(), 0, 256) || !ordinaryBacktestText(class.Family, 1, 128) ||
			(request.Data == nil && (plan.Preflight() != "not-evaluated" || plan.DataValidated())) ||
			(request.Data != nil && (plan.Preflight() != "admitted" || !plan.DataValidated())) {
			return fail(nil)
		}
		data := "absent"
		if plan.DataValidated() {
			data = "validated"
		}
		contract := engine.OrdinaryBacktestReportSchema
		document = ordinaryBacktestInspection{
			Schema: engine.OrdinaryBacktestInspectionSchema, OK: true, RequestID: request.RequestID, Operation: operation, Binding: *bound.Binding,
			Readiness: ordinaryBacktestReadiness{"accepted", data, plan.Preflight(), "not-run"},
			Capabilities: ordinaryBacktestCapabilities{Classification: "ordinary", ContractID: &contract, InputProfile: "original-chart-bars-v1",
				RequiredSeries: []ordinaryBacktestSeries{{"chart", request.Route.Timeframe, "original-ohlcv"}}, Accounting: "supported"},
			Diagnostics: []engine.OrdinaryBacktestDiagnostic{}, SourceFacts: &ordinaryBacktestSourceFacts{plan.SourceName(), class.Family, []string{}},
		}
	} else {
		if run == nil || request.Data == nil || plan.Preflight() != "admitted" || !plan.DataValidated() {
			return fail(nil)
		}
		native, financials := run.NativeResult(), run.Financials()
		if err := ordinaryBacktestValidateNative(request, native, financials); err != nil {
			return fail(err)
		}
		projection, err := deps.project(financials)
		if err != nil {
			// Projection is Go-owned and rejects nonfinite arithmetic as a whole.
			return fail(&engine.OrdinaryBacktestFault{Code: "BACKTEST_NONFINITE_OUTPUT", Stage: "report", Message: "Ordinary financial projection could not be completed."})
		}
		if len(projection.Equity) != len(request.Data.Bars) {
			return fail(nil)
		}
		for i, point := range projection.Equity {
			if point.Index != i || point.T != request.Data.Bars[i][0] {
				return fail(nil)
			}
		}
		finalization := "already-flat"
		if len(native.Trades) > 0 && native.Trades[len(native.Trades)-1].Reason == engine.ReasonEndOfTest {
			finalization = engine.ReasonEndOfTest
		}
		document = ordinaryBacktestReport{
			Schema: engine.OrdinaryBacktestReportSchema, OK: true, RequestID: request.RequestID, Operation: operation, Binding: *bound.Binding,
			Execution: ordinaryBacktestExecutionResult{"completed", "original-chart-bars-v1", "existing-native-finalization", finalization, len(request.Data.Bars), request.Data.FirstT, request.Data.LastT},
			NativeRun: native, Account: projection.Account, Completed: projection.CompletedPositions, Drawdown: projection.Drawdown,
			Equity: projection.Equity, Analyses: projection.Analyses, Diagnostics: []engine.OrdinaryBacktestDiagnostic{},
		}
	}
	raw, err := ordinaryBacktestEncode(document)
	if err != nil {
		return fail(err)
	}
	return string(raw)
}

func ordinaryBacktestText(s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	return utf8.ValidString(s) && n >= min && n <= max
}

func ordinaryBacktestValidateNative(request engine.OrdinaryBacktestRequest, native engine.RunResult, financials engine.OrdinaryBacktestFinancials) error {
	bad := func() error { f := ordinaryBacktestFault("INTERNAL", "report"); return &f }
	costs := request.Execution.Costs
	if request.Data == nil || native.Schema != "dsl-conformance-trades-v1" || native.Case != request.RequestID || native.StrategyID != request.StrategyID ||
		native.Symbol != request.Route.Symbol || native.Timeframe != request.Route.Timeframe || native.HigherTimeframe != "" ||
		native.RangeMethod != request.Execution.RangeMethod || native.Costs != (engine.Costs{FeePerUnit: costs.FeePerUnit, FillOn: costs.FillOn, Slippage: costs.Slippage, SlippageBps: costs.SlippageBps, StartEquity: costs.StartEquity}) ||
		native.SequentialFull != nil || native.TimedAudit != nil || len(native.ClockRangeDays) != 0 || native.TradeCount != len(native.Trades) || len(native.Trades) > 40000 ||
		financials.StartEquity != costs.StartEquity || len(financials.Marks) != len(request.Data.Bars) {
		return bad()
	}
	for i, mark := range financials.Marks {
		if mark.Index != i || mark.T != request.Data.Bars[i][0] {
			return bad()
		}
	}
	for _, trade := range native.Trades {
		if trade.EntryIndex < 0 || trade.ExitIndex < trade.EntryIndex || trade.ExitIndex >= len(request.Data.Bars) ||
			trade.EntryT != request.Data.Bars[trade.EntryIndex][0] || trade.ExitT != request.Data.Bars[trade.ExitIndex][0] ||
			(trade.Side != "long" && trade.Side != "short") || !ordinaryBacktestText(trade.Reason, 1, 128) ||
			!ordinaryBacktestText(trade.Tag, 1, 256) || trade.Rule != "" && !ordinaryBacktestText(trade.Rule, 1, 128) || trade.Meta == nil {
			return bad()
		}
		if err := ordinaryBacktestMetadata(reflect.ValueOf(trade.Meta), 0); err != nil {
			return err
		}
	}
	return nil
}

// Metadata is producer-created JSON, but still constrained by the public schema.
// Depth is checked before recursively visiting values, including cyclic maps.
func ordinaryBacktestMetadata(v reflect.Value, depth int) error {
	bad := func(code string) error { f := ordinaryBacktestFault(code, "report"); return &f }
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		return ordinaryBacktestMetadata(v.Elem(), depth)
	}
	if depth > engine.OrdinaryBacktestMaxDepth {
		return bad("OUTPUT_LIMIT")
	}
	switch v.Kind() {
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String || v.Len() > 128 {
			return bad("INTERNAL")
		}
		iter := v.MapRange()
		for iter.Next() {
			if !ordinaryBacktestText(iter.Key().String(), 0, 128) {
				return bad("INTERNAL")
			}
			if err := ordinaryBacktestMetadata(iter.Value(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Len() > 256 {
			return bad("INTERNAL")
		}
		for i := 0; i < v.Len(); i++ {
			if err := ordinaryBacktestMetadata(v.Index(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			return bad("NONFINITE_OUTPUT")
		}
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return bad("INTERNAL")
		}
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
	default:
		return bad("INTERNAL")
	}
	return nil
}

// The allocation preflight and public wire limit are deliberately different.
// A <=8 MiB/500,000-node document fits comfortably below this 128 MiB
// conservative allocation ceiling (six times string bytes plus scalar/key
// overhead). Its wider depth/node ceilings also accommodate omitted fields.
// The unchanged native marshaler then determines actual JSON; exact byte,
// container-depth and node ceilings are checked before publishing any bytes.
// No output is truncated and the conservative estimate cannot shrink the
// public 8 MiB allowance. Arbitrary custom marshalers cannot bypass the bound.
const ordinaryBacktestAllocationLimit = 128 << 20

func ordinaryBacktestEncode(value any) ([]byte, error) {
	b := ordinaryBacktestOutputBudget{remaining: ordinaryBacktestAllocationLimit}
	if err := b.visit(reflect.ValueOf(value), 0); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		f := ordinaryBacktestFault("INTERNAL", "report")
		return nil, &f
	}
	if len(raw) > engine.OrdinaryBacktestMaxOutputBytes {
		f := ordinaryBacktestFault("OUTPUT_LIMIT", "report")
		return nil, &f
	}
	if err := ordinaryBacktestCheckEncodedBounds(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func ordinaryBacktestCheckEncodedBounds(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	depth, nodes := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF && depth == 0 {
			return nil
		}
		if err != nil {
			f := ordinaryBacktestFault("INTERNAL", "report")
			return &f
		}
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter == '}' || delimiter == ']' {
				depth--
				continue
			}
			depth++
		}
		nodes++ // containers, keys and primitive values; closing tokens are not nodes.
		if nodes > engine.OrdinaryBacktestMaxNodes || depth > engine.OrdinaryBacktestMaxDepth {
			f := ordinaryBacktestFault("OUTPUT_LIMIT", "report")
			return &f
		}
		if number, ok := token.(json.Number); ok {
			value, err := strconv.ParseFloat(string(number), 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				f := ordinaryBacktestFault("NONFINITE_OUTPUT", "report")
				return &f
			}
		}
	}
}

type ordinaryBacktestOutputBudget struct{ remaining, nodes int }

func (b *ordinaryBacktestOutputBudget) add(n int) error {
	if n < 0 || n > b.remaining {
		f := ordinaryBacktestFault("OUTPUT_LIMIT", "report")
		return &f
	}
	b.remaining -= n
	return nil
}

func (b *ordinaryBacktestOutputBudget) visit(v reflect.Value, depth int) error {
	if v.IsValid() {
		marshaler := reflect.TypeOf((*json.Marshaler)(nil)).Elem()
		textMarshaler := reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
		typeOf := v.Type()
		if (typeOf.Implements(marshaler) || reflect.PointerTo(typeOf).Implements(marshaler) ||
			typeOf.Implements(textMarshaler) || reflect.PointerTo(typeOf).Implements(textMarshaler)) &&
			typeOf != reflect.TypeOf(engine.RunResult{}) && typeOf != reflect.TypeOf((*engine.RunResult)(nil)) {
			f := ordinaryBacktestFault("INTERNAL", "report")
			return &f
		}
	}
	if v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
		if !v.IsNil() {
			return b.visit(v.Elem(), depth)
		}
		v = reflect.Value{}
	}
	b.nodes++
	if b.nodes > 4*engine.OrdinaryBacktestMaxNodes {
		f := ordinaryBacktestFault("OUTPUT_LIMIT", "report")
		return &f
	}
	if !v.IsValid() {
		return b.add(4)
	}
	if v.Kind() == reflect.Struct || v.Kind() == reflect.Map || v.Kind() == reflect.Array || v.Kind() == reflect.Slice {
		depth++
		if depth > 2*engine.OrdinaryBacktestMaxDepth {
			f := ordinaryBacktestFault("OUTPUT_LIMIT", "report")
			return &f
		}
	}
	if err := b.add(32); err != nil {
		return err
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			f := ordinaryBacktestFault("INTERNAL", "report")
			return &f
		}
		if v.Len() > b.remaining/6 {
			f := ordinaryBacktestFault("OUTPUT_LIMIT", "report")
			return &f
		}
		return b.add(v.Len() * 6)
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			f := ordinaryBacktestFault("NONFINITE_OUTPUT", "report")
			return &f
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if field.PkgPath != "" || name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if err := b.add(len(name)*6 + 4); err != nil {
				return err
			}
			b.nodes++ // object keys are JSON nodes too
			if err := b.visit(v.Field(i), depth); err != nil {
				return err
			}
		}
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			f := ordinaryBacktestFault("INTERNAL", "report")
			return &f
		}
		iter := v.MapRange()
		for iter.Next() {
			if err := b.visit(iter.Key(), depth); err != nil {
				return err
			}
			if err := b.visit(iter.Value(), depth); err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if err := b.visit(v.Index(i), depth); err != nil {
				return err
			}
		}
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
	default:
		f := ordinaryBacktestFault("INTERNAL", "report")
		return &f
	}
	return nil
}
