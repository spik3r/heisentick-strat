package engine

import "math"

// buildKeltnerExpansionSetup looks for a continuation hold: the previous bar
// closed beyond its own band edge with enough reach, and the current bar
// closes beyond the current edge too (the second-close confirmation).
// Mirrors buildExpansionSetup() in the JS setup.
func (b *broker) buildKeltnerExpansionSetup(i int, atr, mid, lower, upper float64, s side) (setupPlan, bool) {
	p := b.params.Keltner
	if s == sideLong && !b.params.AllowLong {
		return setupPlan{}, false
	}
	if s == sideShort && !b.params.AllowShort {
		return setupPlan{}, false
	}
	if !(atr > 0) || len(b.keltner.window) < 2 {
		return setupPlan{}, false
	}
	current := b.keltner.window[len(b.keltner.window)-1]
	previous := b.keltner.window[len(b.keltner.window)-2]
	if !(previous.ATR > 0) {
		return setupPlan{}, false
	}

	pierce := p.MinPierceATR * previous.ATR
	if s == sideLong && !(previous.C > previous.Upper && previous.H >= previous.Upper+pierce && current.C > current.Upper) {
		return setupPlan{}, false
	}
	if s == sideShort && !(previous.C < previous.Lower && previous.L <= previous.Lower-pierce && current.C < current.Lower) {
		return setupPlan{}, false
	}
	if !b.htfAllows(i, s) {
		return setupPlan{}, false
	}

	entry := b.series.C[i]
	edge := lower
	if s == sideLong {
		edge = upper
	}
	sign := float64(s)
	stop := edge - sign*atr*b.params.StopBufferATR
	if !stopOK(entry, stop, atr, b.params.MinStopATR, b.params.MaxStopATR) {
		return setupPlan{}, false
	}

	risk := math.Abs(entry - stop)
	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: entry + sign*risk*b.params.TargetR,
		Tag:    "DSL-KX",
		Meta: gradeMeta(TradeMeta{
			"setup":    "keltnerExpansion",
			"side":     s.String(),
			"emaLen":   float64(p.EMALen),
			"bandAtr":  p.BandATR,
			"bandEdge": edge,
			"midline":  mid,
		}),
	}, true
}

func (b *broker) onKeltnerExpansionBar(i int) {
	atr, mid, lower, upper := b.updateKeltnerChannel(i)
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	if b.keltner.bars <= b.params.Keltner.EMALen*2 {
		return
	}
	if b.keltnerCooldownActive(i) {
		return
	}

	setup, ok := b.buildKeltnerExpansionSetup(i, atr, mid, lower, upper, sideLong)
	if !ok {
		setup, ok = b.buildKeltnerExpansionSetup(i, atr, mid, lower, upper, sideShort)
	}
	if !ok {
		return
	}
	if b.enterSetup(i, setup) {
		b.keltner.lastEntry = i
		b.keltner.hasEntry = true
	}
}
