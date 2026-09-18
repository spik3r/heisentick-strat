//go:build js && wasm

package main

import "syscall/js"

// WASM entry point. Exposes a single global function `dslParse(source)` that
// returns the parse envelope as a JSON string, then blocks to keep the Go
// runtime (and the exported function) alive.
//
// This is deliberately the whole browser surface: one versioned operation, no
// DOM access, no worker/routing, no product integration. Wiring the browser to
// call this instead of the JavaScript parser is a separate, later step.
func main() {
	js.Global().Set("dslParse", js.FuncOf(dslParse))
	select {}
}

// dslParse is the js.Func adapter around ParseEnvelopeJSON. It always returns a
// JSON string; argument or marshalling problems become an OK=false envelope so
// callers get one stable shape.
func dslParse(_ js.Value, args []js.Value) any {
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return `{"version":1,"ok":false,"error":"dslParse expects one string source argument"}`
	}
	out, err := ParseEnvelopeJSON(args[0].String())
	if err != nil {
		return `{"version":1,"ok":false,"error":"failed to marshal parse envelope"}`
	}
	return string(out)
}
