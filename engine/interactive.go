package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const InteractiveRunSchema = "dsl-interactive-run-v1"

// dslEditorStrategy is an app-owned mutable editor surface. Only the exact
// DEFAULT_SPEC_TEXT snapshot reviewed for this profile may use that ID here;
// edits require a newly qualified source/profile instead of inheriting the
// default's interactive capability.
const editorDefaultSourceSHA256 = "329a0750d4f5770209d8a92951b36e7b02206ef3c72e4f3e35e396104efa1521"

// InteractiveVPNYHandoffSchema identifies the Go-owned composition of the
// archived balanced Session Break Hold DSL and its JavaScript VP handoff veto.
const InteractiveVPNYHandoffSchema = "dsl-interactive-vp-ny-handoff-v1"

const vpNYHandoffBaseSourceSHA256 = "c3fa897885f84b61d718b364e78448457016814efd101125333a87df7878a405"
const vpNYHandoffStrategyID = "dslSessionBreakHoldNyFocusBalancedFifteenVpVeto"

var ErrInteractiveUnsupported = errors.New("interactive route unsupported")

// InteractiveProvenance identifies the exact inputs interpreted by this run.
// The host adds its verified native/WASM artifact digest when presenting it.
type InteractiveProvenance struct {
	FixtureSHA256 string `json:"fixtureSha256"`
	SourceSHA256  string `json:"sourceSha256"`
}

// InteractiveStats uses fee-inclusive trade outcomes and cash equity. Drawdown
// starts at startEquity, observes each bar mark, then observes the final forced
// liquidation cash balance. The absolute and percentage maxima are independent
// maxima against the running peak, not a final-peak denominator.
type InteractiveStats struct {
	Trades            int      `json:"trades"`
	Wins              int      `json:"wins"`
	Losses            int      `json:"losses"`
	WinRate           float64  `json:"winRate"`
	Net               float64  `json:"net"`
	TradeNet          float64  `json:"tradeNet"`
	GrossWin          float64  `json:"grossWin"`
	GrossLoss         float64  `json:"grossLoss"`
	ProfitFactor      *float64 `json:"profitFactor"`
	ProfitFactorState string   `json:"profitFactorState"`
	AvgWin            float64  `json:"avgWin"`
	AvgLoss           float64  `json:"avgLoss"`
	Expectancy        float64  `json:"expectancy"`
	MaxDD             float64  `json:"maxDD"`
	MaxDDpct          float64  `json:"maxDDpct"`
	MaxClosedDD       float64  `json:"maxClosedDD"`
	MaxClosedDDpct    float64  `json:"maxClosedDDpct"`
	MaxWinStreak      int      `json:"maxWinStreak"`
	MaxLossStreak     int      `json:"maxLossStreak"`
	WorstLoss         float64  `json:"worstLoss"`
	AvgHoldBars       float64  `json:"avgHoldBars"`
	EndEquity         float64  `json:"endEquity"`
	ReturnPct         float64  `json:"returnPct"`
}

// InteractiveRunResult is a new, versioned chart result. Run retains the
// existing conformance trade envelope, while the curves and statistics use
// the explicitly defined interactive accounting above.
type InteractiveRunResult struct {
	Schema           string                `json:"schema"`
	Provenance       InteractiveProvenance `json:"provenance"`
	Run              RunResult             `json:"run"`
	TradeNetPnL      []float64             `json:"tradeNetPnl"`
	EquityCurve      []float64             `json:"equityCurve"`
	ClosedEquity     []float64             `json:"closedEquityCurve"`
	CashEndEquity    float64               `json:"cashEndEquity"`
	Skips            map[string]int        `json:"skips"`
	SkipDiagnostics  string                `json:"skipDiagnostics"`
	SkipReasonSchema string                `json:"skipReasonSchema"`
	Stats            InteractiveStats      `json:"stats"`
}

