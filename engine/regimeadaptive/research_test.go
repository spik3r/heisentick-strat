package regimeadaptive

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"testing"
)

func TestResearchCausalityAndSeparateBridge(t *testing.T) {
	bars := make([]Bar, 120)
	for n := range bars {
		c := 100 + float64(n%13)*.1 + float64(n)*.01
		bars[n] = Bar{T: int64(n) * 1800000, CloseT: int64(n+1) * 1800000, Open: c, High: c + 1, Low: c - 1, Close: c + .05}
	}
	full, e := ResearchRows(bars, false)
	if e != nil {
		t.Fatal(e)
	}
	raw, marshalErr := json.Marshal(full)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	t.Logf("RESEARCH_ROWS_SHA256=%x", sha256.Sum256(raw))
	prefix, e := ResearchRows(bars[:90], false)
	if e != nil || !reflect.DeepEqual(full[:90], prefix) {
		t.Fatal("lookahead", e)
	}
	for _, r := range full {
		if r.Signals.ScaleLong || r.Signals.ScaleShort {
			t.Fatal("scale trigger in research")
		}
	}
	wickOff, e := ResearchRows(bars, true)
	if e != nil {
		t.Fatal(e)
	}
	for n, r := range wickOff {
		if r.Signals.InitialLong != full[n].Signals.InitialLong || r.Signals.InitialShort != full[n].Signals.InitialShort {
			t.Fatal("B2 altered trend")
		}
	}
	// Existing inspector still refuses historical labels despite the explicit bridge.
	if _, e := Inspect(Request{Schema: Schema, Mode: "indicators-signals-only", DataKind: "historical", Profile: FlatProfile, SourceSHA256: FlatHash, FixtureID: "test", Timeframe: Timeframe{Unit: "minute", Multiplier: 30}, Bars: bars}); e == nil {
		t.Fatal("inspector admission weakened")
	}
}
