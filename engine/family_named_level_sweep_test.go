package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/marketdata"
)

func TestNamedLevelSweepZeroReclaimRequiresStrictCloseThrough(t *testing.T) {
	rule := namedLevelSweepRule{Mode: "test", HasSweepAtr: true, SweepAtr: 0.35, ReclaimAtr: 0, Side: sideLong}
	bar := marketdata.Bar{O: 100.2, H: 101, L: 99.8, C: 100}
	b := broker{series: marketdata.SeriesFromBars([]marketdata.Bar{bar})}
	if b.namedLevelSweptOrTested(0, rule, 100, 1) {
		t.Fatal("close exactly at the level must not count as closing above it")
	}
	b.series.C[0] = 100.0001
	if !b.namedLevelSweptOrTested(0, rule, 100, 1) {
		t.Fatal("strict close above the level should pass")
	}
}

func TestNamedLevelSweepStrictHTFGateRequiresDirectionalAgreement(t *testing.T) {
	for _, tc := range []struct {
		name  string
		trend int8
		side  side
		want  bool
	}{
		{"up agrees long", trendUp, sideLong, true},
		{"up rejects short", trendUp, sideShort, false},
		{"down agrees short", trendDown, sideShort, true},
		{"down rejects long", trendDown, sideLong, false},
		{"flat rejects long", trendFlat, sideLong, false},
		{"unavailable rejects short", htfUnavailable, sideShort, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := broker{htfTrend: []int8{tc.trend}}
			if got := b.htfDirectionAgrees(0, tc.side); got != tc.want {
				t.Fatalf("htfDirectionAgrees = %v, want %v", got, tc.want)
			}
		})
	}
	b := broker{htfTrend: []int8{trendUp}}
	if b.htfDirectionAgrees(1, sideLong) {
		t.Fatal("missing projected HTF value must fail closed")
	}
}

func TestNamedLevelSweepStrictHTFModeIsOptIn(t *testing.T) {
	base := map[string]any{"namedLevelSweep": map[string]any{}}
	if got := namedLevelSweepParamsFromConfig(base).StrictHTF; got {
		t.Fatal("strict HTF mode must remain off by default")
	}
	strict := map[string]any{
		"namedLevelSweep": map[string]any{},
		"htf":             map[string]any{"mode": "strictAgree"},
	}
	if got := namedLevelSweepParamsFromConfig(strict).StrictHTF; !got {
		t.Fatal("strictAgree mode must enable the named-level strict gate")
	}
}

func TestNamedLevelSweepZeroReclaimRequiresStrictCloseBelow(t *testing.T) {
	rule := namedLevelSweepRule{Mode: "test", HasSweepAtr: true, SweepAtr: 0.35, ReclaimAtr: 0, Side: sideShort}
	bar := marketdata.Bar{O: 99.8, H: 100.2, L: 99, C: 100}
	b := broker{series: marketdata.SeriesFromBars([]marketdata.Bar{bar})}
	if b.namedLevelSweptOrTested(0, rule, 100, 1) {
		t.Fatal("close exactly at the level must not count as closing below it")
	}
	b.series.C[0] = 99.9999
	if !b.namedLevelSweptOrTested(0, rule, 100, 1) {
		t.Fatal("strict close below the level should pass")
	}
}
