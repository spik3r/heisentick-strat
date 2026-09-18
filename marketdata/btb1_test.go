package marketdata

import (
	"strings"
	"testing"
)

func TestBTB1RoundTrip(t *testing.T) {
	rows := []Bar{
		{T: 1000, O: 1.1, H: 1.4, L: 1.0, C: 1.25, V: 42},
		{T: 2000, O: 1.25, H: 1.5, L: 1.2, C: 1.45, V: 7},
	}
	got, err := DecodeBTB1(EncodeBTB1(SeriesFromBars(rows)))
	if err != nil {
		t.Fatalf("DecodeBTB1: %v", err)
	}
	if got.Len() != len(rows) {
		t.Fatalf("Len = %d, want %d", got.Len(), len(rows))
	}
	for i, want := range rows {
		if got.Bar(i) != want {
			t.Fatalf("bar[%d] = %+v, want %+v", i, got.Bar(i), want)
		}
	}
}

func TestBTB1RejectsCorruptBuffers(t *testing.T) {
	_, err := DecodeBTB1(make([]byte, 32))
	if err == nil || !strings.Contains(err.Error(), "invalid bar binary magic") {
		t.Fatalf("DecodeBTB1 corrupt error = %v", err)
	}
}
