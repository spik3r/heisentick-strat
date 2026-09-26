package engine

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestAsiaLondonSweepStateMatchesBrowserSignalFixture(t *testing.T) {
	params := vpWideAsiaTestParams()
	fixture := readVPWideAsiaBrowserFixture(t)
	state := AsiaLondonSweepState{rangeHistory: append([]float64(nil), fixture.PriorAsiaRanges...)}
	for _, bar := range fixture.Bars[:3] {
		if signal := state.Process(bar, params); signal != nil {
			t.Fatalf("source bar at %v emitted a signal: %+v", bar.T, signal)
		}
	}
	signal := state.Process(fixture.Bars[3], params)
	if signal == nil {
		t.Fatal("expected first London sweep signal")
	}
	if signal.Side != fixture.Expected.Side || signal.PredictedSweep != fixture.Expected.PredictedSweep || signal.OpenOutsideCompletedVA != fixture.Expected.OpenOutsideCompletedVA {
		t.Fatalf("signal direction = %+v, want long on expected high sweep outside VA", signal)
	}
	for name, pair := range map[string][2]float64{
		"entry": {signal.Entry, fixture.Expected.Entry}, "stop": {signal.Stop, fixture.Expected.Stop}, "target": {signal.Target, fixture.Expected.Target},
		"POC": {signal.AsiaPOC, fixture.Expected.POC}, "VAH": {signal.AsiaVAH, fixture.Expected.VAH}, "VAL": {signal.AsiaVAL, fixture.Expected.VAL},
		"range": {signal.SourceRange, fixture.Expected.SourceRange}, "target distance R": {signal.FirstRaidTargetR, fixture.Expected.FirstRaidTargetR},
	} {
		if math.Abs(pair[0]-pair[1]) > 1e-9 {
			t.Errorf("%s = %.12g, want %.12g", name, pair[0], pair[1])
		}
	}
	if history := state.RangeHistory(); len(history) != 31 || history[len(history)-1] != 10 {
		t.Fatalf("range history = %v, want 30 prior ranges plus today's range", history)
	}
	if state.profile == nil || state.profile.ProfileHigh != 110 || len(state.profileBars) != 3 {
		t.Fatalf("frozen profile source = %+v / %d bars, want high 110 from 3 Asia bars", state.profile, len(state.profileBars))
	}
	if next := state.Process(fixture.Bars[3], params); next != nil {
		t.Fatalf("second call on the signal bar emitted another entry: %+v", next)
	}
}

func readVPWideAsiaBrowserFixture(t *testing.T) vpWideAsiaBrowserFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/asia-london-sweep-browser-fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture vpWideAsiaBrowserFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

type vpWideAsiaBrowserFixture struct {
	Bars            []marketdata.Bar         `json:"bars"`
	PriorAsiaRanges []float64                `json:"priorAsiaRanges"`
	ShortBars       []marketdata.Bar         `json:"shortBars"`
	Expected        vpWideAsiaExpectedSignal `json:"expected"`
	ExpectedShort   vpWideAsiaExpectedSignal `json:"expectedShort"`
}

type vpWideAsiaExpectedSignal struct {
	Side                   string  `json:"side"`
	Entry                  float64 `json:"entry"`
	Stop                   float64 `json:"stop"`
	Target                 float64 `json:"target"`
	PredictedSweep         string  `json:"predictedSweep"`
	OpenOutsideCompletedVA bool    `json:"openOutsideCompletedVA"`
	FirstRaidTargetR       float64 `json:"firstRaidTargetR"`
	SourceRange            float64 `json:"sourceRange"`
	POC                    float64 `json:"poc"`
	VAH                    float64 `json:"vah"`
	VAL                    float64 `json:"val"`
}

func TestAsiaLondonSweepStateMatchesBrowserShortSignalFixture(t *testing.T) {
	fixture := readVPWideAsiaBrowserFixture(t)
	state := AsiaLondonSweepState{rangeHistory: append([]float64(nil), fixture.PriorAsiaRanges...)}
	params := vpWideAsiaTestParams()
	for _, bar := range fixture.ShortBars[:3] {
		state.Process(bar, params)
	}
	signal := state.Process(fixture.ShortBars[3], params)
	if signal == nil {
		t.Fatal("expected first London short sweep signal")
	}
	want := fixture.ExpectedShort
	if signal.Side != want.Side || signal.PredictedSweep != want.PredictedSweep || signal.OpenOutsideCompletedVA != want.OpenOutsideCompletedVA {
		t.Fatalf("signal = %+v, want short on expected low sweep outside VA", signal)
	}
	for name, pair := range map[string][2]float64{
		"entry": {signal.Entry, want.Entry}, "stop": {signal.Stop, want.Stop}, "target": {signal.Target, want.Target},
		"POC": {signal.AsiaPOC, want.POC}, "VAH": {signal.AsiaVAH, want.VAH}, "VAL": {signal.AsiaVAL, want.VAL},
		"range": {signal.SourceRange, want.SourceRange}, "target distance R": {signal.FirstRaidTargetR, want.FirstRaidTargetR},
	} {
		if math.Abs(pair[0]-pair[1]) > 1e-9 {
			t.Errorf("%s = %.12g, want %.12g", name, pair[0], pair[1])
		}
	}
}

