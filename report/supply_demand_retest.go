package report

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/engine"
)

const (
	SupplyDemandRetestResultSchema   = "strat-supply-demand-retest-result-v1"
	SupplyDemandRetestMetricsBasis   = "supply-demand-retest-broker-close-mtm-v1"
	SupplyDemandRetestMaxOutputBytes = 64 << 20
)

type SupplyDemandRetestCapabilities struct {
	Headline      bool                     `json:"headline"`
	Trades        bool                     `json:"trades"`
	Equity        bool                     `json:"equity"`
	Groupings     bool                     `json:"groupings"`
	Monthly       bool                     `json:"monthly"`
	RDistribution bool                     `json:"rDistribution"`
	Portfolio     bool                     `json:"portfolio"`
	Live          bool                     `json:"live"`
	Paper         bool                     `json:"paper"`
	Limits        SupplyDemandRetestLimits `json:"limits"`
}

type SupplyDemandRetestLimits struct {
	MaxTotalRows    int    `json:"maxTotalRows"`
	MaxRequestBytes int    `json:"maxRequestBytes"`
	MaxSourceBytes  int    `json:"maxSourceBytes"`
	MaxOutputBytes  int    `json:"maxOutputBytes"`
	MaxWorkEstimate uint64 `json:"maxWorkEstimate"`
}

type SupplyDemandRetestReport struct {
	Schema          string                             `json:"schema"`
	ContractVersion int                                `json:"contractVersion"`
	Identity        *engine.SupplyDemandRetestIdentity `json:"identity,omitempty"`
	Run             *engine.RunResult                  `json:"run,omitempty"`
	Metrics         *SequentialMetrics                 `json:"metrics,omitempty"`
	Terminal        *engine.SupplyDemandRetestTerminal `json:"terminal,omitempty"`
	Capabilities    *SupplyDemandRetestCapabilities    `json:"capabilities,omitempty"`
	Error           *SupplyDemandRetestReportError     `json:"error,omitempty"`
}

type SupplyDemandRetestReportError struct {
	Code     string `json:"code"`
	Field    string `json:"field,omitempty"`
	BarIndex *int   `json:"barIndex,omitempty"`
}

// RunSupplyDemandRetestReport is the shared native/WASM entry point. Every
// refusal is one small typed envelope without any partially successful payload.
func RunSupplyDemandRetestReport(requestJSON, source string) (response []byte) {
	defer func() {
		if recover() != nil {
			response = SupplyDemandRetestFailure("internal-error", "")
		}
	}()
	if len(requestJSON) == 0 || len(requestJSON) > engine.SupplyDemandRetestMaxRequestBytes {
		return SupplyDemandRetestFailure("invalid-input", "request.bytes")
	}
	if len(source) == 0 || len(source) > engine.SupplyDemandRetestMaxSourceBytes || !utf8.ValidString(source) {
		return SupplyDemandRetestFailure("invalid-input", "source.bytes")
	}
	execution, err := engine.RunSupplyDemandRetestWithAccounting([]byte(requestJSON), source)
	if err != nil {
		return supplyDemandRetestFailure(err)
	}
	raw, err := EncodeSupplyDemandRetestReport(execution)
	if err != nil {
		return supplyDemandRetestFailure(err)
	}
	return raw
}

// SupplyDemandRetestFailure is used only for transport errors before admission.
// Unknown codes and unbounded fields cannot leak arbitrary internal error text.
func SupplyDemandRetestFailure(code, field string) []byte {
	return supplyDemandRetestFailure(&engine.SupplyDemandRetestError{Code: code, Field: field, BarIndex: -1})
}

func supplyDemandRetestFailure(err error) []byte {
	failure := SupplyDemandRetestReportError{Code: "internal-error"}
	var input *engine.SupplyDemandRetestError
	var accounting *engine.SequentialAccountingError
	var metrics *SequentialMetricsError
	switch {
	case errors.As(err, &input) && input != nil:
		switch input.Code {
		case "invalid-input", "unsupported-profile", "unsupported-route", "invalid-derived-value", "work-limit", "output-limit", "invalid-arguments", "input-unavailable":
			failure.Code = input.Code
			if retestOutputLabel(input.Field, 128, true) {
				failure.Field = input.Field
			}
			if input.BarIndex >= 0 && input.BarIndex < 500000 {
				index := input.BarIndex
				failure.BarIndex = &index
			}
		}
	case errors.As(err, &accounting) && accounting != nil:
		failure.Code = "invalid-derived-value"
	case errors.As(err, &metrics) && metrics != nil:
		failure.Code = "invalid-derived-value"
	}
	// This owned envelope contains only a bounded code, field and index.
	raw, _ := json.Marshal(SupplyDemandRetestReport{Schema: SupplyDemandRetestResultSchema, ContractVersion: 1, Error: &failure})
	return raw
}

