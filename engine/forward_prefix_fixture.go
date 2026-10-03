package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

const ForwardPrefixSchema = "dsl-forward-prefix-v1"

// ForwardPrefixFixtureResult contains preserved-open engine evidence, not
// chart statistics or a resumable Go checkpoint. Both hashes bind exact bytes.
type ForwardPrefixFixtureResult struct {
	Schema     string `json:"schema"`
	Provenance struct {
		FixtureSHA256 string `json:"fixtureSha256"`
		SourceSHA256  string `json:"sourceSha256"`
	} `json:"provenance"`
	Result PrefixResult `json:"result"`
}

// RunForwardPrefixFixture admits ordinary raw-candle, chart-only DSL prefixes.
// It never liquidates at the final supplied bar. HTF, source-entry, authored
// wrappers and parameter overrides need their own reviewed prefix contracts.
func RunForwardPrefixFixture(raw []byte, source string) (ForwardPrefixFixtureResult, error) {
	var zero ForwardPrefixFixtureResult
	if err := validateInteractiveInput(raw); err != nil {
		return zero, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return zero, err
	}
	allowed := map[string]bool{"schema": true, "case": true, "strategyId": true, "symbol": true, "timeframe": true, "rangeMethod": true, "costs": true, "bars": true}
	for key := range fields {
		if !allowed[key] {
			return zero, fmt.Errorf("%w: Forward prefix field %s", ErrInteractiveUnsupported, key)
		}
	}
	for _, key := range []string{"schema", "case", "strategyId", "symbol", "timeframe", "rangeMethod"} {
		var value string
		if json.Unmarshal(fields[key], &value) != nil || value == "" {
			return zero, fmt.Errorf("Forward prefix %s must be a nonempty string", key)
		}
	}
	var costs map[string]json.RawMessage
	if err := json.Unmarshal(fields["costs"], &costs); err != nil {
		return zero, err
	}
	for _, key := range []string{"fillOn", "feePerUnit", "slippage", "slippageBps", "startEquity"} {
		if _, ok := costs[key]; !ok {
			return zero, fmt.Errorf("Forward prefix costs.%s is required", key)
		}
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		return zero, err
	}
	if fixture.RangeMethod != "zone" || len(fixture.Bars) == 0 || len(fixture.Bars) > 30_000 {
		return zero, fmt.Errorf("%w: Forward prefix requires zone context and 1–30000 bars", ErrInteractiveUnsupported)
	}
	for i, bar := range fixture.Bars {
		if bar.T < 0 || bar.T > 9007199254740991 || math.Trunc(bar.T) != bar.T || (i > 0 && bar.T <= fixture.Bars[i-1].T) ||
			bar.V < 0 || bar.H < math.Max(bar.O, bar.C) || bar.L > math.Min(bar.O, bar.C) || bar.H < bar.L {
			return zero, fmt.Errorf("Forward prefix bars[%d] must have ordered integer timestamps and valid OHLCV bounds", i)
		}
	}
	parsed, err := dsl.Parse(source)
	if err != nil {
		return zero, err
	}
	if len(parsed.Errors) != 0 {
		return zero, fmt.Errorf("DSL parse errors: %v", parsed.Errors)
	}
	if !interactiveOrdinaryFamily(setupTypeFromAny(parsed.Config["setupType"])) ||
		stringValue(mapValue(parsed.Config, "htf"), "mode", "off") != "off" || sourceTimeframeFromConfig(parsed.Config) != "" {
		return zero, fmt.Errorf("%w: Forward prefix requires an ordinary chart-only family", ErrInteractiveUnsupported)
	}
	series := marketdata.SeriesFromBars(fixture.Bars)
	if !RouteAllowed(parsed.Config, fixture.Symbol, fixture.Timeframe, series) {
		return zero, fmt.Errorf("%w: Forward prefix is off-route", ErrInteractiveUnsupported)
	}
	result, err := RunPrefix(RunRequest{Config: parsed.Config, Series: series, StrategyID: fixture.StrategyID,
		Symbol: fixture.Symbol, Timeframe: fixture.Timeframe, RangeMethod: fixture.RangeMethod, Costs: fixture.Costs})
	if err != nil {
		return zero, err
	}
	fixtureSum, sourceSum := sha256.Sum256(raw), sha256.Sum256([]byte(source))
	out := ForwardPrefixFixtureResult{Schema: ForwardPrefixSchema, Result: result}
	out.Provenance.FixtureSHA256 = hex.EncodeToString(fixtureSum[:])
	out.Provenance.SourceSHA256 = hex.EncodeToString(sourceSum[:])
	return out, nil
}
