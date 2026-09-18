package engine

import (
	"fmt"
	"math"
)

type fvgZone struct {
	Key                 string
	Side                side
	Low                 float64
	High                float64
	CreatedAt           int
	Used                bool
	GapATR              float64
	DisplacementBodyATR float64
	GapWidth            float64
	FormationATR        float64
	DisplacementBody    float64
}

func (b *broker) onFairValueGapBar(i int) {
	if b.hasPosition {
		b.applyPartialManagement(i)
		b.moveStopToBreakeven(i)
		b.exitAfterBars(i)
		return
	}

	b.detectFairValueGap(i)
	b.pruneFairValueGaps(i)
	p := b.params
	if b.hasFVGEntry && i-b.fvgLastEntry < p.CooldownBars {
		return
	}
	if !inSetupTradeWindow(b.series.T[i], p, 0) {
		return
	}
	for zoneIndex := len(b.fvgZones) - 1; zoneIndex >= 0; zoneIndex-- {
		zone := &b.fvgZones[zoneIndex]
		if zone.Used || i <= zone.CreatedAt {
			continue
		}
		setup, reference, ok := b.fairValueGapSetup(i, zone)
		if !ok {
			continue
		}
		// FVG midpoint limits use their declared entry window rather than the
		// generic swept-edge expiry.
		entryExpireBars := b.params.FairValueGap.EntryExpireCandles
		if entryExpireBars < 1 {
			entryExpireBars = 1
		}
		if b.enterLimit(i, reference, setup, entryExpireBars) {
			zone.Used = true
			b.fvgLastEntry = i
			b.hasFVGEntry = true
		}
		return
	}
}

// fvgSwept reports whether, per the family sweep gate, a swing pivot of the
// opposite side was swept before the displacement candle at d. Left-side
// pivot comparisons are strict; right-side comparisons allow equality so
// pooled equal lows/highs form one liquidity pool. The pivot's right flank
// must complete at or before d, and the sweep must trade strictly through the
// pivot extreme. Mirrors engine/dsl/setups/fairValueGap.js.
func fvgSwept(b *broker, s side, d, lookback, width int) bool {
	if width < 1 {
		width = 2
	}
	if lookback < 1 {
		lookback = 30
	}
	start := width
	if from := d - lookback + 1; from > start {
		start = from
	}
	for j := start; j <= d-width; j++ {
		isPivot := true
		for n := 1; n <= width; n++ {
			if s == sideLong {
				if !(b.series.L[j] < b.series.L[j-n]) || !(b.series.L[j] <= b.series.L[j+n]) {
					isPivot = false
					break
				}
			} else if !(b.series.H[j] > b.series.H[j-n]) || !(b.series.H[j] >= b.series.H[j+n]) {
				isPivot = false
				break
			}
		}
		if !isPivot {
			continue
		}
		for m := j + width; m <= d; m++ {
			if s == sideLong {
				if b.series.L[m] < b.series.L[j] {
					return true
				}
			} else if b.series.H[m] > b.series.H[j] {
				return true
			}
		}
	}
	return false
}

func (b *broker) detectFairValueGap(i int) {
	if i < 2 {
		return
	}
	p := b.params.FairValueGap
	atr := finiteOrZero(b.cols.ATR[i])
	if atr <= 0 || !isFinite(atr) || p.MinGapATR <= 0 || p.MinDisplacementATR <= 0 {
		return
	}
	var sideForGap side
	var low, high float64
	switch {
	case b.series.H[i-2] < b.series.L[i]:
		sideForGap = sideLong
		low, high = b.series.H[i-2], b.series.L[i]
	case b.series.L[i-2] > b.series.H[i]:
		sideForGap = sideShort
		low, high = b.series.H[i], b.series.L[i-2]
	default:
		return
	}
	if !(high > low) || high-low < atr*p.MinGapATR {
		return
	}
	displacementBody := math.Abs(b.series.C[i-1] - b.series.O[i-1])
	if !isFinite(displacementBody) || displacementBody < atr*p.MinDisplacementATR {
		return
	}
	if p.SweepRequired && i-1 < p.SweepPivotWidth {
		return
	}
	if p.SweepRequired && !fvgSwept(b, sideForGap, i-1, p.SweepLookback, p.SweepPivotWidth) {
		return
	}
	key := fmt.Sprintf("%s:%d", sideForGap.String(), i)
	for _, existing := range b.fvgZones {
		if existing.Key == key {
			return
		}
	}
	b.fvgZones = append(b.fvgZones, fvgZone{
		Key:                 key,
		Side:                sideForGap,
		Low:                 low,
		High:                high,
		CreatedAt:           i,
		GapATR:              (high - low) / atr,
		DisplacementBodyATR: displacementBody / atr,
		GapWidth:            high - low,
		FormationATR:        atr,
		DisplacementBody:    displacementBody,
	})
	if len(b.fvgZones) > 80 {
		b.fvgZones = b.fvgZones[len(b.fvgZones)-80:]
	}
}