func retestOutputError(code, field string) error {
	return &engine.SupplyDemandRetestError{Code: code, Field: field, BarIndex: -1}
}

// EncodeSupplyDemandRetestReport projects the unchanged native accounting
// reducer, then validates and bounds the complete owned serialization shape.
// It never reconstructs accounting from rounded legacy trade P&L.
func EncodeSupplyDemandRetestReport(execution engine.SupplyDemandRetestExecution) ([]byte, error) {
	if len(execution.Run.Trades) > 500000 || len(execution.Accounting.Marks) > 500000 || len(execution.Accounting.Trades) > 500000 || len(execution.Accounting.Events) > 1000000 {
		return nil, retestOutputError("output-limit", "counts")
	}
	metrics, err := ProjectSequentialMetrics(execution.Run, execution.Accounting)
	if err != nil {
		return nil, err
	}
	metrics.Basis = SupplyDemandRetestMetricsBasis
	result := SupplyDemandRetestReport{
		Schema: SupplyDemandRetestResultSchema, ContractVersion: 1,
		Identity: &execution.Identity, Run: &execution.Run, Metrics: &metrics, Terminal: &execution.Terminal,
		Capabilities: &SupplyDemandRetestCapabilities{Headline: true, Trades: true, Equity: true,
			Limits: SupplyDemandRetestLimits{MaxTotalRows: 500000, MaxRequestBytes: 64 << 20, MaxSourceBytes: 1 << 20, MaxOutputBytes: SupplyDemandRetestMaxOutputBytes, MaxWorkEstimate: 500000000}},
	}
	if _, err := supplyDemandRetestOutputBound(result, SupplyDemandRetestMaxOutputBytes); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, retestOutputError("invalid-derived-value", "encoding")
	}
	if len(raw) > SupplyDemandRetestMaxOutputBytes {
		return nil, retestOutputError("output-limit", "encodedBytes")
	}
	return raw, nil
}

