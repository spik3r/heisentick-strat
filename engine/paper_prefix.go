package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const (
	PaperPrefixSchema        = "heisentick-paper-prefix-v1"
	PaperPrefixRequestSchema = "heisentick-paper-prefix-request-v1"
	PaperPrefixMaxBars       = 500000
	PaperPrefixMaxEvents     = 2000000
	PaperPrefixMaxBytes      = 128 << 20
)

type PaperRoute struct {
	StrategyID  string `json:"strategyId"`
	Symbol      string `json:"symbol"`
	Timeframe   string `json:"timeframe"`
	RangeMethod string `json:"rangeMethod"`
}

type PaperUnits struct {
	QuoteCurrency string  `json:"quoteCurrency"`
	PriceUnit     string  `json:"priceUnit"`
	QuantityUnit  string  `json:"quantityUnit"`
	Multiplier    float64 `json:"multiplier"`
}

type PaperCosts struct {
	StartEquity float64 `json:"startEquity"`
	FeePerUnit  float64 `json:"feePerUnit"`
	FillOn      string  `json:"fillOn"`
	Slippage    float64 `json:"slippage"`
	SlippageBps float64 `json:"slippageBps"`
}

// T is the bar's open timestamp. ClosedThroughT is the caller's explicit close
// watermark; neither it nor the unit labels attest a provider or instrument.
type PaperPrefixRequest struct {
	Schema          string          `json:"schema"`
	Source          string          `json:"source"`
	Route           PaperRoute      `json:"route"`
	Units           PaperUnits      `json:"units"`
	Costs           PaperCosts      `json:"costs"`
	ActivationFromT int64           `json:"activationFromT"`
	ClosedThroughT  int64           `json:"closedThroughT"`
	Bars            [][]float64     `json:"bars"`
	Checkpoint      json.RawMessage `json:"checkpoint,omitempty"`
}

type PaperSourceIdentity struct {
	SHA256    string `json:"sha256"`
	UTF8Bytes int    `json:"utf8Bytes"`
}

type PaperBinding struct {
	Contract        string                             `json:"contract"`
	Source          PaperSourceIdentity                `json:"source"`
	ConfigIdentity  SequentialInspectionConfigIdentity `json:"configIdentity"`
	Route           PaperRoute                         `json:"route"`
	Units           PaperUnits                         `json:"units"`
	Costs           PaperCosts                         `json:"costs"`
	ActivationFromT int64                              `json:"activationFromT"`
	Producer        SequentialInspectionProducer       `json:"producer"`
}

type PaperEventRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}
type PaperPending struct {
	Market int    `json:"market"`
	Limit  int    `json:"limit"`
	Exit   int    `json:"exit"`
	Digest string `json:"digest"`
}

type PaperPrefixResult struct {
	Schema          string                 `json:"schema"`
	Mode            string                 `json:"mode"`
	Binding         PaperBinding           `json:"binding"`
	BindingDigest   string                 `json:"bindingDigest"`
	InputDigest     string                 `json:"inputDigest"`
	ProcessedBars   int                    `json:"processedBars"`
	LastBarT        float64                `json:"lastBarT"`
	ClosedThroughT  int64                  `json:"closedThroughT"`
	EventRange      PaperEventRange        `json:"eventRange"`
	Events          []PaperEvent           `json:"events"`
	Account         PaperAccount           `json:"account"`
	CompletedTrades PaperTradeStatistics   `json:"completedTrades"`
	OpenPositions   []OpenPositionSnapshot `json:"openPositions"`
	Pending         PaperPending           `json:"pending"`
	Checkpoint      json.RawMessage        `json:"checkpoint"`
}

// PaperPrefixError contains only fixed codes, never authored source or paths.
type PaperPrefixError struct {
	Code string `json:"code"`
}

func (e *PaperPrefixError) Error() string { return e.Code }
func paperError(code string) error        { return &PaperPrefixError{Code: "PAPER_PREFIX_" + code} }

func paperDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", paperError("NONFINITE")
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func paperSourceDigest(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

// DecodePaperPrefixRequest enforces exact JSON field names, required values and
// duplicate-key rejection before the ordinary Go struct decoder can normalize.
func DecodePaperPrefixRequest(raw []byte) (PaperPrefixRequest, error) {
	var r PaperPrefixRequest
	if len(raw) > PaperPrefixMaxBytes || !utf8.Valid(raw) {
		return r, paperError("INPUT")
	}
	if err := paperJSONStrings(raw); err != nil {
		return r, err
	}
	if err := paperJSONUnique(raw); err != nil {
		return r, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return r, paperError("INPUT")
	}
	if err := paperFields(fields, []string{"schema", "source", "route", "units", "costs", "activationFromT", "closedThroughT", "bars"}, []string{"checkpoint"}); err != nil {
		return r, err
	}
	for _, part := range []struct {
		name string
		keys []string
	}{
		{"route", []string{"strategyId", "symbol", "timeframe", "rangeMethod"}},
		{"units", []string{"quoteCurrency", "priceUnit", "quantityUnit", "multiplier"}},
		{"costs", []string{"startEquity", "feePerUnit", "fillOn", "slippage", "slippageBps"}},
	} {
		var obj map[string]json.RawMessage
		if json.Unmarshal(fields[part.name], &obj) != nil {
			return r, paperError("INPUT")
		}
		if err := paperFields(obj, part.keys, nil); err != nil {
			return r, err
		}
	}
	var rows [][]json.RawMessage
	if json.Unmarshal(fields["bars"], &rows) != nil {
		return r, paperError("INPUT")
	}
	for _, row := range rows {
		if len(row) != 6 || !paperExactTimestamp(row[0]) {
			return r, paperError("INPUT")
		}
		for _, v := range row {
			if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				return r, paperError("INPUT")
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&r) != nil {
		return PaperPrefixRequest{}, paperError("INPUT")
	}
	return r, nil
}

func paperFields(fields map[string]json.RawMessage, required, optional []string) error {
	allowed := map[string]bool{}
	for _, key := range required {
		allowed[key] = true
		if len(fields[key]) == 0 || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return paperError("INPUT")
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			return paperError("INPUT")
		}
	}
	return nil
}

func paperJSONUnique(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 64 {
			return paperError("INPUT")
		}
		token, err := decoder.Token()
		if err != nil {
			return paperError("INPUT")
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return paperError("INPUT")
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return paperError("INPUT")
				}
				seen[s] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			token, err = decoder.Token()
			if err != nil || token != json.Delim('}') {
				return paperError("INPUT")
			}
		case '[':
			for decoder.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			token, err = decoder.Token()
			if err != nil || token != json.Delim(']') {
				return paperError("INPUT")
			}
		default:
			return paperError("INPUT")
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return paperError("INPUT")
	}
	return nil
}

func paperTimeframeMS(tf string) int64 {
	return map[string]int64{"1m": 60000, "5m": 300000, "15m": 900000, "30m": 1800000, "1h": 3600000, "4h": 14400000, "1d": 86400000}[tf]
}

