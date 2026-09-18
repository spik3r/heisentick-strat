package main

import (
	"encoding/json"
	"fmt"
	native "github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func runFixture(raw, source string) ([]byte, error) {
	var fixture native.RunFixture
	if err := json.Unmarshal([]byte(raw), &fixture); err != nil {
		return nil, err
	}
	if fixture.Schema != "dsl-conformance-run-fixture-v1" {
		return nil, fmt.Errorf("unsupported fixture schema %q", fixture.Schema)
	}
	// Mirror LoadRunFixture's input adapter; the engine's Bars fields are json:"-".
	fixture.Bars = rowsToBars(fixture.RawBars)
	fixture.SourceBars = rowsToBars(fixture.RawSourceBars)
	fixture.HTFBars = rowsToBars(fixture.RawHTFBars)
	fixture.SourceHTFBars = rowsToBars(fixture.RawSourceHTFBars)
	if fixture.Costs.FillOn == "" {
		fixture.Costs.FillOn = "close"
	}
	if fixture.Costs.StartEquity == 0 {
		fixture.Costs.StartEquity = 10000
	}
	result, err := native.RunFixtureCase(fixture, source)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func rowsToBars(rows [][]float64) []marketdata.Bar {
	bars := make([]marketdata.Bar, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		bars = append(bars, marketdata.Bar{T: row[0], O: row[1], H: row[2], L: row[3], C: row[4], V: row[5]})
	}
	return bars
}
