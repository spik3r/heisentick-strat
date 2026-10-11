//go:build js && wasm

package main

import (
	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/internal/sourcetext"
	"syscall/js"
)

func registerSequentialInspection() {
	js.Global().Set("engineInspectSequentialSource", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 || !sequentialInspectionStringArgument(args[0]) {
			return sequentialInspectionJSON(engine.SequentialInspectionFailure("SEQUENTIAL_INSPECTION_ARGUMENTS"))
		}
		if js.Global().Get("Object").Invoke(args[0]).Length() > engine.SequentialInspectionMaxSource {
			return sequentialInspectionJSON(engine.SequentialInspectionFailure("SEQUENTIAL_INSPECTION_SOURCE"))
		}
		source, err := sourcetext.FromJS(args[0])
		if err != nil {
			return sequentialInspectionJSON(engine.SequentialInspectionFailure("SEQUENTIAL_INSPECTION_SOURCE"))
		}
		return inspectSequentialSource(source)
	}))
}

// Go 1.22 syscall/js cannot classify JavaScript BigInt values and Type panics.
// Keep unsupported foreign types inside this export's fixed argument refusal,
// without allowing one malformed invocation to terminate the shared instance.
func sequentialInspectionStringArgument(value js.Value) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return value.Type() == js.TypeString
}
