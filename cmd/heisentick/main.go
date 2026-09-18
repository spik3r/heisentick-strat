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
  heisentick report --dsl-file=<path> --symbol=<SYMBOL> --tf=<tf> --range=zone|pivot [--slippage=<points>] [--slippage-bps=<basis-points>] [--include-trades=1] [--json-only=1] [--memprofile=<path>]
  heisentick grid --dsl-file=<path> --symbol=<SYMBOL> --tf=<tf> --range=zone|pivot --set <param>=<v1,v2,...> [--json-only=1]
`
}

func usageError(format string, args ...any) error {
	return fmt.Errorf(format+"\n\n"+usageText(), args...)
}
