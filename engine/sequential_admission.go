package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// sequentialPrepared holds a closed, value-only policy and privately owned bars.
// The generic flag defaults, source projection and off-route shortcuts never
// participate in admission or execution of this family.
type sequentialPrepared struct {
	spec dsl.SequentialFullSpec
}

func sequentialAdmissionError(kind, field string, index int) error {
	return &SequentialFullExecutionError{Kind: kind, Field: field, BarIndex: index}
}

func rejectSequentialFullSurface(cfg dsl.Config, surface string) error {
	if !dsl.IsSequentialFullReserved(cfg) {
		return nil
	}
	if _, err := dsl.DecodeSequentialFullConfig(cfg); err != nil {
		return err
	}
	return sequentialAdmissionError("unsupported-sequential-route", surface, -1)
}

func validateSequentialFullCosts(costs Costs) error {
	if !costs.fillsMarketAtNextOpen() {
		return sequentialAdmissionError("unsupported_config", "costs.fillOn", -1)
	}
	for _, field := range []struct {
		name  string
		value float64
	}{
		{"feePerUnit", costs.FeePerUnit}, {"slippage", costs.Slippage},
		{"slippageBps", costs.SlippageBps}, {"startEquity", costs.StartEquity},
	} {
		if !isFiniteDerivedOutput(field.value) || field.value < 0 {
			return sequentialAdmissionError("unsupported_config", "costs."+field.name, -1)
		}
	}
	return nil
}

func sequentialHasColumns(series marketdata.Series) bool {
	return len(series.T)+len(series.O)+len(series.H)+len(series.L)+len(series.C)+len(series.V) != 0
}

func validateSequentialFullRequest(request RunRequest, spec dsl.SequentialFullSpec) error {
	if request.Symbol != spec.Symbol {
		return sequentialAdmissionError("unsupported-sequential-route", "symbol", -1)
	}
	if request.Timeframe != spec.Timeframe {
		return sequentialAdmissionError("unsupported-sequential-route", "timeframe", -1)
	}
	for _, field := range []struct {
		name    string
		present bool
	}{
		{"timedCalendar", request.TimedCalendar != nil},
		{"sourceTimeframe", request.SourceTimeframe != ""},
		{"higherTimeframe", request.HigherTimeframe != ""},
		{"sourceSeries", sequentialHasColumns(request.SourceSeries)},
		{"higherTimeframeSeries", sequentialHasColumns(request.HTFSeries)},
		{"sourceHigherTimeframeSeries", sequentialHasColumns(request.SourceHTFSeries)},
		{"forceRoute", request.ForceRoute}, {"executionWindow", request.ExecutionWindow != nil},
	} {
		if field.present {
			return sequentialAdmissionError("unsupported-sequential-route", field.name, -1)
		}
	}
	if err := validateSequentialFullCosts(request.Costs); err != nil {
		return err
	}
	series := request.Series
	if err := validateSeriesShape("Sequential", series); err != nil {
		return sequentialAdmissionError("invalid-sequential-input", "series.shape", -1)
	}
	step, ok := dsl.SequentialFullTimeframeMS(spec.Timeframe)
	if !ok {
		return sequentialAdmissionError("unsupported_config", "timeframe", -1)
	}
	const maxExactMS = float64(1<<53 - 1)
	for i, t := range series.T {
		if !isFiniteDerivedOutput(t) || t < 0 || t != math.Trunc(t) || t > maxExactMS-float64(step) {
			return sequentialAdmissionError("invalid-sequential-input", "series.timestamp", i)
		}
		if i > 0 && t != series.T[i-1]+float64(step) {
			kind := "unsupported-sequential-time-gap"
			if t <= series.T[i-1] {
				kind = "invalid-sequential-input"
			}
			return sequentialAdmissionError(kind, "series.timestamp", i)
		}
		o, h, l, c := series.O[i], series.H[i], series.L[i], series.C[i]
		if !isFiniteDerivedOutput(o) || !isFiniteDerivedOutput(h) || !isFiniteDerivedOutput(l) || !isFiniteDerivedOutput(c) || h < l || h < o || h < c || l > o || l > c {
			return sequentialAdmissionError("invalid-sequential-input", "series.ohlc", i)
		}
		if len(series.V) != 0 && (!isFiniteDerivedOutput(series.V[i]) || series.V[i] < 0) {
			return sequentialAdmissionError("invalid-sequential-input", "series.volume", i)
		}
	}
	return nil
}