// Admitted ordinary families use the shared per-bar broker loop. Scheduled
// source-entry and special families record their own marks and gate counts.
func interactiveOrdinaryFamily(setupType string) bool {
	switch setupType {
	case string(dsl.FamilySMAGoldenCross), string(dsl.FamilyDualEMAResumption),
		string(dsl.FamilyNamedLevelSweep), string(dsl.FamilySessionBreakHold),
		string(dsl.FamilyVWAPExtensionFade), string(dsl.FamilyNamedLevelFlag),
		string(dsl.FamilyOpeningRangeBreakout), string(dsl.FamilyFailedBreakout):
		return true
	default:
		return false
	}
}

func interactiveSpecialFamily(setupType string) bool {
	return setupType == string(dsl.FamilyDailyFlushFailure) || setupType == string(dsl.FamilyWeekendExtremeFade)
}

const InteractiveSourceSkipReasonSchema = "dsl-skip-reasons-source-v1"

// RunInteractiveFixture admits qualified routes with complete marks and
// identified gate skip units. Source FVG counts source decision bars.
func RunInteractiveFixture(raw []byte, source string) (InteractiveRunResult, error) {
	return runInteractiveFixture(raw, source, false)
}

// RunInteractiveVPNYHandoffVetoFixture is the only admitted Go route for the
// archived JS wrapper. Its source hash and strategy ID bind the specific base
// DSL, and the VP filter is installed before the ordinary broker loop runs.
// Calling RunInteractiveFixture under the base strategy ID runs the unwrapped
// DSL; using the wrapper ID there is rejected.
func RunInteractiveVPNYHandoffVetoFixture(raw []byte, source string) (InteractiveRunResult, error) {
	return runInteractiveFixture(raw, source, true)
}

