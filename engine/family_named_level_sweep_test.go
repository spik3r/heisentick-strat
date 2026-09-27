package engine

import (
	"testing"

	"github.com/spik3r/heisentick-strat/contextcols"
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

func TestNamedLevelSweepHonorsInclusiveSharedStopBounds(t *testing.T) {
	params := namedLevelSweepParamsFromConfig(map[string]any{
		"stop": map[string]any{"minAtr": 0.5, "maxAtr": 5.5},
	})
	if params.MinStopATR != 0.5 || params.MaxStopATR != 5.5 {
		t.Fatalf("stop bounds = [%v, %v], want [0.5, 5.5]", params.MinStopATR, params.MaxStopATR)
	}
	rule := namedLevelSweepRule{Level: "PDL", Mode: "test", HasSweepAtr: true, SweepAtr: 0.35, Side: sideLong}
	setupAccepted := func(low float64) bool {
		bar := marketdata.Bar{O: 100.2, H: 101, L: low, C: 100.5}
		b := broker{
			series: marketdata.SeriesFromBars([]marketdata.Bar{bar}),
			cols:   contextcols.Columns{ATR: []float64{1}, PriorDayL: []float64{100}},
			params: flagParams{
				TargetR:         4,
				NamedLevelSweep: params,
			},
		}
		_, ok := b.namedLevelSweepSetup(0, rule, 1)
		return ok
	}
	if !setupAccepted(100.1) {
		t.Fatal("stop exactly at minimum bound should pass")
	}
	params.MinStopATR = 0.500001
	if setupAccepted(100.1) {
		t.Fatal("stop below minimum bound should fail")
	}
	params.MinStopATR = 0.5
	if !setupAccepted(95) {
		t.Fatal("stop exactly at maximum bound should pass")
	}
	params.MaxStopATR = 5.499999
	if setupAccepted(95) {
		t.Fatal("stop above maximum bound should fail")
	}
}

func TestNamedLevelSweepDailyDedupUsesResolvedLevelPrice(t *testing.T) {
	first := namedLevelSweepSeenKey(sideLong, "PDL", 100)
	if repeated := namedLevelSweepSeenKey(sideLong, "PDL", 100.02); repeated != first {
		t.Fatalf("same rounded level price should deduplicate: %q != %q", repeated, first)
	}
	changed := namedLevelSweepSeenKey(sideLong, "PDL", 100.2)
	if changed == first {
		t.Fatalf("changed resolved level price should be admitted: %q", changed)
	}
	if oppositeSide := namedLevelSweepSeenKey(sideShort, "PDL", 100); oppositeSide == first {
		t.Fatal("opposite side must have an independent dedup key")
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
	legacy := map[string]any{
		"namedLevelSweep": map[string]any{},
		"htf":             map[string]any{"mode": "strictLegacyAgree"},
	}
	if got := namedLevelSweepParamsFromConfig(legacy).StrictHTF; !got {
		t.Fatal("strictLegacyAgree mode must enable the named-level strict gate")
	}
	if got := namedLevelSweepParamsFromConfig(legacy).DedupDaily; !got {
		t.Fatal("daily deduplication must remain enabled by default")
	}
	noDedup := map[string]any{"namedLevelSweep": map[string]any{"dedupDaily": false}}
	if got := namedLevelSweepParamsFromConfig(noDedup).DedupDaily; got {
		t.Fatal("explicit dedupDaily false must permit repeat sweeps")
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
