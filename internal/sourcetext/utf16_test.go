package sourcetext

import (
	"strings"
	"testing"
)

func checkUnits(units []uint16) error {
	return validateUTF16(len(units), func(i int) uint16 { return units[i] })
}

func TestEverySingleCodeUnit(t *testing.T) {
	for unit := 0; unit <= 0xffff; unit++ {
		err := checkUnits([]uint16{uint16(unit)})
		wantError := unit >= 0xd800 && unit <= 0xdfff
		if (err != nil) != wantError {
			t.Fatalf("single code unit U+%04X: error=%v, want error=%v", unit, err, wantError)
		}
	}
}

func TestSurrogatePairBoundaries(t *testing.T) {
	for _, high := range []uint16{0xd800, 0xdbff} {
		for _, low := range []uint16{0xdc00, 0xdfff} {
			for _, units := range [][]uint16{
				{high, low},
				{'a', high, low, 0xfffd, 'z'},
				{high, low, high, low},
			} {
				if err := checkUnits(units); err != nil {
					t.Fatalf("valid code units %x rejected: %v", units, err)
				}
			}
		}
	}
	if err := checkUnits(nil); err != nil {
		t.Fatalf("empty source rejected: %v", err)
	}
}

func TestUnpairedSurrogatesInContext(t *testing.T) {
	for _, units := range [][]uint16{
		{'a', 0xd800},
		{'a', 0xdbff},
		{'a', 0xdc00, 'z'},
		{'a', 0xdfff, 'z'},
		{'a', 0xd800, 'z'},
		{'a', 0xd800, 0xd800, 0xdc00},
		{'a', 0xdc00, 0xd800},
		{'a', 0xd800, 0xfffd, 0xdc00},
	} {
		err := checkUnits(units)
		if err == nil || !strings.Contains(err.Error(), "at code unit 1") {
			t.Fatalf("malformed code units %x: expected error at original offset 1, got %v", units, err)
		}
	}
	for _, units := range [][]uint16{
		{0xd800, 0xdc00, 0xd800},
		{0xd800, 0xdc00, 0xdc00},
	} {
		err := checkUnits(units)
		if err == nil || !strings.Contains(err.Error(), "at code unit 2") {
			t.Fatalf("malformed code units %x: expected error after valid pair, got %v", units, err)
		}
	}
}
