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
