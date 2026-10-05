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
	var fixture RunFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return RunFixture{}, err
	}
	if fixture.Schema != runFixtureSchema {
		return RunFixture{}, fmt.Errorf("%s: schema %q, want %q", path, fixture.Schema, runFixtureSchema)
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
	fixture.ScanRawBarRows(data)
	fixture.Costs = fixture.Costs.normalized()
	fixture.Bars = rowsToBars(fixture.RawBars)
	fixture.SourceBars = rowsToBars(fixture.RawSourceBars)
	fixture.HTFBars = rowsToBars(fixture.RawHTFBars)
	fixture.SourceHTFBars = rowsToBars(fixture.RawSourceHTFBars)
	return fixture, nil
}

func rowsToBars(rows [][]float64) []marketdata.Bar {
	bars := make([]marketdata.Bar, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
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
	return bars
}

// ScanRawBarRows records the first bar-row element of data that is not a JSON
// number. Decoding into [][]float64 turns null into zero, which would let a
// missing value pass for a real quote; the clock range breakout run refuses a
// fixture with such a defect. Other families ignore it. Callers that decode a
// fixture themselves should call this with the same bytes.
func (f *RunFixture) ScanRawBarRows(data []byte) {
	f.rawRowDefect = ""
	var container struct {
		Bars json.RawMessage `json:"bars"`
	}
	if err := json.Unmarshal(data, &container); err != nil {
		f.rawRowDefect = "fixture is not a JSON object"
		return
	}
	if text := bytes.TrimSpace(container.Bars); len(text) == 0 || string(text) == "null" {
		f.rawRowDefect = "bars is missing or null; an empty array is the only way to supply no bars"
		return
	}
	var probe struct {
		Bars [][]json.RawMessage `json:"bars"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		f.rawRowDefect = "bars is not an array of rows"
		return
	}
	for i, row := range probe.Bars {
		for j, element := range row {
			text := bytes.TrimSpace(element)
			if len(text) == 0 || !(text[0] == '-' || text[0] >= '0' && text[0] <= '9') {
				f.rawRowDefect = fmt.Sprintf("bar row %d value %d is %s, not a number", i, j, string(text))
				return
			}
		}
	}
}
