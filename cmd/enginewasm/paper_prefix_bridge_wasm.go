//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/spik3r/heisentick-strat/internal/sourcetext"
)

func registerPaperPrefix() {
	js.Global().Set("engineRunPaperPrefix", js.FuncOf(func(_ js.Value, args []js.Value) (response any) {
		// Foreign JS values or unexpected adapter panics must not terminate the
		// instance or disclose source, paths, stack traces, or exception strings.
		defer func() {
			if recover() != nil {
				response = paperPrefixFailure("PAPER_PREFIX_INTERNAL")
			}
		}()
		if len(args) != 1 || !paperPrefixStringArgument(args[0]) {
			return paperPrefixFailure("PAPER_PREFIX_ARGUMENTS")
		}
		// A UTF-16 code unit requires at least one UTF-8 byte. Refuse oversized
		// strings before the original-code-unit validation and UTF-8 allocation.
		if js.Global().Get("Object").Invoke(args[0]).Length() > paperPrefixWASMMaxRequestBytes {
			return paperPrefixFailure("PAPER_PREFIX_INPUT_LIMIT")
		}
		raw, err := sourcetext.FromJS(args[0])
		if err != nil {
			return paperPrefixFailure("PAPER_PREFIX_INPUT")
		}
		return runPaperPrefixJSON(raw)
	}))
}

// Go 1.22 syscall/js Type panics for BigInt. Keep classification in the fixed
// argument-error boundary, before any string conversion or user-value coercion.
func paperPrefixStringArgument(value js.Value) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return value.Type() == js.TypeString
}