func TestAsiaLondonSweepStateInvalidatesOpposingOrAmbiguousFirstSweep(t *testing.T) {
	for _, tc := range []struct {
		name      string
		firstBar  marketdata.Bar
		wantSweep string
	}{
		{
			name:      "opposite edge first",
			firstBar:  marketdata.Bar{T: vpWideAsiaTime(7, 0), O: 109, H: 109.5, L: 99.5, C: 100, V: 20},
			wantSweep: "low",
		},
		{
			name:      "both edges in one bar",
			firstBar:  marketdata.Bar{T: vpWideAsiaTime(7, 0), O: 109, H: 111, L: 99.5, C: 110.5, V: 20},
			wantSweep: "both",
		},
		{
			name:      "expected edge without close through",
			firstBar:  marketdata.Bar{T: vpWideAsiaTime(7, 0), O: 109, H: 111, L: 108, C: 110, V: 20},
			wantSweep: "high",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := AsiaLondonSweepState{rangeHistory: make([]float64, 30)}
			for i := range state.rangeHistory {
				state.rangeHistory[i] = 10
			}
			params := vpWideAsiaTestParams()
			for _, bar := range vpWideAsiaFixtureBars()[:3] {
				state.Process(bar, params)
			}
			if signal := state.Process(tc.firstBar, params); signal != nil {
				t.Fatalf("invalid first sweep emitted signal: %+v", signal)
			}
			if state.FirstSweep() != tc.wantSweep {
				t.Fatalf("first sweep = %q, want %q", state.FirstSweep(), tc.wantSweep)
			}
			later := marketdata.Bar{T: vpWideAsiaTime(7, 5), O: 109, H: 111, L: 108, C: 110.5, V: 20}
			if signal := state.Process(later, params); signal != nil {
				t.Fatalf("later expected-edge break emitted signal after invalidation: %+v", signal)
			}
		})
	}
}

func TestAsiaLondonSweepStateUsesPriorDaysForRangeFilter(t *testing.T) {
	params := vpWideAsiaTestParams()
	params.MinAsiaRangePercentile = 0.67
	params.AsiaRangeLookback = 120
	params.MinAsiaRangeHistory = 2
	state := AsiaLondonSweepState{rangeHistory: []float64{8, 9}}
	for _, bar := range vpWideAsiaFixtureBars()[:3] {
		state.Process(bar, params)
	}
	bar := vpWideAsiaFixtureBars()[3]
	if signal := state.Process(bar, params); signal == nil {
		t.Fatal("current range 10 should pass the prior range threshold of 8")
	}
	if history := state.RangeHistory(); len(history) != 3 || history[2] != 10 {
		t.Fatalf("history after evaluation = %v, want [8 9 10]", history)
	}

	blocked := AsiaLondonSweepState{rangeHistory: []float64{11, 12}}
	for _, sourceBar := range vpWideAsiaFixtureBars()[:3] {
		blocked.Process(sourceBar, params)
	}
	if signal := blocked.Process(bar, params); signal != nil {
		t.Fatalf("current range 10 should fail a prior threshold of 11: %+v", signal)
	}
	if !blocked.done {
		t.Fatal("range-filter rejection should invalidate today's setup")
	}
}

func vpWideAsiaTestParams() AsiaLondonSweepParams {
	return AsiaLondonSweepParams{
		TickSize: 0.1, RowsLayout: "number_of_rows", SessionRows: 24, ValueAreaPercent: 70,
		SourceStartHour: 0, AsiaEndHour: 7, LondonStartHour: 7, LondonEndHour: 16,
		AsiaRangeLookback: 120, MinAsiaRangeHistory: 30, MaxTargetDistanceRange: 0.25,
		StopBufferRange: 0.35, RMultiple: 0.5, RequireOpenOutsideCompletedValue: true,
	}
}

func vpWideAsiaFixtureBars() []marketdata.Bar {
	return []marketdata.Bar{
		{T: vpWideAsiaTime(0, 0), O: 100, H: 102, L: 100, C: 101, V: 100},
		{T: vpWideAsiaTime(3, 0), O: 100, H: 102, L: 100, C: 101, V: 100},
		{T: vpWideAsiaTime(6, 55), O: 100, H: 110, L: 100, C: 101, V: 10},
		{T: vpWideAsiaTime(7, 0), O: 109, H: 111, L: 108, C: 110.5, V: 20},
	}
}

func vpWideAsiaTime(hour, minute int) float64 {
	base := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	return float64(base + int64(hour*60+minute)*60_000)
}
