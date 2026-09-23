package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// RunRequest describes a direct engine run over already-loaded market columns.
type RunRequest struct {
	Config             dsl.Config
	Series             marketdata.Series
	SourceSeries       marketdata.Series
	HTFSeries          marketdata.Series
	SourceHTFSeries    marketdata.Series
	StrategyID         string
	Symbol             string
	Timeframe          string
	SourceTimeframe    string
	HigherTimeframe    string
	RangeMethod        string
	ReportTradeContext bool
	// ForceRoute permits an explicit transfer run outside the declared route
	// allowlist without changing the strategy config.
	ForceRoute bool
	// ExecutionWindow keeps context bars available to indicators while
	// restricting trade entry, management, and liquidation to the declared
	// half-open trade interval [TradeFromT, TradeToT).
	ExecutionWindow *ExecutionWindow
	Costs           Costs
}

// PreparedRun holds context columns and derived setup state for repeated runs
// of one config/route. It is not safe for concurrent use.
type PreparedRun struct {
	series      marketdata.Series
	cols        contextcols.Columns
	htfTrend    []int8
	ema         []float64
	emaSlope    []float64
	params      flagParams
	fixture     RunFixture
	offRoute    bool
	broker      broker
	trades      []Trade
	c5Source    marketdata.Series
	c5SourceHTF marketdata.Series
	c5Config    dsl.Config
	c5          bool
	execution   ExecutionBounds
	windowed    bool
}

// ReportTradeContext exposes the canonical context labels for one executed
// trade index without leaking the columnar representation to report adapters.
type ReportTradeContext struct {
	OpenLocation string
	PriorDayType string
}

func (r *PreparedRun) ReportTradeContext(index int) (ReportTradeContext, bool) {
	if r == nil || index < 0 || index >= len(r.cols.OpenLocation) || index >= len(r.cols.PriorDayType) {
		return ReportTradeContext{}, false
	}
	return ReportTradeContext{
		OpenLocation: openLocationName(r.cols.OpenLocation[index]),
		PriorDayType: priorDayTypeName(r.cols.PriorDayType[index]),
	}, true
}

// SharedRunContext holds immutable route context that can be reused by
// independent variant runners.
type SharedRunContext struct {
	series       marketdata.Series
	sourceSeries marketdata.Series
	cols         contextcols.Columns
	htfTrend     []int8
	fixture      RunFixture
	options      contextcols.Options
	execution    ExecutionBounds
	windowed     bool
	forceRoute   bool
}

