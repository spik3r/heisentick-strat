package engine

import "math"

func priceMomentumDirection(closes []float64, i int, lookback int, thresholdPct float64) (side, float64, bool) {
	if lookback <= 0 || thresholdPct <= 0 || !isFinite(thresholdPct) || i < lookback || i >= len(closes) {
		return 0, 0, false
	}
	current := closes[i]
	prior := closes[i-lookback]
	if !isFinite(current) || !isFinite(prior) || prior <= 0 {
		return 0, 0, false
	}
	rocPct := 100 * (current/prior - 1)
	if !isFinite(rocPct) {
		return 0, 0, false
	}
	if rocPct >= thresholdPct {
		return sideLong, rocPct, true
	}
	if rocPct <= -thresholdPct {
		return sideShort, rocPct, true
	}
	return 0, rocPct, true
}

func (b *broker) onPriceMomentumBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if b.hasPriceMomentumEntry && i-b.priceMomentumLastEntry < p.CooldownBars {
		return
	}
	s, rocPct, valid := priceMomentumDirection(
		b.series.C,
		i,
		p.PriceMomentum.LookbackBars,
		p.PriceMomentum.ThresholdPct,
	)
	if !valid || s == 0 {
		return
	}
	if s == sideLong && !p.AllowLong || s == sideShort && !p.AllowShort {
		return
	}
	if !b.htfAllows(i, s) {
		return
	}
	atr := finiteOrZero(b.cols.ATR[i])
	if atr <= 0 {
		return
	}
	entry := b.series.C[i]
	stop := recentExtreme(b.series, i, p.StopLookbackCandles, s)
	if s == sideLong {
		stop -= atr * p.StopBufferATR
	} else {
		stop += atr * p.StopBufferATR
	}
	if !stopOK(entry, stop, atr, p.MinStopATR, p.MaxStopATR) {
		return
	}
	risk := math.Abs(entry - stop)
	setup := setupPlan{
		Side:   s,
		Stop:   stop,
		Target: entry + float64(s)*risk*p.TargetR,
		Tag:    "DSL-PM",
		Meta: gradeMeta(TradeMeta{
			"setup":        "priceMomentum",
			"side":         s.String(),
			"lookbackBars": float64(p.PriceMomentum.LookbackBars),
			"thresholdPct": p.PriceMomentum.ThresholdPct,
			"rocPct":       rocPct,
		}),
	}
	if b.enterSetup(i, setup) {
		b.priceMomentumLastEntry = i
		b.hasPriceMomentumEntry = true
	}
}
