package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/spik3r/heisentick-strat/authoredorb"
)

const AuthoredOrbInteractiveSchema = "authored-orb-interactive-v1"
const AuthoredOrbInteractiveVersion = "daySessionOrbOriginalTvParity/go-v1"

const (
	authoredOrbStrategySHA256 = "2d4bf578023fe0391ad2cb6198d0c3a0ad9ad7686f0b87c245fd61a69e8bbc81"
	authoredOrbBaseSHA256     = "ef821531d243682ae6740cefa46fbfdb5d9731f9a66790e07ce5a6e2ed123b12"
)

type AuthoredOrbInteractiveResult struct {
	Schema          string `json:"schema"`
	StrategyVersion string `json:"strategyVersion"`
	Provenance      struct {
		FixtureSHA256       string `json:"fixtureSha256"`
		StrategySHA256      string `json:"strategySha256"`
		InheritedBaseSHA256 string `json:"inheritedBaseSha256"`
	} `json:"provenance"`
	Run struct {
		Schema     string              `json:"schema"`
		StrategyID string              `json:"strategyId"`
		Symbol     string              `json:"symbol"`
		Timeframe  string              `json:"timeframe"`
		Costs      authoredorb.Costs   `json:"costs"`
		TradeCount int                 `json:"tradeCount"`
		Trades     []authoredorb.Trade `json:"trades"`
	} `json:"run"`
	TradeNetPnL   []float64        `json:"tradeNetPnl"`
	EquityCurve   []float64        `json:"equityCurve"`
	ClosedEquity  []float64        `json:"closedEquityCurve"`
	CashEndEquity float64          `json:"cashEndEquity"`
	Stats         InteractiveStats `json:"stats"`
}

