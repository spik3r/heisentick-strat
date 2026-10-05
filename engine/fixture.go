package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spik3r/heisentick-strat/marketdata"
)

// LoadRunFixture reads one conformance run fixture without mutating goldens.
func LoadRunFixture(path string) (RunFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RunFixture{}, err
	}
	fixture, err := DecodeRunFixture(data)
	if err != nil {
		return RunFixture{}, fmt.Errorf("%s: %w", path, err)
	}
	return fixture, nil
}

// DecodeRunFixture is the shared native/WASM fixture adapter. A malformed
// row must fail visibly rather than shifting the indices of all later bars.
func DecodeRunFixture(data []byte) (RunFixture, error) {
	var fixture RunFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return RunFixture{}, err
	}
	if fixture.Schema != runFixtureSchema {
		return RunFixture{}, fmt.Errorf("schema %q, want %q", fixture.Schema, runFixtureSchema)
	}
	if fixture.TimedCalendar != nil {
		var strict struct {
			RunFixture
			Source         json.RawMessage `json:"source,omitempty"`
			ContextOptions map[string]any  `json:"contextOptions"`
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&strict); err != nil {
			return RunFixture{}, fmt.Errorf("timed fixture JSON: %w", err)
		}
		if len(strict.ContextOptions) != 0 {
			return RunFixture{}, fmt.Errorf("timed fixture rejects context options")
		}
	}
	fixture.Costs = fixture.Costs.normalized()
	for _, series := range []struct {
		name string
		raw  [][]float64
		out  *[]marketdata.Bar
	}{
		{"bars", fixture.RawBars, &fixture.Bars},
		{"sourceBars", fixture.RawSourceBars, &fixture.SourceBars},
		{"htfBars", fixture.RawHTFBars, &fixture.HTFBars},
		{"sourceHtfBars", fixture.RawSourceHTFBars, &fixture.SourceHTFBars},
	} {
		bars, err := decodeFixtureBars(series.name, series.raw)
		if err != nil {
			return RunFixture{}, err
		}
		*series.out = bars
	}
	return fixture, nil
}

func decodeFixtureBars(name string, rows [][]float64) ([]marketdata.Bar, error) {
	bars := make([]marketdata.Bar, 0, len(rows))
	for i, row := range rows {
		if len(row) != 6 {
			return nil, fmt.Errorf("%s[%d]: expected six OHLCV values, got %d", name, i, len(row))
		}
		bars = append(bars, marketdata.Bar{
			T: row[0],
			O: row[1],
			H: row[2],
			L: row[3],
			C: row[4],
			V: row[5],
		})
	}
	return bars, nil
}
