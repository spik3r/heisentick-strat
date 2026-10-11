//go:build js && wasm

package main

import (
	"syscall/js"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/engine"
)

// Capture only intrinsic transport operations. Caller objects are never coerced;
// later changes to global/prototype properties cannot repair malformed Unicode.
var paperBidAskJS struct {
	object, apply, bind, charCodeAt js.Value
}

func init() {
	paperBidAskJS.object = js.Global().Get("Object")
	paperBidAskJS.apply = js.Global().Get("Reflect").Get("apply")
	paperBidAskJS.bind = js.Global().Get("Function").Get("prototype").Get("bind")
	paperBidAskJS.charCodeAt = js.Global().Get("String").Get("prototype").Get("charCodeAt")
	js.Global().Set("engineRunPaperBidAsk", js.FuncOf(paperBidAskJSONJS))
}

func paperBidAskJSONJS(_ js.Value, args []js.Value) (response any) {
	defer func() {
		if recover() != nil {
			response = paperBidAskFailure("PAPER_BIDASK_INTERNAL")
		}
	}()
	if len(args) != 1 || !paperBidAskStringArgument(args[0]) {
		return paperBidAskFailure("PAPER_BIDASK_ARGUMENTS")
	}
	raw, ok := paperBidAskString(args[0])
	if !ok {
		return paperBidAskFailure("PAPER_BIDASK_INPUT")
	}
	return runPaperBidAskJSON(raw)
}

// Go 1.22 syscall/js Type panics on BigInt. Keep that in the argument boundary.
func paperBidAskStringArgument(value js.Value) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return value.Type() == js.TypeString
}

func paperBidAskString(value js.Value) (string, bool) {
	unitCount := paperBidAskJS.object.Invoke(value).Get("length").Float()
	if unitCount > float64(engine.PaperBidAskMaxBytes) {
		return "", false
	}
	units := int(unitCount) // Bound before the wasm int-width conversion.
	charCodeAt := paperBidAskJS.apply.Invoke(paperBidAskJS.bind, paperBidAskJS.charCodeAt, []any{value})
	// Construct UTF-8 from these exact code units. Value.String would enter
	// wasm_exec.js valuePrepareString, whose mutable global String/TextEncoder
	// conversion can substitute a different request after validation.
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
		if len(raw)+utf8.RuneLen(scalar) > engine.PaperBidAskMaxBytes {
			return "", false
		}
		raw = utf8.AppendRune(raw, scalar)
	}
	return string(raw), true
}
