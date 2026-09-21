package engine

import "math"

func (b *broker) onDualEMAResumptionBar(i int) {
	p := b.params.DualEMA
	close := b.series.C[i]
	previousClose := close
	if b.dualInitialized {
		previousClose = b.dualPreviousClose
	}
	tr := math.Max(b.series.H[i]-b.series.L[i], math.Max(math.Abs(b.series.H[i]-previousClose), math.Abs(b.series.L[i]-previousClose)))
	previousFast := b.dualFast
	if !b.dualInitialized {
		b.dualFast, b.dualSlow, b.dualATR = close, close, tr
	} else {
		b.dualFast = close*2/float64(p.FastLen+1) + b.dualFast*(1-2/float64(p.FastLen+1))
		b.dualSlow = close*2/float64(p.SlowLen+1) + b.dualSlow*(1-2/float64(p.SlowLen+1))
		b.dualATR = (b.dualATR*float64(p.ATRLen-1) + tr) / float64(p.ATRLen)
	}
	b.dualSlowHistory = append(b.dualSlowHistory, b.dualSlow)
	if len(b.dualSlowHistory) > p.RiseBars+1 {
		b.dualSlowHistory = b.dualSlowHistory[1:]
	}
	oldClose := b.dualPreviousClose
	b.dualPreviousFast, b.dualPreviousClose, b.dualInitialized = b.dualFast, close, true

	if b.dualExitPending && !b.hasPosition {
		b.dualExitPending, b.dualHasPositionEntry = false, false
		return
	}
	if b.lastExitIndex == i {
		b.dualHasPositionEntry = false
		return
	}
	if b.hasPosition {
		if !b.dualHasPositionEntry || b.dualPositionEntry != b.position.EntryIndex {
			b.dualPositionEntry, b.dualHasPositionEntry, b.dualBestClose = b.position.EntryIndex, true, b.position.Entry
		}
		b.dualBestClose = math.Max(b.dualBestClose, close)
		b.tightenStop(b.dualBestClose - p.TrailATR*b.dualATR)
		if close < b.dualSlow && !b.dualExitPending {
			b.pendingExits = append(b.pendingExits, pendingExit{PositionEntryIndex: b.position.EntryIndex, Index: i + 1, Rule: "slow-ema"})
			b.dualExitPending = true
		}
		return
	}
	warmup := max(p.SlowLen, max(p.RiseBars, p.ATRLen)) + 1
	if i < warmup || len(b.dualSlowHistory) < p.RiseBars+1 {
		return
	}
	if b.dualFast > b.dualSlow && b.dualSlow > b.dualSlowHistory[0] && oldClose <= previousFast && close > b.dualFast {
		b.pendingOrders = append(b.pendingOrders, order{Side: sideLong, RiskUSD: b.params.RiskUSD, HasRisk: true, Tag: "DSL-DER", Index: i + 1, StopDistance: p.StopATR * b.dualATR, HasStopDistance: true, GapAwareStop: true, NoTarget: true})
		b.pendingOrders[len(b.pendingOrders)-1].Meta = TradeMeta{
			"setup": "dualEmaResumption", "signalIndex": i, "signalAtr": b.dualATR,
		}
	}
}