// RunAuthoredOrbInteractive extends the historical authored ORB trade result
// with Go-owned chart accounting. No JS source or JS-computed statistics enter
// this contract. The v1 trade envelope and its conformance fixtures are intact.
func RunAuthoredOrbInteractive(raw []byte) (AuthoredOrbInteractiveResult, error) {
	var zero AuthoredOrbInteractiveResult
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return zero, err
	}
	allowed := map[string]bool{"schema": true, "strategyId": true, "symbol": true, "timeframe": true, "params": true, "costs": true, "bars": true}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !allowed[key] {
			return zero, fmt.Errorf("%w: authored ORB field %s", ErrInteractiveUnsupported, key)
		}
	}
	for _, key := range []string{"schema", "strategyId", "symbol", "timeframe", "costs", "bars"} {
		if _, ok := fields[key]; !ok {
			return zero, fmt.Errorf("authored ORB requires %s", key)
		}
	}
	for _, key := range []string{"schema", "strategyId", "symbol", "timeframe"} {
		var value string
		if string(fields[key]) == "null" || json.Unmarshal(fields[key], &value) != nil || value == "" {
			return zero, fmt.Errorf("authored ORB %s must be a nonempty string", key)
		}
	}
	var costFields map[string]json.RawMessage
	if string(fields["costs"]) == "null" || json.Unmarshal(fields["costs"], &costFields) != nil || costFields == nil {
		return zero, errors.New("authored ORB costs must be an object")
	}
	allowedCosts := map[string]bool{"startEquity": true, "fillOn": true, "feePerUnit": true, "slippage": true, "slippageBps": true}
	for key := range costFields {
		if !allowedCosts[key] {
			return zero, fmt.Errorf("%w: authored ORB costs.%s", ErrInteractiveUnsupported, key)
		}
	}
	for _, key := range []string{"startEquity", "fillOn", "feePerUnit", "slippage", "slippageBps"} {
		if _, ok := costFields[key]; !ok {
			return zero, fmt.Errorf("authored ORB requires costs.%s", key)
		}
		if key == "fillOn" {
			var value string
			if string(costFields[key]) == "null" || json.Unmarshal(costFields[key], &value) != nil || (value != "close" && value != "nextOpen") {
				return zero, errors.New("authored ORB costs.fillOn must be close or nextOpen")
			}
			continue
		}
		var value float64
		if string(costFields[key]) == "null" || json.Unmarshal(costFields[key], &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) ||
			(key == "startEquity" && value <= 0) || (key != "startEquity" && value < 0) {
			return zero, fmt.Errorf("authored ORB costs.%s is invalid", key)
		}
	}
	if rawParams, ok := fields["params"]; ok {
		var params map[string]json.RawMessage
		if string(rawParams) == "null" || json.Unmarshal(rawParams, &params) != nil || params == nil {
			return zero, errors.New("authored ORB params must be an object")
		}
		for key, rawValue := range params {
			var value float64
			if string(rawValue) == "null" || json.Unmarshal(rawValue, &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return zero, fmt.Errorf("authored ORB param %s must be finite", key)
			}
		}
	}
	var barFields []map[string]json.RawMessage
	if string(fields["bars"]) == "null" || json.Unmarshal(fields["bars"], &barFields) != nil || len(barFields) == 0 || len(barFields) > 50_000 {
		return zero, errors.New("authored ORB bars must contain 1–50000 objects")
	}
	allowedBar := map[string]bool{"t": true, "o": true, "h": true, "l": true, "c": true, "v": true}
	for i, bar := range barFields {
		for key := range bar {
			if !allowedBar[key] {
				return zero, fmt.Errorf("%w: authored ORB bars[%d].%s", ErrInteractiveUnsupported, i, key)
			}
		}
		for _, key := range []string{"t", "o", "h", "l", "c", "v"} {
			rawValue, ok := bar[key]
			var value float64
			if !ok || string(rawValue) == "null" || json.Unmarshal(rawValue, &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return zero, fmt.Errorf("authored ORB bars[%d].%s must be finite", i, key)
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request authoredorb.Request
	if err := decoder.Decode(&request); err != nil {
		return zero, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return zero, errors.New("trailing JSON after authored ORB request")
		}
		return zero, err
	}
	if request.StrategyID != authoredorb.OriginalTVParity || request.Symbol != "XAUUSD" || request.Timeframe != "30m" || len(request.Bars) == 0 {
		return zero, fmt.Errorf("%w: authored ORB interactive requires %s XAUUSD 30m bars", ErrInteractiveUnsupported, authoredorb.OriginalTVParity)
	}
	if request.Costs.StartEquity == nil || *request.Costs.StartEquity <= 0 {
		return zero, errors.New("authored ORB interactive requires positive startEquity")
	}
	result, err := authoredorb.Run(request)
	if err != nil {
		return zero, err
	}
	statsTrades := make([]Trade, len(result.Trades))
	for i, trade := range result.Trades {
		statsTrades[i] = Trade{PnL: trade.PnL, Size: trade.Size, EntryIndex: trade.EntryIndex, ExitIndex: trade.ExitIndex}
	}
	costs := Costs{StartEquity: *request.Costs.StartEquity, FillOn: request.Costs.FillOn,
		FeePerUnit: request.Costs.FeePerUnit, Slippage: request.Costs.Slippage, SlippageBps: request.Costs.SlippageBps}
	for _, curve := range [][]float64{result.EquityCurve, result.ClosedEquity} {
		for i, value := range curve {
			if !isFiniteDerivedOutput(value) {
				return zero, fmt.Errorf("authored ORB equity[%d] is non-finite", i)
			}
		}
	}
	if !isFiniteDerivedOutput(result.CashEndEquity) {
		return zero, errors.New("authored ORB terminal cash is non-finite")
	}
	stats, tradeNet := interactiveStats(statsTrades, result.EquityCurve, result.ClosedEquity, costs, result.CashEndEquity)
	if err := validateInteractiveStats(stats); err != nil {
		return zero, err
	}
	hash := sha256.Sum256(raw)
	out := AuthoredOrbInteractiveResult{Schema: AuthoredOrbInteractiveSchema, StrategyVersion: AuthoredOrbInteractiveVersion,
		TradeNetPnL: tradeNet, EquityCurve: result.EquityCurve, ClosedEquity: result.ClosedEquity,
		CashEndEquity: result.CashEndEquity, Stats: stats}
	out.Provenance.FixtureSHA256 = hex.EncodeToString(hash[:])
	out.Provenance.StrategySHA256 = authoredOrbStrategySHA256
	out.Provenance.InheritedBaseSHA256 = authoredOrbBaseSHA256
	out.Run.Schema = result.Schema
	out.Run.StrategyID = result.StrategyID
	out.Run.Symbol = result.Symbol
	out.Run.Timeframe = result.Timeframe
	out.Run.Costs = request.Costs
	out.Run.TradeCount = len(result.Trades)
	out.Run.Trades = result.Trades
	return out, nil
}
