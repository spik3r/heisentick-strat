// Package sourcetext validates source transport before a decoder can replace
// malformed Unicode with U+FFFD. It does not interpret or normalize DSL syntax.
package sourcetext

import "fmt"

// validateUTF16 reads original code units without first converting them to a Go
// string. Every surrogate must belong to an adjacent high/low pair. U+FFFD is
// ordinary valid text and must not be used as a proxy for malformed input.
func validateUTF16(length int, codeUnit func(int) uint16) error {
	for i := 0; i < length; i++ {
		unit := codeUnit(i)
		switch {
		case unit >= 0xd800 && unit <= 0xdbff:
			if i+1 < length {
				next := codeUnit(i + 1)
				if next >= 0xdc00 && next <= 0xdfff {
					i++
					continue
				}
			}
			return fmt.Errorf("DSL source is not valid UTF-16: unpaired surrogate at code unit %d", i)
		case unit >= 0xdc00 && unit <= 0xdfff:
			return fmt.Errorf("DSL source is not valid UTF-16: unpaired surrogate at code unit %d", i)
		}
	}
	return nil
}
