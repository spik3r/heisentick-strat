package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The V1 ordinary surfaces are additive. These declarations do not register an
// export, authorize execution, or attest to a released producer artifact.
const (
	OrdinaryBacktestRequestSchema       = "strat-ordinary-backtest-request-v1"
	OrdinaryBacktestInspectionSchema    = "strat-ordinary-backtest-inspection-v1"
	OrdinaryBacktestReportSchema        = "strat-ordinary-backtest-report-v1"
	OrdinaryBacktestErrorSchema         = "strat-ordinary-backtest-error-v1"
	OrdinaryBacktestReportingBasis      = "ordinary-logical-completed-close-mtm-v1"
	OrdinaryBacktestMaxSourceBytes      = 262144
	OrdinaryBacktestMaxInputBytes       = 2097152
	OrdinaryBacktestMaxTextRequestBytes = 65536
	OrdinaryBacktestMaxOutputBytes      = 8388608
	OrdinaryBacktestMaxBars             = 10000
	OrdinaryBacktestMaxDepth            = 32
	OrdinaryBacktestMaxNodes            = 500000
	ordinaryBacktestMaxInteger          = 9007199254740991
)

type OrdinaryBacktestIdentity struct {
	SHA256    string `json:"sha256"`
	UTF8Bytes int    `json:"utf8Bytes"`
}
type OrdinaryBacktestRoute struct {
	Symbol    string `json:"symbol"`
	Timeframe string `json:"timeframe"`
}

// Unlike native Costs, the binding always preserves explicit zero slippageBps.
type OrdinaryBacktestCosts struct {
	FeePerUnit  float64 `json:"feePerUnit"`
	FillOn      string  `json:"fillOn"`
	Slippage    float64 `json:"slippage"`
	SlippageBps float64 `json:"slippageBps"`
	StartEquity float64 `json:"startEquity"`
}
type OrdinaryBacktestExecution struct {
	RangeMethod string                `json:"rangeMethod"`
	Costs       OrdinaryBacktestCosts `json:"costs"`
	Params      struct{}              `json:"params"`
	ForceRoute  bool                  `json:"forceRoute"`
	ChartType   string                `json:"chartType"`
}
type OrdinaryBacktestOrigin struct {
	DatasetID         string  `json:"datasetId"`
	HistoryGeneration float64 `json:"historyGeneration"`
	Provenance        string  `json:"provenance"`
}
type OrdinaryBacktestData struct {
	Schema         string                 `json:"schema"`
	Profile        string                 `json:"profile"`
	Bars           [][6]float64           `json:"bars"`
	FirstT         float64                `json:"firstT"`
	LastT          float64                `json:"lastT"`
	ClosedThroughT float64                `json:"closedThroughT"`
	Origin         OrdinaryBacktestOrigin `json:"origin"`
}
type OrdinaryBacktestRequest struct {
	Schema         string                    `json:"schema"`
	RequestID      string                    `json:"requestId"`
	Operation      string                    `json:"operation"`
	StrategyID     string                    `json:"strategyId"`
	Route          OrdinaryBacktestRoute     `json:"route"`
	Execution      OrdinaryBacktestExecution `json:"execution"`
	ReportingBasis string                    `json:"reportingBasis"`
	Data           *OrdinaryBacktestData     `json:"data"`
}

// Capture is created only after the complete strict request is valid. Its
// mutable slices are private; accessors return copies. No caller receipt is read.
type OrdinaryBacktestCapture struct {
	request             OrdinaryBacktestRequest
	source, requestJSON string
	sourceID, requestID OrdinaryBacktestIdentity
	dataID              *OrdinaryBacktestIdentity
}

func (c *OrdinaryBacktestCapture) Request() OrdinaryBacktestRequest {
	r := c.request
	if r.Data != nil {
		d := *r.Data
		d.Bars = append([][6]float64(nil), d.Bars...)
		r.Data = &d
	}
	return r
}
func (c *OrdinaryBacktestCapture) Source() string                            { return c.source }
func (c *OrdinaryBacktestCapture) RequestJSON() string                       { return c.requestJSON }
func (c *OrdinaryBacktestCapture) SourceIdentity() OrdinaryBacktestIdentity  { return c.sourceID }
func (c *OrdinaryBacktestCapture) RequestIdentity() OrdinaryBacktestIdentity { return c.requestID }
func (c *OrdinaryBacktestCapture) DataIdentity() *OrdinaryBacktestIdentity {
	if c.dataID == nil {
		return nil
	}
	d := *c.dataID
	return &d
}
func ordinaryIdentity(raw []byte) OrdinaryBacktestIdentity {
	sum := sha256.Sum256(raw)
	return OrdinaryBacktestIdentity{hex.EncodeToString(sum[:]), len(raw)}
}