func retestOutputLabel(s string, max int, empty bool) bool {
	if (!empty && s == "") || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// The estimator visits actual typed values without calling any Marshaler and
// without constructing RunResult's per-trade []any projection. Numbers receive
// 32 bytes (including every finite binary64 spelling and every integer width),
// strings six bytes per UTF-8 byte plus quotes (including HTML and U+2028/2029
// escaping), and every field, comma, colon and bracket is included. Omitempty
// fields are conservatively included. The one custom RunResult projection is
// handled explicitly below; no other custom-marshaler type is accepted.
type retestJSONBound struct{ size, limit uint64 }

func (b *retestJSONBound) add(n uint64) error {
	if n > b.limit || b.size > b.limit-n {
		return retestOutputError("output-limit", "encodedUpperBound")
	}
	b.size += n
	return nil
}
func (b *retestJSONBound) text(s string) error {
	if !utf8.ValidString(s) {
		return retestOutputError("invalid-derived-value", "outputUnicode")
	}
	if b.limit < 2 || uint64(len(s)) > (b.limit-2)/6 {
		return retestOutputError("output-limit", "encodedUpperBound")
	}
	return b.add(2 + 6*uint64(len(s)))
}

func supplyDemandRetestOutputBound(result SupplyDemandRetestReport, limit uint64) (uint64, error) {
	if result.Schema != SupplyDemandRetestResultSchema || result.ContractVersion != 1 || result.Error != nil || result.Identity == nil || result.Run == nil || result.Metrics == nil || result.Terminal == nil || result.Capabilities == nil {
		return 0, retestOutputError("invalid-derived-value", "resultShape")
	}
	if err := validateRetestRun(*result.Run); err != nil {
		return 0, err
	}

	if result.Metrics.Basis != SupplyDemandRetestMetricsBasis || len(result.Metrics.Equity) < 2 || len(result.Metrics.Equity) > 500000 || len(result.Metrics.TradeAccounting) != result.Run.TradeCount {
		return 0, retestOutputError("invalid-derived-value", "metricsShape")
	}
	if err := validateRetestIdentity(result); err != nil {
		return 0, err
	}
	terminal := result.Terminal
	if terminal.Policy != "last-close-liquidation" || terminal.Position != "flat" || terminal.EndOfTestTrades < 0 || terminal.EndOfTestTrades > result.Run.TradeCount || terminal.PendingMarketOrders < 0 || terminal.PendingLimitOrders < 0 || terminal.PendingDeferredExits < 0 || terminal.PendingMarketOrders > result.Identity.Chart.Count || terminal.PendingLimitOrders > result.Identity.Chart.Count || terminal.PendingDeferredExits > result.Identity.Chart.Count {
		return 0, retestOutputError("invalid-derived-value", "terminal")
	}
	count := 0
	for _, trade := range result.Run.Trades {
		if trade.ExitIndex >= result.Identity.Chart.Count || trade.Meta["side"] != trade.Side {
			return 0, retestOutputError("invalid-derived-value", "tradeBinding")
		}
		if trade.Reason == engine.ReasonEndOfTest {
			count++
		}
	}
	if count != terminal.EndOfTestTrades || terminal.Liquidated != (count > 0) {
		return 0, retestOutputError("invalid-derived-value", "terminalTrades")
	}
	b := retestJSONBound{limit: limit}
	if err := b.value(reflect.ValueOf(result)); err != nil {
		return 0, err
	}
	return b.size, nil
}

var retestOutputStructs = map[reflect.Type]bool{
	reflect.TypeOf(SupplyDemandRetestReport{}):                true,
	reflect.TypeOf(SupplyDemandRetestCapabilities{}):          true,
	reflect.TypeOf(SupplyDemandRetestLimits{}):                true,
	reflect.TypeOf(SupplyDemandRetestReportError{}):           true,
	reflect.TypeOf(engine.SupplyDemandRetestIdentity{}):       true,
	reflect.TypeOf(engine.SupplyDemandRetestSeriesIdentity{}): true,
	reflect.TypeOf(engine.SupplyDemandRetestLimits{}):         true,
	reflect.TypeOf(engine.SupplyDemandRetestTerminal{}):       true,
	reflect.TypeOf(engine.Costs{}):                            true,
	reflect.TypeOf(SequentialMetrics{}):                       true,
	reflect.TypeOf(SequentialHeadline{}):                      true,
	reflect.TypeOf(SequentialTradeAccounting{}):               true,
	reflect.TypeOf(SequentialEquityPoint{}):                   true,
}

func (b *retestJSONBound) value(v reflect.Value) error {
	if !v.IsValid() {
		return b.add(4)
	}
	if v.Type() == reflect.TypeOf(engine.RunResult{}) {
		return b.run(v.Interface().(engine.RunResult))
	}
	if v.Type() == reflect.TypeOf(engine.Trade{}) {
		return b.trade(v.Interface().(engine.Trade))
	}
	if v.Type() == reflect.TypeOf(engine.TradeMeta{}) {
		return b.meta(v.Interface().(engine.TradeMeta))
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return b.add(4)
		}
		return b.value(v.Elem())
	case reflect.Bool:
		return b.add(5)
	case reflect.String:
		return b.text(v.String())
	case reflect.Float64:
		x := v.Float()
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return retestOutputError("invalid-derived-value", "outputNumber")
		}
		return b.add(32)
	case reflect.Int, reflect.Int64, reflect.Uint64:
		return b.add(32)
	case reflect.Slice:
		if v.Len() > 500000 {
			return retestOutputError("output-limit", "outputCount")
		}
		if v.IsNil() {
			return b.add(4)
		}
		if err := b.add(2); err != nil {
			return err
		}
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				if err := b.add(1); err != nil {
					return err
				}
			}
			if err := b.value(v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		if !retestOutputStructs[v.Type()] {
			return retestOutputError("invalid-derived-value", "outputType")
		}
		return b.object(v)
	default:
		return retestOutputError("invalid-derived-value", "outputType")
	}
}

func (b *retestJSONBound) object(v reflect.Value) error {
	if err := b.add(2); err != nil {
		return err
	}
	fields := 0
	for i := 0; i < v.NumField(); i++ {
		tag := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == "" || !v.Field(i).CanInterface() {
			return retestOutputError("invalid-derived-value", "outputField")
		}
		if fields > 0 {
			if err := b.add(1); err != nil {
				return err
			}
		}
		fields++
		if err := b.text(tag); err != nil {
			return err
		}
		if err := b.add(1); err != nil {
			return err
		}
		if err := b.value(v.Field(i)); err != nil {
			return err
		}
	}
	return nil
}

