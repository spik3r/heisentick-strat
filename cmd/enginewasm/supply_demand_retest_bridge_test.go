//go:build js && wasm

package main

import (
	"encoding/json"
	"strings"
	"syscall/js"
	"testing"

	"github.com/spik3r/heisentick-strat/report"
)

func TestSupplyDemandRetestBridgeStrictPrimitiveArguments(t *testing.T) {
	bad := []js.Value{js.Undefined(), js.Null(), js.ValueOf(true), js.ValueOf(3), js.Global().Get("Object").New(), js.Global().Get("Array").New(), js.Global().Get("String").New("{}"), js.Global().Get("BigInt").Invoke("1"), js.Global().Get("Symbol").Invoke("x")}
	call := func(args []js.Value) string {
		raw, ok := supplyDemandRetestJSONJS(js.Undefined(), args).(string)
		if !ok {
			t.Fatal("non-string response")
		}
		return raw
	}
	expected := string(report.SupplyDemandRetestFailure("invalid-arguments", "arguments"))
	for _, args := range [][]js.Value{nil, {js.ValueOf("{}")}, {js.ValueOf("{}"), js.ValueOf("x"), js.ValueOf("extra")}} {
		if got := call(args); got != expected {
			t.Fatalf("arity refusal %s", got)
		}
	}
	for _, value := range bad {
		for _, args := range [][]js.Value{{value, js.ValueOf("x")}, {js.ValueOf("{}"), value}} {
			if got := call(args); got != expected {
				t.Fatalf("nonprimitive refusal %s", got)
			}
		}
	}
	// Coercion hooks must not be invoked for a caller object.
	invoked := false
	hook := js.FuncOf(func(js.Value, []js.Value) any { invoked = true; return "{}" })
	defer hook.Release()
	obj := js.Global().Get("Object").New()
	obj.Set("toString", hook)
	if call([]js.Value{obj, js.ValueOf("x")}) != expected || invoked {
		t.Fatal("caller object was coerced")
	}
}

func TestSupplyDemandRetestBridgeExactUnicodeAndByteLimit(t *testing.T) {
	from := js.Global().Get("String").Get("fromCharCode")
	for _, value := range []js.Value{from.Invoke(0xd800), from.Invoke(0xdc00), from.Invoke(0xd800, 0x61), from.Invoke(0x61, 0xdc00)} {
		if _, ok := supplyDemandRetestString(value, 1024); ok {
			t.Fatal("unpaired UTF-16 accepted")
		}
		response := supplyDemandRetestJSONJS(js.Undefined(), []js.Value{value, js.ValueOf("x")}).(string)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(response), &fields); err != nil || len(fields) != 3 || fields["error"] == nil {
			t.Fatal("Unicode refusal leaked partial result")
		}
	}
	for _, s := range []string{"", "ascii", "Ω", "😀", "<>&\u2028\u2029"} {
		raw, ok := supplyDemandRetestString(js.ValueOf(s), len(s))
		if !ok || raw != s {
			t.Fatalf("exact UTF-8 changed: %q", s)
		}
		if len(s) > 0 {
			if _, ok := supplyDemandRetestString(js.ValueOf(s), len(s)-1); ok {
				t.Fatal("UTF-8 byte limit bypassed")
			}
		}
	}
	// Intrinsics were captured before this mutation.
	proto := js.Global().Get("String").Get("prototype")
	original := proto.Get("charCodeAt")
	fake := js.FuncOf(func(js.Value, []js.Value) any { return 65 })
	defer fake.Release()
	proto.Set("charCodeAt", fake)
	defer proto.Set("charCodeAt", original)
	if got, ok := supplyDemandRetestString(js.ValueOf("Ω😀"), 6); !ok || got != "Ω😀" {
		t.Fatal("mutable prototype changed admitted bytes")
	}
	if got := supplyDemandRetestJSONJS(js.Undefined(), []js.Value{js.ValueOf("{}"), js.ValueOf("x")}).(string); !strings.Contains(got, `"error"`) {
		t.Fatal("invalid input yielded success")
	}
}
