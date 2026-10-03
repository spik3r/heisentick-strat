package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const AuthoredVPAsiaLondonWideInteractiveSchema = "authored-vp-asia-london-wide-interactive-v1"

const (
	authoredVPWideStrategySHA256 = "cf34cc972b04781df3eca374417eed0bc71a1af2aabecd31f265a3e2d58ff478"
	authoredVPWideBaseSHA256     = "adb1b4ec86a32c9114c449c0f53a10139382fc93c7e48f573e647548778eaddb"
)

// AuthoredVPAsiaLondonWideInteractiveResult is a complete, versioned chart
// result. Its per-bar marked and closed equity include entry and exit fees;
// the final cash balance also includes a forced final-bar liquidation.
type AuthoredVPAsiaLondonWideInteractiveResult struct {
	Schema          string `json:"schema"`
	StrategyVersion string `json:"strategyVersion"`
	Provenance      struct {
		FixtureSHA256       string `json:"fixtureSha256"`
		StrategySHA256      string `json:"strategySha256"`
		InheritedBaseSHA256 string `json:"inheritedBaseSha256"`
	} `json:"provenance"`
	Run           RunResult        `json:"run"`
	TradeNetPnL   []float64        `json:"tradeNetPnl"`
	EquityCurve   []float64        `json:"equityCurve"`
	ClosedEquity  []float64        `json:"closedEquityCurve"`
	CashEndEquity float64          `json:"cashEndEquity"`
	Stats         InteractiveStats `json:"stats"`
}

// RunAuthoredVPAsiaLondonWideInteractive is the native and WASM adapter for
// the active authored strategy ID. Unlike ordinary DSL fixtures, it has no
// source text. Every input field used for execution must be explicit.
func RunAuthoredVPAsiaLondonWideInteractive(raw []byte) (AuthoredVPAsiaLondonWideInteractiveResult, error) {
	if err := validateInteractiveInput(raw); err != nil {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, err
	}
	allowed := map[string]bool{"schema": true, "case": true, "strategyId": true, "symbol": true, "timeframe": true, "rangeMethod": true, "costs": true, "bars": true}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !allowed[key] {
			return AuthoredVPAsiaLondonWideInteractiveResult{}, fmt.Errorf("%w: authored VP field %s", ErrInteractiveUnsupported, key)
		}
	}
	for _, key := range []string{"schema", "case", "strategyId", "symbol", "timeframe", "rangeMethod", "costs", "bars"} {
		if _, ok := fields[key]; !ok {
			return AuthoredVPAsiaLondonWideInteractiveResult{}, fmt.Errorf("authored VP requires %s", key)
		}
	}
	var costFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["costs"], &costFields); err != nil {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, err
	}
	for _, key := range []string{"startEquity", "fillOn", "feePerUnit", "slippage", "slippageBps"} {
		if _, ok := costFields[key]; !ok {
			return AuthoredVPAsiaLondonWideInteractiveResult{}, fmt.Errorf("authored VP requires costs.%s", key)
		}
	}
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, err
	}
	if fixture.StrategyID != authoredVPAsiaLondonWideID || fixture.Symbol != "XAUUSD" || fixture.Timeframe != "5m" || fixture.RangeMethod != "zone" || fixture.Case == "" {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, fmt.Errorf("%w: active authored VP identity requires %s, XAUUSD 5m, zone and a case", ErrInteractiveUnsupported, authoredVPAsiaLondonWideID)
	}
	if len(fixture.Bars) == 0 {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, errors.New("authored VP requires bars")
	}
	b, runFixture, err := runAuthoredVPAsiaLondonWide(fixture.Bars, fixture.Symbol, fixture.Timeframe, fixture.Costs)
	if err != nil {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, err
	}
	runFixture.Case = fixture.Case
	run, err := checkedResultEnvelope(runFixture, b.trades)
	if err != nil {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, err
	}
	cashEnd := fixture.Costs.StartEquity + b.realized
	for _, curve := range [][]float64{b.equityCurve, b.cashCurve} {
		for i, value := range curve {
			if !isFiniteDerivedOutput(value) {
				return AuthoredVPAsiaLondonWideInteractiveResult{}, fmt.Errorf("authored VP equity[%d] is non-finite", i)
			}
		}
	}
	if !isFiniteDerivedOutput(cashEnd) {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, errors.New("authored VP terminal cash is non-finite")
	}
	stats, tradeNet := interactiveStats(b.trades, b.equityCurve, b.cashCurve, fixture.Costs, cashEnd)
	if err := validateInteractiveStats(stats); err != nil {
		return AuthoredVPAsiaLondonWideInteractiveResult{}, err
	}
	hash := sha256.Sum256(raw)
	result := AuthoredVPAsiaLondonWideInteractiveResult{
		Schema: AuthoredVPAsiaLondonWideInteractiveSchema, StrategyVersion: AuthoredVPAsiaLondonWideVersion,
		Run: run, TradeNetPnL: tradeNet, EquityCurve: b.equityCurve, ClosedEquity: b.cashCurve,
		CashEndEquity: cashEnd, Stats: stats,
	}
	result.Provenance.FixtureSHA256 = hex.EncodeToString(hash[:])
	result.Provenance.StrategySHA256 = authoredVPWideStrategySHA256
	result.Provenance.InheritedBaseSHA256 = authoredVPWideBaseSHA256
	return result, nil
}