func (b *retestJSONBound) run(run engine.RunResult) error {
	// MarshalJSON omits family-specific audits, projects an empty higherTimeframe
	// to null and always projects trades to a non-null array. Counting the owned
	// struct conservatively includes omitted audits; four extra bytes cover both
	// higherTimeframe's null substitution and an empty trades slice projection.
	if err := b.add(4); err != nil {
		return err
	}
	return b.object(reflect.ValueOf(run))
}
func (b *retestJSONBound) trade(trade engine.Trade) error {
	// RunResult.MarshalJSON may replace the four stop/target numbers with null.
	// Their 32-byte numeric allowance is strictly larger than a null token.
	return b.object(reflect.ValueOf(trade))
}
func validateRetestRun(run engine.RunResult) error {
	if run.SequentialFull != nil || run.TimedAudit != nil || len(run.ClockRangeDays) != 0 || run.Schema != "dsl-conformance-trades-v1" || run.TradeCount != len(run.Trades) || run.TradeCount < 0 || run.TradeCount > 500000 || run.RangeMethod != "zone" || !retestOutputLabel(run.Case, 256, false) || !retestOutputLabel(run.StrategyID, 128, false) || !retestOutputLabel(run.Symbol, 32, false) {
		return retestOutputError("invalid-derived-value", "runShape")
	}
	if run.Timeframe != "15m" && run.Timeframe != "30m" && run.Timeframe != "1h" {
		return retestOutputError("invalid-derived-value", "runRoute")
	}
	if run.HigherTimeframe != "" && run.HigherTimeframe != "1d" {
		return retestOutputError("invalid-derived-value", "runRoute")
	}
	for _, trade := range run.Trades {
		if trade.Partial || trade.EntryIndex < 0 || trade.ExitIndex < trade.EntryIndex || trade.Side != "long" && trade.Side != "short" || !retestOutputLabel(trade.Reason, 64, false) || !retestOutputLabel(trade.Rule, 128, true) || !retestOutputLabel(trade.Tag, 128, false) {
			return retestOutputError("invalid-derived-value", "tradeShape")
		}
		if err := validateRetestMeta(trade.Meta); err != nil {
			return err
		}
	}
	return nil
}

func validateRetestMeta(meta engine.TradeMeta) error {
	if len(meta) < 18 || len(meta) > 19 {
		return retestOutputError("invalid-derived-value", "tradeMetaShape")
	}
	for _, key := range []string{"baseBodyHigh", "baseBodyLow", "baseEnd", "baseStart", "baseWickHigh", "baseWickLow", "chochLevel", "flipped", "originalZoneType", "pattern", "setup", "side", "zoneCreatedAt", "zoneHigh", "zoneLow", "zoneType", "gradeRequired", "gradeScore"} {
		if _, ok := meta[key]; !ok {
			return retestOutputError("invalid-derived-value", "tradeMetaShape")
		}
	}
	for key, value := range meta {
		switch key {
		case "baseBodyHigh", "baseBodyLow", "baseWickHigh", "baseWickLow", "zoneHigh", "zoneLow", "gradeRequired", "gradeScore":
			if x, ok := value.(float64); !ok || math.IsNaN(x) || math.IsInf(x, 0) {
				return retestOutputError("invalid-derived-value", "tradeMetaNumber")
			}
		case "baseStart", "baseEnd", "zoneCreatedAt":
			if x, ok := value.(int); !ok || x < 0 || x >= 500000 {
				return retestOutputError("invalid-derived-value", "tradeMetaIndex")
			}
		case "chochLevel":
			if value != nil {
				if x, ok := value.(float64); !ok || math.IsNaN(x) || math.IsInf(x, 0) {
					return retestOutputError("invalid-derived-value", "tradeMetaNumber")
				}
			}
		case "flipped":
			if _, ok := value.(bool); !ok {
				return retestOutputError("invalid-derived-value", "tradeMetaType")
			}
		case "originalZoneType", "pattern", "setup", "side", "zoneType", "dayTheme":
			if x, ok := value.(string); !ok || !retestOutputLabel(x, 128, true) {
				return retestOutputError("invalid-derived-value", "tradeMetaString")
			}
		default:
			return retestOutputError("invalid-derived-value", "tradeMetaField")
		}
	}
	return nil
}
func (b *retestJSONBound) meta(meta engine.TradeMeta) error {
	if err := validateRetestMeta(meta); err != nil {
		return err
	}
	if err := b.add(2); err != nil {
		return err
	}
	for key, value := range meta {
		if err := b.text(key); err != nil {
			return err
		}
		if err := b.add(2); err != nil {
			return err
		}
		if err := b.value(reflect.ValueOf(value)); err != nil {
			return err
		}
	}
	return nil
}

