package engine

import (
	"math"

	"github.com/spik3r/heisentick-strat/marketdata"
)

type smaGoldenCrossState struct {
	fast, slow       float64
	hasFast, hasSlow bool
	atr              float64
	atrReady         bool
	atrCount         int
	atrSeedSum       float64
}

func simpleMovingAverage(series marketdata.Series, end, length int) (float64, bool) {
	if length <= 0 || end+1 < length {
		return 0, false
	}
	sum := 0.0
	for i := end - length + 1; i <= end; i++ {
		sum += series.C[i]
	}
	return sum / float64(length), true
}

func smaBullishCross(previousFast, previousSlow, fast, slow float64) bool {
	return previousFast <= previousSlow && fast > slow
}

func smaBearishCross(previousFast, previousSlow, fast, slow float64) bool {
	return previousFast >= previousSlow && fast < slow
}

func smaProtectedDistances(signalATR, stopATR, targetATR float64) (float64, float64, bool) {
	stopDistance := stopATR * signalATR
	targetDistance := targetATR * signalATR
	if math.IsNaN(stopDistance) || math.IsInf(stopDistance, 0) || stopDistance <= 0 ||
		math.IsNaN(targetDistance) || math.IsInf(targetDistance, 0) || targetDistance <= 0 {
		return 0, 0, false
	}
	return stopDistance, targetDistance, true
}

func (b *broker) updateSMAGoldenCrossATR(i, length int) (float64, bool) {
	if length <= 0 {
		return 0, false
	}
	trueRange := b.series.H[i] - b.series.L[i]
	if i > 0 {
		previousClose := b.series.C[i-1]
		trueRange = math.Max(trueRange, math.Max(math.Abs(b.series.H[i]-previousClose), math.Abs(b.series.L[i]-previousClose)))
	}
	b.sma.atrCount++
	if !b.sma.atrReady {
		b.sma.atrSeedSum += trueRange
		if b.sma.atrCount == length {
			b.sma.atr = b.sma.atrSeedSum / float64(length)
			b.sma.atrReady = true
		}
		return b.sma.atr, b.sma.atrReady
	}
	b.sma.atr = (b.sma.atr*float64(length-1) + trueRange) / float64(length)
	return b.sma.atr, true
}

func (b *broker) onSMAGoldenCrossBar(i int) {
	p := b.params.SMAGoldenCross
	protected := p.protected()
	var signalATR float64
	var atrReady bool
	if protected {
		signalATR, atrReady = b.updateSMAGoldenCrossATR(i, p.ATRLen)
	}
	previousFast, previousSlow := b.sma.fast, b.sma.slow
	previousFastReady, previousSlowReady := b.sma.hasFast, b.sma.hasSlow
	fast, fastReady := simpleMovingAverage(b.series, i, p.FastLen)
	slow, slowReady := simpleMovingAverage(b.series, i, p.SlowLen)

	b.sma.fast, b.sma.hasFast = fast, fastReady
	b.sma.slow, b.sma.hasSlow = slow, slowReady
	if !fastReady || !slowReady || !previousFastReady || !previousSlowReady {
		return
	}
	if protected && !atrReady {
		return
	}

	if b.hasPosition {
		if smaBearishCross(previousFast, previousSlow, fast, slow) {
			b.pendingExits = append(b.pendingExits, pendingExit{
				PositionEntryIndex: b.position.EntryIndex,
				Index:              i + 1,
				Reason:             "sma-bearish-cross",
			})
		}
		return
	}
	if !p.AllowLong || !smaBullishCross(previousFast, previousSlow, fast, slow) {
		return
	}

	order := order{
		Side:     sideLong,
		Size:     1,
		HasSize:  true,
		NoStop:   true,
		NoTarget: true,
		Tag:      "DSL-SMA-GC",
		Index:    i + 1,
		Meta: TradeMeta{
			"setup":       "smaGoldenCross",
			"signalIndex": float64(i),
			"fastSma":     fast,
			"slowSma":     slow,
		},
	}
	if protected {
		stopDistance, targetDistance, valid := smaProtectedDistances(signalATR, p.StopATR, p.TargetATR)
		if !valid {
			return
		}
		order.StopDistance = stopDistance
		order.HasStopDistance = true
		order.TargetDistance = targetDistance
		order.HasTargetDistance = true
		order.GapAwareStop = true
		order.NoStop = false
		order.NoTarget = false
		order.Meta["signalAtr"] = signalATR
		order.Meta["forwardBracketAnchor"] = "entry-fill"
		order.Meta["forwardStopDistance"] = order.StopDistance
		order.Meta["forwardTargetDistance"] = order.TargetDistance
	}
	if b.captureEntries {
		b.capturedEntries = append(b.capturedEntries, order)
		return
	}
	b.pendingOrders = append(b.pendingOrders, order)
}
