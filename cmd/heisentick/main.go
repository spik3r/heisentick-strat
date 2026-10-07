package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "heisentick:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return usageError("missing command")
	}
	switch args[0] {
	case "range-reversion-report":
		return runRangeReversionReport(args[1:], out)
	case "adaptive-flag-report":
		return runAdaptiveFlagReport(args[1:], out)
	case "gold-flag-report":
		return runGoldFlagReport(args[1:], out)
	case "master-portable-report":
		return runMasterPortableReport(args[1:], out)
	case "master-report":
		return runMasterReport(args[1:], out)
	case "regime-report":
		return runRegimeReport(args[1:], out)
	case "timed-report":
		return runTimedReport(args[1:], out)
	case "report":
		return runReport(args[1:], out)
	case "forward-prefix":
		return runForwardPrefix(args[1:], out)
	case "grid":
		return runGrid(args[1:], out)
	case "-h", "--help", "help":
		fmt.Fprint(out, usageText())
		return nil
	default:
		return usageError("unknown command %q", args[0])
	}
}

func usageText() string {
	return `heisentick runs the Go DSL backtester.

Usage:
  heisentick range-reversion-report --dsl-file=<path> --entry-bars-file=<six-column BTB1 path> --source-bars-file=<six-column BTB1 path> --trade-from=<UTC RFC3339> --trade-to=<UTC RFC3339> --slippage-per-fill=<price> --commission-per-unit-side=<cash> --units=<fixed quantity>

range-reversion-report runs only the dedicated M30/H1 to H4 model. It requires fixed units and explicit slippage and per-unit commission inputs; point value, account sizing and browser/generic execution are not inferred.

  heisentick adaptive-flag-report --dsl-file=<path> --bars-file=<six-column BTB1 path> [--trade-from=<UTC RFC3339> --trade-to=<UTC RFC3339>]

adaptive-flag-report runs explicit supplied M30/H1 bars through the named delayed OHLC raw reference. It reports model prices and state only, without sizing, costs or account economics. Generic/browser execution and TradingView parity are not implied.

  heisentick gold-flag-report --dsl-file=<path> --m15-file=<BTB1 path> --from=<UTC RFC3339> --to=<UTC RFC3339> --cost=0|0.06|0.15|0.25|0.50

gold-flag-report runs the fixed native offline PR388 causal stress scenario. It retains partial/gap evidence and unresolved local alternatives. It does not establish observed fills or an executable edge.

  heisentick master-portable-report --arithmetic-contract=master-binary64-separated-v1 --dsl-file=<path> --m5-file=<BTB1 path> --warmup-from=<UTC RFC3339> --trade-from=<UTC RFC3339> --trade-to=<UTC RFC3339> --spread=0|1

master-portable-report explicitly selects the bounded, versioned binary64-separated contract shared with engineRunMasterPortableReport. It is a distinct numerical contract, with no tick quantization or Pine/broker parity claim.

  heisentick master-report --dsl-file=<path> --m5-file=<BTB1 path> --warmup-from=<UTC RFC3339> --trade-from=<UTC RFC3339> --trade-to=<UTC RFC3339> --spread=0|1

master-report runs the fixed native offline Master Structural v10 reference. Two closed policies only; aggregated M30 OHLC paths, continuous prices and illustrative 0.1-unit sizing. No Pine/broker parity, financing model, generic or browser execution is implied.

  heisentick regime-report --dsl-file=<path> --m5-file=<BTB1 path> --warmup-from=<UTC RFC3339> --trade-from=<UTC RFC3339> --trade-to=<UTC RFC3339> --spread=0|1

regime-report runs the fixed native offline Regime Engine interpretation. The dedicated result retains terminal exposure; generic report/grid/prefix and engine WASM do not support this family. Parameters, equity10000 and fee0.50/unit/side are fixed. Source quote side, contract/ounce mapping and Pine parity are unverified; financing is unmodeled.

  heisentick timed-report --dsl-file=<path> --calendar-file=<path> --symbol=<SYMBOL> --tf=<tf> --data-root=<path> --slippage=<points> [--slippage-bps=<basis-points>]

timed-report is an offline, one-unit delayed-open OHLC proxy. It requires an explicit pinned calendar/TZif sidecar, rejects incomplete execution coverage before signals, and reports native price-unit P&L with no commission or financing. It does not support ordinary report/grid or forward-prefix modes.

  heisentick report --dsl-file=<path> --symbol=<SYMBOL> --tf=<tf> --range=zone|pivot [--route-mode=declared|transfer] [--slippage=<points>] [--slippage-bps=<basis-points>] [--include-trades=1] [--evidence=1] [--holdout-from=<epoch-ms>] [--json-only=1] [--memprofile=<path>] [--trade-export=1 [--strat-release=<vX.Y.Z-or-commit>] [--strat-digest=<value>]]

--trade-export=1 (requires --include-trades=1) writes a trade-export.v1 document (heisentick-contracts schemas/trade-export.v1.schema.json) instead of the ordinary report: the primary-cost trades, each with a stable signalId, plus the strategy/engine/data-file provenance envelope. --strat-release names the engine release/commit (default: the running binary's Go build-info module version, "(devel)" for an unreleased build); --strat-digest names a strat build/release identity distinct from --strat-release, when needed (default: --strat-release's value).
  heisentick forward-prefix --dsl-file=<path> --symbol=<SYMBOL> --tf=<tf> [--range=zone|pivot] [--slippage=<points>] [--slippage-bps=<basis-points>] [--data-root=<path>] [--checkpoint-out=<new-path>] [--checkpoint-in=<prior-path> --checkpoint-out=<new-path>]
  heisentick grid --dsl-file=<path> --symbol=<SYMBOL> --tf=<tf> --range=zone|pivot --set <param>=<v1,v2,...> [--route-mode=declared|transfer] [--json-only=1]

forward-prefix always writes one JSON envelope for the loaded data prefix. Without checkpoint flags it preserves the existing full-replay behavior. Checkpoint mode supports reviewed ORB routes only, still requires the complete extended bar prefix, and writes the opaque successor to a new candidate file. Input and output paths must differ and an existing output is never replaced. Advance a durable checkpoint pointer only after exit 0 and complete stdout JSON capture; stdout and the candidate-file rename cannot be atomic together.
`
}

func usageError(format string, args ...any) error {
	return fmt.Errorf(format+"\n\n"+usageText(), args...)
}
