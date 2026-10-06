//go:build js && wasm

package sourcetext

import (
	"fmt"
	"syscall/js"
)

// FromJS converts a source string only after validating its original UTF-16.
// js.Value.String uses UTF-8 encoding that repairs unpaired surrogates, so a
// subsequent utf8.ValidString check alone cannot enforce the source boundary.
func FromJS(source js.Value) (string, error) {
	if source.Type() != js.TypeString {
		return "", fmt.Errorf("DSL source must be a string")
	}
	// syscall/js property access requires an object. Boxing preserves the
	// original string's code units; neither this nor charCodeAt decodes them.
	boxed := js.Global().Get("Object").Invoke(source)
	if err := validateUTF16(boxed.Length(), func(i int) uint16 {
		return uint16(boxed.Call("charCodeAt", i).Int())
	}); err != nil {
		return "", err
	}
	return source.String(), nil
}