func preparePaperPrefix(r PaperPrefixRequest) (*PreparedRun, PaperBinding, error) {
	var binding PaperBinding
	if r.Schema != PaperPrefixRequestSchema || len(r.Source) == 0 || len(r.Source) > SequentialInspectionMaxSource || !utf8.ValidString(r.Source) {
		return nil, binding, paperError("INPUT")
	}
	for _, s := range []string{r.Route.StrategyID, r.Route.Symbol, r.Units.QuoteCurrency, r.Units.PriceUnit, r.Units.QuantityUnit} {
		if strings.TrimSpace(s) == "" || len(s) > 128 {
			return nil, binding, paperError("INPUT")
		}
	}
	c := r.Costs
	if !isFinite(c.StartEquity) || c.StartEquity <= 0 || !isFinite(c.FeePerUnit) || c.FeePerUnit < 0 || !isFinite(c.Slippage) || c.Slippage < 0 || !isFinite(c.SlippageBps) || c.SlippageBps < 0 || (c.FillOn != "close" && c.FillOn != "nextOpen") {
		return nil, binding, paperError("COSTS")
	}
	if r.Units.Multiplier != 1 {
		return nil, binding, paperError("UNITS_UNSUPPORTED")
	}
	duration := paperTimeframeMS(r.Route.Timeframe)
	const maxMS = int64(1<<53 - 1)
	if duration == 0 || (r.Route.RangeMethod != "zone" && r.Route.RangeMethod != "pivot") || r.ActivationFromT < 0 || r.ActivationFromT > maxMS-duration || r.ClosedThroughT < 0 || r.ClosedThroughT > maxMS || len(r.Bars) == 0 || len(r.Bars) > PaperPrefixMaxBars {
		return nil, binding, paperError("INPUT")
	}
	series := marketdata.NewSeries(len(r.Bars))
	for i, row := range r.Bars {
		if len(row) != 6 {
			return nil, binding, paperError("BARS")
		}
		for _, v := range row {
			if !isFinite(v) {
				return nil, binding, paperError("BARS")
			}
		}
		t := row[0]
		if t < 0 || t > float64(maxMS-duration) || t != math.Trunc(t) || t+float64(duration) > float64(r.ClosedThroughT) || (i > 0 && (t <= r.Bars[i-1][0] || math.Mod(t-r.Bars[i-1][0], float64(duration)) != 0)) || row[5] < 0 || row[2] < row[3] || row[2] < row[1] || row[2] < row[4] || row[3] > row[1] || row[3] > row[4] {
			return nil, binding, paperError("BARS")
		}
		series.T[i], series.O[i], series.H[i], series.L[i], series.C[i], series.V[i] = row[0], row[1], row[2], row[3], row[4], row[5]
	}
	parsed, err := dsl.Parse(r.Source)
	if err != nil || len(parsed.Errors) != 0 {
		return nil, binding, paperError("SOURCE")
	}
	family := setupTypeFromAny(parsed.Config["setupType"])
	if family != string(dsl.FamilyOpeningRangeBreakout) && family != string(dsl.FamilyLegacySetup9) {
		return nil, binding, paperError("FAMILY_UNSUPPORTED")
	}
	htf := mapValue(parsed.Config, "htf")
	mode, _ := htf["mode"].(string)
	if sourceTimeframeFromConfig(parsed.Config) != "" || (mode != "" && mode != "off") {
		return nil, binding, paperError("CONTEXT_UNSUPPORTED")
	}
	if family == string(dsl.FamilyLegacySetup9) && c.FillOn != "close" {
		return nil, binding, paperError("FILL_UNSUPPORTED")
	}
	request := RunRequest{Config: parsed.Config, Series: series, StrategyID: r.Route.StrategyID, Symbol: r.Route.Symbol, Timeframe: r.Route.Timeframe, RangeMethod: r.Route.RangeMethod, Costs: Costs{StartEquity: c.StartEquity, FeePerUnit: c.FeePerUnit, FillOn: c.FillOn, Slippage: c.Slippage, SlippageBps: c.SlippageBps}}
	prepared, err := PrepareRun(request)
	if err != nil {
		return nil, binding, paperError("ADMISSION")
	}
	if prepared.offRoute || prepared.c5 || prepared.sequentialFull != nil || prepared.params.UseHTFBias {
		return nil, binding, paperError("ROUTE_UNSUPPORTED")
	}
	configHash, err := paperDigest(parsed.Config)
	if err != nil {
		return nil, binding, err
	}
	binding = PaperBinding{Contract: PaperPrefixSchema, Source: PaperSourceIdentity{paperSourceDigest(r.Source), len(r.Source)}, ConfigIdentity: SequentialInspectionConfigIdentity{SHA256: configHash, Encoding: SequentialInspectionConfigEncoding}, Route: r.Route, Units: r.Units, Costs: r.Costs, ActivationFromT: r.ActivationFromT, Producer: sequentialInspectionProducer()}
	return prepared, binding, nil
}

// RunPaperPrefix executes no external orders. Recovery replays and verifies the
// prior receipt, then replays the extended prefix; no caller state is restored.
func RunPaperPrefix(request PaperPrefixRequest) (PaperPrefixResult, error) {
	if bytes.Equal(bytes.TrimSpace(request.Checkpoint), []byte("null")) {
		request.Checkpoint = nil
	}
	prepared, binding, err := preparePaperPrefix(request)
	if err != nil {
		return PaperPrefixResult{}, err
	}
	startEvent := 0
	if len(request.Checkpoint) > 0 {
		prior, err := verifyPaperCheckpoint(request, binding)
		if err != nil {
			return PaperPrefixResult{}, err
		}
		startEvent = prior.EventCount
	}
	result, _, err := executePaperPrefix(request, prepared, binding)
	if err != nil {
		return PaperPrefixResult{}, err
	}
	if startEvent > len(result.Events) {
		return PaperPrefixResult{}, paperError("CHECKPOINT")
	}
	result.EventRange.From = startEvent
	result.Events = append([]PaperEvent{}, result.Events[startEvent:]...)
	if _, err := EncodePaperPrefixResult(result); err != nil {
		return PaperPrefixResult{}, err
	}
	return result, nil
}