func runInteractiveFixture(raw []byte, source string, vpNYHandoff bool) (InteractiveRunResult, error) {
	if err := validateInteractiveInput(raw); err != nil {
		return InteractiveRunResult{}, err
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		return InteractiveRunResult{}, err
	}
	if fixture.StrategyID == "dslEditorStrategy" {
		sum := sha256.Sum256([]byte(source))
		if hex.EncodeToString(sum[:]) != editorDefaultSourceSHA256 {
			return InteractiveRunResult{}, fmt.Errorf("%w: editor default source has changed", ErrInteractiveUnsupported)
		}
	}
	if !vpNYHandoff && fixture.StrategyID == vpNYHandoffStrategyID {
		return InteractiveRunResult{}, fmt.Errorf("%w: VP NY handoff strategy requires the versioned composition route", ErrInteractiveUnsupported)
	}
	if vpNYHandoff {
		hash := sha256.Sum256([]byte(source))
		if fixture.StrategyID != vpNYHandoffStrategyID || hex.EncodeToString(hash[:]) != vpNYHandoffBaseSourceSHA256 ||
			fixture.Symbol != "XAUUSD" || fixture.Timeframe != "15m" || fixture.HigherTimeframe != "1h" ||
			fixture.RangeMethod != "zone" {
			return InteractiveRunResult{}, fmt.Errorf("%w: VP NY handoff requires its pinned XAUUSD 15m/1h balanced source and route", ErrInteractiveUnsupported)
		}
		for i, bar := range fixture.Bars {
			if bar.V <= 0 || bar.L <= 0 || bar.L > bar.H || bar.O < bar.L || bar.O > bar.H || bar.C < bar.L || bar.C > bar.H {
				return InteractiveRunResult{}, fmt.Errorf("%w: VP NY handoff chart bar %d has unsupported OHLCV", ErrInteractiveUnsupported, i)
			}
		}
	}
	parsed, err := dsl.Parse(source)
	if err != nil {
		return InteractiveRunResult{}, err
	}
	if len(parsed.Errors) != 0 {
		return InteractiveRunResult{}, fmt.Errorf("DSL parse errors: %v", parsed.Errors)
	}
	setupType := setupTypeFromAny(parsed.Config["setupType"])
	htfMode := stringValue(mapValue(parsed.Config, "htf"), "mode", "off")
	chartOnly := htfMode == "off" && len(fixture.HTFBars) == 0
	sourceFVG := setupType == string(dsl.FamilyFairValueGap) && fixture.Symbol == "XAUUSD" &&
		fixture.Timeframe == "5m" && fixture.SourceTimeframe == "1h" &&
		sourceTimeframeFromConfig(parsed.Config) == "1h" &&
		stringValue(parsed.Config, "entryTf", "current") == "5m" &&
		len(fixture.SourceBars) != 0 && len(fixture.SourceHTFBars) == 0 && chartOnly
	special := interactiveSpecialFamily(setupType) && chartOnly && len(fixture.Bars) > 0 &&
		sourceTimeframeFromConfig(parsed.Config) == "" && fixture.SourceTimeframe == "" &&
		len(fixture.SourceBars) == 0 && len(fixture.SourceHTFBars) == 0 &&
		(setupType == string(dsl.FamilyDailyFlushFailure) && fixture.Timeframe == "1d" ||
			setupType == string(dsl.FamilyWeekendExtremeFade) && fixture.Timeframe == "4h")
	// The deployed VWAP profile ships an unused HTF series in its fixture.
	// Admit it only after binding its timestamp grid; the strategy's HTF mode
	// remains off, so no higher-timeframe decision is read from those bars.
	vwapWithUnusedHTF := setupType == string(dsl.FamilyVWAPExtensionFade) &&
		htfMode == "off" && fixture.HigherTimeframe != "" && len(fixture.HTFBars) != 0
	htfOrdinary := (setupType == string(dsl.FamilySessionBreakHold) ||
		setupType == string(dsl.FamilyOpeningRangeBreakout)) &&
		htfMode == "notAgainst" && fixture.HigherTimeframe != "" && len(fixture.HTFBars) != 0
	ordinary := interactiveOrdinaryFamily(setupType) &&
		(chartOnly || htfOrdinary || vwapWithUnusedHTF) &&
		sourceTimeframeFromConfig(parsed.Config) == "" &&
		fixture.SourceTimeframe == "" && len(fixture.SourceBars) == 0 &&
		len(fixture.SourceHTFBars) == 0
	if !ordinary && !special && !sourceFVG {
		return InteractiveRunResult{}, fmt.Errorf("%w: route/family has no qualified interactive execution profile", ErrInteractiveUnsupported)
	}
	if special && setupType == string(dsl.FamilyDailyFlushFailure) &&
		!isWeekdayTimestamp(fixture.Bars[len(fixture.Bars)-1].T) {
		return InteractiveRunResult{}, fmt.Errorf("%w: daily flush failure needs a retained weekday final bar", ErrInteractiveUnsupported)
	}
	if htfOrdinary || vwapWithUnusedHTF {
		if err := validateInteractiveHTFBinding(fixture); err != nil {
			return InteractiveRunResult{}, err
		}
	}
	if sourceFVG {
		if err := validateInteractiveSourceBinding(fixture); err != nil {
			return InteractiveRunResult{}, err
		}
	}
	costs := fixture.Costs.normalized()
	if costs.StartEquity <= 0 || costs.FeePerUnit < 0 || costs.Slippage < 0 || costs.SlippageBps < 0 ||
		!isFiniteDerivedOutput(costs.StartEquity) || !isFiniteDerivedOutput(costs.FeePerUnit) ||
		!isFiniteDerivedOutput(costs.Slippage) || !isFiniteDerivedOutput(costs.SlippageBps) ||
		(costs.FillOn != "close" && costs.FillOn != "open" && costs.FillOn != "nextOpen") {
		return InteractiveRunResult{}, errors.New("invalid interactive execution costs")
	}
	series := marketdata.SeriesFromBars(fixture.Bars)
	request := RunRequest{
		Config: parsed.Config, Series: series, HTFSeries: marketdata.SeriesFromBars(fixture.HTFBars), StrategyID: fixture.StrategyID,
		Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe, RangeMethod: fixture.RangeMethod,
		SourceTimeframe: fixture.SourceTimeframe, SourceSeries: marketdata.SeriesFromBars(fixture.SourceBars),
		SourceHTFSeries: marketdata.SeriesFromBars(fixture.SourceHTFBars),
		Costs:           costs,
	}
	if vpNYHandoff {
		// Ordinary shared-run admission rejects the wrapper ID. Composition
		// evaluates its pinned base under the base ID, then binds the final
		// result to the original wrapper fixture and composition schema.
		request.StrategyID = "dslSessionBreakHoldNyFocusBalancedFifteen"
	}
	prepared, err := PrepareRun(request)
	if err != nil {
		return InteractiveRunResult{}, err
	}
	if prepared.offRoute || prepared.windowed || !RouteAllowed(parsed.Config, fixture.Symbol, fixture.Timeframe, series) {
		return InteractiveRunResult{}, fmt.Errorf("%w: route is off-route or windowed", ErrInteractiveUnsupported)
	}
	var trades []Trade
	var equityCurve, cashCurve []float64
	var skips map[string]int
	skipDiagnostics, skipSchema := "measured", InteractiveSkipReasonSchema
	var cashEnd float64
	if sourceFVG {
		if !prepared.c5 {
			return InteractiveRunResult{}, fmt.Errorf("%w: source FVG did not bind the scheduled source-entry route", ErrInteractiveUnsupported)
		}
		trades, equityCurve, cashCurve, skips, cashEnd, err = runSourceEntrySeriesObserved(
			prepared.fixture, parsed.Config, prepared.params, series, prepared.c5Source, prepared.c5SourceHTF,
			prepared.execution, false, true)
		if err != nil {
			return InteractiveRunResult{}, err
		}
		skipDiagnostics, skipSchema = "measured-source", InteractiveSourceSkipReasonSchema
	} else {
		if prepared.c5 {
			return InteractiveRunResult{}, fmt.Errorf("%w: unexpected scheduled route", ErrInteractiveUnsupported)
		}
		if special && setupType == string(dsl.FamilyWeekendExtremeFade) && WeekendExtremeFadeUnreachable(parsed.Config, series) {
			return InteractiveRunResult{}, fmt.Errorf("%w: weekend-extreme setup cannot execute without a contiguous weekend", ErrInteractiveUnsupported)
		}
		prepared.broker.reset(prepared.series, prepared.cols, prepared.htfTrend,
			prepared.ema, prepared.emaSlope, prepared.params, prepared.fixture, nil)
		prepared.broker.equityCurve = make([]float64, series.Len())
		prepared.broker.cashCurve = make([]float64, series.Len())
		prepared.broker.skipCounts = make(map[string]int)
		if vpNYHandoff {
			prepared.broker.vpNYHandoff = &vpNYHandoffState{}
		}
		trades = prepared.broker.run()
		if prepared.broker.hasPosition {
			return InteractiveRunResult{}, errors.New("interactive run ended with an open position")
		}
		equityCurve, cashCurve, skips = prepared.broker.equityCurve, prepared.broker.cashCurve, prepared.broker.skipCounts
		cashEnd = costs.StartEquity + prepared.broker.realized
	}
	run, err := checkedResultEnvelope(fixture, trades)
	if err != nil {
		return InteractiveRunResult{}, err
	}
	if len(equityCurve) != series.Len() || len(cashCurve) != series.Len() || skips == nil {
		return InteractiveRunResult{}, errors.New("interactive execution did not record every chart mark and its skip profile")
	}
	for _, curve := range [][]float64{equityCurve, cashCurve} {
		for i, value := range curve {
			if !isFiniteDerivedOutput(value) {
				return InteractiveRunResult{}, fmt.Errorf("interactive equity[%d] contains non-finite value", i)
			}
		}
	}
	if !isFiniteDerivedOutput(cashEnd) {
		return InteractiveRunResult{}, errors.New("interactive cash end equity contains non-finite value")
	}
	stats, tradeNetPnL := interactiveStats(trades, equityCurve, cashCurve, costs, cashEnd)
	if err := validateInteractiveStats(stats); err != nil {
		return InteractiveRunResult{}, err
	}
	fixtureHash, sourceHash := sha256.Sum256(raw), sha256.Sum256([]byte(source))
	schema := InteractiveRunSchema
	if vpNYHandoff {
		schema = InteractiveVPNYHandoffSchema
		skipSchema = InteractiveVPNYSkipReasonSchema
	}
	return InteractiveRunResult{
		Schema: schema,
		Provenance: InteractiveProvenance{
			FixtureSHA256: hex.EncodeToString(fixtureHash[:]), SourceSHA256: hex.EncodeToString(sourceHash[:]),
		},
		Run: run, TradeNetPnL: tradeNetPnL, EquityCurve: equityCurve,
		ClosedEquity: cashCurve, CashEndEquity: cashEnd,
		Skips: skips, SkipDiagnostics: skipDiagnostics,
		SkipReasonSchema: skipSchema, Stats: stats,
	}, nil
}

