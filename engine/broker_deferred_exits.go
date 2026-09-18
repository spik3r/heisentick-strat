package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/dsl"
)

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
	if !pos.NoStop && pos.GapAwareStop && ((pos.Side == sideLong && b.series.O[i] <= pos.SL) || (pos.Side == sideShort && b.series.O[i] >= pos.SL)) {
		b.closePosition(b.series.O[i], i, "sl")
		return
	}
	hitSL := !pos.NoStop && ((pos.Side == sideLong && b.series.L[i] <= pos.SL) ||
		(pos.Side == sideShort && b.series.H[i] >= pos.SL))
	hitTP := !pos.NoTarget && ((pos.Side == sideLong && b.series.H[i] >= pos.TP) ||
		(pos.Side == sideShort && b.series.L[i] <= pos.TP))
	if hitSL {
		fill := pos.SL
		if pos.GapAwareStop {
			if pos.Side == sideLong {
				fill = math.Min(b.series.O[i], pos.SL)
			} else {
				fill = math.Max(b.series.O[i], pos.SL)
			}
		}
		b.closePosition(fill, i, "sl")
		return
	}
	if hitTP {
		b.closePosition(pos.TP, i, "tp")
	}
}
