package engine

import (
	"math"
	"strconv"
	"time"
)

func dailyFlushFailureDiagnosticNumber(value float64) float64 {
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(value, 'g', 12, 64), 64)
	if err != nil {
		return value
	}
	return rounded
}

func isWeekdayTimestamp(timestamp float64) bool {
	weekday := time.UnixMilli(int64(timestamp)).UTC().Weekday()
	return weekday >= time.Monday && weekday <= time.Friday
}

func (b *broker) runDailyFlushFailure() []Trade {
	end := b.executionEnd()
	if end >= b.series.Len() {
		end = b.series.Len() - 1
	}
	retained := make([]int, 0, b.series.Len())
	atr := make([]float64, 0, b.series.Len())
	for i, timestamp := range b.series.T {
		if !isWeekdayTimestamp(timestamp) {
			continue
		}
		previousClose := b.series.C[i]
		if len(retained) > 0 {
			previousClose = b.series.C[retained[len(retained)-1]]
		}
		trueRange := math.Max(b.series.H[i]-b.series.L[i], math.Max(
			math.Abs(b.series.H[i]-previousClose),
			math.Abs(b.series.L[i]-previousClose),
		))
		value := trueRange
		if len(atr) > 0 {
			value = (atr[len(atr)-1]*float64(b.params.DailyFlushFailure.ATRLength-1) + trueRange) /
				float64(b.params.DailyFlushFailure.ATRLength)
		}
		retained = append(retained, i)
		atr = append(atr, value)
	}

	hasEntryPending := false
	pendingStop := 0.0
	pendingATR := 0.0
	pendingFlushRangeATR := 0.0
	pendingFlushCloseLocation := 0.0
	exitPending := false
	survivedBars := 0
	start := b.executionStart()
	for retainedIndex, rawIndex := range retained {
		if rawIndex > end {
			break
		}
		// The retained ATR history above remains causal context, but a pending
		// setup formed before the tradable window must not enter on its first
		// bar. Reset execution state at the boundary while preserving that
		// indicator history.
		if rawIndex < start {
			hasEntryPending = false
			exitPending = false
			continue
		}
		exitedThisBar := false
		if exitPending && b.hasPosition {
			b.closePosition(b.series.O[rawIndex], rawIndex, "time")
			exitPending = false
			exitedThisBar = true
		}
		if hasEntryPending && !b.hasPosition {
			entry := b.series.O[rawIndex] + b.slippageAt(b.series.O[rawIndex])
			risk := entry - pendingStop
			riskATR := risk / pendingATR
			if risk > 0 && riskATR >= b.params.MinStopATR && riskATR <= b.params.MaxStopATR {
				b.openPosition(sideLong, b.series.O[rawIndex], order{
					Side: sideLong, SL: pendingStop, TP: math.MaxFloat64,
					RiskUSD: b.params.RiskUSD, HasRisk: true, Tag: "DSL-DFF",
					NoTarget: true,
					Meta: TradeMeta{
						"setup":               "dailyFlushFailure",
						"side":                "long",
						"signalRetainedIndex": float64(retainedIndex - 1),
						"entryRetainedIndex":  float64(retainedIndex),
						"flushRangeAtr":       pendingFlushRangeATR,
						"flushCloseLocation":  pendingFlushCloseLocation,
					},
				}, rawIndex)
				survivedBars = 0
			}
			hasEntryPending = false
		}
		if b.hasPosition {
			if b.series.L[rawIndex] <= b.position.SL {
				exit := math.Min(b.series.O[rawIndex], b.position.SL)
				b.closePosition(exit, rawIndex, "sl")
				exitPending = false
				exitedThisBar = true
			} else {
				survivedBars++
				if float64(survivedBars) >= b.params.MaxHoldBars {
					exitPending = true
				}
			}
		}
		if b.hasPosition || hasEntryPending || exitPending || exitedThisBar ||
			retainedIndex < b.params.DailyFlushFailure.ATRLength+2 || retainedIndex+1 >= len(retained) || !b.params.AllowLong {
			continue
		}
		flushRaw := retained[retainedIndex-1]
		flushRange := b.series.H[flushRaw] - b.series.L[flushRaw]
		flushCloseLocation := 1.0
		if flushRange > 0 {
			flushCloseLocation = (b.series.C[flushRaw] - b.series.L[flushRaw]) / flushRange
		}
		isFlush := b.series.C[flushRaw] < b.series.O[flushRaw] &&
			flushRange >= b.params.DailyFlushFailure.MinFlushRangeATR*atr[retainedIndex-1] &&
			flushCloseLocation <= b.params.DailyFlushFailure.MaxFlushCloseLocation
		failedExtension := b.series.L[rawIndex] < b.series.L[flushRaw] &&
			b.series.C[rawIndex] > b.series.O[rawIndex] &&
			b.series.C[rawIndex] > b.series.C[flushRaw]
		if isFlush && failedExtension {
			hasEntryPending = true
			pendingATR = atr[retainedIndex]
			pendingFlushRangeATR = dailyFlushFailureDiagnosticNumber(flushRange / atr[retainedIndex-1])
			pendingFlushCloseLocation = dailyFlushFailureDiagnosticNumber(flushCloseLocation)
			pendingStop = math.Min(b.series.L[rawIndex], b.series.L[flushRaw]) +
				-b.params.StopBufferATR*pendingATR
		}
	}
	if end >= 0 && b.hasPosition {
		b.closePosition(b.series.C[end], end, "eod")
	}
	return b.trades
}
