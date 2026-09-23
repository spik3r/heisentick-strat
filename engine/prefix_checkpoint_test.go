package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func requireSameJSON(t *testing.T, got, want any) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got JSON: %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal wanted JSON: %v", err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("JSON differs\ngot:  %s\nwant: %s", gotJSON, wantJSON)
	}
}

func openingRangeCheckpointRequest(t *testing.T, barCount int, fillOn string) RunRequest {
	t.Helper()
	fixture, err := LoadRunFixture(filepath.Join(runFixtureDir(), "family-opening-range-breakout.fixture.json"))
	if err != nil {
		t.Fatalf("load ORB fixture: %v", err)
	}
	if barCount > len(fixture.Bars) {
		t.Fatalf("ORB fixture bars = %d, need %d", len(fixture.Bars), barCount)
	}
	source, err := os.ReadFile(filepath.Join(runFixtureDir(), "family-opening-range-breakout.strat"))
	if err != nil {
		t.Fatalf("read ORB source: %v", err)
	}
	parsed, err := dsl.Parse(string(source))
	if err != nil || len(parsed.Errors) != 0 {
		t.Fatalf("parse ORB source: %v / %v", err, parsed.Errors)
	}
	lastT := fixture.Bars[barCount-1].T
	htfCount := 0
	for htfCount < len(fixture.HTFBars) && fixture.HTFBars[htfCount].T <= lastT {
		htfCount++
	}
	costs := fixture.Costs
	costs.FillOn = fillOn
	return RunRequest{
		Config: parsed.Config, Series: marketdata.SeriesFromBars(fixture.Bars[:barCount]),
		HTFSeries:  marketdata.SeriesFromBars(fixture.HTFBars[:htfCount]),
		StrategyID: fixture.StrategyID, Symbol: fixture.Symbol, Timeframe: fixture.Timeframe,
		HigherTimeframe: fixture.HigherTimeframe, RangeMethod: fixture.RangeMethod, Costs: costs,
	}
}