func executePaperPrefix(r PaperPrefixRequest, p *PreparedRun, binding PaperBinding) (PaperPrefixResult, paperCheckpoint, error) {
	var result PaperPrefixResult
	var checkpoint paperCheckpoint
	identity, err := paperDigest(binding)
	if err != nil {
		return result, checkpoint, err
	}
	n := p.series.Len()
	start := n
	for i, t := range p.series.T {
		if t >= float64(r.ActivationFromT) {
			if t != float64(r.ActivationFromT) {
				return result, checkpoint, paperError("ACTIVATION")
			}
			start = i
			break
		}
	}
	p.broker.reset(p.series, p.cols, p.htfTrend, p.ema, p.emaSlope, p.params, p.fixture, p.trades)
	capture := newPaperCollector(identity, r.Costs.StartEquity)
	p.broker.paperAccounting = capture
	if start < n {
		p.broker.runRangeWithFinalization(start, false)
	}
	if capture.err != nil {
		return result, checkpoint, capture.err
	}
	if err := validatePrefixOutput(p.fixture.Costs, p.broker.trades, p.broker.openPositionSnapshot()); err != nil {
		return result, checkpoint, paperError("NONFINITE")
	}
	if !paperSame(capture.account.RealizedNet, p.broker.realized) || (capture.open != nil) != p.broker.hasPosition {
		return result, checkpoint, paperError("ACCOUNTING")
	}
	positions := p.broker.openPositionSnapshot()
	if len(positions) > 0 {
		positions[0].PositionID = capture.open.PositionID
	}
	pendingDigest, err := paperDigest(struct {
		Market []order
		Limit  []order
		Exit   []pendingExit
	}{p.broker.pendingOrders, p.broker.limitOrders, p.broker.pendingExits})
	if err != nil {
		return result, checkpoint, err
	}
	input, err := paperDigest(struct {
		Bars           [][]float64 `json:"bars"`
		ClosedThroughT int64       `json:"closedThroughT"`
	}{r.Bars, r.ClosedThroughT})
	if err != nil {
		return result, checkpoint, err
	}
	result = PaperPrefixResult{Schema: PaperPrefixSchema, Mode: "replay-verified", Binding: binding, BindingDigest: identity, InputDigest: input, ProcessedBars: n, LastBarT: p.series.T[n-1], ClosedThroughT: r.ClosedThroughT, EventRange: PaperEventRange{0, len(capture.events)}, Events: capture.events, Account: capture.account, CompletedTrades: capture.statistics, OpenPositions: positions, Pending: PaperPending{len(p.broker.pendingOrders), len(p.broker.limitOrders), len(p.broker.pendingExits), pendingDigest}}
	checkpoint, err = newPaperCheckpoint(result, &p.broker, capture)
	if err != nil {
		return PaperPrefixResult{}, checkpoint, err
	}
	result.Checkpoint, err = json.Marshal(checkpoint)
	if err != nil {
		return PaperPrefixResult{}, checkpoint, paperError("NONFINITE")
	}
	return result, checkpoint, nil
}

// EncodePaperPrefixResult bounds the complete document before any CLI output.
func EncodePaperPrefixResult(result PaperPrefixResult) ([]byte, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, paperError("NONFINITE")
	}
	if len(raw) > PaperPrefixMaxBytes {
		return nil, paperError("OUTPUT_LIMIT")
	}
	return raw, nil
}

// Reject unpaired JSON Unicode escapes before encoding/json repairs them.
func paperJSONStrings(raw []byte) error {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '"' {
			continue
		}
		i++
		for i < len(raw) && raw[i] != '"' {
			if raw[i] != '\\' {
				i++
				continue
			}
			i++
			if i >= len(raw) {
				return paperError("INPUT")
			}
			if raw[i] != 'u' {
				i++
				continue
			}
			if i+4 >= len(raw) {
				return paperError("INPUT")
			}
			v, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
			if err != nil {
				return paperError("INPUT")
			}
			i += 5
			if v >= 0xd800 && v <= 0xdbff {
				if i+5 >= len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
					return paperError("INPUT")
				}
				low, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
				if err != nil || low < 0xdc00 || low > 0xdfff {
					return paperError("INPUT")
				}
				i += 6
			} else if v >= 0xdc00 && v <= 0xdfff {
				return paperError("INPUT")
			}
		}
	}
	return nil
}

// Check the original decimal token before binary64 rounding can erase a
// sub-millisecond fraction. Bounds precede exact-rational parsing.
func paperExactTimestamp(raw []byte) bool {
	token := strings.TrimSpace(string(raw))
	if len(token) == 0 || len(token) > 64 {
		return false
	}
	if i := strings.IndexAny(token, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(token[i+1:])
		if err != nil || exponent < -400 || exponent > 400 {
			return false
		}
	}
	value, ok := new(big.Rat).SetString(token)
	if !ok || !value.IsInt() || !value.Num().IsInt64() {
		return false
	}
	integer := value.Num().Int64()
	return integer >= 0 && integer <= 1<<53-1
}