// The shared projection currently infers duration from the smallest positive
// spacing. Bind both supplied series to their declared fixed timeframes before
// using that projection, so sparse or mislabeled rows cannot change decisions.
func validateInteractiveHTFBinding(fixture RunFixture) error {
	chartDuration, chartOK := interactiveFixedDuration(fixture.Timeframe)
	htfDuration, htfOK := interactiveFixedDuration(fixture.HigherTimeframe)
	if !chartOK || !htfOK || htfDuration <= chartDuration || math.Mod(htfDuration, chartDuration) != 0 {
		return fmt.Errorf("%w: invalid chart/higher-timeframe pair", ErrInteractiveUnsupported)
	}
	for _, input := range []struct {
		name     string
		bars     []marketdata.Bar
		duration float64
	}{
		{name: "chart", bars: fixture.Bars, duration: chartDuration},
		{name: "higher-timeframe", bars: fixture.HTFBars, duration: htfDuration},
	} {
		if len(input.bars) < 2 {
			return fmt.Errorf("%w: %s needs two fixed-timeframe bars", ErrInteractiveUnsupported, input.name)
		}
		foundAdjacent := false
		for i, bar := range input.bars {
			if math.Mod(bar.T, input.duration) != 0 {
				return fmt.Errorf("%w: %s bar %d is off the declared timeframe grid", ErrInteractiveUnsupported, input.name, i)
			}
			if i == 0 {
				continue
			}
			gap := bar.T - input.bars[i-1].T
			if gap <= 0 || math.Mod(gap, input.duration) != 0 {
				return fmt.Errorf("%w: %s bar %d is not ordered on the declared timeframe grid", ErrInteractiveUnsupported, input.name, i)
			}
			foundAdjacent = foundAdjacent || gap == input.duration
		}
		if !foundAdjacent {
			return fmt.Errorf("%w: %s duration cannot be inferred from only gapped bars", ErrInteractiveUnsupported, input.name)
		}
	}
	return nil
}