// Fault contains safe fixed diagnostics, never raw source, JSON or parser text.
// Bound public envelopes are constructed only after producer/config identity is
// available; a strict decoder failure has no trustworthy partial request ID.
type OrdinaryBacktestFault struct {
	Code     string  `json:"code"`
	Stage    string  `json:"stage"`
	Field    *string `json:"field"`
	BarIndex *int    `json:"barIndex"`
	Message  string  `json:"message"`
}

func (f *OrdinaryBacktestFault) Error() string { return f.Code + ": " + f.Message }
func ordinaryFault(code, stage, field string) *OrdinaryBacktestFault {
	var p *string
	if field != "" {
		p = &field
	}
	return &OrdinaryBacktestFault{Code: "BACKTEST_" + code, Stage: stage, Field: p, Message: "Ordinary backtest input or operation refused."}
}
func ordinarySchema(field string) *OrdinaryBacktestFault {
	return ordinaryFault("SCHEMA", "transport", field)
}
func ordinaryOperation(s string) bool { return s == "inspect" || s == "run" }
func ordinaryID(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for i, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			continue
		}
		if i == 0 || !strings.ContainsRune("._:-", rune(c)) {
			return false
		}
	}
	return true
}
func ordinaryString(raw json.RawMessage, min, max int) (string, error) {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil || utf8.RuneCountInString(s) < min || utf8.RuneCountInString(s) > max {
		return "", ordinarySchema("")
	}
	return s, nil
}
func ordinaryNumber(raw json.RawMessage, min float64, integer bool) (float64, error) {
	if len(raw) == 0 || raw[0] != '-' && (raw[0] < '0' || raw[0] > '9') {
		return 0, ordinarySchema("")
	}
	var n float64
	if json.Unmarshal(raw, &n) != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < min || integer && !ordinaryExactSafeInteger(string(raw)) {
		return 0, ordinarySchema("")
	}
	return n, nil
}

// Check the decimal value before float64 rounding (for example, the token
// 9007199254740991.1 must not become an admitted integer). Exponents saturate
// against token length, so huge zero exponents need no big-integer allocation.
func ordinaryExactSafeInteger(token string) bool {
	negative := strings.HasPrefix(token, "-")
	token = strings.TrimPrefix(token, "-")
	mantissa, exponent := token, 0
	if i := strings.IndexAny(token, "eE"); i >= 0 {
		mantissa = token[:i]
		e := token[i+1:]
		sign := 1
		if strings.HasPrefix(e, "-") {
			sign = -1
			e = e[1:]
		} else {
			e = strings.TrimPrefix(e, "+")
		}
		cap := len(token) + 20
		for _, c := range e {
			if exponent > cap/10 {
				exponent = cap
				break
			}
			exponent = exponent*10 + int(c-'0')
			if exponent > cap {
				exponent = cap
				break
			}
		}
		exponent *= sign
	}
	fraction := 0
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		fraction = len(mantissa) - i - 1
		mantissa = mantissa[:i] + mantissa[i+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return true
	}
	if negative {
		return false
	}
	significant := strings.TrimRight(digits, "0")
	shift := exponent - fraction + len(digits) - len(significant)
	if shift < 0 || len(significant)+shift > 16 {
		return false
	}
	return len(significant)+shift < 16 || significant+strings.Repeat("0", shift) <= "9007199254740991"
}

func ordinaryObject(raw json.RawMessage, fields ...string) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &m) != nil {
		return nil, ordinarySchema("")
	}
	allowed := map[string]bool{}
	for _, k := range fields {
		allowed[k] = true
	}
	for k := range m {
		if !allowed[k] {
			return nil, ordinaryFault("UNKNOWN_FIELD", "transport", "")
		}
	}
	for _, k := range fields {
		if _, ok := m[k]; !ok {
			return nil, ordinarySchema(k)
		}
	}
	return m, nil
}
func ordinaryConst(raw json.RawMessage, want string) bool {
	s, e := ordinaryString(raw, 0, 1024)
	return e == nil && s == want
}

