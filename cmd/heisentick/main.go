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
  heisentick report --dsl-file=<path> --symbol=<SYMBOL> --tf=<tf> --range=zone|pivot [--route-mode=declared|transfer] [--slippage=<points>] [--slippage-bps=<basis-points>] [--include-trades=1] [--evidence=1] [--holdout-from=<epoch-ms>] [--json-only=1] [--memprofile=<path>]
  heisentick forward-prefix --dsl-file=<path> --symbol=<SYMBOL> --tf=<tf> [--range=zone|pivot] [--slippage=<points>] [--slippage-bps=<basis-points>] [--data-root=<path>] [--checkpoint-out=<new-path>] [--checkpoint-in=<prior-path> --checkpoint-out=<new-path>]
  heisentick grid --dsl-file=<path> --symbol=<SYMBOL> --tf=<tf> --range=zone|pivot --set <param>=<v1,v2,...> [--route-mode=declared|transfer] [--json-only=1]

forward-prefix always writes one JSON envelope for the loaded data prefix. Without checkpoint flags it preserves the existing full-replay behavior. Checkpoint mode supports reviewed ORB routes only, still requires the complete extended bar prefix, and writes the opaque successor to a new candidate file. Input and output paths must differ and an existing output is never replaced. Advance a durable checkpoint pointer only after exit 0 and complete stdout JSON capture; stdout and the candidate-file rename cannot be atomic together.
`
}

func usageError(format string, args ...any) error {
	return fmt.Errorf(format+"\n\n"+usageText(), args...)
}