func (b *broker) pruneFairValueGaps(i int) {
	maxAge := b.params.FairValueGap.RetestCandles
	if maxAge <= 0 {
		b.fvgZones = nil
		return
	}
	out := b.fvgZones[:0]
	for _, zone := range b.fvgZones {
		if zone.Used || i-zone.CreatedAt > maxAge {
			continue
		}
		if (zone.Side == sideLong && b.series.C[i] < zone.Low) ||
			(zone.Side == sideShort && b.series.C[i] > zone.High) {
			continue
		}
		out = append(out, zone)
	}
	b.fvgZones = out
}

func (b *broker) fairValueGapSetup(i int, zone *fvgZone) (setupPlan, float64, bool) {
	p := b.params.FairValueGap
	atr := finiteOrZero(b.cols.ATR[i])
	if atr <= 0 || !isFinite(atr) || i <= zone.CreatedAt || i-zone.CreatedAt > p.RetestCandles {
		return setupPlan{}, 0, false
	}
	s := zone.Side
	if s == sideLong && !b.params.AllowLong || s == sideShort && !b.params.AllowShort || !b.htfAllows(i, s) {
		return setupPlan{}, 0, false
	}
	reference := (zone.Low + zone.High) / 2
	if p.EntryReference == "proximalEdge" {
		if s == sideLong {
			reference = zone.High
		} else {
			reference = zone.Low
		}
	} else if p.EntryReference == "fib618" {
		if s == sideLong {
			reference = zone.Low + (zone.High-zone.Low)*0.618
		} else {
			reference = zone.High - (zone.High-zone.Low)*0.618
		}
	}
	if !isFinite(reference) || reference <= zone.Low || reference >= zone.High {
		if p.EntryReference != "proximalEdge" {
			return setupPlan{}, 0, false
		}
	}
	tradesIntoZone := b.series.L[i] <= zone.High && b.series.H[i] >= zone.Low
	if !tradesIntoZone {
		return setupPlan{}, 0, false
	}
	stop := zone.Low - atr*b.params.StopBufferATR
	if s == sideShort {
		stop = zone.High + atr*b.params.StopBufferATR
	}
	if !stopOK(reference, stop, atr, b.params.MinStopATR, b.params.MaxStopATR) {
		return setupPlan{}, 0, false
	}
	risk := math.Abs(reference - stop)
	target := reference + float64(s)*risk*b.params.TargetR
	return setupPlan{
		Side:   s,
		Stop:   stop,
		Target: target,
		Tag:    "DSL-FVG",
		Meta: gradeMeta(TradeMeta{
			"setup":            "fairValueGap",
			"side":             s.String(),
			"gapId":            zone.Key,
			"gapLower":         zone.Low,
			"gapUpper":         zone.High,
			"gapWidth":         zone.GapWidth,
			"gapAgeCandles":    i - zone.CreatedAt,
			"formationIndex":   zone.CreatedAt,
			"formationAtr":     zone.FormationATR,
			"displacementBody": zone.DisplacementBody,
			"entryReference":   p.EntryReference,
			"levelPrice":       reference,
			"setupAgeCandles":  i - zone.CreatedAt,
		}),
	}, reference, true
}
