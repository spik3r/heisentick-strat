package report

// TradeExportSchema names the trade-export.v1 document (heisentick-contracts
// schemas/trade-export.v1.schema.json). TradeExportVersion is its version.
const (
	TradeExportSchema  = "heisentick/trade-export"
	TradeExportVersion = 1
)

// TradeExportStrategy is the trade-export.v1 envelope's strategy identity.
type TradeExportStrategy struct {
	ID           string `json:"id"`
	SourceCommit string `json:"sourceCommit"`
	StratDigest  string `json:"stratDigest,omitempty"`
}

// TradeExportEngineInfo names the producing engine.
type TradeExportEngineInfo struct {
	Repo    string `json:"repo"`
	Release string `json:"release"`
}

// TradeExportDataFile is one input data file's identity.
type TradeExportDataFile struct {
	File   string `json:"file"`
	Sha256 string `json:"sha256"`
}

// TradeExportRoute names one symbol/timeframe route contributing trades.
type TradeExportRoute struct {
	Symbol string `json:"symbol"`
	TF     string `json:"tf"`
}

// TradeExportRunConfig is the shared cost/risk configuration the trades were
// produced under.
type TradeExportRunConfig struct {
	CostMode    string   `json:"costMode"`
	Slippage    float64  `json:"slippage"`
	SlippageBps *float64 `json:"slippageBps,omitempty"`
	RiskUsd     *float64 `json:"riskUsd,omitempty"`
}

// TradeExportTrade is one trade-export.v1 trade record.
type TradeExportTrade struct {
	SignalID       string   `json:"signalId"`
	Symbol         string   `json:"symbol"`
	TF             string   `json:"tf"`
	Side           string   `json:"side"`
	EntryTs        int64    `json:"entryTs"`
	EntryPrice     float64  `json:"entryPrice"`
	ExitTs         int64    `json:"exitTs"`
	ExitPrice      float64  `json:"exitPrice"`
	InitialSl      *float64 `json:"initialSl"`
	InitialTp      *float64 `json:"initialTp"`
	RNetGross      *float64 `json:"rNetGross"`
	RNetAfterCosts *float64 `json:"rNetAfterCosts"`
	ExitReason     string   `json:"exitReason"`
	ExitRule       *string  `json:"exitRule"`
	MfeR           *float64 `json:"mfeR"`
	MaeR           *float64 `json:"maeR"`
	TimeToMfeBars  *int     `json:"timeToMfeBars"`
	PostExitMfeR   *float64 `json:"postExitMfeR"`
	PostExitMaeR   *float64 `json:"postExitMaeR"`
	CostModelID    string   `json:"costModelId"`
}

// TradeExportDocument is the full trade-export.v1 document.
type TradeExportDocument struct {
	Schema     string                `json:"schema"`
	Version    int                   `json:"version"`
	Strategy   TradeExportStrategy   `json:"strategy"`
	Engine     TradeExportEngineInfo `json:"engine"`
	DataSha256 []TradeExportDataFile `json:"dataSha256"`
	RunConfig  TradeExportRunConfig  `json:"runConfig"`
	Routes     []TradeExportRoute    `json:"routes"`
	// GeneratedAt is epoch milliseconds UTC (trade-export.v1's generatedAt is
	// common.v1's epochMs, unlike the RFC3339Nano the ordinary Document
	// uses).
	GeneratedAt int64              `json:"generatedAt"`
	Trades      []TradeExportTrade `json:"trades"`
}

// BuildTradeExportTrades converts a report's annotated primary-cost trades
// (as produced with IncludeTrades) into trade-export.v1 trade records.
// costModelID names the cost mode the trades were produced under, e.g.
// "realistic-v1".
//
// rNetGross is the price move in the trade's favor (Points, which already
// includes exit slippage but excludes commission) divided by the initial
// risk distance |Entry-InitialSL|; rNetAfterCosts is the realized PnL
// (Points*Size minus commission) divided by risk-per-unit*Size. Both are nil
// when the initial risk distance is zero or the trade had no stop.
func BuildTradeExportTrades(trades []Trade, costModelID string) []TradeExportTrade {
	out := make([]TradeExportTrade, len(trades))
	for i, trade := range trades {
		out[i] = tradeExportTradeFrom(trade, costModelID)
	}
	return out
}

func tradeExportTradeFrom(trade Trade, costModelID string) TradeExportTrade {
	var initialSL, initialTP *float64
	if !trade.NoStop {
		v := trade.InitialSL
		initialSL = &v
	}
	if !trade.NoTarget {
		v := trade.InitialTP
		initialTP = &v
	}
	var rGross, rNet *float64
	if !trade.NoStop {
		risk := trade.Entry - trade.InitialSL
		if trade.Side == "short" {
			risk = -risk
		}
		if risk > 0 && trade.Size != 0 {
			g := trade.Points / risk
			n := trade.PnL / (risk * trade.Size)
			rGross, rNet = &g, &n
		}
	}
	var exitRule *string
	if trade.Rule != "" {
		r := trade.Rule
		exitRule = &r
	}
	return TradeExportTrade{
		SignalID:       trade.SignalID,
		Symbol:         trade.ReportSymbol,
		TF:             trade.ReportTF,
		Side:           trade.Side,
		EntryTs:        int64(trade.EntryT),
		EntryPrice:     trade.Entry,
		ExitTs:         int64(trade.ExitT),
		ExitPrice:      trade.Exit,
		InitialSl:      initialSL,
		InitialTp:      initialTP,
		RNetGross:      rGross,
		RNetAfterCosts: rNet,
		ExitReason:     trade.Reason,
		ExitRule:       exitRule,
		MfeR:           trade.ReportMfeR,
		MaeR:           trade.ReportMaeR,
		TimeToMfeBars:  trade.ReportTimeToMfeBars,
		PostExitMfeR:   trade.ReportPostExitMfeR,
		PostExitMaeR:   trade.ReportPostExitMaeR,
		CostModelID:    costModelID,
	}
}