// encoding/json replaces lone surrogate escapes. Reject them before decoding;
// escaped-backslash text is not interpreted as a Unicode escape.
func ordinaryJSONUnicode(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	quoted := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !quoted {
			if c == '"' {
				quoted = true
			}
			continue
		}
		if c == '"' {
			quoted = false
			continue
		}
		if c != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		v, e := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if e != nil {
			return false
		}
		i += 4
		if v >= 0xdc00 && v <= 0xdfff {
			return false
		}
		if v >= 0xd800 && v <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			w, e := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if e != nil || w < 0xdc00 || w > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

// Depth counts containers only (root container = 1). Nodes count values and
// decoded object member names. Duplicate names compare after JSON unescaping.
func ordinaryJSONScan(raw []byte, maxBytes, maxDepth, maxNodes int) error {
	if len(raw) > maxBytes {
		return ordinaryFault("INPUT_LIMIT", "transport", "")
	}
	if !ordinaryJSONUnicode(raw) {
		return ordinaryFault("JSON_ENCODING", "transport", "")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	nodes := 0
	bump := func() error {
		nodes++
		if nodes > maxNodes {
			return ordinaryFault("INPUT_LIMIT", "transport", "")
		}
		return nil
	}
	var value func(int) error
	value = func(depth int) error {
		if e := bump(); e != nil {
			return e
		}
		tok, e := dec.Token()
		if e != nil {
			return ordinaryFault("JSON_ENCODING", "transport", "")
		}
		if n, ok := tok.(json.Number); ok {
			f, e := strconv.ParseFloat(string(n), 64)
			if e != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return ordinarySchema("")
			}
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		depth++
		if depth > maxDepth {
			return ordinaryFault("INPUT_LIMIT", "transport", "")
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				if e := bump(); e != nil {
					return e
				}
				k, e := dec.Token()
				s, ok := k.(string)
				if e != nil || !ok {
					return ordinaryFault("JSON_ENCODING", "transport", "")
				}
				if seen[s] {
					return ordinaryFault("DUPLICATE_KEY", "transport", "")
				}
				seen[s] = true
				if e := value(depth); e != nil {
					return e
				}
			}
			tok, e = dec.Token()
			if e != nil || tok != json.Delim('}') {
				return ordinaryFault("JSON_ENCODING", "transport", "")
			}
		case '[':
			for dec.More() {
				if e := value(depth); e != nil {
					return e
				}
			}
			tok, e = dec.Token()
			if e != nil || tok != json.Delim(']') {
				return ordinaryFault("JSON_ENCODING", "transport", "")
			}
		default:
			return ordinaryFault("JSON_ENCODING", "transport", "")
		}
		return nil
	}
	if e := value(0); e != nil {
		return e
	}
	if _, e := dec.Token(); e != io.EOF {
		return ordinaryFault("JSON_ENCODING", "transport", "")
	}
	return nil
}

// DecodeOrdinaryBacktestRequest is transport capture only. It does not parse DSL,
// run a strategy, default costs, or declare the data/profile execution-ready.
func DecodeOrdinaryBacktestRequest(operation, requestJSON, source string) (*OrdinaryBacktestCapture, error) {
	if !ordinaryOperation(operation) {
		return nil, ordinaryFault("ARGUMENTS", "transport", "")
	}
	if !utf8.ValidString(source) {
		return nil, ordinaryFault("SOURCE_ENCODING", "transport", "")
	}
	if len(source) > OrdinaryBacktestMaxSourceBytes || len(requestJSON) > OrdinaryBacktestMaxInputBytes-len(source) {
		return nil, ordinaryFault("INPUT_LIMIT", "transport", "")
	}
	raw := []byte(requestJSON)
	if e := ordinaryJSONScan(raw, OrdinaryBacktestMaxInputBytes, OrdinaryBacktestMaxDepth, OrdinaryBacktestMaxNodes); e != nil {
		return nil, e
	}
	var probe map[string]json.RawMessage
	_ = json.Unmarshal(raw, &probe)
	if operation == "run" && (len(probe["data"]) == 0 || bytes.Equal(probe["data"], []byte("null"))) {
		return nil, ordinaryFault("DATA_REQUIRED", "transport", "data")
	}
	root, e := ordinaryObject(bytes.TrimSpace(raw), "schema", "requestId", "operation", "strategyId", "route", "execution", "reportingBasis", "data")
	if e != nil {
		return nil, e
	}
	var r OrdinaryBacktestRequest
	if !ordinaryConst(root["schema"], OrdinaryBacktestRequestSchema) || !ordinaryConst(root["operation"], operation) || !ordinaryConst(root["reportingBasis"], OrdinaryBacktestReportingBasis) {
		return nil, ordinarySchema("")
	}
	r.Schema, r.Operation, r.ReportingBasis = OrdinaryBacktestRequestSchema, operation, OrdinaryBacktestReportingBasis
	r.RequestID, e = ordinaryString(root["requestId"], 1, 128)
	if e != nil || !ordinaryID(r.RequestID) {
		return nil, ordinarySchema("requestId")
	}
	r.StrategyID, e = ordinaryString(root["strategyId"], 1, 128)
	if e != nil || !ordinaryID(r.StrategyID) {
		return nil, ordinarySchema("strategyId")
	}
	route, e := ordinaryObject(root["route"], "symbol", "timeframe")
	if e != nil {
		return nil, e
	}
	r.Route.Symbol, e = ordinaryString(route["symbol"], 1, 64)
	if e != nil {
		return nil, e
	}
	r.Route.Timeframe, e = ordinaryString(route["timeframe"], 1, 16)
	if e != nil {
		return nil, e
	}
	execution, e := ordinaryObject(root["execution"], "rangeMethod", "costs", "params", "forceRoute", "chartType")
	if e != nil {
		return nil, e
	}
	r.Execution.RangeMethod, e = ordinaryString(execution["rangeMethod"], 1, 8)
	if e != nil || (r.Execution.RangeMethod != "zone" && r.Execution.RangeMethod != "pivot") {
		return nil, ordinarySchema("execution.rangeMethod")
	}
	if _, e = ordinaryObject(execution["params"]); e != nil {
		return nil, e
	}
	if string(execution["forceRoute"]) != "false" || !ordinaryConst(execution["chartType"], "candles") {
		return nil, ordinarySchema("execution")
	}
	r.Execution.ChartType = "candles"
	costs, e := ordinaryObject(execution["costs"], "feePerUnit", "fillOn", "slippage", "slippageBps", "startEquity")
	if e != nil {
		return nil, e
	}
	for _, f := range []struct {
		k string
		p *float64
	}{{"feePerUnit", &r.Execution.Costs.FeePerUnit}, {"slippage", &r.Execution.Costs.Slippage}, {"slippageBps", &r.Execution.Costs.SlippageBps}, {"startEquity", &r.Execution.Costs.StartEquity}} {
		*f.p, e = ordinaryNumber(costs[f.k], 0, false)
		if e != nil {
			return nil, e
		}
	}
	if r.Execution.Costs.StartEquity <= 0 {
		return nil, ordinarySchema("execution.costs.startEquity")
	}
	r.Execution.Costs.FillOn, e = ordinaryString(costs["fillOn"], 1, 8)
	if e != nil || r.Execution.Costs.FillOn != "close" && r.Execution.Costs.FillOn != "open" && r.Execution.Costs.FillOn != "nextOpen" {
		return nil, ordinarySchema("execution.costs.fillOn")
	}
	if !bytes.Equal(root["data"], []byte("null")) {
		r.Data, e = ordinaryDecodeData(root["data"])
		if e != nil {
			return nil, e
		}
	} else if len(raw) > OrdinaryBacktestMaxTextRequestBytes {
		return nil, ordinaryFault("INPUT_LIMIT", "transport", "")
	}
	c := &OrdinaryBacktestCapture{request: r, source: source, requestJSON: requestJSON, sourceID: ordinaryIdentity([]byte(source)), requestID: ordinaryIdentity(raw)}
	if r.Data != nil {
		id := ordinaryIdentity(root["data"])
		c.dataID = &id
	}
	return c, nil
}

func ordinaryDecodeData(raw json.RawMessage) (*OrdinaryBacktestData, error) {
	m, e := ordinaryObject(raw, "schema", "profile", "bars", "firstT", "lastT", "closedThroughT", "origin")
	if e != nil {
		return nil, e
	}
	if !ordinaryConst(m["schema"], "strat-original-bars-v1") || !ordinaryConst(m["profile"], "original-chart-bars-v1") {
		return nil, ordinarySchema("data")
	}
	d := &OrdinaryBacktestData{Schema: "strat-original-bars-v1", Profile: "original-chart-bars-v1"}
	var rows []json.RawMessage
	if len(m["bars"]) == 0 || m["bars"][0] != '[' || json.Unmarshal(m["bars"], &rows) != nil {
		return nil, ordinarySchema("data.bars")
	}
	if len(rows) > OrdinaryBacktestMaxBars {
		return nil, ordinaryFault("DATA_LIMIT", "transport", "data.bars")
	}
	if len(rows) == 0 {
		return nil, ordinarySchema("data.bars")
	}
	d.Bars = make([][6]float64, len(rows))
	for i, rawRow := range rows {
		var row []json.RawMessage
		if len(rawRow) == 0 || rawRow[0] != '[' || json.Unmarshal(rawRow, &row) != nil || len(row) != 6 {
			return nil, ordinarySchema("data.bars")
		}
		for j, x := range row {
			min := math.Inf(-1)
			if j == 0 || j == 5 {
				min = 0
			}
			d.Bars[i][j], e = ordinaryNumber(x, min, j == 0)
			if e != nil {
				return nil, e
			}
		}
	}
	for _, f := range []struct {
		k string
		p *float64
	}{{"firstT", &d.FirstT}, {"lastT", &d.LastT}, {"closedThroughT", &d.ClosedThroughT}} {
		*f.p, e = ordinaryNumber(m[f.k], 0, true)
		if e != nil {
			return nil, e
		}
	}
	origin, e := ordinaryObject(m["origin"], "datasetId", "historyGeneration", "provenance")
	if e != nil {
		return nil, e
	}
	d.Origin.DatasetID, e = ordinaryString(origin["datasetId"], 1, 256)
	if e != nil {
		return nil, e
	}
	d.Origin.HistoryGeneration, e = ordinaryNumber(origin["historyGeneration"], 0, true)
	if e != nil {
		return nil, e
	}
	if !ordinaryConst(origin["provenance"], "caller-declared") {
		return nil, ordinarySchema("data.origin")
	}
	d.Origin.Provenance = "caller-declared"
	return d, nil
}

type OrdinaryBacktestProducer struct {
	Repo            string `json:"repo"`
	Tag             string `json:"tag"`
	SourceCommit    string `json:"sourceCommit"`
	SourceTree      string `json:"sourceTree"`
	Target          string `json:"target"`
	GoVersion       string `json:"goVersion"`
	SchemaSetSHA256 string `json:"schemaSetSha256"`
}
type OrdinaryBacktestBinding struct {
	Source       OrdinaryBacktestIdentity  `json:"source"`
	Request      OrdinaryBacktestIdentity  `json:"request"`
	Data         *OrdinaryBacktestIdentity `json:"data"`
	Producer     OrdinaryBacktestProducer  `json:"producer"`
	StrategyID   string                    `json:"strategyId"`
	Route        OrdinaryBacktestRoute     `json:"route"`
	Execution    OrdinaryBacktestExecution `json:"execution"`
	ConfigSHA256 *string                   `json:"configSha256"`
}
type OrdinaryBacktestDiagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Line     *int   `json:"line"`
	Column   *int   `json:"col"`
}
type OrdinaryBacktestErrorEnvelope struct {
	Schema      string                       `json:"schema"`
	OK          bool                         `json:"ok"`
	RequestID   *string                      `json:"requestId"`
	Operation   string                       `json:"operation"`
	Binding     *OrdinaryBacktestBinding     `json:"binding"`
	Error       OrdinaryBacktestFault        `json:"error"`
	Diagnostics []OrdinaryBacktestDiagnostic `json:"diagnostics"`
}

func ordinaryHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func ordinaryProducerValid(p OrdinaryBacktestProducer) bool {
	return p.Repo == "spik3r/heisentick-strat" && p.Target == "js/wasm" && utf8.ValidString(p.Tag) && utf8.RuneCountInString(p.Tag) >= 1 && utf8.RuneCountInString(p.Tag) <= 64 && ordinaryHex(p.SourceCommit, 40) && ordinaryHex(p.SourceTree, 40) && ordinaryHex(p.SchemaSetSHA256, 64) && utf8.ValidString(p.GoVersion) && utf8.RuneCountInString(p.GoVersion) >= 1 && utf8.RuneCountInString(p.GoVersion) <= 32
}
func ordinaryErrorCode(stage, code string) bool {
	codes := map[string]string{"transport": "ARGUMENTS SOURCE_ENCODING JSON_ENCODING DUPLICATE_KEY UNKNOWN_FIELD SCHEMA INPUT_LIMIT DATA_REQUIRED DATA_LIMIT INTERNAL", "parse": "PARSE_REJECTED INTERNAL", "admission": "ROUTE_UNSUPPORTED OPTIONS_UNSUPPORTED DATA_INVALID FORMING_BAR DEDICATED_REQUIRED ACCOUNTING_UNAVAILABLE INTERNAL", "execution": "INTERNAL", "report": "OUTPUT_LIMIT NONFINITE_OUTPUT INTERNAL"}
	for _, c := range strings.Fields(codes[stage]) {
		if code == "BACKTEST_"+c {
			return true
		}
	}
	return false
}

