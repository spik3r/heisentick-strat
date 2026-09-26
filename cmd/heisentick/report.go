package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/spik3r/heisentick-strat/data"
	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report"
)

func runReport(args []string, out io.Writer) error {
	flags, err := parseFlags(args)
	if err != nil {
		return err
	}
	if profilePath := flags.one("memprofile", ""); profilePath != "" {
		defer func() {
			runtime.GC()
			file, err := os.Create(profilePath)
			if err == nil {
				_ = pprof.WriteHeapProfile(file)
				_ = file.Close()
			}
		}()
	}
	dslFile, err := flags.required("dsl-file")
	if err != nil {
		return err
	}
	parsed, err := loadDSLFile(dslFile)
	if err != nil {
		return err
	}
	route, err := loadRoute(flags, parsed.Config)
	if err != nil {
		return err
	}
	routeMode := flags.one("route-mode", "declared")
	if routeMode != "declared" && routeMode != "transfer" {
		return fmt.Errorf("invalid --route-mode %q: expected declared or transfer", routeMode)
	}
	slippage, err := parseSlippageFlag(flags.one("slippage", ""))
	if err != nil {
		return err
	}
	slippageBps, err := report.ParseSlippageBps(flags.one("slippage-bps", ""))
	if err != nil {
		return err
	}
	holdoutFromT, err := parseHoldoutFlag(flags.one("holdout-from", ""))
	if err != nil {
		return err
	}
	id := report.StrategyID(parsed.Config, fileBaseName(dslFile), flags.one("dsl-id", ""))
	sourceSHA, err := sha256File(dslFile)
	if err != nil {
		return err
	}
	includeTrades := boolFlag(flags.one("include-trades", "0"))
	tradeExport := boolFlag(flags.one("trade-export", "0"))
	if tradeExport && !includeTrades {
		return fmt.Errorf("--trade-export=1 requires --include-trades=1")
	}
	document, err := report.Build(context.Background(), report.Request{
		Config:          parsed.Config,
		RouteMode:       routeMode,
		Route:           route,
		StrategyID:      id,
		StrategyName:    report.StrategyDisplayName(parsed.Config, id, flags.one("dsl-name", "")),
		StrategyVersion: sourceSHA,
		Slippage:        slippage,
		SlippageBps:     slippageBps,
		IncludeTrades:   includeTrades,
		HoldoutFromT:    holdoutFromT,
	})
	if err != nil {
		return err
	}
	// The established CLI payload stays byte-identical; the M2 statistics
	// are opt-in so existing consumers and goldens do not change.
	if !boolFlag(flags.one("evidence", "0")) {
		document.Headline = nil
		document.Groupings = nil
		document.DateBounds = nil
		document.Holdout = nil
	}
	if tradeExport {
		export, err := buildTradeExportDocument(flags, document, route, sourceSHA, parsed.Config)
		if err != nil {
			return err
		}
		return report.WriteJSON(out, export)
	}
	if boolFlag(flags.one("json-only", "0")) {
		return report.WriteJSON(out, document)
	}
	printCostTable(out, document.Costs)
	return report.WriteJSON(out, document)
}

