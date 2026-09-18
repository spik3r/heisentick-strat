//go:build !js || !wasm

package main

import (
	"fmt"
	"io"
	"os"
)

// Native CLI entry point: read DSL source from a file argument or stdin and
// print the parse envelope as JSON. It keeps `package main` buildable and
// testable off-WASM and is handy for debugging the envelope shape locally
// (`go run ./cmd/dslwasm strat.strat`). The browser build uses main_wasm.go.
func main() {
	var (
		source []byte
		err    error
	)
	if len(os.Args) > 1 {
		source, err = os.ReadFile(os.Args[1])
	} else {
		source, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := ParseEnvelopeJSON(string(source))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
