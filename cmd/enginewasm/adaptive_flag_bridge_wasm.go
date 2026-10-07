//go:build js && wasm

package main

import (
	"encoding/binary"
	"fmt"
	"syscall/js"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
)

// Capture the intrinsic operations once, rather than reading shadowable
// properties from caller-owned strings, typed arrays, or backing buffers.
type adaptiveFlagIntrinsics struct {
	object, prototypeOf, apply, bind                            js.Value
	uint8Prototype, bufferPrototype                             js.Value
	viewBuffer, viewLength, viewOffset, bufferLength, viewBrand js.Value
	dataView, readWord, charCodeAt                              js.Value
}

var adaptiveFlagJS adaptiveFlagIntrinsics

func init() {
	object := js.Global().Get("Object")
	prototypeOf := object.Get("getPrototypeOf")
	uint8Prototype := js.Global().Get("Uint8Array").Get("prototype")
	typedArrayPrototype := prototypeOf.Invoke(uint8Prototype)
	bufferPrototype := js.Global().Get("ArrayBuffer").Get("prototype")
	getter := func(prototype js.Value, name string) js.Value {
		return object.Call("getOwnPropertyDescriptor", prototype, name).Get("get")
	}
	dataView := js.Global().Get("DataView")
	adaptiveFlagJS = adaptiveFlagIntrinsics{
		object: object, prototypeOf: prototypeOf,
		apply:          js.Global().Get("Reflect").Get("apply"),
		bind:           js.Global().Get("Function").Get("prototype").Get("bind"),
		uint8Prototype: uint8Prototype, bufferPrototype: bufferPrototype,
		viewBuffer:   getter(typedArrayPrototype, "buffer"),
		viewLength:   getter(typedArrayPrototype, "byteLength"),
		viewOffset:   getter(typedArrayPrototype, "byteOffset"),
		bufferLength: getter(bufferPrototype, "byteLength"),
		viewBrand: object.Call("getOwnPropertyDescriptor", typedArrayPrototype,
			js.Global().Get("Symbol").Get("toStringTag")).Get("get"),
		dataView: dataView, readWord: dataView.Get("prototype").Get("getUint32"),
		charCodeAt: js.Global().Get("String").Get("prototype").Get("charCodeAt"),
	}
	js.Global().Set("engineRunAdaptiveFlagReport", js.FuncOf(adaptiveFlagReportJS))
}

func (intrinsic adaptiveFlagIntrinsics) call(function, receiver js.Value, args ...any) js.Value {
	return intrinsic.apply.Invoke(function, receiver, args)
}

func (intrinsic adaptiveFlagIntrinsics) bound(function, receiver js.Value) js.Value {
	return intrinsic.call(intrinsic.bind, function, receiver)
}

func adaptiveFlagReportJS(_ js.Value, args []js.Value) (response any) {
	phase := "request"
	defer func() {
		if recover() != nil {
			response = adaptiveflag.ErrorJSON(adaptiveflag.Rejection(phase, "invalid or inaccessible adaptive runtime transport value"))
		}
	}()
	if len(args) != 3 {
		return adaptiveflag.ErrorJSON(adaptiveflag.Rejection("request", "expected exactly metadata JSON, DSL source and a Uint8Array BTB1 view"))
	}
	metadata, err := adaptiveFlagBoundedJSString(args[0], adaptiveflag.MaxMetadataBytes, "request")
	if err != nil {
		return adaptiveflag.ErrorJSON(err)
	}
	phase = "source"
	source, err := adaptiveFlagBoundedJSString(args[1], dsl.AdaptiveFlagRuntimeMaxSourceBytes, "source")
	if err != nil {
		return adaptiveflag.ErrorJSON(err)
	}
	// This includes strict metadata/source parsing, diagnostics, named-bundle
	// decoding and timeframe agreement. No bar access or full copy precedes it.
	prepared, err := adaptiveflag.PrepareRuntime(metadata, source)
	if err != nil {
		return adaptiveflag.ErrorJSON(err)
	}
	phase = "input"
	data, err := copyAdaptiveFlagBTB1(args[2])
	if err != nil {
		return adaptiveflag.ErrorJSON(err)
	}
	phase = "execution"
	out, err := prepared.Build(data)
	if err != nil {
		return adaptiveflag.ErrorJSON(err)
	}
	return string(out)
}