// sha256File returns the lower-case hex sha256 of a file's bytes. It is used
// both as the strategy version fed to every trade's SignalID and, for a
// trade-export run, as strategy.sourceCommit.
func sha256File(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s for sha256: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// engineRelease names the running binary's build version for the
// trade-export envelope: the --strat-release override when given, else the
// module version go's build info reports (a tagged release when the binary
// was built with `go install pkg@vX.Y.Z`, "(devel)" otherwise).
func engineRelease(flags flagSet) string {
	if release := flags.one("strat-release", ""); release != "" {
		return release
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// buildTradeExportDocument assembles the trade-export.v1 envelope (§2.3 of
// heisentick-backlog's plans/2026-09-26-meta-labeling-existing-strategies.md)
// around the primary-cost trades a --include-trades=1 report run already
// produced. It builds on the existing report path rather than a parallel
// export command: the trades, their SignalID, MFE/MAE and route context are
// already computed by report.Build, so this only adds the envelope fields
// (strategy/engine/data identity, run config) the schema requires.
func buildTradeExportDocument(flags flagSet, document report.Document, route report.Route, sourceSHA string, cfg dsl.Config) (report.TradeExportDocument, error) {
	if len(document.Slices) != 1 || document.Slices[0].Trades == nil {
		return report.TradeExportDocument{}, fmt.Errorf("trade-export: report produced no trades slice")
	}
	primary := document.Costs[document.PrimaryCost.Index]
	root, err := dataRoot(flags)
	if err != nil {
		return report.TradeExportDocument{}, err
	}
	dataFiles, err := tradeExportDataFiles(root, route)
	if err != nil {
		return report.TradeExportDocument{}, err
	}
	release := engineRelease(flags)
	stratDigest := flags.one("strat-digest", release)
	var riskUsd *float64
	if value, ok := riskUSDFromConfig(cfg); ok {
		riskUsd = &value
	}
	var slippageBps *float64
	if primary.SlippageBps != 0 {
		bps := primary.SlippageBps
		slippageBps = &bps
	}
	return report.TradeExportDocument{
		Schema:  report.TradeExportSchema,
		Version: report.TradeExportVersion,
		Strategy: report.TradeExportStrategy{
			ID:           document.Strategy,
			SourceCommit: sourceSHA,
			StratDigest:  stratDigest,
		},
		Engine: report.TradeExportEngineInfo{
			Repo:    "heisentick-strat",
			Release: release,
		},
		DataSha256: dataFiles,
		RunConfig: report.TradeExportRunConfig{
			CostMode:    primary.Label,
			Slippage:    primary.Slippage,
			SlippageBps: slippageBps,
			RiskUsd:     riskUsd,
		},
		Routes:      []report.TradeExportRoute{{Symbol: route.Symbol, TF: route.TF}},
		GeneratedAt: tradeExportGeneratedAtMs(document.GeneratedAt),
		Trades:      report.BuildTradeExportTrades(*document.Slices[0].Trades, primary.Label+"-v1"),
	}, nil
}

// tradeExportDataFiles hashes every distinct (symbol, timeframe) bar file
// the route consumed (entry, and source/higher timeframe when distinct).
func tradeExportDataFiles(root string, route report.Route) ([]report.TradeExportDataFile, error) {
	type key struct{ symbol, tf string }
	seen := map[key]bool{}
	var out []report.TradeExportDataFile
	add := func(symbol, tf string) error {
		if tf == "" {
			return nil
		}
		k := key{symbol, tf}
		if seen[k] {
			return nil
		}
		seen[k] = true
		path, err := data.Path(root, symbol, tf)
		if err != nil {
			return err
		}
		sum, err := sha256File(path)
		if err != nil {
			return err
		}
		out = append(out, report.TradeExportDataFile{File: symbol + "/" + tf + ".bin", Sha256: sum})
		return nil
	}
	if err := add(route.Symbol, route.TF); err != nil {
		return nil, err
	}
	if err := add(route.Symbol, route.SourceTimeframe); err != nil {
		return nil, err
	}
	if err := add(route.Symbol, route.HigherTimeframe); err != nil {
		return nil, err
	}
	return out, nil
}

// tradeExportGeneratedAtMs converts the ordinary Document's RFC3339Nano
// GeneratedAt to epoch milliseconds: trade-export.v1's generatedAt is
// common.v1's epochMs, unlike the human-facing report's timestamp string.
func tradeExportGeneratedAtMs(rfc3339 string) int64 {
	parsed, err := time.Parse(time.RFC3339Nano, rfc3339)
	if err != nil {
		return time.Now().UnixMilli()
	}
	return parsed.UnixMilli()
}

// riskUSDFromConfig reads the strategy's parsed `execution { risk: <n> USD }`
// value (dsl.Config key "riskUsd"), when present and numeric.
func riskUSDFromConfig(cfg dsl.Config) (float64, bool) {
	switch value := cfg["riskUsd"].(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	default:
		return 0, false
	}
}

func parseSlippageFlag(raw string) (*float64, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := parseFloatFlag("slippage", raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// parseHoldoutFlag reads --holdout-from as ms since the Unix epoch (UTC).
func parseHoldoutFlag(raw string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid --holdout-from value %q: expected epoch milliseconds", raw)
	}
	return &value, nil
}

func printCostTable(out io.Writer, rows []report.CostRow) {
	fmt.Fprintln(out, "label,trades,winRate,pf,net,expectancy,dd")
	for _, row := range rows {
		fmt.Fprintf(out, "%s,%d,%.2f,%s,%.2f,%.2f,%.2f\n", row.Label, row.Trades, row.WinRate, formatPF(row.PF), row.Net, row.Expectancy, row.DD)
	}
}

func formatPF(value *float64) string {
	if value == nil {
		return "inf"
	}
	return strconv.FormatFloat(*value, 'f', 4, 64)
}
