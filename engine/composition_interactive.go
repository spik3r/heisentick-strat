package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const InteractiveCompositionSchema = "dsl-interactive-composition-v1"

type CompositionSourceProvenance struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

type InteractiveCompositionProvenance struct {
	FixtureSHA256  string                        `json:"fixtureSha256"`
	ManifestSHA256 string                        `json:"manifestSha256"`
	SourceRevision string                        `json:"sourceRevision"`
	Children       []CompositionSourceProvenance `json:"children"`
}

// InteractiveCompositionResult has the same cash, mark and statistic meanings
// as InteractiveRunResult, with provenance for the complete pinned wrapper.
type InteractiveCompositionResult struct {
	Schema           string                           `json:"schema"`
	Provenance       InteractiveCompositionProvenance `json:"provenance"`
	Run              RunResult                        `json:"run"`
	TradeNetPnL      []float64                        `json:"tradeNetPnl"`
	EquityCurve      []float64                        `json:"equityCurve"`
	ClosedEquity     []float64                        `json:"closedEquityCurve"`
	CashEndEquity    float64                          `json:"cashEndEquity"`
	Skips            map[string]int                   `json:"skips"`
	SkipDiagnostics  string                           `json:"skipDiagnostics"`
	SkipReasonSchema string                           `json:"skipReasonSchema"`
	Stats            InteractiveStats                 `json:"stats"`
}

