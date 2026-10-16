package engine

import "math"

// donchianChannel returns the highest high and lowest low of the n bars
// before i. Bar i is not part of its own channel.
func donchianChannel(high, low []float64, i int, n int) (float64, float64, bool) {
	if n <= 0 || i < n || i >= len(high) || i >= len(low) {
		return 0, 0, false
	}
	upper := math.Inf(-1)
	lower := math.Inf(1)
	for j := i - n; j < i; j++ {
		if !isFinite(high[j]) || !isFinite(low[j]) {
			return 0, 0, false
		}
		upper = math.Max(upper, high[j])
		lower = math.Min(lower, low[j])
	}
	return upper, lower, true
}

// donchianLastSignal returns the signal bar of the most recent accepted
// entry. The broker's per-family fields live in a fingerprinted file, so the
// family keeps no state of its own: every trade carries its signal bar.
func (b *broker) donchianLastSignal() (int, bool) {
	if len(b.trades) == 0 {
		return 0, false
	}
	signal, ok := numericMetaValue(b.trades[len(b.trades)-1].Meta["signalIndex"])
	return int(signal), ok
}

type donchianSignal struct {
	side   side
	upper  float64
	lower  float64
	ema    float64
	hasEMA bool
}

func (b *broker) donchianBreakoutSignal(i int) (donchianSignal, bool) {
	p := b.params.DonchianBreakout
	var signal donchianSignal
	if p.TrendEMALen > 0 {
		// computeSetupEMA fills the broker's EMA column with
		// computeEMA(close, TrendEMALen): seeded with the first close,
		// alpha = 2 / (N + 1).
		if i >= len(b.ema) || !isFinite(b.ema[i]) {
			return signal, false
		}
		signal.ema = b.ema[i]
		signal.hasEMA = true
	}
	warmup := p.ChannelBars
	if p.TrendEMALen > warmup {
		warmup = p.TrendEMALen
	}
	if i < warmup {
		return signal, false
	}
	upper, lower, ok := donchianChannel(b.series.H, b.series.L, i, p.ChannelBars)
	if !ok {
		return signal, false
	}
	signal.upper, signal.lower = upper, lower
	c := b.series.C[i]
	if c > upper && (!signal.hasEMA || c > signal.ema) {
		signal.side = sideLong
		return signal, true
	}
	if c < lower && (!signal.hasEMA || c < signal.ema) {
		signal.side = sideShort
		return signal, true
	}
	return signal, false
}

func (b *broker) onDonchianBreakoutBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}
	p := b.params
	if last, ok := b.donchianLastSignal(); ok && i-last < p.CooldownBars {
		return
	}
	signal, ok := b.donchianBreakoutSignal(i)
	if !ok {
		return
	}
	s := signal.side
	if s == sideLong && !p.AllowLong || s == sideShort && !p.AllowShort {
		return
	}
	if !b.htfAllows(i, s) {
		return
	}
	atr := finiteOrZero(b.cols.ATR[i])
	stopATR := p.DonchianBreakout.StopATR
	if atr <= 0 || !(stopATR > 0) {
		return
	}
	entry := b.series.C[i]
	sign := float64(s)
	stop := entry - sign*stopATR*atr
	meta := TradeMeta{
		"setup":       "donchianBreakout",
		"side":        s.String(),
		"channelBars": float64(p.DonchianBreakout.ChannelBars),
		"channelHigh": signal.upper,
		"channelLow":  signal.lower,
		"signalIndex": float64(i),
	}
	if signal.hasEMA {
		meta["trendEma"] = signal.ema
	}
	setup := setupPlan{Side: s, Stop: stop, Tag: "DSL-DCB", Meta: gradeMeta(meta), NoTarget: true}
	if p.TargetR > 0 {
		setup.Target = entry + sign*stopATR*atr*p.TargetR
		setup.NoTarget = false
	}
	b.enterSetup(i, setup)
}
