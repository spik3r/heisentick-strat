package engine

import "github.com/spik3r/heisentick-strat/dsl"

func (b *broker) applyPreHandlerTrail(i int) {
	if !b.hasPosition {
		return
	}
	if b.params.SetupType == string(dsl.FamilyElderTripleScreen) {
		b.applyTrailWith(i, trailParams{ATR: b.params.ElderTripleScreen.TrailATR, TriggerR: 1})
		return
	}
	if usesGenericTrail(b.params.SetupType) {
		b.applyTrail(i)
	}
}

func (b *broker) fillPendingExits(i int) {
	for k := len(b.pendingExits) - 1; k >= 0; k-- {
		pending := b.pendingExits[k]
		if pending.Index > i {
			continue
		}
		b.pendingExits = append(b.pendingExits[:k], b.pendingExits[k+1:]...)
		if b.hasPosition && b.position.EntryIndex == pending.PositionEntryIndex {
			b.closePosition(b.series.O[i], i, pending.Reason)
		}
	}
}

func (b *broker) resolveIntrabarExit(i int) {
	if !b.hasPosition {
		return
	}
	pos := &b.position
	// A carried stop that the bar opens beyond fills at the open: the stop
	// level never traded. GapAwareStop families apply this on the entry bar too.
	gapFill := pos.GapAwareStop || pos.EntryIndex < i
	if !pos.NoStop && gapFill && ((pos.Side == sideLong && b.series.O[i] <= pos.SL) || (pos.Side == sideShort && b.series.O[i] >= pos.SL)) {
		b.closePosition(b.series.O[i], i, "sl")
		return
	}
	hitSL := !pos.NoStop && ((pos.Side == sideLong && b.series.L[i] <= pos.SL) ||
		(pos.Side == sideShort && b.series.H[i] >= pos.SL))
	hitTP := !pos.NoTarget && ((pos.Side == sideLong && b.series.H[i] >= pos.TP) ||
		(pos.Side == sideShort && b.series.L[i] <= pos.TP))
	if hitSL {
		b.closePosition(pos.SL, i, "sl")
		return
	}
	if hitTP {
		b.closePosition(pos.TP, i, "tp")
	}
}