// SharedContextKey returns a stable key for the context columns a request needs.
func SharedContextKey(request RunRequest) (string, error) {
	if err := validateRunRequest(request); err != nil {
		return "", err
	}
	if _, _, err := sourceSeriesForRequest(request); err != nil {
		return "", err
	}
	execution, err := ResolveExecutionWindow(request.Series, request.ExecutionWindow)
	if err != nil {
		return "", err
	}
	fixture := fixtureFromRequest(request)
	options := contextOptions(fixture, request.Config)
	options.NeedReportTradeContext = request.ReportTradeContext
	identity := struct {
		Options         contextcols.Options
		SourceTimeframe string
		Execution       ExecutionBounds
		Windowed        bool
	}{Options: options, SourceTimeframe: fixture.SourceTimeframe, Execution: execution, Windowed: request.ExecutionWindow != nil}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// PrepareSharedRunContext builds immutable context columns and higher-timeframe
// state once for a route/config context group.
func PrepareSharedRunContext(request RunRequest) (*SharedRunContext, error) {
	if sourceEntryRequest(request) {
		return nil, errors.New("XAUUSD source-entry execution is not supported by shared grid contexts; use PrepareRun/report or refuse the route")
	}
	if err := validateRunRequest(request); err != nil {
		return nil, err
	}
	series := normalizeOptionalVolume(request.Series)
	sourceSeries, sourceHTFSeries, err := sourceSeriesForRequest(request)
	if err != nil {
		return nil, err
	}
	sourceSeries = normalizeOptionalVolume(sourceSeries)
	sourceHTFSeries = normalizeOptionalVolume(sourceHTFSeries)
	fixture := fixtureFromRequest(request)
	options := contextOptions(fixture, request.Config)
	options.NeedReportTradeContext = request.ReportTradeContext
	execution, err := ResolveExecutionWindow(series, request.ExecutionWindow)
	if err != nil {
		return nil, err
	}
	return &SharedRunContext{
		series:       series,
		sourceSeries: sourceSeries,
		cols:         projectSourceColumns(series, sourceSeries, contextcols.Build(sourceSeries, options)),
		htfTrend:     projectSourceInt8(series, sourceSeries, computeHTFTrend(sourceSeries, sourceHTFSeries)),
		fixture:      fixture,
		options:      options,
		execution:    execution,
		windowed:     request.ExecutionWindow != nil,
		forceRoute:   request.ForceRoute,
	}, nil
}

// PrepareVariant creates a per-variant runner that shares immutable context
// state while owning its broker and trade buffers.
func (s *SharedRunContext) PrepareVariant(cfg dsl.Config) (*PreparedRun, error) {
	if s == nil {
		return nil, errors.New("shared run context is required")
	}
	if cfg == nil {
		return nil, errors.New("engine config is required")
	}
	setupType := setupTypeFromAny(cfg["setupType"])
	if !implementedFamily(setupType) {
		return nil, fmt.Errorf("setup family %q is not implemented", setupType)
	}
	if err := validateDoubleTopBottomPivotWindow(cfg); err != nil {
		return nil, err
	}
	if err := validateSharedAdmissionLists(cfg); err != nil {
		return nil, err
	}
	if err := validateSessionBreakHoldLevelPriority(cfg); err != nil {
		return nil, err
	}
	options := contextOptions(s.fixture, cfg)
	options.NeedReportTradeContext = s.options.NeedReportTradeContext
	if !reflect.DeepEqual(options, s.options) {
		return nil, errors.New("variant config requires different context columns")
	}
	params := paramsFromConfig(cfg)
	// Context and setup indicators are source-derived; the runner still walks
	// chart bars, preserving C4's no-entry-scheduler boundary.
	ema, emaSlope := computeSetupEMA(s.sourceSeries, params)
	if s.sourceSeries.Len() != s.series.Len() {
		ema = projectSourceFloat64(s.series, s.sourceSeries, ema)
		emaSlope = projectSourceFloat64(s.series, s.sourceSeries, emaSlope)
	}
	return &PreparedRun{
		series:    s.series,
		cols:      s.cols,
		htfTrend:  s.htfTrend,
		ema:       ema,
		emaSlope:  emaSlope,
		params:    params,
		fixture:   s.fixture,
		offRoute:  !s.forceRoute && !RouteAllowed(cfg, s.fixture.Symbol, s.fixture.Timeframe, s.series),
		trades:    make([]Trade, 0, 32),
		execution: s.execution,
		windowed:  s.windowed,
	}, nil
}

// PrepareRun builds the causal context columns and setup state for a strategy.
func PrepareRun(request RunRequest) (*PreparedRun, error) {
	if sourceEntryRequest(request) {
		if err := validateRunRequest(request); err != nil {
			return nil, err
		}
		source, sourceHTF, err := sourceSeriesForRequest(request)
		if err != nil {
			return nil, err
		}
		if source.Len() == 0 {
			return nil, errors.New("XAUUSD source-entry execution requires source market series")
		}
		fixture := fixtureFromRequest(request)
		execution, err := ResolveExecutionWindow(request.Series, request.ExecutionWindow)
		if err != nil {
			return nil, err
		}
		return &PreparedRun{
			series: request.Series, params: paramsFromConfig(request.Config), fixture: fixture,
			c5Source: source, c5SourceHTF: sourceHTF,
			c5Config:  request.Config,
			c5:        true,
			trades:    make([]Trade, 0, 32),
			execution: execution,
			windowed:  request.ExecutionWindow != nil,
		}, nil
	}
	shared, err := PrepareSharedRunContext(request)
	if err != nil {
		return nil, err
	}
	return shared.PrepareVariant(request.Config)
}

func sourceEntryRequest(request RunRequest) bool {
	entry := stringValue(request.Config, "entryTf", "current")
	return request.Timeframe == entry && supportedSourceEntryRoute(request.Symbol, sourceTimeframeFromConfig(request.Config), entry)
}

func validateRunRequest(request RunRequest) error {
	if request.Config == nil {
		return errors.New("engine config is required")
	}
	if request.Series.Len() == 0 {
		return errors.New("market series is empty")
	}
	setupType := setupTypeFromAny(request.Config["setupType"])
	if !implementedFamily(setupType) {
		return fmt.Errorf("setup family %q is not implemented", setupType)
	}
	if err := validateDoubleTopBottomPivotWindow(request.Config); err != nil {
		return err
	}
	if err := validateSharedAdmissionLists(request.Config); err != nil {
		return err
	}
	if err := validateSessionBreakHoldLevelPriority(request.Config); err != nil {
		return err
	}
	if err := validateSeriesShape("market", request.Series); err != nil {
		return err
	}
	if err := validateSeriesShape("higher-timeframe", request.HTFSeries); err != nil {
		return err
	}
	if err := validateSeriesValues("market", request.Series); err != nil {
		return err
	}
	if err := validateSeriesValues("higher-timeframe", request.HTFSeries); err != nil {
		return err
	}
	if sourceTimeframeFromConfig(request.Config) != "" {
		if request.SourceSeries.Len() == 0 {
			return errors.New("source market series is required for explicit source timeframe")
		}
		if err := validateSeriesShape("source market", request.SourceSeries); err != nil {
			return err
		}
		if err := validateSeriesShape("source higher-timeframe", request.SourceHTFSeries); err != nil {
			return err
		}
		if err := validateSeriesValues("source market", request.SourceSeries); err != nil {
			return err
		}
		if err := validateSeriesValues("source higher-timeframe", request.SourceHTFSeries); err != nil {
			return err
		}
	}
	return nil
}

func validateDoubleTopBottomPivotWindow(cfg dsl.Config) error {
	if setupTypeFromAny(cfg["setupType"]) != string(dsl.FamilyDoubleTopBottom) {
		return nil
	}
	doubleTopBottom := mapValue(cfg, "doubleTopBottom")
	value, present := doubleTopBottom["pivotWindow"]
	if present && intFromAny(value, 0) < 0 {
		return errors.New("engine config doubleTopBottom.pivotWindow must be non-negative")
	}
	return nil
}

func validateSeriesShape(label string, series marketdata.Series) error {
	timestampLength := len(series.T)
	for _, column := range []struct {
		name   string
		length int
	}{
		{name: "open", length: len(series.O)},
		{name: "high", length: len(series.H)},
		{name: "low", length: len(series.L)},
		{name: "close", length: len(series.C)},
	} {
		if column.length != timestampLength {
			return fmt.Errorf("%s series %s column length %d does not match timestamp length %d", label, column.name, column.length, timestampLength)
		}
	}
	if volumeLength := len(series.V); volumeLength != 0 && volumeLength != timestampLength {
		return fmt.Errorf("%s series volume column length %d must be zero or match timestamp length %d", label, volumeLength, timestampLength)
	}
	return nil
}

func validateSeriesValues(label string, series marketdata.Series) error {
	for _, column := range []struct {
		name   string
		values []float64
	}{
		{name: "timestamp", values: series.T},
		{name: "open", values: series.O},
		{name: "high", values: series.H},
		{name: "low", values: series.L},
		{name: "close", values: series.C},
	} {
		for i, value := range column.values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("%s series %s column contains non-finite value at index %d", label, column.name, i)
			}
		}
	}
	return nil
}