// The source scheduler also infers period lengths from observed spacing. Bind
// both legs before any captured source event can be assigned to a chart bar.
func validateInteractiveSourceBinding(fixture RunFixture) error {
	bound := fixture
	bound.HigherTimeframe = fixture.SourceTimeframe
	bound.HTFBars = fixture.SourceBars
	return validateInteractiveHTFBinding(bound)
}

func interactiveFixedDuration(timeframe string) (float64, bool) {
	switch timeframe {
	case "1m":
		return 60_000, true
	case "5m":
		return 300_000, true
	case "15m":
		return 900_000, true
	case "30m":
		return 1_800_000, true
	case "1h":
		return 3_600_000, true
	case "4h":
		return 14_400_000, true
	case "1d":
		return 86_400_000, true
	default:
		return 0, false
	}
}

// The conformance decoder intentionally accepts metadata and legacy defaults.
// Interactive admission rejects every option or row value it cannot represent
// before that decoder can silently discard it or turn null into zero.
func validateInteractiveInput(raw []byte) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if envelope == nil {
		return errors.New("interactive fixture must be an object")
	}
	names := make([]string, 0, len(envelope))
	for name := range envelope {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		switch name {
		case "schema", "case", "strategyId", "symbol", "timeframe", "higherTimeframe",
			"rangeMethod", "sourceTimeframe", "costs", "bars", "htfBars",
			"sourceBars", "sourceHtfBars", "source":
			// source is provenance metadata, not execution input.
		case "contextOptions":
			var options map[string]json.RawMessage
			if json.Unmarshal(envelope[name], &options) != nil || options == nil || len(options) != 0 {
				return fmt.Errorf("%w: nonempty contextOptions", ErrInteractiveUnsupported)
			}
		default:
			return fmt.Errorf("%w: fixture field %s", ErrInteractiveUnsupported, name)
		}
	}
	for _, name := range []string{"bars", "htfBars", "sourceBars", "sourceHtfBars"} {
		value, exists := envelope[name]
		if !exists {
			continue
		}
		var rows [][]json.RawMessage
		if string(value) == "null" || json.Unmarshal(value, &rows) != nil || rows == nil {
			return fmt.Errorf("interactive %s must be an array", name)
		}
		if len(rows) > 200_000 {
			return fmt.Errorf("%w: %s exceeds the 200000-bar local limit", ErrInteractiveUnsupported, name)
		}
		for i, row := range rows {
			if len(row) != 6 {
				return fmt.Errorf("interactive %s[%d] must contain six OHLCV numbers", name, i)
			}
			for j, cell := range row {
				var number float64
				if string(cell) == "null" || json.Unmarshal(cell, &number) != nil ||
					!isFiniteDerivedOutput(number) {
					return fmt.Errorf("interactive %s[%d][%d] must be a finite number", name, i, j)
				}
			}
		}
	}
	var fields map[string]json.RawMessage
	if len(envelope["costs"]) == 0 || string(envelope["costs"]) == "null" ||
		json.Unmarshal(envelope["costs"], &fields) != nil || fields == nil {
		return errors.New("interactive costs must be an object")
	}
	names = names[:0]
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := fields[name]
		switch name {
		case "feePerUnit", "slippage", "slippageBps", "startEquity":
			var number float64
			if string(value) == "null" || json.Unmarshal(value, &number) != nil ||
				!isFiniteDerivedOutput(number) ||
				(name == "startEquity" && number <= 0) ||
				(name != "startEquity" && number < 0) {
				return fmt.Errorf("interactive costs.%s is invalid", name)
			}
		case "fillOn":
			var fill string
			if string(value) == "null" || json.Unmarshal(value, &fill) != nil ||
				(fill != "close" && fill != "open" && fill != "nextOpen") {
				return errors.New("interactive costs.fillOn must be close, open, or nextOpen")
			}
		default:
			return fmt.Errorf("interactive costs.%s is unsupported", name)
		}
	}
	return nil
}

