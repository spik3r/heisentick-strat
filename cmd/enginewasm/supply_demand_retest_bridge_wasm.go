//go:build js && wasm

package main

import (
	"syscall/js"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/report"
)

// Preserve intrinsic code-unit operations at startup. Caller objects are never
// coerced, and mutable String/TextEncoder conversion cannot replace the bytes
// after validation. This export is additive; existing exports are unchanged.
var supplyDemandRetestJS struct{ object, apply, bind, charCodeAt js.Value }

func init() {
	supplyDemandRetestJS.object = js.Global().Get("Object")
	supplyDemandRetestJS.apply = js.Global().Get("Reflect").Get("apply")
	supplyDemandRetestJS.bind = js.Global().Get("Function").Get("prototype").Get("bind")
	supplyDemandRetestJS.charCodeAt = js.Global().Get("String").Get("prototype").Get("charCodeAt")
	js.Global().Set("engineRunSupplyDemandRetestReport", js.FuncOf(supplyDemandRetestJSONJS))
}

func supplyDemandRetestJSONJS(_ js.Value, args []js.Value) (response any) {
	defer func() {
		if recover() != nil {
			response = string(report.SupplyDemandRetestFailure("internal-error", ""))
		}
	}()
	if len(args) != 2 || !supplyDemandRetestStringArgument(args[0]) || !supplyDemandRetestStringArgument(args[1]) {
		return string(report.SupplyDemandRetestFailure("invalid-arguments", "arguments"))
	}
	raw, ok := supplyDemandRetestString(args[0], engine.SupplyDemandRetestMaxRequestBytes)
	if !ok {
		return string(report.SupplyDemandRetestFailure("invalid-input", "request"))
	}
	source, ok := supplyDemandRetestString(args[1], engine.SupplyDemandRetestMaxSourceBytes)
	if !ok {
		return string(report.SupplyDemandRetestFailure("invalid-input", "source"))
	}
	return string(report.RunSupplyDemandRetestReport(raw, source))
}

// Go 1.22 syscall/js Type panics on BigInt. Refuse it at the argument boundary.
func supplyDemandRetestStringArgument(value js.Value) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return value.Type() == js.TypeString
}

func supplyDemandRetestString(value js.Value, maxBytes int) (string, bool) {
	unitCount := supplyDemandRetestJS.object.Invoke(value).Get("length").Float()
	if unitCount > float64(maxBytes) {
		return "", false
	}
	units := int(unitCount)
	charCodeAt := supplyDemandRetestJS.apply.Invoke(supplyDemandRetestJS.bind, supplyDemandRetestJS.charCodeAt, []any{value})
	raw := make([]byte, 0, units)
	for i := 0; i < units; i++ {
		unit := charCodeAt.Invoke(i).Int()
		scalar := rune(unit)
		switch {
		case unit >= 0xd800 && unit <= 0xdbff:
			if i+1 >= units {
				return "", false
			}
			next := charCodeAt.Invoke(i + 1).Int()
			if next < 0xdc00 || next > 0xdfff {
				return "", false
			}
			scalar = rune(0x10000 + (unit-0xd800)*0x400 + (next - 0xdc00))
			i++
		case unit >= 0xdc00 && unit <= 0xdfff:
			return "", false
		}
		if len(raw)+utf8.RuneLen(scalar) > maxBytes {
			return "", false
		}
		raw = utf8.AppendRune(raw, scalar)
	}
	return string(raw), true
}
