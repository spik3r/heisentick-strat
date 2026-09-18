//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"
)

func main() {
	js.Global().Set("engineRunFixture", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
			return `{"error":"expected fixture JSON and source strings"}`
		}
		out, err := runFixture(args[0].String(), args[1].String())
		if err != nil {
			out, _ = json.Marshal(map[string]string{"error": err.Error()})
		}
		return string(out)
	}))
	select {}
}
