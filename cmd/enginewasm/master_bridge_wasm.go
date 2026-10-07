//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/spik3r/heisentick-strat/internal/sourcetext"
	"github.com/spik3r/heisentick-strat/report/masterstructural"
)

// Registration is separate from the existing generic entry points. They retain
// their dedicated-runner refusals for this family.
func init() {
	js.Global().Set("engineRunMasterPortableReport", js.FuncOf(masterPortableReportJS))
}

func masterPortableReportJS(_ js.Value, args []js.Value) (response any) {
	// Detached buffers and hostile JavaScript accessors can throw. One rejected
	// invocation must not kill the runtime or leave a success-looking document.
	defer func() {
		if recover() != nil {
			response = masterError("master runtime invalid or inaccessible transport value")
		}
	}()
	if len(args) != 3 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
		return masterError("expected metadata JSON, DSL source and a Uint8Array BTB1 view")
	}
	metadata, err := boundedJSString(args[0], masterstructural.MaxMetadataBytes, "metadata")
	if err != nil {
		return masterError(err.Error())
	}
	if _, err := masterstructural.DecodeRuntimeRequest(metadata); err != nil {
		return masterError(err.Error())
	}
	source, err := boundedJSString(args[1], masterstructural.MaxSourceBytes, "source")
	if err != nil {
		return masterError(err.Error())
	}
	data, err := copyMasterBTB1(args[2])
	if err != nil {
		return masterError(err.Error())
	}
	out, err := runMasterPortableReportBTB1(metadata, source, data)
	if err != nil {
		return masterError(err.Error())
	}
	return string(out)
}

func boundedJSString(value js.Value, max int, label string) (string, error) {
	// UTF-8 has at least as many bytes as valid UTF-16 code units. Bound the
	// original units before walking/converting them, then enforce UTF-8 bytes.
	boxed := js.Global().Get("Object").Invoke(value)
	if boxed.Length() > max {
		return "", fmt.Errorf("master runtime %s exceeds %d bytes", label, max)
	}
	text, err := sourcetext.FromJS(value)
	if err != nil {
		return "", err
	}
	if len(text) > max {
		return "", fmt.Errorf("master runtime %s exceeds %d bytes", label, max)
	}
	return text, nil
}

func copyMasterBTB1(value js.Value) ([]byte, error) {
	object := js.Global().Get("Object")
	uint8 := js.Global().Get("Uint8Array")
	if value.Type() != js.TypeObject || value.IsNull() || !value.InstanceOf(uint8) || !object.Call("getPrototypeOf", value).Equal(uint8.Get("prototype")) {
		return nil, fmt.Errorf("master runtime BTB1 must be an ordinary Uint8Array")
	}
	// Read intrinsic typed-array accessors, rather than shadowable own fields.
	// This also rejects objects which merely inherit Uint8Array.prototype.
	prototype := object.Call("getPrototypeOf", uint8.Get("prototype"))
	get := func(name string) js.Value {
		getter := object.Call("getOwnPropertyDescriptor", prototype, name).Get("get")
		return getter.Call("call", value)
	}
	buffer := get("buffer")
	arrayBuffer := js.Global().Get("ArrayBuffer")
	if !buffer.InstanceOf(arrayBuffer) || !object.Call("getPrototypeOf", buffer).Equal(arrayBuffer.Get("prototype")) {
		return nil, fmt.Errorf("master runtime shared or nonstandard ArrayBuffer is unsupported")
	}
	length := get("byteLength").Int()
	offset := get("byteOffset").Int()
	if length < 16 || length > masterstructural.MaxInputBytes {
		return nil, fmt.Errorf("master runtime BTB1 byte length outside [16,%d]", masterstructural.MaxInputBytes)
	}
	// A subarray is supported: both copies use its actual offset and length,
	// never unrelated prefix/suffix bytes from the backing ArrayBuffer.
	header := make([]byte, 16)
	if js.CopyBytesToGo(header, uint8.New(buffer, offset, 16)) != 16 {
		return nil, fmt.Errorf("master runtime incomplete BTB1 header")
	}
	if err := masterstructural.ValidateRuntimeHeader(header, length); err != nil {
		return nil, err
	}
	data := make([]byte, length)
	if js.CopyBytesToGo(data, uint8.New(buffer, offset, length)) != length {
		return nil, fmt.Errorf("master runtime incomplete BTB1 copy")
	}
	return data, nil
}

func masterError(message string) string {
	out, _ := json.Marshal(map[string]string{"error": message})
	return string(out)
}
