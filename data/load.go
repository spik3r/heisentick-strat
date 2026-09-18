// Package data loads repository market-data files for the Go research engine.
package data

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spik3r/heisentick-strat/marketdata"
)

const binExt = ".bin"

// Path returns the repository-style path for a market data binary:
// <root>/<SYMBOL>/<timeframe>.bin.
func Path(root string, symbol string, timeframe string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("data root is required")
	}
	if err := ValidateSymbol(symbol); err != nil {
		return "", err
	}
	if err := ValidateTimeframe(timeframe); err != nil {
		return "", err
	}
	return filepath.Join(root, symbol, timeframe+binExt), nil
}

// Load reads a repository-style market-data binary from root/symbol/timeframe
// and decodes it with the shared BTB1 reader.
func Load(root string, symbol string, timeframe string) (marketdata.Series, error) {
	path, err := Path(root, symbol, timeframe)
	if err != nil {
		return marketdata.Series{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return marketdata.Series{}, fmt.Errorf("read market data %s: %w", path, err)
	}
	defer file.Close()
	series, err := marketdata.DecodeBTB1Reader(file)
	if err != nil {
		return marketdata.Series{}, fmt.Errorf("decode market data %s: %w", path, err)
	}
	return series, nil
}

// ValidateSymbol accepts repository symbols such as XAUUSD, EURUSD, or US30.
// The deliberately narrow form prevents path traversal and ambiguous paths.
func ValidateSymbol(symbol string) error {
	if len(symbol) < 2 || len(symbol) > 32 {
		return fmt.Errorf("invalid symbol %q: must match [A-Z][A-Z0-9]+", symbol)
	}
	for i, r := range symbol {
		switch {
		case i == 0 && r >= 'A' && r <= 'Z':
		case i > 0 && ((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')):
		default:
			return fmt.Errorf("invalid symbol %q: must match [A-Z][A-Z0-9]+", symbol)
		}
	}
	return nil
}

// ValidateTimeframe accepts compact repository timeframes such as 1m, 30m, 1h,
// 4h, 1d, 1w, and tick bars such as 70t. The form is path-safe and rejects
// zero-length periods.
func ValidateTimeframe(timeframe string) error {
	if len(timeframe) < 2 || len(timeframe) > 16 {
		return fmt.Errorf("invalid timeframe %q: must match [1-9][0-9]*(m|h|d|w|t)", timeframe)
	}
	unit := timeframe[len(timeframe)-1]
	if unit != 'm' && unit != 'h' && unit != 'd' && unit != 'w' && unit != 't' {
		return fmt.Errorf("invalid timeframe %q: must match [1-9][0-9]*(m|h|d|w|t)", timeframe)
	}
	if timeframe[0] < '1' || timeframe[0] > '9' {
		return fmt.Errorf("invalid timeframe %q: must match [1-9][0-9]*(m|h|d|w|t)", timeframe)
	}
	n, err := strconv.Atoi(timeframe[:len(timeframe)-1])
	if err != nil || n <= 0 {
		return fmt.Errorf("invalid timeframe %q: must match [1-9][0-9]*(m|h|d|w|t)", timeframe)
	}
	return nil
}
