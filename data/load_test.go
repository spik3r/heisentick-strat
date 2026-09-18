package data

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestLoadValidBTB1(t *testing.T) {
	root := t.TempDir()
	rows := []marketdata.Bar{
		{T: 1000, O: 10, H: 12, L: 9, C: 11, V: 100},
		{T: 2000, O: 11, H: 13, L: 10.5, C: 12.5, V: 125},
	}
	writeFixture(t, root, "XAUUSD", "30m", marketdata.EncodeBTB1(marketdata.SeriesFromBars(rows)))

	got, err := Load(root, "XAUUSD", "30m")
	if err != nil {
		t.Fatalf("Load: %v", err)
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

func TestLoadMissingFileIncludesPathAndWrapsNotExist(t *testing.T) {
	root := t.TempDir()
	_, err := Load(root, "XAUUSD", "5m")
	if err == nil {
		t.Fatal("Load missing file succeeded")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load missing file error = %v, want os.ErrNotExist", err)
	}
	wantPath := filepath.Join(root, "XAUUSD", "5m.bin")
	if !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("Load missing file error = %q, want path %q", err.Error(), wantPath)
	}
}

func TestLoadCorruptFileIncludesPathAndDecodeReason(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "XAUUSD", "1h", []byte("not a BTB1 file"))

	_, err := Load(root, "XAUUSD", "1h")
	if err == nil {
		t.Fatal("Load corrupt file succeeded")
	}
	wantPath := filepath.Join(root, "XAUUSD", "1h.bin")
	if !strings.Contains(err.Error(), wantPath) {
		t.Fatalf("Load corrupt file error = %q, want path %q", err.Error(), wantPath)
	}
	if !strings.Contains(err.Error(), "invalid bar binary magic") {
		t.Fatalf("Load corrupt file error = %q, want BTB1 decode reason", err.Error())
	}
}

func TestPathRejectsUnsafeOrUnsupportedInputs(t *testing.T) {
	tests := []struct {
		name      string
		symbol    string
		timeframe string
	}{
		{name: "symbol traversal", symbol: "../XAUUSD", timeframe: "30m"},
		{name: "lowercase symbol", symbol: "xauusd", timeframe: "30m"},
		{name: "empty symbol", symbol: "", timeframe: "30m"},
		{name: "timeframe traversal", symbol: "XAUUSD", timeframe: "../30m"},
		{name: "uppercase timeframe unit", symbol: "XAUUSD", timeframe: "30M"},
		{name: "zero timeframe", symbol: "XAUUSD", timeframe: "0m"},
		{name: "leading zero timeframe", symbol: "XAUUSD", timeframe: "01m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Path(t.TempDir(), tt.symbol, tt.timeframe)
			if err == nil {
				t.Fatalf("Path(%q, %q) succeeded", tt.symbol, tt.timeframe)
			}
		})
	}
}

func TestPathAcceptsTickTimeframes(t *testing.T) {
	path, err := Path(t.TempDir(), "EURUSD", "70t")
	if err != nil {
		t.Fatalf("Path tick timeframe: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("EURUSD", "70t.bin")) {
		t.Fatalf("Path tick timeframe = %q, want EURUSD/70t.bin suffix", path)
	}
}

func writeFixture(t testing.TB, root string, symbol string, timeframe string, data []byte) {
	t.Helper()
	dir := filepath.Join(root, symbol)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, timeframe+".bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