func prepareSequentialFull(request RunRequest) (*PreparedRun, error) {
	spec, err := dsl.DecodeSequentialFullConfig(request.Config)
	if err != nil {
		return nil, err
	}
	if err := validateSequentialFullRequest(request, spec); err != nil {
		return nil, err
	}
	series := marketdata.Series{
		T: append([]float64(nil), request.Series.T...), O: append([]float64(nil), request.Series.O...),
		H: append([]float64(nil), request.Series.H...), L: append([]float64(nil), request.Series.L...),
		C: append([]float64(nil), request.Series.C...), V: append([]float64(nil), request.Series.V...),
	}
	r := &PreparedRun{
		series: series, fixture: fixtureFromRequest(request),
		params:         flagParams{SetupType: string(dsl.FamilySequentialFull)},
		sequentialFull: &sequentialPrepared{spec: spec},
	}
	if request.ReportTradeContext {
		r.cols = contextcols.Build(series, contextcols.Options{Selective: true, ATRLen: 14, NeedReportTradeContext: true})
	}
	return r, nil
}

func (r *PreparedRun) runSequentialFullChecked(costs Costs) (RunResult, error) {
	if err := validateSequentialFullCosts(costs); err != nil {
		return RunResult{}, err
	}
	fixture := r.fixture
	fixture.Costs = costs.normalized()
	trades, audit, err := runSequentialFull(r.sequentialFull.spec, r.series, fixture)
	if err != nil {
		return RunResult{}, err
	}
	result, err := checkedResultEnvelope(fixture, trades)
	if err != nil {
		return RunResult{}, sequentialAdmissionError("invalid-sequential-derived-value", "trade", -1)
	}
	result.SequentialFull = &audit
	return result, nil
}

func runSequentialFullFixture(fixture RunFixture, cfg dsl.Config) (RunResult, error) {
	if _, err := dsl.DecodeSequentialFullConfig(cfg); err != nil {
		return RunResult{}, err
	}
	if fixture.sequentialEnvelopeError != nil {
		return RunResult{}, fixture.sequentialEnvelopeError
	}
	if fixture.rawRowDefect != "" {
		return RunResult{}, sequentialAdmissionError("invalid-sequential-input", "bars.raw", -1)
	}
	if fixture.RawBars != nil {
		if len(fixture.RawBars) != len(fixture.Bars) {
			return RunResult{}, sequentialAdmissionError("invalid-sequential-input", "bars.shape", -1)
		}
		for i, row := range fixture.RawBars {
			if len(row) != 6 {
				return RunResult{}, sequentialAdmissionError("invalid-sequential-input", "bars.shape", i)
			}
			bar := fixture.Bars[i]
			if row[0] != bar.T || row[1] != bar.O || row[2] != bar.H || row[3] != bar.L || row[4] != bar.C || row[5] != bar.V {
				return RunResult{}, sequentialAdmissionError("invalid-sequential-input", "bars.binding", i)
			}
		}
	}
	if len(fixture.RawSourceBars)+len(fixture.RawHTFBars)+len(fixture.RawSourceHTFBars) != 0 {
		return RunResult{}, sequentialAdmissionError("unsupported-sequential-route", "sourceBars", -1)
	}
	result, err := Run(RunRequest{
		Config: cfg, Series: marketdata.SeriesFromBars(fixture.Bars), StrategyID: fixture.StrategyID,
		Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, Costs: fixture.Costs, RangeMethod: fixture.RangeMethod,
		SourceTimeframe: fixture.SourceTimeframe, HigherTimeframe: fixture.HigherTimeframe,
		TimedCalendar: fixture.TimedCalendar, SourceSeries: marketdata.SeriesFromBars(fixture.SourceBars),
		HTFSeries: marketdata.SeriesFromBars(fixture.HTFBars), SourceHTFSeries: marketdata.SeriesFromBars(fixture.SourceHTFBars),
	})
	if err != nil {
		return RunResult{}, err
	}
	result.Case = fixture.Case
	return result, nil
}