// ErrorEnvelope binds later faults to this exact capture. producer must come
// from a future build-owned bridge, never a request field. A nil config digest
// means parsing/config hashing never succeeded; it is allowed only at parse or
// inspect-report stages. This helper does not manufacture release authority.
func (c *OrdinaryBacktestCapture) ErrorEnvelope(operation string, producer OrdinaryBacktestProducer, config *string, fault OrdinaryBacktestFault) (OrdinaryBacktestErrorEnvelope, error) {
	r := OrdinaryBacktestErrorEnvelope{Schema: OrdinaryBacktestErrorSchema, Operation: operation, Error: fault, Diagnostics: []OrdinaryBacktestDiagnostic{}}
	if fault.Stage != "transport" && c != nil {
		id := c.request.RequestID
		r.RequestID = &id
		var cfg *string
		if config != nil {
			x := *config
			cfg = &x
		}
		r.Binding = &OrdinaryBacktestBinding{c.sourceID, c.requestID, c.DataIdentity(), producer, c.request.StrategyID, c.request.Route, c.request.Execution, cfg}
	}
	if e := ValidateOrdinaryBacktestErrorEnvelope(r, operation, c, producer, config); e != nil {
		return OrdinaryBacktestErrorEnvelope{}, e
	}
	return r, nil
}