func validateInteractiveStats(stats InteractiveStats) error {
	for _, field := range []struct {
		name  string
		value float64
	}{
		{"winRate", stats.WinRate}, {"net", stats.Net}, {"tradeNet", stats.TradeNet},
		{"grossWin", stats.GrossWin}, {"grossLoss", stats.GrossLoss},
		{"avgWin", stats.AvgWin}, {"avgLoss", stats.AvgLoss},
		{"expectancy", stats.Expectancy}, {"maxDD", stats.MaxDD},
		{"maxDDpct", stats.MaxDDpct}, {"maxClosedDD", stats.MaxClosedDD},
		{"maxClosedDDpct", stats.MaxClosedDDpct}, {"worstLoss", stats.WorstLoss},
		{"avgHoldBars", stats.AvgHoldBars}, {"endEquity", stats.EndEquity},
		{"returnPct", stats.ReturnPct},
	} {
		if !isFiniteDerivedOutput(field.value) {
			return fmt.Errorf("interactive stats.%s contains non-finite value", field.name)
		}
	}
	if stats.ProfitFactor != nil && !isFiniteDerivedOutput(*stats.ProfitFactor) {
		return errors.New("interactive stats.profitFactor contains non-finite value")
	}
	return nil
}

func interactiveStats(trades []Trade, marked, closed []float64, costs Costs, cashEnd float64) (InteractiveStats, []float64) {
	stats := InteractiveStats{Trades: len(trades), EndEquity: cashEnd,
		Net: cashEnd - costs.StartEquity, ReturnPct: (cashEnd/costs.StartEquity - 1) * 100}
	tradeNetPnL := make([]float64, 0, len(trades))
	curWin, curLoss, holdSum := 0, 0, 0
	for _, trade := range trades {
		stats.TradeNet += trade.PnL
		// Trade.PnL includes the exit commission. Allocate the already
		// debited entry commission to the closed quantity, including partials.
		net := trade.PnL - costs.FeePerUnit*trade.Size
		tradeNetPnL = append(tradeNetPnL, net)
		if net > 0 {
			stats.Wins++
			stats.GrossWin += net
			curWin++
			curLoss = 0
		} else {
			stats.Losses++
			stats.GrossLoss -= net
			curLoss++
			curWin = 0
		}
		stats.MaxWinStreak = max(stats.MaxWinStreak, curWin)
		stats.MaxLossStreak = max(stats.MaxLossStreak, curLoss)
		stats.WorstLoss = math.Min(stats.WorstLoss, net)
		holdSum += trade.ExitIndex - trade.EntryIndex
	}
	if len(trades) > 0 {
		stats.WinRate = 100 * float64(stats.Wins) / float64(len(trades))
		stats.Expectancy = stats.Net / float64(len(trades))
		stats.AvgHoldBars = float64(holdSum) / float64(len(trades))
	}
	if stats.Wins > 0 {
		stats.AvgWin = stats.GrossWin / float64(stats.Wins)
	}
	if stats.Losses > 0 {
		stats.AvgLoss = stats.GrossLoss / float64(stats.Losses)
	}
	if stats.GrossLoss > 0 {
		ratio := stats.GrossWin / stats.GrossLoss
		stats.ProfitFactor = &ratio
		stats.ProfitFactorState = "finite"
	} else if stats.GrossWin > 0 {
		stats.ProfitFactorState = "unbounded"
	} else {
		zero := 0.0
		stats.ProfitFactor = &zero
		stats.ProfitFactorState = "zero"
	}
	stats.MaxDD, stats.MaxDDpct = runningPeakDrawdown(marked, costs.StartEquity, cashEnd)
	stats.MaxClosedDD, stats.MaxClosedDDpct = runningPeakDrawdown(closed, costs.StartEquity, cashEnd)
	return stats, tradeNetPnL
}

func runningPeakDrawdown(curve []float64, start, terminal float64) (float64, float64) {
	peak, maxAbsolute, maxPercent := start, 0.0, 0.0
	for i := 0; i <= len(curve); i++ {
		value := terminal
		if i < len(curve) {
			value = curve[i]
		}
		peak = math.Max(peak, value)
		drop := peak - value
		maxAbsolute = math.Max(maxAbsolute, drop)
		if peak > 0 {
			maxPercent = math.Max(maxPercent, 100*drop/peak)
		}
	}
	return maxAbsolute, maxPercent
}