func adaptiveFlagBoundedJSString(value js.Value, max int, phase string) (string, error) {
	if value.Type() != js.TypeString {
		return "", adaptiveflag.Rejection(phase, "expected a primitive string")
	}
	// A boxed primitive has an intrinsic nonconfigurable own length property.
	// Bound UTF-16 units before walking, allocating or converting the string.
	units := adaptiveFlagJS.object.Invoke(value).Length()
	tooLarge := func() error {
		return adaptiveflag.Rejection("resource", fmt.Sprintf("adaptive runtime %s exceeds %d UTF-8 bytes", phase, max))
	}
	if units > max {
		return "", tooLarge()
	}
	charCodeAt := adaptiveFlagJS.bound(adaptiveFlagJS.charCodeAt, value)
	bytes := 0
	for i := 0; i < units; i++ {
		unit := charCodeAt.Invoke(i).Int()
		switch {
		case unit >= 0xd800 && unit <= 0xdbff:
			if i+1 >= units {
				return "", adaptiveflag.Rejection(phase, "string contains an unpaired UTF-16 surrogate")
			}
			next := charCodeAt.Invoke(i + 1).Int()
			if next < 0xdc00 || next > 0xdfff {
				return "", adaptiveflag.Rejection(phase, "string contains an unpaired UTF-16 surrogate")
			}
			i++
			bytes += 4
		case unit >= 0xdc00 && unit <= 0xdfff:
			return "", adaptiveflag.Rejection(phase, "string contains an unpaired UTF-16 surrogate")
		case unit < 0x80:
			bytes++
		case unit < 0x800:
			bytes += 2
		default:
			bytes += 3
		}
	}
	if bytes > max {
		return "", tooLarge()
	}
	return value.String(), nil
}

func copyAdaptiveFlagBTB1(value js.Value) (data []byte, err error) {
	phase := "input"
	defer func() {
		if recover() != nil {
			data = nil
			err = adaptiveflag.Rejection(phase, "invalid or inaccessible BTB1 view or copy")
		}
	}()
	if value.Type() != js.TypeObject || value.IsNull() {
		return nil, adaptiveflag.Rejection("input", "BTB1 must be an ordinary Uint8Array")
	}
	intrinsic := adaptiveFlagJS
	// Calling the brand-checked accessors first rejects proxies and objects that
	// merely inherit a Uint8Array prototype, without invoking their own getters.
	buffer := intrinsic.call(intrinsic.viewBuffer, value)
	length := intrinsic.call(intrinsic.viewLength, value).Float()
	offset := intrinsic.call(intrinsic.viewOffset, value).Float()
	// A different genuine typed-array kind can have its prototype changed to
	// Uint8Array.prototype. Only the intrinsic tag getter reads its internal
	// TypedArrayName slot; neither prototype identity nor toString is enough.
	brand := intrinsic.call(intrinsic.viewBrand, value)
	if brand.Type() != js.TypeString || brand.String() != "Uint8Array" || !intrinsic.prototypeOf.Invoke(value).Equal(intrinsic.uint8Prototype) {
		return nil, adaptiveflag.Rejection("input", "BTB1 must be an ordinary Uint8Array")
	}
	// ArrayBuffer's intrinsic byteLength getter rejects SharedArrayBuffer.
	// Exact prototypes also exclude subclasses and caller-supplied wrappers.
	bufferLength := intrinsic.call(intrinsic.bufferLength, buffer).Float()
	if !intrinsic.prototypeOf.Invoke(buffer).Equal(intrinsic.bufferPrototype) {
		return nil, adaptiveflag.Rejection("input", "BTB1 requires an ordinary nonshared ArrayBuffer")
	}
	if length > adaptiveflag.MaxInputBytes {
		return nil, adaptiveflag.Rejection("resource", "BTB1 view exceeds the runtime byte limit")
	}
	if length < 16 || offset < 0 || offset > bufferLength-length {
		return nil, adaptiveflag.Rejection("input", "BTB1 view is detached, short or outside its backing buffer")
	}
	// Offsets remain JS Numbers: narrowing a large backing-buffer offset to the
	// wasm int width could wrap before construction of the selected view.
	headerView := intrinsic.dataView.New(buffer, offset, 16)
	var header [16]byte
	copyAdaptiveFlagWords(header[:], headerView)
	if err := adaptiveflag.ValidateRuntimeHeader(header[:], int(length)); err != nil {
		return nil, err
	}
	phase = "resource"
	data = make([]byte, int(length))
	phase = "input"
	copyAdaptiveFlagWords(data, intrinsic.dataView.New(buffer, offset, length))
	return data, nil
}

// Read raw words, not Float64 values, so NaN payloads and negative zero are
// preserved exactly. Each bound intrinsic call uses syscall/js valueInvoke,
// whose exception path becomes a recoverable Go panic. Go 1.22's bulk
// CopyBytesToGo host import has no exception guard and calls a shadowable JS
// subarray method; using it here would allow a copy trap to escape the runtime.
// Header admission guarantees all copied lengths are multiples of four.
func copyAdaptiveFlagWords(dst []byte, view js.Value) {
	readWord := adaptiveFlagJS.bound(adaptiveFlagJS.readWord, view)
	for offset := 0; offset < len(dst); offset += 4 {
		word := uint32(readWord.Invoke(offset, true).Float())
		binary.LittleEndian.PutUint32(dst[offset:offset+4], word)
	}
}
