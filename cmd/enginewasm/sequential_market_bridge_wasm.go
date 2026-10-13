//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/spik3r/heisentick-strat/internal/sourcetext"
)

// Two additive exports. Each takes exactly (fixtureJSON, source) strings and
// returns one JSON envelope string, success or typed refusal, never a throw.
func registerSequentialMarket() {
	js.Global().Set("engineRunSequentialMarket", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return sequentialMarketExport(args, runSequentialMarket, sequentialMarketFailure)
	}))
	js.Global().Set("engineAdmitSequentialMarket", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return sequentialMarketExport(args, admitSequentialMarket, sequentialMarketAdmissionFailure)
	}))
}

func sequentialMarketExport(args []js.Value, run func(raw, source string) ([]byte, bool), fail func(error) []byte) any {
	if len(args) != 2 || !sequentialMarketStringArgument(args[0]) || !sequentialMarketStringArgument(args[1]) {
		return string(fail(sequentialInvalid("arguments", "expected fixture JSON and source strings")))
	}
	if js.Global().Get("Object").Invoke(args[0]).Length() > sequentialMaxFixture {
		return string(fail(sequentialInvalid("fixture", "fixture JSON exceeds 64 MiB")))
	}
	if js.Global().Get("Object").Invoke(args[1]).Length() > sequentialMaxSource {
		return string(fail(&sequentialInputError{"SEQUENTIAL_DSL_INVALID", "source", sequentialSourceLimitMessage}))
	}
	source, err := sourcetext.FromJS(args[1])
	if err != nil {
		return string(fail(&sequentialInputError{"SEQUENTIAL_DSL_INVALID", "source", err.Error()}))
	}
	raw, err := sourcetext.FromJS(args[0])
	if err != nil {
		return string(fail(sequentialInvalid("fixture", "fixture JSON must contain valid UTF-16")))
	}
	out, _ := run(raw, source)
	return string(out)
}

// Go 1.22 syscall/js panics when Type sees a JavaScript BigInt.
func sequentialMarketStringArgument(value js.Value) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return value.Type() == js.TypeString
}