func TestRunPrefixResumableMatchesFullReplayAcrossOpenPosition(t *testing.T) {
	const splitBars = 36
	const fullBars = 1800
	first, checkpoint, err := RunPrefixResumable(openingRangeCheckpointRequest(t, splitBars, "close"), nil)
	if err != nil {
		t.Fatalf("initial checkpoint prefix: %v", err)
	}
	if len(first.Trades) != 0 || len(first.OpenPositions) != 1 || len(checkpoint) == 0 {
		t.Fatalf("initial result = %#v checkpoint=%s, want one open position", first, checkpoint)
	}
	positionID := first.OpenPositions[0].PositionID

	resumed, next, err := RunPrefixResumable(openingRangeCheckpointRequest(t, fullBars, "close"), checkpoint)
	if err != nil {
		t.Fatalf("resume checkpoint prefix: %v", err)
	}
	full, err := RunPrefix(openingRangeCheckpointRequest(t, fullBars, "close"))
	if err != nil {
		t.Fatalf("full replay: %v", err)
	}
	combined := append(append([]PrefixClosedTrade(nil), first.Trades...), resumed.Trades...)
	requireSameJSON(t, combined, full.Trades)
	requireSameJSON(t, resumed.OpenPositions, full.OpenPositions)
	if resumed.CheckpointDigest != full.CheckpointDigest {
		t.Fatalf("resumed digest = %q, want %q", resumed.CheckpointDigest, full.CheckpointDigest)
	}
	matches := 0
	for _, closed := range resumed.Trades {
		if closed.PositionID == positionID {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("stable position %q closed %d times, want once", positionID, matches)
	}

	repeated, repeatedCheckpoint, err := RunPrefixResumable(openingRangeCheckpointRequest(t, fullBars, "close"), next)
	if err != nil {
		t.Fatalf("repeat completed checkpoint: %v", err)
	}
	if len(repeated.Trades) != 0 || string(repeatedCheckpoint) != string(next) {
		t.Fatalf("repeat result = %#v, want no duplicate closes and stable checkpoint", repeated)
	}
	requireSameJSON(t, repeated.OpenPositions, resumed.OpenPositions)
}

func TestRunPrefixResumableRestoresPendingNextOpenOrder(t *testing.T) {
	const splitBars = 36
	const fullBars = 1800
	first, checkpoint, err := RunPrefixResumable(openingRangeCheckpointRequest(t, splitBars, "open"), nil)
	if err != nil {
		t.Fatalf("initial pending-order prefix: %v", err)
	}
	if len(first.Trades) != 0 || len(first.OpenPositions) != 0 {
		t.Fatalf("initial pending-order result = %#v, want no filled position", first)
	}
	resumed, _, err := RunPrefixResumable(openingRangeCheckpointRequest(t, fullBars, "open"), checkpoint)
	if err != nil {
		t.Fatalf("resume pending-order prefix: %v", err)
	}
	full, err := RunPrefix(openingRangeCheckpointRequest(t, fullBars, "open"))
	if err != nil {
		t.Fatalf("full pending-order replay: %v", err)
	}
	requireSameJSON(t, resumed.Trades, full.Trades)
	requireSameJSON(t, resumed.OpenPositions, full.OpenPositions)
}

func TestRunPrefixResumableRejectsTamperedCheckpoint(t *testing.T) {
	request := openingRangeCheckpointRequest(t, 36, "close")
	_, checkpoint, err := RunPrefixResumable(request, nil)
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(checkpoint, &envelope); err != nil {
		t.Fatalf("decode opaque checkpoint in test: %v", err)
	}
	for _, tc := range []struct {
		name  string
		field string
		value any
	}{
		{name: "version", field: "schema", value: "heisentick-prefix-checkpoint-v2"},
		{name: "binding", field: "bindingDigest", value: "sha256:tampered"},
		{name: "prefix", field: "admittedPrefixDigest", value: "sha256:tampered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := make(map[string]any, len(envelope))
			for key, value := range envelope {
				copy[key] = value
			}
			copy[tc.field] = tc.value
			raw, err := json.Marshal(copy)
			if err != nil {
				t.Fatalf("marshal tampered checkpoint: %v", err)
			}
			_, _, err = RunPrefixResumable(openingRangeCheckpointRequest(t, 1800, "close"), raw)
			var invalid *PrefixCheckpointError
			if !errors.As(err, &invalid) {
				t.Fatalf("tampered checkpoint error = %#v, want typed rejection", err)
			}
		})
	}
	var stateTampered map[string]any
	if err := json.Unmarshal(checkpoint, &stateTampered); err != nil {
		t.Fatalf("decode checkpoint state: %v", err)
	}
	stateTampered["state"].(map[string]any)["realized"] = 12345.0
	raw, err := json.Marshal(stateTampered)
	if err != nil {
		t.Fatalf("marshal state-tampered checkpoint: %v", err)
	}
	_, _, err = RunPrefixResumable(openingRangeCheckpointRequest(t, 1800, "close"), raw)
	var invalid *PrefixCheckpointError
	if !errors.As(err, &invalid) || invalid.Reason != "state digest mismatch" {
		t.Fatalf("state-tampered checkpoint error = %#v, want state digest rejection", err)
	}
}

func TestRunPrefixResumableRejectsEveryUnsupportedPath(t *testing.T) {
	request := openingRangeCheckpointRequest(t, 36, "close")
	tests := []struct {
		name string
		edit func(*RunRequest)
		path string
	}{
		{name: "ordinary family", edit: func(r *RunRequest) { r.Config["setupType"] = string(dsl.FamilyFailedBreakout) }, path: "ordinary family failedBreakout"},
		{name: "special family", edit: func(r *RunRequest) { r.Config["setupType"] = string(dsl.FamilyDailyFlushFailure) }, path: "special family dailyFlushFailure"},
		{name: "execution window", edit: func(r *RunRequest) { r.ExecutionWindow = &ExecutionWindow{} }, path: "bounded execution window"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate := request
			candidate.Config = make(dsl.Config, len(request.Config))
			for key, value := range request.Config {
				candidate.Config[key] = value
			}
			tc.edit(&candidate)
			_, _, err := RunPrefixResumable(candidate, nil)
			var unsupported *PrefixCheckpointUnsupportedError
			if !errors.As(err, &unsupported) || unsupported.Path != tc.path {
				t.Fatalf("unsupported error = %#v, want %q", err, tc.path)
			}
		})
	}
}
