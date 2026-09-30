package engine

import (
	"math"
	"os"
	"testing"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/marketdata"
)

func shockFixture(t *testing.T, name string) (RunFixture, string) {
	t.Helper()
	f, err := LoadRunFixture("../conformance/run/family-down-shock-" + name + ".fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../conformance/run/family-down-shock-" + name + ".strat")
	if err != nil {
		t.Fatal(err)
	}
	return f, string(source)
}
func TestDownShockNormalizerAndPrefix(t *testing.T) {
	f, _ := shockFixture(t, "immediate-time")
	full := downShockSignals(marketdata.SeriesFromBars(f.SourceBars))
	if len(full) != 1 {
		t.Fatalf("signals: %v", full)
	}
	signal := full[0]
	// Values independently checked against Polars bar_features and SMA ATR14.
	if math.Abs(signal.RZ-(-15.12249246216712)) > 1e-10 || math.Abs(signal.ATR-0.3428571428571325) > 1e-10 {
		t.Fatalf("normalizers: %+v", signal)
	}
	for _, n := range []int{459, 460, 900, len(f.SourceBars) - 1} {
		if len(downShockSignals(marketdata.SeriesFromBars(f.SourceBars[:n]))) != 0 {
			t.Fatalf("future signal leaked into prefix %d", n)
		}
	}
	f.SourceBars = append(f.SourceBars, marketdata.Bar{T: signal.KnownAt, O: 1, H: 10000, L: 0.1, C: 1, V: 1})
	later := downShockSignals(marketdata.SeriesFromBars(f.SourceBars))
	if later[0] != signal {
		t.Fatal("future candle changed earlier signal")
	}
}
func TestDownShockExecutionEdges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*RunFixture)
		want   int
		exit   string
	}{
		{"source close boundary", func(f *RunFixture) { f.Bars = f.Bars[1:] }, 1, "tp"},
		{"entry gap", func(f *RunFixture) {
			f.Bars = f.Bars[1:]
			for i := range f.Bars {
				f.Bars[i].T += shockGap + shockMinute
			}
		}, 0, ""},
		{"stop target tie", func(f *RunFixture) { f.Bars = f.Bars[:2]; f.Bars[1].H = 110; f.Bars[1].L = 90 }, 1, "sl"},
		{"holding gap", func(f *RunFixture) {
			f.Bars = f.Bars[:3]
			f.Bars[1].H = 98.21
			f.Bars[2].T += shockGap
			f.Bars[2].O = 97
		}, 1, ReasonRule},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, text := shockFixture(t, "immediate-atr")
			tc.mutate(&f)
			out, err := RunFixtureCase(f, text)
			if err != nil {
				t.Fatal(err)
			}
			if out.TradeCount != tc.want {
				t.Fatalf("count=%d", out.TradeCount)
			}
			if tc.want > 0 && out.Trades[0].Reason != tc.exit {
				t.Fatalf("exit=%v", out.Trades[0])
			}
		})
	}
}
func TestDownShockReversalEntryGap(t *testing.T) {
	f, text := shockFixture(t, "reversal-time")
	f.Bars = f.Bars[:3]
	f.Bars[2].T += shockMinute
	out, err := RunFixtureCase(f, text)
	if err != nil {
		t.Fatal(err)
	}
	if out.TradeCount != 0 {
		t.Fatal("reversal filled across missing minute")
	}
}
func TestDownShockRouteValidation(t *testing.T) {
	for _, source := range []string{"dsl v7\nsetup {\ntype: down shock rebound\n}", "dsl v7\nsetup {\ntype: down shock rebound\nsource timeframe 1h\nentryTf 5m\n}"} {
		p, err := dsl.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Errors) == 0 {
			t.Fatal("invalid route accepted")
		}
	}
}
