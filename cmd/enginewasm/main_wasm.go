//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"syscall/js"
	"unsafe"
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
	js.Global().Set("engineRunColumns", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 8 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
			return columnError("expected metadata JSON, source, and six Float64Array columns")
		}
		copyStart := js.Global().Get("performance").Call("now").Float()
		var columns [6]wasmColumn
		for i := range columns {
			column, err := copyFloat64Column(args[i+2])
			if err != nil {
				return columnError(err.Error())
			}
			columns[i] = column
		}
		copyInDone := js.Global().Get("performance").Call("now").Float()
		values := [6][]float64{columns[0].values, columns[1].values, columns[2].values, columns[3].values, columns[4].values, columns[5].values}
		result, err := runColumns(args[0].String(), args[1].String(), values)
		if err != nil {
			return columnError(err.Error())
		}
		outputStart := js.Global().Get("performance").Call("now").Float()
		trades := float64Array(result.Values)
		copyOutDone := js.Global().Get("performance").Call("now").Float()
		return map[string]any{
			"ok": true, "summaryJSON": result.SummaryJSON, "trades": trades, "stringsJSON": result.StringsJSON,
			"timings": map[string]any{"copyInMs": copyInDone - copyStart, "adapterMs": result.Timings.AdapterMS, "engineMs": result.Timings.EngineMS, "packMs": result.Timings.PackMS, "copyOutMs": copyOutDone - outputStart},
		}
	}))
	select {}
}

type wasmColumn struct {
	values []float64
}

func copyFloat64Column(value js.Value) (wasmColumn, error) {
	ctor := js.Global().Get("Float64Array")
	if !value.InstanceOf(ctor) {
		return wasmColumn{}, fmt.Errorf("column must be a Float64Array")
	}
	bytes := value.Get("byteLength").Int()
	if bytes%8 != 0 {
		return wasmColumn{}, fmt.Errorf("column byte length must be divisible by 8")
	}
	values := make([]float64, bytes/8)
	raw := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(values))), bytes)
	uint8 := js.Global().Get("Uint8Array").New(value.Get("buffer"), value.Get("byteOffset"), bytes)
	if copied := js.CopyBytesToGo(raw, uint8); copied != bytes {
		return wasmColumn{}, fmt.Errorf("copied %d bytes, want %d", copied, bytes)
	}
	return wasmColumn{values: values}, nil
}

func float64Array(values []float64) js.Value {
	out := js.Global().Get("Float64Array").New(len(values))
	if len(values) == 0 {
		return out
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(values))), len(values)*8)
	uint8 := js.Global().Get("Uint8Array").New(out.Get("buffer"))
	js.CopyBytesToJS(uint8, bytes)
	runtime.KeepAlive(values)
	return out
}

func columnError(message string) any { return map[string]any{"ok": false, "error": message} }