func normalizeOptionalVolume(series marketdata.Series) marketdata.Series {
	if series.Len() == 0 || len(series.V) != 0 {
		return series
	}
	series.V = make([]float64, series.Len())
	for i := range series.V {
		series.V[i] = math.NaN()
	}
	return series
}

func fixtureFromRequest(request RunRequest) RunFixture {
	if request.RangeMethod == "" {
		request.RangeMethod = "zone"
	}
	return RunFixture{
		StrategyID:      request.StrategyID,
		Symbol:          request.Symbol,
		Timeframe:       request.Timeframe,
		SourceTimeframe: resolvedSourceTimeframe(request.Timeframe, request.SourceTimeframe, request.Config),
		HigherTimeframe: request.HigherTimeframe,
		RangeMethod:     request.RangeMethod,
		Costs:           request.Costs.normalized(),
	}
}

func sourceTimeframeFromConfig(cfg dsl.Config) string {
	value, _ := cfg["sourceTimeframe"].(string)
	return value
}

func resolvedSourceTimeframe(chartTimeframe, requested string, cfg dsl.Config) string {
	if source := sourceTimeframeFromConfig(cfg); source != "" {
		return source
	}
	if requested != "" {
		return requested
	}
	return chartTimeframe
}

func sourceSeriesForRequest(request RunRequest) (marketdata.Series, marketdata.Series, error) {
	if sourceTimeframeFromConfig(request.Config) == "" {
		return request.Series, request.HTFSeries, nil
	}
	if request.SourceTimeframe != "" && request.SourceTimeframe != sourceTimeframeFromConfig(request.Config) {
		return marketdata.Series{}, marketdata.Series{}, errors.New("source timeframe does not match strategy configuration")
	}
	return request.SourceSeries, request.SourceHTFSeries, nil
}

