package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeRunFixtureRejectsMalformedRowsWithoutShiftingTime(t *testing.T) {
	for _, name := range []string{"bars", "sourceBars", "htfBars", "sourceHtfBars"} {
		for _, row := range []string{"[1,2,3,4,5]", "[1,2,3,4,5,6,7]"} {
			t.Run(name+"/width="+fmt.Sprint(strings.Count(row, ",")+1), func(t *testing.T) {
				raw := fmt.Sprintf(`{"schema":"dsl-conformance-run-fixture-v1","%s":[[0,1,2,0,1,1],%s,[2,1,2,0,1,1]]}`, name, row)
				if _, err := DecodeRunFixture([]byte(raw)); err == nil || !strings.Contains(err.Error(), name+"[1]: expected six OHLCV values") {
					t.Fatalf("decode error = %v", err)
				}
				path := filepath.Join(t.TempDir(), "fixture.json")
				if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := LoadRunFixture(path); err == nil || !strings.Contains(err.Error(), name+"[1]: expected six OHLCV values") {
					t.Fatalf("file loader error = %v", err)
				}
			})
		}
	}
}

func TestDecodeRunFixtureNormalizesCostsAndKeepsAllRows(t *testing.T) {
	raw := []byte(`{"schema":"dsl-conformance-run-fixture-v1","bars":[[0,1,2,0,1,1],[1,2,3,1,2,2]]}`)
	fixture, err := DecodeRunFixture(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.Bars) != 2 || fixture.Bars[1].T != 1 || fixture.Bars[1].V != 2 {
		t.Fatalf("decoded bars = %+v", fixture.Bars)
	}
	if fixture.Costs.FillOn != "close" || fixture.Costs.StartEquity != 10000 {
		t.Fatalf("normalized costs = %+v", fixture.Costs)
	}
}
