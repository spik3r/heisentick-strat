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

func TestActiveWickOnlyCloseCrossAndTrendSignals(t *testing.T) {
	previous := Row{RSI: ptr(45), RSIMA: ptr(50), BBLower: ptr(90), BBUpper: ptr(110)}
	base := Row{Regime: 3, CHOP: ptr(40), ATR: ptr(1), RSI: ptr(45), RSIMA: ptr(50), PSAR: ptr(95), BBBasis: ptr(100), BBLower: ptr(90), BBUpper: ptr(110)}
	cases := []struct {
		name                                       string
		prevClose, close, low, high                float64
		wickLong, wickShort, crossLong, crossShort bool
	}{
		{"wick-long-only", 95, 95, 89, 96, true, false, false, false}, {"wick-short-only", 105, 105, 104, 111, false, true, false, false},
		{"close-cross-long", 95, 89, 88, 96, true, false, true, false}, {"close-cross-short", 105, 111, 104, 112, false, true, false, true},
		{"both-wicks", 100, 100, 89, 111, true, true, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prior := Bar{Close: c.prevClose}
			bar := Bar{Close: c.close, Low: c.low, High: c.high}
			current := base
			current.PSAR = ptr(c.close)
			current.Regime = classify(current.CHOP, current.RSI, current.PSAR, c.close, true)
			if current.Regime != 3 {
				t.Fatal("range fixture not classified range")
			}
			on := signals(current, &previous, bar, &prior, true, FlatProfile)
			off := signals(current, &previous, bar, &prior, false, FlatProfile)
			if on.RangeLong != c.wickLong || on.RangeShort != c.wickShort || off.RangeLong != c.crossLong || off.RangeShort != c.crossShort {
				t.Fatalf("active signal mismatch on%+v off%+v", on, off)
			}
			if c.wickLong && c.wickShort {
				if on.FlatEligible || !on.BothBands {
					t.Fatal("both band eligibility")
				}
			} else if !on.FlatEligible {
				t.Fatal("fixture not active eligible")
			}
		})
	}
	trend := base
	trend.Regime = 1
	trend.RSI = ptr(55)
	bar := Bar{Close: 100, Low: 99, High: 101}
	prior := Bar{Close: 100}
	on := signals(trend, &previous, bar, &prior, true, FlatProfile)
	off := signals(trend, &previous, bar, &prior, false, FlatProfile)
	if !on.InitialLong || !off.InitialLong || on.ScaleLong || off.ScaleLong {
		t.Fatal("active trend changed by wick setting")
	}
}
