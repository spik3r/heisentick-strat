package report

import (
	"strings"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

// Route is one loaded symbol/timeframe route: the entry series plus the
// source and higher-timeframe series the strategy's configuration asks for.
// Callers load the bars; ResolveSourceTimeframe and ResolveHigherTimeframe
// say which extra series a configuration needs.
//
// Request.ExecutionWindow can keep the leading context bars in this series
// while declaring a separate [TradeFromT, TradeToT) tradable interval. The engine
// prepares indicators over the supplied context and only opens, manages, and
// liquidates positions inside that interval. With no window it runs every bar
// and liquidates at the final bar's close.
type Route struct {
	Symbol          string
	TF              string
	SourceTimeframe string
	// Range is the range method, "zone" (the default when empty) or "pivot".
	Range           string
	Series          marketdata.Series
	SourceSeries    marketdata.Series
	HigherTimeframe string
	HTFSeries       marketdata.Series
	SourceHTFSeries marketdata.Series
}

// ResolveHigherTimeframe returns the higher timeframe a configuration's
// notAgainst HTF gate needs for the given source timeframe, or "" when the
// strategy has no such gate.
func ResolveHigherTimeframe(tf string, cfg dsl.Config) string {
	htf, ok := cfg["htf"].(map[string]any)
	if !ok {
		return ""
	}
	if mode, _ := htf["mode"].(string); mode != "notAgainst" {
		return ""
	}
	requested, _ := htf["timeframe"].(string)
	return dsl.ResolveHigherTimeframe(tf, requested)
}

// ResolveSourceTimeframe returns the configuration's explicit source
// timeframe, or the entry timeframe when the strategy does not declare one.
func ResolveSourceTimeframe(tf string, cfg dsl.Config) string {
	if source, _ := cfg["sourceTimeframe"].(string); source != "" {
		return source
	}
	return tf
}

// StrategyID picks the report's strategy id: the requested id, else the
// configuration's name, else the fallback (the CLI passes the file base name).
func StrategyID(cfg dsl.Config, fallback string, requested string) string {
	if requested = strings.TrimSpace(requested); requested != "" {
		return requested
	}
	if name, _ := cfg["name"].(string); strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fallback
}

// StrategyDisplayName picks the evidence envelope's strategy name: the
// requested name, else the configuration's name, else the fallback id.
func StrategyDisplayName(cfg dsl.Config, fallback string, requested string) string {
	if requested = strings.TrimSpace(requested); requested != "" {
		return requested
	}
	if name, _ := cfg["name"].(string); strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fallback
}
