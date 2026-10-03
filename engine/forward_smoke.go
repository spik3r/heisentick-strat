package engine

import (
	"fmt"
	"math"

	"github.com/spik3r/heisentick-strat/contextcols"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// These operational canaries are intentionally unprofitable. Their versions
// bind the port to the frozen authored-JS sources, not to a DSL translation.
const (
	ForwardSmokeBTCID        = "forwardExecutionSmokeThreeCandleBtcusdOneMinute"
	ForwardSmokeAnyID        = "forwardExecutionSmokeTwoCandleAnySymbolOneMinute"
	ForwardSmokeBTCVersion   = "sha256:899e0edfdcd1e2a6a43426b67df9247b2dbaadd227e1ecf2c3f9c9bcfd034bd1"
	ForwardSmokeAnyVersion   = "sha256:f2ebf1a9334cf088464434f49b921a9264287f9ac04577c88437e5eae2d35558"
	forwardSmokeTradesSchema = "forward-smoke-trades-v1"
)

var forwardSmokeSymbols = map[string]bool{
	"XAUUSD": true, "XAGUSD": true, "EURUSD": true, "GBPUSD": true,
	"GBPJPY": true, "AUDUSD": true, "EURGBP": true, "USDJPY": true,
	"LIGHTCMDUSD": true, "BRENTCMDUSD": true, "GASCMDUSD": true,
	"AUS200": true, "US500": true, "NAS100": true, "BTCUSDT": true,
	"BTCUSD": true,
}

// ForwardSmokeRequest is a chart-only, version-pinned authored-JS canary run.
// Unsupported identities fail explicitly; they cannot appear as zero trades.
type ForwardSmokeRequest struct {
	StrategyID      string
	StrategyVersion string
	Symbol          string
	Timeframe       string
	Series          marketdata.Series
	Costs           Costs
}

func (r ForwardSmokeRequest) validate() error {
	want := ""
	switch r.StrategyID {
	case ForwardSmokeBTCID:
		want = ForwardSmokeBTCVersion
		if r.Symbol != "BTCUSD" || r.Timeframe != "1m" {
			return fmt.Errorf("unsupported forward smoke route %s %s for %s", r.Symbol, r.Timeframe, r.StrategyID)
		}
	case ForwardSmokeAnyID:
		want = ForwardSmokeAnyVersion
		if !forwardSmokeSymbols[r.Symbol] || r.Timeframe != "1m" {
			return fmt.Errorf("unsupported forward smoke route %s %s for %s", r.Symbol, r.Timeframe, r.StrategyID)
		}
	default:
		return fmt.Errorf("unsupported forward smoke strategy %q", r.StrategyID)
	}
	if r.StrategyVersion != want {
		return fmt.Errorf("unsupported forward smoke version %q for %s; want %s", r.StrategyVersion, r.StrategyID, want)
	}
	if r.Series.Len() == 0 {
		return fmt.Errorf("forward smoke market series is empty")
	}
	if err := validateSeriesShape("market", r.Series); err != nil {
		return err
	}
	if err := validateSeriesValues("market", r.Series); err != nil {
		return err
	}
	if r.Costs.FillOn != "" && r.Costs.FillOn != "close" && r.Costs.FillOn != "open" && r.Costs.FillOn != "nextOpen" {
		return fmt.Errorf("unsupported forward smoke fillOn %q", r.Costs.FillOn)
	}
	if err := validateDerivedOutput(r.Costs.normalized(), nil); err != nil {
		return err
	}
	if math.IsNaN(r.Costs.SlippageBps) || math.IsInf(r.Costs.SlippageBps, 0) {
		return fmt.Errorf("forward smoke costs.slippageBps contains non-finite value")
	}
	return nil
}

// RunForwardSmoke executes one frozen authored-JS canary through the Go broker.
func RunForwardSmoke(r ForwardSmokeRequest) (RunResult, error) {
	b, err := runForwardSmoke(r, true)
	if err != nil {
		return RunResult{}, err
	}
	result := resultEnvelope(RunFixture{StrategyID: r.StrategyID, Symbol: r.Symbol, Timeframe: r.Timeframe, Costs: r.Costs}, b.trades)
	result.Schema = forwardSmokeTradesSchema
	return result, nil
}

// RunForwardSmokePrefix replays only supplied closed candles, preserving an
// open position and its stable ID across each extended-prefix restart.
func RunForwardSmokePrefix(r ForwardSmokeRequest) (PrefixResult, error) {
	b, err := runForwardSmoke(r, false)
	if err != nil {
		return PrefixResult{}, err
	}
	positions := b.openPositionSnapshot()
	if err := validatePrefixOutput(r.Costs.normalized(), b.trades, positions); err != nil {
		return PrefixResult{}, err
	}
	closed := make([]PrefixClosedTrade, len(b.trades))
	for i, trade := range b.trades {
		id, err := prefixPositionID(r.StrategyID, r.Symbol, r.Timeframe, trade.EntryT, trade.EntryIndex, trade.Side)
		if err != nil {
			return PrefixResult{}, err
		}
		closed[i] = PrefixClosedTrade{PositionID: id, Trade: trade}
	}
	for i := range positions {
		id, err := prefixPositionID(r.StrategyID, r.Symbol, r.Timeframe, positions[i].EntryT, positions[i].EntryIndex, positions[i].Side)
		if err != nil {
			return PrefixResult{}, err
		}
		positions[i].PositionID = id
	}
	digest, err := canonicalDigest("heisentick-forward-smoke-prefix-input", r)
	if err != nil {
		return PrefixResult{}, err
	}
	return PrefixResult{Trades: closed, OpenPositions: positions, CheckpointDigest: "sha256:" + digest}, nil
}

func runForwardSmoke(r ForwardSmokeRequest, liquidate bool) (*broker, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	series := normalizeOptionalVolume(r.Series)
	b := &broker{}
	b.reset(series, contextcols.Columns{}, nil, nil, nil, flagParams{}, RunFixture{StrategyID: r.StrategyID, Symbol: r.Symbol, Timeframe: r.Timeframe, Costs: r.Costs}, nil)
	for i := 0; i < series.Len(); i++ {
		b.fillPending(i)
		b.resolveIntrabarExit(i)
		if i < 1 || b.hasPosition || len(b.pendingOrders) != 0 {
			continue
		}
		s := side(0)
		if series.C[i-1] > series.O[i-1] && series.C[i] > series.O[i] {
			s = sideLong
		}
		if series.C[i-1] < series.O[i-1] && series.C[i] < series.O[i] {
			s = sideShort
		}
		if s == 0 {
			continue
		}
		entry := series.C[i]
		distance := 200.0
		if r.StrategyID == ForwardSmokeAnyID {
			distance = math.Abs(entry) * 0.0025
		}
		tag := "FORWARD-SMOKE-2-BULLISH"
		if s == sideShort {
			tag = "FORWARD-SMOKE-2-BEARISH"
		}
		ord := order{Side: s, SL: entry - float64(s)*distance, TP: entry + float64(s)*distance, RiskUSD: 100, HasRisk: true, Index: i, Tag: tag}
		if b.costs.fillsMarketAtNextOpen() {
			ord.Index = i + 1
			b.pendingOrders = append(b.pendingOrders, ord)
		} else {
			b.openPosition(s, entry, ord, i)
		}
	}
	if liquidate && b.hasPosition {
		b.closePosition(series.C[series.Len()-1], series.Len()-1, ReasonEndOfTest, "")
	}
	if err := validateDerivedOutput(r.Costs.normalized(), b.trades); err != nil {
		return nil, err
	}
	return b, nil
}
