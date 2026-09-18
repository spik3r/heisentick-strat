package engine

import (
	"fmt"
	"math"
	"time"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const dayMS = float64(24 * 60 * 60 * 1000)

// spansWeekendClosure mirrors engine/context/sessionCalendar.js: true when
// [startMs, endMs) overlaps a weekend market closure (any Saturday UTC), which
// is a scheduled closure rather than a missing HTF bar.
func spansWeekendClosure(startMs, endMs float64) bool {
	if !(endMs > startMs) {
		return false
	}
	if endMs-startMs >= 7*dayMS {
		return true
	}
	day := math.Floor(startMs/dayMS) * dayMS
	for ; day < endMs; day += dayMS {
		if time.UnixMilli(int64(day)).UTC().Weekday() == time.Saturday && day+dayMS > startMs {
			return true
		}
	}
	return false
}

// RunFixture executes the fixture's DSL source and returns the conformance envelope.
func RunFixtureCase(fixture RunFixture, source string) (RunResult, error) {
	parsed, err := dsl.Parse(source)
	if err != nil {
		return RunResult{}, err
	}
	if len(parsed.Errors) > 0 {
		return RunResult{}, fmt.Errorf("%s: DSL parse errors: %v", fixture.Case, parsed.Errors)
	}
	if setupType := setupTypeFromAny(parsed.Config["setupType"]); !implementedFamily(setupType) {
		return RunResult{}, fmt.Errorf("%s: setup family %q is not implemented", fixture.Case, setupType)
	}
	series := marketdata.SeriesFromBars(fixture.Bars)
	sourceSeries, sourceHTFSeries, err := sourceSeriesForFixture(fixture, parsed.Config)
	if err != nil {
		return RunResult{}, err
	}
	if !RouteAllowed(parsed.Config, fixture.Symbol, fixture.Timeframe, series) {
		return checkedResultEnvelope(fixture, nil)
	}

	params := paramsFromConfig(parsed.Config)
	if sourceEntryConfig(parsed.Config, fixture) {
		return runSourceEntryFixture(fixture, parsed.Config, params, series, sourceSeries, sourceHTFSeries)
	}
	cols := projectSourceColumns(series, sourceSeries, contextcols.Build(sourceSeries, contextOptions(fixture, parsed.Config)))
	htfTrend := projectSourceInt8(series, sourceSeries, computeHTFTrend(sourceSeries, sourceHTFSeries))
	ema, emaSlope := computeSetupEMA(sourceSeries, params)
	if sourceSeries.Len() != series.Len() {
		ema = projectSourceFloat64(series, sourceSeries, ema)
		emaSlope = projectSourceFloat64(series, sourceSeries, emaSlope)
	}
	var b broker
	b.reset(series, cols, htfTrend, ema, emaSlope, params, fixture, nil)
	trades := b.run()
	return checkedResultEnvelope(fixture, trades)
}

func sourceEntryConfig(cfg dsl.Config, fixture RunFixture) bool {
	entryTf, _ := cfg["entryTf"].(string)
	sourceTf, _ := cfg["sourceTimeframe"].(string)
	return fixture.Timeframe == entryTf && supportedSourceEntryRoute(fixture.Symbol, sourceTf, entryTf)
}

func runSourceEntryFixture(fixture RunFixture, cfg dsl.Config, params flagParams, chart, source, sourceHTF marketdata.Series) (RunResult, error) {
	trades, err := runSourceEntrySeries(fixture, cfg, params, chart, source, sourceHTF)
	if err != nil {
		return RunResult{}, err
	}
	return checkedResultEnvelope(fixture, trades)
}

func runSourceEntrySeries(fixture RunFixture, cfg dsl.Config, params flagParams, chart, source, sourceHTF marketdata.Series) ([]Trade, error) {
	sourceFixture := fixture
	sourceFixture.Timeframe, _ = cfg["sourceTimeframe"].(string)
	sourceFixture.SourceTimeframe = ""
	sourceFixture.Bars = fixture.SourceBars
	sourceFixture.HTFBars = fixture.SourceHTFBars
	sourceCols := contextcols.Build(source, contextOptions(sourceFixture, cfg))
	sourceHTFTrend := computeHTFTrend(source, sourceHTF)
	var sourceBroker broker
	sourceBroker.reset(source, sourceCols, sourceHTFTrend, nil, nil, params, sourceFixture, nil)
	orders := sourceBroker.runCapturedSource()
	indices := make([]int, len(orders))
	for i, order := range orders {
		indices[i] = order.Index
	}
	entries := ScheduleSourceEvents(source, chart, indices)
	chartCols := projectSourceColumns(chart, source, sourceCols)
	chartHTFTrend := projectSourceInt8(chart, source, sourceHTFTrend)
	var chartBroker broker
	chartBroker.reset(chart, chartCols, chartHTFTrend, nil, nil, params, fixture, nil)
	trades := chartBroker.runScheduled(entries, orders)
	return trades, nil
}

func implementedFamily(setupType string) bool {
	switch setupType {
	case string(dsl.FamilyFlagContinuation),
		string(dsl.FamilyRangeBreakFake),
		string(dsl.FamilyOpeningRangeBreakout),
		string(dsl.FamilyInsideDayExpansion),
		string(dsl.FamilyDayOpenReclaim),
		string(dsl.FamilyBreakRetest),
		string(dsl.FamilySessionBreakHold),
		string(dsl.FamilyTrendPullback),
		string(dsl.FamilySupplyDemand),
		string(dsl.FamilyDoubleTopBottom),
		string(dsl.FamilyFibContinuation),
		string(dsl.FamilyChannelBreakHold),
		string(dsl.FamilyFailedBreakout),
		string(dsl.FamilyTriplePushExhaustion),
		string(dsl.FamilyVWAPExtensionFade),
		string(dsl.FamilyVolumeAnomalyExhaustion),
		string(dsl.FamilyElderTripleScreen),
		string(dsl.FamilyPriceMomentum),
		string(dsl.FamilyDailyFlushFailure),
		string(dsl.FamilyFairValueGap),
		string(dsl.FamilyWeekendExtremeFade),
		string(dsl.FamilyIntraHourRunExhaustion),
		string(dsl.FamilyDualEMAResumption),
		string(dsl.FamilySMAGoldenCross),
		string(dsl.FamilyKeltnerReversion),
		string(dsl.FamilyKeltnerExpansion):
		return true
	default:
		return false
	}
}

type PreparedRunner struct {
	series   marketdata.Series
	cols     contextcols.Columns
	htfTrend []int8
	ema      []float64
	emaSlope []float64
	params   flagParams
	fixture  RunFixture
	offRoute bool
	broker   broker
	trades   []Trade
}

func newPreparedRunner(fixture RunFixture, cfg dsl.Config) PreparedRunner {
	series := marketdata.SeriesFromBars(fixture.Bars)
	sourceSeries, sourceHTFSeries, err := sourceSeriesForFixture(fixture, cfg)
	if err != nil {
		panic(err)
	}
	params := paramsFromConfig(cfg)
	ema, emaSlope := computeSetupEMA(sourceSeries, params)
	if sourceSeries.Len() != series.Len() {
		ema = projectSourceFloat64(series, sourceSeries, ema)
		emaSlope = projectSourceFloat64(series, sourceSeries, emaSlope)
	}
	return PreparedRunner{
		series:   series,
		cols:     projectSourceColumns(series, sourceSeries, contextcols.Build(sourceSeries, contextOptions(fixture, cfg))),
		htfTrend: projectSourceInt8(series, sourceSeries, computeHTFTrend(sourceSeries, sourceHTFSeries)),
		ema:      ema,
		emaSlope: emaSlope,
		params:   params,
		fixture:  fixture,
		offRoute: !RouteAllowed(cfg, fixture.Symbol, fixture.Timeframe, series),
		trades:   make([]Trade, 0, 32),
	}
}

func sourceSeriesForFixture(fixture RunFixture, cfg dsl.Config) (marketdata.Series, marketdata.Series, error) {
	if sourceTimeframeFromConfig(cfg) == "" {
		return marketdata.SeriesFromBars(fixture.Bars), marketdata.SeriesFromBars(fixture.HTFBars), nil
	}
	if fixture.SourceTimeframe != "" && fixture.SourceTimeframe != sourceTimeframeFromConfig(cfg) {
		return marketdata.Series{}, marketdata.Series{}, fmt.Errorf("%s: source timeframe does not match strategy configuration", fixture.Case)
	}
	if len(fixture.SourceBars) == 0 {
		return marketdata.Series{}, marketdata.Series{}, fmt.Errorf("%s: source bars are required for explicit source timeframe", fixture.Case)
	}
	return marketdata.SeriesFromBars(fixture.SourceBars), marketdata.SeriesFromBars(fixture.SourceHTFBars), nil
}

func (r *PreparedRunner) RunPrepared() []Trade {
	if r.offRoute {
		r.trades = r.trades[:0]
		return r.trades
	}
	r.broker.reset(r.series, r.cols, r.htfTrend, r.ema, r.emaSlope, r.params, r.fixture, r.trades)
	r.trades = r.broker.run()
	return r.trades
}

func computeSetupEMA(series marketdata.Series, params flagParams) ([]float64, []float64) {
	if params.SetupType != string(dsl.FamilyTrendPullback) {
		return nil, nil
	}
	ema := computeEMA(series, params.TPBEMALen)
	return ema, computeSlope(ema, params.TPBEMASlopeLen)
}

func computeEMA(series marketdata.Series, length int) []float64 {
	n := series.Len()
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	if length < 1 {
		length = 1
	}
	k := 2 / (float64(length) + 1)
	value := series.C[0]
	out[0] = value
	for i := 1; i < n; i++ {
		value += k * (series.C[i] - value)
		out[i] = value
	}
	return out
}

func computeSlope(values []float64, length int) []float64 {
	out := make([]float64, len(values))
	for i := range out {
		out[i] = math.NaN()
	}
	if length < 1 {
		length = 1
	}
	for i := length; i < len(values); i++ {
		if isFinite(values[i]) && isFinite(values[i-length]) {
			out[i] = values[i] - values[i-length]
		}
	}
	return out
}

func contextOptions(fixture RunFixture, cfg dsl.Config) contextcols.Options {
	channel := mapValue(cfg, "channel")
	elder := mapValue(cfg, "elderTripleScreen")
	target := mapValue(cfg, "target")
	setupType := setupTypeFromAny(cfg["setupType"])
	emaLen := 0
	emaSlopeLen := 0
	if setupType == string(dsl.FamilyElderTripleScreen) {
		emaLen = intValue(elder, "emaLen", intFromAny(cfg["emaLen"], 21))
		emaSlopeLen = intValue(elder, "emaSlopeLen", 5)
	}
	options := contextcols.Options{
		EMAFastLen:      emaLen,
		EMAFastSlopeLen: emaSlopeLen,
		Range: contextcols.RangeOptions{
			Method: fixture.RangeMethod,
		},
		Channel: contextcols.ChannelOptions{
			Enabled:     setupType == string(dsl.FamilyChannelBreakHold) || boolValue(channel, "enabled", false) || stringValue(target, "edge", "range") == "channel",
			MinWidthATR: numberValue(channel, "minWidthAtr", 0),
			MaxWidthATR: numberValue(channel, "maxWidthAtr", 0),
			Lookback:    intValue(channel, "lookback", 0),
			MinSpan:     intValue(channel, "minSpan", 0),
			Source:      stringValue(channel, "source", ""),
		},
	}
	if setupType == string(dsl.FamilyFlagContinuation) {
		flag := mapValue(cfg, "flag")
		options.Selective = true
		options.NeedPriorDay = true
		options.NeedRegimeTrend = true
		options.NeedRangeActive = true
		options.NeedOpenLocation = len(stringSliceValue(cfg["dayThemes"])) > 0
		options.NeedSessionPhase = boolValue(flag, "avoidLunchBreakouts", false) || len(stringSliceValue(cfg["sessionPhases"])) > 0
	}
	return options
}

// inferSeriesDurationMs returns the smallest positive spacing between
// consecutive bars, which is the bar duration for a fixed-timeframe series
// (gaps are multiples and never win). Returns 0 when it cannot be determined.
func inferSeriesDurationMs(s marketdata.Series) float64 {
	dur := 0.0
	for i := 1; i < s.Len(); i++ {
		d := s.T[i] - s.T[i-1]
		if d > 0 && (dur == 0 || d < dur) {
			dur = d
		}
	}
	return dur
}

// computeHTFTrend mirrors the JS causal completed-bar projection
// (plans/2026-07-15-causal-mtf-semantics-roadmap.md). A nil result means HTF
// was not requested for the run; otherwise index i holds trendUp/trendDown/
// trendFlat for the completed HTF bar projected at the primary bar's close, or
// htfUnavailable (0) when no completed bar is available (warmup or a source gap).
func computeHTFTrend(series marketdata.Series, htf marketdata.Series) []int8 {
	n := series.Len()
	if n == 0 || htf.Len() == 0 {
		return nil
	}
	out := make([]int8, n) // 0 == htfUnavailable
	const biasBars = 24
	const biasATR = 0.5
	primaryDur := inferSeriesDurationMs(series)
	htfDur := inferSeriesDurationMs(htf)
	if primaryDur <= 0 || htfDur <= 0 {
		return out
	}
	atr := contextcols.ComputeATR(htf, 14)
	p := -1
	for i := 0; i < n; i++ {
		decision := series.T[i] + primaryDur // primary bar close
		for p+1 < htf.Len() && htf.T[p+1]+htfDur <= decision {
			p++
		}
		if p < 0 {
			continue // no completed HTF bar yet
		}
		projClose := htf.T[p] + htfDur
		if decision-projClose >= htfDur {
			// Stale projection: unavailable only for a genuine intraday hole. A
			// hole spanning a weekend closure is legitimate; reuse completed bar.
			holeEnd := decision
			if p+1 < htf.Len() {
				holeEnd = htf.T[p+1]
			}
			if !spansWeekendClosure(projClose, holeEnd) {
				continue
			}
		}
		if p < biasBars {
			continue
		}
		delta := htf.C[p] - htf.C[p-biasBars]
		threshold := atr[p] * biasATR
		switch {
		case delta > threshold:
			out[i] = trendUp
		case delta < -threshold:
			out[i] = trendDown
		default:
			out[i] = trendFlat
		}
	}
	return out
}

func resultEnvelope(fixture RunFixture, trades []Trade) RunResult {
	serialized := serializeTrades(trades)
	out := RunResult{
		Case:            fixture.Case,
		Costs:           fixture.Costs.normalized(),
		HigherTimeframe: fixture.HigherTimeframe,
		RangeMethod:     fixture.RangeMethod,
		Schema:          tradesSchema,
		StrategyID:      fixture.StrategyID,
		Symbol:          fixture.Symbol,
		Timeframe:       fixture.Timeframe,
		TradeCount:      len(serialized),
		Trades:          serialized,
	}
	if out.Trades == nil {
		out.Trades = []Trade{}
	}
	return out
}
