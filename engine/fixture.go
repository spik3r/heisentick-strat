package engine

import (
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
