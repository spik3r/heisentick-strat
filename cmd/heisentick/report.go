package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"

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
	document, err := report.Build(context.Background(), report.Request{
		Config:        parsed.Config,
		RouteMode:     routeMode,
		Route:         route,
		StrategyID:    id,
		StrategyName:  report.StrategyDisplayName(parsed.Config, id, flags.one("dsl-name", "")),
		Slippage:      slippage,
		SlippageBps:   slippageBps,
		IncludeTrades: boolFlag(flags.one("include-trades", "0")),
		HoldoutFromT:  holdoutFromT,
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
	if boolFlag(flags.one("json-only", "0")) {
		return report.WriteJSON(out, document)
	}
	printCostTable(out, document.Costs)
	return report.WriteJSON(out, document)
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