func validateRetestIdentity(result SupplyDemandRetestReport) error {
	id, run := result.Identity, result.Run
	digest := func(s string) bool {
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
	if !digest(id.RequestSHA256) || !digest(id.SourceSHA256) || id.Case != run.Case || id.StrategyID != run.StrategyID || id.Symbol != run.Symbol || id.Timeframe != run.Timeframe || id.HigherTimeframe != run.HigherTimeframe || id.SourceTimeframe != "4h" || id.RangeMethod != "zone" || id.Costs != run.Costs || id.Profile != engine.SupplyDemandRetestProfile || id.Basis != SupplyDemandRetestMetricsBasis || id.TerminalPolicy != "last-close-liquidation" {
		return retestOutputError("invalid-derived-value", "identity")
	}
	wantLimits := engine.SupplyDemandRetestLimits{MaxRequestBytes: 64 << 20, MaxSourceBytes: 1 << 20, MaxOutputBytes: 64 << 20, MaxRows: 500000, MaxWorkScore: 500000000}
	if id.Limits != wantLimits || id.WorkScore == 0 || id.WorkScore > 500000000 {
		return retestOutputError("invalid-derived-value", "limits")
	}
	if id.HTFMode == "off" {
		if id.HTFPolicy != "off" || id.HigherTimeframe != "" || id.Daily.Count != 0 {
			return retestOutputError("invalid-derived-value", "htfIdentity")
		}
	} else if id.HTFMode == "notAgainst" {
		if id.HTFPolicy != engine.SupplyDemandRetestHTFPolicy || id.HigherTimeframe != "1d" || id.Daily.Count < 2 {
			return retestOutputError("invalid-derived-value", "htfIdentity")
		}
	} else {
		return retestOutputError("invalid-derived-value", "htfIdentity")
	}
	chartMS := int64(3600000)
	if id.Timeframe == "15m" {
		chartMS = 900000
	} else if id.Timeframe == "30m" {
		chartMS = 1800000
	}
	series := []engine.SupplyDemandRetestSeriesIdentity{id.Chart, id.Source, id.Daily}
	durations := []int64{chartMS, 14400000, 86400000}
	rows := 0
	for i, s := range series {
		if s.Count < 0 || s.Count > 500000 {
			return retestOutputError("invalid-derived-value", "seriesCount")
		}
		rows += s.Count
		if i == 2 && s.Count == 0 {
			if s.FirstT != nil || s.LastT != nil || s.DurationMS != 0 {
				return retestOutputError("invalid-derived-value", "emptySeries")
			}
			continue
		}
		if s.Count < 2 || s.FirstT == nil || s.LastT == nil || s.DurationMS != durations[i] {
			return retestOutputError("invalid-derived-value", "seriesIdentity")
		}
		first, last := *s.FirstT, *s.LastT
		if math.IsNaN(first) || math.IsInf(first, 0) || math.IsNaN(last) || math.IsInf(last, 0) || first < 0 || last <= first || last > float64(1<<53-1)-float64(s.DurationMS) || math.Mod(first, float64(s.DurationMS)) != 0 || math.Mod(last, float64(s.DurationMS)) != 0 {
			return retestOutputError("invalid-derived-value", "seriesTimes")
		}
	}
	if rows > 500000 || id.Chart.Count != len(result.Metrics.Equity) || *id.Chart.FirstT != result.Metrics.Equity[0].T || *id.Chart.LastT != result.Metrics.Equity[len(result.Metrics.Equity)-1].T {
		return retestOutputError("invalid-derived-value", "seriesBinding")
	}
	if run.Costs.StartEquity != result.Metrics.Headline.StartEquity {
		return retestOutputError("invalid-derived-value", "startEquityBinding")
	}
	if run.Costs.FeePerUnit < 0 || run.Costs.Slippage < 0 || run.Costs.SlippageBps < 0 || run.Costs.StartEquity <= 0 || run.Costs.FillOn != "close" && run.Costs.FillOn != "open" && run.Costs.FillOn != "nextOpen" {
		return retestOutputError("invalid-derived-value", "costs")
	}
	cap := result.Capabilities
	if !cap.Headline || !cap.Trades || !cap.Equity || cap.Groupings || cap.Monthly || cap.RDistribution || cap.Portfolio || cap.Live || cap.Paper || cap.Limits != (SupplyDemandRetestLimits{500000, 64 << 20, 1 << 20, 64 << 20, 500000000}) {
		return retestOutputError("invalid-derived-value", "capabilities")
	}
	return nil
}