// Kept separate from legacy validation: no existing family acquires these
// input restrictions or execution semantics.
func sequentialFullUnsupportedPreparedRunner(cfg dsl.Config) {
	if err := rejectSequentialFullSurface(cfg, "internal PreparedRunner"); err != nil {
		panic(fmt.Errorf("%w", err))
	}
}

// ValidateSequentialFullJSONEnvelope preserves the closed input boundary before
// permissive Go struct decoding can discard an unknown field or duplicate key.
// Existing families do not consume this family-specific validation result.
func ValidateSequentialFullJSONEnvelope(raw []byte, column bool) error {
	allowed := map[string]bool{"schema": true, "case": true, "strategyId": true, "symbol": true, "timeframe": true, "higherTimeframe": true, "sourceTimeframe": true, "rangeMethod": true, "costs": true}
	if column {
		allowed["context"] = true
	} else {
		for _, key := range []string{"bars", "htfBars", "sourceBars", "sourceHtfBars", "timedCalendar", "source", "contextOptions"} {
			allowed[key] = true
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(string, int) error
	walk = func(path string, depth int) error {
		if depth > 64 {
			return sequentialAdmissionError("invalid-sequential-input", path+".depth", -1)
		}
		token, err := decoder.Token()
		if err != nil {
			return sequentialAdmissionError("invalid-sequential-input", path, -1)
		}
		if !sequentialSourceTokenValid(path, token) {
			return sequentialAdmissionError("invalid-sequential-input", path, -1)
		}
		if path == "$.contextOptions" && token != json.Delim('{') {
			return sequentialAdmissionError("unsupported_config", "contextOptions", -1)
		}
		if path == "$.bars[][0]" {
			number, ok := token.(json.Number)
			if !ok || !sequentialExactJSONTimestamp(string(number)) {
				return sequentialAdmissionError("invalid-sequential-input", "bars.timestamp", -1)
			}
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for decoder.More() {
					keyToken, err := decoder.Token()
					if err != nil {
						return sequentialAdmissionError("invalid-sequential-input", path, -1)
					}
					key, ok := keyToken.(string)
					if !ok || seen[key] {
						return sequentialAdmissionError("invalid-sequential-input", path+".duplicate-key", -1)
					}
					seen[key] = true
					if path == "$" && !allowed[key] {
						return sequentialAdmissionError("unsupported_config", key, -1)
					}
					if path == "$" && key == "timedCalendar" {
						return sequentialAdmissionError("unsupported-sequential-route", key, -1)
					}
					if path == "$.contextOptions" {
						return sequentialAdmissionError("unsupported_config", "contextOptions", -1)
					}
					if path == "$.source" && key != "bars" && key != "htfBars" {
						return sequentialAdmissionError("unsupported_config", "source."+key, -1)
					}
					if path == "$.source.bars" || path == "$.source.htfBars" {
						switch key {
						case "kind", "path", "barCount", "startIndex", "from", "to", "symbol", "timeframe":
						default:
							return sequentialAdmissionError("unsupported_config", path+"."+key, -1)
						}
					}
					if path == "$.costs" {
						switch key {
						case "feePerUnit", "fillOn", "slippage", "slippageBps", "startEquity":
						default:
							return sequentialAdmissionError("unsupported_config", "costs."+key, -1)
						}
					}
					if err := walk(path+"."+key, depth+1); err != nil {
						return err
					}
				}
				if path == "$.source" && (!seen["bars"] || !seen["htfBars"]) {
					return sequentialAdmissionError("invalid-sequential-input", path, -1)
				}
				if (path == "$.source.bars" || path == "$.source.htfBars") && !seen["kind"] {
					return sequentialAdmissionError("invalid-sequential-input", path+".kind", -1)
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim('}') {
					return sequentialAdmissionError("invalid-sequential-input", path, -1)
				}
			case '[':
				if path == "$" {
					return sequentialAdmissionError("invalid-sequential-input", "envelope", -1)
				}
				index := 0
				for decoder.More() {
					child := path + "[]"
					if path == "$.bars[]" {
						child = fmt.Sprintf("%s[%d]", path, index)
					}
					if err := walk(child, depth+1); err != nil {
						return err
					}
					index++
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim(']') {
					return sequentialAdmissionError("invalid-sequential-input", path, -1)
				}
			default:
				return sequentialAdmissionError("invalid-sequential-input", path, -1)
			}
		} else if path == "$" || (token == nil && (path == "$.costs" || strings.HasPrefix(path, "$.costs.") || strings.HasPrefix(path, "$.bars"))) {
			return sequentialAdmissionError("invalid-sequential-input", path, -1)
		}
		return nil
	}
	if err := walk("$", 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return sequentialAdmissionError("invalid-sequential-input", "trailing JSON", -1)
	}
	return nil
}

// Check integral milliseconds before binary64 decoding can round a fractional
// decimal into an integer. Normalize the finite decimal spelling directly:
// no exponent-sized allocation or arbitrary-precision exponent expansion.
func sequentialExactJSONTimestamp(raw string) bool {
	negative := strings.HasPrefix(raw, "-")
	raw = strings.TrimPrefix(raw, "-")
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(raw), "e")
	fractionDigits := 0
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		fractionDigits = len(mantissa) - dot - 1
		mantissa = mantissa[:dot] + mantissa[dot+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return true
	}
	if negative {
		return false
	}
	exponent := int64(0)
	if hasExponent {
		var err error
		exponent, err = strconv.ParseInt(exponentText, 10, 64)
		if err != nil {
			return false
		}
	}
	// A nonzero safe integer cannot need a scale outside the token's digit
	// count and the 16 decimal digits of the maximum exact timestamp.
	if exponent < -int64(len(raw)) || exponent > int64(len(raw))+16 {
		return false
	}
	scale := exponent - int64(fractionDigits)
	if scale < 0 {
		remove := -scale
		if remove >= int64(len(digits)) {
			return false
		}
		cut := len(digits) - int(remove)
		if strings.Trim(digits[cut:], "0") != "" {
			return false
		}
		digits = digits[:cut]
	} else {
		if int64(len(digits))+scale > 16 {
			return false
		}
		digits += strings.Repeat("0", int(scale))
	}
	if len(digits) > 16 {
		return false
	}
	value, err := strconv.ParseUint(digits, 10, 64)
	return err == nil && value <= 1<<53-1
}

// Optional source provenance follows the existing fixture schema and never
// supplies route, calendar, indicator or policy inputs.
func sequentialSourceTokenValid(path string, token any) bool {
	if path == "$.source" || path == "$.source.bars" {
		return token == json.Delim('{')
	}
	if path == "$.source.htfBars" {
		return token == nil || token == json.Delim('{')
	}
	field, isRef := strings.CutPrefix(path, "$.source.bars.")
	if !isRef {
		field, isRef = strings.CutPrefix(path, "$.source.htfBars.")
	}
	if !isRef {
		return true
	}
	switch field {
	case "kind", "path", "symbol", "timeframe":
		value, ok := token.(string)
		return ok && value != ""
	case "barCount", "startIndex":
		value, ok := token.(json.Number)
		return ok && sequentialExactJSONTimestamp(string(value))
	case "from", "to":
		value, ok := token.(json.Number)
		if !ok {
			return false
		}
		number, err := value.Float64()
		return err == nil && isFiniteDerivedOutput(number)
	default:
		return false
	}
}
