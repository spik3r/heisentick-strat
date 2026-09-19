// Command conformance generates and checks the Strat conformance corpus.
//
// The Go engine is the corpus generator: `regen` writes every parse golden
// (conformance/parse/<case>.cfg.json), every run golden
// (conformance/run/<case>.trades.json) the engine implements, and the run
// scoreboard in conformance/metadata.json. `check` computes the same bytes
// and fails on any difference, so a golden and the engine cannot drift.
// Other implementations (the app's JavaScript runtime, the WASM builds)
// conform to these files; they never write them.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "conformance:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return usageError("missing command")
	}
	command, rest := args[0], args[1:]
	dir := "conformance"
	for _, arg := range rest {
		switch {
		case strings.HasPrefix(arg, "--dir="):
			dir = strings.TrimPrefix(arg, "--dir=")
		default:
			return usageError("unexpected argument %q", arg)
		}
	}
	switch command {
	case "regen":
		return regen(dir, out)
	case "check":
		return check(dir, out)
	case "list":
		return list(dir, out)
	case "-h", "--help", "help":
		fmt.Fprint(out, usageText())
		return nil
	default:
		return usageError("unknown command %q", command)
	}
}

func usageText() string {
	return `conformance generates and checks the Strat conformance corpus from the Go engine.

Usage (from the repository root):
  go run ./cmd/conformance regen [--dir=conformance]   write parse and run goldens and the scoreboard
  go run ./cmd/conformance check [--dir=conformance]   fail when any golden differs from what the engine produces
  go run ./cmd/conformance list  [--dir=conformance]   print every run case with its implementation status

regen never writes a golden for a run case listed as TODO in engine/run_todo.go.
A changed golden is a reviewed corpus-regeneration PR; see conformance/README.md.
`
}

func usageError(format string, args ...any) error {
	return fmt.Errorf(format+"\n\n"+usageText(), args...)
}
