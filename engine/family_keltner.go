package engine

import "math"

// The EMA fold below mirrors JavaScript exactly, so it must round twice.
// The Go spec lets an implementation fuse `x*y + z*w` into FMA instructions
// that round once (this happens on arm64), while V8 — the reference this
// engine mirrors — never does. The spec also states that an explicit
// float64() conversion forces the intermediate to be rounded, which is what
// keeps the two engines bit-identical here. Do not remove those conversions.

// keltnerBar is a snapshot of one completed bar's band geometry, captured at
// the moment the channel was updated on that bar. Mirrors the per-bar
// records kept in state.window by engine/dsl/setups/keltnerReversion.js.
type keltnerBar struct {
	L, H, C      float64
	Lower, Upper float64
	ATR          float64
}

// keltnerState is the Keltner family's per-run broker state (both
// keltnerReversion and keltnerExpansion share it, including the cooldown
// entry marker, mirroring the JS setups' shared api.state.keltnerReversion
// and api.state.keltnerReversionLastEntry keys). Grouped into one struct so
// broker.go carries a single field and a single reset call.
type keltnerState struct {
	lastEntry int
	hasEntry  bool
	ema       float64
	hasEMA    bool
	bars      int
	window    []keltnerBar
}

func (s *keltnerState) reset() {
	*s = keltnerState{window: s.window[:0]}
}

// updateKeltnerChannel folds the current bar's close into the EMA, advances
// the bar counter, and appends this bar's band snapshot to the rolling
// window (trimmed to ReclaimCandles). It must run once per bar, before any
// position or entry logic, mirroring updateChannel() in the JS setup.
func (b *broker) updateKeltnerChannel(i int) (atr, mid, lower, upper float64) {
	p := b.params.Keltner
	close := b.series.C[i]
	alpha := 2 / (float64(p.EMALen) + 1)
	if !b.keltner.hasEMA {
		b.keltner.ema = close
		b.keltner.hasEMA = true
	} else {
		b.keltner.ema = float64(close*alpha) + float64(b.keltner.ema*(1-alpha))
	}
	b.keltner.bars++

	atr = finiteOrZero(b.cols.ATR[i])
	mid = b.keltner.ema
	band := atr * p.BandATR
	lower = mid - band
	upper = mid + band

	b.keltner.window = append(b.keltner.window, keltnerBar{
		L: b.series.L[i], H: b.series.H[i], C: close,
		Lower: lower, Upper: upper, ATR: atr,
	})
	if reclaim := p.ReclaimCandles; reclaim > 0 && len(b.keltner.window) > reclaim {
		b.keltner.window = b.keltner.window[len(b.keltner.window)-reclaim:]
	}
	return atr, mid, lower, upper
}

// findKeltnerStretch reports the furthest extreme among window bars that
// traded beyond their own band edge by at least minPierceATR times that
// bar's own ATR. Mirrors findStretch() in the JS setup: each bar is judged
// against the edge that existed on that bar, never the current one.
func findKeltnerStretch(window []keltnerBar, s side, minPierceATR float64) (float64, bool) {
	extreme := 0.0
	found := false
	for _, bar := range window {
		if !(bar.ATR > 0) {
			continue
		}
		pierce := minPierceATR * bar.ATR
		if s == sideLong && bar.L <= bar.Lower-pierce {
			if !found || bar.L < extreme {
				extreme = bar.L
			}
			found = true
		}
		if s == sideShort && bar.H >= bar.Upper+pierce {
			if !found || bar.H > extreme {
				extreme = bar.H
			}
			found = true
		}
	}
	return extreme, found
}

func (b *broker) keltnerCooldownActive(i int) bool {
	return b.keltner.hasEntry && i-b.keltner.lastEntry < b.params.CooldownBars
}

func (b *broker) buildKeltnerReversionSetup(i int, atr, mid, lower, upper float64, s side) (setupPlan, bool) {
	p := b.params.Keltner
	if s == sideLong && !b.params.AllowLong {
		return setupPlan{}, false
	}
	if s == sideShort && !b.params.AllowShort {
		return setupPlan{}, false
	}
	if !(atr > 0) {
		return setupPlan{}, false
	}
	entry := b.series.C[i]

	if s == sideLong && !(entry > lower && entry < mid) {
		return setupPlan{}, false
	}
	if s == sideShort && !(entry < upper && entry > mid) {
		return setupPlan{}, false
	}

	stretchExtreme, found := findKeltnerStretch(b.keltner.window, s, p.MinPierceATR)
	if !found {
		return setupPlan{}, false
	}
	if !b.htfAllows(i, s) {
		return setupPlan{}, false
	}

	sign := float64(s)
	stop := stretchExtreme - sign*atr*b.params.StopBufferATR
	if !stopOK(entry, stop, atr, b.params.MinStopATR, b.params.MaxStopATR) {
		return setupPlan{}, false
	}

	risk := math.Abs(entry - stop)
	target := entry + sign*risk*b.params.TargetR
	targetSource := "fixedR"
	if p.TargetMode == "midlineElseFixedR" {
		midlineReward := sign * (mid - entry)
		if midlineReward >= risk*p.MinR {
			target = mid
			targetSource = "midline"
		}
	}

	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: target,
		Tag:    "DSL-KR",
		Meta: gradeMeta(TradeMeta{
			"setup":          "keltnerReversion",
			"side":           s.String(),
			"emaLen":         float64(p.EMALen),
			"bandAtr":        p.BandATR,
			"midline":        mid,
			"bandLower":      lower,
			"bandUpper":      upper,
			"stretchExtreme": stretchExtreme,
			"targetSource":   targetSource,
		}),
	}, true
}

// keltnerParamsFrom builds keltnerParams from the parsed "keltnerReversion"
// and "target" config maps. Colocated with the Keltner family's runtime
// rather than left inline in config.go's builder.
func keltnerParamsFrom(keltner, target map[string]any) keltnerParams {
	return keltnerParams{
		EMALen:         intValue(keltner, "emaLen", 20),
		BandATR:        numberValue(keltner, "bandAtr", 2),
		MinPierceATR:   numberValue(keltner, "minPierceAtr", 0.25),
		ReclaimCandles: intValue(keltner, "reclaimCandles", 3),
		TargetMode:     stringValue(keltner, "targetMode", "midlineElseFixedR"),
		MinR:           numberValue(target, "minR", 0.5),
	}
}

// onKeltnerBar dispatches to the keltnerReversion or keltnerExpansion bar
// handler based on the strategy's setup type, letting broker.go's main
// switch carry a single combined case for both Keltner family members.
func (b *broker) onKeltnerBar(i int, setupType string) {
	if setupType == "keltnerExpansion" {
		b.onKeltnerExpansionBar(i)
		return
	}
	b.onKeltnerReversionBar(i)
}

func (b *broker) onKeltnerReversionBar(i int) {
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

	setup, ok := b.buildKeltnerReversionSetup(i, atr, mid, lower, upper, sideLong)
	if !ok {
		setup, ok = b.buildKeltnerReversionSetup(i, atr, mid, lower, upper, sideShort)
	}
	if !ok {
		return
	}
	if b.enterSetup(i, setup) {
		b.keltner.lastEntry = i
		b.keltner.hasEntry = true
	}
}
