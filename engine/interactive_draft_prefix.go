package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const InteractiveDraftPrefixSchema = "dsl-interactive-draft-prefix-v1"

// This is preserved exposure, not finalized chart/report equity or resumable
// state. TradeNetPnL aligns with Result.Trades (including their position IDs).
type InteractiveDraftPrefixResult struct {
	Schema            string                `json:"schema"`
	Provenance        InteractiveProvenance `json:"provenance"`
	StrategyVersion   string                `json:"strategyVersion"`
	CalculationSource string                `json:"calculationSource"`
	Costs             Costs                 `json:"costs"`
	Result            PrefixResult          `json:"result"`
	TradeNetPnL       []float64             `json:"tradeNetPnl"`
}

// RunInteractiveDraftPrefixFixture preserves the last open position and leaves
// unfilled next-open orders pending. No caller-supplied finalization switch is
// accepted. Exact strict source and all costs/bars are revalidated at execution.
func RunInteractiveDraftPrefixFixture(raw []byte, source string) (InteractiveDraftPrefixResult, error) {
	var zero InteractiveDraftPrefixResult
	prepared, err := prepareInteractiveDraftFixture(raw, source)
	if err != nil {
		return zero, err
	}
	f := prepared.fixture
	if f.CalculationSource != "" && f.CalculationSource != "raw" {
		return zero, fmt.Errorf("%w: draft prefix requires raw calculation source", ErrInteractiveUnsupported)
	}
	if f.Costs.FillOn != "close" && f.Costs.FillOn != "nextOpen" {
		return zero, fmt.Errorf("%w: draft prefix fillOn must be close or nextOpen", ErrInteractiveUnsupported)
	}
	sourceSum, fixtureSum := sha256.Sum256([]byte(source)), sha256.Sum256(raw)
	provenance := InteractiveProvenance{SourceSHA256: hex.EncodeToString(sourceSum[:]), FixtureSHA256: hex.EncodeToString(fixtureSum[:])}
	// Namespace the unchanged core identity by source, costs and history origin.
	// Runtime/build scope is supplied by the verified artifact manifest. Full
	// append-only continuity remains caller-owned; checkpointDigest binds input.
	scope, err := canonicalDigest("heisentick-draft-prefix-source-cost-origin", struct {
		SourceSHA256 string         `json:"sourceSha256"`
		Costs        Costs          `json:"costs"`
		Origin       marketdata.Bar `json:"origin"`
	}{provenance.SourceSHA256, f.Costs.normalized(), f.Bars[0]})
	if err != nil {
		return zero, err
	}
	version := "gd_" + scope
	result, err := RunPrefix(RunRequest{interactiveDraft: true, Config: prepared.parsed.Config,
		Series: marketdata.SeriesFromBars(f.Bars), StrategyID: version,
		Symbol: f.Symbol, Timeframe: f.Timeframe, RangeMethod: f.RangeMethod, Costs: f.Costs})
	if err != nil {
		return zero, err
	}
	net := make([]float64, len(result.Trades))
	for i, closed := range result.Trades {
		// The strict draft families currently forbid partial exits. Keep entry
		// fees in Go's reporting projection; open exposure has no net summary.
		if closed.Trade.Partial {
			return zero, fmt.Errorf("%w: draft prefix partial accounting is unqualified", ErrInteractiveUnsupported)
		}
		net[i] = closed.Trade.PnL - f.Costs.FeePerUnit*closed.Trade.Size
		if !isFiniteDerivedOutput(net[i]) {
			return zero, fmt.Errorf("draft prefix net P&L must be finite")
		}
	}
	return InteractiveDraftPrefixResult{Schema: InteractiveDraftPrefixSchema, Provenance: provenance,
		StrategyVersion: version, CalculationSource: "raw", Costs: f.Costs.normalized(), Result: result, TradeNetPnL: net}, nil
}
