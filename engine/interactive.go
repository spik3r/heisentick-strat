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
	Schema        string                `json:"schema"`
	Provenance    InteractiveProvenance `json:"provenance"`
	Run           RunResult             `json:"run"`
	EquityCurve   []float64             `json:"equityCurve"`
	ClosedEquity  []float64             `json:"closedEquityCurve"`
	CashEndEquity float64               `json:"cashEndEquity"`
	Skips         map[string]int        `json:"skips"`
	Stats         InteractiveStats      `json:"stats"`
}

// The admitted ordinary families make no skip-reason decisions. Scheduled
// source-entry and special-family loops do not yet emit complete per-bar marks;
// other families have not qualified their skip diagnostics.
func interactiveOrdinaryFamily(setupType string) bool {
	switch setupType {
	case string(dsl.FamilySMAGoldenCross), string(dsl.FamilyDualEMAResumption):
		return true
	default:
		return false
	}
}

// RunInteractiveFixture admits only ordinary chart-timeframe families whose
// complete per-bar marks and empty skip map are verified above.
func RunInteractiveFixture(raw []byte, source string) (InteractiveRunResult, error) {
	if err := validateInteractiveInput(raw); err != nil {
		return InteractiveRunResult{}, err
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		return InteractiveRunResult{}, err
	}
	parsed, err := dsl.Parse(source)
	if err != nil {
		return InteractiveRunResult{}, err
	}
	if len(parsed.Errors) != 0 {
		return InteractiveRunResult{}, fmt.Errorf("DSL parse errors: %v", parsed.Errors)
	}
	if !interactiveOrdinaryFamily(setupTypeFromAny(parsed.Config["setupType"])) ||
		sourceTimeframeFromConfig(parsed.Config) != "" ||
		fixture.SourceTimeframe != "" || len(fixture.SourceBars) != 0 ||
		len(fixture.HTFBars) != 0 || len(fixture.SourceHTFBars) != 0 {
		return InteractiveRunResult{}, fmt.Errorf("%w: only qualified chart-timeframe ordinary families without source/HTF inputs are admitted", ErrInteractiveUnsupported)
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
		Config: parsed.Config, Series: series, StrategyID: fixture.StrategyID,
		Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe, RangeMethod: fixture.RangeMethod,
		Costs: costs,
	}
	prepared, err := PrepareRun(request)
	if err != nil {
		return InteractiveRunResult{}, err
	}
	if prepared.offRoute || prepared.c5 || prepared.windowed {
		return InteractiveRunResult{}, fmt.Errorf("%w: route is off-route, scheduled, or windowed", ErrInteractiveUnsupported)
	}
	prepared.broker.reset(prepared.series, prepared.cols, prepared.htfTrend,
		prepared.ema, prepared.emaSlope, prepared.params, prepared.fixture, nil)
	prepared.broker.equityCurve = make([]float64, series.Len())
	prepared.broker.cashCurve = make([]float64, series.Len())
	trades := prepared.broker.run()
	if prepared.broker.hasPosition {
		return InteractiveRunResult{}, errors.New("interactive run ended with an open position")
	}
	run, err := checkedResultEnvelope(fixture, trades)
	if err != nil {
		return InteractiveRunResult{}, err
	}
	cashEnd := costs.StartEquity + prepared.broker.realized
	for _, curve := range [][]float64{prepared.broker.equityCurve, prepared.broker.cashCurve} {
		for i, value := range curve {
			if !isFiniteDerivedOutput(value) {
				return InteractiveRunResult{}, fmt.Errorf("interactive equity[%d] contains non-finite value", i)
			}
		}
	}
	if !isFiniteDerivedOutput(cashEnd) {
		return InteractiveRunResult{}, errors.New("interactive cash end equity contains non-finite value")
	}
	stats := interactiveStats(trades, prepared.broker.equityCurve, prepared.broker.cashCurve, costs, cashEnd)
	if err := validateInteractiveStats(stats); err != nil {
		return InteractiveRunResult{}, err
	}
	fixtureHash, sourceHash := sha256.Sum256(raw), sha256.Sum256([]byte(source))
	return InteractiveRunResult{
		Schema: InteractiveRunSchema,
		Provenance: InteractiveProvenance{
			FixtureSHA256: hex.EncodeToString(fixtureHash[:]), SourceSHA256: hex.EncodeToString(sourceHash[:]),
		},
		Run: run, EquityCurve: prepared.broker.equityCurve,
		ClosedEquity: prepared.broker.cashCurve, CashEndEquity: cashEnd,
		Skips: map[string]int{}, Stats: stats,
	}, nil
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
		if len(rows) > 50_000 {
			return fmt.Errorf("%w: %s exceeds the 50000-bar local limit", ErrInteractiveUnsupported, name)
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

func interactiveStats(trades []Trade, marked, closed []float64, costs Costs, cashEnd float64) InteractiveStats {
	stats := InteractiveStats{Trades: len(trades), EndEquity: cashEnd,
		Net: cashEnd - costs.StartEquity, ReturnPct: (cashEnd/costs.StartEquity - 1) * 100}
	curWin, curLoss, holdSum := 0, 0, 0
	for _, trade := range trades {
		stats.TradeNet += trade.PnL
		// Trade.PnL includes the exit commission. Allocate the already
		// debited entry commission to the closed quantity, including partials.
		net := trade.PnL - costs.FeePerUnit*trade.Size
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
	return stats
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
