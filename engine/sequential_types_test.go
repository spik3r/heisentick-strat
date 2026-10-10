package engine

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestSequentialFullAuditJSONAndRerunIsolation(t *testing.T) {
	spec := sequentialFullTestSpec("E1")
	series := marketdata.SeriesFromBars(sequentialFullE1Bars())
	fixture := RunFixture{Costs: Costs{FillOn: "open"}}
	trades, first, err := runSequentialFull(spec, series, fixture)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip SequentialFullAudit
	if err := json.Unmarshal(encoded, &roundTrip); err != nil || !reflect.DeepEqual(first, roundTrip) {
		t.Fatalf("typed audit roundtrip=%+v err=%v", roundTrip, err)
	}
	*first.Opportunities[0].Fill = -1
	first.Opportunities[0].EpisodeID = "changed by caller"
	trades[0].Meta["episodeId"] = "changed by caller"
	_, second, err := runSequentialFull(spec, series, fixture)
	if err != nil || !reflect.DeepEqual(second, roundTrip) {
		t.Fatalf("rerun leaked prior output mutation: %+v err=%v", second, err)
	}
	_, empty, err := runSequentialFull(spec, marketdata.SeriesFromBars(sequentialFullE1Bars()[:5]), fixture)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil || string(object["opportunities"]) != "[]" {
		t.Fatalf("empty opportunity list must be []: %s err=%v", data, err)
	}
}

func TestSequentialFullErrorIdentityJSON(t *testing.T) {
	err := &SequentialFullExecutionError{"unsupported-incomplete-terminal-run", "held position", 14, "episode:perf"}
	if err.Error() != "unsupported-incomplete-terminal-run: held position at bar 14" {
		t.Fatal(err)
	}
	data, e := json.Marshal(err)
	if e != nil || string(data) != `{"kind":"unsupported-incomplete-terminal-run","field":"held position","barIndex":14,"opportunityId":"episode:perf"}` {
		t.Fatalf("typed error=%s err=%v", data, e)
	}
}
