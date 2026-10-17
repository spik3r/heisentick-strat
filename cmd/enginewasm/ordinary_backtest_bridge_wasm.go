//go:build js && wasm

package main

import (
	"syscall/js"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/internal/sourcetext"
)

// Capture intrinsic code-unit operations before publishing the exports. No
// caller object is coerced and mutable String/TextEncoder cannot substitute a
// different request after its Unicode validation.
var ordinaryBacktestJS struct{ object, apply, bind, charCodeAt js.Value }

func registerOrdinaryBacktest() {
	ordinaryBacktestJS.object = js.Global().Get("Object")
	ordinaryBacktestJS.apply = js.Global().Get("Reflect").Get("apply")
	ordinaryBacktestJS.bind = js.Global().Get("Function").Get("prototype").Get("bind")
	ordinaryBacktestJS.charCodeAt = js.Global().Get("String").Get("prototype").Get("charCodeAt")
	js.Global().Set("engineInspectBacktestV1", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return ordinaryBacktestJSONJS("inspect", args)
	}))
	js.Global().Set("engineRunBacktestReportV1", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return ordinaryBacktestJSONJS("run", args)
	}))
}

func ordinaryBacktestJSONJS(operation string, args []js.Value) (response any) {
	defer func() {
		if recover() != nil {
			response = ordinaryBacktestTransportFailure(operation, "INTERNAL")
		}
	}()
	if len(args) != 2 || !ordinaryBacktestStringArgument(args[0]) || !ordinaryBacktestStringArgument(args[1]) {
		return ordinaryBacktestTransportFailure(operation, "ARGUMENTS")
	}
	source, code := ordinaryBacktestString(args[1], engine.OrdinaryBacktestMaxSourceBytes, "SOURCE_ENCODING")
	if code != "" {
		return ordinaryBacktestTransportFailure(operation, code)
	}
	raw, code := ordinaryBacktestString(args[0], engine.OrdinaryBacktestMaxInputBytes-len(source), "JSON_ENCODING")
	if code != "" {
		return ordinaryBacktestTransportFailure(operation, code)
	}
	if operation == "inspect" {
		return inspectOrdinaryBacktestJSON(raw, source)
	}
	return runOrdinaryBacktestReportJSON(raw, source)
}

// Go 1.22 Type panics on BigInt. Treat all foreign/non-primitive types as the
// argument refusal, including boxed strings, proxies, symbols and BigInts.
func ordinaryBacktestStringArgument(value js.Value) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return value.Type() == js.TypeString
}

func ordinaryBacktestString(value js.Value, limit int, encodingCode string) (string, string) {
	count := ordinaryBacktestJS.object.Invoke(value).Get("length").Float()
	if count > float64(limit) {
		return "", "INPUT_LIMIT"
	}
	units := int(count) // Bound before conversion to the WASM int width.
	charCodeAt := ordinaryBacktestJS.apply.Invoke(ordinaryBacktestJS.bind, ordinaryBacktestJS.charCodeAt, []any{value})
	raw := make([]byte, 0, units)
	for i := 0; i < units; i++ {
		unit := charCodeAt.Invoke(i).Int()
		scalar := rune(unit)
		switch {
		case unit >= 0xd800 && unit <= 0xdbff:
			if i+1 >= units {
				return "", encodingCode
			}
			next := charCodeAt.Invoke(i + 1).Int()
			if next < 0xdc00 || next > 0xdfff {
				return "", encodingCode
			}
			scalar = rune(0x10000 + (unit-0xd800)*0x400 + next - 0xdc00)
			i++
		case unit >= 0xdc00 && unit <= 0xdfff:
			return "", encodingCode
		}
		if len(raw)+utf8.RuneLen(scalar) > limit {
			return "", "INPUT_LIMIT"
		}
		raw = utf8.AppendRune(raw, scalar)
	}
	// Retain the repository's shared source transport check, but do not trust
	// its mutable JS string conversion as the captured bytes. Both paths must
	// agree; a later prototype/global mutation can only cause a refusal.
	checked, err := sourcetext.FromJS(value)
	if err != nil || checked != string(raw) {
		return "", encodingCode
	}
	return string(raw), ""
}