// This is operation/capture validation, not the root document-union schema.
func ValidateOrdinaryBacktestErrorEnvelope(r OrdinaryBacktestErrorEnvelope, operation string, c *OrdinaryBacktestCapture, producer OrdinaryBacktestProducer, config *string) error {
	bad := func() error { return fmt.Errorf("invalid ordinary backtest error envelope") }
	if !ordinaryOperation(operation) || r.Operation != operation || r.Schema != OrdinaryBacktestErrorSchema || r.OK || !ordinaryErrorCode(r.Error.Stage, r.Error.Code) || operation == "inspect" && r.Error.Stage == "execution" || r.Diagnostics == nil || len(r.Diagnostics) > 256 {
		return bad()
	}
	if !utf8.ValidString(r.Error.Message) || utf8.RuneCountInString(r.Error.Message) < 1 || utf8.RuneCountInString(r.Error.Message) > 2048 {
		return bad()
	}
	if r.Error.Field != nil && (!utf8.ValidString(*r.Error.Field) || utf8.RuneCountInString(*r.Error.Field) < 1 || utf8.RuneCountInString(*r.Error.Field) > 256) {
		return bad()
	}
	if r.Error.BarIndex != nil && (*r.Error.BarIndex < 0 || int64(*r.Error.BarIndex) > ordinaryBacktestMaxInteger) {
		return bad()
	}
	for _, d := range r.Diagnostics {
		if !utf8.ValidString(d.Code) || utf8.RuneCountInString(d.Code) < 1 || utf8.RuneCountInString(d.Code) > 80 || (d.Severity != "error" && d.Severity != "warning") || !utf8.ValidString(d.Message) || utf8.RuneCountInString(d.Message) < 1 || utf8.RuneCountInString(d.Message) > 2048 || (d.Line != nil && *d.Line < 1) || (d.Column != nil && *d.Column < 1) {
			return bad()
		}
	}
	if r.Error.Stage == "transport" {
		if r.RequestID != nil || r.Binding != nil {
			return bad()
		}
		return nil
	}
	if c == nil || c.request.Operation != operation || r.RequestID == nil || *r.RequestID != c.request.RequestID || r.Binding == nil || !ordinaryProducerValid(producer) {
		return bad()
	}
	b := r.Binding
	if b.StrategyID != c.request.StrategyID || b.Source != c.sourceID || b.Request != c.requestID || !reflect.DeepEqual(b.Data, c.dataID) || b.Route != c.request.Route || b.Execution != c.request.Execution || b.Producer != producer || !reflect.DeepEqual(b.ConfigSHA256, config) {
		return bad()
	}
	if config != nil && !ordinaryHex(*config, 64) {
		return bad()
	}
	if r.Error.Stage == "parse" {
		if config != nil {
			return bad()
		}
	} else if !(operation == "inspect" && r.Error.Stage == "report") && config == nil {
		return bad()
	}
	return nil
}
