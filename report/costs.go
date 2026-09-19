package report

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/spik3r/heisentick-strat/engine"
)

// CostMode is one execution-cost scenario the report runs. Slip is the
// per-fill slippage in price points; Bps is slippage in basis points of the
// fill price. The engine models no spread, commission or financing, and every
// run starts at the engine's default equity (10000) with fills on the close.
type CostMode struct {
	Label   string
	Slip    float64
	Bps     float64
	Primary bool
}

// CostModes returns the cost rows for a symbol. With an explicit slippage it
// is one primary row labelled "slip <x>"; otherwise the instrument's raw,
// realistic and harsh checks, with realistic primary.
func CostModes(symbol string, slippage *float64) ([]CostMode, PrimaryCost) {
	if slippage != nil {
		label := "slip " + trimFloat(*slippage)
		return []CostMode{{Label: label, Slip: *slippage, Primary: true}}, PrimaryCost{Index: 0, Label: label}
	}
	checks := instrumentCostChecks(symbol)
	bpsChecks := instrumentCostBpsChecks(symbol)
	modes := []CostMode{
		{Label: "raw", Slip: checks[0], Bps: bpsChecks[0]},
		{Label: "realistic", Slip: checks[1], Bps: bpsChecks[1], Primary: true},
		{Label: "harsh", Slip: checks[2], Bps: bpsChecks[2]},
	}
	return modes, PrimaryCost{Index: 1, Label: "realistic"}
}

// ParseSlippageBps parses a basis-point override; "" means none.
func ParseSlippageBps(raw string) (*float64, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fmt.Errorf("invalid slippage-bps: %s", raw)
	}
	return &value, nil
}

func instrumentCostBpsChecks(symbol string) [3]float64 {
	if strings.EqualFold(strings.TrimSpace(symbol), "BTCUSDT") {
		return [3]float64{0, 5, 10}
	}
	return [3]float64{}
}

func instrumentCostChecks(symbol string) [3]float64 {
	switch strings.ToUpper(strings.TrimSpace(symbol)) {
	case "BTCUSDT":
		return [3]float64{}
	case "XAUUSD":
		return [3]float64{0, 0.06, 0.12}
	case "EURUSD", "GBPUSD", "AUDUSD", "EURGBP":
		return [3]float64{0, 0.00005, 0.0001}
	case "GBPJPY", "USDJPY":
		return [3]float64{0, 0.005, 0.01}
	case "LIGHTCMDUSD", "BRENTCMDUSD":
		return [3]float64{0, 0.02, 0.04}
	case "GASCMDUSD":
		return [3]float64{0, 0.0025, 0.005}
	case "AUS200":
		return [3]float64{0, 0.5, 1}
	default:
		return [3]float64{0, 0.00005, 0.0001}
	}
}

// summarize builds one cost row from a run. Units: net and expectancy are in
// the account currency the engine's pnl uses; winRate is a percentage; dd is
// the largest peak-to-trough equity drop as a percentage of the final peak,
// over an equity curve that starts at the run's start equity and applies
// trades in entry-time order.
func summarize(label string, slippage float64, result engine.RunResult, bps ...float64) (CostRow, error) {
	trades := append([]engine.Trade(nil), result.Trades...)
	sort.SliceStable(trades, func(i, j int) bool {
		return trades[i].EntryT < trades[j].EntryT
	})
	var grossWin float64
	var grossLoss float64
	var equity = result.Costs.StartEquity
	if equity == 0 {
		equity = 10000
	}
	peak := equity
	maxDD := 0.0
	wins := 0
	for _, trade := range trades {
		if trade.PnL > 0 {
			wins++
			grossWin += trade.PnL
		} else {
			grossLoss += math.Abs(trade.PnL)
		}
		equity += trade.PnL
		if equity > peak {
			peak = equity
		}
		if drawdown := peak - equity; drawdown > maxDD {
			maxDD = drawdown
		}
	}
	for _, field := range []struct {
		name  string
		value float64
	}{
		{name: "grossWin", value: grossWin},
		{name: "grossLoss", value: grossLoss},
		{name: "equity", value: equity},
		{name: "peak", value: peak},
		{name: "maxDrawdown", value: maxDD},
	} {
		if err := validateSummaryField(field.name, field.value); err != nil {
			return CostRow{}, err
		}
	}

	count := len(trades)
	net := grossWin - grossLoss
	if err := validateSummaryField("net", net); err != nil {
		return CostRow{}, err
	}
	winRate := 0.0
	if count > 0 {
		winRate = (float64(wins) / float64(count)) * 100
	}
	if err := validateSummaryField("winRate", winRate); err != nil {
		return CostRow{}, err
	}

	pf := new(float64)
	if grossLoss > 0 {
		*pf = grossWin / grossLoss
		if err := validateSummaryField("profitFactor", *pf); err != nil {
			return CostRow{}, err
		}
	} else if grossWin > 0 {
		pf = nil
	}

	expectancy := 0.0
	if count > 0 {
		expectancy = net / float64(count)
	}
	if err := validateSummaryField("expectancy", expectancy); err != nil {
		return CostRow{}, err
	}
	dd := 0.0
	if peak > 0 {
		dd = (maxDD / peak) * 100
	}
	if err := validateSummaryField("drawdown", dd); err != nil {
		return CostRow{}, err
	}

	var slippageBps float64
	if len(bps) > 0 {
		slippageBps = bps[0]
	}
	return CostRow{
		Label:       label,
		Slippage:    slippage,
		SlippageBps: slippageBps,
		Trades:      count,
		WinRate:     winRate,
		PF:          pf,
		Net:         net,
		Expectancy:  expectancy,
		DD:          dd,
	}, nil
}

func validateSummaryField(name string, value float64) error {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return fmt.Errorf("summary %s contains non-finite value", name)
	}
	return nil
}

func trimFloat(value float64) string {
	if value == math.Trunc(value) {
		return strconvFormatFloat(value, 'f', 0)
	}
	return strconvFormatFloat(value, 'f', -1)
}

func strconvFormatFloat(value float64, fmt byte, prec int) string {
	return strconv.FormatFloat(value, fmt, prec, 64)
}
