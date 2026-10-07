// Package float64contract implements checked, explicitly rounded binary64
// operations for opt-in portable numerical contracts. Legacy paths do not call it.
package float64contract

import (
	"fmt"
	"math"
)

// Error marks an unexpected nonfinite operand or intermediate. Intentional
// warm-up sentinels must be handled by their owning algorithm before arithmetic.
type Error struct{ Operation string }

func (e *Error) Error() string { return fmt.Sprintf("portable binary64 nonfinite %s", e.Operation) }

func Check(value float64, operation string) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		panic(&Error{Operation: operation})
	}
	return value
}

// Each explicit conversion is a language-level rounding barrier. A temporary
// variable alone is insufficient: Go permits contraction across statements.
func Add(a, b float64) float64 {
	Check(a, "add operand")
	Check(b, "add operand")
	return Check(float64(a+b), "add")
}
func Sub(a, b float64) float64 {
	Check(a, "subtract operand")
	Check(b, "subtract operand")
	return Check(float64(a-b), "subtract")
}
func Mul(a, b float64) float64 {
	Check(a, "multiply operand")
	Check(b, "multiply operand")
	return Check(float64(a*b), "multiply")
}
func Div(a, b float64) float64 {
	Check(a, "divide operand")
	Check(b, "divide operand")
	return Check(float64(a/b), "divide")
}

// Recover converts only this package's arithmetic failure. Unexpected bugs are
// re-panicked. Callers also clear their result so no partial success can escape.
func Recover(err *error) {
	if value := recover(); value != nil {
		if failure, ok := value.(*Error); ok {
			*err = failure
			return
		}
		panic(value)
	}
}