// RunInteractiveCompositionFixture runs an authored wrapper wholly in Go. It
// rejects unqualified routes and inputs instead of returning a trade-only or
// empty successful chart result. Every child in the manifest is digest checked.
func RunInteractiveCompositionFixture(raw []byte, sources map[string]string) (InteractiveCompositionResult, error) {
	if err := validateInteractiveInput(raw); err != nil {
		return InteractiveCompositionResult{}, err
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		return InteractiveCompositionResult{}, err
	}
	if len(fixture.Bars) == 0 {
		return InteractiveCompositionResult{}, errors.New("interactive composition needs chart bars")
	}
	if fixture.SourceTimeframe != "" || len(fixture.SourceBars) != 0 || len(fixture.SourceHTFBars) != 0 {
		return InteractiveCompositionResult{}, fmt.Errorf("%w: source-timeframe input", ErrInteractiveUnsupported)
	}
	if (fixture.HigherTimeframe == "") != (len(fixture.HTFBars) == 0) {
		return InteractiveCompositionResult{}, fmt.Errorf("%w: incomplete higher-timeframe input", ErrInteractiveUnsupported)
	}
	if len(fixture.HTFBars) > 0 {
		if err := validateInteractiveHTFBinding(fixture); err != nil {
			return InteractiveCompositionResult{}, err
		}
	}
	costs := fixture.Costs.normalized()
	if costs.StartEquity <= 0 || costs.FeePerUnit < 0 || costs.Slippage < 0 || costs.SlippageBps < 0 ||
		!isFiniteDerivedOutput(costs.StartEquity) || !isFiniteDerivedOutput(costs.FeePerUnit) ||
		!isFiniteDerivedOutput(costs.Slippage) || !isFiniteDerivedOutput(costs.SlippageBps) ||
		(costs.FillOn != "close" && costs.FillOn != "open" && costs.FillOn != "nextOpen") {
		return InteractiveCompositionResult{}, errors.New("invalid interactive execution costs")
	}
	manifest, err := BuiltInCompositionManifest()
	if err != nil {
		return InteractiveCompositionResult{}, err
	}
	request := RunRequest{
		Series: marketdata.SeriesFromBars(fixture.Bars), HTFSeries: marketdata.SeriesFromBars(fixture.HTFBars),
		StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe, RangeMethod: fixture.RangeMethod, Costs: costs,
	}
	prepared, offRoute, err := prepareComposition(manifest, request, sources)
	if err != nil {
		return InteractiveCompositionResult{}, err
	}
	if offRoute {
		return InteractiveCompositionResult{}, fmt.Errorf("%w: undeclared composite route %s %s", ErrInteractiveUnsupported, fixture.Symbol, fixture.Timeframe)
	}
	for _, child := range prepared {
		if child.params.UseHTFBias && len(fixture.HTFBars) == 0 {
			return InteractiveCompositionResult{}, fmt.Errorf("%w: composite child requires higher-timeframe bars", ErrInteractiveUnsupported)
		}
		if child.c5 || child.windowed || child.series.Len() != len(fixture.Bars) {
			return InteractiveCompositionResult{}, fmt.Errorf("%w: scheduled, windowed, or mismatched child series", ErrInteractiveUnsupported)
		}
	}
	var run RunResult
	var broker *broker
	if len(prepared) == 1 {
		p := prepared[0]
		p.broker.reset(p.series, p.cols, p.htfTrend, p.ema, p.emaSlope, p.params, p.fixture, nil)
		p.broker.equityCurve = make([]float64, p.series.Len())
		p.broker.cashCurve = make([]float64, p.series.Len())
		p.broker.skipCounts = make(map[string]int)
		trades := p.broker.run()
		run, err = checkedResultEnvelope(fixture, trades)
		broker = &p.broker
	} else {
		run, broker, err = runOrderedSessionBreakHoldWithBroker(prepared, costs, true)
	}
	if err != nil {
		return InteractiveCompositionResult{}, err
	}
	if broker.hasPosition {
		return InteractiveCompositionResult{}, errors.New("interactive composition ended with open position")
	}
	run = normalizeCompositionResult(run)
	run.Case = fixture.Case
	cashEnd := costs.StartEquity + broker.realized
	for _, curve := range [][]float64{broker.equityCurve, broker.cashCurve} {
		if len(curve) != len(fixture.Bars) {
			return InteractiveCompositionResult{}, errors.New("incomplete interactive composition curve")
		}
		for i, value := range curve {
			if !isFiniteDerivedOutput(value) {
				return InteractiveCompositionResult{}, fmt.Errorf("interactive composition equity[%d] is non-finite", i)
			}
		}
	}
	if !isFiniteDerivedOutput(cashEnd) {
		return InteractiveCompositionResult{}, errors.New("non-finite composition cash end")
	}
	stats, tradeNetPnL := interactiveStats(broker.trades, broker.equityCurve, broker.cashCurve, costs, cashEnd)
	if err := validateInteractiveStats(stats); err != nil {
		return InteractiveCompositionResult{}, err
	}
	fixtureHash, manifestHash := sha256.Sum256(raw), sha256.Sum256(builtInCompositionJSON)
	selectedIDs := make(map[string]bool)
	for _, composite := range manifest.Composites {
		if composite.ID == fixture.StrategyID {
			for _, route := range composite.Routes {
				for _, id := range route.Children {
					selectedIDs[id] = true
				}
			}
		}
	}
	childProvenance := make([]CompositionSourceProvenance, 0, len(selectedIDs))
	for _, child := range manifest.Children {
		if selectedIDs[child.ID] {
			childProvenance = append(childProvenance, CompositionSourceProvenance{ID: child.ID, SHA256: child.SHA256})
		}
	}
	sort.Slice(childProvenance, func(i, j int) bool { return childProvenance[i].ID < childProvenance[j].ID })
	return InteractiveCompositionResult{
		Schema:     InteractiveCompositionSchema,
		Provenance: InteractiveCompositionProvenance{FixtureSHA256: hex.EncodeToString(fixtureHash[:]), ManifestSHA256: hex.EncodeToString(manifestHash[:]), SourceRevision: manifest.SourceRevision, Children: childProvenance},
		Run:        run, TradeNetPnL: tradeNetPnL, EquityCurve: broker.equityCurve, ClosedEquity: broker.cashCurve,
		CashEndEquity: cashEnd, Skips: broker.skipCounts, SkipDiagnostics: "measured", SkipReasonSchema: InteractiveSkipReasonSchema, Stats: stats,
	}, nil
}
