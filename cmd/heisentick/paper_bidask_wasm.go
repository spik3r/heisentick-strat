//go:build js && wasm

package main

import (
	"errors"
	"io"

	"github.com/spik3r/heisentick-strat/engine"
)

// The native file command has no js/wasm filesystem substitute. Keep the command
// package compilable for its existing WASM tests without exposing model execution
// or weakening the native regular-file, nonblocking and no-follow checks.
func runPaperBidAsk(_ []string, out io.Writer) error {
	const refusal = "{\"schema\":\"" + engine.PaperBidAskSchema + "\",\"status\":\"refused\",\"code\":\"PAPER_BIDASK_NATIVE_COMMAND_UNAVAILABLE\"}\n"
	if n, err := out.Write([]byte(refusal)); err != nil || n != len(refusal) {
		return errors.New("PAPER_BIDASK_OUTPUT")
	}
	return nil
}
