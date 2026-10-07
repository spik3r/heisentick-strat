package float64contract

import (
	"encoding/binary"
	"math"
	"testing"
)

// Operands are decoded from runtime bytes, not constant expressions. Expected
// bits follow IEEE binary64 round-to-nearest/even, independently of Go FMA.
func fromBits(bits uint64) float64 {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, bits)
	return math.Float64frombits(binary.LittleEndian.Uint64(b))
}
func TestPortablePrimitiveExpectedBits(t *testing.T) {
	cases := []struct {
		name       string
		op         func(float64, float64) float64
		a, b, want uint64
	}{
		{"halfway-even-down", Add, 0x3ff0000000000000, 0x3ca0000000000000, 0x3ff0000000000000},
		{"halfway-even-up", Add, 0x3ff0000000000001, 0x3ca0000000000000, 0x3ff0000000000002},
		{"negative-zero-multiply", Mul, 0x8000000000000000, 0x3ff0000000000000, 0x8000000000000000},
		{"negative-zero-add", Add, 0x8000000000000000, 0x8000000000000000, 0x8000000000000000},
		{"negative-zero-subtract", Sub, 0x8000000000000000, 0, 0x8000000000000000},
		{"subnormal", Mul, 0x0010000000000000, 0x3fe0000000000000, 0x0008000000000000},
		{"subnormal-halfway-zero", Div, 1, 0x4000000000000000, 0},
		{"negative-underflow-zero", Div, 0x8000000000000001, 0x4000000000000000, 0x8000000000000000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := math.Float64bits(c.op(fromBits(c.a), fromBits(c.b)))
			if got != c.want {
				t.Fatalf("bits %016x want %016x", got, c.want)
			}
		})
	}
}
func TestPortableProductBarrierPreventsFMA(t *testing.T) {
	a, b, c := fromBits(0x3ff0000002000000), fromBits(0x3feffffffc000000), fromBits(0xbff0000000000000)
	if got := math.Float64bits(Add(Mul(a, b), c)); got != 0 {
		t.Fatalf("separated result %016x", got)
	}
	if got := math.Float64bits(math.FMA(a, b, c)); got != 0xbc90000000000000 {
		t.Fatalf("fused control %016x", got)
	}
}
func TestPortableUnexpectedNonfiniteIsImmediate(t *testing.T) {
	for _, fn := range []func(){func() { Mul(math.MaxFloat64, 2) }, func() { Add(math.MaxFloat64, math.MaxFloat64) }, func() { Sub(math.MaxFloat64, -math.MaxFloat64) }, func() { Div(1, 0) }, func() { Add(math.NaN(), 1) }, func() { Mul(math.Inf(1), 0) }} {
		func() {
			defer func() {
				if _, ok := recover().(*Error); !ok {
					t.Error("missing arithmetic failure")
				}
			}()
			fn()
			t.Error("nonfinite operation returned")
		}()
	}
	// A later finite min/max or cancellation must never mask the overflow.
	func() {
		defer func() {
			if _, ok := recover().(*Error); !ok {
				t.Error("masked overflow")
			}
		}()
		_ = math.Min(Mul(math.MaxFloat64, 2), 1)
		t.Error("min masked overflow")
	}()
}
func TestPortableRecoverDoesNotSwallowBugs(t *testing.T) {
	var err error
	func() { defer Recover(&err); Add(math.Inf(1), 1) }()
	if err == nil {
		t.Fatal("typed error not recovered")
	}
	func() {
		defer func() {
			if recover() != "unexpected" {
				t.Error("unrelated panic swallowed")
			}
		}()
		func() { defer Recover(&err); panic("unexpected") }()
	}()
}