func (r *PreparedRun) runRaw(costs Costs) []Trade {
	r.fixture.Costs = costs.normalized()
	if r.offRoute {
		r.trades = r.trades[:0]
		return r.trades
	}
	if r.c5 {
		// PrepareRun validates and binds source data before this point. Keep C5
		// structurally unable to fall through to chart-timeframe execution.
		if r.c5Source.Len() == 0 {
			r.trades = r.trades[:0]
			return r.trades
		}
		r.fixture.Costs = costs.normalized()
		trades, err := runSourceEntrySeries(r.fixture, r.c5Config, r.params, r.series, r.c5Source, r.c5SourceHTF, r.execution, r.windowed)
		if err != nil {
			r.trades = r.trades[:0]
			return r.trades
		}
		r.trades = trades
		return r.trades
	}
	r.broker.reset(r.series, r.cols, r.htfTrend, r.ema, r.emaSlope, r.params, r.fixture, r.trades)
	if r.windowed {
		r.broker.setExecutionWindow(r.execution)
	}
	r.trades = r.broker.run()
	return r.trades
}

// Run executes this prepared strategy with the supplied costs and preserves
// the legacy JSON-safe serialization behavior for derived non-finite values.
func (r *PreparedRun) Run(costs Costs) RunResult {
	trades := r.runRaw(costs)
	return resultEnvelope(r.fixture, trades)
}

// RunChecked executes this prepared strategy and rejects non-finite derived
// output before applying the existing JSON serialization.
func (r *PreparedRun) RunChecked(costs Costs) (RunResult, error) {
	trades := r.runRaw(costs)
	return checkedResultEnvelope(r.fixture, trades)
}

// Run executes a direct engine request in one call.
func Run(request RunRequest) (RunResult, error) {
	prepared, err := PrepareRun(request)
	if err != nil {
		return RunResult{}, err
	}
	return prepared.RunChecked(request.Costs)
}
